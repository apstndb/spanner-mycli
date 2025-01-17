//
// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package main

import (
	"bufio"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"log"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/apstndb/adcplus"
	"github.com/kballard/go-shellquote"
	"github.com/ngicks/go-iterator-helper/hiter/stringsiter"
	"github.com/nyaosorg/go-readline-ny"

	"github.com/hymkor/go-multiline-ny"
	"github.com/nyaosorg/go-readline-ny/keys"
	"github.com/nyaosorg/go-readline-ny/simplehistory"

	"github.com/mattn/go-runewidth"

	"golang.org/x/term"

	"github.com/apstndb/adcplus/tokensource"
	"github.com/apstndb/gsqlutils"
	"github.com/apstndb/lox"
	"github.com/ngicks/go-iterator-helper/x/exp/xiter"

	"github.com/ngicks/go-iterator-helper/hiter"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/timestamppb"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/olekukonko/tablewriter"

	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
)

type DisplayMode int

const (
	DisplayModeTable DisplayMode = iota
	DisplayModeVertical
	DisplayModeTab
)

const (
	exitCodeSuccess = 0
	exitCodeError   = 1
)

type Cli struct {
	Session         *Session
	Credential      []byte
	InStream        io.ReadCloser
	OutStream       io.Writer
	ErrStream       io.Writer
	SystemVariables *systemVariables
	waitingStatus   string
}

type command struct {
	Stmt Statement
}

func NewCli(ctx context.Context, credential []byte, inStream io.ReadCloser, outStream, errStream io.Writer, sysVars *systemVariables) (*Cli, error) {
	session, err := createSession(ctx, credential, sysVars)
	if err != nil {
		return nil, err
	}

	return &Cli{
		Session:         session,
		Credential:      credential,
		InStream:        inStream,
		OutStream:       outStream,
		ErrStream:       errStream,
		SystemVariables: sysVars,
	}, nil
}

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

func PS1PS2FuncToPromptFunc(ps1F func() string, ps2F func(ps1 string) string) func(w io.Writer, lnum int) (int, error) {
	return func(w io.Writer, lnum int) (int, error) {
		if lnum == 0 {
			return io.WriteString(w, ps1F())
		}
		return io.WriteString(w, ps2F(ps1F()))
	}
}

func (c *Cli) RunInteractive(ctx context.Context) int {
	var ed multiline.Editor

	type ac = readline.AnonymousCommand
	type _ = ac

	// TODO: There is no multiline version of CmdISearchBackward.
	err := ed.BindKey(keys.CtrlR, readline.CmdISearchBackward)
	if err != nil {
		return c.ExitOnError(err)
	}

	history, err := newPersistentHistory(c.SystemVariables.HistoryFile, simplehistory.New())
	if err != nil {
		return c.ExitOnError(err)
	}
	ed.SetHistory(history)
	ed.SetHistoryCycling(true)

	exists, err := c.Session.DatabaseExists()
	if err != nil {
		return c.ExitOnError(err)
	}
	if exists {
		fmt.Fprintf(c.OutStream, "Connected.\n")
	} else {
		return c.ExitOnError(fmt.Errorf("unknown database %q", c.SystemVariables.Database))
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

	// ensure reset
	c.waitingStatus = ""

	for {
		input, err := readInteractiveInput(ctx, &ed)

		// reset default
		ed.SetDefault(nil)

		if errors.Is(err, io.EOF) {
			return c.Exit()
		}
		if errors.Is(err, readline.CtrlC) {
			c.PrintInteractiveError(err)
			continue
		}
		if err != nil {
			c.PrintInteractiveError(err)
			continue
		}

		stmt, err := BuildStatementWithCommentsWithMode(input.statementWithoutComments, input.statement, c.SystemVariables.BuildStatementMode)
		if err != nil {
			c.PrintInteractiveError(err)
			continue
		}
		history.Add(input.statement + ";")

		var disableSpinner bool
		if _, ok := stmt.(*ExitStatement); ok {
			return c.Exit()
		}

		if s, ok := stmt.(*UseStatement); ok {
			newSystemVariables := *c.SystemVariables

			newSystemVariables.Database = s.Database
			newSystemVariables.Role = s.Role

			newSession, err := createSession(ctx, c.Credential, &newSystemVariables)
			if err != nil {
				c.PrintInteractiveError(err)
				continue
			}

			exists, err := newSession.DatabaseExists()
			if err != nil {
				newSession.Close()
				c.PrintInteractiveError(err)
				continue
			}
			if !exists {
				newSession.Close()
				c.PrintInteractiveError(fmt.Errorf("unknown database %q\n", s.Database))
				continue
			}

			c.Session.Close()
			c.Session = newSession

			c.SystemVariables = &newSystemVariables

			fmt.Fprintf(c.OutStream, "Database changed")
			continue
		}

		if s, ok := stmt.(*DropDatabaseStatement); ok {
			if c.SystemVariables.Database == s.DatabaseId {
				c.PrintInteractiveError(
					fmt.Errorf("database %q is currently used, it can not be dropped", s.DatabaseId),
				)
				continue
			}

			if !confirm(c.OutStream, fmt.Sprintf("Database %q will be dropped.\nDo you want to continue?", s.DatabaseId)) {
				continue
			}
		}
		switch stmt.(type) {
		case *DdlStatement, *SyncProtoStatement, *BulkDdlStatement, *RunBatchStatement:
			disableSpinner = true
		}

		// Execute the statement.
		ctx, cancel := context.WithCancel(ctx)
		go handleInterrupt(cancel)
		stop := lo.TernaryF(disableSpinner,
			func() func() { return func() {} },
			func() func() { return c.PrintProgressingMark() })
		t0 := time.Now()
		result, err := c.Session.ExecuteStatement(ctx, stmt)
		elapsed := time.Since(t0).Seconds()
		stop()
		if err != nil {
			if spanner.ErrCode(err) == codes.Aborted {
				// Once the transaction is aborted, the underlying session gains higher lock priority for the next transaction.
				// This makes the result of subsequent transaction in spanner-cli inconsistent, so we recreate the client to replace
				// the Cloud Spanner's session with new one to revert the lock priority of the session.
				// See: https://cloud.google.com/spanner/docs/reference/rest/v1/TransactionOptions#retrying-aborted-transactions
				innerErr := c.Session.RecreateClient()
				if innerErr != nil {
					err = errors.Join(err, innerErr)
				}
			}
			c.PrintInteractiveError(err)
			cancel()
			continue
		}

		// only SELECT statement has the elapsed time measured by the server
		if result.Stats.ElapsedTime == "" {
			result.Stats.ElapsedTime = fmt.Sprintf("%0.2f sec", elapsed)
		}

		if !result.KeepVariables {
			c.updateSystemVariables(result)
		}

		size := math.MaxInt
		if c.SystemVariables.AutoWrap {
			size, _, err = term.GetSize(int(os.Stdout.Fd()))
			if err != nil {
				size = math.MaxInt
			}
		}

		c.PrintResult(size, result, c.SystemVariables.CLIFormat, true)

		fmt.Fprintf(c.OutStream, "\n")
		cancel()

		ed.SetDefault(strings.Split(result.PreInput, "\n"))
	}
}

func (c *Cli) updateSystemVariables(result *Result) {
	if result.IsMutation {
		c.SystemVariables.ReadTimestamp = time.Time{}
		c.SystemVariables.CommitTimestamp = result.Timestamp
	} else {
		c.SystemVariables.ReadTimestamp = result.Timestamp
		c.SystemVariables.CommitTimestamp = time.Time{}
	}

	if result.CommitStats != nil {
		c.SystemVariables.CommitResponse = &sppb.CommitResponse{CommitStats: result.CommitStats, CommitTimestamp: timestamppb.New(result.Timestamp)}
	} else {
		c.SystemVariables.CommitResponse = nil
	}
}

func (c *Cli) RunBatch(ctx context.Context, input string) int {
	cmds, err := buildCommands(input, c.SystemVariables.BuildStatementMode)
	if err != nil {
		c.PrintBatchError(err)
		return exitCodeError
	}

	ctx, cancel := context.WithCancel(ctx)
	go handleInterrupt(cancel)

	for _, cmd := range cmds {
		result, err := c.Session.ExecuteStatement(ctx, cmd.Stmt)
		if err != nil {
			c.PrintBatchError(err)
			return exitCodeError
		}

		if !result.KeepVariables {
			c.updateSystemVariables(result)
		}

		c.PrintResult(math.MaxInt, result, c.SystemVariables.CLIFormat, false)
	}

	return exitCodeSuccess
}

func (c *Cli) Exit() int {
	c.Session.Close()
	fmt.Fprintln(c.OutStream, "Bye")
	return exitCodeSuccess
}

func (c *Cli) ExitOnError(err error) int {
	c.Session.Close()
	printError(c.ErrStream, err)
	return exitCodeError
}

func (c *Cli) PrintInteractiveError(err error) {
	printError(c.OutStream, err)
}

func printError(w io.Writer, err error) {
	if code := spanner.ErrCode(err); code != codes.Unknown {
		before, _, found := strings.Cut(err.Error(), " spanner:")
		if !found {
			fmt.Fprintf(w, "ERROR: %s\n", err)
			return
		}

		desc := spanner.ErrDesc(err)
		if unquoted, err := strconv.Unquote(`"` + desc + `"`); err != nil {
			fmt.Fprintf(w, "ERROR: %v spanner: code=%q, desc: %v\n", before, code, desc)
		} else {
			fmt.Fprintf(w, "ERROR: %v spanner: code=%q, desc: %v\n", before, code, unquoted)
		}
	} else {
		fmt.Fprintf(w, "ERROR: %s\n", err)
	}
}

func (c *Cli) PrintBatchError(err error) {
	printError(c.ErrStream, err)
}

func (c *Cli) PrintResult(screenWidth int, result *Result, mode DisplayMode, interactive bool) {
	ostream := c.OutStream
	var cmd *exec.Cmd
	if c.SystemVariables.UsePager {
		pagerpath := cmp.Or(os.Getenv("PAGER"), "less")

		split, err := shellquote.Split(pagerpath)
		if err != nil {
			return
		}
		cmd = exec.CommandContext(context.Background(), split[0], split[1:]...)

		pr, pw := io.Pipe()
		ostream = pw
		cmd.Stdin = pr
		cmd.Stdout = c.OutStream

		err = cmd.Start()
		if err != nil {
			log.Println(err)
			return
		}
		defer func() {
			err := pw.Close()
			if err != nil {
				log.Println(err)
			}
			err = cmd.Wait()
			if err != nil {
				log.Println(err)
			}
		}()
	}
	printResult(c.SystemVariables.Debug, screenWidth, ostream, result, mode, interactive, c.SystemVariables.Verbose)
}

func (c *Cli) PrintProgressingMark() func() {
	progressMarks := []string{`-`, `\`, `|`, `/`}
	ticker := time.NewTicker(time.Millisecond * 100)
	go func() {
		// wait to avoid corruption with first output of command
		<-ticker.C

		i := 0
		for {
			<-ticker.C
			mark := progressMarks[i%len(progressMarks)]
			fmt.Fprintf(c.OutStream, "\r%s", mark)
			i++
		}
	}()

	stop := func() {
		ticker.Stop()
		fmt.Fprintf(c.OutStream, "\r") // clear progressing mark
	}
	return stop
}

var promptRe = regexp.MustCompile(`(%[^{])|%\{[^}]+}`)
var promptSystemVariableRe = regexp.MustCompile(`%\{([^}]+)}`)

func (c *Cli) getInterpolatedPrompt(prompt string) string {
	return promptRe.ReplaceAllStringFunc(prompt, func(s string) string {
		return lo.Switch[string, string](s).
			Case("%%", "%").
			Case("%n", "\n").
			Case("%p", c.SystemVariables.Project).
			Case("%i", c.SystemVariables.Instance).
			Case("%d", c.SystemVariables.Database).
			Case("%t", lo.
				If(c.Session.InReadWriteTransaction(), "(rw txn)").
				ElseIf(c.Session.InReadOnlyTransaction(), "(ro txn)").
				ElseIf(c.Session.InPendingTransaction(), "(txn)").
				Else("")).
			Case("%R", runewidth.FillLeft(
				lo.CoalesceOrEmpty(strings.ReplaceAll(c.waitingStatus, "*/", "/*"), "-"), 3)).
			DefaultF(
				func() string {
					varName := promptSystemVariableRe.FindStringSubmatch(s)[1]
					value, err := c.SystemVariables.Get(varName)
					if err != nil {
						return fmt.Sprintf("INVALID_VAR{%v}", varName)
					}
					return value[varName]
				},
			)
	})
}

func createSession(ctx context.Context, credential []byte, sysVars *systemVariables) (*Session, error) {
	var opts []option.ClientOption
	if sysVars.Endpoint != "" {
		opts = append(opts, option.WithEndpoint(sysVars.Endpoint))
	}

	if sysVars.EnableADCPlus {
		source, err := tokensource.SmartAccessTokenSource(ctx, adcplus.WithCredentialsJSON(credential), adcplus.WithTargetPrincipal(sysVars.ImpersonateServiceAccount))
		if err != nil {
			return nil, err
		}
		opts = append(opts, option.WithTokenSource(source))
	} else {
		if len(credential) > 0 {
			opts = append(opts, option.WithCredentialsJSON(credential))
		}
	}

	return NewSession(ctx, sysVars, opts...)
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

func maxWidth(s string) int {
	return hiter.Max(xiter.Map(
		runewidth.StringWidth,
		stringsiter.SplitFunc(s, 0, stringsiter.CutNewLine)))
}

func clipToMax[S interface{ ~[]E }, E cmp.Ordered](s S, maxValue E) iter.Seq[E] {
	return xiter.Map(
		func(in E) E {
			return min(in, maxValue)
		},
		slices.Values(s),
	)
}

func adjustToSum(limit int, vs []int) ([]int, int) {
	sumVs := lo.Sum(vs)
	remains := limit - sumVs
	if remains >= 0 {
		return vs, remains
	}

	curVs := vs
	for i := 1; ; i++ {
		rev := lo.Reverse(slices.Sorted(slices.Values(lo.Uniq(vs))))
		v, ok := hiter.Nth(i, slices.Values(rev))
		if !ok {
			break
		}
		curVs = slices.Collect(clipToMax(vs, v))
		if lo.Sum(curVs) <= limit {
			break
		}
	}
	return curVs, limit - lo.Sum(curVs)
}

var invalidWidthCount = WidthCount{
	// impossible to fit any width
	width: math.MaxInt,
	// least significant
	count: math.MinInt,
}

func maxIndex(ignoreMax int, adjustWidths []int, seq iter.Seq[WidthCount]) (int, WidthCount) {
	return MaxByWithIdx(
		invalidWidthCount,
		WidthCount.Count,
		hiter.Unify(
			func(adjustWidth int, wc WidthCount) WidthCount {
				return lo.Ternary(wc.Length()-adjustWidth <= ignoreMax, wc, invalidWidthCount)
			},
			hiter.Pairs(slices.Values(adjustWidths), seq)))
}

func calculateOptimalWidth(debug bool, screenWidth int, header []string, rows []Row) []int {

	// table overhead is:
	// len(`|  |`) +
	// len(` | `) * len(columns) - 1
	overheadWidth := 4 + 3*(len(header)-1)

	// don't mutate
	termWidthWithoutOverhead := screenWidth - overheadWidth

	if debug {
		log.Printf("screenWitdh: %v, remainsWidth: %v", screenWidth, termWidthWithoutOverhead)
	}

	formatIntermediate := func(remainsWidth int, adjustedWidths []int) string {
		return fmt.Sprintf("remaining %v, adjustedWidths: %v", remainsWidth-lo.Sum(adjustedWidths), adjustedWidths)
	}

	adjustedWidths := adjustByHeader(header, termWidthWithoutOverhead)

	if debug {
		log.Println("adjustByName:", formatIntermediate(termWidthWithoutOverhead, adjustedWidths))
	}

	var transposedRows [][]string
	for columnIdx := range len(header) {
		transposedRows = append(transposedRows, slices.Collect(
			xiter.Map(
				func(in Row) string {
					return lo.Must(lo.Nth(in.Columns, columnIdx))
				},
				xiter.Concat(hiter.Once(toRow(header...)), slices.Values(rows)),
			)))
	}

	widthCounts := calculateWidthCounts(adjustedWidths, transposedRows)
	for {
		if debug {
			log.Println("widthCounts:", widthCounts)
		}

		firstCounts :=
			xiter.Map(
				func(wcs []WidthCount) WidthCount {
					return lo.FirstOr(wcs, invalidWidthCount)
				},
				slices.Values(widthCounts))

		// find the largest count idx within available width
		idx, target := maxIndex(termWidthWithoutOverhead-lo.Sum(adjustedWidths), adjustedWidths, firstCounts)
		if idx < 0 || target.Count() < 1 {
			break
		}

		widthCounts[idx] = widthCounts[idx][1:]
		adjustedWidths[idx] = target.Length()

		if debug {
			log.Println("adjusting:", formatIntermediate(termWidthWithoutOverhead, adjustedWidths))
		}
	}

	if debug {
		log.Println("semi final:", formatIntermediate(termWidthWithoutOverhead, adjustedWidths))
	}

	// Add rest to the longest shortage column.
	longestWidths := lo.Map(widthCounts, func(item []WidthCount, _ int) int {
		return hiter.Max(xiter.Map(WidthCount.Length, slices.Values(item)))
	})

	idx, _ := MaxWithIdx(math.MinInt, hiter.Unify(
		func(longestWidth, adjustedWidth int) int {
			return longestWidth - adjustedWidth
		},
		hiter.Pairs(slices.Values(longestWidths), slices.Values(adjustedWidths))))

	if idx != -1 {
		adjustedWidths[idx] += termWidthWithoutOverhead - lo.Sum(adjustedWidths)
	}

	if debug {
		log.Println("final:", formatIntermediate(termWidthWithoutOverhead, adjustedWidths))
	}

	return adjustedWidths
}

func MaxWithIdx[E cmp.Ordered](fallback E, seq iter.Seq[E]) (int, E) {
	return MaxByWithIdx(fallback, lox.Identity, seq)
}

func MaxByWithIdx[O cmp.Ordered, E any](fallback E, f func(E) O, seq iter.Seq[E]) (int, E) {
	val := fallback
	idx := -1
	current := -1
	for v := range seq {
		current++
		if f(val) < f(v) {
			val = v
			idx = current
		}
	}
	return idx, val
}

func countWidth(ss []string) iter.Seq[WidthCount] {
	return xiter.Map(
		func(e lo.Entry[int, int]) WidthCount {
			return WidthCount{
				width: e.Key,
				count: e.Value,
			}
		},
		slices.Values(lox.EntriesSortedByKey(lo.CountValuesBy(ss, maxWidth))))
}

func calculateWidthCounts(currentWidths []int, rows [][]string) [][]WidthCount {
	var result [][]WidthCount
	for columnNo := range len(currentWidths) {
		currentWidth := currentWidths[columnNo]
		columnValues := rows[columnNo]
		largerWidthCounts := slices.Collect(
			xiter.Filter(
				func(v WidthCount) bool {
					return v.Length() > currentWidth
				},
				countWidth(columnValues),
			))
		result = append(result, largerWidthCounts)
	}
	return result
}

type WidthCount struct{ width, count int }

func (wc WidthCount) Length() int { return wc.width }
func (wc WidthCount) Count() int  { return wc.count }

func adjustByHeader(headers []string, availableWidth int) []int {
	nameWidths := slices.Collect(xiter.Map(runewidth.StringWidth, slices.Values(headers)))

	adjustWidths, _ := adjustToSum(availableWidth, nameWidths)

	return adjustWidths
}

func printResult(debug bool, screenWidth int, out io.Writer, result *Result, mode DisplayMode, interactive, verbose bool) {
	// screenWidth <= means no limit.
	if screenWidth <= 0 {
		screenWidth = math.MaxInt
	}

	switch {
	case mode == DisplayModeTable:
		table := tablewriter.NewWriter(out)
		table.SetAutoFormatHeaders(false)
		table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
		table.SetAlignment(tablewriter.ALIGN_LEFT)
		table.SetAutoWrapText(false)
		if len(result.ColumnAlign) > 0 {
			table.SetColumnAlignment(result.ColumnAlign)
		}
		var adjustedWidths []int
		if len(result.ColumnTypes) > 0 {
			names := slices.Collect(xiter.Map(
				(*sppb.StructType_Field).GetName,
				slices.Values(result.ColumnTypes),
			))
			header := slices.Collect(xiter.Map(formatTypedHeaderColumn, slices.Values(result.ColumnTypes)))
			adjustedWidths = calculateOptimalWidth(debug, screenWidth, names, slices.Concat(sliceOf(toRow(header...)), result.Rows))
		} else {
			adjustedWidths = calculateOptimalWidth(debug, screenWidth, result.ColumnNames, slices.Concat(sliceOf(toRow(result.ColumnNames...)), result.Rows))
		}
		var forceTableRender bool
		if verbose && len(result.ColumnTypes) > 0 {
			forceTableRender = true

			headers := slices.Collect(hiter.Unify(
				runewidth.Wrap,
				hiter.Pairs(
					xiter.Map(formatTypedHeaderColumn, slices.Values(result.ColumnTypes)),
					slices.Values(adjustedWidths))),
			)
			table.SetHeader(headers)
		} else {
			table.SetHeader(result.ColumnNames)
		}
		for _, row := range result.Rows {
			wrappedColumns := slices.Collect(hiter.Unify(
				runewidth.Wrap,
				hiter.Pairs(slices.Values(row.Columns), slices.Values(adjustedWidths))),
			)
			table.Append(wrappedColumns)
		}
		if forceTableRender || len(result.Rows) > 0 {
			table.Render()
		}
	case mode == DisplayModeVertical:
		maxLen := 0
		for _, columnName := range result.ColumnNames {
			if len(columnName) > maxLen {
				maxLen = len(columnName)
			}
		}
		format := fmt.Sprintf("%%%ds: %%s\n", maxLen)
		for i, row := range result.Rows {
			fmt.Fprintf(out, "*************************** %d. row ***************************\n", i+1)
			for j, column := range row.Columns {
				fmt.Fprintf(out, format, result.ColumnNames[j], column)
			}
		}
	case mode == DisplayModeTab:
		if len(result.ColumnNames) > 0 {
			fmt.Fprintln(out, strings.Join(result.ColumnNames, "\t"))
			for _, row := range result.Rows {
				fmt.Fprintln(out, strings.Join(row.Columns, "\t"))
			}
		}
	}

	if len(result.Predicates) > 0 {
		fmt.Fprintln(out, "Predicates(identified by ID):")
		for _, s := range result.Predicates {
			fmt.Fprintf(out, " %s\n", s)
		}
		fmt.Fprintln(out)
	}

	if len(result.LintResults) > 0 {
		fmt.Fprintln(out, "Experimental Lint Result:")
		for _, s := range result.LintResults {
			fmt.Fprintf(out, " %s\n", s)
		}
		fmt.Fprintln(out)
	}
	if verbose || result.ForceVerbose {
		fmt.Fprint(out, resultLine(result, true))
	} else if interactive {
		fmt.Fprint(out, resultLine(result, verbose))
	}
}

func formatTypedHeaderColumn(field *sppb.StructType_Field) string {
	return field.GetName() + "\n" + formatTypeSimple(field.GetType())
}

func resultLine(result *Result, verbose bool) string {
	var timestamp string
	if !result.Timestamp.IsZero() {
		timestamp = result.Timestamp.Format(time.RFC3339Nano)
	}

	// FIXME: Currently, ElapsedTime is not populated in batch mode.
	elapsedTimePart := lox.IfOrEmpty(result.Stats.ElapsedTime != "", fmt.Sprintf(" (%s)", result.Stats.ElapsedTime))

	var batchInfo string
	switch {
	case result.BatchInfo == nil:
		break
	default:
		batchInfo = fmt.Sprintf(" (%d %s%s in batch)", result.BatchInfo.Size,
			lo.Ternary(result.BatchInfo.Mode == batchModeDDL, "DDL", "DML"),
			lox.IfOrEmpty(result.BatchInfo.Size > 1, "s"),
		)
	}

	if result.IsMutation {
		var affectedRowsPrefix string
		switch result.AffectedRowsType {
		case rowCountTypeLowerBound:
			// For Partitioned DML the result's row count is lower bounded number, so we add "at least" to express ambiguity.
			// See https://cloud.google.com/spanner/docs/reference/rpc/google.spanner.v1?hl=en#resultsetstats
			affectedRowsPrefix = "at least "
		case rowCountTypeUpperBound:
			// For batch DML, same rows can be processed by statements.
			affectedRowsPrefix = "at most "
		}

		var detail string
		if verbose {
			if timestamp != "" {
				detail += fmt.Sprintf("timestamp:      %s\n", timestamp)
			}
			if result.CommitStats != nil {
				detail += fmt.Sprintf("mutation_count: %d\n", result.CommitStats.GetMutationCount())
			}
		}
		return fmt.Sprintf("Query OK, %s%d rows affected%s%s\n%s",
			affectedRowsPrefix, result.AffectedRows, elapsedTimePart, batchInfo, detail)
	}

	var set string
	if result.AffectedRows == 0 {
		set = "Empty set"
	} else {
		set = fmt.Sprintf("%d rows in set%s", result.AffectedRows, batchInfo)
	}

	if verbose {
		// detail is aligned with max length of key (current: 20)
		var detail string
		if timestamp != "" {
			detail += fmt.Sprintf("timestamp:            %s\n", timestamp)
		}
		if result.Stats.CPUTime != "" {
			detail += fmt.Sprintf("cpu time:             %s\n", result.Stats.CPUTime)
		}
		if result.Stats.RowsScanned != "" {
			detail += fmt.Sprintf("rows scanned:         %s rows\n", result.Stats.RowsScanned)
		}
		if result.Stats.DeletedRowsScanned != "" {
			detail += fmt.Sprintf("deleted rows scanned: %s rows\n", result.Stats.DeletedRowsScanned)
		}
		if result.Stats.OptimizerVersion != "" {
			detail += fmt.Sprintf("optimizer version:    %s\n", result.Stats.OptimizerVersion)
		}
		if result.Stats.OptimizerStatisticsPackage != "" {
			detail += fmt.Sprintf("optimizer statistics: %s\n", result.Stats.OptimizerStatisticsPackage)
		}
		return fmt.Sprintf("%s%s\n%s", set, elapsedTimePart, detail)
	}
	return fmt.Sprintf("%s%s\n", set, elapsedTimePart)
}

func buildCommands(input string, mode parseMode) ([]*command, error) {
	var cmds []*command
	var pendingDdls []string

	stmts, err := separateInput(input)
	if err != nil {
		return nil, err
	}
	for _, separated := range stmts {
		// Ignore the last empty statement
		if separated.delim == delimiterUndefined && separated.statementWithoutComments == "" {
			continue
		}

		stmt, err := BuildStatementWithCommentsWithMode(strings.TrimSpace(separated.statementWithoutComments), separated.statement, mode)
		if err != nil {
			return nil, fmt.Errorf("failed with statement, error: %w, statement: %q, without comments: %q", err, separated.statement, separated.statementWithoutComments)
		}
		if ddl, ok := stmt.(*DdlStatement); ok {
			pendingDdls = append(pendingDdls, ddl.Ddl)
			continue
		}

		// Flush pending DDLs
		if len(pendingDdls) > 0 {
			cmds = append(cmds, &command{&BulkDdlStatement{pendingDdls}})
			pendingDdls = nil
		}

		cmds = append(cmds, &command{stmt})
	}

	// Flush pending DDLs
	if len(pendingDdls) > 0 {
		cmds = append(cmds, &command{&BulkDdlStatement{pendingDdls}})
	}

	return cmds, nil
}

func confirm(out io.Writer, msg string) bool {
	fmt.Fprintf(out, "%s [yes/no] ", msg)

	s := bufio.NewScanner(os.Stdin)
	for {
		s.Scan()
		switch strings.ToLower(s.Text()) {
		case "yes":
			return true
		case "no":
			return false
		default:
			fmt.Fprint(out, "Please answer yes or no: ")
		}
	}
}

func handleInterrupt(cancel context.CancelFunc) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
	cancel()
}
