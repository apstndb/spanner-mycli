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
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	autoDMLCountSQLA = "INSERT INTO AutoCount (id) VALUES (1)"
	autoDMLCountSQLB = "INSERT INTO AutoCount (id) VALUES (2)"
	autoDMLCountSQLC = "UPDATE AutoCount SET flag = TRUE WHERE id > 0"
)

func TestVerifyAutomaticDMLCounts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		queued  []automaticDMLEntry
		counts  []int64
		wantErr bool
		pos     int
		actual  string
	}{
		{
			name:   "all disabled ignores mismatch",
			queued: []automaticDMLEntry{{expected: 1}, {expected: 1}},
			counts: []int64{0, 2},
		},
		{
			name:    "per-statement mismatch despite equal totals",
			queued:  []automaticDMLEntry{{expected: 1, verify: true}, {expected: 1, verify: true}},
			counts:  []int64{0, 2},
			wantErr: true,
			pos:     1,
			actual:  "actual 0",
		},
		{
			name:    "second statement mismatch",
			queued:  []automaticDMLEntry{{expected: 1, verify: true}, {expected: 1, verify: true}},
			counts:  []int64{1, 0},
			wantErr: true,
			pos:     2,
			actual:  "actual 0",
		},
		{
			name:   "matching enabled counts",
			queued: []automaticDMLEntry{{expected: 1, verify: true}, {expected: 1, verify: true}},
			counts: []int64{1, 1},
		},
		{
			name:   "zero expected and actual",
			queued: []automaticDMLEntry{{expected: 0, verify: true}},
			counts: []int64{0},
		},
		{
			name:   "later SET enabling verification does not check frozen-off entry",
			queued: []automaticDMLEntry{{expected: 1, verify: false}, {expected: 1, verify: true}},
			counts: []int64{9, 1},
		},
		{
			name:    "missing actual count is not fabricated",
			queued:  []automaticDMLEntry{{expected: 1, verify: true}},
			counts:  nil,
			wantErr: true,
			pos:     1,
			actual:  "actual count missing",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := verifyAutomaticDMLCounts(tc.queued, tc.counts)
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("verifyAutomaticDMLCounts: %v", err)
				}
				return
			}
			if !errors.Is(err, errAutomaticDMLCountMismatch) {
				t.Fatalf("error = %v, want %v", err, errAutomaticDMLCountMismatch)
			}
			if !strings.Contains(err.Error(), "statement "+strconv.Itoa(tc.pos)) {
				t.Fatalf("error = %v, want statement %d", err, tc.pos)
			}
			if !strings.Contains(err.Error(), tc.actual) {
				t.Fatalf("error = %v, want %q", err, tc.actual)
			}
		})
	}
}

func TestAutomaticDMLExpectedCountPolicyFrozenAtEnqueue(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	h.tm.sysVars.Transaction.AutoBatchDMLUpdateCount = 1
	h.tm.sysVars.Transaction.AutoBatchDMLUpdateCountVerification = true
	if ok, err := h.tm.TryEnqueueAutomaticDML(spanner.NewStatement(autoDMLCountSQLA)); err != nil || !ok {
		t.Fatalf("enqueue A: ok=%v err=%v", ok, err)
	}
	h.tm.sysVars.Transaction.AutoBatchDMLUpdateCount = 0
	h.tm.sysVars.Transaction.AutoBatchDMLUpdateCountVerification = false
	if ok, err := h.tm.TryEnqueueAutomaticDML(spanner.NewStatement(autoDMLCountSQLB)); err != nil || !ok {
		t.Fatalf("enqueue B: ok=%v err=%v", ok, err)
	}

	h.tm.mu.RLock()
	queued := append([]automaticDMLEntry(nil), h.tm.tc.autoDML...)
	h.tm.mu.RUnlock()
	if len(queued) != 2 {
		t.Fatalf("queued = %d, want 2", len(queued))
	}
	if !queued[0].verify || queued[0].expected != 1 {
		t.Fatalf("entry 0 = %+v, want verify=true expected=1", queued[0])
	}
	if queued[1].verify || queued[1].expected != 0 {
		t.Fatalf("entry 1 = %+v, want verify=false expected=0", queued[1])
	}

	h.server.setSQLRowCount(autoDMLCountSQLA, 1)
	h.server.setSQLRowCount(autoDMLCountSQLB, 7)
	res, err := h.tm.FlushAutomaticDML(ctx)
	if err != nil {
		t.Fatalf("flush mixed frozen policy: %v", err)
	}
	if res == nil || !res.IsExecutedDML || res.AffectedRows != 8 {
		t.Fatalf("flush result: %+v", res)
	}
}

func TestAutomaticDMLExpectedCountFlushMatrix(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tests := []struct {
		name       string
		setup      func(*TransactionManager)
		sqls       []string
		counts     map[string]int64
		wantErr    bool
		wantCause  error
		wantSubstr string
		wantRows   int
	}{
		{
			name:     "default verification off accepts multi-row UPDATE",
			sqls:     []string{autoDMLCountSQLC},
			counts:   map[string]int64{autoDMLCountSQLC: 5},
			wantRows: 5,
		},
		{
			name: "enabled [1,1] matches actual [1,1]",
			setup: func(tm *TransactionManager) {
				tm.sysVars.Transaction.AutoBatchDMLUpdateCountVerification = true
			},
			sqls:     []string{autoDMLCountSQLA, autoDMLCountSQLB},
			wantRows: 2,
		},
		{
			name: "enabled [1,1] rejects actual [0,2]",
			setup: func(tm *TransactionManager) {
				tm.sysVars.Transaction.AutoBatchDMLUpdateCountVerification = true
			},
			sqls:       []string{autoDMLCountSQLA, autoDMLCountSQLB},
			counts:     map[string]int64{autoDMLCountSQLA: 0, autoDMLCountSQLB: 2},
			wantErr:    true,
			wantCause:  errAutomaticDMLCountMismatch,
			wantSubstr: "statement 1: expected 1, actual 0",
		},
		{
			name: "enabled expectation 0 matches actual 0",
			setup: func(tm *TransactionManager) {
				tm.sysVars.Transaction.AutoBatchDMLUpdateCount = 0
				tm.sysVars.Transaction.AutoBatchDMLUpdateCountVerification = true
			},
			sqls:     []string{autoDMLCountSQLC},
			counts:   map[string]int64{autoDMLCountSQLC: 0},
			wantRows: 0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHeartbeatHarness(t)
			if tc.setup != nil {
				tc.setup(h.tm)
			}
			if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
				t.Fatal(err)
			}
			for _, sql := range tc.sqls {
				if n, ok := tc.counts[sql]; ok {
					h.server.setSQLRowCount(sql, n)
				}
				if ok, err := h.tm.TryEnqueueAutomaticDML(spanner.NewStatement(sql)); err != nil || !ok {
					t.Fatalf("enqueue %s: ok=%v err=%v", sql, ok, err)
				}
			}
			res, err := h.tm.FlushAutomaticDML(ctx)
			if tc.wantErr {
				if err == nil {
					t.Fatal("flush succeeded")
				}
				if tc.wantCause != nil && !errors.Is(err, tc.wantCause) {
					t.Fatalf("flush error = %v, want %v", err, tc.wantCause)
				}
				if tc.wantSubstr != "" && !strings.Contains(err.Error(), tc.wantSubstr) {
					t.Fatalf("flush error = %v, want %q", err, tc.wantSubstr)
				}
				if h.tm.HasAutomaticDML() {
					t.Fatal("failed flush left replayable automatic DML")
				}
				if h.tm.InTransaction() {
					t.Fatal("mismatch without SAVEPOINT left the owner")
				}
				if len(h.server.commitIDs()) != 0 {
					t.Fatalf("mismatch committed: %v", h.server.commitIDs())
				}
				return
			}
			if err != nil {
				t.Fatalf("flush: %v", err)
			}
			if res == nil || !res.IsExecutedDML || res.AffectedRows != tc.wantRows {
				t.Fatalf("flush result: %+v, want affected %d", res, tc.wantRows)
			}
		})
	}
}

func TestAutomaticDMLExpectedCountRPCFailurePreserved(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	h.tm.sysVars.Transaction.AutoBatchDMLUpdateCountVerification = true
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	if ok, err := h.tm.TryEnqueueAutomaticDML(spanner.NewStatement(autoDMLCountSQLA)); err != nil || !ok {
		t.Fatalf("enqueue A: ok=%v err=%v", ok, err)
	}
	if ok, err := h.tm.TryEnqueueAutomaticDML(spanner.NewStatement(autoDMLCountSQLB)); err != nil || !ok {
		t.Fatalf("enqueue B: ok=%v err=%v", ok, err)
	}
	injected := status.Error(codes.AlreadyExists, "injected automatic DML prefix failure")
	h.server.setFailBatchDML(injected)
	_, err := h.tm.FlushAutomaticDML(ctx)
	if err == nil {
		t.Fatal("flush succeeded")
	}
	if !strings.Contains(err.Error(), "injected automatic DML prefix failure") {
		t.Fatalf("flush error = %v, want original RPC cause", err)
	}
	if errors.Is(err, errAutomaticDMLCountMismatch) {
		t.Fatalf("RPC failure was replaced with count mismatch: %v", err)
	}
	if h.tm.HasAutomaticDML() {
		t.Fatal("RPC failure left replayable automatic DML")
	}
	if h.tm.InTransaction() {
		t.Fatal("RPC failure left the owner")
	}
}

func TestAutomaticDMLExpectedCountMismatchWithSavepointRequiresRecovery(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	h.tm.enableSavepointCaptureForTest()
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	if err := h.tm.CreateSavepoint(ctx, "keep"); err != nil {
		t.Fatal(err)
	}
	owner := txnContext(h.tm)
	keepBytes := replayRetainedBytes(h.tm)
	h.tm.sysVars.Transaction.AutoBatchDMLUpdateCountVerification = true
	if ok, err := h.tm.TryEnqueueAutomaticDML(spanner.NewStatement(autoDMLCountSQLA)); err != nil || !ok {
		t.Fatalf("enqueue A: ok=%v err=%v", ok, err)
	}
	if ok, err := h.tm.TryEnqueueAutomaticDML(spanner.NewStatement(autoDMLCountSQLB)); err != nil || !ok {
		t.Fatalf("enqueue B: ok=%v err=%v", ok, err)
	}
	h.server.setSQLRowCount(autoDMLCountSQLA, 0)
	h.server.setSQLRowCount(autoDMLCountSQLB, 2)
	_, err := h.tm.FlushAutomaticDML(ctx)
	if !errors.Is(err, errAutomaticDMLCountMismatch) {
		t.Fatalf("flush error = %v, want mismatch", err)
	}
	if !strings.Contains(err.Error(), "statement 1: expected 1, actual 0") {
		t.Fatalf("flush error = %v, want statement 1 details", err)
	}
	if txnContext(h.tm) != owner {
		t.Fatal("mismatch retired the logical owner")
	}
	if !h.tm.NeedsRecovery() {
		t.Fatal("mismatch did not enter recovery-required")
	}
	if _, err := h.tm.CommitReadWriteTransaction(ctx); !errors.Is(err, errSavepointRecovery) {
		t.Fatalf("COMMIT after mismatch: %v", err)
	}
	if got := replayJournal(h.tm); len(got) != 0 {
		t.Fatalf("mismatched batch was journaled: %+v", got)
	}
	if got := replayRetainedBytes(h.tm); got != keepBytes {
		t.Fatalf("retained after mismatch = %d, want keep marker %d", got, keepBytes)
	}
	if err := h.tm.RollbackToSavepoint(ctx, "keep"); err != nil {
		t.Fatal(err)
	}
	if h.tm.NeedsRecovery() {
		t.Fatal("ROLLBACK TO did not clear recovery-required")
	}
	if _, err := h.tm.CommitReadWriteTransaction(ctx); err != nil {
		t.Fatalf("COMMIT after ROLLBACK TO: %v", err)
	}
}

func TestAutomaticDMLExpectedCountMismatchBeforeSavepointEndsOwner(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	h.tm.enableSavepointCaptureForTest()
	h.tm.sysVars.Transaction.AutoBatchDMLUpdateCountVerification = true
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	if ok, err := h.tm.TryEnqueueAutomaticDML(spanner.NewStatement(autoDMLCountSQLA)); err != nil || !ok {
		t.Fatalf("enqueue: ok=%v err=%v", ok, err)
	}
	h.server.setSQLRowCount(autoDMLCountSQLA, 0)
	_, err := h.tm.FlushAutomaticDML(ctx)
	if !errors.Is(err, errAutomaticDMLCountMismatch) {
		t.Fatalf("flush error = %v, want mismatch", err)
	}
	if h.tm.InTransaction() {
		t.Fatal("mismatch before SAVEPOINT left a logical owner")
	}
	if h.tm.NeedsRecovery() {
		t.Fatal("mismatch before SAVEPOINT entered reconstruction recovery")
	}
	if len(h.server.commitIDs()) != 0 {
		t.Fatalf("mismatch committed: %v", h.server.commitIDs())
	}
}

func TestAutomaticDMLExpectedCountFlushTriggers(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tests := []struct {
		name    string
		trigger func(t *testing.T, ctx context.Context, h *heartbeatHarness, session *Session) (*Result, error)
		check   func(t *testing.T, res *Result)
	}{
		{
			name: "SELECT",
			trigger: func(t *testing.T, ctx context.Context, h *heartbeatHarness, session *Session) (*Result, error) {
				t.Helper()
				return executeSQLImplWithVars(ctx, session, "SELECT 1", session.systemVariables, OperationOutput{w: io.Discard})
			},
			check: func(t *testing.T, res *Result) {
				t.Helper()
				if res.IsExecutedDML {
					t.Fatalf("SELECT presented flushed DML as its own result: %+v", res)
				}
			},
		},
		{
			name: "SAVEPOINT",
			trigger: func(t *testing.T, ctx context.Context, h *heartbeatHarness, session *Session) (*Result, error) {
				t.Helper()
				return nil, h.tm.CreateSavepoint(ctx, "after")
			},
		},
		{
			name: "RUN BATCH",
			trigger: func(t *testing.T, ctx context.Context, h *heartbeatHarness, session *Session) (*Result, error) {
				t.Helper()
				return session.ExecuteStatement(ctx, &RunBatchStatement{})
			},
			check: func(t *testing.T, res *Result) {
				t.Helper()
				if res == nil || !res.IsExecutedDML || res.AffectedRows != 2 {
					t.Fatalf("RUN BATCH result: %+v", res)
				}
			},
		},
		{
			name: "COMMIT",
			trigger: func(t *testing.T, ctx context.Context, h *heartbeatHarness, session *Session) (*Result, error) {
				t.Helper()
				return session.ExecuteStatement(ctx, &CommitStatement{})
			},
			check: func(t *testing.T, res *Result) {
				t.Helper()
				if res == nil || !res.IsExecutedDML || res.AffectedRows != 2 {
					t.Fatalf("COMMIT result: %+v", res)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHeartbeatHarness(t)
			session := sessionForTM(t, h.tm)
			h.tm.sysVars.Transaction.AutoBatchDMLUpdateCountVerification = true
			if tc.name == "SAVEPOINT" {
				h.tm.enableSavepointCaptureForTest()
			}
			if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
				t.Fatal(err)
			}
			if tc.name == "SAVEPOINT" {
				if err := h.tm.CreateSavepoint(ctx, "keep"); err != nil {
					t.Fatal(err)
				}
			}
			if ok, err := h.tm.TryEnqueueAutomaticDML(spanner.NewStatement(autoDMLCountSQLA)); err != nil || !ok {
				t.Fatalf("enqueue A: ok=%v err=%v", ok, err)
			}
			if ok, err := h.tm.TryEnqueueAutomaticDML(spanner.NewStatement(autoDMLCountSQLB)); err != nil || !ok {
				t.Fatalf("enqueue B: ok=%v err=%v", ok, err)
			}
			res, err := tc.trigger(t, ctx, h, session)
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if h.tm.HasAutomaticDML() {
				t.Fatalf("%s left automatic work queued", tc.name)
			}
			if n := len(h.server.batchObservations()); n != 1 {
				t.Fatalf("%s issued %d BatchUpdate calls: %v", tc.name, n, h.server.batchObservations())
			}
			if tc.check != nil {
				tc.check(t, res)
			}
		})
	}
}

func TestAutomaticDMLExpectedCountMismatchOnSavepointFlush(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	h.tm.enableSavepointCaptureForTest()
	h.tm.sysVars.Transaction.AutoBatchDMLUpdateCountVerification = true
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	if err := h.tm.CreateSavepoint(ctx, "keep"); err != nil {
		t.Fatal(err)
	}
	if ok, err := h.tm.TryEnqueueAutomaticDML(spanner.NewStatement(autoDMLCountSQLA)); err != nil || !ok {
		t.Fatalf("enqueue: ok=%v err=%v", ok, err)
	}
	h.server.setSQLRowCount(autoDMLCountSQLA, 0)
	err := h.tm.CreateSavepoint(ctx, "later")
	if !errors.Is(err, errAutomaticDMLCountMismatch) {
		t.Fatalf("CreateSavepoint: %v", err)
	}
	if !h.tm.NeedsRecovery() {
		t.Fatal("SAVEPOINT flush mismatch did not enter recovery")
	}
	if _, err := h.tm.CommitReadWriteTransaction(ctx); !errors.Is(err, errSavepointRecovery) {
		t.Fatalf("COMMIT after SAVEPOINT flush mismatch: %v", err)
	}
}

func TestAutomaticDMLReplayActualCountsRemainRequired(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	h.tm.enableSavepointCaptureForTest()
	session := sessionForTM(t, h.tm)
	h.tm.sysVars.Transaction.AutoBatchDMLUpdateCountVerification = true
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	if ok, err := h.tm.TryEnqueueAutomaticDML(spanner.NewStatement(autoDMLCountSQLA)); err != nil || !ok {
		t.Fatalf("enqueue A: ok=%v err=%v", ok, err)
	}
	if ok, err := h.tm.TryEnqueueAutomaticDML(spanner.NewStatement(autoDMLCountSQLB)); err != nil || !ok {
		t.Fatalf("enqueue B: ok=%v err=%v", ok, err)
	}
	if _, err := h.tm.FlushAutomaticDML(ctx); err != nil {
		t.Fatal(err)
	}
	if err := h.tm.CreateSavepoint(ctx, "keep"); err != nil {
		t.Fatal(err)
	}
	journal := replayJournal(h.tm)
	if len(journal) != 1 || journal[0].kind != replayKindBatchDML {
		t.Fatalf("journal after flush: %+v", journal)
	}
	h.tm.sysVars.Transaction.AutoBatchDMLUpdateCountVerification = false
	h.server.setSQLRowCount(autoDMLCountSQLA, 0)
	h.server.setSQLRowCount(autoDMLCountSQLB, 2)
	err := h.tm.RollbackToSavepoint(ctx, "keep")
	assertReconstructionEnded(t, h, err, errSavepointFingerprintMismatch)
	if session.txn.HasAutomaticDML() {
		t.Fatal("replay left automatic DML queued")
	}
}

func TestAutomaticDMLEnqueueDoesNotReportExpectedCountAsAffectedRows(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	h.tm.sysVars.Transaction.AutoBatchDML = true
	h.tm.sysVars.Transaction.AutoBatchDMLUpdateCount = 1
	h.tm.sysVars.Transaction.AutoBatchDMLUpdateCountVerification = true
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	res, err := bufferOrExecuteDML(ctx, session, autoDMLCountSQLA)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsExecutedDML || res.AffectedRows != 0 || res.TableHeader != nil {
		t.Fatalf("enqueue fabricated observed rows: %+v", res)
	}
	if !session.txn.HasAutomaticDML() {
		t.Fatal("AUTO_BATCH_DML did not enqueue")
	}
}
