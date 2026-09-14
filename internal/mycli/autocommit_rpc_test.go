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
	"strings"
	"testing"

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
	_, _ = execSQL(t, ctx, session, "EXPLAIN SELECT 1")
	assertIdle(t, session, "EXPLAIN PLAN SELECT left an owner")

	before := len(h.server.commits)
	_, _ = execSQL(t, ctx, session, "EXPLAIN UPDATE T SET id = 1 WHERE true")
	assertIdle(t, session, "EXPLAIN PLAN DML must not leave an owner")
	if n := len(h.server.commits); n < before {
		t.Fatalf("EXPLAIN PLAN DML commit count went backwards: %d -> %d", before, n)
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
	_, _ = execSQL(t, ctx, session, "PARTITIONED UPDATE T SET id = 1 WHERE true")
	assertIdle(t, session, "explicit PDML created a session owner")

	_, _ = execSQL(t, ctx, session, "TRUNCATE TABLE T")
	assertIdle(t, session, "TRUNCATE created a session owner")
	if n := len(h.server.commits); n != 0 {
		t.Fatalf("PDML/TRUNCATE used RW Commit: %d", n)
	}
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
