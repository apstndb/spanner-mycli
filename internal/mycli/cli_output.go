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
	"github.com/apstndb/lox"
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
	// Handle direct output - pre-formatted text that should be printed as-is
	if result.IsDirectOutput {
		for _, row := range result.Rows {
			if len(row) > 0 {
				fmt.Fprintln(out, row[0])
			}
		}
		return nil
	}

	// screenWidth <= 0 means no limit.
	if screenWidth <= 0 {
		screenWidth = math.MaxInt
	}

	columnNames := extractTableColumnNames(result.TableHeader)

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

	// Determine the display format to use
	displayFormat := sysVars.CLIFormat

	// SQL export formats require values to be formatted as SQL literals for valid SQL generation.
	// When HasSQLFormattedValues is false, the values are formatted for display (e.g., TIMESTAMP
	// as "2024-01-01T00:00:00Z" instead of TIMESTAMP "2024-01-01T00:00:00Z").
	// Attempting to use display-formatted values in INSERT statements would generate invalid SQL.
	// Therefore, we fall back to table format for safety.
	// This affects metadata queries (SHOW CREATE TABLE, EXPLAIN) and DML with THEN RETURN.
	if sysVars.CLIFormat.IsSQLExport() && !result.HasSQLFormattedValues {
		slog.Warn("SQL export format not applicable for this statement type, using table format instead",
			"requestedFormat", sysVars.CLIFormat,
			"statementType", "non-SELECT/DML")
		displayFormat = enums.DisplayModeTable // Fall back to table format
	}

	// Build FormatConfig from systemVariables
	config := sysVars.toFormatConfig()

	// For SQL export, resolve the table name from Result if available
	if displayFormat.IsSQLExport() && result.SQLTableNameForExport != "" {
		config.SQLTableName = result.SQLTableNameForExport
	}

	// Create the appropriate formatter based on the display mode
	formatter, err := format.NewFormatter(displayFormat)
	if err != nil {
		return fmt.Errorf("failed to create formatter: %w", err)
	}

	// For table mode, pass verbose headers and column align via WriteTableWithParams
	if displayFormat == enums.DisplayModeUnspecified || displayFormat == enums.DisplayModeTable || displayFormat == enums.DisplayModeTableComment || displayFormat == enums.DisplayModeTableDetailComment {
		verboseHeaders := renderTableHeader(result.TableHeader, true)
		tableMode := displayFormat
		if tableMode == enums.DisplayModeUnspecified {
			tableMode = enums.DisplayModeTable
		}
		return format.WriteTableWithParams(out, result.Rows, columnNames, config, screenWidth, tableMode, format.TableParams{
			VerboseHeaders: verboseHeaders,
			ColumnAlign:    result.ColumnAlign,
		})
	}

	// Format and write the result
	if err := formatter(out, result.Rows, columnNames, config, screenWidth); err != nil {
		return fmt.Errorf("formatting failed for mode %v: %w", sysVars.CLIFormat, err)
	}

	return nil
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
	if !sysVars.SuppressResultLines && (sysVars.Verbose || result.ForceVerbose || interactive) {
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

func formatTypedHeaderColumn(field *sppb.StructType_Field) string {
	return field.GetName() + "\n" + decoder.FormatTypeSimple(field.GetType())
}
