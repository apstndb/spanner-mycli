// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mycli

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/enums"
)

func TestOperationReceiptErrorCannotSucceed(t *testing.T) {
	t.Parallel()
	rec := &operationReceipt{}
	row, err := spanner.NewRow([]string{"v"}, []any{int64(1)})
	if err != nil {
		t.Fatal(err)
	}
	if err := rec.ObserveRow(row); err != nil {
		t.Fatal(err)
	}
	injected := errors.New("formatter failed")
	if _, err := rec.Finish(injected); !errors.Is(err, injected) {
		t.Fatalf("Finish: %v", err)
	}
	if rec.Succeeded() {
		t.Fatal("formatter error became a successful receipt")
	}
	if _, err := rec.Finish(nil); err == nil {
		t.Fatal("second Finish repaired a failed receipt")
	}
	if rec.Succeeded() {
		t.Fatal("truncated/failed receipt became success")
	}
}

func TestOperationReceiptNilObserverIsNoop(t *testing.T) {
	t.Parallel()
	var rec *operationReceipt
	if err := rec.ObserveMetadata(nil); err != nil {
		t.Fatal(err)
	}
	if err := rec.ObserveRow(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := rec.Finish(nil); err != nil {
		t.Fatal(err)
	}
	if rec.Succeeded() {
		t.Fatal("nil receipt reported success")
	}
}

func TestSavepointReceiptBufferedAndStreamingOnce(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	stmt := spanner.NewStatement("SELECT 1")

	iter, _, err := h.tm.RunQueryWithStats(ctx, stmt, false, sppb.ExecuteSqlRequest_PROFILE)
	if err != nil {
		t.Fatal(err)
	}
	buffered := &operationReceipt{}
	if _, _, _, _, err := consumeRowIterObserving(iter, func(*spanner.Row) error { return nil }, buffered); err != nil {
		t.Fatal(err)
	}
	if _, err := buffered.Finish(nil); err != nil {
		t.Fatal(err)
	}
	if !buffered.Succeeded() || buffered.observed != 1 {
		t.Fatalf("buffered receipt: succeeded=%v rows=%d", buffered.Succeeded(), buffered.observed)
	}

	iter, _, err = h.tm.RunQueryWithStats(ctx, stmt, false, sppb.ExecuteSqlRequest_PROFILE)
	if err != nil {
		t.Fatal(err)
	}
	writerRec := &operationReceipt{}
	if _, _, err := runRowIteratorTransform(iter, func(row *spanner.Row) (*spanner.Row, error) { return row, nil }, rowIteratorSink[*spanner.Row]{}, withRowIteratorReceipt(writerRec)); err != nil {
		t.Fatal(err)
	}
	if !writerRec.Succeeded() || writerRec.observed != 1 {
		t.Fatalf("writer-style streaming receipt: succeeded=%v rows=%d", writerRec.Succeeded(), writerRec.observed)
	}

	iter, _, err = h.tm.RunQueryWithStats(ctx, stmt, false, sppb.ExecuteSqlRequest_PROFILE)
	if err != nil {
		t.Fatal(err)
	}
	procRec := &operationReceipt{}
	transformCalls := 0
	if _, _, err := runRowIteratorTransform(iter, func(row *spanner.Row) (Row, error) {
		transformCalls++
		return Row{nil}, nil
	}, rowIteratorSink[Row]{
		Write: func(Row) error { return nil },
	}, withRowIteratorReceipt(procRec)); err != nil {
		t.Fatal(err)
	}
	if !procRec.Succeeded() || procRec.observed != 1 || transformCalls != 1 {
		t.Fatalf("processor streaming receipt: succeeded=%v rows=%d transforms=%d", procRec.Succeeded(), procRec.observed, transformCalls)
	}
	if !bytes.Equal(buffered.fingerprint, writerRec.fingerprint) || !bytes.Equal(writerRec.fingerprint, procRec.fingerprint) {
		t.Fatal("buffered and streaming receipts hashed differently for the same SELECT 1")
	}
}

func TestSavepointReceiptWriteErrorIsNotSuccess(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	iter, _, err := h.tm.RunQueryWithStats(ctx, spanner.NewStatement("SELECT 1"), false, sppb.ExecuteSqlRequest_PROFILE)
	if err != nil {
		t.Fatal(err)
	}
	rec := &operationReceipt{}
	writeErr := errors.New("pager failed")
	_, _, err = runRowIteratorTransform(iter, func(row *spanner.Row) (*spanner.Row, error) { return row, nil }, rowIteratorSink[*spanner.Row]{
		Write: func(*spanner.Row) error { return writeErr },
	}, withRowIteratorReceipt(rec))
	if !errors.Is(err, writeErr) {
		t.Fatalf("write error: %v", err)
	}
	if rec.Succeeded() {
		t.Fatal("write error finalized a successful receipt")
	}
}

func TestSavepointReceiptObserverOffUnchanged(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	iter, _, err := h.tm.RunQueryWithStats(ctx, spanner.NewStatement("SELECT 1"), false, sppb.ExecuteSqlRequest_PROFILE)
	if err != nil {
		t.Fatal(err)
	}
	var seen int
	_, _, _, _, err = consumeRowIter(iter, func(*spanner.Row) error {
		seen++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen != 1 {
		t.Fatalf("observer-off consume: rows=%d", seen)
	}
	iter, _, err = h.tm.RunQueryWithStats(ctx, spanner.NewStatement("SELECT 1"), false, sppb.ExecuteSqlRequest_PROFILE)
	if err != nil {
		t.Fatal(err)
	}
	res, n, err := runRowIteratorTransform(iter, func(row *spanner.Row) (*spanner.Row, error) { return row, nil }, rowIteratorSink[*spanner.Row]{})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || res == nil {
		t.Fatalf("observer-off transform: n=%d result=%v", n, res)
	}
}

func TestSavepointReceiptDMLCollectObserving(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	iter, _, err := h.tm.RunQueryWithStats(ctx, spanner.NewStatement("SELECT 1"), false, sppb.ExecuteSqlRequest_PROFILE)
	if err != nil {
		t.Fatal(err)
	}
	rec := &operationReceipt{}
	rows, _, _, _, _, err := consumeRowIterCollectObserving(iter, func(r *spanner.Row) (*spanner.Row, error) { return r, nil }, rec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rec.FinishDML(1, err); err != nil {
		t.Fatal(err)
	}
	if !rec.Succeeded() || len(rows) != 1 {
		t.Fatalf("DML-style collect: succeeded=%v rows=%d", rec.Succeeded(), len(rows))
	}

	iter, _, err = h.tm.RunQueryWithStats(ctx, spanner.NewStatement("SELECT 1"), false, sppb.ExecuteSqlRequest_PROFILE)
	if err != nil {
		t.Fatal(err)
	}
	other := &operationReceipt{}
	_, _, _, _, _, err = consumeRowIterCollectObserving(iter, func(r *spanner.Row) (*spanner.Row, error) { return r, nil }, other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.FinishDML(2, nil); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(rec.fingerprint, other.fingerprint) {
		t.Fatal("DML collector fingerprints ignored affected-row counts")
	}
}

func TestSavepointReceiptEmptyResultIsSuccess(t *testing.T) {
	t.Parallel()
	rec := &operationReceipt{}
	md := &sppb.ResultSetMetadata{RowType: &sppb.StructType{Fields: []*sppb.StructType_Field{
		{Name: "id", Type: &sppb.Type{Code: sppb.TypeCode_INT64}},
	}}}
	if err := rec.ObserveMetadata(md); err != nil {
		t.Fatal(err)
	}
	if _, err := rec.Finish(nil); err != nil {
		t.Fatal(err)
	}
	if !rec.Succeeded() || rec.observed != 0 {
		t.Fatalf("empty result: succeeded=%v rows=%d", rec.Succeeded(), rec.observed)
	}
}

func TestSavepointReceiptTruncationIsNotSuccess(t *testing.T) {
	t.Parallel()
	rec := &operationReceipt{}
	row, err := spanner.NewRow([]string{"v"}, []any{int64(1)})
	if err != nil {
		t.Fatal(err)
	}
	if err := rec.ObserveRow(row); err != nil {
		t.Fatal(err)
	}
	if _, err := rec.Finish(io.ErrUnexpectedEOF); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("truncated Finish: %v", err)
	}
	if rec.Succeeded() {
		t.Fatal("truncated iteration became a successful receipt")
	}
}

func TestSavepointReceiptFinishDMLIncludesAffectedCount(t *testing.T) {
	t.Parallel()
	row, err := spanner.NewRow([]string{"id"}, []any{int64(1)})
	if err != nil {
		t.Fatal(err)
	}
	query := &operationReceipt{}
	if err := query.ObserveRow(row); err != nil {
		t.Fatal(err)
	}
	if _, err := query.Finish(nil); err != nil {
		t.Fatal(err)
	}
	dml := &operationReceipt{}
	if err := dml.ObserveRow(row); err != nil {
		t.Fatal(err)
	}
	if _, err := dml.FinishDML(1, nil); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(query.fingerprint, dml.fingerprint) {
		t.Fatal("DML fingerprint matched query fingerprint; affected count was not hashed")
	}
	if !dml.Succeeded() {
		t.Fatal("successful DML receipt was not marked succeeded")
	}
	if _, err := query.FinishDML(1, nil); err == nil {
		t.Fatal("FinishDML reused a query completion fingerprint")
	}
}

func TestSavepointReceiptFinishBatchIncludesCountVector(t *testing.T) {
	t.Parallel()
	rec := &operationReceipt{}
	if _, err := rec.FinishBatch([]int64{1, 2}, nil); err != nil {
		t.Fatal(err)
	}
	other := &operationReceipt{}
	if _, err := other.FinishBatch([]int64{1, 3}, nil); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(rec.fingerprint, other.fingerprint) {
		t.Fatal("batch fingerprints ignored per-statement counts")
	}
	if !rec.Succeeded() {
		t.Fatal("successful batch receipt was not marked succeeded")
	}
}

func TestSavepointReceiptQueryExecutionForwardingPaths(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	sysVars := h.tm.sysVars
	session := sessionForTM(t, h.tm)

	newExec := func(t *testing.T, rec *operationReceipt, mode enums.DisplayMode) *queryExecution {
		t.Helper()
		iter, _, err := h.tm.RunQueryWithStats(ctx, spanner.NewStatement("SELECT 1"), false, sppb.ExecuteSqlRequest_PROFILE)
		if err != nil {
			t.Fatal(err)
		}
		sysVars.Display.CLIFormat = mode
		render, err := prepareFormatConfig("SELECT 1", sysVars, queryRenderingFrom(sysVars))
		if err != nil {
			t.Fatal(err)
		}
		render.CLIFormat = mode
		render.Export.CLIFormat = mode
		render.StreamingMode = enums.StreamingModeTrue
		return &queryExecution{
			Session: session,
			Out:     OperationOutput{w: io.Discard},
			Iter:    iter,
			SQL:     "SELECT 1",
			SysVars: sysVars,
			Render:  render,
			Metrics: newMetrics(sysVars),
			Receipt: rec,
		}
	}

	buffered := &operationReceipt{}
	if _, err := executeWithBuffering(ctx, newExec(t, buffered, enums.DisplayModeTable)); err != nil {
		t.Fatal(err)
	}
	if !buffered.Succeeded() || buffered.observed != 1 {
		t.Fatalf("executeWithBuffering receipt: succeeded=%v rows=%d", buffered.Succeeded(), buffered.observed)
	}

	writerRec := &operationReceipt{}
	writerQE := newExec(t, writerRec, enums.DisplayModeCSV)
	w, handled, err := newSpanvalueRowIteratorWriter(writerQE)
	if err != nil || !handled || w == nil {
		t.Fatalf("CSV writer: handled=%v err=%v", handled, err)
	}
	if _, _, err := runSpanvalueRowIterator(writerQE, w); err != nil {
		t.Fatal(err)
	}
	if !writerRec.Succeeded() || writerRec.observed != 1 {
		t.Fatalf("runSpanvalueRowIterator receipt: succeeded=%v rows=%d", writerRec.Succeeded(), writerRec.observed)
	}

	procRec := &operationReceipt{}
	procQE := newExec(t, procRec, enums.DisplayModeTab)
	proc, err := streamingProcessorForMode(procQE.Render, io.Discard, 80)
	if err != nil {
		t.Fatal(err)
	}
	procQE.Processor = proc
	if _, _, err := runSpanvalueRowIteratorWithProcessor(procQE, spannerRowToRow(procQE.Render.Spanvalue, procQE.Render.TypeStyles, procQE.Render.NullStyle)); err != nil {
		t.Fatal(err)
	}
	if !procRec.Succeeded() || procRec.observed != 1 {
		t.Fatalf("runSpanvalueRowIteratorWithProcessor receipt: succeeded=%v rows=%d", procRec.Succeeded(), procRec.observed)
	}
	if !bytes.Equal(buffered.fingerprint, writerRec.fingerprint) || !bytes.Equal(writerRec.fingerprint, procRec.fingerprint) {
		t.Fatal("forwarding paths hashed the same SELECT 1 differently")
	}
}

func TestSavepointReceiptQueryExecutionZeroRowsAndTransformError(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	session, live := newEmptySQLRPCSession(t, nil, map[string]any{})
	iter, _, err := session.txn.RunQueryWithStats(ctx, spanner.NewStatement("SELECT 1"), false, sppb.ExecuteSqlRequest_PROFILE)
	if err != nil {
		t.Fatal(err)
	}
	empty := &operationReceipt{}
	if _, err := executeWithBuffering(ctx, &queryExecution{
		Session: session,
		Iter:    iter,
		SQL:     "SELECT 1",
		SysVars: live,
		Render:  queryRenderingFrom(live),
		Metrics: newMetrics(live),
		Receipt: empty,
	}); err != nil {
		t.Fatal(err)
	}
	if !empty.Succeeded() || empty.observed != 0 {
		t.Fatalf("zero-row buffering receipt: succeeded=%v rows=%d", empty.Succeeded(), empty.observed)
	}

	h := newHeartbeatHarness(t)
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	iter, _, err = h.tm.RunQueryWithStats(ctx, spanner.NewStatement("SELECT 1"), false, sppb.ExecuteSqlRequest_PROFILE)
	if err != nil {
		t.Fatal(err)
	}
	sysVars := h.tm.sysVars
	sysVars.Display.CLIFormat = enums.DisplayModeTab
	render, err := prepareFormatConfig("SELECT 1", sysVars, queryRenderingFrom(sysVars))
	if err != nil {
		t.Fatal(err)
	}
	render.CLIFormat = enums.DisplayModeTab
	render.StreamingMode = enums.StreamingModeTrue
	failRec := &operationReceipt{}
	qe := &queryExecution{
		Session: sessionForTM(t, h.tm),
		Out:     OperationOutput{w: io.Discard},
		Iter:    iter,
		SQL:     "SELECT 1",
		SysVars: sysVars,
		Render:  render,
		Metrics: newMetrics(sysVars),
		Receipt: failRec,
	}
	proc, err := streamingProcessorForMode(qe.Render, io.Discard, 80)
	if err != nil {
		t.Fatal(err)
	}
	qe.Processor = proc
	transformErr := errors.New("transform failed")
	_, _, err = runSpanvalueRowIteratorWithProcessor(qe, func(*spanner.Row) (Row, error) {
		return nil, transformErr
	})
	if !errors.Is(err, transformErr) {
		t.Fatalf("processor transform error: %v", err)
	}
	if failRec.Succeeded() {
		t.Fatal("transform error finalized a successful receipt")
	}

	iter, _, err = h.tm.RunQueryWithStats(ctx, spanner.NewStatement("SELECT 1"), false, sppb.ExecuteSqlRequest_PROFILE)
	if err != nil {
		t.Fatal(err)
	}
	writeErr := errors.New("writer failed")
	writerFail := &operationReceipt{}
	writerQE := &queryExecution{
		Session: sessionForTM(t, h.tm),
		Out:     OperationOutput{w: io.Discard},
		Iter:    iter,
		SQL:     "SELECT 1",
		SysVars: sysVars,
		Render:  render,
		Metrics: newMetrics(sysVars),
		Receipt: writerFail,
	}
	_, _, err = runSpanvalueRowIterator(writerQE, failingRowIteratorWriter{err: writeErr})
	if !errors.Is(err, writeErr) {
		t.Fatalf("writer error: %v", err)
	}
	if writerFail.Succeeded() {
		t.Fatal("writer error finalized a successful receipt")
	}
}

type failingRowIteratorWriter struct {
	err error
}

func (w failingRowIteratorWriter) PrepareRowType(*sppb.StructType) error { return nil }

func (w failingRowIteratorWriter) WriteRow(*spanner.Row) error { return w.err }

func (w failingRowIteratorWriter) Flush() error { return nil }
