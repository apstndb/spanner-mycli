package main

import (
	"cmp"
	_ "embed"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"html"
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
	DisplayModeHTML
	DisplayModeXML
	DisplayModeCSV
)

// parseDisplayMode converts a string format name to DisplayMode.
// It accepts both uppercase and lowercase format names.
// Returns an error if the format name is invalid.
func parseDisplayMode(format string) (DisplayMode, error) {
	switch strings.ToUpper(format) {
	case "TABLE":
		return DisplayModeTable, nil
	case "TABLE_COMMENT":
		return DisplayModeTableComment, nil
	case "TABLE_DETAIL_COMMENT":
		return DisplayModeTableDetailComment, nil
	case "VERTICAL":
		return DisplayModeVertical, nil
	case "TAB":
		return DisplayModeTab, nil
	case "HTML":
		return DisplayModeHTML, nil
	case "XML":
		return DisplayModeXML, nil
	case "CSV":
		return DisplayModeCSV, nil
	default:
		return DisplayModeTable, fmt.Errorf("invalid format: %v", format)
	}
}

// renderTableHeader renders TableHeader. It is nil safe.
func renderTableHeader(header TableHeader, verbose bool) []string {
	if header == nil {
		return nil
	}

	return header.internalRender(verbose)
}

func printTableData(sysVars *systemVariables, screenWidth int, out io.Writer, result *Result) {
	// TODO(#388): Migrate all output functions to return errors for better error handling.
	// Currently, only HTML and XML formats return errors as a first step toward
	// improving error handling across all output formats.
	mode := sysVars.CLIFormat

	// screenWidth <= means no limit.
	if screenWidth <= 0 {
		screenWidth = math.MaxInt
	}

	columnNames := renderTableHeader(result.TableHeader, false)

	// Early return if no columns - Spanner requires at least one column in SELECT,
	// so this only happens for edge cases where no output is expected
	if len(columnNames) == 0 {
		// Log edge case where we have rows but no columns
		if len(result.Rows) > 0 {
			slog.Warn("printTableData called with empty column headers but non-empty rows",
				"rowCount", len(result.Rows))
		}
		return
	}

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

		if !sysVars.SkipColumnNames {
			table.Header(headers)
		}

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
				var columnName string
				if j < len(columnNames) {
					columnName = columnNames[j]
				} else {
					// Use a default column name if row has more columns than headers
					columnName = fmt.Sprintf("Column_%d", j+1)
				}
				fmt.Fprintf(out, format, columnName, column)
			}
		}
	case DisplayModeTab:
		if len(columnNames) > 0 {
			if !sysVars.SkipColumnNames {
				fmt.Fprintln(out, strings.Join(columnNames, "\t"))
			}
			for _, row := range result.Rows {
				fmt.Fprintln(out, strings.Join(row, "\t"))
			}
		}
	case DisplayModeHTML:
		// Output data in HTML table format compatible with Google Cloud Spanner CLI.
		// All values are properly escaped using html.EscapeString for security.
		if err := printHTMLTable(out, columnNames, result.Rows, sysVars.SkipColumnNames); err != nil {
			slog.Error("printHTMLTable() failed", "err", err)
		}
	case DisplayModeXML:
		// Output data in XML format compatible with Google Cloud Spanner CLI.
		// All values are properly escaped using encoding/xml for security.
		// Schema:
		//   <resultset xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
		//     <header>  <!-- optional, omitted when SkipColumnNames is true -->
		//       <field>column1</field>
		//       ...
		//     </header>
		//     <row>
		//       <field>value1</field>
		//       ...
		//     </row>
		//     ...
		//   </resultset>
		if err := printXMLResultSet(out, columnNames, result.Rows, sysVars.SkipColumnNames); err != nil {
			slog.Error("printXMLResultSet() failed", "err", err)
		}
	case DisplayModeCSV:
		// Output data in CSV format using encoding/csv for RFC 4180 compliance.
		// This provides automatic escaping of special characters (commas, quotes, newlines).
		if err := printCSVTable(out, columnNames, result.Rows, sysVars.SkipColumnNames); err != nil {
			slog.Error("printCSVTable() failed", "err", err)
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

	// Echo the input SQL if CLI_ECHO_INPUT is enabled
	// This output is intentionally sent to 'out' (not TtyOutStream) so it's captured in tee files
	// This provides complete context in logs showing which queries produced which results
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

var (
	topLeftRe     = regexp.MustCompile(`^\+`)
	bottomRightRe = regexp.MustCompile(`\+$`)
)

func formatTypedHeaderColumn(field *sppb.StructType_Field) string {
	return field.GetName() + "\n" + formatTypeSimple(field.GetType())
}

// printHTMLTable outputs query results in HTML table format.
// The format is compatible with Google Cloud Spanner CLI:
//   - Uses uppercase HTML tags (TABLE, TR, TD, TH) for compatibility
//   - Includes BORDER='1' attribute on TABLE element
//   - All values are HTML-escaped using html.EscapeString for security
//
// Note: This function streams output row-by-row for memory efficiency.
func printHTMLTable(out io.Writer, columnNames []string, rows []Row, skipColumnNames bool) error {
	if len(columnNames) == 0 {
		return fmt.Errorf("no columns to output")
	}

	if _, err := fmt.Fprint(out, "<TABLE BORDER='1'>"); err != nil {
		return err
	}

	// Add header row unless skipping column names
	if !skipColumnNames {
		if _, err := fmt.Fprint(out, "<TR>"); err != nil {
			return err
		}
		for _, col := range columnNames {
			if _, err := fmt.Fprintf(out, "<TH>%s</TH>", html.EscapeString(col)); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprint(out, "</TR>"); err != nil {
			return err
		}
	}

	// Add data rows
	for _, row := range rows {
		if _, err := fmt.Fprint(out, "<TR>"); err != nil {
			return err
		}
		for _, col := range row {
			if _, err := fmt.Fprintf(out, "<TD>%s</TD>", html.EscapeString(col)); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprint(out, "</TR>"); err != nil {
			return err
		}
	}

	_, err := fmt.Fprintln(out, "</TABLE>")
	return err
}

// xmlField represents a field element in XML output.
type xmlField struct {
	XMLName xml.Name `xml:"field"`
	Value   string   `xml:",chardata"`
}

// xmlRow represents a row element containing multiple fields.
type xmlRow struct {
	XMLName xml.Name   `xml:"row"`
	Fields  []xmlField `xml:"field"`
}

// xmlHeader represents the optional header element containing column names.
type xmlHeader struct {
	XMLName xml.Name   `xml:"header"`
	Fields  []xmlField `xml:"field"`
}

// xmlResultSet represents the root element of the XML output.
type xmlResultSet struct {
	XMLName xml.Name   `xml:"resultset"`
	XMLNS   string     `xml:"xmlns:xsi,attr"`
	Header  *xmlHeader `xml:"header,omitempty"`
	Rows    []xmlRow   `xml:"row"`
}

// printXMLResultSet outputs query results in XML format.
// The format is compatible with Google Cloud Spanner CLI:
//   - Uses XML declaration with single quotes: <?xml version='1.0'?>
//   - Includes xmlns:xsi namespace for compatibility
//   - All values are automatically XML-escaped by encoding/xml package
//
// Note: This implementation builds the entire result set in memory before
// encoding. For very large result sets, consider using TAB format for
// streaming output. This design prioritizes simplicity and consistency
// with the TABLE format over memory efficiency.
func printXMLResultSet(out io.Writer, columnNames []string, rows []Row, skipColumnNames bool) error {
	if len(columnNames) == 0 {
		return fmt.Errorf("no columns to output")
	}

	// Build the result set structure
	resultSet := xmlResultSet{
		XMLNS: "http://www.w3.org/2001/XMLSchema-instance",
		Rows:  make([]xmlRow, 0, len(rows)),
	}

	// Add header fields only if not skipping column names
	if !skipColumnNames {
		header := &xmlHeader{Fields: make([]xmlField, 0, len(columnNames))}
		for _, col := range columnNames {
			header.Fields = append(header.Fields, xmlField{Value: col})
		}
		resultSet.Header = header
	}

	// Add rows
	for _, row := range rows {
		xmlRow := xmlRow{Fields: make([]xmlField, 0, len(row))}
		for _, col := range row {
			xmlRow.Fields = append(xmlRow.Fields, xmlField{Value: col})
		}
		resultSet.Rows = append(resultSet.Rows, xmlRow)
	}

	// Write XML declaration
	if _, err := fmt.Fprintln(out, "<?xml version='1.0'?>"); err != nil {
		return err
	}

	// Marshal the result set
	encoder := xml.NewEncoder(out)
	encoder.Indent("", "\t")
	if err := encoder.Encode(resultSet); err != nil {
		return fmt.Errorf("xml.Encoder.Encode() failed: %w", err)
	}
	_, err := fmt.Fprintln(out) // Add final newline
	return err
}

// printCSVTable outputs query results in CSV format.
// The format follows RFC 4180 with automatic escaping via encoding/csv.
// Column headers are included unless skipColumnNames is true.
//
// Note: Spanner requires at least one column in SELECT queries, so columnNames
// should never be empty for valid query results. The empty check is defensive
// programming for edge cases like client-side statements or error conditions.
func printCSVTable(out io.Writer, columnNames []string, rows []Row, skipColumnNames bool) error {
	if len(columnNames) == 0 {
		return fmt.Errorf("no columns to output")
	}

	csvWriter := csv.NewWriter(out)
	defer csvWriter.Flush()

	if !skipColumnNames {
		if err := csvWriter.Write(columnNames); err != nil {
			return err
		}
	}

	for _, row := range rows {
		if err := csvWriter.Write(row); err != nil {
			return err
		}
	}

	return nil
}
