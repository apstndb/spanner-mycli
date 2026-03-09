package mycli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanner-mycli/internal/mycli/decoder"
	"github.com/apstndb/spanner-mycli/internal/mycli/format"
	"github.com/apstndb/spanner-mycli/internal/mycli/formatsql"
	"github.com/apstndb/spanner-mycli/internal/mycli/metrics"
	"github.com/apstndb/spanvalue"
	"google.golang.org/grpc/codes"
)

// executeSQLWithFormatAndTxn executes SQL with specific format settings and within a given transaction.
// This is for use within withReadOnlyTransaction callbacks where we already have a transaction.
func executeSQLWithFormatAndTxn(ctx context.Context, session *Session, txn *spanner.ReadOnlyTransaction, sql string, format enums.DisplayMode, streamingMode enums.StreamingMode, sqlTableName string) (*Result, error) {
	// Create a copy of the system variables for this specific execution
	tempVars := *session.systemVariables

	// Set temporary values on the copy
	tempVars.Display.CLIFormat = format
	tempVars.Query.StreamingMode = streamingMode
	if sqlTableName != "" {
		tempVars.Display.SQLTableName = sqlTableName
	}
	tempVars.Display.SkipColumnNames = true
	tempVars.Display.SuppressResultLines = true
	tempVars.Display.EnableProgressBar = false

	// Execute with the transaction directly
	return executeSQLImplWithTxn(ctx, session, txn, sql, &tempVars)
}

func executeSQL(ctx context.Context, session *Session, sql string) (*Result, error) {
	return executeSQLImpl(ctx, session, sql)
}

// executeSQLImpl delegates to executeSQLImplWithVars with the session's system variables
func executeSQLImpl(ctx context.Context, session *Session, sql string) (*Result, error) {
	return executeSQLImplWithVars(ctx, session, sql, session.systemVariables)
}

// prepareFormatConfig determines the appropriate format configuration based on the display mode.
// It returns the format config, the value format mode used, and the potentially modified sysVars.
// This is extracted as a common function to avoid duplication between executeSQLImplWithTxn and executeSQLImplWithVars.
//
// Instead of switching on specific mode names, this function queries the registry
// for the ValueFormatMode declared by the formatter.
// Note: execute_sql.go still imports formatsql for ExtractTableNameFromQuery (SQL parsing utility).
func prepareFormatConfig(sql string, sysVars *systemVariables) (*spanvalue.FormatConfig, format.ValueFormatMode, *systemVariables, error) {
	fmtMode := format.Mode(sysVars.Display.CLIFormat.String())
	vfm := format.ValueFormatModeFor(fmtMode)

	switch vfm {
	case format.SQLLiteralValues:
		// Use SQL literal formatting for modes that declared SQLLiteralValues
		// LiteralFormatConfig formats values as valid Spanner SQL literals
		fc := spanvalue.LiteralFormatConfig

		// Auto-detect table name if not explicitly set
		if sysVars.Display.SQLTableName == "" {
			detectedTableName, detectionErr := formatsql.ExtractTableNameFromQuery(sql)
			if detectedTableName != "" {
				// Create a copy of sysVars to use the detected table name for this execution only.
				// This is important for:
				// 1. Scope isolation: auto-detection only affects this specific query execution
				// 2. Thread safety: if sysVars is shared across goroutines, we don't modify the original
				// 3. Preserving user settings: the original CLI_SQL_TABLE_NAME remains unchanged
				tempVars := *sysVars
				tempVars.Display.SQLTableName = detectedTableName
				sysVars = &tempVars
				slog.Debug("Auto-detected table name for SQL export", "table", detectedTableName)
			} else if detectionErr != nil {
				// Log why auto-detection failed for debugging
				slog.Debug("Table name auto-detection failed", "reason", detectionErr.Error())
			}
		}

		return fc, vfm, sysVars, nil
	default:
		// Use regular display formatting for other modes
		// formatConfigWithProto handles custom proto descriptors if set
		fc, err := decoder.FormatConfigWithProto(sysVars.Internal.ProtoDescriptor, sysVars.Display.MultilineProtoText)
		return fc, vfm, sysVars, err
	}
}

// newMetrics creates and initializes execution metrics from system variables.
func newMetrics(sysVars *systemVariables) *metrics.ExecutionMetrics {
	m := &metrics.ExecutionMetrics{
		QueryStartTime: time.Now(),
		Profile:        sysVars.Query.Profile,
	}
	if sysVars.Query.Profile {
		before := metrics.GetMemoryStats()
		m.MemoryBefore = &before
	}
	return m
}

// finalizeMetrics completes metrics collection after query execution.
func finalizeMetrics(m *metrics.ExecutionMetrics, sysVars *systemVariables) {
	m.CompletionTime = time.Now()
	if sysVars.Query.Profile {
		after := metrics.GetMemoryStats()
		m.MemoryAfter = &after
	}
}

// queryExecution bundles the parameters for the query execution pipeline,
// avoiding 9+ individual parameters through executeAndCollect and its downstream functions.
type queryExecution struct {
	Session      *Session
	Iter         *spanner.RowIterator
	ReadOnlyTxn  *spanner.ReadOnlyTransaction
	FormatConfig *spanvalue.FormatConfig
	SQL          string
	SysVars      *systemVariables
	Metrics      *metrics.ExecutionMetrics
	ValueFmtMode format.ValueFormatMode
	Processor    RowProcessor // set by executeAndCollect after decideExecutionMode
}

// executeAndCollect runs the query iterator (streaming or buffered) and attaches metrics to the result.
func executeAndCollect(ctx context.Context, qe *queryExecution) (*Result, error) {
	useStreaming, processor := decideExecutionMode(qe.SysVars)
	qe.Metrics.IsStreaming = useStreaming
	qe.Processor = processor

	slog.Debug("executeSQL decision",
		"useStreaming", useStreaming,
		"format", qe.SysVars.Display.CLIFormat,
		"sqlTableName", qe.SysVars.Display.SQLTableName)

	var result *Result
	var err error
	if useStreaming {
		result, err = executeWithStreaming(ctx, qe)
	} else {
		result, err = executeWithBuffering(ctx, qe)
	}
	if err != nil {
		return nil, err
	}

	finalizeMetrics(qe.Metrics, qe.SysVars)
	result.Metrics = qe.Metrics
	return result, nil
}

// executeSQLImplWithTxn executes SQL with specific system variables and within a given transaction.
// This is for use when we have a specific transaction to use.
func executeSQLImplWithTxn(ctx context.Context, session *Session, txn *spanner.ReadOnlyTransaction, sql string, sysVars *systemVariables) (*Result, error) {
	m := newMetrics(sysVars)

	fc, vfm, sysVars, err := prepareFormatConfig(sql, sysVars)
	if err != nil {
		return nil, err
	}

	stmt, err := newStatement(sql, sysVars.Params, false)
	if err != nil {
		return nil, err
	}

	// Always use ExecuteSqlRequest_PROFILE mode to get execution statistics from Spanner.
	opts := spanner.QueryOptions{
		Mode:     sppb.ExecuteSqlRequest_PROFILE.Enum(),
		Priority: sysVars.Query.RPCPriority,
	}
	iter := txn.QueryWithOptions(ctx, stmt, opts)

	return executeAndCollect(ctx, &queryExecution{
		Session:      session,
		Iter:         iter,
		ReadOnlyTxn:  txn,
		FormatConfig: fc,
		SQL:          sql,
		SysVars:      sysVars,
		Metrics:      m,
		ValueFmtMode: vfm,
	})
}

// executeSQLImplWithVars is the actual implementation that accepts custom system variables
func executeSQLImplWithVars(ctx context.Context, session *Session, sql string, sysVars *systemVariables) (*Result, error) {
	m := newMetrics(sysVars)

	fc, vfm, sysVars, err := prepareFormatConfig(sql, sysVars)
	if err != nil {
		return nil, err
	}

	stmt, err := newStatement(sql, sysVars.Params, false)
	if err != nil {
		return nil, err
	}

	iter, roTxn := session.RunQueryWithStats(ctx, stmt, false)

	result, err := executeAndCollect(ctx, &queryExecution{
		Session:      session,
		Iter:         iter,
		ReadOnlyTxn:  roTxn,
		FormatConfig: fc,
		SQL:          sql,
		SysVars:      sysVars,
		Metrics:      m,
		ValueFmtMode: vfm,
	})
	if err != nil {
		// Handle aborted transaction
		if session.InReadWriteTransaction() && spanner.ErrCode(err) == codes.Aborted {
			rollback := &RollbackStatement{}
			if _, rollbackErr := rollback.Execute(ctx, session); rollbackErr != nil {
				return nil, errors.Join(err, fmt.Errorf("error on rollback: %w", rollbackErr))
			}
		}
		return nil, err
	}

	// Store the SQL table name if we're using a format that requires SQL literals
	if vfm == format.SQLLiteralValues && sysVars.Display.SQLTableName != "" {
		result.SQLTableNameForExport = sysVars.Display.SQLTableName
	}

	return result, nil
}

// decideExecutionMode determines whether to use streaming or buffered mode.
// Returns true and a processor if streaming should be used, false and nil otherwise.
func decideExecutionMode(sysVars *systemVariables) (bool, RowProcessor) {
	// Get output writer from StreamManager (respects tee/redirect settings)
	outStream := sysVars.StreamManager.GetWriter()
	if outStream == nil {
		return false, nil
	}

	// Determine screen width based on system variables
	screenWidth := math.MaxInt
	if sysVars.Display.AutoWrap {
		if sysVars.Display.FixedWidth != nil {
			screenWidth = int(*sysVars.Display.FixedWidth)
		} else {
			// Get terminal width from StreamManager
			width, err := sysVars.StreamManager.GetTerminalWidth()
			if err != nil {
				// If terminal width cannot be determined, don't wrap
				screenWidth = math.MaxInt
			} else {
				screenWidth = width
			}
		}
	}

	// Try to create streaming processor based on settings
	processor, _ := createStreamingProcessor(sysVars, outStream, screenWidth)
	return processor != nil, processor
}

// executeWithStreaming executes the query using streaming mode.
func executeWithStreaming(ctx context.Context, qe *queryExecution) (*Result, error) {
	// Collect memory stats if debug logging is enabled
	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		metrics.LogMemoryStats("Before streaming")
		defer metrics.LogMemoryStats("After streaming")
	}

	slog.Debug("Using streaming mode", "startTime", time.Now().Format(time.RFC3339Nano))
	return executeStreamingSQL(ctx, qe)
}

// finalizeQueryResult parses query stats, extracts the read timestamp, and updates the query cache.
func finalizeQueryResult(result *Result, stats map[string]any, roTxn *spanner.ReadOnlyTransaction, plan *sppb.QueryPlan, sysVars *systemVariables, m *metrics.ExecutionMetrics) error {
	queryStats, err := parseQueryStats(stats)
	if err != nil {
		return err
	}
	result.Stats = queryStats
	m.ServerElapsedTime = queryStats.ElapsedTime
	m.ServerCPUTime = queryStats.CPUTime

	if roTxn != nil {
		ts, err := roTxn.Timestamp()
		if err != nil {
			slog.Warn("failed to get read-only transaction timestamp", "err", err)
		} else {
			result.ReadTimestamp = ts
		}
	}

	sysVars.Internal.LastQueryCache = &LastQueryCache{
		QueryPlan:     plan,
		QueryStats:    stats,
		ReadTimestamp: result.ReadTimestamp,
	}
	return nil
}

// executeWithBuffering executes the query using buffered mode.
func executeWithBuffering(ctx context.Context, qe *queryExecution) (*Result, error) {
	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		metrics.LogMemoryStats("Before buffered")
		defer metrics.LogMemoryStats("After buffered")
	}

	slog.Debug("Using buffered mode", "startTime", time.Now().Format(time.RFC3339Nano))

	rows, stats, _, metadata, plan, err := consumeRowIterCollectWithMetrics(qe.Iter, spannerRowToRow(qe.FormatConfig, qe.SysVars.typeStyles, qe.SysVars.nullStyle), qe.Metrics)
	if err != nil {
		return nil, err
	}

	slog.Debug("Buffered mode complete",
		"endTime", time.Now().Format(time.RFC3339Nano),
		"rowCount", len(rows))

	result := &Result{
		Rows:                  rows,
		TableHeader:           toTableHeader(metadata.GetRowType().GetFields()),
		AffectedRows:          len(rows),
		HasSQLFormattedValues: qe.ValueFmtMode == format.SQLLiteralValues,
	}

	if err := finalizeQueryResult(result, stats, qe.ReadOnlyTxn, plan, qe.SysVars, qe.Metrics); err != nil {
		return nil, err
	}
	return result, nil
}

// executeStreamingSQL processes query results in streaming mode.
func executeStreamingSQL(ctx context.Context, qe *queryExecution) (*Result, error) {
	slog.Debug("executeStreamingSQL called", "format", qe.SysVars.Display.CLIFormat)

	rowTransform := spannerRowToRow(qe.FormatConfig, qe.SysVars.typeStyles, qe.SysVars.nullStyle)
	slog.Debug("executeStreamingSQL calling consumeRowIterWithProcessor")
	stats, rowCount, metadata, plan, err := consumeRowIterWithProcessor(qe.Iter, qe.Processor, rowTransform, qe.SysVars, qe.Metrics)
	slog.Debug("executeStreamingSQL after consumeRowIterWithProcessor", "err", err, "metadata", metadata != nil, "rowCount", rowCount)
	if err != nil {
		return nil, err
	}

	result := &Result{
		Rows:                  nil,
		TableHeader:           toTableHeader(metadata.GetRowType().GetFields()),
		AffectedRows:          int(rowCount),
		Streamed:              true,
		HasSQLFormattedValues: qe.ValueFmtMode == format.SQLLiteralValues,
	}

	if err := finalizeQueryResult(result, stats, qe.ReadOnlyTxn, plan, qe.SysVars, qe.Metrics); err != nil {
		return nil, err
	}
	return result, nil
}

// createStreamingProcessor creates the appropriate streaming processor based on format and streaming mode.
// Returns nil if streaming should not be used (based on StreamingMode setting and format).
func createStreamingProcessor(sysVars *systemVariables, out io.Writer, screenWidth int) (RowProcessor, error) {
	// Check if streaming should be used based on mode
	shouldStream := false
	switch sysVars.Query.StreamingMode {
	case enums.StreamingModeTrue:
		// Always stream if format supports it
		shouldStream = true
	case enums.StreamingModeFalse:
		// Never stream
		return nil, nil
	case enums.StreamingModeAuto:
		// AUTO mode: decide based on format
		switch sysVars.Display.CLIFormat {
		case enums.DisplayModeTable, enums.DisplayModeTableComment, enums.DisplayModeTableDetailComment:
			// Table formats: buffer by default for accurate column widths
			shouldStream = false
		case enums.DisplayModeCSV, enums.DisplayModeTab, enums.DisplayModeVertical, enums.DisplayModeHTML, enums.DisplayModeXML,
			enums.DisplayModeSQLInsert, enums.DisplayModeSQLInsertOrIgnore, enums.DisplayModeSQLInsertOrUpdate:
			// Other formats: stream by default for better performance
			shouldStream = true
		default:
			// Unknown format: buffer for safety
			shouldStream = false
		}
	default:
		// Unknown mode: buffer for safety
		return nil, nil
	}

	if !shouldStream {
		return nil, nil
	}

	// Use the shared processor creation logic to avoid duplication
	return createStreamingProcessorForMode(sysVars.Display.CLIFormat, out, sysVars, screenWidth)
}
