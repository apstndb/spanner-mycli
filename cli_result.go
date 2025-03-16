package main

import (
	"cmp"
	"fmt"
	"io"
	"iter"
	"log"
	"math"
	"regexp"
	"slices"
	"strings"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/lox"
	"github.com/mattn/go-runewidth"
	"github.com/ngicks/go-iterator-helper/hiter"
	"github.com/ngicks/go-iterator-helper/hiter/stringsiter"
	"github.com/ngicks/go-iterator-helper/x/exp/xiter"
	"github.com/olekukonko/tablewriter"
	"github.com/samber/lo"
)

func printResult(sysVars *systemVariables, screenWidth int, out io.Writer, result *Result, interactive bool, input string) {
	mode := sysVars.CLIFormat

	if sysVars.MarkdownCodeblock {
		fmt.Fprintln(out, "```sql")
	}

	if sysVars.EchoInput && input != "" {
		fmt.Fprintln(out, input+";")
	}

	// screenWidth <= means no limit.
	if screenWidth <= 0 {
		screenWidth = math.MaxInt
	}

	switch mode {
	case DisplayModeTable, DisplayModeTableComment, DisplayModeTableDetailComment:
		var tableBuf strings.Builder
		table := tablewriter.NewWriter(&tableBuf)
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
			adjustedWidths = calculateOptimalWidth(sysVars.Debug, screenWidth, names, slices.Concat(sliceOf(toRow(header...)), result.Rows))
		} else {
			adjustedWidths = calculateOptimalWidth(sysVars.Debug, screenWidth, result.ColumnNames, slices.Concat(sliceOf(toRow(result.ColumnNames...)), result.Rows))
		}
		var forceTableRender bool
		if sysVars.Verbose && len(result.ColumnTypes) > 0 {
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
				hiter.Pairs(slices.Values(row), slices.Values(adjustedWidths))),
			)
			table.Append(wrappedColumns)
		}
		if forceTableRender || len(result.Rows) > 0 {
			table.Render()
		}

		s := strings.TrimSpace(tableBuf.String())
		if mode == DisplayModeTableComment || mode == DisplayModeTableDetailComment {
			s = strings.ReplaceAll(s, "\n", "\n ")
			s = topLeftRe.ReplaceAllLiteralString(s, "/*")
		}

		if mode == DisplayModeTableComment {
			s = bottomRightRe.ReplaceAllLiteralString(s, "*/")
		}

		fmt.Fprintln(out, s)
	case DisplayModeVertical:
		maxLen := 0
		for _, columnName := range result.ColumnNames {
			if len(columnName) > maxLen {
				maxLen = len(columnName)
			}
		}
		format := fmt.Sprintf("%%%ds: %%s\n", maxLen)
		for i, row := range result.Rows {
			fmt.Fprintf(out, "*************************** %d. row ***************************\n", i+1)
			for j, column := range row { // j represents the index of the column in the row

				fmt.Fprintf(out, format, result.ColumnNames[j], column)
			}
		}
	case DisplayModeTab:
		if len(result.ColumnNames) > 0 {
			fmt.Fprintln(out, strings.Join(result.ColumnNames, "\t"))
			for _, row := range result.Rows {
				fmt.Fprintln(out, strings.Join(row, "\t"))
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
	if sysVars.Verbose || result.ForceVerbose {
		fmt.Fprint(out, resultLine(result, true))
	} else if interactive {
		fmt.Fprint(out, resultLine(result, sysVars.Verbose))
	}
	if mode == DisplayModeTableDetailComment {
		fmt.Fprintln(out, "*/")
	}

	if sysVars.MarkdownCodeblock {
		fmt.Fprintln(out, "```")
	}
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

		var detail string
		if verbose {
			if timestamp != "" {
				detail += fmt.Sprintf("timestamp:      %s\n", timestamp)
			}
			if result.CommitStats != nil {
				detail += fmt.Sprintf("mutation_count: %d\n", result.CommitStats.GetMutationCount())
			}
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
					return lo.Must(lo.Nth(in, columnIdx)) // columnIdx represents the index of the column in the row
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

var (
	topLeftRe     = regexp.MustCompile(`^\+`)
	bottomRightRe = regexp.MustCompile(`\+$`)
)

func formatTypedHeaderColumn(field *sppb.StructType_Field) string {
	return field.GetName() + "\n" + formatTypeSimple(field.GetType())
}
