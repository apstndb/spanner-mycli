// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mycli

import (
	"errors"
	"slices"
	"strings"
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/enums"
)

func newAutocommitRPCSession(t *testing.T) (*heartbeatHarness, *Session) {
	t.Helper()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	h.attachSessionClient(session)
	if err := session.systemVariables.CaptureStartupSnapshots(); err != nil {
		t.Fatal(err)
	}
	return h, session
}

func assertIdle(t *testing.T, session *Session, msg string) {
	t.Helper()
	if session.txn != nil && session.txn.InTransaction() {
		t.Fatal(msg)
	}
}

func TestAutocommitTrueIdleSelectCreatesNoOwner(t *testing.T) {
	t.Parallel()
	h, session := newAutocommitRPCSession(t)
	ctx := t.Context()

	if got := mustGetVar(t, session, "AUTOCOMMIT"); got != "TRUE" {
		t.Fatalf("default AUTOCOMMIT = %q, want TRUE", got)
	}
	mustExec(t, ctx, session, "SELECT 1")
	assertIdle(t, session, "idle AUTOCOMMIT=true SELECT created a logical owner")
	if n := len(h.server.commits); n != 0 {
		t.Fatalf("idle SELECT commits = %d, want 0", n)
	}
}

func TestAutocommitFalseGroupsUntilCommitThenStaysIdle(t *testing.T) {
	t.Parallel()
	h, session := newAutocommitRPCSession(t)
	ctx := t.Context()

	mustExec(t, ctx, session, "SET AUTOCOMMIT = FALSE")
	mustExec(t, ctx, session, "SELECT 1")
	if !session.txn.InTransaction() {
		t.Fatal("AUTOCOMMIT=false SELECT must create a logical owner")
	}
	if n := len(h.server.commits); n != 0 {
		t.Fatalf("grouped SELECT committed early: %d", n)
	}

	mustExec(t, ctx, session, "INSERT INTO T (id) VALUES (1)")
	if !session.txn.InReadWriteTransaction() && !session.txn.InPendingTransaction() && !session.txn.InTransaction() {
		t.Fatal("ordinary DML under AUTOCOMMIT=false must stay on the owner")
	}
	if n := len(h.server.commits); n != 0 {
		t.Fatalf("grouped DML committed before COMMIT: %d", n)
	}

	mustExec(t, ctx, session, "COMMIT")
	assertIdle(t, session, "COMMIT must leave the session idle")
	if n := len(h.server.commits); n != 1 {
		t.Fatalf("COMMIT RPC count = %d, want 1", n)
	}

	mustExec(t, ctx, session, "SHOW VARIABLES")
	assertIdle(t, session, "SHOW VARIABLES after COMMIT must not create an owner")

	mustExec(t, ctx, session, "SELECT 1")
	if !session.txn.InTransaction() {
		t.Fatal("next eligible statement after COMMIT must create a new owner")
	}
}

func TestAutocommitFalseSameValueSetAndResetWithOwner(t *testing.T) {
	t.Parallel()
	_, session := newAutocommitRPCSession(t)
	ctx := t.Context()

	mustExec(t, ctx, session, "SET AUTOCOMMIT = FALSE")
	mustExec(t, ctx, session, "SELECT 1")
	if !session.txn.InTransaction() {
		t.Fatal("expected owner")
	}

	mustExec(t, ctx, session, "SET AUTOCOMMIT = FALSE")
	if err := session.systemVariables.Reset("AUTOCOMMIT"); err == nil || !errors.Is(err, errSetterInTransaction) {
		t.Fatalf("RESET toggle with owner: %v, want %v", err, errSetterInTransaction)
	}
	if got := mustGetVar(t, session, "AUTOCOMMIT"); got != "FALSE" {
		t.Fatalf("rejected RESET mutated AUTOCOMMIT = %q", got)
	}

	err := session.systemVariables.SetFromSimple("AUTOCOMMIT", "TRUE")
	if !errors.Is(err, errSetterInTransaction) {
		t.Fatalf("toggle SET with owner: %v, want %v", err, errSetterInTransaction)
	}
}

func TestAutocommitFalseManualBatchToggleAndRun(t *testing.T) {
	t.Parallel()
	h, session := newAutocommitRPCSession(t)
	ctx := t.Context()

	mustExec(t, ctx, session, "SET AUTOCOMMIT = FALSE")
	mustExec(t, ctx, session, "START BATCH DML")
	assertIdle(t, session, "START BATCH DML must not create an owner")

	mustExec(t, ctx, session, "SET AUTOCOMMIT = FALSE")
	err := session.systemVariables.SetFromSimple("AUTOCOMMIT", "TRUE")
	if !errors.Is(err, errSetterInManualBatch) {
		t.Fatalf("toggle during batch: %v, want %v", err, errSetterInManualBatch)
	}

	mustExec(t, ctx, session, "INSERT INTO T (id) VALUES (1)")
	assertIdle(t, session, "manual-batch DML enqueue must not create an owner")

	mustExec(t, ctx, session, "RUN BATCH")
	if !session.txn.InTransaction() {
		t.Fatal("nonempty RUN BATCH DML must acquire an owner")
	}
	if n := len(h.server.commits); n != 0 {
		t.Fatalf("RUN BATCH under AUTOCOMMIT=false committed: %d", n)
	}
	mustExec(t, ctx, session, "COMMIT")
	if n := len(h.server.commits); n != 1 {
		t.Fatalf("COMMIT after RUN BATCH = %d, want 1", n)
	}
}

func TestAutocommitFalseEmptyRunBatchDoesNotAcquireOwner(t *testing.T) {
	t.Parallel()
	_, session := newAutocommitRPCSession(t)
	ctx := t.Context()

	mustExec(t, ctx, session, "SET AUTOCOMMIT = FALSE")
	_, err := execSQL(t, ctx, session, "RUN BATCH")
	if err == nil || !strings.Contains(err.Error(), "no active batch") {
		t.Fatalf("empty RUN BATCH: %v, want no active batch", err)
	}
	assertIdle(t, session, "empty RUN BATCH acquired an owner")
}

func TestAutocommitFalseSavepointAdmission(t *testing.T) {
	t.Parallel()
	_, session := newAutocommitRPCSession(t)
	ctx := t.Context()

	mustExec(t, ctx, session, "SET AUTOCOMMIT = FALSE")
	_, err := execSQL(t, ctx, session, "SAVEPOINT keep")
	if !errors.Is(err, errSavepointDisabled) {
		t.Fatalf("disabled SAVEPOINT: %v, want %v", err, errSavepointDisabled)
	}
	assertIdle(t, session, "disabled SAVEPOINT acquired an owner")

	_, err = session.ExecuteStatement(ctx, &SavepointStatement{})
	if !errors.Is(err, errSavepointEmptyName) {
		t.Fatalf("invalid SAVEPOINT: %v, want %v", err, errSavepointEmptyName)
	}
	assertIdle(t, session, "invalid SAVEPOINT acquired an owner")

	mustExec(t, ctx, session, "SET CLI_SAVEPOINT_SUPPORT = 'ENABLED'")
	mustExec(t, ctx, session, "SAVEPOINT keep")
	if !session.txn.InTransaction() {
		t.Fatal("enabled SAVEPOINT must acquire an owner")
	}
}

func TestAutocommitFalseReadonlyAndInspection(t *testing.T) {
	t.Parallel()
	h, session := newAutocommitRPCSession(t)
	ctx := t.Context()

	mustExec(t, ctx, session, "SET AUTOCOMMIT = FALSE")
	mustExec(t, ctx, session, "SET READONLY = TRUE")
	_, err := execSQL(t, ctx, session, "INSERT INTO T (id) VALUES (1)")
	if !errors.Is(err, errReadOnly) {
		t.Fatalf("READONLY DML: %v, want %v", err, errReadOnly)
	}
	assertIdle(t, session, "READONLY DML acquired an owner")
	if n := len(h.server.begins); n != 0 {
		t.Fatalf("READONLY DML began a txn: %d", n)
	}

	mustExec(t, ctx, session, "SELECT 1")
	if !session.txn.InTransaction() {
		t.Fatal("READONLY SELECT must still create an owner")
	}
	if !session.txn.InReadOnlyTransaction() {
		t.Fatal("READONLY capture must resolve the owner as RO")
	}

	mustExec(t, ctx, session, "SHOW TRANSACTION READ ONLY")
	if !session.txn.InTransaction() {
		t.Fatal("SHOW TRANSACTION must keep the existing owner")
	}
}

func TestAutocommitFalseExplainPlanDoesNotLeaveOwner(t *testing.T) {
	t.Parallel()
	h, session := newAutocommitRPCSession(t)
	ctx := t.Context()

	mustExec(t, ctx, session, "SET AUTOCOMMIT = FALSE")

	_, err := execSQL(t, ctx, session, "EXPLAIN SELECT 1")
	if err == nil || !strings.Contains(err.Error(), "EXPLAIN statement is not supported for Cloud Spanner Emulator") {
		t.Fatalf("EXPLAIN PLAN SELECT: %v, want emulator plan rejection after the PLAN RPC", err)
	}
	assertIdle(t, session, "EXPLAIN PLAN SELECT left an owner")
	selectObs := requireUserSQL(t, h, "SELECT 1")
	if selectObs.queryMode != sppb.ExecuteSqlRequest_PLAN {
		t.Fatalf("EXPLAIN PLAN SELECT QueryMode = %v, want PLAN", selectObs.queryMode)
	}
	if selectObs.partitioned {
		t.Fatal("EXPLAIN PLAN SELECT used a PDML selector")
	}
	if n := len(h.server.commits); n != 0 {
		t.Fatalf("EXPLAIN PLAN SELECT commits = %d, want 0", n)
	}

	const planDML = "UPDATE T SET id = 1 WHERE true"
	_, err = execSQL(t, ctx, session, "EXPLAIN "+planDML)
	if err == nil || !strings.Contains(err.Error(), "EXPLAIN statement is not supported for Cloud Spanner Emulator") {
		t.Fatalf("EXPLAIN PLAN DML: %v, want emulator plan rejection after the PLAN RPC", err)
	}
	assertIdle(t, session, "EXPLAIN PLAN DML must not leave an owner")
	dmlObs := requireUserSQL(t, h, planDML)
	if dmlObs.queryMode != sppb.ExecuteSqlRequest_PLAN {
		t.Fatalf("EXPLAIN PLAN DML QueryMode = %v, want PLAN", dmlObs.queryMode)
	}
	if dmlObs.partitioned {
		t.Fatal("EXPLAIN PLAN DML used a PDML selector")
	}
	if n := len(h.server.commits); n != 1 {
		t.Fatalf("idle EXPLAIN PLAN DML commits = %d, want 1 implicit one-shot Commit", n)
	}
	if pdml := pdmlBeginCount(h); pdml != 0 {
		t.Fatalf("EXPLAIN PLAN DML began PDML: %d", pdml)
	}
}

func TestAutocommitFalseExplainAnalyzeAcquiresOwner(t *testing.T) {
	t.Parallel()
	_, session := newAutocommitRPCSession(t)
	ctx := t.Context()

	mustExec(t, ctx, session, "SET AUTOCOMMIT = FALSE")
	_, err := execSQL(t, ctx, session, "EXPLAIN ANALYZE SELECT 1")
	if err != nil && !errors.Is(err, errExplainAnalyzeUnsupportedOnEmulator) {
		t.Fatalf("EXPLAIN ANALYZE: %v", err)
	}
	if !session.txn.InTransaction() {
		t.Fatal("EXPLAIN ANALYZE must acquire an owner under AUTOCOMMIT=false")
	}
}

func TestAutocommitFalseOrdinaryDMLDoesNotUsePDMLOrFallback(t *testing.T) {
	t.Parallel()
	h, session := newAutocommitRPCSession(t)
	ctx := t.Context()

	mustExec(t, ctx, session, "SET AUTOCOMMIT = FALSE")
	mustExec(t, ctx, session, "SET AUTOCOMMIT_DML_MODE = 'PARTITIONED_NON_ATOMIC'")
	mustExec(t, ctx, session, "UPDATE T SET id = 1 WHERE true")
	if !session.txn.InTransaction() {
		t.Fatal("ordinary DML under AUTOCOMMIT=false must use the logical owner")
	}
	h.server.mu.Lock()
	pdml := len(h.server.pdmlIDs)
	h.server.mu.Unlock()
	if pdml != 0 {
		t.Fatalf("ordinary DML used PDML: %d partitioned txns", pdml)
	}
	if n := len(h.server.commits); n != 0 {
		t.Fatalf("ordinary DML committed via autocommit path: %d", n)
	}

	session.systemVariables.Transaction.AutocommitDMLMode = enums.AutocommitDMLModeTransactionalWithFallbackToPartitionedNonAtomic
	if attempt := captureMutationLimitFallback(session, "UPDATE T SET id = 2 WHERE true"); attempt != nil {
		t.Fatal("pending/false-mode owner must suppress mutation-limit fallback")
	}
}

func TestAutocommitFalseExplicitPDMLAndTruncateStayOutside(t *testing.T) {
	t.Parallel()
	h, session := newAutocommitRPCSession(t)
	ctx := t.Context()

	mustExec(t, ctx, session, "SET AUTOCOMMIT = FALSE")

	const pdmlSQL = "UPDATE T SET id = 1 WHERE true"
	res, err := execSQL(t, ctx, session, "PARTITIONED "+pdmlSQL)
	if err != nil {
		t.Fatalf("PARTITIONED UPDATE: %v", err)
	}
	if !res.IsExecutedDML {
		t.Fatalf("PARTITIONED UPDATE result = %+v, want executed DML", res)
	}
	assertIdle(t, session, "explicit PDML created a session owner")
	pdmlObs := requireUserSQL(t, h, pdmlSQL)
	if !pdmlObs.partitioned {
		t.Fatalf("PARTITIONED UPDATE selector = %+v, want PDML", pdmlObs)
	}
	if pdmlBeginCount(h) == 0 {
		t.Fatalf("PARTITIONED UPDATE issued no PDML Begin: begins=%v sql=%+v", h.server.beginObservations(), h.server.sqlObservations())
	}

	res, err = execSQL(t, ctx, session, "TRUNCATE TABLE T")
	if err != nil {
		t.Fatalf("TRUNCATE TABLE: %v", err)
	}
	if !res.IsExecutedDML {
		t.Fatalf("TRUNCATE result = %+v, want executed DML", res)
	}
	assertIdle(t, session, "TRUNCATE created a session owner")
	truncObs, ok := lastPartitionedUserSQL(h)
	if !ok || !strings.Contains(truncObs.sql, "DELETE FROM") || !strings.Contains(truncObs.sql, "T") {
		t.Fatalf("TRUNCATE PDML SQL = %+v, want partitioned DELETE FROM T", truncObs)
	}
	if n := len(h.server.commits); n != 0 {
		t.Fatalf("PDML/TRUNCATE used RW Commit: %d", n)
	}
	if pdmlBeginCount(h) < 2 {
		t.Fatalf("PDML Begin count = %d, want at least 2 (PARTITIONED UPDATE + TRUNCATE)", pdmlBeginCount(h))
	}
}

func TestAutocommitFalseManualBatchOrdinaryVsProfileDML(t *testing.T) {
	t.Parallel()

	t.Run("ordinary enqueue", func(t *testing.T) {
		t.Parallel()
		h, session := newAutocommitRPCSession(t)
		ctx := t.Context()
		mustExec(t, ctx, session, "SET AUTOCOMMIT = FALSE")
		mustExec(t, ctx, session, "START BATCH DML")
		assertIdle(t, session, "START BATCH DML must not create an owner")

		const insertSQL = "INSERT INTO T (id) VALUES (1)"
		res := mustExec(t, ctx, session, insertSQL)
		if res.IsExecutedDML {
			t.Fatal("ordinary manual-batch DML reported execution")
		}
		assertIdle(t, session, "ordinary manual-batch DML enqueue must not create an owner")
		assertManualBatchDMLSize(t, session, 1)
		if _, ok := lastSQLObservation(userSQLObservations(h.server.sqlObservations()), insertSQL); ok {
			t.Fatalf("ordinary manual-batch DML executed: %+v", h.server.sqlObservations())
		}
		if n := len(h.server.commits); n != 0 {
			t.Fatalf("ordinary manual-batch DML commits = %d, want 0", n)
		}
		if pdml := pdmlBeginCount(h); pdml != 0 {
			t.Fatalf("ordinary manual-batch DML began PDML: %d", pdml)
		}
	})

	t.Run("PROFILE executes and joins owner", func(t *testing.T) {
		t.Parallel()
		h, session := newAutocommitRPCSession(t)
		ctx := t.Context()
		mustExec(t, ctx, session, "SET AUTOCOMMIT = FALSE")
		mustExec(t, ctx, session, "START BATCH DML")
		mustExec(t, ctx, session, "SET CLI_QUERY_MODE = 'PROFILE'")

		const updateSQL = "UPDATE T SET id = 1 WHERE true"
		_, err := execSQL(t, ctx, session, updateSQL)
		if !errors.Is(err, errExplainAnalyzeUnsupportedOnEmulator) {
			t.Fatalf("PROFILE DML in a manual batch: %v, want %v after execution", err, errExplainAnalyzeUnsupportedOnEmulator)
		}
		if !session.txn.InTransaction() {
			t.Fatal("PROFILE DML in a manual batch must join an owner")
		}
		assertManualBatchDMLSize(t, session, 0)
		obs := requireUserSQL(t, h, updateSQL)
		if obs.queryMode != sppb.ExecuteSqlRequest_PROFILE {
			t.Fatalf("PROFILE DML QueryMode = %v, want PROFILE", obs.queryMode)
		}
		if obs.partitioned {
			t.Fatal("PROFILE DML used a PDML selector")
		}
		if n := len(h.server.commits); n != 0 {
			t.Fatalf("PROFILE DML in a manual batch committed: %d", n)
		}
		if pdml := pdmlBeginCount(h); pdml != 0 {
			t.Fatalf("PROFILE DML in a manual batch began PDML: %d", pdml)
		}
	})
}

func requireUserSQL(t *testing.T, h *heartbeatHarness, sql string) sqlObservation {
	t.Helper()
	obs, ok := lastSQLObservation(userSQLObservations(h.server.sqlObservations()), sql)
	if !ok {
		t.Fatalf("missing user SQL %q: %+v", sql, h.server.sqlObservations())
	}
	return obs
}

func lastPartitionedUserSQL(h *heartbeatHarness) (sqlObservation, bool) {
	obs := userSQLObservations(h.server.sqlObservations())
	for _, ob := range slices.Backward(obs) {
		if ob.partitioned {
			return ob, true
		}
	}
	return sqlObservation{}, false
}

func assertManualBatchDMLSize(t *testing.T, session *Session, want int) {
	t.Helper()
	b, ok := session.batch.Current().(*BatchDMLStatement)
	if !ok {
		t.Fatalf("batch.Current() = %T, want *BatchDMLStatement", session.batch.Current())
	}
	if got := len(b.DMLs); got != want {
		t.Fatalf("manual batch size = %d, want %d", got, want)
	}
}

func pdmlBeginCount(h *heartbeatHarness) int {
	h.server.mu.Lock()
	defer h.server.mu.Unlock()
	return len(h.server.pdmlIDs)
}

func TestAutocommitFalseCloseDoesNotCommit(t *testing.T) {
	t.Parallel()
	h, session := newAutocommitRPCSession(t)
	ctx := t.Context()

	mustExec(t, ctx, session, "SET AUTOCOMMIT = FALSE")
	mustExec(t, ctx, session, "SET CLI_VERBOSE = FALSE")
	mustExec(t, ctx, session, "SELECT 1")
	mustExec(t, ctx, session, "SET LOCAL CLI_VERBOSE = TRUE")
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "TRUE" {
		t.Fatalf("LOCAL CLI_VERBOSE = %q, want TRUE", got)
	}

	session.Close()
	if n := len(h.server.commits); n != 0 {
		t.Fatalf("Close committed unfinished work: %d", n)
	}
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "FALSE" {
		t.Fatalf("Close LOCAL restore = %q, want FALSE", got)
	}
}

func TestAutocommitFalseExplicitBeginStillRejectedWhenBusy(t *testing.T) {
	t.Parallel()
	_, session := newAutocommitRPCSession(t)
	ctx := t.Context()

	mustExec(t, ctx, session, "SET AUTOCOMMIT = FALSE")
	mustExec(t, ctx, session, "BEGIN")
	_, err := execSQL(t, ctx, session, "BEGIN")
	if err == nil || !strings.Contains(err.Error(), "you're in transaction") {
		t.Fatalf("second BEGIN: %v, want you're in transaction", err)
	}
}

func TestAutocommitFalseAutomaticDMLUsesOwner(t *testing.T) {
	t.Parallel()
	h, session := newAutocommitRPCSession(t)
	ctx := t.Context()

	mustExec(t, ctx, session, "SET AUTOCOMMIT = FALSE")
	mustExec(t, ctx, session, "SET AUTO_BATCH_DML = TRUE")
	mustExec(t, ctx, session, "INSERT INTO T (id) VALUES (1)")
	if !session.txn.InTransaction() {
		t.Fatal("automatic DML must install a logical owner")
	}
	if n := len(h.server.commits); n != 0 {
		t.Fatalf("automatic DML committed immediately: %d", n)
	}
}

func TestAutocommitFalseCliTeardownDoesNotCommit(t *testing.T) {
	t.Parallel()
	h, session := newAutocommitRPCSession(t)
	ctx := t.Context()

	mustExec(t, ctx, session, "SET AUTOCOMMIT = FALSE")
	mustExec(t, ctx, session, "INSERT INTO T (id) VALUES (1)")
	cli := &Cli{SessionHandler: &SessionHandler{Session: session}}
	if code := cli.handleExit(); code != exitCodeSuccess {
		t.Fatalf("handleExit = %d, want %d", code, exitCodeSuccess)
	}
	if n := len(h.server.commits); n != 0 {
		t.Fatalf("EXIT/Close committed unfinished work: %d", n)
	}
}
