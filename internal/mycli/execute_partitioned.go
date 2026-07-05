package mycli

import (
	"cmp"
	"context"
	"errors"
	"io"
	"iter"
	"runtime"
	"sync/atomic"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spaniter"
	"github.com/apstndb/spanner-mycli/internal/mycli/format"
	"github.com/apstndb/spanvalue"
	"github.com/apstndb/spanvalue/writer"
	"github.com/sourcegraph/conc/pool"
)

func runPartitionedQuery(ctx context.Context, session *Session, sql string) (*Result, error) {
	fc, vfm, sysVars, err := prepareFormatConfig(sql, session.systemVariables)
	if err != nil {
		return nil, err
	}

	stmt, err := newStatement(sql, sysVars.Params, false)
	if err != nil {
		return nil, err
	}

	partitions, batchROTx, err := session.txn.RunPartitionQuery(ctx, stmt)
	if err != nil {
		return nil, err
	}
	defer func() {
		batchROTx.Cleanup(ctx)
		batchROTx.Close()
	}()

	// Go 1.25+ automatically detects container CPU limits on Linux through container-aware GOMAXPROCS.
	// This ensures optimal parallelism in containerized environments (Docker, Kubernetes) without manual configuration.
	parallelism := cmp.Or(int(sysVars.Query.MaxPartitionedParallelism), runtime.GOMAXPROCS(0))

	// Formats backed by a spanvalue RowIteratorWriter (CSV, JSONL, SQL_INSERT*)
	// stream merged partition rows without buffering, like the query path.
	result, handled, err := streamPartitionedQuery(ctx, session.outputWriter(), batchROTx, partitions, parallelism, sysVars, fc, vfm)
	if err != nil {
		return nil, err
	}
	if handled {
		return result, nil
	}

	return bufferPartitionedQuery(ctx, batchROTx, partitions, parallelism, sysVars, fc, vfm)
}

// streamPartitionedQuery streams merged partition rows through the same
// spanvalue RowIteratorWriter as the query streaming path, via
// writer.RunRowSeq. Returns handled=false when the current format has no
// spanvalue writer (table and processor-based formats keep the buffered path).
func streamPartitionedQuery(
	ctx context.Context,
	out io.Writer,
	batchROTx *spanner.BatchReadOnlyTransaction,
	partitions []*spanner.Partition,
	parallelism int,
	sysVars *systemVariables,
	fc *spanvalue.FormatConfig,
	vfm format.ValueFormatMode,
) (*Result, bool, error) {
	w, handled, err := newSpanvalueRowIteratorWriterFor(out, sysVars, fc)
	if err != nil || !handled {
		return nil, handled, err
	}

	hooks := writer.RowIteratorHooksFromWriter(w)

	var runResult *writer.RowIteratorResult
	err = runPartitionedRowSeq(ctx, batchROTx, partitions, parallelism,
		func(md *sppb.ResultSetMetadata, rows iter.Seq2[*spanner.Row, error]) error {
			res, err := writer.RunRowSeq(md, rows, hooks)
			runResult = res
			return err
		})
	if err != nil {
		return nil, true, normalizeSpanvalueWriterError(err)
	}
	if runResult == nil || runResult.Metadata == nil {
		return nil, true, errors.New("partitioned query writer returned nil metadata")
	}

	return &Result{
		TableHeader:      toTableHeader(runResult.Metadata.GetRowType().GetFields()),
		AffectedRows:     runResult.RowsRead,
		PartitionCount:   len(partitions),
		Streamed:         true,
		SQLExportAllowed: vfm == format.SQLLiteralValues,
	}, true, nil
}

// bufferPartitionedQuery collects all partition rows into memory for formats
// that need the full result set (table width calculation) or have no
// streaming writer.
func bufferPartitionedQuery(
	ctx context.Context,
	batchROTx *spanner.BatchReadOnlyTransaction,
	partitions []*spanner.Partition,
	parallelism int,
	sysVars *systemVariables,
	fc *spanvalue.FormatConfig,
	vfm format.ValueFormatMode,
) (*Result, error) {
	transform := spannerRowToRow(fc, sysVars.typeStyles, sysVars.nullStyle)
	if vfm == format.JSONValues {
		transform = withRawJSONMarker(transform)
	}

	type partitionQueryResult struct {
		Metadata *sppb.ResultSetMetadata
		Rows     []Row
	}

	p := pool.NewWithResults[*partitionQueryResult]().
		WithContext(ctx).
		WithMaxGoroutines(parallelism)

	for _, partition := range partitions {
		p.Go(func(ctx context.Context) (*partitionQueryResult, error) {
			iter := batchROTx.Execute(ctx, partition)
			rows, _, _, md, _, err := consumeRowIterCollect(iter, transform)
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

	return &Result{
		Rows:           allRows,
		TableHeader:    toTableHeader(rowType.GetFields()),
		AffectedRows:   len(allRows),
		PartitionCount: len(partitions),
	}, nil
}

// partitionedRow is one fan-in item: a data row or a terminal partition error.
type partitionedRow struct {
	row *spanner.Row
	err error
}

// runPartitionedRowSeq executes all partitions with bounded concurrency and
// hands consume the row-type metadata plus a merged, fallible row sequence.
// The single consumer serializes output, so sinks need no locking. Metadata is
// captured from whichever partition responds first (every partition of one
// query shares the same row type) and is available before the first row is
// yielded; it is nil only when no partition reported metadata.
//
// consume aborting early (including on a yielded error) cancels the remaining
// partition work; partition errors are surfaced through the sequence itself.
func runPartitionedRowSeq(
	ctx context.Context,
	batchROTx *spanner.BatchReadOnlyTransaction,
	partitions []*spanner.Partition,
	parallelism int,
	consume func(md *sppb.ResultSetMetadata, rows iter.Seq2[*spanner.Row, error]) error,
) error {
	// Do not shadow the parent context: it must stay observable below to
	// distinguish external cancellation from our own consumer-driven cancel.
	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	ch := make(chan partitionedRow)
	var capturedMD atomic.Pointer[sppb.ResultSetMetadata]

	p := pool.New().WithContext(childCtx).WithMaxGoroutines(parallelism)
	for _, partition := range partitions {
		p.Go(func(workerCtx context.Context) error {
			rowIter := batchROTx.Execute(workerCtx, partition)
			var result spaniter.RowIteratorResult
			rows := spaniter.RowIteratorSeq(rowIter, spaniter.WithResult(&result))
			captureMetadata := func() {
				if capturedMD.Load() != nil {
					return
				}
				if rowIter.Metadata != nil {
					capturedMD.CompareAndSwap(nil, rowIter.Metadata)
				}
				if result.Metadata != nil {
					capturedMD.CompareAndSwap(nil, result.Metadata)
				}
			}
			for row, err := range rows {
				captureMetadata()
				if err != nil {
					select {
					case ch <- partitionedRow{err: err}:
					case <-workerCtx.Done():
					}
					return err
				}
				select {
				case ch <- partitionedRow{row: row}:
				case <-workerCtx.Done():
					return workerCtx.Err()
				}
			}
			captureMetadata()
			return nil
		})
	}

	producersDone := make(chan error, 1)
	go func() {
		producersDone <- p.Wait()
		close(ch)
	}()

	// Hold the first item back until metadata is known: its producer stored
	// metadata before sending, and a closed channel means all producers
	// finished (zero data rows) after storing whatever metadata they saw.
	first, hasFirst := <-ch

	rows := func(yield func(*spanner.Row, error) bool) {
		if hasFirst {
			if !yield(first.row, first.err) || first.err != nil {
				return
			}
		}
		for item := range ch {
			if !yield(item.row, item.err) || item.err != nil {
				return
			}
		}
	}

	consumeErr := consume(capturedMD.Load(), rows)

	// Release any producers blocked on send, then wait for them to finish.
	cancel()
	for range ch {
	}
	producerErr := <-producersDone

	if consumeErr != nil {
		return consumeErr
	}
	// A partition error is normally surfaced through the sequence and returned
	// by consume above; this covers errors raised after consume stopped reading.
	if producerErr != nil && !errors.Is(producerErr, context.Canceled) {
		return producerErr
	}
	// External cancellation (timeout, interrupt) makes producers exit with
	// context.Canceled and the sequence end early, which would otherwise be
	// indistinguishable from a successful run with fewer rows. Report it so a
	// truncated result is never treated as success.
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
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
