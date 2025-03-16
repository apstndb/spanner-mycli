package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
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
	"github.com/ngicks/go-iterator-helper/x/exp/xiter"
	"github.com/nyaosorg/go-readline-ny"
	"github.com/nyaosorg/go-readline-ny/keys"
	"github.com/nyaosorg/go-readline-ny/simplehistory"
	"github.com/samber/lo"
)

// This file contains readline related code

type History interface {
	readline.IHistory
	Add(string)
}
type persistentHistory struct {
	filename string
	history  *simplehistory.Container
}

func (p *persistentHistory) Len() int {
	return p.history.Len()
}

func (p *persistentHistory) At(i int) string {
	return p.history.At(i)
}

func (p *persistentHistory) Add(s string) {
	p.history.Add(s)
	file, err := os.OpenFile(p.filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
	if err != nil {
		log.Println(err)
		return
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			log.Println(err)
		}
	}(file)
	_, err = fmt.Fprintf(file, "%q\n", s)
	if err != nil {
		log.Println(err)
	}
}

func newPersistentHistory(filename string, h *simplehistory.Container) (History, error) {
	b, err := os.ReadFile(filename)
	if errors.Is(err, os.ErrNotExist) {
		return &persistentHistory{filename: filename, history: h}, nil
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
	return &persistentHistory{filename: filename, history: h}, nil
}

func initializeMultilineEditor(c *Cli) (*multiline.Editor, History, error) {
	ed := &multiline.Editor{}

	err := ed.BindKey(keys.CtrlJ, readline.AnonymousCommand(ed.NewLine))
	if err != nil {
		return nil, nil, err
	}

	history, err := setupHistory(ed, c.SystemVariables.HistoryFile)
	if err != nil {
		return nil, nil, err
	}

	ed.SubmitOnEnterWhen(func(lines []string, _ int) bool {
		statements, err := separateInput(strings.Join(lines, "\n"))

		// Continue with waiting prompt if there is an error with waiting status
		if e, ok := lo.ErrorsAs[*gsqlutils.ErrLexerStatus](err); ok {
			c.waitingStatus = e.WaitingString
			return false
		}

		// reset waitingStatus
		c.waitingStatus = ""

		// Submit if there is an error or completed statement.
		return err != nil || len(statements) > 1 || (len(statements) == 1 && statements[0].delim != delimiterUndefined)
	})

	ed.SetPrompt(PS1PS2FuncToPromptFunc(
		func() string {
			return c.getInterpolatedPrompt(c.SystemVariables.Prompt)
		},
		func(ps1 string) string {
			lastLineOfPrompt := lo.LastOrEmpty(strings.Split(ps1, "\n"))

			prompt2, needPadding := strings.CutPrefix(c.SystemVariables.Prompt2, "%P")
			interpolatedPrompt2 := c.getInterpolatedPrompt(prompt2)
			return lo.Ternary(needPadding, runewidth.FillLeft(interpolatedPrompt2, runewidth.StringWidth(lastLineOfPrompt)), interpolatedPrompt2)
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

const errMessageUnclosedTripleQuotedStringLiteral = `unclosed triple-quoted string literal`
const errMessageUnclosedStringLiteral = `unclosed string literal`
const errMessageUnclosedComment = `unclosed comment`

func commentHighlighter() highlighterFunc {
	return lexerHighlighterWithError(func(tok token.Token) [][]int {
		return slices.Collect(xiter.Map(func(comment token.TokenComment) []int {
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

func readInteractiveInput(ctx context.Context, ed *multiline.Editor) (*inputStatement, error) {
	lines, err := ed.Read(ctx)
	if err != nil {
		if len(lines) == 0 {
			return nil, err
		}

		str := strings.Join(lines, "\n")
		return &inputStatement{
			statement:                str,
			statementWithoutComments: str,
			delim:                    "",
		}, err
	}

	input := strings.Join(lines, "\n") + "\n"

	statements, err := separateInput(input)
	if err != nil {
		return nil, err
	}

	switch len(statements) {
	case 0:
		return nil, errors.New("no input")
	case 1:
		return &statements[0], nil
	default:
		return nil, errors.New("sql queries are limited to single statements in interactive mode")
	}
}

func isInterrupted(err error) bool {
	return errors.Is(err, readline.CtrlC)
}
