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
	"github.com/apstndb/spanner-mycli/internal/mycli/metrics"
	"github.com/apstndb/spanvalue"
	"github.com/samber/lo"
	"google.golang.org/grpc/codes"
)

// effectiveQueryMode resolves the request-level ExecuteSqlRequest.QueryMode for
// regular statement execution from the user-specified CLI_QUERY_MODE.
//
// The CLI defaults to PROFILE so execution statistics are always available for
// verbose output, CLI_INLINE_STATS, and EXPLAIN LAST QUERY. CLI_QUERY_MODE=PLAN
// and PROFILE are dispatched to the EXPLAIN / EXPLAIN ANALYZE execution paths
// before reaching regular execution, so only WITH_STATS and WITH_PLAN_AND_STATS
// need to be respected here; other values (nil, NORMAL) keep the PROFILE default.
func effectiveQueryMode(userMode *sppb.ExecuteSqlRequest_QueryMode) sppb.ExecuteSqlRequest_QueryMode {
	switch mode := lo.FromPtr(userMode); mode {
	case sppb.ExecuteSqlRequest_WITH_STATS, sppb.ExecuteSqlRequest_WITH_PLAN_AND_STATS:
		return mode
	default:
		return sppb.ExecuteSqlRequest_PROFILE
	}
}

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
func prepareFormatConfig(sql string, sysVars *systemVariables) (*spanvalue.FormatConfig, format.ValueFormatMode, *systemVariables, error) {
	fmtMode := format.Mode(sysVars.Display.CLIFormat.String())
	vfm := format.ValueFormatModeFor(fmtMode)

	switch vfm {
	case format.SQLLiteralValues:
		// Use SQL literal formatting for modes that declared SQLLiteralValues
		// LiteralFormatConfig formats values as valid Spanner SQL literals
		fc := spanvalue.LiteralFormatConfig()

		// Auto-detect table name if not explicitly set
		if sysVars.Display.SQLTableName == "" {
			detectedTableName, detectionErr := extractTableNameFromQuery(sql)
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
	case format.JSONValues:
		// Use JSON formatting: each value becomes a valid JSON fragment
		return decoder.JSONFormatConfig(), vfm, sysVars, nil
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
	useStreaming, processor, err := decideExecutionMode(qe)
	if err != nil {
		if qe.Iter != nil {
			qe.Iter.Stop()
		}
		return nil, err
	}
	qe.Metrics.IsStreaming = useStreaming
	qe.Processor = processor

	slog.Debug("executeSQL decision",
		"useStreaming", useStreaming,
		"format", qe.SysVars.Display.CLIFormat,
		"sqlTableName", qe.SysVars.Display.SQLTableName)

	var result *Result
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

	// Resolve the request-level query mode from CLI_QUERY_MODE; the default is
	// PROFILE so execution statistics are always available from Spanner.
	opts := spanner.QueryOptions{
		Mode:     effectiveQueryMode(sysVars.Query.QueryMode).Enum(),
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

	iter, roTxn, err := session.RunQueryWithStats(ctx, stmt, false, effectiveQueryMode(sysVars.Query.QueryMode))
	if err != nil {
		return nil, err
	}

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
// Spanvalue-writer formats (CSV/JSONL/SQL_INSERT*) stream without a
// RowProcessor: executeStreamingSQLWithSpanvalueWriter creates and validates
// the writer itself.
func decideExecutionMode(qe *queryExecution) (bool, RowProcessor, error) {
	// The per-statement output destination (respects tee/redirect settings
	// and the MCP handler's capture buffer).
	outStream := qe.Session.outputWriter()
	if outStream == nil {
		return false, nil, nil
	}

	if usesSpanvalueWriter(qe.SysVars.Display.CLIFormat) {
		return true, nil, nil
	}

	screenWidth := qe.Session.displayWidthFor(qe.SysVars)

	// Try to create streaming processor based on settings
	processor, err := createStreamingProcessor(qe.SysVars, outStream, screenWidth)
	if err != nil {
		return false, nil, err
	}
	return processor != nil, processor, nil
}

func displayScreenWidth(sysVars *systemVariables) int {
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
	return screenWidth
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

	sysVars.LastResult.QueryCache = &LastQueryCache{
		QueryPlan:     plan,
		QueryStats:    stats,
		ReadTimestamp: result.ReadTimestamp,
	}

	return applyQueryModeStatsRendering(result, plan, sysVars)
}

// applyQueryModeStatsRendering reflects the user-specified stats query modes
// in the presentation of result: stats are rendered even without
// CLI_VERBOSE, and WITH_PLAN_AND_STATS additionally renders the query plan as
// a result appendix, when a plan is available (e.g. absent for the Cloud
// Spanner Emulator). This is shared between SELECT result construction
// (finalizeQueryResult) and DML result construction (buildDMLResult) so both
// honor CLI_QUERY_MODE identically.
func applyQueryModeStatsRendering(result *Result, plan *sppb.QueryPlan, sysVars *systemVariables) error {
	switch lo.FromPtr(sysVars.Query.QueryMode) {
	case sppb.ExecuteSqlRequest_WITH_STATS:
		result.ForceVerbose = true
	case sppb.ExecuteSqlRequest_WITH_PLAN_AND_STATS:
		result.ForceVerbose = true
		if plan != nil {
			appendices, err := buildQueryPlanAppendix(sysVars, plan)
			if err != nil {
				return err
			}
			result.Appendices = append(result.Appendices, appendices...)
		}
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

	// Capture the raw typed rows (identity transform); display formatting is
	// deferred to printTableData so every CLI_FORMAT re-renders from values
	// (issue #738). Format is therefore no longer decided at collection time.
	rows, stats, _, metadata, plan, err := consumeRowIterCollectWithMetrics(
		qe.Iter, func(r *spanner.Row) (*spanner.Row, error) { return r, nil }, qe.Metrics)
	if err != nil {
		return nil, err
	}

	slog.Debug("Buffered mode complete",
		"endTime", time.Now().Format(time.RFC3339Nano),
		"rowCount", len(rows))

	result := &Result{
		Typed: &TypedRows{
			Metadata:         metadata,
			Rows:             rows,
			SQLExportAllowed: qe.ValueFmtMode == format.SQLLiteralValues,
		},
		TableHeader:  toTableHeader(metadata.GetRowType().GetFields()),
		AffectedRows: len(rows),
	}

	if err := finalizeQueryResult(result, stats, qe.ReadOnlyTxn, plan, qe.SysVars, qe.Metrics); err != nil {
		return nil, err
	}
	return result, nil
}

// executeStreamingSQL processes query results in streaming mode.
func executeStreamingSQL(ctx context.Context, qe *queryExecution) (*Result, error) {
	slog.Debug("executeStreamingSQL called", "format", qe.SysVars.Display.CLIFormat)

	if result, handled, err := executeStreamingSQLWithSpanvalueWriter(qe); handled || err != nil {
		return result, err
	}
	return executeStreamingSQLWithSpanvalueProcessor(qe)
}

// createStreamingProcessor creates the appropriate streaming processor based on format and streaming mode.
// Non-table formats are always streaming because they do not benefit from row buffering.
// For table formats, CLI_TABLE_STREAMING controls whether to trade layout quality for immediate output.
// Spanvalue-writer formats (CSV/JSONL/SQL_INSERT*) never reach this function:
// decideExecutionMode routes them to the spanvalue writer path directly.
func createStreamingProcessor(sysVars *systemVariables, out io.Writer, screenWidth int) (RowProcessor, error) {
	fmtMode := format.Mode(sysVars.Display.CLIFormat.String())
	if fmtMode.IsTableMode() || fmtMode == format.ModeUnspecified {
		switch sysVars.Query.StreamingMode {
		case enums.StreamingModeTrue:
			return createStreamingProcessorForMode(sysVars.Display.CLIFormat, out, sysVars, screenWidth)
		default:
			// Table formats buffer by default for accurate column widths.
			return nil, nil
		}
	}

	// Non-table formats always stream regardless of CLI_TABLE_STREAMING; only
	// guard against an unexpected enum value.
	switch sysVars.Query.StreamingMode {
	case enums.StreamingModeTrue, enums.StreamingModeFalse, enums.StreamingModeAuto:
		return createStreamingProcessorForMode(sysVars.Display.CLIFormat, out, sysVars, screenWidth)
	default:
		return nil, nil
	}
}
