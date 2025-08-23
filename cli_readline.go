package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/apstndb/gsqlutils"
	"github.com/apstndb/lox"
	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/token"
	"github.com/fatih/color"
	"github.com/hymkor/go-multiline-ny"
	"github.com/mattn/go-runewidth"
	"github.com/ngicks/go-iterator-helper/hiter"
	"github.com/nyaosorg/go-readline-ny"
	"github.com/nyaosorg/go-readline-ny/keys"
	"github.com/nyaosorg/go-readline-ny/simplehistory"
	"github.com/samber/lo"
	"github.com/spf13/afero"
)

// This file contains readline related code

type History interface {
	readline.IHistory
	Add(string)
}
type persistentHistory struct {
	filename string
	history  *simplehistory.Container
	fs       afero.Fs
}

func (p *persistentHistory) Len() int {
	return p.history.Len()
}

func (p *persistentHistory) At(i int) string {
	return p.history.At(i)
}

func (p *persistentHistory) Add(s string) {
	p.history.Add(s)
	file, err := p.fs.OpenFile(p.filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		slog.Error("failed to open history file", "file", p.filename, "err", err)
		return
	}
	defer func(file afero.File) {
		err := file.Close()
		if err != nil {
			slog.Error("failed to close history file", "file", p.filename, "err", err)
		}
	}(file)
	_, err = fmt.Fprintf(file, "%q\n", s)
	if err != nil {
		slog.Error("failed to write to history file", "file", p.filename, "err", err)
	}
}

func newPersistentHistory(filename string, h *simplehistory.Container) (History, error) {
	return newPersistentHistoryWithFS(filename, h, afero.NewOsFs())
}

func newPersistentHistoryWithFS(filename string, h *simplehistory.Container, fs afero.Fs) (History, error) {
	b, err := afero.ReadFile(fs, filename)
	if errors.Is(err, os.ErrNotExist) {
		return &persistentHistory{filename: filename, history: h, fs: fs}, nil
	}
	if err != nil {
		return nil, err
	}
	for _, s := range strings.Split(string(b), "\n") {
		if s == "" {
			continue
		}
		unquoted, err := strconv.Unquote(s)
		if err != nil {
			return nil, fmt.Errorf("history file format error, maybe you should remove %v, err: %w", filename, err)
		}
		h.Add(unquoted)
	}
	return &persistentHistory{filename: filename, history: h, fs: fs}, nil
}

// shouldSubmitStatement determines if the input should be submitted based on statements and errors
func shouldSubmitStatement(statements []inputStatement, err error) (shouldSubmit bool, waitingStatus string) {
	// Continue with waiting prompt if there is an error with waiting status
	if e, ok := lo.ErrorsAs[*gsqlutils.ErrLexerStatus](err); ok {
		return false, e.WaitingString
	}

	// Submit if there is an error or completed statement.
	shouldSubmit = err != nil || len(statements) > 1 || (len(statements) == 1 && statements[0].delim != delimiterUndefined)
	return shouldSubmit, ""
}

// generatePS2Prompt generates the continuation prompt (PS2) based on PS1 and PS2 template
func generatePS2Prompt(ps1 string, ps2Template string, ps2Interpolated string) string {
	lastLineOfPrompt := lo.LastOrEmpty(strings.Split(ps1, "\n"))
	_, needPadding := strings.CutPrefix(ps2Template, "%P")
	return lo.Ternary(needPadding, runewidth.FillLeft(ps2Interpolated, runewidth.StringWidth(lastLineOfPrompt)), ps2Interpolated)
}

func initializeMultilineEditor(c *Cli) (*multiline.Editor, History, error) {
	ed := &multiline.Editor{}

	// Configure the LineEditor with proper I/O streams
	// Use TtyOutStream for readline to avoid polluting tee output with prompts.
	// Interactive mode requires a TTY for output.
	ttyStream := c.SystemVariables.StreamManager.GetTtyStream()
	if ttyStream == nil {
		return nil, nil, fmt.Errorf("cannot run in interactive mode: stdout is not a terminal")
	}
	ed.LineEditor.Writer = ttyStream

	err := ed.BindKey(keys.CtrlJ, readline.AnonymousCommand(ed.NewLine))
	if err != nil {
		return nil, nil, err
	}

	history, err := setupHistory(ed, c.SystemVariables.HistoryFile)
	if err != nil {
		return nil, nil, err
	}

	ed.SubmitOnEnterWhen(func(lines []string, _ int) bool {
		text := strings.Join(lines, "\n")

		// Meta commands are submitted immediately on Enter
		if IsMetaCommand(strings.TrimSpace(text)) {
			c.waitingStatus = ""
			return true
		}

		statements, err := separateInput(text)
		shouldSubmit, newWaitingStatus := shouldSubmitStatement(statements, err)
		c.waitingStatus = newWaitingStatus
		return shouldSubmit
	})

	ed.SetPrompt(PS1PS2FuncToPromptFunc(
		func() string {
			return c.getInterpolatedPrompt(c.SystemVariables.Prompt)
		},
		func(ps1 string) string {
			prompt2, _ := strings.CutPrefix(c.SystemVariables.Prompt2, "%P")
			interpolatedPrompt2 := c.getInterpolatedPrompt(prompt2)
			return generatePS2Prompt(ps1, c.SystemVariables.Prompt2, interpolatedPrompt2)
		}))

	return ed, history, nil
}

func PS1PS2FuncToPromptFunc(ps1F func() string, ps2F func(ps1 string) string) func(w io.Writer, lnum int) (int, error) {
	return func(w io.Writer, lnum int) (int, error) {
		if lnum == 0 {
			return io.WriteString(w, ps1F())
		}
		return io.WriteString(w, ps2F(ps1F()))
	}
}

type highlighter interface {
	FindAllStringIndex(string, int) [][]int
}

var _ highlighter = highlighterFunc(nil)

type highlighterFunc func(string, int) [][]int

func (f highlighterFunc) FindAllStringIndex(s string, i int) [][]int {
	return f(s, i)
}

func lexerHighlighterWithError(f func(tok token.Token) [][]int, errf func(me *memefish.Error) bool) highlighterFunc {
	return func(s string, i int) [][]int {
		var results [][]int
		for tok, err := range gsqlutils.NewLexerSeq("", s) {
			if err != nil {
				if me, ok := lo.ErrorsAs[*memefish.Error](err); ok && errf != nil && errf(me) {
					results = append(results, sliceOf(int(me.Position.Pos), int(me.Position.End)))
				}
				break
			}

			if f != nil {
				results = append(results, f(tok)...)
			}
		}
		return results
	}
}

func errorHighlighter(f func(*memefish.Error) bool) highlighterFunc {
	return lexerHighlighterWithError(nil, f)
}

func lexerHighlighter(f func(tok token.Token) [][]int) highlighterFunc {
	return lexerHighlighterWithError(f, nil)
}

func tokenHighlighter(pred func(tok token.Token) bool) highlighterFunc {
	return lexerHighlighter(func(tok token.Token) [][]int {
		return lox.IfOrEmpty(pred(tok), sliceOf(sliceOf(int(tok.Pos), int(tok.End))))
	})
}

func kindHighlighter(kinds ...token.TokenKind) highlighterFunc {
	return tokenHighlighter(func(tok token.Token) bool {
		return slices.Contains(kinds, tok.Kind)
	})
}

const (
	errMessageUnclosedTripleQuotedStringLiteral = `unclosed triple-quoted string literal`
	errMessageUnclosedStringLiteral             = `unclosed string literal`
	errMessageUnclosedComment                   = `unclosed comment`
)

func commentHighlighter() highlighterFunc {
	return lexerHighlighterWithError(func(tok token.Token) [][]int {
		return slices.Collect(hiter.Map(func(comment token.TokenComment) []int {
			return sliceOf(int(comment.Pos), int(comment.End))
		}, slices.Values(tok.Comments)))
	}, func(me *memefish.Error) bool {
		return me.Message == errMessageUnclosedComment
	})
}

func colorToSequence(attr ...color.Attribute) string {
	var sb strings.Builder
	color.New(attr...).SetWriter(&sb)
	return sb.String()
}

var (
	alnumRe = regexp.MustCompile("^[a-zA-Z0-9]+$")

	defaultHighlights = []readline.Highlight{
		// Note: multiline comments break highlight because of restriction of go-multiline-ny
		{Pattern: commentHighlighter(), Sequence: colorToSequence(color.FgWhite, color.Faint)},

		// string literals(including string-based literals like timestamp literals) and byte literals
		{Pattern: kindHighlighter(token.TokenString, token.TokenBytes), Sequence: colorToSequence(color.FgGreen, color.Bold)},

		// unclosed string literals
		// Note: multiline literals break highlight because of restriction of go-multiline-ny
		{Pattern: errorHighlighter(func(me *memefish.Error) bool {
			return me.Message == errMessageUnclosedStringLiteral || me.Message == errMessageUnclosedTripleQuotedStringLiteral
		}), Sequence: colorToSequence(color.FgHiGreen, color.Bold)},

		// numbers
		{Pattern: kindHighlighter(token.TokenFloat, token.TokenInt), Sequence: colorToSequence(color.FgHiBlue, color.Bold)},

		// params
		{Pattern: kindHighlighter(token.TokenParam), Sequence: colorToSequence(color.FgMagenta, color.Bold)},

		// keywords
		{Pattern: tokenHighlighter(func(tok token.Token) bool {
			return alnumRe.MatchString(string(tok.Kind))
		}), Sequence: colorToSequence(color.FgHiYellow, color.Bold)},

		// idents
		{Pattern: kindHighlighter(token.TokenIdent), Sequence: colorToSequence(color.FgHiWhite)},
	}
)

func setLineEditor(ed *multiline.Editor, enableHighlight bool) {
	if color.NoColor || !enableHighlight {
		ed.Highlight = nil
		ed.DefaultColor = ""
		ed.ResetColor = ""
		return
	}

	ed.Highlight = defaultHighlights
	ed.ResetColor = colorToSequence(color.Reset)
	ed.DefaultColor = colorToSequence(color.Reset)
}

func setupHistory(ed *multiline.Editor, historyFileName string) (History, error) {
	history, err := newPersistentHistory(historyFileName, simplehistory.New())
	if err != nil {
		return nil, err
	}

	ed.SetHistory(history)
	ed.SetHistoryCycling(true)

	return history, nil
}

// processInputLines converts lines and read error into an inputStatement
func processInputLines(lines []string, readErr error) (*inputStatement, error) {
	if readErr != nil {
		if len(lines) == 0 {
			return nil, fmt.Errorf("failed to read input: %w", readErr)
		}

		str := strings.Join(lines, "\n")
		return &inputStatement{
			statement:                str,
			statementWithoutComments: str,
			delim:                    "",
		}, readErr
	}
	return nil, nil
}

// validateInteractiveInput validates that input contains at most one statement for interactive mode
func validateInteractiveInput(statements []inputStatement) (*inputStatement, error) {
	switch len(statements) {
	case 0:
		return nil, errors.New("no input")
	case 1:
		return &statements[0], nil
	default:
		return nil, errors.New("sql queries are limited to single statements in interactive mode")
	}
}

func readInteractiveInput(ctx context.Context, ed *multiline.Editor) (*inputStatement, error) {
	lines, err := ed.Read(ctx)

	// Handle read errors
	if stmt, procErr := processInputLines(lines, err); procErr != nil || stmt != nil {
		return stmt, procErr
	}

	input := strings.Join(lines, "\n") + "\n"

	// Check if this is a meta command (starts with \)
	trimmed := strings.TrimSpace(input)
	if IsMetaCommand(trimmed) {
		// For meta commands, return the raw input without SQL parsing
		return &inputStatement{
			statement:                trimmed,
			statementWithoutComments: trimmed,
			delim:                    "", // Meta commands don't use semicolon delimiters
		}, nil
	}

	statements, err := separateInput(input)
	if err != nil {
		return nil, err
	}

	return validateInteractiveInput(statements)
}

func isInterrupted(err error) bool {
	return errors.Is(err, readline.CtrlC)
}
