package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/apstndb/gsqlutils"
	"github.com/apstndb/spanvalue"
	"github.com/ngicks/go-iterator-helper/hiter"
	"github.com/ngicks/go-iterator-helper/x/exp/xiter"
	"github.com/sourcegraph/conc/pool"
	"google.golang.org/protobuf/types/known/timestamppb"

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
	fc, err := formatConfigWithProto(session.systemVariables.ProtoDescriptor, session.systemVariables.MultilineProtoText)
	if err != nil {
		return nil, err
	}

	params := session.systemVariables.Params
	stmt, err := newStatement(sql, params, false)
	if err != nil {
		return nil, err
	}

	iter, roTxn := session.RunQueryWithStats(ctx, stmt, false)

	rows, stats, _, metadata, _, err := consumeRowIterCollect(iter, spannerRowToRow(fc))
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
		Rows:         rows,
		TableHeader:  toTableHeader(metadata.GetRowType().GetFields()),
		AffectedRows: len(rows),
		Stats:        queryStats,
	}

	// ReadOnlyTransaction.Timestamp() is invalid until read.
	if roTxn != nil {
		result.Timestamp, _ = roTxn.Timestamp()
	}

	return result, nil
}

func bufferOrExecuteDdlStatements(ctx context.Context, session *Session, ddls []string) (*Result, error) {
	switch b := session.currentBatch.(type) {
	case *BatchDMLStatement:
		return nil, errors.New("there is active batch DML")
	case *BulkDdlStatement:
		b.Ddls = append(b.Ddls, ddls...)
		return &Result{IsMutation: true}, nil
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
			IsMutation:  true,
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
	result := &Result{IsMutation: true, Timestamp: lastCommitTS}
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
		return &Result{IsMutation: true}, nil
	case *BulkDdlStatement:
		return nil, errors.New("there is active batch DDL")
	default:
		if session.InReadWriteTransaction() && session.systemVariables.AutoBatchDML {
			stmt, err := newStatement(sql, session.systemVariables.Params, false)
			if err != nil {
				return nil, err
			}
			session.currentBatch = &BatchDMLStatement{DMLs: []spanner.Statement{stmt}}
			return &Result{IsMutation: true}, nil
		}

		if !session.InTransaction() &&
			!isInsert(sql) &&
			session.systemVariables.AutocommitDMLMode == AutocommitDMLModePartitionedNonAtomic {
			return executePDML(ctx, session, sql)
		}

		return executeDML(ctx, session, sql)
	}
}

func executeBatchDML(ctx context.Context, session *Session, dmls []spanner.Statement) (*Result, error) {
	var affectedRowSlice []int64
	affected, commitResp, _, _, err := session.RunInNewOrExistRwTx(ctx, func(implicit bool) (affected int64, plan *sppb.QueryPlan, metadata *sppb.ResultSetMetadata, err error) {
		affectedRowSlice, err = session.tc.RWTxn().BatchUpdateWithOptions(ctx, dmls, spanner.QueryOptions{LastStatement: implicit})
		return lo.Sum(affectedRowSlice), nil, nil, err
	})
	if err != nil {
		return nil, err
	}

	return &Result{
		IsMutation:  true,
		Timestamp:   commitResp.CommitTs,
		CommitStats: commitResp.CommitStats,
		Rows: slices.Collect(hiter.Unify(
			func(s spanner.Statement, n int64) Row {
				return toRow(s.SQL, strconv.FormatInt(n, 10))
			},
			hiter.Pairs(slices.Values(dmls), slices.Values(affectedRowSlice)))),
		TableHeader:      toTableHeader("DML", "Rows"),
		AffectedRows:     int(affected),
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
	affected, commitResp, _, metadata, err := session.RunInNewOrExistRwTx(ctx, func(implicit bool) (affected int64, plan *sppb.QueryPlan, metadata *sppb.ResultSetMetadata, err error) {
		rs, stats, _, num, meta, err := session.RunUpdate(ctx, stmt, implicit)
		rows = rs
		queryStats = stats
		return num, nil, meta, err
	})
	if err != nil {
		return nil, err
	}

	stats, err := parseQueryStats(queryStats)
	if err != nil {
		return nil, err
	}

	return &Result{
		IsMutation:   true,
		Timestamp:    commitResp.CommitTs,
		CommitStats:  commitResp.CommitStats,
		Stats:        stats,
		TableHeader:  toTableHeader(metadata.GetRowType().GetFields()),
		Rows:         rows,
		AffectedRows: int(affected),
	}, nil
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

	ctx, cancel := context.WithTimeout(ctx, pdmlTimeout)
	defer cancel()

	count, err := session.client.PartitionedUpdateWithOptions(ctx, stmt, spanner.QueryOptions{})
	if err != nil {
		return nil, err
	}

	return &Result{
		IsMutation:       true,
		AffectedRows:     int(count),
		AffectedRowsType: rowCountTypeLowerBound,
	}, nil
}
