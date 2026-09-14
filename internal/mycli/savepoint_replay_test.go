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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/enums"
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

func TestSavepointReplayQueryRowOrderMismatchEndsTransaction(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	const sql = "SELECT ordered"
	h := newHeartbeatHarness(t)
	h.tm.enableSavepointCaptureForTest()
	session := sessionForTM(t, h.tm)
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	h.server.setSQLRows(sql, []string{"1", "2"})
	if _, err := executeSQLImplWithVars(ctx, session, sql, session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	if err := h.tm.CreateSavepoint(ctx, "keep"); err != nil {
		t.Fatal(err)
	}
	h.server.setSQLRows(sql, []string{"2", "1"})
	err := h.tm.RollbackToSavepoint(ctx, "keep")
	assertReconstructionEnded(t, h, err, errSavepointFingerprintMismatch)
}

func TestSavepointReplayBatchCountVectorMismatchEndsTransaction(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	sqlA := "INSERT INTO T (id) VALUES (1)"
	sqlB := "INSERT INTO T (id) VALUES (2)"
	h := newHeartbeatHarness(t)
	h.tm.enableSavepointCaptureForTest()
	session := sessionForTM(t, h.tm)
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	h.server.setSQLRowCount(sqlA, 1)
	h.server.setSQLRowCount(sqlB, 2)
	if _, err := executeBatchDML(ctx, session, []spanner.Statement{
		spanner.NewStatement(sqlA),
		spanner.NewStatement(sqlB),
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.tm.CreateSavepoint(ctx, "keep"); err != nil {
		t.Fatal(err)
	}
	h.server.setSQLRowCount(sqlA, 2)
	h.server.setSQLRowCount(sqlB, 1)
	err := h.tm.RollbackToSavepoint(ctx, "keep")
	assertReconstructionEnded(t, h, err, errSavepointFingerprintMismatch)
}

func TestSavepointReplayUsesFrozenParamsOptionsAndTransactionTag(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	const sql = "SELECT @p"
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	mustExec(t, ctx, session, "SET CLI_SAVEPOINT_SUPPORT = 'ENABLED'")
	mustExec(t, ctx, session, "SET TRANSACTION_TAG = 'owner-tag'")
	mustExec(t, ctx, session, "SET STATEMENT_TAG = 'stmt-tag'")
	mustExec(t, ctx, session, "SET OPTIMIZER_VERSION = '1'")
	mustExec(t, ctx, session, "SET PARAM p = 1")
	mustExec(t, ctx, session, "BEGIN RW")
	if _, err := executeSQLImplWithVars(ctx, session, sql, session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	mustExec(t, ctx, session, "SAVEPOINT keep")
	before := replayRetainedBytes(h.tm)
	mustExec(t, ctx, session, "SET STATEMENT_TAG = 'later-tag'")
	mustExec(t, ctx, session, "SET OPTIMIZER_VERSION = '2'")
	mustExec(t, ctx, session, "SET PARAM p = 99")
	h.tm.mu.Lock()
	h.tm.sysVars.Transaction.TransactionTag = "next-txn"
	h.tm.mu.Unlock()
	mustExec(t, ctx, session, "ROLLBACK TO SAVEPOINT keep")
	if got := replayRetainedBytes(h.tm); got != before {
		t.Fatalf("replay retainedBytes=%d, want prefix %d", got, before)
	}

	replay, ok := lastSQLObservation(h.server.sqlObservations(), sql)
	if !ok {
		t.Fatal("no replay ExecuteSql for SELECT @p")
	}
	if replay.params["p"] != "1" {
		t.Fatalf("replay param p = %q, want frozen 1; obs=%v", replay.params["p"], replay.params)
	}
	if replay.reqTag != "stmt-tag" {
		t.Fatalf("replay STATEMENT_TAG = %q", replay.reqTag)
	}
	if replay.optimizer != "1" {
		t.Fatalf("replay OPTIMIZER_VERSION = %q", replay.optimizer)
	}

	begins := h.server.beginObservations()
	if len(begins) < 2 {
		t.Fatalf("begin observations = %+v", begins)
	}
	if begins[0].txnTag != "owner-tag" {
		t.Fatalf("original begin tag = %q", begins[0].txnTag)
	}
	if begins[len(begins)-1].txnTag != "owner-tag" {
		t.Fatalf("reconstructed begin tag = %q, want frozen owner-tag", begins[len(begins)-1].txnTag)
	}
	if got := mustGetVar(t, session, "TRANSACTION_TAG"); got != "owner-tag" {
		t.Fatalf("applied TRANSACTION_TAG after replay = %q", got)
	}

	if _, err := executeSQLImplWithVars(ctx, session, sql, session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	later, ok := lastSQLObservation(h.server.sqlObservations(), sql)
	if !ok {
		t.Fatal("no later ExecuteSql for SELECT @p")
	}
	if later.params["p"] != "99" {
		t.Fatalf("later param p = %q, want current 99; obs=%v", later.params["p"], later.params)
	}
	if later.reqTag != "later-tag" {
		t.Fatalf("later STATEMENT_TAG = %q", later.reqTag)
	}
	if later.optimizer != "2" {
		t.Fatalf("later OPTIMIZER_VERSION = %q", later.optimizer)
	}

	mustExec(t, ctx, session, "COMMIT")
	mustExec(t, ctx, session, "BEGIN RW")
	begins = h.server.beginObservations()
	if begins[len(begins)-1].txnTag != "next-txn" {
		t.Fatalf("next transaction tag = %q; reconstruction consumed the next-owner slot; begins=%+v", begins[len(begins)-1].txnTag, begins)
	}
}

func TestSavepointReplayUsesFrozenMixedCaseParamSpelling(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	const sql = "SELECT @mixedcase"
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	mustExec(t, ctx, session, "SET CLI_SAVEPOINT_SUPPORT = 'ENABLED'")
	mustExec(t, ctx, session, "SET PARAM MixedCase = 1")
	mustExec(t, ctx, session, "BEGIN RW")
	if _, err := executeSQLImplWithVars(ctx, session, sql, session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	mustExec(t, ctx, session, "SAVEPOINT keep")
	mustExec(t, ctx, session, "SET PARAM mixedcase = 99")
	mustExec(t, ctx, session, "ROLLBACK TO SAVEPOINT keep")

	replay, ok := lastSQLObservation(h.server.sqlObservations(), sql)
	if !ok {
		t.Fatal("no replay ExecuteSql for SELECT @mixedcase")
	}
	if replay.sql != sql {
		t.Fatalf("replay SQL = %q, want original bytes", replay.sql)
	}
	if _, ok := replay.params["MixedCase"]; ok {
		t.Fatalf("replay used stored spelling MixedCase: %v", replay.params)
	}
	if replay.params["mixedcase"] != "1" {
		t.Fatalf("replay param mixedcase = %q, want frozen 1; obs=%v", replay.params["mixedcase"], replay.params)
	}

	if _, err := executeSQLImplWithVars(ctx, session, sql, session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	later, ok := lastSQLObservation(h.server.sqlObservations(), sql)
	if !ok {
		t.Fatal("no later ExecuteSql for SELECT @mixedcase")
	}
	if later.params["mixedcase"] != "99" {
		t.Fatalf("later param mixedcase = %q, want current 99; obs=%v", later.params["mixedcase"], later.params)
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
	session := sessionForTM(t, h.tm)

	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1 AS keep", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	if !h.tm.heartbeatEnabled() {
		t.Fatal("first user operation did not enable heartbeat")
	}
	idOld := lastUserSQLTxnID(h, "SELECT 1 AS keep")
	if idOld == "" {
		t.Fatal("no transaction id captured for the original attempt")
	}
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

func TestSavepointHeartbeatDelayedStartupDoesNotHitReplacementAttempt(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	h.tm.enableSavepointCaptureForTest()
	session := sessionForTM(t, h.tm)

	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	startGate := make(chan struct{})
	oldDone := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-startGate:
		default:
			close(startGate)
		}
	})
	var n atomic.Int32
	h.tm.mu.Lock()
	orig := h.tm.tc.heartbeatFunc
	h.tm.tc.heartbeatFunc = func(hbCtx context.Context, startedAttempt uint64) {
		if n.Add(1) == 1 {
			<-startGate
			defer close(oldDone)
		}
		orig(hbCtx, startedAttempt)
	}
	h.tm.mu.Unlock()

	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1 AS keep", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	if !h.tm.heartbeatEnabled() {
		t.Fatal("first user operation did not enable heartbeat")
	}
	idOld := lastUserSQLTxnID(h, "SELECT 1 AS keep")
	if idOld == "" {
		t.Fatal("no transaction id captured for the original attempt")
	}
	if err := h.tm.CreateSavepoint(ctx, "keep"); err != nil {
		t.Fatal(err)
	}
	if err := h.tm.RollbackToSavepoint(ctx, "keep"); err != nil {
		t.Fatal(err)
	}
	idNew := lastUserSQLTxnID(h, "SELECT 1 AS keep")
	if idNew == "" || idNew == idOld {
		t.Fatalf("replay did not start a new physical attempt; old=%s new=%s obs=%v", idOld, idNew, h.server.sqlObservations())
	}

	close(startGate)
	waitChan(t, oldDone, "delayed original heartbeat startup")
	if got := h.server.heartbeatIDs(); slices.Contains(got, idNew) {
		t.Fatalf("delayed old startup issued SELECT 1 on new attempt %s; heartbeats=%v", idNew, got)
	}
	if got := h.server.heartbeatIDs(); slices.Contains(got, idOld) {
		t.Fatalf("delayed old startup issued SELECT 1 on discarded attempt %s; heartbeats=%v", idOld, got)
	}

	h.tm.heartbeatAfterAttempt = func() {
		select {
		case h.attempt <- struct{}{}:
		default:
		}
	}
	sendTick(t, h.ticks)
	waitChan(t, h.attempt, "restarted heartbeat after delayed startup")
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

func TestSavepointCreateFirstMarkerAutomaticFlushFailureDoesNotPanic(t *testing.T) {
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
	h.server.setFailBatchDML(status.Error(codes.PermissionDenied, "injected first-marker flush failure"))
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("CreateSavepoint panicked after terminal flush: %v", r)
		}
	}()
	err = h.tm.CreateSavepoint(ctx, "first")
	if err == nil {
		t.Fatal("CreateSavepoint succeeded after automatic DML flush failure")
	}
	if h.tm.InTransaction() {
		t.Fatal("first-marker flush failure left a logical owner")
	}
	if h.tm.NeedsRecovery() {
		t.Fatal("first-marker flush failure entered reconstruction recovery")
	}
	if got := replayMarkerNames(h.tm); len(got) != 0 {
		t.Fatalf("failed first marker was recorded: %v", got)
	}
}

func TestSavepointCreateLaterMarkerAutomaticFlushFailureEntersRecovery(t *testing.T) {
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
	ok, err := h.tm.TryEnqueueAutomaticDML(spanner.NewStatement("INSERT INTO T (id) VALUES (2)"))
	if err != nil || !ok {
		t.Fatalf("enqueue: ok=%v err=%v", ok, err)
	}
	h.server.setFailBatchDML(status.Error(codes.PermissionDenied, "injected later-marker flush failure"))
	if err := h.tm.CreateSavepoint(ctx, "later"); err == nil {
		t.Fatal("CreateSavepoint succeeded after automatic DML flush failure")
	}
	if txnContext(h.tm) != owner {
		t.Fatal("later-marker flush failure retired the logical owner")
	}
	if !h.tm.NeedsRecovery() {
		t.Fatal("later-marker flush failure did not enter recovery-required")
	}
	if got := replayMarkerNames(h.tm); !slices.Equal(got, []string{"keep"}) {
		t.Fatalf("markers after later-marker flush failure = %v, want [keep]", got)
	}
	if got := replayRetainedBytes(h.tm); got != keepBytes {
		t.Fatalf("retained after later-marker flush failure = %d, want keep marker %d", got, keepBytes)
	}
	assertReplayReservationEqualsPayload(t, h.tm)
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
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 2", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	if err := h.tm.CreateSavepoint(ctx, "keep"); err != nil {
		t.Fatal(err)
	}
	idOld := lastUserSQLTxnID(h, "SELECT 1")
	if idOld == "" {
		t.Fatal("no transaction id captured for the original attempt")
	}

	releaseSQL := make(chan struct{})
	blocked := make(chan struct{})
	h.server.setSQLBlocked(sync.OnceFunc(func() { close(blocked) }))
	h.server.setSkipSQL(1)
	h.server.setBlockSQL(releaseSQL)
	t.Cleanup(func() {
		select {
		case <-releaseSQL:
		default:
			close(releaseSQL)
		}
	})

	cmdCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- h.tm.RollbackToSavepoint(cmdCtx, "keep")
	}()

	waitChan(t, blocked, "reconstruction candidate SQL after id assignment")
	candidateID := lastUserSQLTxnID(h, "SELECT 1")
	if candidateID == "" || candidateID == idOld {
		t.Fatalf("blocked replay did not preserve a new candidate id; old=%s new=%s obs=%v", idOld, candidateID, h.server.sqlObservations())
	}

	cancel()
	// Keep the candidate SQL blocked. prepareSQL already selects on ctx.Done();
	// closing releaseSQL here used to make the successful-response path
	// available before cancellation necessarily reached the RPC handler.
	// t.Cleanup still unblocks the mock if the test fails or times out.

	var err error
	select {
	case err = <-errCh:
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for cancelled ROLLBACK TO")
	case <-t.Context().Done():
		t.Fatalf("test cancelled waiting for ROLLBACK TO: %v", t.Context().Err())
	}
	assertReconstructionEnded(t, h, err, nil)
	if !isCanceledCause(err) {
		t.Fatalf("cancelled replay error = %v, want canceled cause", err)
	}
	if !slices.Contains(h.server.rollbackIDs(), candidateID) {
		t.Fatalf("cancelled replay cleanup missed candidate %s; rollbacks=%v", candidateID, h.server.rollbackIDs())
	}
	if slices.Contains(h.server.commitIDs(), candidateID) {
		t.Fatalf("cancelled replay committed candidate %s; commits=%v", candidateID, h.server.commitIDs())
	}
}

func TestSavepointStaleIteratorDoesNotPoisonReplacementOwner(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	h.tm.enableSavepointCaptureForTest()
	session := sessionForTM(t, h.tm)
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	const staleSQL = "SELECT 1 AS stale"
	iterA, _, tokA, err := h.tm.runQueryWithStatsAndCapture(ctx, spanner.NewStatement(staleSQL), false, sppb.ExecuteSqlRequest_PROFILE)
	if err != nil {
		t.Fatal(err)
	}
	if tokA == nil {
		t.Fatal("query A was not admitted")
	}
	if err := h.tm.RollbackReadWriteTransaction(ctx); err != nil {
		t.Fatal(err)
	}

	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 2", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	if err := h.tm.CreateSavepoint(ctx, "keep"); err != nil {
		t.Fatal(err)
	}
	ownerB := txnContext(h.tm)
	h.tm.mu.RLock()
	handleB := h.tm.tc.txn
	attemptB := h.tm.tc.attempt
	h.tm.mu.RUnlock()
	if handleB == nil {
		t.Fatal("replacement owner has no physical handle")
	}
	journalB := replayJournal(h.tm)
	obs, ok := lastSQLObservation(h.server.sqlObservations(), "SELECT 2")
	if !ok {
		t.Fatal("replacement SELECT 2 was not observed")
	}
	idB := obs.txnID
	rollbacksBefore := h.server.rollbackIDs()

	run := func(context.Context, spanner.Statement, bool, sppb.ExecuteSqlRequest_QueryMode) (*spanner.RowIterator, *spanner.ReadOnlyTransaction, *captureToken, error) {
		return iterA, nil, tokA, nil
	}
	_, err = executeSQLImplWithQueryRunner(ctx, session, staleSQL, session.systemVariables, run, true, OperationOutput{w: io.Discard})
	if err == nil {
		t.Fatal("retired iterator A succeeded")
	}
	if spanner.ErrCode(err) != codes.FailedPrecondition {
		t.Fatalf("old iterator error = %v (%v), want FailedPrecondition", err, spanner.ErrCode(err))
	}

	if txnContext(h.tm) != ownerB {
		t.Fatal("stale iterator error replaced owner B")
	}
	h.tm.mu.RLock()
	still := h.tm.tc.txn
	attempt := h.tm.tc.attempt
	h.tm.mu.RUnlock()
	if still != handleB {
		t.Fatal("stale iterator error discarded owner B's handle")
	}
	if attempt != attemptB {
		t.Fatalf("stale iterator error changed B attempt %d -> %d", attemptB, attempt)
	}
	if h.tm.NeedsRecovery() {
		t.Fatal("stale iterator error set B to recovery-required")
	}
	if got := replayMarkerNames(h.tm); !slices.Equal(got, []string{"keep"}) {
		t.Fatalf("B markers = %v, want [keep]", got)
	}
	after := replayJournal(h.tm)
	if len(after) != len(journalB) {
		t.Fatalf("B journal changed: before=%+v after=%+v", journalB, after)
	}
	for _, id := range h.server.rollbackIDs()[len(rollbacksBefore):] {
		if id == idB {
			t.Fatal("cleanup RPC targeted replacement owner B")
		}
	}
}

func TestSavepointSingleUseFailureDoesNotPoisonExplicitOwner(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	h.tm.enableSavepointCaptureForTest()
	session := sessionForTM(t, h.tm)
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 2", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	if err := h.tm.CreateSavepoint(ctx, "keep"); err != nil {
		t.Fatal(err)
	}
	owner := txnContext(h.tm)
	h.tm.mu.RLock()
	handle := h.tm.tc.txn
	h.tm.mu.RUnlock()
	obs, ok := lastSQLObservation(h.server.sqlObservations(), "SELECT 2")
	if !ok {
		t.Fatal("explicit SELECT 2 was not observed")
	}
	idB := obs.txnID
	rollbacksBefore := h.server.rollbackIDs()

	h.server.setFailSQL(status.Error(codes.PermissionDenied, "injected single-use failure"))
	_, err := executeSQLImplSingleUse(ctx, session, "SELECT 9", session.systemVariables, OperationOutput{w: io.Discard})
	if err == nil {
		t.Fatal("single-use failure succeeded")
	}
	if txnContext(h.tm) != owner {
		t.Fatal("single-use failure replaced the explicit owner")
	}
	h.tm.mu.RLock()
	still := h.tm.tc.txn
	h.tm.mu.RUnlock()
	if still != handle {
		t.Fatal("single-use failure discarded the explicit handle")
	}
	if h.tm.NeedsRecovery() {
		t.Fatal("single-use failure set recovery-required")
	}
	if got := replayMarkerNames(h.tm); !slices.Equal(got, []string{"keep"}) {
		t.Fatalf("markers = %v, want [keep]", got)
	}
	for _, id := range h.server.rollbackIDs()[len(rollbacksBefore):] {
		if id == idB {
			t.Fatal("single-use failure rolled back the explicit owner")
		}
	}
}

func TestSavepointOwnerQueryFailureStillEntersRecovery(t *testing.T) {
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
	owner := txnContext(h.tm)
	h.server.setFailSQL(status.Error(codes.AlreadyExists, "injected owner query failure"))
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 3", session.systemVariables, OperationOutput{w: io.Discard}); err == nil {
		t.Fatal("owner query succeeded")
	}
	if txnContext(h.tm) != owner {
		t.Fatal("owner query failure retired the logical owner")
	}
	if !h.tm.NeedsRecovery() {
		t.Fatal("same-owner query failure did not enter recovery-required")
	}
	h.tm.mu.RLock()
	handle := h.tm.tc.txn
	h.tm.mu.RUnlock()
	if handle != nil {
		t.Fatal("recovery left the failed physical handle")
	}
}

func TestSavepointExplainAnalyzeFailureEntersRecovery(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		code codes.Code
		run  func(context.Context, *testing.T, *Session) error
	}{
		{
			name: "explain_analyze_permission_denied",
			code: codes.PermissionDenied,
			run: func(ctx context.Context, t *testing.T, session *Session) error {
				t.Helper()
				_, err := executeExplainAnalyze(ctx, session, "SELECT 9", enums.ExplainFormatUnspecified, 0, nil)
				return err
			},
		},
		{
			name: "cli_query_mode_profile_permission_denied",
			code: codes.PermissionDenied,
			run: func(ctx context.Context, t *testing.T, session *Session) error {
				t.Helper()
				session.systemVariables.Query.QueryMode = sppb.ExecuteSqlRequest_PROFILE.Enum()
				_, err := (&SelectStatement{Query: "SELECT 9"}).Execute(ctx, session, OperationOutput{w: io.Discard})
				return err
			},
		},
		{
			name: "explain_analyze_aborted",
			code: codes.Aborted,
			run: func(ctx context.Context, t *testing.T, session *Session) error {
				t.Helper()
				_, err := executeExplainAnalyze(ctx, session, "SELECT 9", enums.ExplainFormatUnspecified, 0, nil)
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
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
			owner := txnContext(h.tm)
			prefix := replayJournal(h.tm)

			h.server.setFailSQL(status.Error(tc.code, "injected explain analyze failure"))
			err := tc.run(ctx, t, session)
			if err == nil {
				t.Fatal("EXPLAIN ANALYZE succeeded")
			}
			if spanner.ErrCode(err) != tc.code {
				t.Fatalf("error = %v (%v), want %v", err, spanner.ErrCode(err), tc.code)
			}
			if txnContext(h.tm) != owner {
				t.Fatal("EXPLAIN ANALYZE failure retired the logical owner")
			}
			if !h.tm.NeedsRecovery() {
				t.Fatal("EXPLAIN ANALYZE failure did not enter recovery-required")
			}
			h.tm.mu.RLock()
			handle := h.tm.tc.txn
			h.tm.mu.RUnlock()
			if handle != nil {
				t.Fatal("recovery left the failed physical handle")
			}
			if _, err := h.tm.CommitReadWriteTransaction(ctx); !errors.Is(err, errSavepointRecovery) {
				t.Fatalf("COMMIT during recovery: %v", err)
			}
			if err := h.tm.CreateSavepoint(ctx, "later"); !errors.Is(err, errSavepointRecovery) {
				t.Fatalf("SAVEPOINT during recovery: %v", err)
			}
			if _, err := executeSQLImplWithVars(ctx, session, "SELECT 2", session.systemVariables, OperationOutput{w: io.Discard}); !errors.Is(err, errSavepointRecovery) {
				t.Fatalf("SELECT during recovery: %v", err)
			}

			h.server.setFailSQL(nil)
			if err := h.tm.RollbackToSavepoint(ctx, "keep"); err != nil {
				t.Fatal(err)
			}
			if h.tm.NeedsRecovery() {
				t.Fatal("ROLLBACK TO did not clear recovery-required")
			}
			after := replayJournal(h.tm)
			if len(after) != len(prefix) {
				t.Fatalf("recovered journal = %+v, want prefix %+v", after, prefix)
			}
			if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSavepointExplainDescribeSelectFailureEntersRecovery(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		code codes.Code
		run  func(context.Context, *testing.T, *Session) error
	}{
		{
			name: "explain_select_permission_denied",
			code: codes.PermissionDenied,
			run: func(ctx context.Context, t *testing.T, session *Session) error {
				t.Helper()
				_, err := executeExplain(ctx, session, "SELECT 2", false, enums.ExplainFormatUnspecified, 0, nil)
				return err
			},
		},
		{
			name: "describe_select_permission_denied",
			code: codes.PermissionDenied,
			run: func(ctx context.Context, t *testing.T, session *Session) error {
				t.Helper()
				_, err := (&DescribeStatement{Statement: "SELECT 2"}).Execute(ctx, session, OperationOutput{w: io.Discard})
				return err
			},
		},
		{
			name: "explain_select_aborted",
			code: codes.Aborted,
			run: func(ctx context.Context, t *testing.T, session *Session) error {
				t.Helper()
				_, err := executeExplain(ctx, session, "SELECT 2", false, enums.ExplainFormatUnspecified, 0, nil)
				return err
			},
		},
		{
			name: "describe_select_aborted",
			code: codes.Aborted,
			run: func(ctx context.Context, t *testing.T, session *Session) error {
				t.Helper()
				_, err := (&DescribeStatement{Statement: "SELECT 2"}).Execute(ctx, session, OperationOutput{w: io.Discard})
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
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
			owner := txnContext(h.tm)
			prefix := replayJournal(h.tm)

			h.server.setFailSQL(status.Error(tc.code, "injected explain describe select failure"))
			err := tc.run(ctx, t, session)
			if err == nil {
				t.Fatal("EXPLAIN/DESCRIBE SELECT succeeded")
			}
			if spanner.ErrCode(err) != tc.code {
				t.Fatalf("error = %v (%v), want %v", err, spanner.ErrCode(err), tc.code)
			}
			if txnContext(h.tm) != owner {
				t.Fatal("EXPLAIN/DESCRIBE SELECT failure retired the logical owner")
			}
			if !h.tm.NeedsRecovery() {
				t.Fatal("EXPLAIN/DESCRIBE SELECT failure did not enter recovery-required")
			}
			h.tm.mu.RLock()
			handle := h.tm.tc.txn
			inFlight := h.tm.tc.inFlight
			h.tm.mu.RUnlock()
			if handle != nil {
				t.Fatal("recovery left the failed physical handle")
			}
			if inFlight != 0 {
				t.Fatalf("recovery left inFlight=%d", inFlight)
			}
			if _, err := h.tm.CommitReadWriteTransaction(ctx); !errors.Is(err, errSavepointRecovery) {
				t.Fatalf("COMMIT during recovery: %v", err)
			}
			if err := h.tm.CreateSavepoint(ctx, "after_error"); err == nil || !errors.Is(err, errSavepointRecovery) {
				t.Fatalf("CreateSavepoint after PLAN failure: %v", err)
			}

			h.server.setFailSQL(nil)
			if err := h.tm.RollbackToSavepoint(ctx, "keep"); err != nil {
				t.Fatal(err)
			}
			if h.tm.NeedsRecovery() {
				t.Fatal("ROLLBACK TO did not clear recovery-required")
			}
			after := replayJournal(h.tm)
			if len(after) != len(prefix) {
				t.Fatalf("recovered journal = %+v, want prefix %+v", after, prefix)
			}
		})
	}
}

func TestSavepointExplainSelectSuccessStaysOutOfJournal(t *testing.T) {
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
	prefix := replayJournal(h.tm)
	if _, _, err := h.tm.RunAnalyzeQuery(ctx, spanner.NewStatement("SELECT 2")); err != nil {
		t.Fatal(err)
	}
	if _, err := (&DescribeStatement{Statement: "SELECT 2"}).Execute(ctx, session, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	after := replayJournal(h.tm)
	if len(after) != len(prefix) {
		t.Fatalf("successful PLAN journaled: %+v, want prefix %+v", after, prefix)
	}
	h.tm.mu.RLock()
	inFlight := 0
	if h.tm.tc != nil {
		inFlight = h.tm.tc.inFlight
	}
	h.tm.mu.RUnlock()
	if inFlight != 0 {
		t.Fatalf("successful PLAN left inFlight=%d", inFlight)
	}
	if h.tm.NeedsRecovery() {
		t.Fatal("successful PLAN entered recovery-required")
	}
	if err := h.tm.CreateSavepoint(ctx, "after_plan"); err != nil {
		t.Fatal(err)
	}
}

func isCanceledCause(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || status.Code(err) == codes.Canceled || spanner.ErrCode(err) == codes.Canceled {
		return true
	}
	switch x := err.(type) {
	case interface{ Unwrap() error }:
		return isCanceledCause(x.Unwrap())
	case interface{ Unwrap() []error }:
		return slices.ContainsFunc(x.Unwrap(), isCanceledCause)
	default:
		return false
	}
}
