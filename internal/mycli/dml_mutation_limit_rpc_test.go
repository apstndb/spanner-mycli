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
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/googleapis/gax-go/v2/apierror"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const mutationLimitDML = "UPDATE T SET v = @v WHERE id = @id"

func newMutationLimitSession(t *testing.T, h *heartbeatHarness) *Session {
	t.Helper()
	session := sessionForTM(t, h.tm)
	h.attachSessionClient(session)
	return session
}

func userSQLObservations(obs []sqlObservation) []sqlObservation {
	var out []sqlObservation
	for _, o := range obs {
		if o.reqTag == "spanner_mycli_heartbeat" {
			continue
		}
		out = append(out, o)
	}
	return out
}

func countRPC(obs []sqlObservation, rpc string, partitioned bool) int {
	var n int
	for _, o := range obs {
		if o.rpc == rpc && o.partitioned == partitioned {
			n++
		}
	}
	return n
}

func TestMutationLimitFallbackSQLPhasePositive(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	session := newMutationLimitSession(t, h)
	ctx := t.Context()
	h.server.setFailStreamingSQL(mutationLimitStatusErr(t))
	h.server.setSQLRowCount(mutationLimitDML, 7)
	mustExec(t, ctx, session, "SET AUTOCOMMIT_DML_MODE = 'TRANSACTIONAL_WITH_FALLBACK_TO_PARTITIONED_NON_ATOMIC'")
	mustExec(t, ctx, session, "SET PARAM v = 1")
	mustExec(t, ctx, session, "SET PARAM id = 2")

	res, err := execSQL(t, ctx, session, mutationLimitDML)
	if err != nil {
		t.Fatalf("fallback: %v", err)
	}
	if !res.IsExecutedDML || !res.MutationLimitFallback || res.AffectedRows != 7 || res.AffectedRowsType != rowCountTypeLowerBound {
		t.Fatalf("result: %+v", res)
	}
	if !res.CommitTimestamp.IsZero() || res.CommitStats != nil {
		t.Fatalf("fallback retained transactional commit metadata: %+v", res)
	}
	if session.systemVariables.LastResult.QueryCache != nil {
		t.Fatalf("fallback published stale QueryCache: %+v", session.systemVariables.LastResult.QueryCache)
	}
	if session.txn.InTransaction() || session.txn.NeedsRecovery() {
		t.Fatal("owner remained after fallback")
	}

	obs := userSQLObservations(h.server.sqlObservations())
	if got := countRPC(obs, "ExecuteStreamingSql", false); got != 1 {
		t.Fatalf("transactional streaming SQL = %d, want 1; obs=%+v", got, obs)
	}
	if got := countRPC(obs, "ExecuteSql", true); got != 1 {
		t.Fatalf("PDML ExecuteSql = %d, want 1; obs=%+v", got, obs)
	}
	if len(h.server.commitIDs()) != 0 {
		t.Fatalf("original Commit RPCs = %v, want none", h.server.commitIDs())
	}
	if len(h.server.rollbackIDs()) == 0 {
		t.Fatal("expected Rollback of the failed owner")
	}

	var stream sqlObservation
	var foundStream bool
	for _, o := range obs {
		if o.rpc == "ExecuteStreamingSql" && !o.partitioned {
			stream = o
			foundStream = true
			break
		}
	}
	if !foundStream {
		t.Fatalf("missing streaming observation: %+v", obs)
	}
	if stream.sql != mutationLimitDML {
		t.Fatalf("streaming SQL = %q", stream.sql)
	}
	var apiErr *apierror.APIError
	if !errors.As(spanner.ToSpannerError(h.server.failStreamingSQL), &apiErr) {
		// The live client path is asserted by success; this pins the fixture
		// status still carries Help after SDK wrapping of the same proto.
		if live := mutationLimitStatusErr(t); !isMutationLimitExceeded(fmtAborted(live)) {
			t.Fatal("fixture Help is not retained after abort wrap")
		}
	}
}

func fmtAborted(err error) error {
	return &wrapErr{prefix: "transaction was aborted: ", err: err}
}

type wrapErr struct {
	prefix string
	err    error
}

func (e *wrapErr) Error() string { return e.prefix + e.err.Error() }
func (e *wrapErr) Unwrap() error { return e.err }

func TestMutationLimitFallbackCommitPhaseNegative(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	session := newMutationLimitSession(t, h)
	ctx := t.Context()
	h.server.setFailCommit(mutationLimitStatusErr(t))
	mustExec(t, ctx, session, "SET AUTOCOMMIT_DML_MODE = 'TRANSACTIONAL_WITH_FALLBACK_TO_PARTITIONED_NON_ATOMIC'")

	_, err := execSQL(t, ctx, session, "UPDATE T SET v = 1 WHERE TRUE")
	if err == nil {
		t.Fatal("expected Commit-phase failure")
	}
	if isMutationLimitExceeded(err) && countRPC(userSQLObservations(h.server.sqlObservations()), "ExecuteSql", true) != 0 {
		t.Fatalf("Commit-phase Help must not start PDML: %v obs=%+v", err, h.server.sqlObservations())
	}
	if countRPC(userSQLObservations(h.server.sqlObservations()), "ExecuteSql", true) != 0 {
		t.Fatalf("PDML after Commit-phase failure: %+v", h.server.sqlObservations())
	}
	if len(h.server.commitIDs()) == 0 {
		t.Fatal("Commit RPC was not attempted")
	}
}

func TestMutationLimitFallbackDefaultAndPDMLModes(t *testing.T) {
	t.Parallel()
	t.Run("default TRANSACTIONAL", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newMutationLimitSession(t, h)
		h.server.setFailStreamingSQL(mutationLimitStatusErr(t))
		_, err := execSQL(t, t.Context(), session, "UPDATE T SET v = 1 WHERE TRUE")
		if err == nil {
			t.Fatal("expected transactional failure")
		}
		if countRPC(userSQLObservations(h.server.sqlObservations()), "ExecuteSql", true) != 0 {
			t.Fatalf("default mode issued PDML: %+v", h.server.sqlObservations())
		}
	})
	t.Run("explicit PARTITIONED_NON_ATOMIC", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newMutationLimitSession(t, h)
		ctx := t.Context()
		h.server.setSQLRowCount("UPDATE T SET v = 1 WHERE TRUE", 3)
		mustExec(t, ctx, session, "SET AUTOCOMMIT_DML_MODE = 'PARTITIONED_NON_ATOMIC'")
		res := mustExec(t, ctx, session, "UPDATE T SET v = 1 WHERE TRUE")
		if res.MutationLimitFallback || res.AffectedRows != 3 || res.AffectedRowsType != rowCountTypeLowerBound {
			t.Fatalf("direct PDML: %+v", res)
		}
		obs := userSQLObservations(h.server.sqlObservations())
		if countRPC(obs, "ExecuteStreamingSql", false) != 0 {
			t.Fatalf("direct PDML used transactional streaming: %+v", obs)
		}
		if countRPC(obs, "ExecuteSql", true) != 1 {
			t.Fatalf("direct PDML ExecuteSql = %d; %+v", countRPC(obs, "ExecuteSql", true), obs)
		}
		if len(h.server.commitIDs()) != 0 {
			t.Fatalf("direct PDML committed a RW txn: %v", h.server.commitIDs())
		}
	})
}

func TestMutationLimitFallbackEligibilityNegatives(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		run  func(t *testing.T, ctx context.Context, session *Session)
	}{
		{"message only", func(t *testing.T, ctx context.Context, session *Session) {
			_, err := execSQL(t, ctx, session, "UPDATE T SET v = 1 WHERE TRUE")
			if err == nil {
				t.Fatal("expected message-only failure")
			}
		}},
		{"insert", func(t *testing.T, ctx context.Context, session *Session) {
			_, err := execSQL(t, ctx, session, "INSERT INTO T (id) VALUES (1)")
			if err == nil {
				t.Fatal("expected INSERT failure")
			}
		}},
		{"returning", func(t *testing.T, ctx context.Context, session *Session) {
			_, err := execSQL(t, ctx, session, "UPDATE T SET v = 1 WHERE TRUE THEN RETURN v")
			if err == nil {
				t.Fatal("expected THEN RETURN failure")
			}
		}},
		{"explicit RW", func(t *testing.T, ctx context.Context, session *Session) {
			mustExec(t, ctx, session, "BEGIN RW")
			_, err := execSQL(t, ctx, session, "UPDATE T SET v = 1 WHERE TRUE")
			if err == nil {
				t.Fatal("expected explicit RW failure")
			}
		}},
		{"pending", func(t *testing.T, ctx context.Context, session *Session) {
			mustExec(t, ctx, session, "BEGIN")
			_, err := execSQL(t, ctx, session, "UPDATE T SET v = 1 WHERE TRUE")
			if err == nil {
				t.Fatal("expected pending-activated failure")
			}
		}},
		{"auto batch enqueue", func(t *testing.T, ctx context.Context, session *Session) {
			mustExec(t, ctx, session, "SET AUTO_BATCH_DML = TRUE")
			mustExec(t, ctx, session, "BEGIN RW")
			res := mustExec(t, ctx, session, "UPDATE T SET v = 1 WHERE TRUE")
			if res.IsExecutedDML || res.MutationLimitFallback {
				t.Fatalf("enqueue executed or fell back: %+v", res)
			}
		}},
		{"manual batch", func(t *testing.T, ctx context.Context, session *Session) {
			mustExec(t, ctx, session, "START BATCH DML")
			res := mustExec(t, ctx, session, "UPDATE T SET v = 1 WHERE TRUE")
			if res.IsExecutedDML || res.MutationLimitFallback {
				t.Fatalf("manual batch executed or fell back: %+v", res)
			}
		}},
		{"savepoint owner", func(t *testing.T, ctx context.Context, session *Session) {
			mustExec(t, ctx, session, "SET CLI_SAVEPOINT_SUPPORT = 'ENABLED'")
			mustExec(t, ctx, session, "BEGIN RW")
			mustExec(t, ctx, session, "SAVEPOINT s1")
			_, err := execSQL(t, ctx, session, "UPDATE T SET v = 1 WHERE TRUE")
			if err == nil {
				t.Fatal("expected SAVEPOINT-owner failure")
			}
		}},
		{"explain analysis", func(t *testing.T, ctx context.Context, session *Session) {
			mustExec(t, ctx, session, "SET CLI_QUERY_MODE = 'PLAN'")
			_, err := execSQL(t, ctx, session, "UPDATE T SET v = 1 WHERE TRUE")
			if err == nil {
				t.Log("PLAN UPDATE returned success; still must not PDML")
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHeartbeatHarness(t)
			session := newMutationLimitSession(t, h)
			ctx := t.Context()
			switch tc.name {
			case "auto batch enqueue", "manual batch":
			case "message only":
				h.server.setFailStreamingSQL(status.Error(codes.InvalidArgument, mutationLimitSentence))
			default:
				h.server.setFailStreamingSQL(mutationLimitStatusErr(t))
			}
			mustExec(t, ctx, session, "SET AUTOCOMMIT_DML_MODE = 'TRANSACTIONAL_WITH_FALLBACK_TO_PARTITIONED_NON_ATOMIC'")
			tc.run(t, ctx, session)
			if countRPC(userSQLObservations(h.server.sqlObservations()), "ExecuteSql", true) != 0 {
				t.Fatalf("PDML issued for %s: %+v", tc.name, h.server.sqlObservations())
			}
		})
	}
}

func TestMutationLimitFallbackFailedPDMLWrapsBoth(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	session := newMutationLimitSession(t, h)
	ctx := t.Context()
	h.server.setFailStreamingSQL(mutationLimitStatusErr(t))
	h.server.setFailUnarySQL(status.Error(codes.InvalidArgument, "unsupported for partitioned DML"))
	mustExec(t, ctx, session, "SET AUTOCOMMIT_DML_MODE = 'TRANSACTIONAL_WITH_FALLBACK_TO_PARTITIONED_NON_ATOMIC'")

	_, err := execSQL(t, ctx, session, "UPDATE T SET v = 1 WHERE TRUE")
	if err == nil {
		t.Fatal("expected wrapped PDML failure")
	}
	if !strings.Contains(err.Error(), "may have partially committed") {
		t.Fatalf("missing partial-commit warning: %v", err)
	}
	if !isMutationLimitExceeded(err) {
		t.Fatalf("original cause lost: %v", err)
	}
	if !strings.Contains(err.Error(), "unsupported for partitioned DML") {
		t.Fatalf("fallback cause lost: %v", err)
	}
	if countRPC(userSQLObservations(h.server.sqlObservations()), "ExecuteSql", true) != 1 {
		t.Fatalf("want exactly one PDML attempt: %+v", h.server.sqlObservations())
	}
}

func TestMutationLimitFallbackNoRecursiveRetry(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	session := newMutationLimitSession(t, h)
	ctx := t.Context()
	h.server.setFailStreamingSQL(mutationLimitStatusErr(t))
	h.server.setFailUnarySQL(mutationLimitStatusErr(t))
	mustExec(t, ctx, session, "SET AUTOCOMMIT_DML_MODE = 'TRANSACTIONAL_WITH_FALLBACK_TO_PARTITIONED_NON_ATOMIC'")

	_, err := execSQL(t, ctx, session, "UPDATE T SET v = 1 WHERE TRUE")
	if err == nil {
		t.Fatal("expected fallback failure")
	}
	if countRPC(userSQLObservations(h.server.sqlObservations()), "ExecuteSql", true) != 1 {
		t.Fatalf("recursive PDML: %+v", h.server.sqlObservations())
	}
}

func TestMutationLimitFallbackFrozenInputs(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	session := newMutationLimitSession(t, h)
	ctx := t.Context()
	h.server.setFailStreamingSQL(mutationLimitStatusErr(t))
	h.server.setSQLRowCount(mutationLimitDML, 4)
	mustExec(t, ctx, session, "SET AUTOCOMMIT_DML_MODE = 'TRANSACTIONAL_WITH_FALLBACK_TO_PARTITIONED_NON_ATOMIC'")
	mustExec(t, ctx, session, "SET PARAM v = 9")
	mustExec(t, ctx, session, "SET PARAM id = 3")
	mustExec(t, ctx, session, "SET STATEMENT_TAG = 'frozen-tag'")
	mustExec(t, ctx, session, "SET RPC_PRIORITY = 'HIGH'")
	mustExec(t, ctx, session, "SET OPTIMIZER_VERSION = '4'")
	mustExec(t, ctx, session, "SET TRANSACTION_TAG = 'must-not-leak'")

	session.txn.afterMutationLimitBeforeFallback = func() {
		mustExec(t, ctx, session, "SET PARAM v = 99")
		mustExec(t, ctx, session, "SET PARAM id = 33")
		session.systemVariables.Transaction.RequestTag = "mutated-tag"
		session.systemVariables.Query.RPCPriority = sppb.RequestOptions_PRIORITY_LOW
		session.systemVariables.Query.OptimizerVersion = "99"
	}

	res := mustExec(t, ctx, session, mutationLimitDML)
	if !res.MutationLimitFallback {
		t.Fatalf("expected fallback: %+v", res)
	}
	var pdml sqlObservation
	var found bool
	for _, o := range userSQLObservations(h.server.sqlObservations()) {
		if o.partitioned {
			pdml = o
			found = true
		}
	}
	if !found {
		t.Fatalf("missing PDML observation: %+v", h.server.sqlObservations())
	}
	if pdml.params["v"] != "9" || pdml.params["id"] != "3" {
		t.Fatalf("PDML params = %v, want frozen v=9 id=3", pdml.params)
	}
	if pdml.reqTag != "frozen-tag" {
		t.Fatalf("PDML RequestTag = %q", pdml.reqTag)
	}
	if pdml.priority != sppb.RequestOptions_PRIORITY_HIGH {
		t.Fatalf("PDML priority = %v", pdml.priority)
	}
	if pdml.optimizer != "4" {
		t.Fatalf("PDML optimizer = %q", pdml.optimizer)
	}
	for _, o := range userSQLObservations(h.server.sqlObservations()) {
		if !o.partitioned {
			continue
		}
		for _, b := range h.server.beginObservations() {
			if b.txnID == o.txnID && b.txnTag == "must-not-leak" {
				t.Fatalf("transaction tag leaked into PDML begin: %+v", h.server.beginObservations())
			}
		}
	}
}

func TestMutationLimitFallbackBudgets(t *testing.T) {
	t.Parallel()
	t.Run("cancel between phases skips PDML", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newMutationLimitSession(t, h)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		h.server.setFailStreamingSQL(mutationLimitStatusErr(t))
		mustExec(t, ctx, session, "SET AUTOCOMMIT_DML_MODE = 'TRANSACTIONAL_WITH_FALLBACK_TO_PARTITIONED_NON_ATOMIC'")
		session.txn.afterMutationLimitBeforeFallback = cancel
		_, err := execSQL(t, ctx, session, "UPDATE T SET v = 1 WHERE TRUE")
		if err == nil || !strings.Contains(err.Error(), "fallback skipped") {
			t.Fatalf("cancel skip: %v", err)
		}
		if countRPC(userSQLObservations(h.server.sqlObservations()), "ExecuteSql", true) != 0 {
			t.Fatalf("canceled fallback still issued PDML: %+v", h.server.sqlObservations())
		}
	})
	t.Run("expired transaction deadline skips PDML", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newMutationLimitSession(t, h)
		ctx := t.Context()
		start := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
		var advanced atomic.Bool
		h.tm.nowFunc = func() time.Time {
			if advanced.Load() {
				return start.Add(3 * time.Second)
			}
			return start
		}
		h.server.setFailStreamingSQL(mutationLimitStatusErr(t))
		mustExec(t, ctx, session, "SET AUTOCOMMIT_DML_MODE = 'TRANSACTIONAL_WITH_FALLBACK_TO_PARTITIONED_NON_ATOMIC'")
		mustExec(t, ctx, session, "SET TRANSACTION_TIMEOUT = '2s'")
		session.txn.afterMutationLimitBeforeFallback = func() { advanced.Store(true) }
		_, err := execSQL(t, ctx, session, "UPDATE T SET v = 1 WHERE TRUE")
		if err == nil || !errors.Is(err, errTransactionTimeout) {
			t.Fatalf("deadline skip: %v", err)
		}
		if countRPC(userSQLObservations(h.server.sqlObservations()), "ExecuteSql", true) != 0 {
			t.Fatalf("expired budget still issued PDML: %+v", h.server.sqlObservations())
		}
	})
	t.Run("remaining transaction deadline and no statement-timeout restart", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := newMutationLimitSession(t, h)
		ctx := t.Context()
		h.server.setFailStreamingSQL(mutationLimitStatusErr(t))
		h.server.setSQLRowCount("UPDATE T SET v = 1 WHERE TRUE", 1)
		mustExec(t, ctx, session, "SET AUTOCOMMIT_DML_MODE = 'TRANSACTIONAL_WITH_FALLBACK_TO_PARTITIONED_NON_ATOMIC'")
		mustExec(t, ctx, session, "SET STATEMENT_TIMEOUT = '45s'")
		mustExec(t, ctx, session, "SET TRANSACTION_TIMEOUT = '20s'")
		res := mustExec(t, ctx, session, "UPDATE T SET v = 1 WHERE TRUE")
		if !res.MutationLimitFallback {
			t.Fatalf("expected fallback: %+v", res)
		}
		var stream, pdml sqlObservation
		for _, o := range userSQLObservations(h.server.sqlObservations()) {
			switch {
			case o.rpc == "ExecuteStreamingSql" && !o.partitioned:
				stream = o
			case o.partitioned:
				pdml = o
			}
		}
		if !stream.hasDeadline || !pdml.hasDeadline {
			t.Fatalf("missing deadlines stream=%+v pdml=%+v", stream, pdml)
		}
		if drift := pdml.deadline.Sub(stream.deadline); drift < -50*time.Millisecond || drift > 50*time.Millisecond {
			t.Fatalf("PDML deadline drifted from captured SQL deadline stream=%v pdml=%v drift=%v", stream.deadline, pdml.deadline, drift)
		}
		remain := time.Until(pdml.deadline)
		if remain > 21*time.Second {
			t.Fatalf("PDML deadline remaining %v looks like a restarted STATEMENT_TIMEOUT or 24h PDML default", remain)
		}
		if remain < 0 {
			t.Fatalf("PDML deadline already expired: %v", pdml.deadline)
		}
	})
}

func TestMutationLimitFallbackRetainedHelpThroughSDK(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	session := newMutationLimitSession(t, h)
	h.server.setFailStreamingSQL(mutationLimitStatusErr(t))
	mustExec(t, t.Context(), session, "SET AUTOCOMMIT_DML_MODE = 'TRANSACTIONAL'")
	_, err := execSQL(t, t.Context(), session, "UPDATE T SET v = 1 WHERE TRUE")
	if err == nil {
		t.Fatal("expected SQL-phase failure")
	}
	if !isMutationLimitExceeded(err) {
		t.Fatalf("live ExecuteStreamingSql error lost Help: %v", err)
	}
	var apiErr *apierror.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("live error has no *apierror.APIError: %T %v", err, err)
	}
	type grpcStatuser interface{ GRPCStatus() *status.Status }
	if gs, ok := err.(grpcStatuser); ok {
		if details := gs.GRPCStatus().Details(); len(details) != 0 {
			t.Fatalf("GRPCStatus unexpectedly retained Help details: %#v", details)
		}
	}
}
