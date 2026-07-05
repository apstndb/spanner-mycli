package mycli

import (
	_ "embed"
	"fmt"
	"io"
	"log/slog"
	"math"
	"strings"
	"text/template"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanner-mycli/internal/mycli/decoder"
	"github.com/apstndb/spanner-mycli/internal/mycli/format"
	"github.com/apstndb/spanner-mycli/internal/mycli/metrics"
	"github.com/go-sprout/sprout"
	"github.com/go-sprout/sprout/group/hermetic"
	"github.com/samber/lo"
)

// renderTableHeader renders TableHeader. It is nil safe.
func renderTableHeader(header TableHeader, verbose bool) []string {
	if header == nil {
		return nil
	}

	return header.Render(verbose)
}

// extractTableColumnNames extracts pure column names from the table header without type information.
// This is used for table structure and layout calculations.
// It is nil-safe and returns nil for a nil header.
func extractTableColumnNames(header TableHeader) []string {
	return renderTableHeader(header, false)
}

func printTableData(sysVars *systemVariables, screenWidth int, out io.Writer, result *Result) error {
	// screenWidth <= 0 means no limit.
	if screenWidth <= 0 {
		screenWidth = math.MaxInt
	}

	columnNames := extractTableColumnNames(result.TableHeader)

	// rows holds the display-text cells to render. For a typed buffered result
	// (Result.Typed, issue #738) they are derived lazily below; otherwise they
	// are the presentation cells already on Result.Rows.
	rows := result.Rows
	bodyRowCount := len(rows)
	if result.Typed != nil {
		bodyRowCount = len(result.Typed.Rows)
	}

	// Log logic error where we have rows but no columns
	if len(columnNames) == 0 && bodyRowCount > 0 {
		slog.Error("printTableData called with empty column headers but non-empty rows - this indicates a logic error",
			"rowCount", bodyRowCount)
	}

	// Debug logging
	slog.Debug("printTableData",
		"columnCount", len(columnNames),
		"rowCount", bodyRowCount,
		"format", sysVars.Display.CLIFormat)

	// Skip formatting only if there's no header at all (e.g., SET statements)
	// Empty query results with columns should still output headers
	if len(columnNames) == 0 {
		return nil
	}

	// Build FormatConfig from systemVariables
	config := sysVars.toFormatConfig()

	fmtMode := format.Mode(sysVars.Display.CLIFormat.String())
	if fmtMode == format.ModeUnspecified {
		fmtMode = format.ModeTable
	}

	// SQL export is allowed only for genuine query results. The typed and
	// display-text paths carry this distinction on different fields (issue #738).
	sqlExportAllowed := result.SQLExportAllowed
	if result.Typed != nil {
		sqlExportAllowed = result.Typed.SQLExportAllowed
	}

	// Typed buffered results carry raw *spanner.Row values. Export formats
	// (CSV/JSONL/SQL_INSERT*) replay them through the single spanvalue emitters;
	// table-family formats derive display cells with the same transform as the
	// query path. This is where lazy formatting happens for issue #738.
	if t := result.Typed; t != nil {
		if usesSpanvalueWriter(sysVars.Display.CLIFormat) &&
			(format.ValueFormatModeFor(fmtMode) != format.SQLLiteralValues || sqlExportAllowed) {
			return writeTypedRows(out, sysVars, result)
		}
		var err error
		if rows, err = deriveDisplayRows(sysVars, t); err != nil {
			return err
		}
	}

	// Modes that require SQL literal values (e.g., SQL_INSERT) must fall back to
	// table format when values were not formatted as SQL literals. This affects
	// non-query buffered results such as metadata statements and DML THEN RETURN,
	// and typed results whose SQLExportAllowed is false (writeTypedRows skipped).
	if format.ValueFormatModeFor(fmtMode) == format.SQLLiteralValues && !sqlExportAllowed {
		slog.Warn("SQL export format not applicable for this statement type, using table format instead",
			"requestedFormat", sysVars.Display.CLIFormat,
			"statementType", "non-SELECT/DML")
		fmtMode = format.ModeTable
	}

	if !fmtMode.IsTableMode() {
		// CSV/JSONL/SQL_INSERT* replay buffered rows through the spanvalue
		// writers so those formats have a single byte-emitting implementation
		// shared with the streaming paths. The fallback above guarantees the
		// SQL modes only reach this replay with SQL-literal formatted cells.
		if handled, err := writeBufferedRowsWithSpanvalueWriter(out, sysVars, result.SQLTableNameForExport, columnNames, rows); handled || err != nil {
			if err != nil {
				return fmt.Errorf("spanvalue writer failed for buffered rows in mode %v: %w", sysVars.Display.CLIFormat, err)
			}
			return nil
		}
		formatter, err := format.NewStreamingFormatter(fmtMode, out, config)
		if err != nil {
			return fmt.Errorf("failed to create streaming formatter for buffered rows: %w", err)
		}
		if err := format.ExecuteWithFormatter(formatter, rows, columnNames, config); err != nil {
			return fmt.Errorf("streaming formatter failed for buffered rows in mode %v: %w", sysVars.Display.CLIFormat, err)
		}
		return nil
	}

	verboseHeaders := renderTableHeader(result.TableHeader, true)
	return format.WriteTableWithParams(out, rows, columnNames, config, screenWidth, fmtMode, format.TableParams{
		VerboseHeaders: verboseHeaders,
		ColumnAlign:    result.ColumnAlign,
	})
}

// printResult writes the result body (table data, appendices, result line) to
// out. The surrounding decorations (CLI_MARKDOWN_CODEBLOCK fence,
// CLI_ECHO_INPUT echo) and the CLI_USE_PAGER pager are owned by resultSink so
// they order correctly around streamed rows; pass a resultSink as out to get
// the decorated output.
func printResult(sysVars *systemVariables, screenWidth int, out io.Writer, result *Result, interactive bool) error {
	// Skip table data if already streamed or pre-rendered by an execution path
	// that needs atomic output after side effects such as implicit DML commit.
	if !result.Streamed {
		if result.HasRenderedOutput {
			if _, err := out.Write(result.RenderedOutput); err != nil {
				return err
			}
		} else {
			if err := printTableData(sysVars, screenWidth, out, result); err != nil {
				return err
			}
		}
	}

	if len(result.Appendices) > 0 {
		for _, appendix := range result.Appendices {
			printResultAppendix(out, appendix)
		}
	} else if len(result.Predicates) > 0 {
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

	if len(result.IndexAdvice) > 0 {
		fmt.Fprintln(out, "Query Advisor Recommendations:")
		for _, advice := range result.IndexAdvice {
			for _, ddl := range advice.DDL {
				if advice.ImprovementFactor > 0 {
					fmt.Fprintf(out, "  %s  -- Est. improvement: %.2f%%\n", ddl, (1-1/advice.ImprovementFactor)*100)
				} else {
					fmt.Fprintf(out, "  %s\n", ddl)
				}
			}
		}
		fmt.Fprintln(out)
	}

	// Only print result line if not suppressed
	if !sysVars.Display.SuppressResultLines && (sysVars.Display.Verbose || result.ForceVerbose || interactive) {
		fmt.Fprint(out, resultLine(sysVars.Display.OutputTemplate, result, sysVars.Display.Verbose || result.ForceVerbose))
	}

	if sysVars.Display.CLIFormat == enums.DisplayModeTableDetailComment {
		fmt.Fprintln(out, "*/")
	}

	return nil
}

func printResultAppendix(out io.Writer, appendix ResultAppendix) {
	if len(appendix.Lines) == 0 {
		return
	}
	fmt.Fprintln(out, appendix.Title)
	for _, s := range appendix.Lines {
		fmt.Fprintf(out, " %s\n", s)
	}
	fmt.Fprintln(out)
}

type OutputContext struct {
	Verbose       bool
	IsExecutedDML bool
	// Timestamp is kept for custom templates written before the read/commit split.
	Timestamp       string
	ReadTimestamp   string
	CommitTimestamp string
	Stats           *QueryStats
	CommitStats     *sppb.CommitResponse_CommitStats
	Metrics         *metrics.ExecutionMetrics
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

	readTimestamp := formatTimestamp(result.ReadTimestamp, "")
	commitTimestamp := formatTimestamp(result.CommitTimestamp, "")
	timestamp := readTimestamp
	if timestamp == "" {
		timestamp = commitTimestamp
	}

	elapsedTimePart := lo.Ternary(result.Stats.ElapsedTime != "", fmt.Sprintf(" (%s)", result.Stats.ElapsedTime), lo.Empty[string]())

	var batchInfo string
	switch result.BatchInfo {
	case nil:
	default:
		batchInfo = fmt.Sprintf(" (%d %s%s in batch)", result.BatchInfo.Size,
			lo.Ternary(result.BatchInfo.Mode == batchModeDDL, "DDL", "DML"),
			lo.Ternary(result.BatchInfo.Size > 1, "s", lo.Empty[string]()),
		)
	}

	var sb strings.Builder
	err := outputTemplate.Execute(&sb, OutputContext{
		Verbose:         verbose,
		IsExecutedDML:   result.IsExecutedDML,
		Timestamp:       timestamp,
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

func formatTypedHeaderColumn(field *sppb.StructType_Field) string {
	return field.GetName() + "\n" + decoder.FormatTypeSimple(field.GetType())
}
