package main

import (
	"cmp"
	_ "embed"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"math"
	"regexp"
	"slices"
	"strings"
	"text/template"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/go-runewidthex"
	"github.com/apstndb/lox"
	"github.com/go-sprout/sprout"
	"github.com/go-sprout/sprout/group/hermetic"
	"github.com/mattn/go-runewidth"
	"github.com/ngicks/go-iterator-helper/hiter"
	"github.com/ngicks/go-iterator-helper/hiter/stringsiter"
	"github.com/ngicks/go-iterator-helper/x/exp/xiter"
	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/renderer"
	"github.com/olekukonko/tablewriter/tw"
	"github.com/samber/lo"
)

type DisplayMode int

const (
	DisplayModeTable DisplayMode = iota
	DisplayModeTableComment
	DisplayModeTableDetailComment
	DisplayModeVertical
	DisplayModeTab
)

// renderTableHeader renders TableHeader. It is nil safe.
func renderTableHeader(header TableHeader, verbose bool) []string {
	if header == nil {
		return nil
	}

	return header.internalRender(verbose)
}

func printTableData(sysVars *systemVariables, screenWidth int, out io.Writer, result *Result) {
	mode := sysVars.CLIFormat

	// screenWidth <= means no limit.
	if screenWidth <= 0 {
		screenWidth = math.MaxInt
	}

	columnNames := renderTableHeader(result.TableHeader, false)

	switch mode {
	case DisplayModeTable, DisplayModeTableComment, DisplayModeTableDetailComment:
		rw := runewidthex.NewCondition()
		rw.TabWidth = cmp.Or(int(sysVars.TabWidth), 4)

		rows := result.Rows

		var tableBuf strings.Builder
		table := tablewriter.NewTable(&tableBuf,
			tablewriter.WithRenderer(
				renderer.NewBlueprint(tw.Rendition{Symbols: tw.NewSymbols(tw.StyleASCII)})),
			tablewriter.WithHeaderAlignment(tw.AlignLeft),
			tablewriter.WithTrimSpace(tw.Off),
			tablewriter.WithHeaderAutoFormat(tw.Off),
		).Configure(func(config *tablewriter.Config) {
			config.Row.ColumnAligns = result.ColumnAlign
			config.Row.Formatting.AutoWrap = tw.WrapNone
		})

		wc := &widthCalculator{Condition: rw}
		adjustedWidths := calculateWidth(result, wc, screenWidth, rows)

		headers := slices.Collect(hiter.Unify(
			rw.Wrap,
			hiter.Pairs(
				slices.Values(renderTableHeader(result.TableHeader, sysVars.Verbose)),
				slices.Values(adjustedWidths))),
		)

		table.Header(headers)

		for _, row := range rows {
			wrappedColumns := slices.Collect(hiter.Unify(
				rw.Wrap,
				hiter.Pairs(slices.Values(row), slices.Values(adjustedWidths))),
			)
			if err := table.Append(wrappedColumns); err != nil {
				slog.Error("tablewriter.Table.Append() failed", "err", err)
			}
		}

		forceTableRender := sysVars.Verbose && len(headers) > 0

		if forceTableRender || len(rows) > 0 {
			if err := table.Render(); err != nil {
				slog.Error("tablewriter.Table.Render() failed", "err", err)
			}
		}

		s := strings.TrimSpace(tableBuf.String())
		if mode == DisplayModeTableComment || mode == DisplayModeTableDetailComment {
			s = strings.ReplaceAll(s, "\n", "\n ")
			s = topLeftRe.ReplaceAllLiteralString(s, "/*")
		}

		if mode == DisplayModeTableComment {
			s = bottomRightRe.ReplaceAllLiteralString(s, "*/")
		}

		if s != "" {
			fmt.Fprintln(out, s)
		}
	case DisplayModeVertical:
		maxLen := 0
		for _, columnName := range columnNames {
			if len(columnName) > maxLen {
				maxLen = len(columnName)
			}
		}
		format := fmt.Sprintf("%%%ds: %%s\n", maxLen)
		for i, row := range result.Rows {
			fmt.Fprintf(out, "*************************** %d. row ***************************\n", i+1)
			for j, column := range row { // j represents the index of the column in the row

				fmt.Fprintf(out, format, columnNames[j], column)
			}
		}
	case DisplayModeTab:
		if len(columnNames) > 0 {
			fmt.Fprintln(out, strings.Join(columnNames, "\t"))
			for _, row := range result.Rows {
				fmt.Fprintln(out, strings.Join(row, "\t"))
			}
		}
	}
}

func calculateWidth(result *Result, wc *widthCalculator, screenWidth int, rows []Row) []int {
	names := renderTableHeader(result.TableHeader, false)
	header := renderTableHeader(result.TableHeader, true)
	return calculateOptimalWidth(wc, screenWidth, names, slices.Concat(sliceOf(toRow(header...)), rows))
}

func printResult(sysVars *systemVariables, screenWidth int, out io.Writer, result *Result, interactive bool, input string) {
	if sysVars.MarkdownCodeblock {
		fmt.Fprintln(out, "```sql")
	}

	if sysVars.EchoInput && input != "" {
		fmt.Fprintln(out, input+";")
	}

	printTableData(sysVars, screenWidth, out, result)

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

	if sysVars.CLIFormat == DisplayModeTableDetailComment {
		fmt.Fprintln(out, "*/")
	}

	if sysVars.MarkdownCodeblock {
		fmt.Fprintln(out, "```")
	}
}

type OutputContext struct {
	Verbose     bool
	IsMutation  bool
	Timestamp   string
	Stats       *QueryStats
	CommitStats *sppb.CommitResponse_CommitStats
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

	var sb strings.Builder
	err := outputTemplate.Execute(&sb, OutputContext{
		Verbose:     verbose,
		IsMutation:  result.IsMutation,
		Timestamp:   timestamp,
		Stats:       &result.Stats,
		CommitStats: result.CommitStats,
	})
	if err != nil {
		slog.Error("error on outputTemplate.Execute()", "err", err)
	}
	detail := sb.String()

	if result.IsMutation {
		var affectedRowsPart string
		// If it is a valid mutation, 0 affected row is not printed to avoid confusion.
		if result.AffectedRows > 0 || result.CommitStats.GetMutationCount() == 0 {
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
			affectedRowsPart = fmt.Sprintf(", %s%d rows affected", affectedRowsPrefix, result.AffectedRows)
		}

		return fmt.Sprintf("Query OK%s%s%s\n%s", affectedRowsPart, elapsedTimePart, batchInfo, detail)
	}

	partitionedQueryInfo := lo.Ternary(result.PartitionCount > 0, fmt.Sprintf(" from %v partitions", result.PartitionCount), "")

	var set string
	if result.AffectedRows == 0 {
		set = "Empty set"
	} else {
		set = fmt.Sprintf("%d rows in set%s%s", result.AffectedRows, partitionedQueryInfo, batchInfo)
	}

	return fmt.Sprintf("%s%s\n%s", set, elapsedTimePart, detail)
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

		firstCounts :=
			xiter.Map(
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

var (
	topLeftRe     = regexp.MustCompile(`^\+`)
	bottomRightRe = regexp.MustCompile(`\+$`)
)

func formatTypedHeaderColumn(field *sppb.StructType_Field) string {
	return field.GetName() + "\n" + formatTypeSimple(field.GetType())
}
