package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

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
	defer iter.Stop()

	rows, columnNames, err := parseQueryResult(iter)
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
	result := &Result{
		ColumnNames: columnNames,
		Rows:        rows,
	}
	result.ColumnTypes = iter.Metadata.GetRowType().GetFields()

	queryStats, err := parseQueryStats(iter.QueryStats)
	if err != nil {
		return nil, err
	}

	rowsReturned, err := strconv.Atoi(queryStats.RowsReturned)
	if err != nil {
		return nil, fmt.Errorf("rowsReturned is invalid: %v", err)
	}

	result.AffectedRows = rowsReturned
	result.Stats = queryStats

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

func executeAnalyze(ctx context.Context, session *Session, sql string, isDML bool) (*Result, error) {
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

	// consume iter
	err = iter.Do(func(*spanner.Row) error {
		return nil
	})
	if err != nil {
		return nil, err
	}

	queryStats, err := parseQueryStats(iter.QueryStats)
	if err != nil {
		return nil, err
	}

	rowsReturned, err := strconv.Atoi(queryStats.RowsReturned)
	if err != nil {
		return nil, fmt.Errorf("rowsReturned is invalid: %v", err)
	}

	// Cloud Spanner Emulator doesn't set query plan nodes to the result.
	// See: https://github.com/GoogleCloudPlatform/cloud-spanner-emulator/blob/77188b228e7757cd56ecffb5bc3ee85dce5d6ae1/frontend/handlers/queries.cc#L224-L230
	if iter.QueryPlan == nil {
		return nil, errors.New("query plan is not available. EXPLAIN ANALYZE statement is not supported for Cloud Spanner Emulator.")
	}

	rows, predicates, err := processPlanWithStats(iter.QueryPlan)
	if err != nil {
		return nil, err
	}

	// ReadOnlyTransaction.Timestamp() is invalid until read.
	var timestamp time.Time
	if roTxn != nil {
		timestamp, _ = roTxn.Timestamp()
	}

	result := &Result{
		ColumnNames:  explainAnalyzeColumnNames,
		ForceVerbose: true,
		AffectedRows: rowsReturned,
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
		defer iter.Stop()
		err := iter.Do(func(r *spanner.Row) error { return nil })
		if err != nil {
			return 0, nil, nil, err
		}
		return iter.RowCount, iter.QueryPlan, iter.Metadata, nil
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
