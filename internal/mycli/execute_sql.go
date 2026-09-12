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

// queryRendering is the per-query subset of display inputs. Query execution
// and mutation keep the live *systemVariables; this value never carries
// Registry, callbacks, Params, LastResult, or StreamManager.
//
// Query mode, metrics, cache destination, progress lifecycle, and summary
// policy stay on the live settings (and outside format.FormatConfig).
type queryRendering struct {
	CLIFormat     enums.DisplayMode
	StreamingMode enums.StreamingMode
	Formatter     format.FormatConfig
	Export        exportWriterOptions
	ValueFmtMode  format.ValueFormatMode
	Spanvalue     *spanvalue.FormatConfig
	TypeStyles    map[sppb.TypeCode]string
	NullStyle     string
}

func queryRenderingFrom(sysVars *systemVariables) queryRendering {
	if sysVars == nil {
		return queryRendering{}
	}
	return queryRendering{
		CLIFormat:     sysVars.Display.CLIFormat,
		StreamingMode: sysVars.Query.StreamingMode,
		Formatter:     sysVars.toFormatConfig(),
		Export:        exportWriterOptionsFrom(sysVars),
		TypeStyles:    sysVars.typeStyles,
		NullStyle:     sysVars.nullStyle,
	}
}

// withExecuteOverrides applies DUMP's format/streaming/table/header overrides
// to this rendering value. It does not mutate live settings and does not put
// summary or progress policy onto the rendering value.
func (r queryRendering) withExecuteOverrides(mode enums.DisplayMode, streaming enums.StreamingMode, sqlTableName string) queryRendering {
	r.CLIFormat = mode
	r.StreamingMode = streaming
	r.Export.CLIFormat = mode
	if sqlTableName != "" {
		r.Export.SQLTableName = sqlTableName
	}
	r.Export.SkipColumnNames = true
	r.Formatter.SkipColumnNames = true
	return r
}

// executeSQLWithFormatAndTxn executes SQL with specific format settings and
// within a given transaction. out is the operation destination; DUMP overrides
// a local copy's writer for internal buffering while keeping the outer width.
// dro is the caller's captured directed-read option for this read-only request.
func executeSQLWithFormatAndTxn(ctx context.Context, session *Session, txn *spanner.ReadOnlyTransaction, sql string, format enums.DisplayMode, streamingMode enums.StreamingMode, sqlTableName string, dro *sppb.DirectedReadOptions, out OperationOutput) (*Result, error) {
	render := queryRenderingFrom(session.systemVariables).withExecuteOverrides(format, streamingMode, sqlTableName)
	return executeSQLImplWithTxn(ctx, session, txn, sql, render, dro, out)
}

func executeSQL(ctx context.Context, session *Session, sql string, out OperationOutput) (*Result, error) {
	return executeSQLImpl(ctx, session, sql, out)
}

// executeSQLImpl delegates to executeSQLImplWithVars with the session's system variables
func executeSQLImpl(ctx context.Context, session *Session, sql string, out OperationOutput) (*Result, error) {
	return executeSQLImplWithVars(ctx, session, sql, session.systemVariables, out)
}

// prepareFormatConfig fills the spanvalue formatter, value-format mode, and
// SQL-export table name on render. sysVars is the live settings object used
// only for proto decoder inputs; it is never copied or replaced.
// Failed auto-detection leaves the table name empty and does not fail the
// query. An explicit name on render wins over detection.
func prepareFormatConfig(sql string, sysVars *systemVariables, render queryRendering) (queryRendering, error) {
	vfm := format.ValueFormatModeFor(format.Mode(render.CLIFormat.String()))
	render.ValueFmtMode = vfm

	switch vfm {
	case format.SQLLiteralValues:
		render.Spanvalue = sqlLiteralFormatConfig()
		if render.Export.SQLTableName == "" {
			detectedTableName, detectionErr := extractTableNameFromQuery(sql)
			if detectedTableName != "" {
				render.Export.SQLTableName = detectedTableName
				slog.Debug("Auto-detected table name for SQL export", "table", detectedTableName)
			} else if detectionErr != nil {
				slog.Debug("Table name auto-detection failed", "reason", detectionErr.Error())
			}
		}
		return render, nil
	case format.JSONValues:
		render.Spanvalue = decoder.JSONFormatConfig()
		return render, nil
	default:
		if sysVars == nil {
			fc, err := decoder.FormatConfigWithProto(nil, false)
			render.Spanvalue = fc
			return render, err
		}
		fc, err := decoder.FormatConfigWithProto(sysVars.Internal.ProtoDescriptor, sysVars.Display.MultilineProtoText)
		render.Spanvalue = fc
		return render, err
	}
}

// sqlLiteralFormatConfig keeps SQL export and typed replay on the same policy.
func sqlLiteralFormatConfig() *spanvalue.FormatConfig {
	// spanvalue v0.8.4 emits CAST(-0 AS FLOAT32), whose integer operand loses
	// the sign on replay. Remove this bridge after adopting an upstream version
	// that preserves FLOAT32 negative zero. A typed plugin also covers nested
	// values without rewriting matching text inside STRING or JSON literals.
	return spanvalue.LiteralFormatConfig().WithComplexPlugin(spanvalue.PluginForTypeCode(
		sppb.TypeCode_FLOAT32,
		func(_ spanvalue.Formatter, value spanner.GenericColumnValue, _ bool) (string, error) {
			var f spanner.NullFloat32
			if err := value.Decode(&f); err != nil {
				return "", err
			}
			if f.Valid && f.Float32 == 0 && math.Signbit(float64(f.Float32)) {
				return "CAST(-0.0 AS FLOAT32)", nil
			}
			return "", spanvalue.ErrFallthrough
		},
	))
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
	Session     *Session
	Out         OperationOutput
	Iter        *spanner.RowIterator
	ReadOnlyTxn *spanner.ReadOnlyTransaction
	SQL         string
	SysVars     *systemVariables // live settings; never a per-query copy
	Render      queryRendering
	Metrics     *metrics.ExecutionMetrics
	Processor   RowProcessor // set by executeAndCollect after decideExecutionMode
	// QueryCacheDest is the caller's LastResult.QueryCache slot. It is captured
	// from the live settings before prepareFormatConfig fills per-query
	// rendering. Nil skips publication (DUMP's executeSQLWithFormatAndTxn
	// path). Ownership is never inferred from pointer equality or display format.
	QueryCacheDest **LastQueryCache
}

func (qe *queryExecution) outputWriter() io.Writer {
	return qe.Out.Writer()
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
		"format", qe.Render.CLIFormat,
		"sqlTableName", qe.Render.Export.SQLTableName)

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

// executeSQLImplWithTxn executes SQL within a given transaction using live
// session settings for parameters, Mode/Priority, and metrics. render carries
// DUMP format/streaming/table/header overrides; dro is captured for the DUMP request.
func executeSQLImplWithTxn(ctx context.Context, session *Session, txn *spanner.ReadOnlyTransaction, sql string, render queryRendering, dro *sppb.DirectedReadOptions, out OperationOutput) (*Result, error) {
	sysVars := session.systemVariables
	m := newMetrics(sysVars)

	render, err := prepareFormatConfig(sql, sysVars, render)
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
		Mode:                effectiveQueryMode(sysVars.Query.QueryMode).Enum(),
		Priority:            sysVars.Query.RPCPriority,
		DirectedReadOptions: dro,
	}
	iter := txn.QueryWithOptions(ctx, stmt, opts)

	return executeAndCollect(ctx, &queryExecution{
		Session:        session,
		Out:            out,
		Iter:           iter,
		ReadOnlyTxn:    txn,
		SQL:            sql,
		SysVars:        sysVars,
		Render:         render,
		Metrics:        m,
		QueryCacheDest: nil, // DUMP / isolated format+txn execution must not replace the user's cache
	})
}

// executeSQLImplWithVars runs SQL against the live settings object (Params,
// QueryMode, cache destination, metrics). Per-query display overrides live on
// queryRendering, not a copy of sysVars.
func executeSQLImplWithVars(ctx context.Context, session *Session, sql string, sysVars *systemVariables, out OperationOutput) (*Result, error) {
	if _, err := session.txn.FlushAutomaticDML(ctx); err != nil {
		return nil, err
	}
	return executeSQLImplWithQueryRunner(ctx, session, sql, sysVars, session.txn.RunQueryWithStats, true, out)
}

// rollbackReadWriteIfAborted rolls back a live RW owner when err is Aborted so
// RecreateClient can replace the session. The initiating error is preserved.
// Rollback emits no statement output, so this calls the transaction manager
// directly instead of manufacturing a nested Statement.Execute destination.
func rollbackReadWriteIfAborted(ctx context.Context, session *Session, err error) error {
	if err == nil || session == nil || session.txn == nil {
		return err
	}
	if !session.txn.InReadWriteTransaction() || spanner.ErrCode(err) != codes.Aborted {
		return err
	}
	if rollbackErr := session.txn.RollbackReadWriteTransaction(ctx); rollbackErr != nil {
		return errors.Join(err, fmt.Errorf("error on rollback: %w", rollbackErr))
	}
	return err
}

// executeSQLImplSingleUse executes SQL outside the session's explicit
// transaction while preserving normal query options and one-shot request-tag
// consumption.
func executeSQLImplSingleUse(ctx context.Context, session *Session, sql string, sysVars *systemVariables, out OperationOutput) (*Result, error) {
	run := func(ctx context.Context, stmt spanner.Statement, _ bool, mode sppb.ExecuteSqlRequest_QueryMode) (*spanner.RowIterator, *spanner.ReadOnlyTransaction, error) {
		return session.txn.RunSingleUseQueryWithStats(ctx, stmt, mode)
	}
	return executeSQLImplWithQueryRunner(ctx, session, sql, sysVars, run, false, out)
}

type queryWithStatsRunner func(context.Context, spanner.Statement, bool, sppb.ExecuteSqlRequest_QueryMode) (*spanner.RowIterator, *spanner.ReadOnlyTransaction, error)

func executeSQLImplWithQueryRunner(ctx context.Context, session *Session, sql string, sysVars *systemVariables, run queryWithStatsRunner, rollbackActiveTransactionOnAbort bool, out OperationOutput) (*Result, error) {
	// Direct Execute tests often pass a zero OperationOutput after swapping
	// StreamManager. Resolve here so a nil writer still uses StreamManager,
	// without mutating Session. Caller-provided writers still win.
	out = session.resolveOperationOutput(out)
	m := newMetrics(sysVars)

	// Capture the caller's cache slot before prepareFormatConfig fills
	// per-query rendering. DUMP's executeSQLWithFormatAndTxn path passes nil.
	queryCacheDest := &sysVars.LastResult.QueryCache

	render, err := prepareFormatConfig(sql, sysVars, queryRenderingFrom(sysVars))
	if err != nil {
		return nil, err
	}

	stmt, err := newStatement(sql, sysVars.Params, false)
	if err != nil {
		return nil, err
	}

	iter, roTxn, err := run(ctx, stmt, false, effectiveQueryMode(sysVars.Query.QueryMode))
	if err != nil {
		return nil, err
	}

	result, err := executeAndCollect(ctx, &queryExecution{
		Session:        session,
		Out:            out,
		Iter:           iter,
		ReadOnlyTxn:    roTxn,
		SQL:            sql,
		SysVars:        sysVars,
		Render:         render,
		Metrics:        m,
		QueryCacheDest: queryCacheDest,
	})
	if err == nil && session != nil && session.txn != nil {
		err = session.txn.invokeQueryAfterCollectHook()
	}
	if err != nil {
		if rollbackActiveTransactionOnAbort {
			return nil, rollbackReadWriteIfAborted(ctx, session, err)
		}
		return nil, err
	}

	if render.ValueFmtMode == format.SQLLiteralValues && render.Export.SQLTableName != "" {
		result.SQLTableNameForExport = render.Export.SQLTableName
	}

	return result, nil
}

// decideExecutionMode determines whether to use streaming or buffered mode.
// Returns true and a processor if streaming should be used, false and nil otherwise.
// Spanvalue-writer formats (CSV/JSONL/SQL_INSERT*) stream without a
// RowProcessor: executeStreamingSQLWithSpanvalueWriter creates and validates
// the writer itself.
func decideExecutionMode(qe *queryExecution) (bool, RowProcessor, error) {
	// The explicit operation destination takes precedence over the statement
	// destination, which respects tee/redirect settings and MCP capture.
	outStream := qe.outputWriter()
	if outStream == nil {
		return false, nil, nil
	}

	if usesSpanvalueWriter(qe.Render.CLIFormat) {
		return true, nil, nil
	}

	screenWidth := qe.Out.ScreenWidth()

	// Try to create streaming processor based on settings
	processor, err := streamingProcessorFor(qe.Render, outStream, screenWidth)
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

// finalizeQueryResult parses query stats, extracts the read timestamp, and
// publishes the query cache. Publication uses QueryCacheDest, not SysVars:
// DUMP passes nil so per-table reads do not replace the user's last-query
// cache. Timing is after query-stat parsing and read-timestamp collection
// and before appendix rendering; a later appendix or after-collect hook
// failure does not clear an already published cache. Iterator or parse
// failure never reaches here, so the previous cache remains.
func (qe *queryExecution) finalizeQueryResult(result *Result, stats map[string]any, plan *sppb.QueryPlan) error {
	queryStats, err := parseQueryStats(stats)
	if err != nil {
		return err
	}
	result.Stats = queryStats
	qe.Metrics.ServerElapsedTime = queryStats.ElapsedTime
	qe.Metrics.ServerCPUTime = queryStats.CPUTime

	if qe.ReadOnlyTxn != nil {
		ts, err := qe.ReadOnlyTxn.Timestamp()
		if err != nil {
			slog.Warn("failed to get read-only transaction timestamp", "err", err)
		} else {
			result.ReadTimestamp = ts
		}
	}

	if qe.QueryCacheDest != nil {
		*qe.QueryCacheDest = &LastQueryCache{
			QueryPlan:     plan,
			QueryStats:    stats,
			ReadTimestamp: result.ReadTimestamp,
		}
	}

	return applyQueryModeStatsRendering(result, plan, qe.SysVars)
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
		Body: TypedBody(&TypedRows{
			Metadata:         metadata,
			Rows:             rows,
			SQLExportAllowed: qe.Render.ValueFmtMode == format.SQLLiteralValues,
		}),
		TableHeader:  toTableHeader(metadata.GetRowType().GetFields()),
		AffectedRows: len(rows),
	}

	if err := qe.finalizeQueryResult(result, stats, plan); err != nil {
		return nil, err
	}
	return result, nil
}

// executeStreamingSQL processes query results in streaming mode.
func executeStreamingSQL(ctx context.Context, qe *queryExecution) (*Result, error) {
	slog.Debug("executeStreamingSQL called", "format", qe.Render.CLIFormat)

	if result, handled, err := executeStreamingSQLWithSpanvalueWriter(qe); handled || err != nil {
		return result, err
	}
	return executeStreamingSQLWithSpanvalueProcessor(qe)
}

// streamingProcessorFor creates the appropriate streaming processor based on format and streaming mode.
// Non-table formats are always streaming because they do not benefit from row buffering.
// For table formats, CLI_TABLE_STREAMING controls whether to trade layout quality for immediate output.
// Spanvalue-writer formats (CSV/JSONL/SQL_INSERT*) never reach this function:
// decideExecutionMode routes them to the spanvalue writer path directly.
func streamingProcessorFor(render queryRendering, out io.Writer, screenWidth int) (RowProcessor, error) {
	fmtMode := format.Mode(render.CLIFormat.String())
	if fmtMode.IsTableMode() || fmtMode == format.ModeUnspecified {
		switch render.StreamingMode {
		case enums.StreamingModeTrue:
			return streamingProcessorForMode(render, out, screenWidth)
		default:
			// Table formats buffer by default for accurate column widths.
			return nil, nil
		}
	}

	// Non-table formats always stream regardless of CLI_TABLE_STREAMING; only
	// guard against an unexpected enum value.
	switch render.StreamingMode {
	case enums.StreamingModeTrue, enums.StreamingModeFalse, enums.StreamingModeAuto:
		return streamingProcessorForMode(render, out, screenWidth)
	default:
		return nil, nil
	}
}
