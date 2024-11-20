package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/ngicks/go-iterator-helper/x/exp/xiter"

	"github.com/apstndb/lox"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"cloud.google.com/go/spanner/apiv1/spannerpb"
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
		time.Sleep(3 * time.Second)
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
		AffectedRows: len(rows),
		Rows:         rows,
		Timestamp:    timestamp,
		Predicates:   predicates,
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
	timestamp := lox.IfOrEmptyF(roTxn != nil, func() time.Time {
		ts, _ := roTxn.Timestamp()
		return ts
	})

	result := &Result{
		ColumnNames:  explainAnalyzeColumnNames,
		ForceVerbose: true,
		AffectedRows: len(rows),
		Stats:        queryStats,
		Timestamp:    timestamp,
		Rows:         rows,
		Predicates:   predicates,
	}
	return result, nil
}

func executeDML(ctx context.Context, session *Session, sql string) (*Result, error) {
	stmt, err := newStatement(sql, session.systemVariables.Params, false)
	if err != nil {
		return nil, err
	}

	result := &Result{IsMutation: true}

	var rows []Row
	var columnNames []string
	var numRows int64
	var metadata *spannerpb.ResultSetMetadata
	if session.InReadWriteTransaction() {
		rows, columnNames, numRows, metadata, err = session.RunUpdate(ctx, stmt, false)
		if err != nil {
			// Need to call rollback to free the acquired session in underlying google-cloud-go/spanner.
			rollback := &RollbackStatement{}
			if _, rollbackErr := rollback.Execute(ctx, session); rollbackErr != nil {
				return nil, errors.Join(err, fmt.Errorf("error on rollback: %w", rollbackErr))
			}
			return nil, fmt.Errorf("transaction was aborted: %v", err)
		}
	} else {
		// Start implicit transaction.
		begin := BeginRwStatement{}
		if _, err = begin.Execute(ctx, session); err != nil {
			return nil, err
		}

		rows, columnNames, numRows, metadata, err = session.RunUpdate(ctx, stmt, false)
		if err != nil {
			// once error has happened, escape from implicit transaction
			rollback := &RollbackStatement{}
			if _, rollbackErr := rollback.Execute(ctx, session); rollbackErr != nil {
				return nil, errors.Join(err, fmt.Errorf("error on rollback: %w", rollbackErr))
			}
			return nil, err
		}

		commit := CommitStatement{}
		txnResult, err := commit.Execute(ctx, session)
		if err != nil {
			return nil, err
		}
		result.Timestamp = txnResult.Timestamp
		result.CommitStats = txnResult.CommitStats
	}

	result.ColumnTypes = metadata.GetRowType().GetFields()
	result.Rows = rows
	result.ColumnNames = columnNames
	result.AffectedRows = int(numRows)

	return result, nil
}

func executeExplainAnalyzeDML(ctx context.Context, session *Session, sql string) (*Result, error) {
	stmt, err := newStatement(sql, session.systemVariables.Params, false)
	if err != nil {
		return nil, err
	}

	affectedRows, timestamp, queryPlan, _, err := runInNewOrExistRwTxForExplain(ctx, session, func() (int64, *spannerpb.QueryPlan, *spannerpb.ResultSetMetadata, error) {
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
		Timestamp:        timestamp,
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

func runAnalyzeQuery(ctx context.Context, session *Session, stmt spanner.Statement, isDML bool) (queryPlan *spannerpb.QueryPlan, commitTimestamp time.Time, metadata *spannerpb.ResultSetMetadata, err error) {
	if !isDML {
		queryPlan, metadata, err := session.RunAnalyzeQuery(ctx, stmt)
		return queryPlan, time.Time{}, metadata, err
	}

	_, timestamp, queryPlan, metadata, err := runInNewOrExistRwTxForExplain(ctx, session, func() (int64, *spannerpb.QueryPlan, *spannerpb.ResultSetMetadata, error) {
		plan, metadata, err := session.RunAnalyzeQuery(ctx, stmt)
		return 0, plan, metadata, err
	})
	return queryPlan, timestamp, metadata, err
}

// runInNewOrExistRwTxForExplain is a helper function for runAnalyzeQuery and ExplainAnalyzeDmlStatement.
// It execute a function in the current RW transaction or an implicit RW transaction.
func runInNewOrExistRwTxForExplain(ctx context.Context, session *Session, f func() (affected int64, plan *spannerpb.QueryPlan, metadata *spannerpb.ResultSetMetadata, err error)) (affected int64, ts time.Time, plan *spannerpb.QueryPlan, metadata *spannerpb.ResultSetMetadata, err error) {
	var implicitRWTx bool
	if !session.InReadWriteTransaction() {
		implicitRWTx = true
		// Start implicit transaction.
		begin := BeginRwStatement{}
		if _, err := begin.Execute(ctx, session); err != nil {
			return 0, time.Time{}, nil, nil, err
		}
	}

	affected, plan, metadata, err = f()
	if err != nil {
		// once error has happened, escape from implicit transaction
		rollback := &RollbackStatement{}
		if _, rollbackErr := rollback.Execute(ctx, session); rollbackErr != nil {
			err = errors.Join(err, fmt.Errorf("error on rollback: %w", rollbackErr))
		}
		return 0, time.Time{}, nil, nil, fmt.Errorf("transaction was aborted: %w", err)
	}

	var commitTimestamp time.Time
	if implicitRWTx {
		// query mode PLAN doesn't have any side effects, but use commit to get commit timestamp.
		commit := CommitStatement{}
		txnResult, err := commit.Execute(ctx, session)
		if err != nil {
			return 0, time.Time{}, nil, nil, err
		}
		commitTimestamp = txnResult.Timestamp
	}

	return affected, commitTimestamp, plan, metadata, nil
}

// extractColumnNames extract column names from ResultSetMetadata.RowType.Fields.
func extractColumnNames(fields []*spannerpb.StructType_Field) []string {
	return slices.Collect(xiter.Map((*spannerpb.StructType_Field).GetName, slices.Values(fields)))
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
func consumeRowIterDiscard(iter *spanner.RowIterator) (queryStats map[string]interface{}, rowCount int64, metadata *spannerpb.ResultSetMetadata, queryPlan *spannerpb.QueryPlan, err error) {
	return consumeRowIter(iter, func(*spanner.Row) error { return nil })
}

// consumeRowIter calls iter.Stop().
func consumeRowIter(iter *spanner.RowIterator, f func(*spanner.Row) error) (queryStats map[string]interface{}, rowCount int64, metadata *spannerpb.ResultSetMetadata, queryPlan *spannerpb.QueryPlan, err error) {
	defer iter.Stop()
	err = iter.Do(f)
	if err != nil {
		return nil, 0, nil, nil, err
	}

	return iter.QueryStats, iter.RowCount, iter.Metadata, iter.QueryPlan, nil
}

func consumeRowIterCollect[T any](iter *spanner.RowIterator, f func(*spanner.Row) (T, error)) (rows []T, queryStats map[string]interface{}, rowCount int64, metadata *spannerpb.ResultSetMetadata, queryPlan *spannerpb.QueryPlan, err error) {
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
