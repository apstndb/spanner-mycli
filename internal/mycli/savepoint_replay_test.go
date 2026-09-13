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
	"slices"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestSavepointRollbackToReplaysPrefix(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	h.tm.enableSavepointCaptureForTest()
	session := sessionForTM(t, h.tm)
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	if err := h.tm.CreateSavepoint(ctx, "keep"); err != nil {
		t.Fatal(err)
	}
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	if _, err := executeDML(ctx, session, "INSERT INTO T (id) VALUES (1)"); err != nil {
		t.Fatal(err)
	}
	before := replayJournal(h.tm)
	if len(before) != 3 {
		t.Fatalf("journal before rollback: %+v", before)
	}
	if err := h.tm.RollbackToSavepoint(ctx, "keep"); err != nil {
		t.Fatal(err)
	}
	after := replayJournal(h.tm)
	if len(after) != 1 || after[0].stmt.SQL != "SELECT 1" {
		t.Fatalf("journal after rollback: %+v", after)
	}
	h.tm.mu.RLock()
	_, _, ok := h.tm.tc.replay.lookup("keep")
	h.tm.mu.RUnlock()
	if !ok {
		t.Fatal("ROLLBACK TO dropped the target marker")
	}
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
}

func TestSavepointRecoveryRequiredUntilRollbackTo(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	h.tm.enableSavepointCaptureForTest()
	session := sessionForTM(t, h.tm)
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	if err := h.tm.CreateSavepoint(ctx, "keep"); err != nil {
		t.Fatal(err)
	}
	ok, err := h.tm.TryEnqueueAutomaticDML(spanner.NewStatement("INSERT INTO T (id) VALUES (2)"))
	if err != nil || !ok {
		t.Fatalf("enqueue: ok=%v err=%v", ok, err)
	}
	h.server.setFailBatchDML(status.Error(codes.AlreadyExists, "injected batch failure"))
	if _, err := h.tm.FlushAutomaticDML(ctx); err == nil {
		t.Fatal("flush succeeded")
	}
	if !h.tm.NeedsRecovery() {
		t.Fatal("failed DML did not enter recovery-required")
	}
	if _, err := h.tm.CommitReadWriteTransaction(ctx); !errors.Is(err, errSavepointRecovery) {
		t.Fatalf("COMMIT during recovery: %v", err)
	}
	if err := h.tm.CreateSavepoint(ctx, "later"); !errors.Is(err, errSavepointRecovery) {
		t.Fatalf("SAVEPOINT during recovery: %v", err)
	}
	h.server.setFailBatchDML(nil)
	if err := h.tm.RollbackToSavepoint(ctx, "keep"); err != nil {
		t.Fatal(err)
	}
	if h.tm.NeedsRecovery() {
		t.Fatal("ROLLBACK TO did not clear recovery-required")
	}
	after := replayJournal(h.tm)
	if len(after) != 1 {
		t.Fatalf("recovered journal: %+v", after)
	}
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
}

func TestSavepointReleaseAndDuplicateAndUnknown(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	h.tm.enableSavepointCaptureForTest()
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	if err := h.tm.CreateSavepoint(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if err := h.tm.CreateSavepoint(ctx, "a"); !errors.Is(err, errSavepointDuplicate) {
		t.Fatalf("duplicate: %v", err)
	}
	if err := h.tm.CreateSavepoint(ctx, "b"); err != nil {
		t.Fatal(err)
	}
	if err := h.tm.ReleaseSavepoint("a"); err != nil {
		t.Fatal(err)
	}
	if err := h.tm.RollbackToSavepoint(ctx, "a"); !errors.Is(err, errSavepointUnknown) {
		t.Fatalf("unknown after RELEASE: %v", err)
	}
	if err := h.tm.RollbackToSavepoint(ctx, "missing"); !errors.Is(err, errSavepointUnknown) {
		t.Fatalf("unknown: %v", err)
	}
}

func TestSavepointCommandsRequireExplicitJournal(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	if err := h.tm.CreateSavepoint(ctx, "s"); !errors.Is(err, errSavepointNotInTransaction) {
		t.Fatalf("idle SAVEPOINT: %v", err)
	}
	h.tm.enableSavepointCaptureForTest()
	_, err := h.tm.RunInNewOrExistRwTx(ctx, func(*spanner.ReadWriteStmtBasedTransaction, bool) (int64, *sppb.QueryPlan, *sppb.ResultSetMetadata, error) {
		return 0, nil, nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func lastUserSQLTxnID(h *heartbeatHarness, sql string) string {
	var id string
	for _, rec := range h.server.sqlObservations() {
		if rec.sql == sql && rec.reqTag != "spanner_mycli_heartbeat" {
			id = rec.txnID
		}
	}
	return id
}

func assertReconstructionEnded(t *testing.T, h *heartbeatHarness, err error, wantCause error) {
	t.Helper()
	if !errors.Is(err, errSavepointReconstructionFailed) {
		t.Fatalf("reconstruction error = %v, want %v", err, errSavepointReconstructionFailed)
	}
	if wantCause != nil && !errors.Is(err, wantCause) {
		t.Fatalf("reconstruction cause = %v, want %v", err, wantCause)
	}
	if h.tm.InTransaction() {
		t.Fatal("logical owner survived reconstruction failure")
	}
	if h.tm.NeedsRecovery() {
		t.Fatal("reconstruction failure left recovery-required set")
	}
	if commits := h.server.commitIDs(); len(commits) != 0 {
		t.Fatalf("failed candidate committed: %v", commits)
	}
}

func TestSavepointReplayFingerprintMismatchEndsTransaction(t *testing.T) {
	t.Parallel()
	const (
		dmlSQL   = "INSERT INTO T (id) VALUES (1)"
		querySQL = "SELECT 1"
	)
	tests := []struct {
		name   string
		dml    bool
		sql    string
		mutate func(*heartbeatRPCServer)
		cause  error
	}{
		{
			name:   "dml count 1 to 2",
			dml:    true,
			sql:    dmlSQL,
			mutate: func(s *heartbeatRPCServer) { s.setSQLRowCount(dmlSQL, 2) },
			cause:  errSavepointFingerprintMismatch,
		},
		{
			name:   "dml count 1 to 0",
			dml:    true,
			sql:    dmlSQL,
			mutate: func(s *heartbeatRPCServer) { s.setSQLRowCount(dmlSQL, 0) },
			cause:  errSavepointFingerprintMismatch,
		},
		{
			name:   "dml returned row changed",
			dml:    true,
			sql:    dmlSQL,
			mutate: func(s *heartbeatRPCServer) { s.setSQLValue(dmlSQL, "2") },
			cause:  errSavepointFingerprintMismatch,
		},
		{
			name:   "query returned row changed",
			sql:    querySQL,
			mutate: func(s *heartbeatRPCServer) { s.setSQLValue(querySQL, "2") },
			cause:  errSavepointFingerprintMismatch,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			h := newHeartbeatHarness(t)
			h.tm.enableSavepointCaptureForTest()
			session := sessionForTM(t, h.tm)
			if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
				t.Fatal(err)
			}
			if tc.dml {
				if _, err := executeDML(ctx, session, tc.sql); err != nil {
					t.Fatal(err)
				}
			} else if _, err := executeSQLImplWithVars(ctx, session, tc.sql, session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
				t.Fatal(err)
			}
			if err := h.tm.CreateSavepoint(ctx, "keep"); err != nil {
				t.Fatal(err)
			}
			tc.mutate(h.server)
			err := h.tm.RollbackToSavepoint(ctx, "keep")
			assertReconstructionEnded(t, h, err, tc.cause)
			if len(h.server.rollbackIDs()) == 0 {
				t.Fatal("mismatch did not roll back a physical attempt")
			}
		})
	}
}

func TestSavepointRollbackToDropsEqualPositionMarkers(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tests := []struct {
		name  string
		begin func(*testing.T, *heartbeatHarness)
	}{
		{
			name: "pending",
			begin: func(t *testing.T, h *heartbeatHarness) {
				t.Helper()
				if err := h.tm.BeginPendingTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "read-only",
			begin: func(t *testing.T, h *heartbeatHarness) {
				t.Helper()
				if _, err := h.tm.BeginReadOnlyTransaction(ctx, timestampBoundUnspecified, 0, time.Time{}, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "read-write",
			begin: func(t *testing.T, h *heartbeatHarness) {
				t.Helper()
				if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHeartbeatHarness(t)
			h.tm.enableSavepointCaptureForTest()
			tc.begin(t, h)
			for _, name := range []string{"a", "b", "c"} {
				if err := h.tm.CreateSavepoint(ctx, name); err != nil {
					t.Fatal(err)
				}
			}
			if got := replayMarkerNames(h.tm); !slices.Equal(got, []string{"a", "b", "c"}) {
				t.Fatalf("markers before rollback: %v", got)
			}
			if err := h.tm.RollbackToSavepoint(ctx, "a"); err != nil {
				t.Fatal(err)
			}
			if got := replayMarkerNames(h.tm); !slices.Equal(got, []string{"a"}) {
				t.Fatalf("markers after ROLLBACK TO a: %v", got)
			}
			if h.tm.NeedsRecovery() {
				t.Fatal("equal-position rollback set recovery-required")
			}
		})
	}
}

func TestSavepointHeartbeatDelayedTickDoesNotHitReplacementAttempt(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	h.tm.enableSavepointCaptureForTest()
	h.installBeforeAcquireBarrier()

	idOld := beginRWAndProbe(t, ctx, h, "SELECT 1 AS keep")
	if err := h.tm.CreateSavepoint(ctx, "keep"); err != nil {
		t.Fatal(err)
	}
	sendTick(t, h.ticks)
	waitChan(t, h.arrived, "old-attempt heartbeat eligibility snapshot")

	if err := h.tm.RollbackToSavepoint(ctx, "keep"); err != nil {
		t.Fatal(err)
	}
	idNew := lastUserSQLTxnID(h, "SELECT 1 AS keep")
	if idNew == "" || idNew == idOld {
		t.Fatalf("replay did not start a new physical attempt; old=%s new=%s obs=%v", idOld, idNew, h.server.sqlObservations())
	}

	close(h.release)
	waitChan(t, h.attempt, "old-attempt delayed acquire")
	if got := h.server.heartbeatIDs(); slices.Contains(got, idNew) {
		t.Fatalf("delayed old tick issued SELECT 1 on new attempt %s; heartbeats=%v", idNew, got)
	}
	if got := h.server.heartbeatIDs(); slices.Contains(got, idOld) {
		t.Fatalf("delayed old tick issued SELECT 1 on discarded attempt %s; heartbeats=%v", idOld, got)
	}

	sendTick(t, h.ticks)
	waitChan(t, h.attempt, "restarted heartbeat attempt")
	got := h.server.heartbeatIDs()
	if !slices.Contains(got, idNew) {
		t.Fatalf("restarted heartbeat missing on new attempt; heartbeats=%v new=%s", got, idNew)
	}
	if slices.Contains(got, idOld) {
		t.Fatalf("restarted heartbeat also hit old attempt; heartbeats=%v old=%s new=%s", got, idOld, idNew)
	}
	assertHeartbeatMeta(t, h.server.heartbeatRecords())
}

func TestSavepointROQueryFailurePreservesHandleWithoutRecovery(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	h.tm.enableSavepointCaptureForTest()
	session := sessionForTM(t, h.tm)
	if _, err := h.tm.BeginReadOnlyTransaction(ctx, timestampBoundUnspecified, 0, time.Time{}, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	if err := h.tm.CreateSavepoint(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	owner := txnContext(h.tm)
	h.tm.mu.RLock()
	handle := h.tm.tc.txn
	h.tm.mu.RUnlock()
	if handle == nil {
		t.Fatal("RO begin did not install a handle")
	}

	h.server.setFailROQuery(status.Error(codes.PermissionDenied, "injected RO query failure"))
	_, err := executeSQLImplWithVars(ctx, session, "SELECT 2", session.systemVariables, OperationOutput{w: io.Discard})
	if err == nil {
		t.Fatal("injected RO query succeeded")
	}
	if txnContext(h.tm) != owner {
		t.Fatal("RO query failure replaced the owner")
	}
	if !h.tm.InReadOnlyTransaction() {
		t.Fatalf("mode after RO query failure = %q", h.tm.TransactionMode())
	}
	h.tm.mu.RLock()
	still := h.tm.tc.txn
	h.tm.mu.RUnlock()
	if still != handle {
		t.Fatal("RO query failure discarded the snapshot handle")
	}
	if h.tm.NeedsRecovery() {
		t.Fatal("RO query failure entered reconstruction recovery")
	}
}

func TestSavepointCreateReservesMarkerBeforeAutomaticFlush(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	h.tm.enableSavepointCaptureForTest()
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	ok, err := h.tm.TryEnqueueAutomaticDML(spanner.NewStatement("INSERT INTO T (id) VALUES (2)"))
	if err != nil || !ok {
		t.Fatalf("enqueue: ok=%v err=%v", ok, err)
	}
	owner := txnContext(h.tm)
	name := strings.Repeat("s", savepointNameLimit)
	h.tm.mu.Lock()
	h.tm.tc.replay.retainedBytes = savepointJournalLimit
	h.tm.mu.Unlock()

	if err := h.tm.CreateSavepoint(ctx, name); !errors.Is(err, errSavepointJournalFull) {
		t.Fatalf("CreateSavepoint: %v", err)
	}
	if txnContext(h.tm) != owner {
		t.Fatal("marker admission retired the owner")
	}
	if len(h.server.batchObservations()) != 0 {
		t.Fatalf("CreateSavepoint flushed automatic DML: %v", h.server.batchObservations())
	}
	if len(replayQueued(h.tm)) != 1 {
		t.Fatalf("queued automatic DML was dropped: %+v", replayQueued(h.tm))
	}
	if got := replayMarkerNames(h.tm); len(got) != 0 {
		t.Fatalf("failed SAVEPOINT recorded a marker: %v", got)
	}
}

func TestSavepointReplayCancellationCleansUpWithoutCommit(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	h.tm.enableSavepointCaptureForTest()
	session := sessionForTM(t, h.tm)
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	if err := h.tm.CreateSavepoint(ctx, "keep"); err != nil {
		t.Fatal(err)
	}
	h.server.setFailSQL(context.Canceled)
	err := h.tm.RollbackToSavepoint(ctx, "keep")
	assertReconstructionEnded(t, h, err, context.Canceled)
	if len(h.server.rollbackIDs()) == 0 {
		t.Fatal("cancelled replay did not attempt rollback cleanup")
	}
}
