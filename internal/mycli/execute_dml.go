package mycli

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strconv"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/gsqlutils"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanner-mycli/internal/mycli/iterutil"
	"github.com/samber/lo"
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
		inTransaction, inReadWriteTransaction := session.txn.GetTransactionFlagsWithLock()

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
	result, err := session.txn.RunInNewOrExistRwTx(ctx, func(tx *spanner.ReadWriteStmtBasedTransaction, implicit bool) (affected int64, plan *sppb.QueryPlan, metadata *sppb.ResultSetMetadata, err error) {
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
		Rows: slices.Collect(iterutil.ZipShortestBy(slices.Values(dmls), slices.Values(affectedRowSlice), func(s spanner.Statement, affectedRows int64) Row {
			return toRow(s.SQL, strconv.FormatInt(affectedRows, 10))
		})),
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

	var renderedOutput []byte
	var hasRenderedOutput bool
	var queryStats map[string]any
	var tableHeader TableHeader
	result, err := session.txn.RunInNewOrExistRwTx(ctx, func(tx *spanner.ReadWriteStmtBasedTransaction, implicit bool) (affected int64, plan *sppb.QueryPlan, metadata *sppb.ResultSetMetadata, err error) {
		updateResult, err := session.txn.runUpdateOnTransaction(ctx, tx, stmt, implicit)
		if err != nil {
			return 0, nil, nil, err
		}
		queryStats = updateResult.Stats
		tableHeader = toTableHeader(updateResult.Metadata.GetRowType().GetFields())
		if tableHeader != nil {
			// Render inside the transaction callback so a formatting error
			// aborts the implicit commit instead of committing without output.
			renderedOutput, err = renderDMLReturnedRows(session.systemVariables, tableHeader, updateResult.Metadata, updateResult.Rows)
			if err != nil {
				return 0, nil, nil, err
			}
			hasRenderedOutput = true
		}
		return updateResult.Count, updateResult.Plan, updateResult.Metadata, nil
	})
	if err != nil {
		return nil, err
	}

	stats, err := parseQueryStats(queryStats)
	if err != nil {
		return nil, err
	}

	session.systemVariables.LastResult.QueryCache = &LastQueryCache{
		QueryPlan:       result.Plan,
		QueryStats:      queryStats,
		CommitTimestamp: result.CommitResponse.CommitTs,
	}

	return buildDMLResult(result, stats, tableHeader, renderedOutput, hasRenderedOutput, session.systemVariables)
}

// buildDMLResult assembles the final Result for a regular (non-batch) DML
// statement from its DMLResult and pre-rendered THEN RETURN output.
//
// It applies the same CLI_QUERY_MODE-driven presentation rules as SELECT
// results (see applyQueryModeStatsRendering): WITH_STATS forces stats to
// render even without CLI_VERBOSE, and WITH_PLAN_AND_STATS additionally
// appends the query plan collected by runUpdateOnTransaction as a result
// appendix, when a plan is available.
func buildDMLResult(dmlResult *DMLResult, stats QueryStats, tableHeader TableHeader, renderedOutput []byte, hasRenderedOutput bool, sysVars *systemVariables) (*Result, error) {
	result := &Result{
		IsExecutedDML:     true, // This is a regular DML statement
		CommitTimestamp:   dmlResult.CommitResponse.CommitTs,
		CommitStats:       dmlResult.CommitResponse.CommitStats,
		Stats:             stats,
		TableHeader:       tableHeader,
		RenderedOutput:    renderedOutput,
		HasRenderedOutput: hasRenderedOutput,
		AffectedRows:      int(dmlResult.Affected),
		SQLExportAllowed:  false, // DML with THEN RETURN uses regular formatting, not SQL literals
	}

	if err := applyQueryModeStatsRendering(result, dmlResult.Plan, sysVars); err != nil {
		return nil, err
	}

	return result, nil
}

// renderDMLReturnedRows renders THEN RETURN rows to bytes at display time from
// the raw typed rows, so value types are preserved for the active CLI_FORMAT
// (issue #738 PR2). Previously the rows were converted to display-text cells
// inside the transaction with the display FormatConfig, which under
// CLI_FORMAT=JSONL emitted type-unfaithful output (e.g. {"n":"1"} instead of
// {"n":1}). The temp Result is kind (c) (Typed) with SQLExportAllowed=false,
// preserving the rule that THEN RETURN falls back to a table under SQL export.
//
// It is called inside the DML transaction callback so a render failure aborts
// the implicit commit rather than committing without output.
func renderDMLReturnedRows(sysVars *systemVariables, tableHeader TableHeader, metadata *sppb.ResultSetMetadata, rows []*spanner.Row) ([]byte, error) {
	var buf bytes.Buffer
	result := &Result{
		TableHeader: tableHeader,
		Typed: &TypedRows{
			Metadata:         metadata,
			Rows:             rows,
			SQLExportAllowed: false,
		},
	}
	if err := printTableData(sysVars, displayScreenWidth(sysVars), &buf, result); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
