package mycli

import (
	"cmp"
	"context"
	"runtime"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/internal/mycli/decoder"
	"github.com/sourcegraph/conc/pool"
)

func runPartitionedQuery(ctx context.Context, session *Session, sql string) (*Result, error) {
	fc, err := decoder.FormatConfigWithProto(session.systemVariables.Internal.ProtoDescriptor, session.systemVariables.Display.MultilineProtoText)
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
		WithMaxGoroutines(cmp.Or(int(session.systemVariables.Query.MaxPartitionedParallelism), runtime.GOMAXPROCS(0)))

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
