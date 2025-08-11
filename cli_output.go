package main

import (
	"cmp"
	_ "embed"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"math"
	"slices"
	"strings"
	"text/template"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/go-runewidthex"
	"github.com/apstndb/lox"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/go-sprout/sprout"
	"github.com/go-sprout/sprout/group/hermetic"
	"github.com/mattn/go-runewidth"
	"github.com/ngicks/go-iterator-helper/hiter"
	"github.com/ngicks/go-iterator-helper/hiter/stringsiter"
	"github.com/ngicks/go-iterator-helper/x/exp/xiter"
	"github.com/samber/lo"
)

// renderTableHeader renders TableHeader. It is nil safe.
func renderTableHeader(header TableHeader, verbose bool) []string {
	if header == nil {
		return nil
	}

	return header.internalRender(verbose)
}

func printTableData(sysVars *systemVariables, screenWidth int, out io.Writer, result *Result) error {
	// screenWidth <= 0 means no limit.
	if screenWidth <= 0 {
		screenWidth = math.MaxInt
	}

	columnNames := renderTableHeader(result.TableHeader, false)

	// Log logic error where we have rows but no columns
	if len(columnNames) == 0 && len(result.Rows) > 0 {
		slog.Error("printTableData called with empty column headers but non-empty rows - this indicates a logic error",
			"rowCount", len(result.Rows))
	}

	// Debug logging
	slog.Debug("printTableData",
		"columnCount", len(columnNames),
		"rowCount", len(result.Rows),
		"format", sysVars.CLIFormat)

	// Skip formatting only if there's no header at all (e.g., SET statements)
	// Empty query results with columns should still output headers
	if len(columnNames) == 0 {
		return nil
	}

	// Create the appropriate formatter based on the display mode
	formatter, err := NewFormatter(sysVars.CLIFormat)
	if err != nil {
		return fmt.Errorf("failed to create formatter: %w", err)
	}

	// Format and write the result
	// Individual formatters handle empty columns appropriately for their format
	if err := formatter(out, result, columnNames, sysVars, screenWidth); err != nil {
		return fmt.Errorf("formatting failed for mode %v: %w", sysVars.CLIFormat, err)
	}

	return nil
}

func calculateWidth(result *Result, wc *widthCalculator, screenWidth int, rows []Row) []int {
	names := renderTableHeader(result.TableHeader, false)
	header := renderTableHeader(result.TableHeader, true)
	return calculateOptimalWidth(wc, screenWidth, names, slices.Concat(sliceOf(toRow(header...)), rows))
}

func printResult(sysVars *systemVariables, screenWidth int, out io.Writer, result *Result, interactive bool, input string) error {
	if sysVars.MarkdownCodeblock {
		fmt.Fprintln(out, "```sql")
	}

	// Echo the input SQL if CLI_ECHO_INPUT is enabled
	// This output is intentionally sent to 'out' (not TtyOutStream) so it's captured in tee files
	// This provides complete context in logs showing which queries produced which results
	if sysVars.EchoInput && input != "" {
		fmt.Fprintln(out, input+";")
	}

	// Skip table data if already streamed
	if !result.Streamed {
		if err := printTableData(sysVars, screenWidth, out, result); err != nil {
			return err
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

	if sysVars.Verbose || result.ForceVerbose || interactive {
		fmt.Fprint(out, resultLine(sysVars.OutputTemplate, result, sysVars.Verbose || result.ForceVerbose))
	}

	if sysVars.CLIFormat == enums.DisplayModeTableDetailComment {
		fmt.Fprintln(out, "*/")
	}

	if sysVars.MarkdownCodeblock {
		fmt.Fprintln(out, "```")
	}

	return nil
}

type OutputContext struct {
	Verbose         bool
	IsExecutedDML   bool
	ReadTimestamp   string
	CommitTimestamp string
	Stats           *QueryStats
	CommitStats     *sppb.CommitResponse_CommitStats
	Metrics         *ExecutionMetrics
}

func sproutFuncMap() template.FuncMap {
	handler := sprout.New()
	lo.Must0(handler.AddGroups(hermetic.RegistryGroup()))
	return handler.Build()
}

//go:embed output_default.tmpl
var outputTemplateStr string

func resultLine(outputTemplate *template.Template, result *Result, verbose bool) string {
	if outputTemplate == nil {
		outputTemplate = defaultOutputFormat
	}

	var readTimestamp string
	if !result.ReadTimestamp.IsZero() {
		readTimestamp = result.ReadTimestamp.Format(time.RFC3339Nano)
	}

	var commitTimestamp string
	if !result.CommitTimestamp.IsZero() {
		commitTimestamp = result.CommitTimestamp.Format(time.RFC3339Nano)
	}

	elapsedTimePart := lox.IfOrEmpty(result.Stats.ElapsedTime != "", fmt.Sprintf(" (%s)", result.Stats.ElapsedTime))

	var batchInfo string
	switch result.BatchInfo {
	case nil:
	default:
		batchInfo = fmt.Sprintf(" (%d %s%s in batch)", result.BatchInfo.Size,
			lo.Ternary(result.BatchInfo.Mode == batchModeDDL, "DDL", "DML"),
			lox.IfOrEmpty(result.BatchInfo.Size > 1, "s"),
		)
	}

	var sb strings.Builder
	err := outputTemplate.Execute(&sb, OutputContext{
		Verbose:         verbose,
		IsExecutedDML:   result.IsExecutedDML,
		ReadTimestamp:   readTimestamp,
		CommitTimestamp: commitTimestamp,
		Stats:           &result.Stats,
		CommitStats:     result.CommitStats,
		Metrics:         result.Metrics,
	})
	if err != nil {
		slog.Error("error on outputTemplate.Execute()", "err", err)
	}
	detail := sb.String()

	// Check if statement has a result set (indicated by TableHeader)
	// Special case: RUN BATCH DML has a TableHeader but should be treated as a DML execution result
	if result.TableHeader != nil && result.BatchInfo == nil {
		// Statement has a result set (SELECT, SHOW, DML with THEN RETURN, EXPLAIN ANALYZE)
		partitionedQueryInfo := lo.Ternary(result.PartitionCount > 0, fmt.Sprintf(" from %v partitions", result.PartitionCount), "")

		var set string
		if result.AffectedRows == 0 {
			set = "Empty set"
		} else {
			set = fmt.Sprintf("%d rows in set%s%s", result.AffectedRows, partitionedQueryInfo, batchInfo)
		}

		return fmt.Sprintf("%s%s\n%s", set, elapsedTimePart, detail)
	}

	// Statement has no result set (SET, DDL, DML without THEN RETURN, MUTATE)
	var affectedRowsPart string
	if result.IsExecutedDML {
		// For DML statements (not DDL or MUTATE), show affected rows count
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

		// Always show affected rows for DML (including "0 rows affected" for MySQL compatibility)
		affectedRowsPart = fmt.Sprintf(", %s%d rows affected", affectedRowsPrefix, result.AffectedRows)
	}

	return fmt.Sprintf("Query OK%s%s%s\n%s", affectedRowsPart, elapsedTimePart, batchInfo, detail)
}

func calculateOptimalWidth(wc *widthCalculator, screenWidth int, header []string, rows []Row) []int {
	// table overhead is:
	// len(`|  |`) +
	// len(` | `) * len(columns) - 1
	overheadWidth := 4 + 3*(len(header)-1)

	// don't mutate
	termWidthWithoutOverhead := screenWidth - overheadWidth

	slog.Debug("screen width info", "screenWidth", screenWidth, "remainsWidth", termWidthWithoutOverhead)

	formatIntermediate := func(remainsWidth int, adjustedWidths []int) string {
		return fmt.Sprintf("remaining %v, adjustedWidths: %v", remainsWidth-lo.Sum(adjustedWidths), adjustedWidths)
	}

	adjustedWidths := adjustByHeader(header, termWidthWithoutOverhead)

	slog.Debug("adjustByName", "info", formatIntermediate(termWidthWithoutOverhead, adjustedWidths))

	var transposedRows [][]string
	for columnIdx := range len(header) {
		transposedRows = append(transposedRows, slices.Collect(
			xiter.Map(
				func(in Row) string {
					return lo.Must(lo.Nth(in, columnIdx)) // columnIdx represents the index of the column in the row
				},
				xiter.Concat(hiter.Once(toRow(header...)), slices.Values(rows)),
			)))
	}

	widthCounts := wc.calculateWidthCounts(adjustedWidths, transposedRows)
	for {
		slog.Debug("widthCounts", "counts", widthCounts)

		firstCounts := xiter.Map(
			func(wcs []WidthCount) WidthCount {
				return lo.FirstOr(wcs, invalidWidthCount)
			},
			slices.Values(widthCounts))

		// find the largest count idx within available width
		idx, target := wc.maxIndex(termWidthWithoutOverhead-lo.Sum(adjustedWidths), adjustedWidths, firstCounts)
		if idx < 0 || target.Count() < 1 {
			break
		}

		widthCounts[idx] = widthCounts[idx][1:]
		adjustedWidths[idx] = target.Length()

		slog.Debug("adjusting", "info", formatIntermediate(termWidthWithoutOverhead, adjustedWidths))
	}

	slog.Debug("semi final", "info", formatIntermediate(termWidthWithoutOverhead, adjustedWidths))

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

	slog.Debug("final", "info", formatIntermediate(termWidthWithoutOverhead, adjustedWidths))

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

func (wc *widthCalculator) StringWidth(s string) int {
	return wc.Condition.StringWidth(s)
}

func (wc *widthCalculator) maxWidth(s string) int {
	return hiter.Max(xiter.Map(
		wc.StringWidth,
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

func asc[T cmp.Ordered](left, right T) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func desc[T cmp.Ordered](left, right T) int {
	return asc(right, left)
}

func adjustToSum(limit int, vs []int) ([]int, int) {
	sumVs := lo.Sum(vs)
	remains := limit - sumVs
	if remains >= 0 {
		return vs, remains
	}

	curVs := vs
	for i := 1; ; i++ {
		rev := slices.SortedFunc(slices.Values(lo.Uniq(vs)), desc)
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

type widthCalculator struct{ Condition *runewidthex.Condition }

func (wc *widthCalculator) maxIndex(ignoreMax int, adjustWidths []int, seq iter.Seq[WidthCount]) (int, WidthCount) {
	return MaxByWithIdx(
		invalidWidthCount,
		WidthCount.Count,
		hiter.Unify(
			func(adjustWidth int, wc WidthCount) WidthCount {
				return lo.Ternary(wc.Length()-adjustWidth <= ignoreMax, wc, invalidWidthCount)
			},
			hiter.Pairs(slices.Values(adjustWidths), seq)))
}

func (wc *widthCalculator) countWidth(ss []string) iter.Seq[WidthCount] {
	return xiter.Map(
		func(e lo.Entry[int, int]) WidthCount {
			return WidthCount{
				width: e.Key,
				count: e.Value,
			}
		},
		slices.Values(lox.EntriesSortedByKey(lo.CountValuesBy(ss, wc.maxWidth))))
}

func (wc *widthCalculator) calculateWidthCounts(currentWidths []int, rows [][]string) [][]WidthCount {
	var result [][]WidthCount
	for columnNo := range len(currentWidths) {
		currentWidth := currentWidths[columnNo]
		columnValues := rows[columnNo]
		largerWidthCounts := slices.Collect(
			xiter.Filter(
				func(v WidthCount) bool {
					return v.Length() > currentWidth
				},
				wc.countWidth(columnValues),
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

func formatTypedHeaderColumn(field *sppb.StructType_Field) string {
	return field.GetName() + "\n" + formatTypeSimple(field.GetType())
}
