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
	"github.com/cloudspannerecosystem/memefish/ast"
	"github.com/cloudspannerecosystem/memefish/token"
	"github.com/samber/lo"
)

var errReturningDMLNotSupportedInBatch = errors.New("THEN RETURN is not supported in batch DML")

func isInsert(sql string) bool {
	token, err := gsqlutils.FirstNonHintToken("", sql)
	if err != nil {
		return false
	}

	return token.IsKeywordLike("INSERT")
}

// autocommitUsesPartitionedDML reports whether autocommit DML sql would be
// executed as partitioned DML. Pending transactions count as in-transaction,
// matching GetTransactionFlagsWithLock in bufferOrExecuteDML.
func autocommitUsesPartitionedDML(session *Session, sql string) bool {
	if session == nil || session.txn == nil || session.systemVariables == nil {
		return false
	}
	inTransaction, _ := session.txn.GetTransactionFlagsWithLock()
	return !inTransaction &&
		!isInsert(sql) &&
		session.systemVariables.Transaction.AutocommitDMLMode == enums.AutocommitDMLModePartitionedNonAtomic
}

func bufferOrExecuteDML(ctx context.Context, session *Session, sql string) (*Result, error) {
	switch b := session.batch.Current().(type) {
	case *BatchDMLStatement:
		if dmlHasReturningClause(sql) {
			return nil, errReturningDMLNotSupportedInBatch
		}
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
		hasReturning := dmlHasReturningClause(sql)
		if session.systemVariables.Transaction.AutoBatchDML && !hasReturning {
			stmt, err := newStatement(sql, session.systemVariables.Params, false)
			if err != nil {
				return nil, err
			}
			enqueued, err := session.txn.TryEnqueueAutomaticDML(stmt)
			if err != nil {
				return nil, err
			}
			if enqueued {
				return &Result{}, nil
			}
		}

		if _, err := session.txn.FlushAutomaticDML(ctx); err != nil {
			return nil, err
		}

		if autocommitUsesPartitionedDML(session, sql) {
			return executePDML(ctx, session, sql)
		}

		return executeDML(ctx, session, sql)
	}
}

// dmlHasReturningClause reports whether sql includes a THEN RETURN clause.
// The AST path ignores comments and string literals. If memefish cannot parse
// the statement, a token scan is used; lexer errors fail closed toward the
// row-producing executeDML path so automatic BatchUpdate cannot drop rows.
func dmlHasReturningClause(sql string) bool {
	stmt, err := parseMemefishStatement("", sql)
	if err == nil {
		switch s := stmt.(type) {
		case *ast.Insert:
			return s.ThenReturn != nil
		case *ast.Delete:
			return s.ThenReturn != nil
		case *ast.Update:
			return s.ThenReturn != nil
		default:
			return false
		}
	}
	return dmlHasReturningClauseLexical(sql)
}

func dmlHasReturningClauseLexical(sql string) bool {
	sawThen := false
	for tok, err := range gsqlutils.NewLexerSeq("", sql) {
		if err != nil {
			return true
		}
		if sawThen && tokenKeywordLike(tok, "RETURN") {
			return true
		}
		sawThen = tokenKeywordLike(tok, "THEN")
	}
	return false
}

func tokenKeywordLike(tok token.Token, keyword string) bool {
	if tok.IsKeywordLike(keyword) {
		return true
	}
	return tok.Kind == token.TokenKind(keyword)
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

	return newBatchDMLResult(dmls, affectedRowSlice, result), nil
}

func newBatchDMLResult(dmls []spanner.Statement, affectedRowSlice []int64, result *DMLResult) *Result {
	var commit spanner.CommitResponse
	if result != nil {
		commit = result.CommitResponse
	}
	return &Result{
		IsExecutedDML:   true, // This is a batch DML statement
		CommitTimestamp: commit.CommitTs,
		CommitStats:     commit.CommitStats,
		Rows: slices.Collect(iterutil.ZipShortestBy(slices.Values(dmls), slices.Values(affectedRowSlice), func(s spanner.Statement, affectedRows int64) Row {
			return toRow(s.SQL, strconv.FormatInt(affectedRows, 10))
		})),
		TableHeader:      toTableHeader("DML", "Rows"),
		AffectedRows:     int(lo.Sum(affectedRowSlice)),
		AffectedRowsType: lo.Ternary(len(dmls) > 1, rowCountTypeUpperBound, rowCountTypeExact),
	}
}

func executeDML(ctx context.Context, session *Session, sql string) (*Result, error) {
	stmt, err := newStatement(sql, session.systemVariables.Params, false)
	if err != nil {
		return nil, err
	}

	var renderedOutput []byte
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
			// renderedOutput may be empty (e.g. JSONL with zero returned rows);
			// printResult re-derives the same empty body when it is nil, so a
			// separate "has rendered output" flag is unnecessary.
			renderedOutput, err = renderDMLReturnedRows(session.systemVariables, tableHeader, updateResult.Metadata, updateResult.Rows)
			if err != nil {
				return 0, nil, nil, err
			}
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

	return buildDMLResult(result, stats, tableHeader, renderedOutput, session.systemVariables)
}

// buildDMLResult assembles the final Result for a regular (non-batch) DML
// statement from its DMLResult and pre-rendered THEN RETURN output.
//
// It applies the same CLI_QUERY_MODE-driven presentation rules as SELECT
// results (see applyQueryModeStatsRendering): WITH_STATS forces stats to
// render even without CLI_VERBOSE, and WITH_PLAN_AND_STATS additionally
// appends the query plan collected by runUpdateOnTransaction as a result
// appendix, when a plan is available.
func buildDMLResult(dmlResult *DMLResult, stats QueryStats, tableHeader TableHeader, renderedOutput []byte, sysVars *systemVariables) (*Result, error) {
	result := &Result{
		IsExecutedDML:    true, // This is a regular DML statement
		CommitTimestamp:  dmlResult.CommitResponse.CommitTs,
		CommitStats:      dmlResult.CommitResponse.CommitStats,
		Stats:            stats,
		TableHeader:      tableHeader,
		RenderedOutput:   renderedOutput,
		AffectedRows:     int(dmlResult.Affected),
		SQLExportAllowed: false, // DML with THEN RETURN uses regular formatting, not SQL literals
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
