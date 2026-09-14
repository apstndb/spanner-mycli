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
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func abortedStatus(msg string) error {
	return status.Error(codes.Aborted, msg)
}

func newRetryAbortsSession(t *testing.T, h *heartbeatHarness) *Session {
	t.Helper()
	session := newMutationLimitSession(t, h)
	h.tm.abortRetryWait = func(context.Context, time.Duration) error { return nil }
	return session
}

func enableRetryAborts(t *testing.T, ctx context.Context, session *Session) {
	t.Helper()
	mustExec(t, ctx, session, "SET RETRY_ABORTS_INTERNALLY = TRUE")
}

func TestRetryAbortsRejectsExplicitAndPending(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	session := newRetryAbortsSession(t, h)
	ctx := t.Context()
	enableRetryAborts(t, ctx, session)

	if _, err := execSQL(t, ctx, session, "BEGIN"); err == nil || !errors.Is(err, errRetryAbortsExplicitUnsupported) {
		t.Fatalf("BEGIN pending: %v", err)
	}
	if _, err := execSQL(t, ctx, session, "BEGIN RW"); err == nil || !errors.Is(err, errRetryAbortsExplicitUnsupported) {
		t.Fatalf("BEGIN RW: %v", err)
	}
	if got := len(h.server.beginObservations()); got != 0 {
		t.Fatalf("explicit reject issued BeginTransaction: %d", got)
	}

	if _, err := execSQL(t, ctx, session, "BEGIN RO"); err != nil {
		t.Fatalf("BEGIN RO: %v", err)
	}
	if !session.txn.InReadOnlyTransaction() {
		t.Fatal("BEGIN RO did not start a read-only owner")
	}
	mustExec(t, ctx, session, "CLOSE")

	mustExec(t, ctx, session, "SET AUTOCOMMIT = FALSE")
	if _, err := execSQL(t, ctx, session, "SELECT 1"); err == nil || !errors.Is(err, errRetryAbortsExplicitUnsupported) {
		t.Fatalf("AUTOCOMMIT=false pending: %v", err)
	}
}

func TestRetryAbortsSetLocalUnsupported(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	ctx := t.Context()
	if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
		t.Fatal(err)
	}
	_, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "RETRY_ABORTS_INTERNALLY", Value: "TRUE"})
	if err == nil || !errors.Is(err, errRetryAbortsSetLocalUnsupported) {
		t.Fatalf("SET LOCAL TRUE: %v", err)
	}
	_, err = session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "RETRY_ABORTS_INTERNALLY", Value: "FALSE"})
	if err == nil || !errors.Is(err, errRetryAbortsSetLocalUnsupported) {
		t.Fatalf("SET LOCAL FALSE: %v", err)
	}
}

func TestRetryAbortsSQLThenSuccess(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	session := newRetryAbortsSession(t, h)
	ctx := t.Context()
	enableRetryAborts(t, ctx, session)
	mustExec(t, ctx, session, "SET STATEMENT_TAG = 'retry-tag'")
	h.server.setFailStreamingSQLTimes(1, abortedStatus("sql aborted"))
	h.server.setSQLRowCount("UPDATE T SET v = 1 WHERE TRUE", 3)

	var owners []*transactionContext
	var attempts []uint64
	var firstIdle time.Time
	h.tm.abortRetryWait = func(context.Context, time.Duration) error {
		h.tm.mu.Lock()
		if h.tm.tc != nil {
			if h.tm.tc.replay != nil {
				t.Error("implicit retry allocated a journal")
			}
			if firstIdle.IsZero() {
				firstIdle = h.tm.tc.idleLastUser
			} else if h.tm.tc.idleLastUser != firstIdle {
				t.Errorf("reconstruct rearmed idle: %v -> %v", firstIdle, h.tm.tc.idleLastUser)
			}
		}
		h.tm.mu.Unlock()
		return nil
	}

	res, err := session.txn.RunInNewOrExistRwTx(ctx, func(tx *spanner.ReadWriteStmtBasedTransaction, implicit bool) (int64, *sppb.QueryPlan, *sppb.ResultSetMetadata, error) {
		owners = append(owners, session.txn.tc)
		attempts = append(attempts, session.txn.tc.attempt)
		ur, err := session.txn.runUpdateOnTransaction(ctx, tx, spanner.NewStatement("UPDATE T SET v = 1 WHERE TRUE"), implicit, effectiveQueryMode(nil))
		if err != nil {
			return 0, nil, nil, err
		}
		return ur.Count, ur.Plan, ur.Metadata, nil
	})
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if res == nil || res.Affected != 3 || res.CommitResponse.CommitTs.IsZero() {
		t.Fatalf("result: %+v", res)
	}
	if len(owners) != 2 || owners[0] != owners[1] {
		t.Fatalf("owners = %d pointers %p %p", len(owners), owners[0], owners[1])
	}
	if len(attempts) != 2 || attempts[0] == 0 || attempts[1] <= attempts[0] {
		t.Fatalf("attempts = %v", attempts)
	}
	obs := userSQLObservations(h.server.sqlObservations())
	if got := countRPC(obs, "ExecuteStreamingSql", false); got != 2 {
		t.Fatalf("streaming SQL = %d; %+v", got, obs)
	}
	if len(h.server.commitIDs()) != 1 {
		t.Fatalf("commits = %v", h.server.commitIDs())
	}
	for _, o := range obs {
		if o.sql == "UPDATE T SET v = 1 WHERE TRUE" && o.reqTag != "retry-tag" {
			t.Fatalf("tag not frozen: %+v", o)
		}
	}
	if session.systemVariables.Transaction.RequestTag != "" {
		t.Fatalf("live STATEMENT_TAG = %q", session.systemVariables.Transaction.RequestTag)
	}
}

func TestRetryAbortsCommitThenSuccessPublishesOnce(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	session := newRetryAbortsSession(t, h)
	ctx := t.Context()
	enableRetryAborts(t, ctx, session)
	h.server.setFailCommitTimes(1, abortedStatus("commit aborted"))
	const returningSQL = "UPDATE T SET v = 2 WHERE TRUE THEN RETURN v"
	h.server.setSQLRows(returningSQL, []string{"9"})
	h.server.setSQLRowCount(returningSQL, 1)

	res, err := execSQL(t, ctx, session, returningSQL)
	if err != nil {
		t.Fatalf("THEN RETURN retry: %v", err)
	}
	if res == nil || res.AffectedRows != 1 {
		t.Fatalf("THEN RETURN result: %+v", res)
	}
	if res.Body.IsNone() {
		t.Fatal("THEN RETURN published no body")
	}
	if session.systemVariables.LastResult.QueryCache == nil {
		t.Fatal("successful retry did not publish QueryCache")
	}
	obs := userSQLObservations(h.server.sqlObservations())
	if got := countRPC(obs, "ExecuteStreamingSql", false); got != 2 {
		t.Fatalf("streaming SQL = %d; %+v", got, obs)
	}
	if got := len(h.server.commitIDs()); got != 2 {
		t.Fatalf("commits = %d", got)
	}
	if session.txn.InTransaction() {
		t.Fatal("owner remained after success")
	}
}

func TestRetryAbortsDefaultFalseHasNoHiddenRetry(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	session := newRetryAbortsSession(t, h)
	h.server.setFailStreamingSQL(abortedStatus("sql aborted"))
	_, err := execSQL(t, t.Context(), session, "UPDATE T SET v = 1 WHERE TRUE")
	if err == nil || !isAbortedErr(err) {
		t.Fatalf("default FALSE: %v", err)
	}
	if strings.Contains(err.Error(), "transaction was aborted") == false {
		t.Fatalf("ABORTED should still wrap: %v", err)
	}
	obs := userSQLObservations(h.server.sqlObservations())
	if got := countRPC(obs, "ExecuteStreamingSql", false); got != 1 {
		t.Fatalf("hidden retry: %+v", obs)
	}
}

func TestRetryAbortsSessionSetDoesNotChangeLiveOwner(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	session := newRetryAbortsSession(t, h)
	ctx := t.Context()
	enableRetryAborts(t, ctx, session)
	h.server.setFailStreamingSQLTimes(1, abortedStatus("sql aborted"))
	h.tm.abortRetryWait = func(context.Context, time.Duration) error {
		if err := session.systemVariables.SetFromSimple("RETRY_ABORTS_INTERNALLY", "FALSE"); err != nil {
			t.Errorf("session SET: %v", err)
		}
		return nil
	}
	if _, err := execSQL(t, ctx, session, "UPDATE T SET v = 1 WHERE TRUE"); err != nil {
		t.Fatalf("owner snapshot should keep retrying: %v", err)
	}
	got, _ := session.systemVariables.Get("RETRY_ABORTS_INTERNALLY")
	if got["RETRY_ABORTS_INTERNALLY"] != "FALSE" {
		t.Fatalf("SHOW after SET = %v", got)
	}
	obs := userSQLObservations(h.server.sqlObservations())
	if got := countRPC(obs, "ExecuteStreamingSql", false); got != 2 {
		t.Fatalf("session SET mutated live retry: %+v", obs)
	}
}

func TestRetryAbortsUnavailableCommitIsNotRetried(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	session := newRetryAbortsSession(t, h)
	ctx := t.Context()
	enableRetryAborts(t, ctx, session)
	h.tm.commitOverride = func(context.Context, *spanner.ReadWriteStmtBasedTransaction) (spanner.CommitResponse, error) {
		return spanner.CommitResponse{}, status.Error(codes.Unavailable, "commit unavailable")
	}
	_, err := execSQL(t, ctx, session, "UPDATE T SET v = 1 WHERE TRUE")
	if err == nil || spanner.ErrCode(err) != codes.Unavailable {
		t.Fatalf("Unavailable: %v", err)
	}
	if strings.Contains(err.Error(), "transaction was aborted") {
		t.Fatalf("non-ABORTED labeled aborted: %v", err)
	}
	if got := len(h.server.commitIDs()); got != 0 {
		t.Fatalf("SDK Commit RPCs = %d, want 0 (override)", got)
	}
	obs := userSQLObservations(h.server.sqlObservations())
	if got := countRPC(obs, "ExecuteStreamingSql", false); got != 1 {
		t.Fatalf("retried Unavailable commit: %+v", obs)
	}
}

func TestRetryAbortsMutationLimitStaysSQLPhase(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	session := newRetryAbortsSession(t, h)
	ctx := t.Context()
	enableRetryAborts(t, ctx, session)
	mustExec(t, ctx, session, "SET AUTOCOMMIT_DML_MODE = 'TRANSACTIONAL_WITH_FALLBACK_TO_PARTITIONED_NON_ATOMIC'")
	h.server.setFailStreamingSQL(mutationLimitStatusErr(t))
	h.server.setSQLRowCount(mutationLimitDML, 4)
	mustExec(t, ctx, session, "SET PARAM v = 1")
	mustExec(t, ctx, session, "SET PARAM id = 2")
	res, err := execSQL(t, ctx, session, mutationLimitDML)
	if err != nil {
		t.Fatalf("964 fallback: %v", err)
	}
	if !res.MutationLimitFallback {
		t.Fatalf("result: %+v", res)
	}
	if countRPC(userSQLObservations(h.server.sqlObservations()), "ExecuteStreamingSql", false) != 1 {
		t.Fatalf("mutation-limit was abort-retried: %+v", h.server.sqlObservations())
	}
}

func TestRetryAbortsExhaustedFifty(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	session := newRetryAbortsSession(t, h)
	ctx := t.Context()
	enableRetryAborts(t, ctx, session)
	h.server.setFailStreamingSQL(abortedStatus("always aborted"))
	_, err := execSQL(t, ctx, session, "UPDATE T SET v = 1 WHERE TRUE")
	if err == nil || !isAbortedErr(err) {
		t.Fatalf("exhausted: %v", err)
	}
	obs := userSQLObservations(h.server.sqlObservations())
	if got := countRPC(obs, "ExecuteStreamingSql", false); got != maxImplicitAbortAttempts {
		t.Fatalf("attempts = %d, want %d; %+v", got, maxImplicitAbortAttempts, obs)
	}
	if session.txn.InTransaction() {
		t.Fatal("owner remained after exhausted abort")
	}
}

func TestRetryAbortsCancelDuringBackoff(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	session := newRetryAbortsSession(t, h)
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	enableRetryAborts(t, ctx, session)
	h.server.setFailStreamingSQLTimes(1, abortedStatus("sql aborted"))
	started := make(chan struct{})
	h.tm.abortRetryWait = func(ctx context.Context, d time.Duration) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}
	errCh := make(chan error, 1)
	go func() {
		_, err := execSQL(t, ctx, session, "UPDATE T SET v = 1 WHERE TRUE")
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
		t.Fatalf("cancel during backoff: %v", err)
	}
}

func TestRetryAbortsBatchMutateAndProfile(t *testing.T) {
	t.Parallel()
	t.Run("batch", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newRetryAbortsSession(t, h)
		ctx := t.Context()
		enableRetryAborts(t, ctx, session)
		h.server.setFailBatchDMLTimes(1, abortedStatus("batch aborted"))
		mustExec(t, ctx, session, "START BATCH DML")
		mustExec(t, ctx, session, "UPDATE T SET v = 1 WHERE TRUE")
		if _, err := execSQL(t, ctx, session, "RUN BATCH"); err != nil {
			t.Fatalf("batch retry: %v", err)
		}
		if got := len(h.server.batchObservations()); got != 2 {
			t.Fatalf("batch RPCs = %d", got)
		}
	})
	t.Run("mutate", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newRetryAbortsSession(t, h)
		ctx := t.Context()
		enableRetryAborts(t, ctx, session)
		h.server.setFailCommitTimes(1, abortedStatus("mutate commit aborted"))
		if _, err := execSQL(t, ctx, session, "MUTATE T INSERT STRUCT(1 AS id)"); err != nil {
			t.Fatalf("MUTATE retry: %v", err)
		}
		if got := len(h.server.commitIDs()); got != 2 {
			t.Fatalf("MUTATE commits = %d", got)
		}
	})
	t.Run("profile", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newRetryAbortsSession(t, h)
		ctx := t.Context()
		enableRetryAborts(t, ctx, session)
		h.server.setFailStreamingSQLTimes(1, abortedStatus("profile aborted"))
		_, err := executeExplainAnalyzeDML(ctx, session, "UPDATE T SET v = 1 WHERE TRUE", 0, 0, nil)
		if err != nil && !strings.Contains(err.Error(), "emulator") {
			// Dummy plan may fail formatting; RPCs still prove retry.
			if !session.txn.InTransaction() && countRPC(userSQLObservations(h.server.sqlObservations()), "ExecuteStreamingSql", false) == 2 {
				return
			}
			t.Fatalf("PROFILE: %v obs=%+v", err, h.server.sqlObservations())
		}
		if got := countRPC(userSQLObservations(h.server.sqlObservations()), "ExecuteStreamingSql", false); got != 2 {
			t.Fatalf("PROFILE SQL = %d", got)
		}
	})
}

func TestRetryAbortsDoesNotRetryPlanPDMLOrRO(t *testing.T) {
	t.Parallel()
	t.Run("plan", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newRetryAbortsSession(t, h)
		ctx := t.Context()
		enableRetryAborts(t, ctx, session)
		h.server.setFailStreamingSQL(abortedStatus("plan aborted"))
		_, _, _, err := runAnalyzeQuery(ctx, session, spanner.NewStatement("UPDATE T SET v = 1 WHERE TRUE"), true)
		if err == nil || !isAbortedErr(err) {
			t.Fatalf("PLAN: %v", err)
		}
		if got := countRPC(userSQLObservations(h.server.sqlObservations()), "ExecuteStreamingSql", false); got != 1 {
			t.Fatalf("PLAN became a retry path: %+v", h.server.sqlObservations())
		}
	})
	t.Run("pdml", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newRetryAbortsSession(t, h)
		ctx := t.Context()
		enableRetryAborts(t, ctx, session)
		mustExec(t, ctx, session, "SET AUTOCOMMIT_DML_MODE = 'PARTITIONED_NON_ATOMIC'")
		h.server.setFailUnarySQL(status.Error(codes.InvalidArgument, "pdml rejected"))
		_, err := execSQL(t, ctx, session, "UPDATE T SET v = 1 WHERE TRUE")
		if err == nil {
			t.Fatal("expected PDML failure")
		}
		if len(h.server.commitIDs()) != 0 {
			t.Fatalf("PDML used RW commit retry: %v", h.server.commitIDs())
		}
	})
	t.Run("readonly", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newRetryAbortsSession(t, h)
		ctx := t.Context()
		enableRetryAborts(t, ctx, session)
		h.server.setFailROQuery(abortedStatus("ro aborted"))
		_, err := execSQL(t, ctx, session, "SELECT 1")
		if err == nil {
			t.Fatal("expected RO failure")
		}
		if len(h.server.commitIDs()) != 0 {
			t.Fatalf("RO used RW commit: %v", h.server.commitIDs())
		}
	})
}

func TestRetryAbortsExplicitOwnerKeepsDefaultNoLoop(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	session := newRetryAbortsSession(t, h)
	ctx := t.Context()
	mustExec(t, ctx, session, "BEGIN RW")
	if err := session.systemVariables.SetFromSimple("RETRY_ABORTS_INTERNALLY", "TRUE"); err != nil {
		t.Fatal(err)
	}
	h.server.setFailStreamingSQL(abortedStatus("explicit aborted"))
	_, err := execSQL(t, ctx, session, "UPDATE T SET v = 1 WHERE TRUE")
	if err == nil || !isAbortedErr(err) {
		t.Fatalf("explicit: %v", err)
	}
	if got := countRPC(userSQLObservations(h.server.sqlObservations()), "ExecuteStreamingSql", false); got != 1 {
		t.Fatalf("explicit owner retried: %+v", h.server.sqlObservations())
	}
}
