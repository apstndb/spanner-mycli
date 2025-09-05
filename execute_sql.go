package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/apstndb/gsqlutils"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanvalue"
	"github.com/ngicks/go-iterator-helper/hiter"
	"github.com/sourcegraph/conc/pool"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/apstndb/lox"
	"github.com/go-json-experiment/json"

	"cloud.google.com/go/spanner"
	adminapi "cloud.google.com/go/spanner/admin/database/apiv1"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/mattn/go-runewidth"
	"github.com/samber/lo"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
)

// executeSQLWithFormatAndTxn executes SQL with specific format settings and within a given transaction.
// This is for use within withReadOnlyTransaction callbacks where we already have a transaction.
func executeSQLWithFormatAndTxn(ctx context.Context, session *Session, txn *spanner.ReadOnlyTransaction, sql string, format enums.DisplayMode, streamingMode enums.StreamingMode, sqlTableName string) (*Result, error) {
	// Create a copy of the system variables for this specific execution
	tempVars := *session.systemVariables

	// Set temporary values on the copy
	tempVars.CLIFormat = format
	tempVars.StreamingMode = streamingMode
	if sqlTableName != "" {
		tempVars.SQLTableName = sqlTableName
	}
	tempVars.SkipColumnNames = true
	tempVars.SuppressResultLines = true
	tempVars.EnableProgressBar = false

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
// It returns the format config, whether SQL literals are being used, and the potentially modified sysVars.
// This is extracted as a common function to avoid duplication between executeSQLImplWithTxn and executeSQLImplWithVars.
func prepareFormatConfig(sql string, sysVars *systemVariables) (*spanvalue.FormatConfig, bool, *systemVariables, error) {
	var fc *spanvalue.FormatConfig
	var usingSQLLiterals bool
	var err error

	switch sysVars.CLIFormat {
	case enums.DisplayModeSQLInsert, enums.DisplayModeSQLInsertOrIgnore, enums.DisplayModeSQLInsertOrUpdate:
		// Use SQL literal formatting for SQL export modes
		// LiteralFormatConfig formats values as valid Spanner SQL literals
		fc = spanvalue.LiteralFormatConfig
		usingSQLLiterals = true
		err = nil

		// Auto-detect table name if not explicitly set
		if sysVars.SQLTableName == "" {
			detectedTableName, detectionErr := extractTableNameFromQuery(sql)
			if detectedTableName != "" {
				// Create a copy of sysVars to use the detected table name for this execution only.
				// This is important for:
				// 1. Scope isolation: auto-detection only affects this specific query execution
				// 2. Thread safety: if sysVars is shared across goroutines, we don't modify the original
				// 3. Preserving user settings: the original CLI_SQL_TABLE_NAME remains unchanged
				tempVars := *sysVars
				tempVars.SQLTableName = detectedTableName
				sysVars = &tempVars
				slog.Debug("Auto-detected table name for SQL export", "table", detectedTableName)
			} else if detectionErr != nil {
				// Log why auto-detection failed for debugging
				slog.Debug("Table name auto-detection failed", "reason", detectionErr.Error())
			}
		}
	default:
		// Use regular display formatting for other modes
		// formatConfigWithProto handles custom proto descriptors if set
		fc, err = formatConfigWithProto(sysVars.ProtoDescriptor, sysVars.MultilineProtoText)
		usingSQLLiterals = false
	}

	return fc, usingSQLLiterals, sysVars, err
}

// executeSQLImplWithTxn executes SQL with specific system variables and within a given transaction.
// This is for use when we have a specific transaction to use.
func executeSQLImplWithTxn(ctx context.Context, session *Session, txn *spanner.ReadOnlyTransaction, sql string, sysVars *systemVariables) (*Result, error) {
	// Always collect metrics - display is controlled by template based on Profile flag
	metrics := &ExecutionMetrics{
		QueryStartTime: time.Now(),
		Profile:        sysVars.Profile,
	}

	// Capture memory snapshot before execution if profiling is enabled
	if sysVars.Profile {
		before := GetMemoryStats()
		metrics.MemoryBefore = &before
	}

	// Choose the appropriate format config based on the output format
	fc, usingSQLLiterals, sysVars, err := prepareFormatConfig(sql, sysVars)
	if err != nil {
		return nil, err
	}

	params := sysVars.Params
	stmt, err := newStatement(sql, params, false)
	if err != nil {
		return nil, err
	}

	// Prepare query options to preserve profile mode and priority
	// Note: We always use ExecuteSqlRequest_PROFILE mode (not NORMAL) because:
	// 1. Spanner historically doesn't return any execution summary in NORMAL mode
	// 2. spanner-mycli needs execution statistics even when not displaying query plan trees
	// 3. sysVars.Profile is for CLI tool profiling (memory stats, etc.), NOT for Spanner's PROFILE mode
	//    - sysVars.Profile=true: Enables CLI profiling features (memory tracking)
	//    - ExecuteSqlRequest_PROFILE: Always used to get execution statistics from Spanner
	opts := spanner.QueryOptions{
		Mode:     sppb.ExecuteSqlRequest_PROFILE.Enum(),
		Priority: sysVars.RPCPriority,
	}
	// Use the transaction directly with query options
	iter := txn.QueryWithOptions(ctx, stmt, opts)

	// Decide whether to use streaming or buffered mode
	useStreaming, processor := decideExecutionMode(ctx, session, fc, sysVars)
	metrics.IsStreaming = useStreaming

	slog.Debug("executeSQL decision",
		"useStreaming", useStreaming,
		"format", sysVars.CLIFormat,
		"sqlTableName", sysVars.SQLTableName)

	// Execute with the appropriate mode
	var result *Result

	if useStreaming {
		result, err = executeWithStreaming(ctx, session, iter, txn, fc, processor, metrics, usingSQLLiterals, sysVars)
	} else {
		result, err = executeWithBuffering(ctx, session, iter, txn, fc, sql, metrics, usingSQLLiterals, sysVars)
	}

	if err != nil {
		return nil, err
	}

	// Set post-execution fields that were not handled by the processor
	metrics.CompletionTime = time.Now()
	// sysVars.Profile enables CLI tool profiling features (memory stats tracking)
	// This is unrelated to Spanner's ExecuteSqlRequest_PROFILE mode
	if sysVars.Profile {
		after := GetMemoryStats()
		metrics.MemoryAfter = &after
	}
	result.Metrics = metrics

	return result, nil
}

// executeSQLImplWithVars is the actual implementation that accepts custom system variables
func executeSQLImplWithVars(ctx context.Context, session *Session, sql string, sysVars *systemVariables) (*Result, error) {
	// Always collect metrics - display is controlled by template based on Profile flag
	metrics := &ExecutionMetrics{
		QueryStartTime: time.Now(),
		Profile:        sysVars.Profile,
	}

	// Capture memory snapshot before execution if profiling is enabled
	if sysVars.Profile {
		before := GetMemoryStats()
		metrics.MemoryBefore = &before
	}

	// Choose the appropriate format config based on the output format
	// TODO(future): Current design formats values early (at Result creation time) which limits flexibility.
	// Ideally, Result should store raw *spanner.Row data and formatters should handle conversion.
	// This would allow:
	// - Different formats for the same data without re-querying
	// - Format-specific optimizations
	// - Better separation of concerns
	// However, this requires significant changes to Result struct and RowProcessor interface.
	fc, usingSQLLiterals, sysVars, err := prepareFormatConfig(sql, sysVars)
	if err != nil {
		return nil, err
	}

	params := sysVars.Params
	stmt, err := newStatement(sql, params, false)
	if err != nil {
		return nil, err
	}

	iter, roTxn := session.RunQueryWithStats(ctx, stmt, false)

	// Decide whether to use streaming or buffered mode
	useStreaming, processor := decideExecutionMode(ctx, session, fc, sysVars)
	metrics.IsStreaming = useStreaming

	slog.Debug("executeSQL decision",
		"useStreaming", useStreaming,
		"format", sysVars.CLIFormat,
		"sqlTableName", sysVars.SQLTableName)

	// Execute with the appropriate mode
	var result *Result

	if useStreaming {
		result, err = executeWithStreaming(ctx, session, iter, roTxn, fc, processor, metrics, usingSQLLiterals, sysVars)
	} else {
		result, err = executeWithBuffering(ctx, session, iter, roTxn, fc, sql, metrics, usingSQLLiterals, sysVars)
	}

	// Complete metrics collection
	metrics.CompletionTime = time.Now()

	// Capture memory snapshot after execution if profiling is enabled
	if sysVars.Profile {
		after := GetMemoryStats()
		metrics.MemoryAfter = &after
	}

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

	// Attach metrics to result
	result.Metrics = metrics

	// Store the SQL table name if we're using SQL export format
	// This ensures the table name is available during formatting phase in buffered mode
	if sysVars.CLIFormat.IsSQLExport() && sysVars.SQLTableName != "" {
		result.SQLTableNameForExport = sysVars.SQLTableName
	}

	return result, nil
}

// decideExecutionMode determines whether to use streaming or buffered mode.
// Returns true and a processor if streaming should be used, false and nil otherwise.
func decideExecutionMode(ctx context.Context, session *Session, fc *spanvalue.FormatConfig, sysVars *systemVariables) (bool, RowProcessor) {
	// Get output writer from StreamManager (respects tee/redirect settings)
	outStream := sysVars.StreamManager.GetWriter()
	if outStream == nil {
		return false, nil
	}

	// Determine screen width based on system variables
	screenWidth := math.MaxInt
	if sysVars.AutoWrap {
		if sysVars.FixedWidth != nil {
			screenWidth = int(*sysVars.FixedWidth)
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
func executeWithStreaming(ctx context.Context, session *Session, iter *spanner.RowIterator, roTxn *spanner.ReadOnlyTransaction, fc *spanvalue.FormatConfig, processor RowProcessor, metrics *ExecutionMetrics, usingSQLLiterals bool, sysVars *systemVariables) (*Result, error) {
	// Collect memory stats if debug logging is enabled
	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		LogMemoryStats("Before streaming")
		defer LogMemoryStats("After streaming")
	}

	slog.Debug("Using streaming mode", "startTime", time.Now().Format(time.RFC3339Nano))
	return executeStreamingSQL(ctx, session, iter, roTxn, fc, processor, metrics, usingSQLLiterals, sysVars)
}

// executeWithBuffering executes the query using buffered mode.
func executeWithBuffering(ctx context.Context, session *Session, iter *spanner.RowIterator, roTxn *spanner.ReadOnlyTransaction, fc *spanvalue.FormatConfig, sql string, metrics *ExecutionMetrics, usingSQLLiterals bool, sysVars *systemVariables) (*Result, error) {
	// Collect memory stats if debug logging is enabled
	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		LogMemoryStats("Before buffered")
		defer LogMemoryStats("After buffered")
	}

	slog.Debug("Using buffered mode", "startTime", time.Now().Format(time.RFC3339Nano))

	// Collect all rows with metrics
	rows, stats, _, metadata, plan, err := consumeRowIterCollectWithMetrics(iter, spannerRowToRow(fc), metrics)
	if err != nil {
		return nil, err
	}

	slog.Debug("Buffered mode complete",
		"endTime", time.Now().Format(time.RFC3339Nano),
		"rowCount", len(rows))

	// Parse query stats
	queryStats, err := parseQueryStats(stats)
	if err != nil {
		return nil, err
	}

	// Extract server-side metrics from stats
	metrics.ServerElapsedTime = queryStats.ElapsedTime
	metrics.ServerCPUTime = queryStats.CPUTime

	// Build result
	result := &Result{
		Rows:                  rows,
		TableHeader:           toTableHeader(metadata.GetRowType().GetFields()),
		AffectedRows:          len(rows),
		Stats:                 queryStats,
		HasSQLFormattedValues: usingSQLLiterals, // Only true when using SQL literal formatting
	}

	// Get transaction timestamp if available
	if roTxn != nil {
		ts, err := roTxn.Timestamp()
		if err != nil {
			slog.Warn("failed to get read-only transaction timestamp", "err", err, "sql", sql)
		} else {
			result.ReadTimestamp = ts
		}
	}

	// Update query cache
	sysVars.LastQueryCache = &LastQueryCache{
		QueryPlan:     plan,
		QueryStats:    stats,
		ReadTimestamp: result.ReadTimestamp,
	}

	return result, nil
}

// executeStreamingSQL processes query results in streaming mode.
// It outputs rows directly as they arrive without buffering the entire result set.
func executeStreamingSQL(ctx context.Context, session *Session, iter *spanner.RowIterator, roTxn *spanner.ReadOnlyTransaction, fc *spanvalue.FormatConfig, processor RowProcessor, metrics *ExecutionMetrics, usingSQLLiterals bool, sysVars *systemVariables) (*Result, error) {
	slog.Debug("executeStreamingSQL called",
		"format", sysVars.CLIFormat)

	// Process the stream with metrics
	rowTransform := spannerRowToRow(fc)
	slog.Debug("executeStreamingSQL calling consumeRowIterWithProcessor")
	stats, rowCount, metadata, plan, err := consumeRowIterWithProcessor(iter, processor, rowTransform, sysVars, metrics)
	slog.Debug("executeStreamingSQL after consumeRowIterWithProcessor", "err", err, "metadata", metadata != nil, "rowCount", rowCount)
	if err != nil {
		return nil, err
	}

	// Parse stats
	queryStats, err := parseQueryStats(stats)
	if err != nil {
		return nil, err
	}

	// Extract server-side metrics from stats
	metrics.ServerElapsedTime = queryStats.ElapsedTime
	metrics.ServerCPUTime = queryStats.CPUTime

	// Create result for metadata (rows already streamed)
	result := &Result{
		Rows:                  nil, // Already streamed
		TableHeader:           toTableHeader(metadata.GetRowType().GetFields()),
		AffectedRows:          int(rowCount),
		Stats:                 queryStats,
		Streamed:              true,             // Mark as streamed
		HasSQLFormattedValues: usingSQLLiterals, // Only true when using SQL literal formatting
	}

	// Handle ReadOnlyTransaction timestamp
	if roTxn != nil {
		ts, err := roTxn.Timestamp()
		if err != nil {
			slog.Warn("failed to get read-only transaction timestamp", "err", err)
		} else {
			result.ReadTimestamp = ts
		}
	}

	// Update last query cache
	sysVars.LastQueryCache = &LastQueryCache{
		QueryPlan:     plan,
		QueryStats:    stats,
		ReadTimestamp: result.ReadTimestamp,
	}

	return result, nil
}

// createStreamingProcessor creates the appropriate streaming processor based on format and streaming mode.
// Returns nil if streaming should not be used (based on StreamingMode setting and format).
func createStreamingProcessor(sysVars *systemVariables, out io.Writer, screenWidth int) (RowProcessor, error) {
	// Check if streaming should be used based on mode
	shouldStream := false
	switch sysVars.StreamingMode {
	case enums.StreamingModeTrue:
		// Always stream if format supports it
		shouldStream = true
	case enums.StreamingModeFalse:
		// Never stream
		return nil, nil
	case enums.StreamingModeAuto:
		// AUTO mode: decide based on format
		switch sysVars.CLIFormat {
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
	return createStreamingProcessorForMode(sysVars.CLIFormat, out, sysVars, screenWidth)
}

func bufferOrExecuteDdlStatements(ctx context.Context, session *Session, ddls []string) (*Result, error) {
	switch b := session.currentBatch.(type) {
	case *BatchDMLStatement:
		return nil, errors.New("there is active batch DML")
	case *BulkDdlStatement:
		b.Ddls = append(b.Ddls, ddls...)
		return &Result{}, nil
	default:
		return executeDdlStatements(ctx, session, ddls)
	}
}

// replacerForProgress replaces tabs and newlines to avoid breaking progress bars.
var replacerForProgress = strings.NewReplacer(
	"\n", " ",
	"\t", " ",
)

func executeDdlStatements(ctx context.Context, session *Session, ddls []string) (*Result, error) {
	if len(ddls) == 0 {
		return &Result{
			TableHeader: toTableHeader(lox.IfOrEmpty(session.systemVariables.EchoExecutedDDL, sliceOf("Executed", "Commit Timestamp"))),
		}, nil
	}

	b, err := proto.Marshal(session.systemVariables.ProtoDescriptor)
	if err != nil {
		return nil, err
	}

	var p *mpb.Progress
	var bars []*mpb.Bar
	teardown := func() {
		for _, bar := range bars {
			bar.Abort(true)
		}
		if p != nil {
			p.Wait()
		}
	}
	if session.systemVariables.EnableProgressBar {
		p = mpb.NewWithContext(ctx)

		for _, ddl := range ddls {
			bar := p.AddBar(int64(100),
				mpb.PrependDecorators(
					decor.Spinner(nil, decor.WCSyncSpaceR),
					decor.Name(runewidth.Truncate(replacerForProgress.Replace(ddl), 40, "..."), decor.WCSyncSpaceR),
					decor.Percentage(decor.WCSyncSpace),
					decor.Elapsed(decor.ET_STYLE_MMSS, decor.WCSyncSpace)),
				mpb.BarRemoveOnComplete(),
			)
			bar.EnableTriggerComplete()
			bars = append(bars, bar)
		}
	}

	op, err := session.adminClient.UpdateDatabaseDdl(ctx, &databasepb.UpdateDatabaseDdlRequest{
		Database:         session.DatabasePath(),
		Statements:       ddls,
		ProtoDescriptors: b,
	})
	if err != nil {
		teardown()
		return nil, fmt.Errorf("error on create op: %w", err)
	}

	// If async mode is enabled, return operation info immediately
	// This allows the client to continue without waiting for the DDL operation to complete.
	// In async DDL, errors are reported when polling, not immediately available.
	if session.systemVariables.AsyncDDL {
		return formatAsyncDdlResult(op)
	}

	for !op.Done() {
		time.Sleep(5 * time.Second)
		err := op.Poll(ctx)
		if err != nil {
			teardown()
			return nil, err
		}

		metadata, err := op.Metadata()
		if err != nil {
			teardown()
			return nil, err
		}

		if bars != nil {
			progresses := metadata.GetProgress()
			for i, progress := range progresses {
				bar := bars[i]
				if bar.Completed() {
					continue
				}
				progressPercent := int64(progress.ProgressPercent)
				bar.SetCurrent(progressPercent)
			}
		}
	}

	metadata, err := op.Metadata()
	if err != nil {
		teardown()
		return nil, err
	}

	if p != nil {
		// force bars are completed even if in emulator
		for _, bar := range bars {
			if bar.Completed() {
				continue
			}
			bar.SetCurrent(100)
		}

		p.Wait()
	}

	lastCommitTS := lo.LastOrEmpty(metadata.CommitTimestamps).AsTime()
	result := &Result{CommitTimestamp: lastCommitTS}
	if session.systemVariables.EchoExecutedDDL {
		result.TableHeader = toTableHeader("Executed", "Commit Timestamp")
		result.Rows = slices.Collect(hiter.Unify(
			func(ddl string, v *timestamppb.Timestamp) Row {
				return toRow(ddl+";", v.AsTime().Format(time.RFC3339Nano))
			},
			hiter.Pairs(slices.Values(ddls), slices.Values(metadata.GetCommitTimestamps())),
		),
		)
	}

	return result, nil
}

func isInsert(sql string) bool {
	token, err := gsqlutils.FirstNonHintToken("", sql)
	if err != nil {
		return false
	}

	return token.IsKeywordLike("INSERT")
}

func bufferOrExecuteDML(ctx context.Context, session *Session, sql string) (*Result, error) {
	switch b := session.currentBatch.(type) {
	case *BatchDMLStatement:
		stmt, err := newStatement(sql, session.systemVariables.Params, false)
		if err != nil {
			return nil, err
		}
		b.DMLs = append(b.DMLs, stmt)
		// Buffered DML is not executed yet, so no IsExecutedDML flag
		return &Result{}, nil
	case *BulkDdlStatement:
		return nil, errors.New("there is active batch DDL")
	default:
		// Get both transaction flags in a single lock acquisition
		inTransaction, inReadWriteTransaction := session.GetTransactionFlagsWithLock()

		if inReadWriteTransaction && session.systemVariables.AutoBatchDML {
			stmt, err := newStatement(sql, session.systemVariables.Params, false)
			if err != nil {
				return nil, err
			}
			session.currentBatch = &BatchDMLStatement{DMLs: []spanner.Statement{stmt}}
			return &Result{}, nil
		}

		if !inTransaction &&
			!isInsert(sql) &&
			session.systemVariables.AutocommitDMLMode == enums.AutocommitDMLModePartitionedNonAtomic {
			return executePDML(ctx, session, sql)
		}

		return executeDML(ctx, session, sql)
	}
}

func executeBatchDML(ctx context.Context, session *Session, dmls []spanner.Statement) (*Result, error) {
	var affectedRowSlice []int64
	result, err := session.RunInNewOrExistRwTx(ctx, func(tx *spanner.ReadWriteStmtBasedTransaction, implicit bool) (affected int64, plan *sppb.QueryPlan, metadata *sppb.ResultSetMetadata, err error) {
		affectedRowSlice, err = tx.BatchUpdateWithOptions(ctx, dmls, spanner.QueryOptions{LastStatement: implicit})
		return lo.Sum(affectedRowSlice), nil, nil, err
	})
	if err != nil {
		return nil, err
	}

	return &Result{
		IsExecutedDML:   true, // This is a batch DML statement
		CommitTimestamp: result.CommitResponse.CommitTs,
		CommitStats:     result.CommitResponse.CommitStats,
		Rows: slices.Collect(hiter.Unify(
			func(s spanner.Statement, n int64) Row {
				return toRow(s.SQL, strconv.FormatInt(n, 10))
			},
			hiter.Pairs(slices.Values(dmls), slices.Values(affectedRowSlice)))),
		TableHeader:      toTableHeader("DML", "Rows"),
		AffectedRows:     int(result.Affected),
		AffectedRowsType: lo.Ternary(len(dmls) > 1, rowCountTypeUpperBound, rowCountTypeExact),
	}, nil
}

func executeDML(ctx context.Context, session *Session, sql string) (*Result, error) {
	stmt, err := newStatement(sql, session.systemVariables.Params, false)
	if err != nil {
		return nil, err
	}

	var rows []Row
	var queryStats map[string]any
	result, err := session.RunInNewOrExistRwTx(ctx, func(tx *spanner.ReadWriteStmtBasedTransaction, implicit bool) (affected int64, plan *sppb.QueryPlan, metadata *sppb.ResultSetMetadata, err error) {
		updateResult, err := session.runUpdateOnTransaction(ctx, tx, stmt, implicit)
		if err != nil {
			return 0, nil, nil, err
		}
		rows = updateResult.Rows
		queryStats = updateResult.Stats
		return updateResult.Count, updateResult.Plan, updateResult.Metadata, nil
	})
	if err != nil {
		return nil, err
	}

	stats, err := parseQueryStats(queryStats)
	if err != nil {
		return nil, err
	}

	session.systemVariables.LastQueryCache = &LastQueryCache{
		QueryPlan:       result.Plan,
		QueryStats:      queryStats,
		CommitTimestamp: result.CommitResponse.CommitTs,
	}

	return &Result{
		IsExecutedDML:         true, // This is a regular DML statement
		CommitTimestamp:       result.CommitResponse.CommitTs,
		CommitStats:           result.CommitResponse.CommitStats,
		Stats:                 stats,
		TableHeader:           toTableHeader(result.Metadata.GetRowType().GetFields()),
		Rows:                  rows,
		AffectedRows:          int(result.Affected),
		HasSQLFormattedValues: false, // DML with THEN RETURN uses regular formatting, not SQL literals
	}, nil
}

// extractColumnNames extract column names from ResultSetMetadata.RowType.Fields.
func extractColumnNames(fields []*sppb.StructType_Field) []string {
	return slices.Collect(hiter.Map((*sppb.StructType_Field).GetName, slices.Values(fields)))
}

// parseQueryStats parses spanner.RowIterator.QueryStats.
func parseQueryStats(stats map[string]any) (QueryStats, error) {
	var queryStats QueryStats

	b, err := json.Marshal(stats)
	if err != nil {
		return queryStats, err
	}

	err = json.Unmarshal(b, &queryStats)
	if err != nil {
		return queryStats, err
	}
	return queryStats, nil
}

// consumeRowIterDiscard calls iter.Stop().
func consumeRowIterDiscard(iter *spanner.RowIterator) (queryStats map[string]interface{}, rowCount int64, metadata *sppb.ResultSetMetadata, queryPlan *sppb.QueryPlan, err error) {
	return consumeRowIter(iter, func(*spanner.Row) error { return nil })
}

// consumeRowIter calls iter.Stop().
func consumeRowIter(iter *spanner.RowIterator, f func(*spanner.Row) error) (queryStats map[string]interface{}, rowCount int64, metadata *sppb.ResultSetMetadata, queryPlan *sppb.QueryPlan, err error) {
	defer iter.Stop()
	err = iter.Do(f)
	if err != nil {
		return nil, 0, nil, nil, err
	}

	return iter.QueryStats, iter.RowCount, iter.Metadata, iter.QueryPlan, nil
}

func consumeRowIterCollect[T any](iter *spanner.RowIterator, f func(*spanner.Row) (T, error)) (rows []T, queryStats map[string]interface{}, rowCount int64, metadata *sppb.ResultSetMetadata, queryPlan *sppb.QueryPlan, err error) {
	var results []T
	stats, count, metadata, plan, err := consumeRowIter(iter, func(row *spanner.Row) error {
		v, err := f(row)
		if err != nil {
			return err
		}
		results = append(results, v)
		return nil
	})

	return results, stats, count, metadata, plan, err
}

// consumeRowIterCollectWithMetrics is like consumeRowIterCollect but collects metrics during execution.
func consumeRowIterCollectWithMetrics[T any](iter *spanner.RowIterator, f func(*spanner.Row) (T, error), metrics *ExecutionMetrics) (rows []T, queryStats map[string]interface{}, rowCount int64, metadata *sppb.ResultSetMetadata, queryPlan *sppb.QueryPlan, err error) {
	var results []T
	firstRow := true

	stats, count, metadata, plan, err := consumeRowIter(iter, func(row *spanner.Row) error {
		now := time.Now()

		// Record TTFB on first row
		if firstRow {
			firstRow = false
			metrics.FirstRowTime = &now
		}

		v, err := f(row)
		if err != nil {
			return err
		}
		results = append(results, v)

		// Update last row time
		metrics.LastRowTime = &now
		metrics.RowCount = int64(len(results))

		return nil
	})

	return results, stats, count, metadata, plan, err
}

func spannerRowToRow(fc *spanvalue.FormatConfig) func(row *spanner.Row) (Row, error) {
	return func(row *spanner.Row) (Row, error) {
		columns, err := fc.FormatRow(row)
		if err != nil {
			return Row{}, err
		}
		return toRow(columns...), nil
	}
}

func runPartitionedQuery(ctx context.Context, session *Session, sql string) (*Result, error) {
	fc, err := formatConfigWithProto(session.systemVariables.ProtoDescriptor, session.systemVariables.MultilineProtoText)
	if err != nil {
		return nil, err
	}

	stmt, err := newStatement(sql, session.systemVariables.Params, false)
	if err != nil {
		return nil, err
	}

	partitions, batchROTx, err := session.RunPartitionQuery(ctx, stmt)
	if err != nil {
		return nil, err
	}
	defer func() {
		batchROTx.Cleanup(ctx)
		batchROTx.Close()
	}()

	type partitionQueryResult struct {
		Metadata *sppb.ResultSetMetadata
		Rows     []Row
	}

	// Go 1.25+ automatically detects container CPU limits on Linux through container-aware GOMAXPROCS.
	// This ensures optimal parallelism in containerized environments (Docker, Kubernetes) without manual configuration.
	p := pool.NewWithResults[*partitionQueryResult]().
		WithContext(ctx).
		WithMaxGoroutines(cmp.Or(int(session.systemVariables.MaxPartitionedParallelism), runtime.GOMAXPROCS(0)))

	for _, partition := range partitions {
		p.Go(func(ctx context.Context) (*partitionQueryResult, error) {
			iter := batchROTx.Execute(ctx, partition)
			rows, _, _, md, _, err := consumeRowIterCollect(iter, spannerRowToRow(fc))
			if err != nil {
				return nil, err
			}
			return &partitionQueryResult{md, rows}, nil
		})
	}

	results, err := p.Wait()
	if err != nil {
		return nil, err
	}

	var allRows []Row
	var rowType *sppb.StructType
	for _, result := range results {
		allRows = append(allRows, result.Rows...)

		if len(result.Metadata.GetRowType().GetFields()) > 0 {
			rowType = result.Metadata.GetRowType()
		}
	}

	result := &Result{
		Rows:           allRows,
		TableHeader:    toTableHeader(rowType.GetFields()),
		AffectedRows:   len(allRows),
		PartitionCount: len(partitions),
	}
	return result, nil
}

func executePDML(ctx context.Context, session *Session, sql string) (*Result, error) {
	stmt, err := newStatement(sql, session.systemVariables.Params, false)
	if err != nil {
		return nil, err
	}

	count, err := session.client.PartitionedUpdateWithOptions(ctx, stmt, spanner.QueryOptions{})
	if err != nil {
		return nil, err
	}

	return &Result{
		IsExecutedDML:    true, // This is a partitioned DML statement
		AffectedRows:     int(count),
		AffectedRowsType: rowCountTypeLowerBound,
	}, nil
}

// formatAsyncDdlResult formats the async DDL operation result in the same format as SHOW OPERATION
func formatAsyncDdlResult(op *adminapi.UpdateDatabaseDdlOperation) (*Result, error) {
	// Get the metadata from the operation
	metadata, err := op.Metadata()
	if err != nil {
		return nil, fmt.Errorf("failed to get operation metadata: %w", err)
	}

	operationId := lo.LastOrEmpty(strings.Split(op.Name(), "/"))

	// Use the same formatting logic as SHOW OPERATION statement
	// For async DDL, errors are reported when polling, not immediately available
	rows := formatUpdateDatabaseDdlRows(operationId, metadata, op.Done(), "")

	return &Result{
		TableHeader:  toTableHeader("OPERATION_ID", "STATEMENTS", "DONE", "PROGRESS", "COMMIT_TIMESTAMP", "ERROR"),
		Rows:         rows,
		AffectedRows: 1,
	}, nil
}
