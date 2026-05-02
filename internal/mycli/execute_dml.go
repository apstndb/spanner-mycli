package mycli

import (
	"context"
	"errors"
	"slices"
	"strconv"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/gsqlutils"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/samber/lo"
	loi "github.com/samber/lo/it"
)

func isInsert(sql string) bool {
	token, err := gsqlutils.FirstNonHintToken("", sql)
	if err != nil {
		return false
	}

	return token.IsKeywordLike("INSERT")
}

func bufferOrExecuteDML(ctx context.Context, session *Session, sql string) (*Result, error) {
	switch b := session.batch.Current().(type) {
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

		if inReadWriteTransaction && session.systemVariables.Transaction.AutoBatchDML {
			stmt, err := newStatement(sql, session.systemVariables.Params, false)
			if err != nil {
				return nil, err
			}
			session.batch.SetCurrent(&BatchDMLStatement{DMLs: []spanner.Statement{stmt}})
			return &Result{}, nil
		}

		if !inTransaction &&
			!isInsert(sql) &&
			session.systemVariables.Transaction.AutocommitDMLMode == enums.AutocommitDMLModePartitionedNonAtomic {
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
		Rows: slices.Collect(loi.ZipBy2(
			slices.Values(dmls),
			slices.Values(affectedRowSlice),
			func(s spanner.Statement, n int64) Row {
				return toRow(s.SQL, strconv.FormatInt(n, 10))
			},
		)),
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

	session.systemVariables.Internal.LastQueryCache = &LastQueryCache{
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
