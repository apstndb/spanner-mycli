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
	"errors"
	"io"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/enums"
)

func ownerHasReplay(tm *TransactionManager) bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.capturingLocked()
}

func assertSavepointCommandsDisabled(t *testing.T, session *Session, tm *TransactionManager) {
	t.Helper()
	ctx := t.Context()
	if _, err := execSQL(t, ctx, session, "SAVEPOINT keep"); !errors.Is(err, errSavepointDisabled) {
		t.Fatalf("SAVEPOINT: %v, want %v", err, errSavepointDisabled)
	}
	if _, err := execSQL(t, ctx, session, "RELEASE SAVEPOINT keep"); !errors.Is(err, errSavepointDisabled) {
		t.Fatalf("RELEASE: %v, want %v", err, errSavepointDisabled)
	}
	if _, err := execSQL(t, ctx, session, "ROLLBACK TO SAVEPOINT keep"); !errors.Is(err, errSavepointDisabled) {
		t.Fatalf("ROLLBACK TO: %v, want %v", err, errSavepointDisabled)
	}
	if tm.sysVars.Transaction.SavepointSupport != enums.SavepointSupportDisabled {
		t.Fatalf("CLI_SAVEPOINT_SUPPORT = %v, want DISABLED", tm.sysVars.Transaction.SavepointSupport)
	}
}

func TestSavepointRetryOwnerAllocatesJournalBeforeFirstOp(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	if ownerHasReplay(h.tm) {
		t.Fatal("retry-disabled owner allocated a journal")
	}

	h.tm.attachRetryReplayForTest()
	if !ownerHasReplay(h.tm) {
		t.Fatal("retry-enabled owner has no journal before the first operation")
	}
	if entries := replayJournal(h.tm); len(entries) != 0 {
		t.Fatalf("empty journal has entries: %+v", entries)
	}
	assertSavepointCommandsDisabled(t, session, h.tm)

	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	entries := replayJournal(h.tm)
	if len(entries) != 1 || entries[0].kind != replayKindSQL || entries[0].stmt.SQL != "SELECT 1" {
		t.Fatalf("journal = %+v", entries)
	}
	if len(entries[0].fingerprint) != 32 {
		t.Fatalf("fingerprint size %d", len(entries[0].fingerprint))
	}
	assertSavepointCommandsDisabled(t, session, h.tm)
	if len(replayJournal(h.tm)) != 1 {
		t.Fatal("disabled SAVEPOINT commands mutated the retry journal")
	}
}

func TestSavepointRetryDisabledAndSavepointDisabledAllocateNone(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	if ownerHasReplay(h.tm) {
		t.Fatal("disabled owner allocated a journal at BEGIN")
	}
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	if ownerHasReplay(h.tm) || len(replayJournal(h.tm)) != 0 {
		t.Fatalf("disabled owner journaled: replay=%v entries=%+v", ownerHasReplay(h.tm), replayJournal(h.tm))
	}
}

func TestSavepointRetrySessionSetDoesNotEnableOrDisableOwnerCapture(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	owner := txnContext(h.tm)
	mustExec(t, ctx, session, "SET RETRY_ABORTS_INTERNALLY = TRUE")
	if ownerHasReplay(h.tm) {
		t.Fatal("session SET TRUE enabled capture on an existing owner")
	}
	if txnContext(h.tm) != owner {
		t.Fatal("session SET replaced the owner")
	}

	h.tm.attachRetryReplayForTest()
	if !ownerHasReplay(h.tm) || txnContext(h.tm) != owner {
		t.Fatal("attachRetryReplayForTest lost the owner or journal")
	}
	mustExec(t, ctx, session, "SET RETRY_ABORTS_INTERNALLY = FALSE")
	if !ownerHasReplay(h.tm) || txnContext(h.tm) != owner {
		t.Fatal("session SET FALSE disabled capture on a retry-required owner")
	}

	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	entries := replayJournal(h.tm)
	if len(entries) != 1 || entries[0].stmt.SQL != "SELECT 1" {
		t.Fatalf("later session SET stopped capture: %+v", entries)
	}
	assertSavepointCommandsDisabled(t, session, h.tm)
}

func TestSavepointRetryPendingOwnerHasJournalWithoutCommands(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	if err := h.tm.BeginPendingTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	if ownerHasReplay(h.tm) {
		t.Fatal("retry-disabled pending owner allocated a journal")
	}
	pending := txnContext(h.tm)
	h.tm.attachRetryReplayForTest()
	if !ownerHasReplay(h.tm) {
		t.Fatal("retry-enabled pending owner has no journal")
	}
	if txnContext(h.tm) != pending {
		t.Fatal("attachRetryReplayForTest replaced the pending owner")
	}
	assertSavepointCommandsDisabled(t, session, h.tm)

	if _, err := h.tm.DetermineTransaction(ctx); err != nil {
		t.Fatalf("pending RW activation: %v", err)
	}
	if txnContext(h.tm) != pending {
		t.Fatal("activation replaced the pending owner")
	}
	if !ownerHasReplay(h.tm) {
		t.Fatal("activation discarded the retry journal")
	}
	assertSavepointCommandsDisabled(t, session, h.tm)
}

func TestSavepointRetryReadOnlyOwnerAllocatesNone(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	if _, err := h.tm.BeginReadOnlyTransaction(ctx, timestampBoundUnspecified, 0, time.Time{}, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	h.tm.attachRetryReplayForTest()
	if ownerHasReplay(h.tm) {
		t.Fatal("read-only retry owner allocated a journal")
	}
}

func TestSavepointRetryReleaseLastMarkerRetainsHistory(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	h.tm.enableSavepointCaptureForTest()
	session := sessionForTM(t, h.tm)
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	h.tm.attachRetryReplayForTest()
	if err := h.tm.CreateSavepoint(ctx, "keep"); err != nil {
		t.Fatal(err)
	}
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	if names := replayMarkerNames(h.tm); len(names) != 1 || names[0] != "keep" {
		t.Fatalf("markers = %v", names)
	}
	if err := h.tm.ReleaseSavepoint("keep"); err != nil {
		t.Fatal(err)
	}
	if names := replayMarkerNames(h.tm); len(names) != 0 {
		t.Fatalf("released markers remain: %v", names)
	}
	entries := replayJournal(h.tm)
	if len(entries) != 1 || entries[0].stmt.SQL != "SELECT 1" {
		t.Fatalf("release discarded retry history: %+v", entries)
	}
	if !ownerHasReplay(h.tm) {
		t.Fatal("releasing the last marker dropped the retry journal")
	}
}

func TestSavepointRetryImplicitRemainsJournalFree(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	mustExec(t, ctx, session, "SET RETRY_ABORTS_INTERNALLY = TRUE")
	_, err := h.tm.RunInNewOrExistRwTx(ctx, func(tx *spanner.ReadWriteStmtBasedTransaction, implicit bool) (int64, *sppb.QueryPlan, *sppb.ResultSetMetadata, error) {
		if !implicit {
			t.Fatal("expected implicit RW")
		}
		if h.tm.capturingLocked() {
			t.Fatal("implicit RW attached a journal")
		}
		return 0, nil, nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if ownerHasReplay(h.tm) {
		t.Fatal("implicit path left a journal")
	}
	if _, err := execSQL(t, ctx, session, "BEGIN RW"); err != nil {
		t.Fatalf("public explicit TRUE: %v", err)
	}
	if !ownerHasReplay(h.tm) {
		t.Fatal("explicit TRUE BEGIN did not allocate a journal")
	}
	assertSavepointCommandsDisabled(t, session, h.tm)
}

func TestSavepointRetryOwnerCapturesSQLBatchDMLAndMutate(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	h.tm.attachRetryReplayForTest()

	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	if _, err := executeDML(ctx, session, "INSERT INTO T (id) VALUES (1)"); err != nil {
		t.Fatal(err)
	}
	ok, err := h.tm.TryEnqueueAutomaticDML(spanner.NewStatement("INSERT INTO T (id) VALUES (2)"))
	if err != nil || !ok {
		t.Fatalf("enqueue: ok=%v err=%v", ok, err)
	}
	if len(replayJournal(h.tm)) != 2 {
		t.Fatal("queued automatic DML became a journal entry")
	}
	if len(replayQueued(h.tm)) != 1 {
		t.Fatal("queued automatic DML was not reserved")
	}
	if _, err := h.tm.FlushAutomaticDML(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := executeBatchDML(ctx, session, []spanner.Statement{spanner.NewStatement("INSERT INTO T (id) VALUES (3)")}); err != nil {
		t.Fatal(err)
	}
	if _, err := (&MutateStatement{Table: "T", Operation: "INSERT", Body: "STRUCT(1 AS id)"}).Execute(ctx, session, OperationOutput{}); err != nil {
		t.Fatal(err)
	}

	entries := replayJournal(h.tm)
	if len(entries) != 5 {
		t.Fatalf("journal = %+v", entries)
	}
	if entries[0].kind != replayKindSQL || entries[0].stmt.SQL != "SELECT 1" {
		t.Fatalf("SQL: %+v", entries[0])
	}
	if entries[1].kind != replayKindSQL || !entries[1].dml {
		t.Fatalf("DML: %+v", entries[1])
	}
	if entries[2].kind != replayKindBatchDML || entries[3].kind != replayKindBatchDML {
		t.Fatalf("batch: %+v", entries)
	}
	if entries[4].kind != replayKindMutate {
		t.Fatalf("mutate: %+v", entries[4])
	}
	for i, e := range entries {
		if len(e.fingerprint) != 32 {
			t.Fatalf("entry %d fingerprint size %d", i, len(e.fingerprint))
		}
	}
	assertSavepointCommandsDisabled(t, session, h.tm)
}

func TestSavepointRetryOwnerSkipsPlanOnly(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	h.tm.attachRetryReplayForTest()
	if _, _, err := h.tm.RunAnalyzeQuery(ctx, spanner.NewStatement("SELECT 1")); err != nil {
		t.Fatal(err)
	}
	if entries := replayJournal(h.tm); len(entries) != 0 {
		t.Fatalf("PLAN journaled: %+v", entries)
	}
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	if entries := replayJournal(h.tm); len(entries) != 1 {
		t.Fatalf("user query journal: %+v", entries)
	}
}

func TestSavepointRetryJournalOverflowPreservesHistory(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	h.tm.attachRetryReplayForTest()
	owner := txnContext(h.tm)
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}

	stmt := spanner.NewStatement("INSERT INTO T (id) VALUES (2)")
	h.tm.mu.Lock()
	h.tm.tc.replay.retainedBytes = savepointJournalLimit
	h.tm.mu.Unlock()
	ok, err := h.tm.TryEnqueueAutomaticDML(stmt)
	if ok || !errors.Is(err, errSavepointJournalFull) {
		t.Fatalf("enqueue: ok=%v err=%v", ok, err)
	}
	if _, err := executeBatchDML(ctx, session, []spanner.Statement{spanner.NewStatement("INSERT INTO T (id) VALUES (3)")}); !errors.Is(err, errSavepointJournalFull) {
		t.Fatalf("manual batch: %v", err)
	}
	if _, err := (&MutateStatement{Table: "T", Operation: "INSERT", Body: "STRUCT(1 AS id)"}).Execute(ctx, session, OperationOutput{}); !errors.Is(err, errSavepointJournalFull) {
		t.Fatalf("mutate: %v", err)
	}
	if txnContext(h.tm) != owner {
		t.Fatal("overflow retired the owner")
	}
	entries := replayJournal(h.tm)
	if len(entries) != 1 || entries[0].stmt.SQL != "SELECT 1" {
		t.Fatalf("overflow mutated history: %+v", entries)
	}
}

func TestRetryAbortsSetLocalAllowedOnUnusedPending(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	if err := h.tm.BeginPendingTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	pending := txnContext(h.tm)
	if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "RETRY_ABORTS_INTERNALLY", Value: "TRUE"}); err != nil {
		t.Fatalf("SET LOCAL unused pending: %v", err)
	}
	if txnContext(h.tm) != pending {
		t.Fatal("SET LOCAL replaced the pending owner")
	}
	h.tm.mu.Lock()
	enabled := h.tm.tc.retryAborts
	h.tm.mu.Unlock()
	if !enabled || !ownerHasReplay(h.tm) {
		t.Fatal("SET LOCAL TRUE did not capture retry or allocate a journal")
	}
	mustExec(t, ctx, session, "SET RETRY_ABORTS_INTERNALLY = FALSE")
	h.tm.mu.Lock()
	enabled = h.tm.tc.retryAborts
	h.tm.mu.Unlock()
	if !enabled {
		t.Fatal("ordinary SET overrode the captured LOCAL retry policy")
	}
}

func TestTransactionContextRetryReplayKeepsIdentity(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	if err := h.tm.BeginPendingTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	pending := txnContext(h.tm)
	h.tm.attachRetryReplayForTest()
	if txnContext(h.tm) != pending {
		t.Fatal("retry journal attach replaced the pending owner")
	}
	mustExec(t, ctx, session, "SET RETRY_ABORTS_INTERNALLY = TRUE")
	mustExec(t, ctx, session, "SET RETRY_ABORTS_INTERNALLY = FALSE")
	if txnContext(h.tm) != pending {
		t.Fatal("session SET replaced the pending owner")
	}
	if !ownerHasReplay(h.tm) {
		t.Fatal("session SET dropped the pending retry journal")
	}
}
