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
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/internal/mycli/streamio"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func beginExplicitRetry(t *testing.T, ctx context.Context, session *Session) {
	t.Helper()
	enableRetryAborts(t, ctx, session)
	mustExec(t, ctx, session, "BEGIN RW")
	if !ownerHasReplay(session.txn) {
		t.Fatal("explicit retry owner has no journal")
	}
}

func admittedRetryToken(t *testing.T, tm *TransactionManager) *captureToken {
	t.Helper()
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if tm.tc == nil {
		t.Fatal("no owner for admitted retry token")
	}
	return &captureToken{owner: tm.tc, attempt: tm.tc.attempt}
}

func ownerAttemptHandle(tm *TransactionManager) (*transactionContext, uint64, transaction) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	if tm.tc == nil {
		return nil, 0, nil
	}
	return tm.tc, tm.tc.attempt, tm.tc.txn
}

func TestRetryAbortsExplicitFirstSelectAndDML(t *testing.T) {
	t.Parallel()
	t.Run("select", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newRetryAbortsSession(t, h)
		ctx := t.Context()
		beginExplicitRetry(t, ctx, session)
		h.server.setSQLRows("SELECT 1", []string{"1"})
		h.server.setFailStreamingSQLTimes(1, abortedStatus("select aborted"))
		res, err := execSQL(t, ctx, session, "SELECT 1")
		if err != nil {
			t.Fatalf("SELECT retry: %v", err)
		}
		if res == nil || res.AffectedRows != 1 {
			t.Fatalf("SELECT result: %+v", res)
		}
		if got := countRPC(userSQLObservations(h.server.sqlObservations()), "ExecuteStreamingSql", false); got != 2 {
			t.Fatalf("SELECT RPCs = %d; %+v", got, h.server.sqlObservations())
		}
		if got := len(h.server.beginObservations()); got != 2 {
			t.Fatalf("begins = %d, want original+reconstruct", got)
		}
		if !session.txn.InReadWriteTransaction() {
			t.Fatal("successful retry retired the owner")
		}
	})
	t.Run("dml", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newRetryAbortsSession(t, h)
		ctx := t.Context()
		beginExplicitRetry(t, ctx, session)
		h.server.setSQLRowCount("UPDATE T SET v = 1 WHERE TRUE", 3)
		h.server.setFailStreamingSQLTimes(1, abortedStatus("dml aborted"))
		res, err := execSQL(t, ctx, session, "UPDATE T SET v = 1 WHERE TRUE")
		if err != nil {
			t.Fatalf("DML retry: %v", err)
		}
		if res == nil || res.AffectedRows != 3 {
			t.Fatalf("DML result: %+v", res)
		}
		if got := countRPC(userSQLObservations(h.server.sqlObservations()), "ExecuteStreamingSql", false); got != 2 {
			t.Fatalf("DML RPCs = %d", got)
		}
	})
}

func TestRetryAbortsExplicitCommitAndMutate(t *testing.T) {
	t.Parallel()
	t.Run("commit", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newRetryAbortsSession(t, h)
		ctx := t.Context()
		beginExplicitRetry(t, ctx, session)
		h.server.setSQLRows("SELECT 1", []string{"1"})
		mustExec(t, ctx, session, "SELECT 1")
		h.server.setFailCommitTimes(1, abortedStatus("commit aborted"))
		if _, err := execSQL(t, ctx, session, "COMMIT"); err != nil {
			t.Fatalf("COMMIT retry: %v", err)
		}
		if got := len(h.server.commitIDs()); got != 2 {
			t.Fatalf("commits = %d", got)
		}
		if got := countRPC(userSQLObservations(h.server.sqlObservations()), "ExecuteStreamingSql", false); got != 2 {
			t.Fatalf("prefix replay SQL = %d; want original+silent replay", got)
		}
		if session.txn.InTransaction() {
			t.Fatal("COMMIT left an owner")
		}
	})
	t.Run("mutate", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newRetryAbortsSession(t, h)
		ctx := t.Context()
		beginExplicitRetry(t, ctx, session)
		if _, err := execSQL(t, ctx, session, "MUTATE T INSERT STRUCT(1 AS id)"); err != nil {
			t.Fatal(err)
		}
		h.server.setFailCommitTimes(1, abortedStatus("mutate commit aborted"))
		if _, err := execSQL(t, ctx, session, "COMMIT"); err != nil {
			t.Fatalf("MUTATE commit retry: %v", err)
		}
		if got := len(h.server.commitIDs()); got != 2 {
			t.Fatalf("MUTATE commits = %d", got)
		}
	})
}

func TestRetryAbortsExplicitBatchThenReturnAndProfile(t *testing.T) {
	t.Parallel()
	t.Run("manual_batch", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newRetryAbortsSession(t, h)
		ctx := t.Context()
		beginExplicitRetry(t, ctx, session)
		h.server.setFailBatchDMLTimes(1, abortedStatus("batch aborted"))
		mustExec(t, ctx, session, "START BATCH DML")
		mustExec(t, ctx, session, "UPDATE T SET v = 1 WHERE TRUE")
		if _, err := execSQL(t, ctx, session, "RUN BATCH"); err != nil {
			t.Fatalf("manual batch retry: %v", err)
		}
		if got := len(h.server.batchObservations()); got != 2 {
			t.Fatalf("batch RPCs = %d", got)
		}
	})
	t.Run("automatic_batch", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newRetryAbortsSession(t, h)
		ctx := t.Context()
		beginExplicitRetry(t, ctx, session)
		mustExec(t, ctx, session, "SET AUTO_BATCH_DML = TRUE")
		mustExec(t, ctx, session, "UPDATE T SET v = 1 WHERE TRUE")
		h.server.setFailBatchDMLTimes(1, abortedStatus("auto batch aborted"))
		if _, err := execSQL(t, ctx, session, "SELECT 1"); err != nil {
			t.Fatalf("automatic flush retry: %v", err)
		}
		if got := len(h.server.batchObservations()); got != 2 {
			t.Fatalf("automatic batch RPCs = %d", got)
		}
	})
	t.Run("then_return", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newRetryAbortsSession(t, h)
		ctx := t.Context()
		beginExplicitRetry(t, ctx, session)
		const sql = "UPDATE T SET v = 2 WHERE TRUE THEN RETURN v"
		h.server.setSQLRows(sql, []string{"9"})
		h.server.setSQLRowCount(sql, 1)
		h.server.setFailStreamingSQLTimes(1, abortedStatus("then return aborted"))
		res, err := execSQL(t, ctx, session, sql)
		if err != nil {
			t.Fatalf("THEN RETURN retry: %v", err)
		}
		if res == nil || res.AffectedRows != 1 {
			t.Fatalf("THEN RETURN result: %+v", res)
		}
		if got := countRPC(userSQLObservations(h.server.sqlObservations()), "ExecuteStreamingSql", false); got != 2 {
			t.Fatalf("THEN RETURN RPCs = %d", got)
		}
	})
	t.Run("profile", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newRetryAbortsSession(t, h)
		ctx := t.Context()
		beginExplicitRetry(t, ctx, session)
		h.server.setQueryPlan(&sppb.QueryPlan{PlanNodes: hangingIndentPlanNodes()})
		h.server.setSQLRows("SELECT 1", []string{"1"})
		h.server.setFailStreamingSQLTimes(1, abortedStatus("profile aborted"))
		res, err := executeExplainAnalyze(ctx, session, "SELECT 1", 0, 0, nil)
		if err != nil {
			t.Fatalf("PROFILE retry: %v", err)
		}
		if res == nil || res.AffectedRows != 1 {
			t.Fatalf("PROFILE result: %+v", res)
		}
		if session.systemVariables.LastResult.QueryCache == nil || session.systemVariables.LastResult.QueryCache.QueryPlan == nil {
			t.Fatal("PROFILE did not publish QueryCache")
		}
		if got := countRPC(userSQLObservations(h.server.sqlObservations()), "ExecuteStreamingSql", false); got != 2 {
			t.Fatalf("PROFILE RPCs = %d", got)
		}
	})
}

func TestRetryAbortsExplicitChangedReplayRetires(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		setup  func(*testing.T, context.Context, *Session, *heartbeatHarness)
		mutate func(*heartbeatRPCServer)
	}{
		{
			name: "value",
			setup: func(t *testing.T, ctx context.Context, session *Session, h *heartbeatHarness) {
				h.server.setSQLRows("SELECT 1", []string{"1"})
				mustExec(t, ctx, session, "SELECT 1")
			},
			mutate: func(s *heartbeatRPCServer) { s.setSQLRows("SELECT 1", []string{"2"}) },
		},
		{
			name: "type",
			setup: func(t *testing.T, ctx context.Context, session *Session, h *heartbeatHarness) {
				h.server.setSQLRows("SELECT 1", []string{"1"})
				mustExec(t, ctx, session, "SELECT 1")
			},
			mutate: func(s *heartbeatRPCServer) { s.setSQLType("SELECT 1", sppb.TypeCode_STRING) },
		},
		{
			name: "row_order",
			setup: func(t *testing.T, ctx context.Context, session *Session, h *heartbeatHarness) {
				h.server.setSQLRows("SELECT ordered", []string{"1", "2"})
				mustExec(t, ctx, session, "SELECT ordered")
			},
			mutate: func(s *heartbeatRPCServer) { s.setSQLRows("SELECT ordered", []string{"2", "1"}) },
		},
		{
			name: "row_count",
			setup: func(t *testing.T, ctx context.Context, session *Session, h *heartbeatHarness) {
				h.server.setSQLRows("SELECT 1", []string{"1", "2"})
				mustExec(t, ctx, session, "SELECT 1")
			},
			mutate: func(s *heartbeatRPCServer) { s.setSQLRows("SELECT 1", []string{"1"}) },
		},
		{
			name: "zero_rows",
			setup: func(t *testing.T, ctx context.Context, session *Session, h *heartbeatHarness) {
				h.server.setSQLRows("SELECT 1", []string{"1"})
				mustExec(t, ctx, session, "SELECT 1")
			},
			mutate: func(s *heartbeatRPCServer) { s.setSQLRows("SELECT 1", nil) },
		},
		{
			name: "dml_count",
			setup: func(t *testing.T, ctx context.Context, session *Session, h *heartbeatHarness) {
				h.server.setSQLRowCount("UPDATE T SET v = 1 WHERE TRUE", 3)
				mustExec(t, ctx, session, "UPDATE T SET v = 1 WHERE TRUE")
			},
			mutate: func(s *heartbeatRPCServer) { s.setSQLRowCount("UPDATE T SET v = 1 WHERE TRUE", 0) },
		},
		{
			name: "batch_counts",
			setup: func(t *testing.T, ctx context.Context, session *Session, h *heartbeatHarness) {
				h.server.setSQLRowCount("UPDATE T SET v = 1 WHERE TRUE", 1)
				h.server.setSQLRowCount("UPDATE T SET v = 2 WHERE TRUE", 2)
				mustExec(t, ctx, session, "START BATCH DML")
				mustExec(t, ctx, session, "UPDATE T SET v = 1 WHERE TRUE")
				mustExec(t, ctx, session, "UPDATE T SET v = 2 WHERE TRUE")
				mustExec(t, ctx, session, "RUN BATCH")
			},
			mutate: func(s *heartbeatRPCServer) {
				s.setSQLRowCount("UPDATE T SET v = 1 WHERE TRUE", 2)
				s.setSQLRowCount("UPDATE T SET v = 2 WHERE TRUE", 1)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHeartbeatHarness(t)
			session := newRetryAbortsSession(t, h)
			ctx := t.Context()
			beginExplicitRetry(t, ctx, session)
			tc.setup(t, ctx, session, h)
			tc.mutate(h.server)
			h.server.setFailStreamingSQLTimes(1, abortedStatus("next aborted"))
			_, err := execSQL(t, ctx, session, "SELECT 2")
			if err == nil || !errors.Is(err, errSavepointReconstructionFailed) || !errors.Is(err, errSavepointFingerprintMismatch) {
				t.Fatalf("changed replay: %v", err)
			}
			if session.txn.InTransaction() {
				t.Fatal("mismatch kept the logical owner")
			}
		})
	}
}

func TestRetryAbortsExplicitOutputCompletion(t *testing.T) {
	t.Parallel()
	t.Run("sdk_midstream_abort_retries_as_unpublished", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newRetryAbortsSession(t, h)
		ctx := t.Context()
		beginExplicitRetry(t, ctx, session)
		session.systemVariables.StreamManager = nil
		session.systemVariables.Display.AutoWrap = false
		h.server.setSQLRows("SELECT 1", []string{"1", "2"})
		h.server.setFailStreamingSQLAfterRows(1, abortedStatus("midstream unpublished"))
		res, err := executeSQLImplWithVars(ctx, session, "SELECT 1", session.systemVariables, OperationOutput{})
		if err != nil {
			t.Fatalf("midstream unpublished retry: %v", err)
		}
		if res == nil || res.AffectedRows != 2 {
			t.Fatalf("midstream unpublished result: %+v", res)
		}
		if got := countRPC(userSQLObservations(h.server.sqlObservations()), "ExecuteStreamingSql", false); got != 2 {
			t.Fatalf("midstream unpublished RPCs = %d", got)
		}
	})
	t.Run("buffered_unpublished_retries", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newRetryAbortsSession(t, h)
		ctx := t.Context()
		beginExplicitRetry(t, ctx, session)
		session.systemVariables.StreamManager = nil
		session.systemVariables.Display.AutoWrap = false
		h.server.setSQLRows("SELECT 1", []string{"1"})
		h.server.setFailStreamingSQLTimes(1, abortedStatus("buffered aborted"))
		res, err := executeSQLImplWithVars(ctx, session, "SELECT 1", session.systemVariables, OperationOutput{})
		if err != nil {
			t.Fatalf("buffered retry: %v", err)
		}
		if res == nil || res.AffectedRows != 1 {
			t.Fatalf("buffered result: %+v", res)
		}
		if res.alreadyDelivered() {
			t.Fatal("buffered path delivered during execution")
		}
		if got := countRPC(userSQLObservations(h.server.sqlObservations()), "ExecuteStreamingSql", false); got != 2 {
			t.Fatalf("buffered RPCs = %d", got)
		}
	})
	t.Run("partial_stream_prevents_retry", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newRetryAbortsSession(t, h)
		ctx := t.Context()
		beginExplicitRetry(t, ctx, session)
		begins := len(h.server.beginObservations())
		recovered, handled, err := session.txn.tryExplicitAbortRetry(ctx, admittedRetryToken(t, session.txn), abortedStatus("partial presented"), true)
		if recovered || !handled || !isAbortedErr(err) {
			t.Fatalf("partial presented: recovered=%v handled=%v err=%v", recovered, handled, err)
		}
		if got := len(h.server.beginObservations()); got != begins {
			t.Fatalf("partial output reconstructed: %d -> %d", begins, got)
		}
		if session.txn.InTransaction() && session.txn.NeedsRecovery() {
			t.Fatal("partial output entered SAVEPOINT recovery without a marker")
		}
		if !session.txn.InTransaction() && replayMarkerNames(h.tm) != nil {
			t.Fatal("unexpected marker recovery state")
		}
	})
	t.Run("prior_output_once", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newRetryAbortsSession(t, h)
		ctx := t.Context()
		var out bytes.Buffer
		session.systemVariables.StreamManager = streamio.NewStreamManager(io.NopCloser(strings.NewReader("")), &out, io.Discard)
		mustExec(t, ctx, session, "SET CLI_FORMAT = 'CSV'")
		beginExplicitRetry(t, ctx, session)
		h.server.setSQLRows("SELECT 1", []string{"keep"})
		mustExec(t, ctx, session, "SELECT 1")
		h.server.setSQLRows("SELECT 2", []string{"next"})
		h.server.setFailStreamingSQLTimes(1, abortedStatus("second aborted"))
		if _, err := execSQL(t, ctx, session, "SELECT 2"); err != nil {
			t.Fatalf("second SELECT retry: %v", err)
		}
		if got := strings.Count(out.String(), "keep"); got != 1 {
			t.Fatalf("prior output appearances = %d; out=%q", got, out.String())
		}
		var first, second int
		for _, o := range userSQLObservations(h.server.sqlObservations()) {
			switch o.sql {
			case "SELECT 1":
				first++
			case "SELECT 2":
				second++
			}
		}
		if first != 2 || second != 2 {
			t.Fatalf("SQL counts first=%d second=%d; %+v", first, second, h.server.sqlObservations())
		}
	})
	t.Run("vertical_partial_prevents_retry", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newRetryAbortsSession(t, h)
		ctx := t.Context()
		mustExec(t, ctx, session, "SET CLI_SAVEPOINT_SUPPORT = 'ENABLED'")
		mustExec(t, ctx, session, "SET RETRY_ABORTS_INTERNALLY = TRUE")
		mustExec(t, ctx, session, "BEGIN RW")
		mustExec(t, ctx, session, "SAVEPOINT keep")
		begins := len(h.server.beginObservations())
		recovered, handled, err := session.txn.tryExplicitAbortRetry(ctx, admittedRetryToken(t, session.txn), abortedStatus("vertical presented"), true)
		if recovered || !handled || !isAbortedErr(err) {
			t.Fatalf("vertical presented: recovered=%v handled=%v err=%v", recovered, handled, err)
		}
		if got := len(h.server.beginObservations()); got != begins {
			t.Fatalf("partial output reconstructed: %d -> %d", begins, got)
		}
		if !session.txn.NeedsRecovery() {
			t.Fatal("partial output with a marker did not keep valid recovery")
		}
	})
}

func TestRetryAbortsExplicitMarkersAndBudget(t *testing.T) {
	t.Parallel()
	t.Run("marker_survives", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newRetryAbortsSession(t, h)
		ctx := t.Context()
		mustExec(t, ctx, session, "SET CLI_SAVEPOINT_SUPPORT = 'ENABLED'")
		mustExec(t, ctx, session, "SET RETRY_ABORTS_INTERNALLY = TRUE")
		mustExec(t, ctx, session, "BEGIN RW")
		mustExec(t, ctx, session, "SAVEPOINT keep")
		h.server.setSQLRows("SELECT 1", []string{"1"})
		mustExec(t, ctx, session, "SELECT 1")
		h.server.setFailStreamingSQLTimes(1, abortedStatus("after marker"))
		if _, err := execSQL(t, ctx, session, "SELECT 2"); err != nil {
			t.Fatalf("retry with marker: %v", err)
		}
		if names := replayMarkerNames(h.tm); len(names) != 1 || names[0] != "keep" {
			t.Fatalf("markers after retry = %v", names)
		}
	})
	t.Run("release_retains_history", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newRetryAbortsSession(t, h)
		ctx := t.Context()
		mustExec(t, ctx, session, "SET CLI_SAVEPOINT_SUPPORT = 'ENABLED'")
		mustExec(t, ctx, session, "SET RETRY_ABORTS_INTERNALLY = TRUE")
		mustExec(t, ctx, session, "BEGIN RW")
		mustExec(t, ctx, session, "SAVEPOINT keep")
		h.server.setSQLRows("SELECT 1", []string{"1"})
		mustExec(t, ctx, session, "SELECT 1")
		mustExec(t, ctx, session, "RELEASE SAVEPOINT keep")
		if names := replayMarkerNames(h.tm); len(names) != 0 {
			t.Fatalf("released markers remain: %v", names)
		}
		entries := replayJournal(h.tm)
		if len(entries) != 1 || entries[0].stmt.SQL != "SELECT 1" {
			t.Fatalf("release dropped retry history: %+v", entries)
		}
		h.server.setFailStreamingSQLTimes(1, abortedStatus("after release"))
		if _, err := execSQL(t, ctx, session, "SELECT 2"); err != nil {
			t.Fatalf("retry after release: %v", err)
		}
	})
	t.Run("rollback_to_does_not_reset_budget", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newRetryAbortsSession(t, h)
		ctx := t.Context()
		mustExec(t, ctx, session, "SET CLI_SAVEPOINT_SUPPORT = 'ENABLED'")
		mustExec(t, ctx, session, "SET RETRY_ABORTS_INTERNALLY = TRUE")
		mustExec(t, ctx, session, "BEGIN RW")
		mustExec(t, ctx, session, "SAVEPOINT keep")
		h.server.setFailStreamingSQLTimes(1, abortedStatus("budgeted"))
		if _, err := execSQL(t, ctx, session, "SELECT 1"); err != nil {
			t.Fatal(err)
		}
		h.tm.mu.Lock()
		before := h.tm.tc.abortRetries
		h.tm.mu.Unlock()
		if before != 1 {
			t.Fatalf("abortRetries after first retry = %d", before)
		}
		if err := h.tm.RollbackToSavepoint(ctx, "keep"); err != nil {
			t.Fatal(err)
		}
		h.tm.mu.Lock()
		after := h.tm.tc.abortRetries
		h.tm.mu.Unlock()
		if after != before {
			t.Fatalf("ROLLBACK TO reset abortRetries %d -> %d", before, after)
		}
	})
	t.Run("exhausted_uses_marker_recovery", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newRetryAbortsSession(t, h)
		ctx := t.Context()
		mustExec(t, ctx, session, "SET CLI_SAVEPOINT_SUPPORT = 'ENABLED'")
		mustExec(t, ctx, session, "SET RETRY_ABORTS_INTERNALLY = TRUE")
		mustExec(t, ctx, session, "BEGIN RW")
		mustExec(t, ctx, session, "SAVEPOINT keep")
		h.tm.mu.Lock()
		h.tm.tc.abortRetries = maxExplicitAbortRetries
		h.tm.mu.Unlock()
		h.server.setFailStreamingSQL(abortedStatus("exhausted"))
		_, err := execSQL(t, ctx, session, "SELECT 1")
		if err == nil || !isAbortedErr(err) {
			t.Fatalf("exhausted: %v", err)
		}
		if !session.txn.NeedsRecovery() {
			t.Fatal("exhausted abort with marker did not enter recovery")
		}
		if got := countRPC(userSQLObservations(h.server.sqlObservations()), "ExecuteStreamingSql", false); got != 1 {
			t.Fatalf("exhausted retried: %d", got)
		}
	})
}

func TestRetryAbortsExplicitSetLocalAndSession(t *testing.T) {
	t.Parallel()
	t.Run("reject_direct_rw_and_ro", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newRetryAbortsSession(t, h)
		ctx := t.Context()
		mustExec(t, ctx, session, "BEGIN RW")
		_, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "RETRY_ABORTS_INTERNALLY", Value: "TRUE"})
		if !errors.Is(err, errRetryAbortsSetLocalNotPending) {
			t.Fatalf("SET LOCAL on RW: %v", err)
		}
		mustExec(t, ctx, session, "ROLLBACK")
		mustExec(t, ctx, session, "BEGIN RO")
		_, err = session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "RETRY_ABORTS_INTERNALLY", Value: "TRUE"})
		if !errors.Is(err, errRetryAbortsSetLocalNotPending) {
			t.Fatalf("SET LOCAL on RO: %v", err)
		}
	})
	t.Run("reject_after_use", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newRetryAbortsSession(t, h)
		ctx := t.Context()
		mustExec(t, ctx, session, "BEGIN")
		mustExec(t, ctx, session, "SELECT 1")
		_, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "RETRY_ABORTS_INTERNALLY", Value: "TRUE"})
		if !errors.Is(err, errRetryAbortsSetLocalNotPending) {
			t.Fatalf("SET LOCAL after use: %v", err)
		}
		if got := mustGetVar(t, session, "RETRY_ABORTS_INTERNALLY"); got != "FALSE" {
			t.Fatalf("rejected SET LOCAL changed session value: %s", got)
		}
	})
	t.Run("local_false_drops_unused_journal", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newRetryAbortsSession(t, h)
		ctx := t.Context()
		mustExec(t, ctx, session, "BEGIN")
		if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "RETRY_ABORTS_INTERNALLY", Value: "TRUE"}); err != nil {
			t.Fatal(err)
		}
		if !ownerHasReplay(h.tm) {
			t.Fatal("SET LOCAL TRUE did not allocate a journal")
		}
		if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "RETRY_ABORTS_INTERNALLY", Value: "FALSE"}); err != nil {
			t.Fatal(err)
		}
		if ownerHasReplay(h.tm) {
			t.Fatal("SET LOCAL FALSE left a retry journal")
		}
	})
	t.Run("reset_does_not_change_captured_owner", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newRetryAbortsSession(t, h)
		ctx := t.Context()
		beginExplicitRetry(t, ctx, session)
		if err := session.systemVariables.SetFromSimple("RETRY_ABORTS_INTERNALLY", "FALSE"); err != nil {
			t.Fatal(err)
		}
		if got := mustGetVar(t, session, "RETRY_ABORTS_INTERNALLY"); got != "FALSE" {
			t.Fatalf("session SET value = %s", got)
		}
		h.server.setSQLRows("SELECT 1", []string{"1"})
		h.server.setFailStreamingSQLTimes(1, abortedStatus("captured still true"))
		if _, err := execSQL(t, ctx, session, "SELECT 1"); err != nil {
			t.Fatalf("captured TRUE after RESET: %v", err)
		}
		if got := countRPC(userSQLObservations(h.server.sqlObservations()), "ExecuteStreamingSql", false); got != 2 {
			t.Fatalf("RESET mutated live owner: %d", got)
		}
	})
	t.Run("set_after_local_overrides_undo", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newRetryAbortsSession(t, h)
		ctx := t.Context()
		mustExec(t, ctx, session, "BEGIN")
		if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "RETRY_ABORTS_INTERNALLY", Value: "TRUE"}); err != nil {
			t.Fatal(err)
		}
		mustExec(t, ctx, session, "SET RETRY_ABORTS_INTERNALLY = TRUE")
		mustExec(t, ctx, session, "ROLLBACK")
		if got := mustGetVar(t, session, "RETRY_ABORTS_INTERNALLY"); got != "TRUE" {
			t.Fatalf("SET after LOCAL did not keep session TRUE: %s", got)
		}
	})
	t.Run("stale_completion", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newRetryAbortsSession(t, h)
		ctx := t.Context()
		beginExplicitRetry(t, ctx, session)
		iter, _, tok, err := h.tm.runQueryWithStatsAndCapture(ctx, spanner.NewStatement("SELECT 1"), false, sppb.ExecuteSqlRequest_PROFILE)
		if err != nil {
			t.Fatal(err)
		}
		if tok == nil {
			t.Fatal("query was not admitted")
		}
		defer iter.Stop()
		h.tm.mu.Lock()
		h.tm.tc.attempt++
		replacement := &captureToken{owner: h.tm.tc, attempt: h.tm.tc.attempt, rec: &operationReceipt{}, reserved: 48}
		h.tm.tc.pending = replacement
		h.tm.tc.inFlight = 1
		after := h.tm.tc.replay.retainedBytes
		h.tm.mu.Unlock()
		if err := h.tm.finishQueryCapture(tok, errors.New("stale completion")); err == nil || err.Error() != "stale completion" {
			t.Fatalf("stale finish = %v", err)
		}
		h.tm.mu.Lock()
		defer h.tm.mu.Unlock()
		if h.tm.tc.pending != replacement || h.tm.tc.replay.retainedBytes != after {
			t.Fatal("stale completion mutated the replacement")
		}
	})
}

func TestRetryAbortsExplicitCancelDuringDelay(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	session := newRetryAbortsSession(t, h)
	ctx := t.Context()
	beginExplicitRetry(t, ctx, session)
	started := make(chan struct{})
	h.tm.abortRetryWait = func(ctx context.Context, _ time.Duration) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}
	h.server.setFailStreamingSQL(abortedStatus("wait abort"))
	runCtx, cancel := context.WithCancel(ctx)
	errCh := make(chan error, 1)
	go func() {
		_, err := execSQL(t, runCtx, session, "SELECT 1")
		errCh <- err
	}()
	select {
	case <-started:
	case err := <-errCh:
		t.Fatalf("finished before wait: %v", err)
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}
	cancel()
	err := <-errCh
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel during delay: %v", err)
	}
	if session.txn.InTransaction() {
		t.Fatal("canceled retry left an owner")
	}
}

func TestRetryAbortsExplicitDefaultFalseUnchanged(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	session := newRetryAbortsSession(t, h)
	ctx := t.Context()
	mustExec(t, ctx, session, "BEGIN RW")
	h.server.setFailStreamingSQL(abortedStatus("default false"))
	_, err := execSQL(t, ctx, session, "SELECT 1")
	if err == nil || !isAbortedErr(err) {
		t.Fatalf("default FALSE: %v", err)
	}
	if got := countRPC(userSQLObservations(h.server.sqlObservations()), "ExecuteStreamingSql", false); got != 1 {
		t.Fatalf("default FALSE retried: %d", got)
	}
}

func TestRetryAbortsExplicitDoesNotRetryPlan(t *testing.T) {
	t.Parallel()
	t.Run("select", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newRetryAbortsSession(t, h)
		ctx := t.Context()
		beginExplicitRetry(t, ctx, session)
		h.server.setFailStreamingSQLTimes(1, abortedStatus("plan aborted"))
		_, _, err := h.tm.RunAnalyzeQuery(ctx, spanner.NewStatement("SELECT 1"))
		if err == nil || !isAbortedErr(err) {
			t.Fatalf("PLAN: %v", err)
		}
		if got := countRPC(userSQLObservations(h.server.sqlObservations()), "ExecuteStreamingSql", false); got != 1 {
			t.Fatalf("PLAN retried: %d", got)
		}
	})
	t.Run("dml", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newRetryAbortsSession(t, h)
		ctx := t.Context()
		beginExplicitRetry(t, ctx, session)
		h.server.setFailStreamingSQLTimes(1, abortedStatus("plan dml aborted"))
		_, _, _, err := runAnalyzeQuery(ctx, session, spanner.NewStatement("UPDATE T SET v = 1 WHERE TRUE"), true)
		if err == nil || !isAbortedErr(err) {
			t.Fatalf("PLAN DML: %v", err)
		}
		if got := countRPC(userSQLObservations(h.server.sqlObservations()), "ExecuteStreamingSql", false); got != 1 {
			t.Fatalf("PLAN DML retried: %d", got)
		}
	})
}

func TestRetryAbortsExplicitUnavailableCommitIsNotRetried(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	session := newRetryAbortsSession(t, h)
	ctx := t.Context()
	beginExplicitRetry(t, ctx, session)
	h.server.setSQLRows("SELECT 1", []string{"1"})
	mustExec(t, ctx, session, "SELECT 1")
	var commits atomic.Int32
	h.tm.commitOverride = func(context.Context, *spanner.ReadWriteStmtBasedTransaction) (spanner.CommitResponse, error) {
		commits.Add(1)
		return spanner.CommitResponse{}, status.Error(codes.Unavailable, "commit unavailable")
	}
	_, err := execSQL(t, ctx, session, "COMMIT")
	if err == nil || spanner.ErrCode(err) != codes.Unavailable {
		t.Fatalf("Unavailable COMMIT: %v", err)
	}
	if isAbortedErr(err) {
		t.Fatalf("non-ABORTED labeled aborted: %v", err)
	}
	if got := commits.Load(); got != 1 {
		t.Fatalf("Unavailable COMMIT retried: %d", got)
	}
	if session.txn.InTransaction() {
		t.Fatal("non-ABORTED Commit left an owner")
	}
}

func TestRetryAbortsExplicitOwnerDeadlineStopsRetry(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	session := newRetryAbortsSession(t, h)
	ctx := t.Context()
	mustExec(t, ctx, session, "SET TRANSACTION_TIMEOUT = '1s'")
	beginExplicitRetry(t, ctx, session)
	start := time.Now()
	h.tm.nowFunc = func() time.Time {
		if len(h.server.beginObservations()) >= 2 {
			return start.Add(2 * time.Second)
		}
		return start
	}
	var waited atomic.Bool
	h.tm.abortRetryWait = func(context.Context, time.Duration) error {
		waited.Store(true)
		return nil
	}
	h.server.setFailStreamingSQLTimes(1, abortedStatus("owner deadline"))
	_, err := execSQL(t, ctx, session, "SELECT 1")
	if err == nil || !errors.Is(err, errTransactionTimeout) {
		t.Fatalf("owner deadline: %v", err)
	}
	if waited.Load() {
		t.Fatal("exhausted owner deadline still entered backoff wait")
	}
	if session.txn.InTransaction() {
		t.Fatal("owner remained after TRANSACTION_TIMEOUT")
	}
}

func TestRetryAbortsExplicitCancelDuringReplay(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	session := newRetryAbortsSession(t, h)
	ctx := t.Context()
	beginExplicitRetry(t, ctx, session)
	h.server.setSQLRows("SELECT 1", []string{"1"})
	mustExec(t, ctx, session, "SELECT 1")
	unblock := make(chan struct{})
	started := make(chan struct{})
	h.server.skipSQL = 1
	h.server.blockSQL = unblock
	h.server.sqlBlocked = func() { close(started) }
	h.server.setFailStreamingSQLTimes(1, abortedStatus("replay cancel"))
	runCtx, cancel := context.WithCancel(ctx)
	errCh := make(chan error, 1)
	go func() {
		_, err := execSQL(t, runCtx, session, "SELECT 2")
		errCh <- err
	}()
	select {
	case <-started:
	case err := <-errCh:
		t.Fatalf("finished before replay block: %v", err)
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}
	cancel()
	err := <-errCh
	if !errors.Is(err, context.Canceled) && spanner.ErrCode(err) != codes.Canceled {
		t.Fatalf("cancel during replay: %v", err)
	}
	if session.txn.InTransaction() {
		t.Fatal("canceled replay left an owner")
	}
}

func TestRetryAbortsExplicitSingleUseExportDoesNotReconstructOwner(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	session := newRetryAbortsSession(t, h)
	ctx := t.Context()
	beginExplicitRetry(t, ctx, session)
	owner, attempt, handle := ownerAttemptHandle(h.tm)
	begins := len(h.server.beginObservations())
	const exportSQL = "EXPORT DATA OPTIONS (format = 'CLOUD_SPANNER', table = 'Account') AS SELECT 1"
	h.server.setFailStreamingSQLTimes(1, abortedStatus("export aborted"))
	_, err := session.ExecuteStatement(ctx, &ExportDataStatement{SQL: exportSQL})
	if err == nil || !isAbortedErr(err) || !strings.Contains(err.Error(), "export aborted") {
		t.Fatalf("EXPORT DATA abort: %v", err)
	}
	gotOwner, gotAttempt, gotHandle := ownerAttemptHandle(h.tm)
	if gotOwner != owner || gotAttempt != attempt || gotHandle != handle {
		t.Fatalf("EXPORT DATA reconstructed owner/attempt/handle: %p/%d/%p -> %p/%d/%p",
			owner, attempt, handle, gotOwner, gotAttempt, gotHandle)
	}
	if got := len(h.server.beginObservations()); got != begins {
		t.Fatalf("EXPORT DATA BeginTransaction %d -> %d", begins, got)
	}
	if !session.txn.InReadWriteTransaction() {
		t.Fatal("EXPORT DATA abort retired the isolated RW owner")
	}
}

func TestRetryAbortsExplicitStaleTokenDoesNotRetryReplacement(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	session := newRetryAbortsSession(t, h)
	ctx := t.Context()
	beginExplicitRetry(t, ctx, session)
	iter, _, staleTok, err := h.tm.runQueryWithStatsAndCapture(ctx, spanner.NewStatement("SELECT 1"), false, sppb.ExecuteSqlRequest_PROFILE)
	if err != nil {
		t.Fatal(err)
	}
	if staleTok == nil {
		t.Fatal("first SELECT was not admitted")
	}
	iter.Stop()
	if err := h.tm.finishQueryCapture(staleTok, errors.New("abandon first")); err == nil || err.Error() != "abandon first" {
		t.Fatalf("abandon first: %v", err)
	}
	mustExec(t, ctx, session, "ROLLBACK")
	mustExec(t, ctx, session, "BEGIN RW")
	if !ownerHasReplay(session.txn) {
		t.Fatal("replacement owner has no journal")
	}
	owner, attempt, handle := ownerAttemptHandle(h.tm)
	begins := len(h.server.beginObservations())
	var calls int
	run := func(context.Context, spanner.Statement, bool, sppb.ExecuteSqlRequest_QueryMode) (*spanner.RowIterator, *spanner.ReadOnlyTransaction, *captureToken, error) {
		calls++
		return nil, nil, staleTok, abortedStatus("stale-token")
	}
	_, err = executeSQLImplWithQueryRunner(ctx, session, "SELECT 1", session.systemVariables, run, true, OperationOutput{w: io.Discard})
	if err == nil || !isAbortedErr(err) || !strings.Contains(err.Error(), "stale-token") {
		t.Fatalf("stale token abort: %v", err)
	}
	if calls != 1 {
		t.Fatalf("stale token runner calls = %d, want 1", calls)
	}
	gotOwner, gotAttempt, gotHandle := ownerAttemptHandle(h.tm)
	if gotOwner != owner || gotAttempt != attempt || gotHandle != handle {
		t.Fatalf("stale token reconstructed replacement: %p/%d/%p -> %p/%d/%p",
			owner, attempt, handle, gotOwner, gotAttempt, gotHandle)
	}
	if got := len(h.server.beginObservations()); got != begins {
		t.Fatalf("stale token BeginTransaction %d -> %d", begins, got)
	}
}

func TestRetryAbortsExplicitSelectFreezesRequestOptions(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	session := newRetryAbortsSession(t, h)
	ctx := t.Context()
	beginExplicitRetry(t, ctx, session)
	mustExec(t, ctx, session, "SET STATEMENT_TAG = 'keep-on-retry'")
	mustExec(t, ctx, session, "SET OPTIMIZER_VERSION = '1'")
	h.tm.abortRetryWait = func(context.Context, time.Duration) error {
		session.systemVariables.Query.OptimizerVersion = "2"
		session.systemVariables.Transaction.RequestTag = "mutated-after-consume"
		return nil
	}
	h.server.setSQLRows("SELECT 1", []string{"1"})
	h.server.setFailStreamingSQLTimes(1, abortedStatus("select tag abort"))
	if _, err := execSQL(t, ctx, session, "SELECT 1"); err != nil {
		t.Fatalf("SELECT retry: %v", err)
	}
	obs := userSQLObservations(h.server.sqlObservations())
	var selects []sqlObservation
	for _, o := range obs {
		if o.sql == "SELECT 1" {
			selects = append(selects, o)
		}
	}
	if len(selects) != 2 {
		t.Fatalf("SELECT RPCs = %d; %+v", len(selects), obs)
	}
	for i, o := range selects {
		if o.reqTag != "keep-on-retry" {
			t.Fatalf("SELECT[%d] tag = %q; %+v", i, o.reqTag, o)
		}
		if o.optimizer != "1" {
			t.Fatalf("SELECT[%d] optimizer = %q; %+v", i, o.optimizer, o)
		}
	}
	if session.systemVariables.Transaction.RequestTag != "mutated-after-consume" {
		t.Fatalf("live STATEMENT_TAG = %q", session.systemVariables.Transaction.RequestTag)
	}
}

func TestRetryAbortsExplicitProfileFreezesRequestOptions(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	session := newRetryAbortsSession(t, h)
	ctx := t.Context()
	beginExplicitRetry(t, ctx, session)
	mustExec(t, ctx, session, "SET STATEMENT_TAG = 'keep-on-retry'")
	mustExec(t, ctx, session, "SET OPTIMIZER_VERSION = '1'")
	h.tm.abortRetryWait = func(context.Context, time.Duration) error {
		session.systemVariables.Query.OptimizerVersion = "2"
		session.systemVariables.Transaction.RequestTag = "mutated-after-consume"
		return nil
	}
	h.server.setQueryPlan(&sppb.QueryPlan{PlanNodes: hangingIndentPlanNodes()})
	h.server.setSQLRows("SELECT 1", []string{"1"})
	h.server.setFailStreamingSQLTimes(1, abortedStatus("profile tag abort"))
	if _, err := executeExplainAnalyze(ctx, session, "SELECT 1", 0, 0, nil); err != nil {
		t.Fatalf("PROFILE retry: %v", err)
	}
	obs := userSQLObservations(h.server.sqlObservations())
	var profiles []sqlObservation
	for _, o := range obs {
		if o.sql == "SELECT 1" {
			profiles = append(profiles, o)
		}
	}
	if len(profiles) != 2 {
		t.Fatalf("PROFILE RPCs = %d; %+v", len(profiles), obs)
	}
	for i, o := range profiles {
		if o.reqTag != "keep-on-retry" {
			t.Fatalf("PROFILE[%d] tag = %q; %+v", i, o.reqTag, o)
		}
		if o.optimizer != "1" {
			t.Fatalf("PROFILE[%d] optimizer = %q; %+v", i, o.optimizer, o)
		}
		if o.queryMode != sppb.ExecuteSqlRequest_PROFILE {
			t.Fatalf("PROFILE[%d] mode = %v", i, o.queryMode)
		}
	}
	if session.systemVariables.Transaction.RequestTag != "mutated-after-consume" {
		t.Fatalf("live STATEMENT_TAG = %q", session.systemVariables.Transaction.RequestTag)
	}
}

func TestRetryAbortsExplicitCommitLostOwnerLeavesReplacement(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	session := newRetryAbortsSession(t, h)
	ctx := t.Context()
	beginExplicitRetry(t, ctx, session)
	h.server.setSQLRows("SELECT 1", []string{"1"})
	mustExec(t, ctx, session, "SELECT 1")
	original, _, _ := ownerAttemptHandle(h.tm)
	if original == nil {
		t.Fatal("missing commit owner")
	}

	var (
		replacement     *transactionContext
		replAttempt     uint64
		replHandle      transaction
		replIdle        time.Time
		replRetryAborts bool
	)
	h.server.setFailCommitTimes(1, abortedWithRetryDelay("commit aborted", 20*time.Millisecond))
	h.tm.abortRetryWait = func(context.Context, time.Duration) error {
		if err := h.tm.RollbackReadWriteTransaction(ctx); err != nil {
			t.Errorf("rollback during commit wait: %v", err)
			return err
		}
		if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
			t.Errorf("begin during commit wait: %v", err)
			return err
		}
		if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "OPTIMIZER_VERSION", Value: "7"}); err != nil {
			t.Errorf("SET LOCAL during commit wait: %v", err)
			return err
		}
		replacement, replAttempt, replHandle = ownerAttemptHandle(h.tm)
		h.tm.mu.RLock()
		if h.tm.tc != nil {
			replIdle = h.tm.tc.idleLastUser
			replRetryAborts = h.tm.tc.retryAborts
		}
		h.tm.mu.RUnlock()
		return nil
	}

	_, err := session.txn.CommitReadWriteTransaction(ctx)
	if !errors.Is(err, errImplicitAbortRetryLostOwner) {
		t.Fatalf("commit lost owner: %v", err)
	}
	if replacement == nil || replacement == original {
		t.Fatal("wait seam did not install a replacement owner")
	}
	gotOwner, gotAttempt, gotHandle := ownerAttemptHandle(h.tm)
	if gotOwner != replacement || gotAttempt != replAttempt || gotHandle != replHandle {
		t.Fatalf("commit completion mutated replacement: %p/%d/%p -> %p/%d/%p",
			replacement, replAttempt, replHandle, gotOwner, gotAttempt, gotHandle)
	}
	h.tm.mu.RLock()
	idle := time.Time{}
	retryAborts := false
	if h.tm.tc != nil {
		idle = h.tm.tc.idleLastUser
		retryAborts = h.tm.tc.retryAborts
	}
	h.tm.mu.RUnlock()
	if idle != replIdle {
		t.Fatalf("replacement idle changed: %v -> %v", replIdle, idle)
	}
	if retryAborts != replRetryAborts {
		t.Fatalf("replacement retryAborts changed: %v -> %v", replRetryAborts, retryAborts)
	}
	if got := mustGetVar(t, session, "OPTIMIZER_VERSION"); got != "7" {
		t.Fatalf("replacement SET LOCAL OPTIMIZER_VERSION = %s", got)
	}
	if !session.txn.InReadWriteTransaction() {
		t.Fatal("commit lost-owner retired the replacement")
	}
}

func TestRetryAbortsExplicitProfileResumableRowRetries(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	session := newRetryAbortsSession(t, h)
	ctx := t.Context()
	beginExplicitRetry(t, ctx, session)
	h.server.setQueryPlan(&sppb.QueryPlan{PlanNodes: hangingIndentPlanNodes()})
	h.server.setSQLRows("SELECT 1", []string{"1", "2"})
	h.server.setStreamingResumeTokens(true)
	h.server.setFailStreamingSQLAfterRows(1, abortedStatus("profile resumable"))
	res, err := executeExplainAnalyze(ctx, session, "SELECT 1", 0, 0, nil)
	if err != nil {
		t.Fatalf("PROFILE resumable retry: %v", err)
	}
	if res == nil || res.AffectedRows != 2 {
		t.Fatalf("PROFILE resumable result: %+v", res)
	}
	if got := countRPC(userSQLObservations(h.server.sqlObservations()), "ExecuteStreamingSql", false); got != 2 {
		t.Fatalf("PROFILE resumable RPCs = %d", got)
	}
}

func TestRetryAbortsExplicitResumablePartialOutputIsNotRetried(t *testing.T) {
	t.Parallel()
	t.Run("csv", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newRetryAbortsSession(t, h)
		ctx := t.Context()
		var out bytes.Buffer
		session.systemVariables.StreamManager = streamio.NewStreamManager(io.NopCloser(strings.NewReader("")), &out, io.Discard)
		mustExec(t, ctx, session, "SET CLI_FORMAT = 'CSV'")
		beginExplicitRetry(t, ctx, session)
		h.server.setSQLRows("SELECT 1", []string{"1", "2"})
		h.server.setStreamingResumeTokens(true)
		h.server.setFailStreamingSQLAfterRows(1, abortedStatus("csv resumable"))
		_, err := execSQL(t, ctx, session, "SELECT 1")
		if err == nil || !isAbortedErr(err) {
			t.Fatalf("CSV resumable: %v", err)
		}
		if got := countRPC(userSQLObservations(h.server.sqlObservations()), "ExecuteStreamingSql", false); got != 1 {
			t.Fatalf("CSV resumable retried: %d out=%q", got, out.String())
		}
	})
	t.Run("vertical", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newRetryAbortsSession(t, h)
		ctx := t.Context()
		var out bytes.Buffer
		session.systemVariables.StreamManager = streamio.NewStreamManager(io.NopCloser(strings.NewReader("")), &out, io.Discard)
		mustExec(t, ctx, session, "SET CLI_FORMAT = 'VERTICAL'")
		beginExplicitRetry(t, ctx, session)
		h.server.setSQLRows("SELECT 1", []string{"1", "2"})
		h.server.setStreamingResumeTokens(true)
		h.server.setFailStreamingSQLAfterRows(1, abortedStatus("vertical resumable"))
		_, err := execSQL(t, ctx, session, "SELECT 1")
		if err == nil || !isAbortedErr(err) {
			t.Fatalf("VERTICAL resumable: %v", err)
		}
		if got := countRPC(userSQLObservations(h.server.sqlObservations()), "ExecuteStreamingSql", false); got != 1 {
			t.Fatalf("VERTICAL resumable retried: %d out=%q", got, out.String())
		}
	})
	t.Run("buffered_table", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newRetryAbortsSession(t, h)
		ctx := t.Context()
		beginExplicitRetry(t, ctx, session)
		session.systemVariables.StreamManager = nil
		session.systemVariables.Display.AutoWrap = false
		h.server.setSQLRows("SELECT 1", []string{"1", "2"})
		h.server.setStreamingResumeTokens(true)
		h.server.setFailStreamingSQLAfterRows(1, abortedStatus("table resumable"))
		res, err := executeSQLImplWithVars(ctx, session, "SELECT 1", session.systemVariables, OperationOutput{})
		if err != nil {
			t.Fatalf("buffered TABLE resumable retry: %v", err)
		}
		if res == nil || res.AffectedRows != 2 {
			t.Fatalf("buffered TABLE result: %+v", res)
		}
		if got := countRPC(userSQLObservations(h.server.sqlObservations()), "ExecuteStreamingSql", false); got != 2 {
			t.Fatalf("buffered TABLE RPCs = %d", got)
		}
	})
}

func TestRetryAbortsExplicitWriterFailureIsNotReceipt(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	session := newRetryAbortsSession(t, h)
	ctx := t.Context()
	beginExplicitRetry(t, ctx, session)
	mustExec(t, ctx, session, "SET CLI_FORMAT = 'CSV'")
	h.server.setSQLRows("SELECT 1", []string{"1"})
	_, err := executeSQLImplWithVars(ctx, session, "SELECT 1", session.systemVariables, OperationOutput{w: errWriter{}})
	if err == nil {
		t.Fatal("writer failure succeeded")
	}
	if isAbortedErr(err) {
		t.Fatalf("writer failure treated as ABORTED: %v", err)
	}
	entries := replayJournal(h.tm)
	if len(entries) != 0 {
		t.Fatalf("writer failure journaled: %+v", entries)
	}
}

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) {
	return 0, io.ErrClosedPipe
}
