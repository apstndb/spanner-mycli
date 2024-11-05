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
	"os/signal"
	"regexp"
	"slices"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/apstndb/lox"
	"golang.org/x/exp/constraints"

	"github.com/ngicks/go-iterator-helper/x/exp/xiter"

	"github.com/chzyer/readline/runes"
	"github.com/ngicks/go-iterator-helper/hiter"
	"github.com/reeflective/readline/inputrc"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/timestamppb"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/olekukonko/tablewriter"
	"github.com/reeflective/readline"
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

func (c *Cli) RunInteractive(ctx context.Context) int {
	shell := readline.NewShell()

	shell.Keymap.Register(map[string]func(){"force-end-of-file": func() {
		switch shell.Line().Len() {
		case 0:
			shell.Display.AcceptLine()
			shell.History.Accept(false, false, io.EOF)
		default:
			shell.Display.AcceptLine()
			shell.History.Accept(false, false, nil)
		}
	}})

	err := shell.Config.Bind("emacs", inputrc.Unescape(`\C-D`), "force-end-of-file", false)
	if err != nil {
		return c.ExitOnError(err)
	}

	shell.AcceptMultiline = func(line []rune) (accept bool) {
		statements, err := separateInput(string(line))
		if e, ok := lo.ErrorsAs[*ErrLexerStatus](err); ok {
			shell.Prompt.Secondary(func() string {
				return e.WaitingString + "->"
			})
			return false
		}
		if err != nil {
			return true
		}
		switch len(statements) {
		case 0:
			return false
		case 1:
			if statements[0].delim != delimiterUndefined {
				return true
			}
		default:
			return true
		}
		return false
	}

	shell.History.AddFromFile("history name", c.SystemVariables.HistoryFile)

	exists, err := c.Session.DatabaseExists()
	if err != nil {
		return c.ExitOnError(err)
	}
	if exists {
		fmt.Fprintf(c.OutStream, "Connected.\n")
	} else {
		return c.ExitOnError(fmt.Errorf("unknown database %q", c.SystemVariables.Database))
	}

	for {
		prompt := c.getInterpolatedPrompt()

		shell.Prompt.Primary(func() string {
			return prompt
		})

		// TODO: Currently not work
		shell.Prompt.Secondary(func() string {
			return "->"
		})

		input, err := readInteractiveInput(shell, prompt)
		if err == io.EOF {
			return c.Exit()
		}
		if errors.Is(err, readline.ErrInterrupt) {
			return c.Exit()
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
				c.PrintInteractiveError(fmt.Errorf("ERROR: Unknown database %q\n", s.Database))
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

		// Execute the statement.
		ctx, cancel := context.WithCancel(ctx)
		go handleInterrupt(cancel)
		stop := c.PrintProgressingMark()
		t0 := time.Now()
		result, err := stmt.Execute(ctx, c.Session)
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

		size, _, err := term.GetSize(int(os.Stdout.Fd()))
		if err != nil {
			size = math.MaxInt
		}

		c.PrintResult(size, result, c.SystemVariables.CLIFormat, true)

		fmt.Fprintf(c.OutStream, "\n")
		cancel()
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
		result, err := cmd.Stmt.Execute(ctx, c.Session)
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
	fmt.Fprintf(c.ErrStream, "ERROR: %s\n", err)
	return exitCodeError
}

func (c *Cli) PrintInteractiveError(err error) {
	fmt.Fprintf(c.OutStream, "ERROR: %s\n", err)
}

func (c *Cli) PrintBatchError(err error) {
	fmt.Fprintf(c.ErrStream, "ERROR: %s\n", err)
}

func (c *Cli) PrintResult(screenWidth int, result *Result, mode DisplayMode, interactive bool) {
	printResult(c.SystemVariables.Debug, screenWidth, c.OutStream, result, mode, interactive, c.SystemVariables.Verbose)
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

func (c *Cli) getInterpolatedPrompt() string {
	return promptRe.ReplaceAllStringFunc(c.SystemVariables.Prompt, func(s string) string {
		return lo.Switch[string, string](s).
			Case("%%", "%").
			Case("%n", "\n").
			Case("%p", c.SystemVariables.Project).
			Case("%i", c.SystemVariables.Instance).
			Case("%d", c.SystemVariables.Database).
			Case("%t", lo.
				If(c.Session.InReadWriteTransaction(), "(rw txn)").
				ElseIf(c.Session.InReadOnlyTransaction(), "(ro txn)").
				Else("")).
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
	if credential != nil {
		opts = append(opts, option.WithCredentialsJSON(credential))
	}
	if sysVars.Endpoint != "" {
		opts = append(opts, option.WithEndpoint(sysVars.Endpoint))
	}
	return NewSession(ctx, sysVars, opts...)
}

func readInteractiveInput(rl *readline.Shell, prompt string) (*inputStatement, error) {
	var input string
	for {
		line, err := rl.Readline()
		if err != nil {
			return nil, err
		}
		input += line + "\n"

		statements, err := separateInput(input)
		if err != nil {
			return nil, err
		}

		switch len(statements) {
		case 0:
			// read next input
		case 1:
			return &statements[0], nil
		default:
			return nil, errors.New("sql queries are limited to single statements in interactive mode")
		}

		// show prompt to urge next input
		var margin string
		if l := len(prompt); l >= 3 {
			margin = strings.Repeat(" ", l-3)
		}
		_ = margin
	}

}

func splitLineWithWidth(s string, maxWidth int) iter.Seq[string] {
	return func(yield func(string) bool) {
		for line := range hiter.StringsSplitFunc(s, 0, hiter.StringsCutNewLine) {
			lineRunes := []rune(line)
			if maxWidth >= runes.WidthAll(lineRunes) {
				if !yield(line) {
					return
				}
				continue
			}

			var sb strings.Builder
			currentWidth := 0
			for _, r := range lineRunes {
				runeWidth := runes.Width(r)
				if currentWidth+runeWidth > maxWidth {
					if !yield(sb.String()) {
						return
					}
					sb.Reset()
					currentWidth = 0
				}

				sb.WriteRune(r)
				currentWidth += runeWidth
			}

			if sb.Len() > 0 {
				yield(sb.String())
			}
		}
	}
}

func WrapLines(width int, s string) string {
	return strings.Join(
		slices.Collect(
			splitLineWithWidth(s, width),
		),
		"\n")
}

func maxWidth(s string) int {
	return hiter.Max(xiter.Map(
		func(in string) int {
			return runes.WidthAll([]rune(in))
		},
		hiter.StringsSplitFunc(s, 0, hiter.StringsCutNewLine)))
}

func stringWidthAll(s string) int {
	return runes.WidthAll([]rune(s))
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

	// maxV := slices.Max(vs)
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

func maxIndex(ignoreMax int, adjustWidths []int, seq iter.Seq[WidthCount]) (int, WidthCount) {
	current := -1
	maxIdx := -1
	var candidate WidthCount
	for v := range seq {
		current++
		if ignoreMax >= v.Length-adjustWidths[current] && v.Count > candidate.Count {
			candidate = v
			maxIdx = current
		}
	}
	return maxIdx, candidate
}

func calculateOptimalWidth(debug bool, screenWidth int, types []*sppb.StructType_Field, rows []Row) []int {
	// table overhead is:
	// len(`|  |`) +
	// len(` | `) * len(columns) - 1
	overheadWidth := 4 + 3*(len(types)-1)

	// don't mutate
	remainsWidth := screenWidth - overheadWidth

	if debug {
		log.Printf("screenWitdh: %v, remainsWidth: %v", screenWidth, remainsWidth)
	}

	formatIntermediate := func(remainsWidth int, adjustedWidths []int) string {
		return fmt.Sprintf("remaining %v, adjustedWidths: %v", remainsWidth-lo.Sum(adjustedWidths), adjustedWidths)
	}

	adjustWidths := adjustByName(types, remainsWidth)

	if debug {
		log.Println("adjustByName:", formatIntermediate(remainsWidth, adjustWidths))
	}

	var transposedRows [][]string
	for columnIdx := range len(types) {
		transposedRows = append(transposedRows, slices.Collect(
			xiter.Concat(
				hiter.Once(formatTypedHeaderColumn(types[columnIdx])),
				xiter.Map(
					func(in Row) string {
						return lo.Must(lo.Nth(in.Columns, columnIdx))
					},
					slices.Values(rows),
				))))
	}

	widthCounts := calcurateWidthCounts(adjustWidths, transposedRows)
	for {
		if debug {
			log.Println("widthCounts:", widthCounts)
		}

		firstCounts :=
			xiter.Map(
				func(in []WidthCount) WidthCount {
					return lo.FirstOr(in, WidthCount{
						Length: math.MinInt,
						Count:  0,
					})
				},
				slices.Values(widthCounts))

		idx, target := maxIndex(remainsWidth-lo.Sum(adjustWidths), adjustWidths, firstCounts)
		if idx < 0 {
			break
		}

		widthCounts[idx] = widthCounts[idx][1:]
		adjustWidths[idx] = target.Length

		if debug {
			log.Println("adjusting:", formatIntermediate(remainsWidth, adjustWidths))
		}
	}

	if debug {
		log.Println("semi final:", formatIntermediate(remainsWidth, adjustWidths))
	}

	longestWidths := lo.Map(widthCounts, func(item []WidthCount, index int) int {
		return hiter.Max(xiter.Map(func(wc WidthCount) int { return wc.Length }, slices.Values(item)))
	})

	idx, _ := MaxByWithIdx(math.MinInt, hiter.Unify(func(first, second int) int {
		return second - first
	}, hiter.Pairs(slices.Values(adjustWidths), slices.Values(longestWidths))))

	if idx != -1 {
		adjustWidths[idx] += remainsWidth - lo.Sum(adjustWidths)
	}

	if debug {
		log.Println("final:", formatIntermediate(remainsWidth, adjustWidths))
	}

	return adjustWidths
}

func MaxByWithIdx[E cmp.Ordered](fallback E, seq iter.Seq[E]) (int, E) {
	val := fallback
	idx := -1
	current := -1
	for v := range seq {
		current++
		if val < v {
			val = v
			idx = current
		}
	}
	return idx, val
}

func EntriesSortedByValue[K constraints.Ordered, V constraints.Ordered](m map[K]V) []lo.Entry[K, V] {
	entries := lo.Entries(m)
	lox.SortBy(entries, func(t lo.Entry[K, V]) V { return t.Value })
	return entries
}

func countLen(ss []string) iter.Seq[WidthCount] {
	return xiter.Map(func(in lo.Entry[int, int]) WidthCount {
		return WidthCount{
			Length: in.Key,
			Count:  in.Value,
		}
	}, slices.Values(lox.EntriesSortedByKey(lo.CountValuesBy(ss, maxWidth))))
}

func GreaterThan[T cmp.Ordered](v1 T) func(v2 T) bool {
	return func(v2 T) bool {
		return v2 > v1
	}
}

func calcurateWidthCounts(currentWidths []int, rows [][]string) [][]WidthCount {
	var result [][]WidthCount
	for columnNo := range len(currentWidths) {
		currentWidth := currentWidths[columnNo]
		columnValues := rows[columnNo]
		largerWidthCounts := slices.Collect(
			xiter.Filter(
				func(v WidthCount) bool {
					return v.Length > currentWidth
				},
				countLen(columnValues),
			))
		result = append(result, largerWidthCounts)
	}
	return result
}

type WidthCount struct{ Length, Count int }

func adjustByName(types []*sppb.StructType_Field, availableWidth int) []int {
	names := slices.Collect(xiter.Map(
		(*sppb.StructType_Field).GetName,
		slices.Values(types),
	))
	nameWidths := slices.Collect(xiter.Map(stringWidthAll, slices.Values(names)))

	adjustWidths, _ := adjustToSum(availableWidth, nameWidths)

	return adjustWidths
}

func printResult(debug bool, screenWidth int, out io.Writer, result *Result, mode DisplayMode, interactive, verbose bool) {
	if mode == DisplayModeTable {
		table := tablewriter.NewWriter(out)
		table.SetAutoFormatHeaders(false)
		table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
		table.SetAlignment(tablewriter.ALIGN_LEFT)
		table.SetAutoWrapText(false)

		adjustedWidths := calculateOptimalWidth(debug, screenWidth, result.ColumnTypes, result.Rows)

		var forceTableRender bool
		// This condition is true if statement is SelectStatement or DmlStatement

		if verbose && len(result.ColumnTypes) > 0 {
			forceTableRender = true
			var headers []string
			for i, field := range result.ColumnTypes {
				headers = append(headers, WrapLines(adjustedWidths[i], formatTypedHeaderColumn(field)))
			}
			table.SetHeader(headers)
		} else {
			table.SetHeader(result.ColumnNames)
		}

		for _, row := range result.Rows {
			wrappedRow := Row{
				Columns: slices.Collect(hiter.Unify(
					func(header int, col string) string {
						return WrapLines(header, col)
					},
					hiter.Pairs(slices.Values(adjustedWidths), slices.Values(row.Columns))),
				),
			}
			table.Append(wrappedRow.Columns)
		}

		if forceTableRender || len(result.Rows) > 0 {
			table.Render()
		}
	} else if mode == DisplayModeVertical {
		maxLen := 0
		for _, columnName := range result.ColumnNames {
			if len(columnName) > maxLen {
				maxLen = len(columnName)
			}
		}
		format := fmt.Sprintf("%%%ds: %%s\n", maxLen) // for align right
		for i, row := range result.Rows {
			fmt.Fprintf(out, "*************************** %d. row ***************************\n", i+1)
			for j, column := range row.Columns {
				fmt.Fprintf(out, format, result.ColumnNames[j], column)
			}
		}
	} else if mode == DisplayModeTab {
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

	if result.IsMutation {
		var affectedRowsPrefix string
		if result.AffectedRowsType == rowCountTypeLowerBound {
			// For Partitioned DML the result's row count is lower bounded number, so we add "at least" to express ambiguity.
			// See https://cloud.google.com/spanner/docs/reference/rpc/google.spanner.v1?hl=en#resultsetstats
			affectedRowsPrefix = "at least "
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
		return fmt.Sprintf("Query OK, %s%d rows affected (%s)\n%s",
			affectedRowsPrefix, result.AffectedRows, result.Stats.ElapsedTime, detail)
	}

	var set string
	if result.AffectedRows == 0 {
		set = "Empty set"
	} else {
		set = fmt.Sprintf("%d rows in set", result.AffectedRows)
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
		return fmt.Sprintf("%s (%s)\n%s", set, result.Stats.ElapsedTime, detail)
	}
	return fmt.Sprintf("%s (%s)\n", set, result.Stats.ElapsedTime)
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
