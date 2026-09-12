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
	// they are derived lazily below; otherwise they are the presentation cells.
	var rows []Row
	var typed *TypedRows
	switch result.Body.kind {
	case resultBodyPresentation:
		rows = result.Body.rows
	case resultBodyTyped:
		typed = result.Body.typed
	default:
		// no-body, prepared bytes, and delivered output are not tables.
		return nil
	}

	bodyRowCount := len(rows)
	if typed != nil {
		bodyRowCount = len(typed.Rows)
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

	// Skip formatting only if there's no header at all (e.g. SET statements).
	// Empty query results with columns should still output headers.
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
	// presentation paths carry this distinction on different fields.
	sqlExportAllowed := result.SQLExportAllowed
	if typed != nil {
		sqlExportAllowed = typed.SQLExportAllowed
	}

	// Typed buffered results carry raw *spanner.Row values. Export formats
	// (CSV/JSONL/SQL_INSERT*) replay them through the single spanvalue emitters;
	// table-family formats derive display cells with the same transform as the
	// query path.
	if typed != nil {
		if usesSpanvalueWriter(sysVars.Display.CLIFormat) &&
			(format.ValueFormatModeFor(fmtMode) != format.SQLLiteralValues || sqlExportAllowed) {
			return writeTypedRows(out, sysVars, result)
		}
		var err error
		if rows, err = deriveDisplayRows(sysVars, typed); err != nil {
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
		// CSV/JSONL/SQL_INSERT* replay display-text presentation rows through the
		// spanvalue writers so those formats have a single byte-emitting
		// implementation shared with the streaming and typed-buffered paths. The
		// SQL fallback above guarantees presentation tables (SQLExportAllowed
		// false) never reach this replay in a SQL mode.
		if handled, err := writeDisplayRows(out, sysVars, columnNames, rows); handled || err != nil {
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
	switch result.Body.kind {
	case resultBodyDelivered:
		// Body was already written during execution.
	case resultBodyPrepared:
		if _, err := out.Write(result.Body.prepared); err != nil {
			return err
		}
	case resultBodyPresentation, resultBodyTyped:
		if err := printTableData(sysVars, screenWidth, out, result); err != nil {
			return err
		}
	case resultBodyNone:
		// No body is not delivered; skip the table and still print appendices
		// and summaries below.
	}

	if len(result.Appendices) > 0 {
		for _, appendix := range result.Appendices {
			if err := printResultAppendix(out, appendix); err != nil {
				return err
			}
		}
	} else if len(result.Predicates) > 0 {
		if _, err := fmt.Fprintln(out, "Predicates(identified by ID):"); err != nil {
			return err
		}
		for _, s := range result.Predicates {
			if _, err := fmt.Fprintf(out, " %s\n", s); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
	}

	if len(result.LintResults) > 0 {
		if _, err := fmt.Fprintln(out, "Experimental Lint Result:"); err != nil {
			return err
		}
		for _, s := range result.LintResults {
			if _, err := fmt.Fprintf(out, " %s\n", s); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
	}

	if len(result.IndexAdvice) > 0 {
		if _, err := fmt.Fprintln(out, "Query Advisor Recommendations:"); err != nil {
			return err
		}
		for _, advice := range result.IndexAdvice {
			for _, ddl := range advice.DDL {
				if advice.ImprovementFactor > 0 {
					if _, err := fmt.Fprintf(out, "  %s  -- Est. improvement: %.2f%%\n", ddl, (1-1/advice.ImprovementFactor)*100); err != nil {
						return err
					}
				} else {
					if _, err := fmt.Fprintf(out, "  %s\n", ddl); err != nil {
						return err
					}
				}
			}
		}
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
	}

	// Only print result line if not suppressed
	if !sysVars.Display.SuppressResultLines && (sysVars.Display.Verbose || result.ForceVerbose || interactive) {
		if _, err := fmt.Fprint(out, resultLine(sysVars.Display.OutputTemplate, result, sysVars.Display.Verbose || result.ForceVerbose)); err != nil {
			return err
		}
	}

	if sysVars.Display.CLIFormat == enums.DisplayModeTableDetailComment {
		if _, err := fmt.Fprintln(out, "*/"); err != nil {
			return err
		}
	}

	return nil
}

func printResultAppendix(out io.Writer, appendix ResultAppendix) error {
	if len(appendix.Lines) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(out, appendix.Title); err != nil {
		return err
	}
	for _, s := range appendix.Lines {
		if _, err := fmt.Fprintf(out, " %s\n", s); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(out)
	return err
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
