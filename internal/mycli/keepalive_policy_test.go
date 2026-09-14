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
	"io"
	"slices"
	"strings"
	"testing"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
)

func ownerKeepAliveDisabled(tm *TransactionManager) bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.tc != nil && tm.tc.keepAliveDisabled
}

func ownerHeartbeatScheduled(tm *TransactionManager) bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.tc != nil && tm.tc.heartbeatCancel != nil
}

func beginRWAndQuery(t *testing.T, ctx context.Context, h *heartbeatHarness, sql string) string {
	t.Helper()
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatalf("BeginReadWriteTransaction: %v", err)
	}
	iter, _, err := h.tm.RunQuery(ctx, spanner.NewStatement(sql))
	if err != nil {
		t.Fatalf("RunQuery(%s): %v", sql, err)
	}
	if _, _, _, _, err := consumeRowIterDiscard(iter); err != nil {
		t.Fatalf("drain %s: %v", sql, err)
	}
	id := h.server.txnIDForSQL(sql)
	if id == "" {
		t.Fatalf("no transaction id captured for %q", sql)
	}
	return id
}

func requireNoHeartbeat(t *testing.T, h *heartbeatHarness) {
	t.Helper()
	if h.tm.heartbeatEnabled() {
		t.Fatal("heartbeat marked enabled")
	}
	if ownerHeartbeatScheduled(h.tm) {
		t.Fatal("heartbeat goroutine was scheduled")
	}
	if recs := h.server.heartbeatRecords(); len(recs) != 0 {
		t.Fatalf("heartbeat RPCs = %v, want none", recs)
	}
}

func TestKeepTransactionAliveRegistryAndNoLocal(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	ctx := t.Context()
	if got := mustGetVar(t, session, "KEEP_TRANSACTION_ALIVE"); got != "TRUE" {
		t.Fatalf("default = %q, want TRUE", got)
	}
	mustExec(t, ctx, session, "SET KEEP_TRANSACTION_ALIVE = FALSE")
	if got := mustGetVar(t, session, "KEEP_TRANSACTION_ALIVE"); got != "FALSE" {
		t.Fatalf("SET FALSE = %q", got)
	}
	mustExec(t, ctx, session, "SET KEEP_TRANSACTION_ALIVE = TRUE")
	if got := mustGetVar(t, session, "KEEP_TRANSACTION_ALIVE"); got != "TRUE" {
		t.Fatalf("SET TRUE = %q", got)
	}
	mustExec(t, ctx, session, "BEGIN")
	_, err := execSQL(t, ctx, session, "SET LOCAL KEEP_TRANSACTION_ALIVE = FALSE")
	if err == nil || !strings.Contains(err.Error(), "does not support SET LOCAL") {
		t.Fatalf("SET LOCAL: %v", err)
	}
	if got := mustGetVar(t, session, "KEEP_TRANSACTION_ALIVE"); got != "TRUE" {
		t.Fatalf("rejected SET LOCAL changed value: %s", got)
	}
}

func TestKeepTransactionAliveDefaultSendsHeartbeat(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	id := beginRWAndProbe(t, ctx, h, "SELECT 1 AS keep_alive")
	if ownerKeepAliveDisabled(h.tm) {
		t.Fatal("default owner disabled keepalive")
	}
	sendTick(t, h.ticks)
	waitChan(t, h.server.heartbeatStarted, "default keepalive heartbeat")
	got := h.server.heartbeatIDs()
	if len(got) != 1 || got[0] != id {
		t.Fatalf("heartbeats=%v, want only %s", got, id)
	}
	assertHeartbeatMeta(t, h.server.heartbeatRecords())
	if _, err := h.tm.CommitReadWriteTransaction(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestKeepTransactionAliveFalseSchedulesNoHeartbeat(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	mustExec(t, ctx, session, "SET KEEP_TRANSACTION_ALIVE = FALSE")
	id := beginRWAndQuery(t, ctx, h, "SELECT 1 AS disabled")
	if !ownerKeepAliveDisabled(h.tm) {
		t.Fatal("disabled policy was not frozen on the owner")
	}
	requireNoHeartbeat(t, h)
	if _, err := executeDML(ctx, session, "INSERT INTO T (id) VALUES (1)"); err != nil {
		t.Fatal(err)
	}
	requireNoHeartbeat(t, h)
	if _, err := h.tm.CommitReadWriteTransaction(ctx); err != nil {
		t.Fatal(err)
	}
	if recs := h.server.heartbeatRecords(); len(recs) != 0 {
		t.Fatalf("commit observed heartbeat RPCs %v on %s", recs, id)
	}
}

func TestKeepTransactionAliveFalseLeavesUserOpsAndCancel(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	mustExec(t, ctx, session, "SET KEEP_TRANSACTION_ALIVE = FALSE")
	_ = beginRWAndQuery(t, ctx, h, "SELECT 1 AS cancel_ok")
	requireNoHeartbeat(t, h)
	if err := h.tm.RollbackReadWriteTransaction(ctx); err != nil {
		t.Fatal(err)
	}
	if ownerHeartbeatScheduled(h.tm) {
		t.Fatal("rollback left a heartbeat goroutine")
	}
	if recs := h.server.heartbeatRecords(); len(recs) != 0 {
		t.Fatalf("rollback observed heartbeat RPCs %v", recs)
	}
}

func TestKeepTransactionAliveFrozenAfterOwnerStart(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	id := beginRWAndProbe(t, ctx, h, "SELECT 1 AS frozen_on")
	mustExec(t, ctx, session, "SET KEEP_TRANSACTION_ALIVE = FALSE")
	if ownerKeepAliveDisabled(h.tm) {
		t.Fatal("later SET FALSE mutated the active owner snapshot")
	}
	sendTick(t, h.ticks)
	waitChan(t, h.server.heartbeatStarted, "frozen-enabled heartbeat")
	if got := h.server.heartbeatIDs(); len(got) != 1 || got[0] != id {
		t.Fatalf("heartbeats=%v, want only %s", got, id)
	}
	assertHeartbeatMeta(t, h.server.heartbeatRecords())
}

func TestKeepTransactionAliveFrozenDisabledIgnoresLaterEnable(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	mustExec(t, ctx, session, "SET KEEP_TRANSACTION_ALIVE = FALSE")
	_ = beginRWAndQuery(t, ctx, h, "SELECT 1 AS frozen_off")
	mustExec(t, ctx, session, "SET KEEP_TRANSACTION_ALIVE = TRUE")
	if !ownerKeepAliveDisabled(h.tm) {
		t.Fatal("later SET TRUE mutated the active owner snapshot")
	}
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1 AS still_off", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	requireNoHeartbeat(t, h)
}

func TestKeepTransactionAlivePendingActivationCapturesSET(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	mustExec(t, ctx, session, "SET KEEP_TRANSACTION_ALIVE = TRUE")
	mustExec(t, ctx, session, "BEGIN")
	mustExec(t, ctx, session, "SET KEEP_TRANSACTION_ALIVE = FALSE")
	if _, err := executeDML(ctx, session, "INSERT INTO T (id) VALUES (1)"); err != nil {
		t.Fatal(err)
	}
	if !ownerKeepAliveDisabled(h.tm) {
		t.Fatal("pending SET FALSE was not captured at RW construction")
	}
	requireNoHeartbeat(t, h)
	mustExec(t, ctx, session, "COMMIT")
}

func TestKeepTransactionAliveReplacementOwnerUsesNewPolicy(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	mustExec(t, ctx, session, "SET KEEP_TRANSACTION_ALIVE = FALSE")
	idA := beginRWAndQuery(t, ctx, h, "SELECT 1 AS owner_a")
	requireNoHeartbeat(t, h)
	if err := h.tm.RollbackReadWriteTransaction(ctx); err != nil {
		t.Fatal(err)
	}
	mustExec(t, ctx, session, "SET KEEP_TRANSACTION_ALIVE = TRUE")
	idB := beginRWAndProbe(t, ctx, h, "SELECT 1 AS owner_b")
	if idA == idB {
		t.Fatal("replacement owner reused transaction id A")
	}
	sendTick(t, h.ticks)
	waitChan(t, h.server.heartbeatStarted, "replacement owner B heartbeat")
	got := h.server.heartbeatIDs()
	if len(got) != 1 || got[0] != idB {
		t.Fatalf("heartbeats=%v, want only B=%s (A=%s)", got, idB, idA)
	}
	assertHeartbeatMeta(t, h.server.heartbeatRecords())
}

func TestKeepTransactionAliveSavepointPreservesDisabledPolicy(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	h.tm.enableSavepointCaptureForTest()
	session := sessionForTM(t, h.tm)
	mustExec(t, ctx, session, "SET KEEP_TRANSACTION_ALIVE = FALSE")
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1 AS keep", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	requireNoHeartbeat(t, h)
	if err := h.tm.CreateSavepoint(ctx, "keep"); err != nil {
		t.Fatal(err)
	}
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1 AS extra", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	mustExec(t, ctx, session, "SET KEEP_TRANSACTION_ALIVE = TRUE")
	if err := h.tm.RollbackToSavepoint(ctx, "keep"); err != nil {
		t.Fatal(err)
	}
	if !ownerKeepAliveDisabled(h.tm) {
		t.Fatal("SAVEPOINT reconstruction dropped the disabled keepalive snapshot")
	}
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1 AS after_rollback", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	requireNoHeartbeat(t, h)
	mustExec(t, ctx, session, "COMMIT")
}

func TestKeepTransactionAliveSavepointPreservesEnabledPolicy(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	h.tm.enableSavepointCaptureForTest()
	session := sessionForTM(t, h.tm)
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1 AS keep", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	if !h.tm.heartbeatEnabled() || !ownerHeartbeatScheduled(h.tm) {
		t.Fatal("first user operation did not enable heartbeat")
	}
	idOld := lastUserSQLTxnID(h, "SELECT 1 AS keep")
	if err := h.tm.CreateSavepoint(ctx, "keep"); err != nil {
		t.Fatal(err)
	}
	mustExec(t, ctx, session, "SET KEEP_TRANSACTION_ALIVE = FALSE")
	if err := h.tm.RollbackToSavepoint(ctx, "keep"); err != nil {
		t.Fatal(err)
	}
	if ownerKeepAliveDisabled(h.tm) {
		t.Fatal("SAVEPOINT reconstruction followed later KEEP_TRANSACTION_ALIVE")
	}
	idNew := lastUserSQLTxnID(h, "SELECT 1 AS keep")
	if idNew == "" || idNew == idOld {
		t.Fatalf("replay did not start a new physical attempt; old=%s new=%s", idOld, idNew)
	}
	if !h.tm.heartbeatEnabled() || !ownerHeartbeatScheduled(h.tm) {
		t.Fatal("reconstruction did not restart keepalive on the new attempt")
	}
	sendTick(t, h.ticks)
	waitChan(t, h.server.heartbeatStarted, "reconstructed keepalive")
	got := h.server.heartbeatIDs()
	if !slices.Contains(got, idNew) {
		t.Fatalf("restarted heartbeat missing on new attempt; heartbeats=%v new=%s", got, idNew)
	}
	if slices.Contains(got, idOld) {
		t.Fatalf("restarted heartbeat also hit old attempt; heartbeats=%v old=%s new=%s", got, idOld, idNew)
	}
	assertHeartbeatMeta(t, h.server.heartbeatRecords())
}

func TestKeepTransactionAliveDelayedTickDoesNotBorrowReplacement(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	h.installBeforeAcquireBarrier()

	idA := beginRWAndProbe(t, ctx, h, "SELECT 1 AS owner_a")
	sendTick(t, h.ticks)
	waitChan(t, h.arrived, "owner A eligibility snapshot")

	if err := h.tm.RollbackReadWriteTransaction(ctx); err != nil {
		t.Fatalf("rollback A: %v", err)
	}
	mustExec(t, ctx, session, "SET KEEP_TRANSACTION_ALIVE = FALSE")
	idB := beginRWAndQuery(t, ctx, h, "SELECT 1 AS owner_b")
	if idA == idB {
		t.Fatal("replacement owner reused transaction id A")
	}
	requireNoHeartbeat(t, h)

	close(h.release)
	waitChan(t, h.attempt, "owner A delayed acquire attempt")
	if got := h.server.heartbeatIDs(); slices.Contains(got, idB) {
		t.Fatalf("delayed A tick issued SELECT 1 on disabled owner B; heartbeats=%v B=%s", got, idB)
	}
	if got := h.server.heartbeatIDs(); slices.Contains(got, idA) {
		t.Fatalf("delayed A tick issued SELECT 1 after A ended; heartbeats=%v A=%s", got, idA)
	}
	requireNoHeartbeat(t, h)
}

func TestEnqueueAutomaticDMLRespectsDisabledKeepalive(t *testing.T) {
	t.Parallel()
	sysVars := newSystemVariablesWithDefaultsForTest()
	sysVars.Transaction.KeepTransactionAlive = false
	tm := NewTransactionManager(nil, sysVars, spanner.ClientConfig{})
	tm.tc = &transactionContext{
		attrs:             transactionAttributes{mode: transactionModeReadWrite},
		keepAliveDisabled: true,
		heartbeatFunc: func(context.Context, uint64) {
			t.Error("heartbeat goroutine started on a disabled owner")
		},
	}
	ok, err := tm.TryEnqueueAutomaticDML(spanner.NewStatement("INSERT INTO t (id) VALUES (1)"))
	if err != nil || !ok {
		t.Fatalf("TryEnqueueAutomaticDML: ok=%v err=%v", ok, err)
	}
	if tm.heartbeatEnabled() {
		t.Fatal("disabled owner marked heartbeat enabled")
	}
	if ownerHeartbeatScheduled(tm) {
		t.Fatal("disabled owner scheduled a heartbeat goroutine")
	}
	if !tm.HasAutomaticDML() {
		t.Fatal("disabled keepalive discarded the automatic DML queue")
	}
}
