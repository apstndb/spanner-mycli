package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/ngicks/go-iterator-helper/x/exp/xiter"

	"github.com/apstndb/lox"
	"github.com/go-json-experiment/json"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/mattn/go-runewidth"
	"github.com/samber/lo"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
)

func executeSQL(ctx context.Context, session *Session, sql string) (*Result, error) {
	params := session.systemVariables.Params
	stmt, err := newStatement(sql, params, false)
	if err != nil {
		return nil, err
	}

	iter, roTxn := session.RunQueryWithStats(ctx, stmt)

	rows, stats, _, metadata, _, err := consumeRowIterCollect(iter, spannerRowToRow)
	if err != nil {
		if session.InReadWriteTransaction() && spanner.ErrCode(err) == codes.Aborted {
			// Need to call rollback to free the acquired session in underlying google-cloud-go/spanner.
			rollback := &RollbackStatement{}
			if _, rollbackErr := rollback.Execute(ctx, session); err != nil {
				return nil, errors.Join(err, fmt.Errorf("error on rollback: %w", rollbackErr))
			}
		}
		return nil, err
	}

	queryStats, err := parseQueryStats(stats)
	if err != nil {
		return nil, err
	}

	result := &Result{
		ColumnNames:  extractColumnNames(metadata.GetRowType().GetFields()),
		Rows:         rows,
		ColumnTypes:  metadata.GetRowType().GetFields(),
		AffectedRows: len(rows),
		Stats:        queryStats,
	}

	// ReadOnlyTransaction.Timestamp() is invalid until read.
	if roTxn != nil {
		result.Timestamp, _ = roTxn.Timestamp()
	}

	return result, nil
}

func executeDdlStatements(ctx context.Context, session *Session, ddls []string) (*Result, error) {
	logParseStatements(ddls)

	b, err := proto.Marshal(session.systemVariables.ProtoDescriptor)
	if err != nil {
		return nil, err
	}

	p := mpb.NewWithContext(ctx)
	defer p.Shutdown()
	var bars []*mpb.Bar
	for _, ddl := range ddls {
		bar := p.AddBar(int64(100), mpb.PrependDecorators(decor.Spinner(nil, decor.WCSyncSpaceR), decor.Name(runewidth.Truncate(ddl, 40, "..."), decor.WCSyncSpaceR), decor.Percentage(decor.WCSyncSpace), decor.Elapsed(decor.ET_STYLE_MMSS, decor.WCSyncSpace)))
		bars = append(bars, bar)
	}

	op, err := session.adminClient.UpdateDatabaseDdl(ctx, &databasepb.UpdateDatabaseDdlRequest{
		Database:         session.DatabasePath(),
		Statements:       ddls,
		ProtoDescriptors: b,
	})
	if err != nil {
		return nil, fmt.Errorf("error on create op: %w", err)
	}

	for {
		time.Sleep(5 * time.Second)
		err := op.Poll(ctx)
		if err != nil {
			return nil, err
		}

		metadata, err := op.Metadata()
		if err != nil {
			return nil, err
		}

		progresses := metadata.GetProgress()
		for i, progress := range progresses {
			bar := bars[i]
			progressPercent := int64(progress.ProgressPercent)
			bar.SetCurrent(progressPercent)
		}

		if op.Done() {
			lastCommitTS := lo.LastOrEmpty(metadata.CommitTimestamps).AsTime()
			return &Result{IsMutation: true, Timestamp: lastCommitTS}, nil
		}
	}
}

func executeExplain(ctx context.Context, session *Session, sql string, isDML bool) (*Result, error) {
	stmt, err := newStatement(sql, session.systemVariables.Params, true)
	if err != nil {
		return nil, err
	}

	queryPlan, timestamp, _, err := runAnalyzeQuery(ctx, session, stmt, isDML)
	if err != nil {
		return nil, err
	}

	if queryPlan == nil {
		return nil, errors.New("EXPLAIN statement is not supported for Cloud Spanner Emulator.")
	}

	rows, predicates, err := processPlanWithoutStats(queryPlan)
	if err != nil {
		return nil, err
	}

	result := &Result{
		ColumnNames:  explainColumnNames,
		ColumnAlign:  explainColumnAlign,
		AffectedRows: len(rows),
		Rows:         rows,
		Timestamp:    timestamp,
		Predicates:   predicates,
		LintResults:  lox.IfOrEmptyF(session.systemVariables.LintPlan, func() []string { return lintPlan(queryPlan) }),
	}

	return result, nil
}

func executeExplainAnalyze(ctx context.Context, session *Session, sql string) (*Result, error) {
	stmt, err := newStatement(sql, session.systemVariables.Params, false)
	if err != nil {
		return nil, err
	}

	iter, roTxn := session.RunQueryWithStats(ctx, stmt)

	stats, _, _, plan, err := consumeRowIterDiscard(iter)
	if err != nil {
		return nil, err
	}

	queryStats, err := parseQueryStats(stats)
	if err != nil {
		return nil, err
	}

	// Cloud Spanner Emulator doesn't set query plan nodes to the result.
	// See: https://github.com/GoogleCloudPlatform/cloud-spanner-emulator/blob/77188b228e7757cd56ecffb5bc3ee85dce5d6ae1/frontend/handlers/queries.cc#L224-L230
	if plan == nil {
		return nil, errors.New("query plan is not available. EXPLAIN ANALYZE statement is not supported for Cloud Spanner Emulator.")
	}

	rows, predicates, err := processPlanWithStats(plan)
	if err != nil {
		return nil, err
	}

	// ReadOnlyTransaction.Timestamp() is invalid until read.
	result := &Result{
		ColumnNames:  explainAnalyzeColumnNames,
		ColumnAlign:  explainAnalyzeColumnAlign,
		ForceVerbose: true,
		AffectedRows: len(rows),
		Stats:        queryStats,
		Timestamp:    lox.IfOrEmptyF(roTxn != nil, func() time.Time { return ignoreError(roTxn.Timestamp()) }),
		Rows:         rows,
		Predicates:   predicates,
		LintResults:  lox.IfOrEmptyF(session.systemVariables.LintPlan, func() []string { return lintPlan(plan) }),
	}
	return result, nil
}

func executeDML(ctx context.Context, session *Session, sql string) (*Result, error) {
	stmt, err := newStatement(sql, session.systemVariables.Params, false)
	if err != nil {
		return nil, err
	}

	var rows []Row
	var columnNames []string
	affected, commitResp, _, metadata, err := session.RunInNewOrExistRwTx(ctx, func() (affected int64, plan *sppb.QueryPlan, metadata *sppb.ResultSetMetadata, err error) {
		rs, columns, num, meta, err := session.RunUpdate(ctx, stmt)
		rows = rs
		columnNames = columns
		return num, nil, meta, err
	})
	if err != nil {
		return nil, err
	}

	return &Result{
		IsMutation:   true,
		Timestamp:    commitResp.CommitTs,
		CommitStats:  commitResp.CommitStats,
		ColumnTypes:  metadata.GetRowType().GetFields(),
		Rows:         rows,
		ColumnNames:  columnNames,
		AffectedRows: int(affected),
	}, nil
}

func executeExplainAnalyzeDML(ctx context.Context, session *Session, sql string) (*Result, error) {
	stmt, err := newStatement(sql, session.systemVariables.Params, false)
	if err != nil {
		return nil, err
	}

	affectedRows, commitResp, queryPlan, _, err := session.RunInNewOrExistRwTx(ctx, func() (int64, *sppb.QueryPlan, *sppb.ResultSetMetadata, error) {
		iter, _ := session.RunQueryWithStats(ctx, stmt)
		_, count, metadata, plan, err := consumeRowIterDiscard(iter)
		return count, plan, metadata, err
	})
	if err != nil {
		return nil, err
	}

	rows, predicates, err := processPlanWithStats(queryPlan)
	if err != nil {
		return nil, err
	}

	result := &Result{
		IsMutation:       true,
		ColumnNames:      explainAnalyzeColumnNames,
		ForceVerbose:     true,
		AffectedRows:     int(affectedRows),
		AffectedRowsType: rowCountTypeExact,
		Rows:             rows,
		Predicates:       predicates,
		Timestamp:        commitResp.CommitTs,
		LintResults:      lox.IfOrEmptyF(session.systemVariables.LintPlan, func() []string { return lintPlan(queryPlan) }),
	}

	return result, nil
}

func executeInformationSchemaBasedStatement(ctx context.Context, session *Session, stmtName string, stmt spanner.Statement, emptyErrorF func() error) (*Result, error) {
	if session.InReadWriteTransaction() {
		// INFORMATION_SCHEMA can't be used in read-write transaction.
		// https://cloud.google.com/spanner/docs/information-schema
		return nil, fmt.Errorf(`%q can not be used in a read-write transaction`, stmtName)
	}

	iter, _ := session.RunQuery(ctx, stmt)
	rows, _, _, metadata, _, err := consumeRowIterCollect(iter, spannerRowToRow)
	if err != nil {
		return nil, err
	}

	if len(rows) == 0 && emptyErrorF != nil {
		return nil, emptyErrorF()
	}

	return &Result{
		ColumnNames:  extractColumnNames(metadata.GetRowType().GetFields()),
		Rows:         rows,
		AffectedRows: len(rows),
	}, nil
}

func runAnalyzeQuery(ctx context.Context, session *Session, stmt spanner.Statement, isDML bool) (queryPlan *sppb.QueryPlan, commitTimestamp time.Time, metadata *sppb.ResultSetMetadata, err error) {
	if !isDML {
		queryPlan, metadata, err := session.RunAnalyzeQuery(ctx, stmt)
		return queryPlan, time.Time{}, metadata, err
	}

	_, commitResp, queryPlan, metadata, err := session.RunInNewOrExistRwTx(ctx, func() (int64, *sppb.QueryPlan, *sppb.ResultSetMetadata, error) {
		plan, metadata, err := session.RunAnalyzeQuery(ctx, stmt)
		return 0, plan, metadata, err
	})
	return queryPlan, commitResp.CommitTs, metadata, err
}

// extractColumnNames extract column names from ResultSetMetadata.RowType.Fields.
func extractColumnNames(fields []*sppb.StructType_Field) []string {
	return slices.Collect(xiter.Map((*sppb.StructType_Field).GetName, slices.Values(fields)))
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

func spannerRowToRow(row *spanner.Row) (Row, error) {
	columns, err := DecodeRow(row)
	if err != nil {
		return Row{}, err
	}
	return toRow(columns...), nil
}
