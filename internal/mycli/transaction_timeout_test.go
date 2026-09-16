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
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/samber/lo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func ownerTimeout(tm *TransactionManager) (d time.Duration, captured, armed bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	if tm.tc == nil {
		return 0, false, false
	}
	return tm.tc.timeout, tm.tc.timeoutCaptured, !tm.tc.deadline.IsZero()
}

func ownerDeadline(tm *TransactionManager) time.Time {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	if tm.tc == nil {
		return time.Time{}
	}
	return tm.tc.deadline
}

func ownerFirstUse(tm *TransactionManager) bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.tc != nil && tm.tc.firstUse
}

func requireOwner(t *testing.T, tm *TransactionManager) *transactionContext {
	t.Helper()
	owner := txnContext(tm)
	if owner == nil {
		t.Fatal("expected a live logical owner")
	}
	return owner
}

func TestTransactionTimeoutRegistryAndLocal(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	ctx := t.Context()
	if got := mustGetVar(t, session, "TRANSACTION_TIMEOUT"); got != "NULL" {
		t.Fatalf("default = %q, want NULL", got)
	}
	mustExec(t, ctx, session, "SET TRANSACTION_TIMEOUT = '30s'")
	if got := mustGetVar(t, session, "TRANSACTION_TIMEOUT"); got != "30s" {
		t.Fatalf("SET 30s = %q", got)
	}
	mustExec(t, ctx, session, "SET TRANSACTION_TIMEOUT = NULL")
	if got := mustGetVar(t, session, "TRANSACTION_TIMEOUT"); got != "NULL" {
		t.Fatalf("SET NULL = %q", got)
	}
	mustExec(t, ctx, session, "SET TRANSACTION_TIMEOUT = '0s'")
	if got := mustGetVar(t, session, "TRANSACTION_TIMEOUT"); got != "0s" {
		t.Fatalf("SET 0s = %q", got)
	}
	mustExec(t, ctx, session, "BEGIN")
	mustExec(t, ctx, session, "SET LOCAL TRANSACTION_TIMEOUT = '45s'")
	if got := mustGetVar(t, session, "TRANSACTION_TIMEOUT"); got != "45s" {
		t.Fatalf("SET LOCAL = %q", got)
	}
	d, captured, armed := ownerTimeout(session.txn)
	if !captured || d != 45*time.Second || armed {
		t.Fatalf("pending local snapshot d=%s captured=%v armed=%v", d, captured, armed)
	}
}

func TestTransactionTimeoutPendingDoesNotArmOnShowOrSet(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	ctx := t.Context()
	mustExec(t, ctx, session, "SET TRANSACTION_TIMEOUT = '10s'")
	mustExec(t, ctx, session, "BEGIN")
	mustExec(t, ctx, session, "SHOW VARIABLE TRANSACTION_TIMEOUT")
	mustExec(t, ctx, session, "SHOW TRANSACTION ISOLATION LEVEL")
	mustExec(t, ctx, session, "SET LOCAL CLI_VERBOSE = TRUE")
	_, _, armed := ownerTimeout(session.txn)
	if armed {
		t.Fatal("client-only SHOW/SET LOCAL armed the budget")
	}
}

func TestTransactionTimeoutSessionSETAfterBeginDoesNotChangeOwner(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	ctx := t.Context()
	mustExec(t, ctx, session, "SET TRANSACTION_TIMEOUT = '10s'")
	mustExec(t, ctx, session, "BEGIN")
	mustExec(t, ctx, session, "SET TRANSACTION_TIMEOUT = '30s'")
	d, captured, armed := ownerTimeout(session.txn)
	if !captured || d != 10*time.Second || armed {
		t.Fatalf("owner snapshot d=%s captured=%v armed=%v", d, captured, armed)
	}
	if got := mustGetVar(t, session, "TRANSACTION_TIMEOUT"); got != "30s" {
		t.Fatalf("session SHOW = %q, want 30s", got)
	}
}

func TestTransactionTimeoutResetIsSessionOnly(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	ctx := t.Context()
	mustExec(t, ctx, session, "SET TRANSACTION_TIMEOUT = '10s'")
	if err := session.systemVariables.CaptureStartupSnapshots(); err != nil {
		t.Fatal(err)
	}
	mustExec(t, ctx, session, "SET TRANSACTION_TIMEOUT = '30s'")
	mustExec(t, ctx, session, "BEGIN")
	mustExec(t, ctx, session, "SET LOCAL TRANSACTION_TIMEOUT = '2m'")
	if _, err := session.ExecuteStatement(ctx, &ResetStatement{VarName: "TRANSACTION_TIMEOUT"}); err != nil {
		t.Fatalf("RESET TRANSACTION_TIMEOUT: %v", err)
	}
	if got := mustGetVar(t, session, "TRANSACTION_TIMEOUT"); got != "10s" {
		t.Fatalf("RESET SHOW = %q, want 10s snapshot", got)
	}
	d, captured, armed := ownerTimeout(session.txn)
	if !captured || d != 2*time.Minute || armed {
		t.Fatalf("RESET mutated owner snapshot d=%s captured=%v armed=%v", d, captured, armed)
	}
}

func TestTransactionTimeoutPendingLocalSelectsDuration(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	mustExec(t, ctx, session, "SET TRANSACTION_TIMEOUT = '10s'")
	mustExec(t, ctx, session, "BEGIN")
	mustExec(t, ctx, session, "SET LOCAL TRANSACTION_TIMEOUT = '2m'")
	if _, err := h.tm.DetermineTransaction(ctx); err != nil {
		t.Fatal(err)
	}
	d, captured, armed := ownerTimeout(h.tm)
	if !captured || d != 2*time.Minute || !armed {
		t.Fatalf("activated snapshot d=%s captured=%v armed=%v", d, captured, armed)
	}
	remaining := time.Until(ownerDeadline(h.tm))
	if remaining > 2*time.Minute || remaining < 2*time.Minute-2*time.Second {
		t.Fatalf("remaining=%s, want ~2m", remaining)
	}
}

func TestTransactionTimeoutRejectsLocalChangeAfterArm(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	mustExec(t, ctx, session, "SET TRANSACTION_TIMEOUT = '1h'")
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	_, err := execSQL(t, ctx, session, "SET LOCAL TRANSACTION_TIMEOUT = '2h'")
	if err == nil || !errors.Is(err, errTransactionTimeoutFrozen) {
		t.Fatalf("SET LOCAL after arm: %v", err)
	}
	if got := mustGetVar(t, session, "TRANSACTION_TIMEOUT"); got != time.Hour.String() {
		t.Fatalf("rejected LOCAL leaked: %s", got)
	}
	mustExec(t, ctx, session, "SET LOCAL TRANSACTION_TIMEOUT = '1h'")
}

func TestTransactionTimeoutConstructorStartsBudget(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	mustExec(t, ctx, session, "SET TRANSACTION_TIMEOUT = '1h'")
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	_, _, armed := ownerTimeout(h.tm)
	if !armed {
		t.Fatal("constructor BeginTransaction did not start the budget")
	}
	if len(h.server.beginObservations()) == 0 {
		t.Fatal("constructor did not issue BeginTransaction")
	}
}

func TestTransactionTimeoutFailedConstructorPreservesPending(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	mustExec(t, ctx, session, "SET TRANSACTION_TIMEOUT = '1h'")
	mustExec(t, ctx, session, "BEGIN")
	mustExec(t, ctx, session, "SET LOCAL CLI_VERBOSE = TRUE")
	pending := requireOwner(t, h.tm)
	injected := status.Error(codes.PermissionDenied, "injected constructor failure")
	h.server.failBegin = injected
	err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED)
	if err == nil || !strings.Contains(err.Error(), "injected constructor failure") {
		t.Fatalf("constructor: %v", err)
	}
	if txnContext(h.tm) != pending {
		t.Fatal("failed constructor replaced the pending owner")
	}
	if pending.attrs.mode != transactionModePending {
		t.Fatalf("mode = %q, want pending", pending.attrs.mode)
	}
	d, captured, armed := ownerTimeout(h.tm)
	if !captured || d != time.Hour || !armed {
		t.Fatalf("failed constructor snapshot d=%s captured=%v armed=%v", d, captured, armed)
	}
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "TRUE" {
		t.Fatalf("failed constructor restored LOCAL: %s", got)
	}
	deadline := ownerDeadline(h.tm)
	h.server.failBegin = nil
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	if txnContext(h.tm) != pending {
		t.Fatal("retry replaced the pending owner")
	}
	if !ownerDeadline(h.tm).Equal(deadline) {
		t.Fatal("retry restarted the transaction budget")
	}
}

func TestTransactionTimeoutIdleConstructorFailureLeavesNoOwner(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	mustExec(t, ctx, session, "SET TRANSACTION_TIMEOUT = '1h'")
	h.server.failBegin = status.Error(codes.PermissionDenied, "idle constructor failure")
	err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED)
	if err == nil || !strings.Contains(err.Error(), "idle constructor failure") {
		t.Fatalf("idle constructor: %v", err)
	}
	if txnContext(h.tm) != nil {
		t.Fatal("failed idle constructor left a logical owner")
	}
}

func TestTransactionTimeoutCancelsRPCWhileMutexHeld(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	// Constructor BeginTransaction must not consume the 30ms budget. SET
	// TRANSACTION_TIMEOUT before Begin arms at the constructor RPC, which
	// failed once under -race on 2026-09-16 during setup. SET LOCAL is frozen
	// after first use, so capture the duration unarmed after construction.
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	owner := requireOwner(t, h.tm)
	expired := make(chan struct{})
	h.tm.timeoutAfterExpire = func(got *transactionContext) {
		if got == owner {
			close(expired)
		}
	}
	block := make(chan struct{})
	inFlight := make(chan struct{})
	h.server.setBlockSQL(block)
	h.server.setSQLBlocked(func() { close(inFlight) })
	captureUnarmedTransactionTimeout(t, h.tm, 30*time.Millisecond)

	errCh := make(chan error, 1)
	go func() {
		iter, _, err := h.tm.RunQuery(ctx, spanner.NewStatement("SELECT 1 AS held"))
		if err != nil {
			errCh <- err
			return
		}
		_, _, _, _, err = consumeRowIterDiscard(iter)
		errCh <- err
	}()
	waitChan(t, inFlight, "query RPC while transaction mutex held")
	err := waitTimeoutErr(t, errCh, "cancelled query")
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, errTransactionTimeout) && status.Code(err) != codes.DeadlineExceeded && status.Code(err) != codes.Canceled {
		t.Fatalf("held-mutex cancel: %v", err)
	}
	waitChan(t, expired, "matching owner retire after held-mutex cancel")
	if txnContext(h.tm) != nil {
		t.Fatal("expiry left a replacement or stale owner")
	}
}

func TestTransactionTimeoutIdleExpiryThenNewOwner(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	mustExec(t, ctx, session, "SET TRANSACTION_TIMEOUT = '1h'")
	mustExec(t, ctx, session, "BEGIN")
	mustExec(t, ctx, session, "SET LOCAL CLI_VERBOSE = TRUE")
	mustExec(t, ctx, session, "SET LOCAL TRANSACTION_TIMEOUT = '20ms'")
	ownerA := requireOwner(t, h.tm)
	expired := make(chan struct{})
	h.tm.timeoutAfterExpire = func(owner *transactionContext) {
		if owner == ownerA {
			close(expired)
		}
	}
	if _, err := h.tm.DetermineTransaction(ctx); err != nil && !isTimeoutish(err) {
		t.Fatal(err)
	}
	waitChan(t, expired, "owner A expiry")
	if txnContext(h.tm) != nil {
		t.Fatal("expired owner A was not retired")
	}
	mustExec(t, ctx, session, "BEGIN")
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "FALSE" {
		t.Fatalf("LOCAL undo not restored before new owner: %s", got)
	}
	if got := mustGetVar(t, session, "TRANSACTION_TIMEOUT"); got != time.Hour.String() {
		t.Fatalf("LOCAL timeout not restored before new owner: %s", got)
	}
	ownerB := requireOwner(t, h.tm)
	if ownerB == ownerA {
		t.Fatal("new BEGIN reused expired owner A")
	}
	d, captured, armed := ownerTimeout(h.tm)
	if !captured || d != time.Hour || armed {
		t.Fatalf("new owner snapshot d=%s captured=%v armed=%v", d, captured, armed)
	}
}

func TestTransactionTimeoutDelayedCallbackDoesNotRetireReplacement(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	mustExec(t, ctx, session, "SET TRANSACTION_TIMEOUT = '1h'")
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	ownerA := requireOwner(t, h.tm)
	if err := h.tm.RollbackReadWriteTransaction(ctx); err != nil {
		t.Fatal(err)
	}
	h.tm.restoreLocalVarsIfIdle()
	mustExec(t, ctx, session, "SET TRANSACTION_TIMEOUT = '2h'")
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	ownerB := requireOwner(t, h.tm)
	if ownerB == ownerA {
		t.Fatal("replacement reused owner A")
	}
	h.tm.watchTransactionDeadline(ownerA, expiredContext())
	if txnContext(h.tm) != ownerB {
		t.Fatal("delayed A callback retired replacement owner B")
	}
	d, _, _ := ownerTimeout(h.tm)
	if d != 2*time.Hour {
		t.Fatalf("delayed callback mutated B budget: %s", d)
	}
}

func TestTransactionTimeoutSavepointPreservesBudget(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	h.tm.enableSavepointCaptureForTest()
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	mustExec(t, ctx, session, "SET TRANSACTION_TIMEOUT = '1h'")
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	owner := requireOwner(t, h.tm)
	deadline := ownerDeadline(h.tm)
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1 AS keep", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	if err := h.tm.CreateSavepoint(ctx, "keep"); err != nil {
		t.Fatal(err)
	}
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1 AS extra", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	mustExec(t, ctx, session, "SET TRANSACTION_TIMEOUT = '10s'")
	if err := h.tm.RollbackToSavepoint(ctx, "keep"); err != nil {
		t.Fatal(err)
	}
	if txnContext(h.tm) != owner {
		t.Fatal("ROLLBACK TO replaced the logical owner")
	}
	if !ownerDeadline(h.tm).Equal(deadline) {
		t.Fatal("ROLLBACK TO restarted the transaction budget")
	}
	d, _, armed := ownerTimeout(h.tm)
	if d != time.Hour || !armed {
		t.Fatalf("reconstructed snapshot d=%s armed=%v", d, armed)
	}
}

func TestTransactionTimeoutAutomaticDMLActivation(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	mustExec(t, ctx, session, "SET TRANSACTION_TIMEOUT = '1h'")
	mustExec(t, ctx, session, "SET AUTO_BATCH_DML = TRUE")
	mustExec(t, ctx, session, "BEGIN RW")
	_, _, armed := ownerTimeout(h.tm)
	if !armed {
		t.Fatal("BEGIN RW constructor did not arm before automatic DML")
	}
	res := mustExec(t, ctx, session, "INSERT INTO T (id) VALUES (1)")
	if res.IsExecutedDML || !h.tm.HasAutomaticDML() {
		t.Fatalf("expected queued automatic DML: executed=%v queued=%v", res.IsExecutedDML, h.tm.HasAutomaticDML())
	}
	deadline := ownerDeadline(h.tm)
	mustExec(t, ctx, session, "COMMIT")
	if txnContext(h.tm) != nil {
		t.Fatal("COMMIT left the owner")
	}
	if deadline.IsZero() {
		t.Fatal("automatic DML path never armed a deadline")
	}
}

func TestTransactionTimeoutZeroAndNullDoNotArm(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"NULL", "'0s'"} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			h := newHeartbeatHarness(t)
			ctx := t.Context()
			session := sessionForTM(t, h.tm)
			mustExec(t, ctx, session, "SET TRANSACTION_TIMEOUT = "+value)
			if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
				t.Fatal(err)
			}
			if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1 AS unlimited", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
				t.Fatal(err)
			}
			_, _, armed := ownerTimeout(h.tm)
			if armed {
				t.Fatal("NULL/0 budget was armed")
			}
			if !ownerFirstUse(h.tm) {
				t.Fatal("NULL/0 first database use was not recorded")
			}
			_, err := execSQL(t, ctx, session, "SET LOCAL TRANSACTION_TIMEOUT = '1h'")
			if err == nil || !errors.Is(err, errTransactionTimeoutFrozen) {
				t.Fatalf("SET LOCAL after NULL/0 first use: %v", err)
			}
		})
	}
}

func TestTransactionTimeoutShorterCallerContext(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	mustExec(t, t.Context(), session, "SET TRANSACTION_TIMEOUT = '1h'")
	if err := h.tm.BeginReadWriteTransaction(t.Context(), sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	block := make(chan struct{})
	inFlight := make(chan struct{})
	h.server.setBlockSQL(block)
	h.server.setSQLBlocked(func() { close(inFlight) })
	caller, cancel := context.WithCancel(t.Context())
	errCh := make(chan error, 1)
	go func() {
		iter, _, err := h.tm.RunQuery(caller, spanner.NewStatement("SELECT 1 AS caller"))
		if err != nil {
			errCh <- err
			return
		}
		_, _, _, _, err = consumeRowIterDiscard(iter)
		errCh <- err
	}()
	waitChan(t, inFlight, "caller-cancelled query")
	cancel()
	err := waitTimeoutErr(t, errCh, "caller cancel")
	if !errors.Is(err, context.Canceled) && status.Code(err) != codes.Canceled {
		t.Fatalf("caller cancel: %v", err)
	}
	if txnContext(h.tm) == nil {
		t.Fatal("caller cancel retired the TRANSACTION_TIMEOUT owner")
	}
}

func TestTransactionTimeoutEnqueueDoesNotStartBudget(t *testing.T) {
	t.Parallel()
	sysVars := newSystemVariablesWithDefaultsForTest()
	sysVars.Transaction.TransactionTimeout = lo.ToPtr(time.Hour)
	tm := NewTransactionManager(nil, sysVars, spanner.ClientConfig{})
	tm.tc = &transactionContext{
		attrs:           transactionAttributes{mode: transactionModeReadWrite},
		timeout:         time.Hour,
		timeoutCaptured: true,
	}
	ok, err := tm.TryEnqueueAutomaticDML(spanner.NewStatement("INSERT INTO t (id) VALUES (1)"))
	if err != nil || !ok {
		t.Fatalf("TryEnqueueAutomaticDML: ok=%v err=%v", ok, err)
	}
	_, _, armed := ownerTimeout(tm)
	if armed {
		t.Fatal("buffering automatic DML started the budget")
	}
	if !tm.HasAutomaticDML() {
		t.Fatal("timeout path discarded the automatic DML queue")
	}
}

func TestTransactionTimeoutConstructorCancelWhileMutexHeld(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	mustExec(t, ctx, session, "SET TRANSACTION_TIMEOUT = '20ms'")
	mustExec(t, ctx, session, "BEGIN")
	owner := requireOwner(t, h.tm)
	expired := make(chan struct{})
	h.tm.timeoutAfterExpire = func(got *transactionContext) {
		if got == owner {
			close(expired)
		}
	}
	block := make(chan struct{})
	inFlight := make(chan struct{})
	h.server.blockBegin = block
	h.server.beginBlocked = sync.OnceFunc(func() { close(inFlight) })
	errCh := make(chan error, 1)
	go func() {
		errCh <- h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED)
	}()
	waitChan(t, inFlight, "constructor BeginTransaction while mutex held")
	err := waitTimeoutErr(t, errCh, "cancelled constructor")
	if !isTimeoutish(err) {
		t.Fatalf("constructor cancel: %v", err)
	}
	waitChan(t, expired, "matching pending owner retire after constructor cancel")
	if txnContext(h.tm) != nil {
		t.Fatal("constructor expiry left a stale owner")
	}
}

func TestTransactionTimeoutNullZeroFreezeAfterConstructor(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"NULL", "'0s'"} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			h := newHeartbeatHarness(t)
			ctx := t.Context()
			session := sessionForTM(t, h.tm)
			mustExec(t, ctx, session, "SET TRANSACTION_TIMEOUT = "+value)
			if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
				t.Fatal(err)
			}
			if len(h.server.beginObservations()) == 0 {
				t.Fatal("constructor did not issue BeginTransaction")
			}
			_, _, armed := ownerTimeout(h.tm)
			if armed {
				t.Fatal("NULL/0 constructor started a timer")
			}
			if !ownerFirstUse(h.tm) {
				t.Fatal("NULL/0 constructor did not record first use")
			}
			_, err := execSQL(t, ctx, session, "SET LOCAL TRANSACTION_TIMEOUT = '1h'")
			if err == nil || !errors.Is(err, errTransactionTimeoutFrozen) {
				t.Fatalf("SET LOCAL after constructor BeginTransaction: %v", err)
			}
			if got := mustGetVar(t, session, "TRANSACTION_TIMEOUT"); got != "NULL" && got != "0s" {
				t.Fatalf("rejected LOCAL leaked: %s", got)
			}
		})
	}
}

func TestTransactionTimeoutFailedConstructorPreservesNullFirstUse(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"NULL", "'0s'"} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			h := newHeartbeatHarness(t)
			ctx := t.Context()
			session := sessionForTM(t, h.tm)
			mustExec(t, ctx, session, "SET TRANSACTION_TIMEOUT = "+value)
			mustExec(t, ctx, session, "BEGIN")
			mustExec(t, ctx, session, "SET LOCAL CLI_VERBOSE = TRUE")
			pending := requireOwner(t, h.tm)
			if ownerFirstUse(h.tm) {
				t.Fatal("pending BEGIN recorded first use")
			}
			injected := status.Error(codes.PermissionDenied, "injected constructor failure")
			h.server.failBegin = injected
			err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED)
			if err == nil || !strings.Contains(err.Error(), "injected constructor failure") {
				t.Fatalf("constructor: %v", err)
			}
			if txnContext(h.tm) != pending {
				t.Fatal("failed constructor replaced the pending owner")
			}
			if !ownerFirstUse(h.tm) {
				t.Fatal("failed constructor dropped first-use")
			}
			_, _, armed := ownerTimeout(h.tm)
			if armed {
				t.Fatal("NULL/0 failed constructor started a timer")
			}
			_, localErr := execSQL(t, ctx, session, "SET LOCAL TRANSACTION_TIMEOUT = '45s'")
			if localErr == nil || !errors.Is(localErr, errTransactionTimeoutFrozen) {
				t.Fatalf("SET LOCAL after failed constructor first-use: %v", localErr)
			}
			h.server.failBegin = nil
			if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
				t.Fatal(err)
			}
			if txnContext(h.tm) != pending {
				t.Fatal("retry replaced the pending owner")
			}
			if !ownerFirstUse(h.tm) {
				t.Fatal("retry dropped first-use")
			}
		})
	}
}

func TestTransactionTimeoutBatchDMLBindsRemainingDeadline(t *testing.T) {
	t.Parallel()
	const txnBudget = 2 * time.Second
	const callerBudget = 20 * time.Second
	dml := "INSERT INTO T (id) VALUES (1)"

	tests := []struct {
		name string
		run  func(t *testing.T, ctx context.Context, h *heartbeatHarness, session *Session)
	}{
		{
			name: "manual BatchUpdate",
			run: func(t *testing.T, ctx context.Context, h *heartbeatHarness, session *Session) {
				if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
					t.Fatal(err)
				}
				if _, err := executeBatchDML(ctx, session, []spanner.Statement{spanner.NewStatement(dml)}); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "RUN BATCH",
			run: func(t *testing.T, ctx context.Context, h *heartbeatHarness, session *Session) {
				mustExec(t, ctx, session, "BEGIN RW")
				mustExec(t, ctx, session, "START BATCH DML")
				mustExec(t, ctx, session, dml)
				mustExec(t, ctx, session, "RUN BATCH")
			},
		},
		{
			name: "FlushAutomaticDML",
			run: func(t *testing.T, ctx context.Context, h *heartbeatHarness, session *Session) {
				mustExec(t, t.Context(), session, "SET AUTO_BATCH_DML = TRUE")
				mustExec(t, ctx, session, "BEGIN RW")
				res := mustExec(t, ctx, session, dml)
				if res.IsExecutedDML || !h.tm.HasAutomaticDML() {
					t.Fatalf("expected queued automatic DML: executed=%v queued=%v", res.IsExecutedDML, h.tm.HasAutomaticDML())
				}
				if _, err := h.tm.FlushAutomaticDML(ctx); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "flush-before-read",
			run: func(t *testing.T, ctx context.Context, h *heartbeatHarness, session *Session) {
				mustExec(t, t.Context(), session, "SET AUTO_BATCH_DML = TRUE")
				mustExec(t, ctx, session, "BEGIN RW")
				res := mustExec(t, ctx, session, dml)
				if res.IsExecutedDML || !h.tm.HasAutomaticDML() {
					t.Fatalf("expected queued automatic DML: executed=%v queued=%v", res.IsExecutedDML, h.tm.HasAutomaticDML())
				}
				if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1 AS after_flush", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHeartbeatHarness(t)
			session := sessionForTM(t, h.tm)
			h.attachSessionClient(session)
			mustExec(t, t.Context(), session, "SET TRANSACTION_TIMEOUT = '2s'")
			caller, cancel := context.WithTimeout(t.Context(), callerBudget)
			defer cancel()
			tc.run(t, caller, h, session)
			obs := h.server.batchObservations()
			if len(obs) == 0 {
				t.Fatal("expected a Batch DML RPC")
			}
			last := obs[len(obs)-1]
			if !last.hasDeadline {
				t.Fatal("Batch DML RPC had no context deadline")
			}
			assertApproxDeadline(t, last.deadlineRemaining, txnBudget)
			if last.deadlineRemaining > 5*time.Second {
				t.Fatalf("Batch DML used caller/statement deadline remaining=%s", last.deadlineRemaining)
			}
		})
	}
}

func TestTransactionTimeoutBatchDMLCancelsWhileMutexHeld(t *testing.T) {
	t.Parallel()
	dml := "INSERT INTO T (id) VALUES (1)"
	tests := []struct {
		name string
		run  func(t *testing.T, ctx context.Context, h *heartbeatHarness, session *Session) error
	}{
		{
			name: "manual BatchUpdate",
			run: func(t *testing.T, ctx context.Context, h *heartbeatHarness, session *Session) error {
				if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
					return err
				}
				_, err := executeBatchDML(ctx, session, []spanner.Statement{spanner.NewStatement(dml)})
				return err
			},
		},
		{
			name: "RUN BATCH",
			run: func(t *testing.T, ctx context.Context, h *heartbeatHarness, session *Session) error {
				if err := h.tm.BeginReadWriteTransaction(t.Context(), sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
					return err
				}
				mustExec(t, t.Context(), session, "START BATCH DML")
				mustExec(t, t.Context(), session, dml)
				_, err := execSQL(t, ctx, session, "RUN BATCH")
				return err
			},
		},
		{
			name: "FlushAutomaticDML",
			run: func(t *testing.T, ctx context.Context, h *heartbeatHarness, session *Session) error {
				mustExec(t, t.Context(), session, "SET AUTO_BATCH_DML = TRUE")
				if err := h.tm.BeginReadWriteTransaction(t.Context(), sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
					return err
				}
				res := mustExec(t, t.Context(), session, dml)
				if res.IsExecutedDML || !h.tm.HasAutomaticDML() {
					t.Fatalf("expected queued automatic DML: executed=%v queued=%v", res.IsExecutedDML, h.tm.HasAutomaticDML())
				}
				_, err := h.tm.FlushAutomaticDML(ctx)
				return err
			},
		},
		{
			name: "flush-before-read",
			run: func(t *testing.T, ctx context.Context, h *heartbeatHarness, session *Session) error {
				mustExec(t, t.Context(), session, "SET AUTO_BATCH_DML = TRUE")
				if err := h.tm.BeginReadWriteTransaction(t.Context(), sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
					return err
				}
				res := mustExec(t, t.Context(), session, dml)
				if res.IsExecutedDML || !h.tm.HasAutomaticDML() {
					t.Fatalf("expected queued automatic DML: executed=%v queued=%v", res.IsExecutedDML, h.tm.HasAutomaticDML())
				}
				_, err := executeSQLImplWithVars(ctx, session, "SELECT 1 AS after_flush", session.systemVariables, OperationOutput{w: io.Discard})
				return err
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHeartbeatHarness(t)
			ctx := t.Context()
			session := sessionForTM(t, h.tm)
			h.attachSessionClient(session)
			mustExec(t, ctx, session, "SET TRANSACTION_TIMEOUT = '30ms'")
			ownerReady := make(chan *transactionContext, 1)
			// Arm via constructor before blocking Batch DML so first-use is the
			// BeginTransaction RPC, then expire during the mutex-held batch RPC.
			if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
				t.Fatal(err)
			}
			owner := requireOwner(t, h.tm)
			ownerReady <- owner
			expired := make(chan struct{})
			h.tm.timeoutAfterExpire = func(got *transactionContext) {
				if got == owner {
					close(expired)
				}
			}
			block := make(chan struct{})
			inFlight := make(chan struct{})
			h.server.setBlockBatchDML(block)
			h.server.setBatchBlocked(func() { close(inFlight) })

			errCh := make(chan error, 1)
			go func() {
				// Routes that begin themselves would re-enter an active txn.
				// Reuse the already-started owner and only issue the batch RPC.
				switch tc.name {
				case "manual BatchUpdate":
					_, err := executeBatchDML(ctx, session, []spanner.Statement{spanner.NewStatement(dml)})
					errCh <- err
				case "RUN BATCH":
					mustExec(t, ctx, session, "START BATCH DML")
					mustExec(t, ctx, session, dml)
					_, err := execSQL(t, ctx, session, "RUN BATCH")
					errCh <- err
				case "FlushAutomaticDML":
					mustExec(t, ctx, session, "SET AUTO_BATCH_DML = TRUE")
					res := mustExec(t, ctx, session, dml)
					if res.IsExecutedDML || !h.tm.HasAutomaticDML() {
						errCh <- fmt.Errorf("expected queued automatic DML: executed=%v queued=%v", res.IsExecutedDML, h.tm.HasAutomaticDML())
						return
					}
					_, err := h.tm.FlushAutomaticDML(ctx)
					errCh <- err
				case "flush-before-read":
					mustExec(t, ctx, session, "SET AUTO_BATCH_DML = TRUE")
					res := mustExec(t, ctx, session, dml)
					if res.IsExecutedDML || !h.tm.HasAutomaticDML() {
						errCh <- fmt.Errorf("expected queued automatic DML: executed=%v queued=%v", res.IsExecutedDML, h.tm.HasAutomaticDML())
						return
					}
					_, err := executeSQLImplWithVars(ctx, session, "SELECT 1 AS after_flush", session.systemVariables, OperationOutput{w: io.Discard})
					errCh <- err
				default:
					errCh <- tc.run(t, ctx, h, session)
				}
			}()
			_ = ownerReady
			waitChan(t, inFlight, "Batch DML RPC while transaction mutex held")
			err := waitTimeoutErr(t, errCh, "cancelled Batch DML")
			if !isTimeoutish(err) {
				t.Fatalf("held-mutex Batch DML cancel: %v", err)
			}
			// The expiry watcher or handleOwnerFailure rollback may retire
			// the owner. Rollback closes the deadline context with Canceled,
			// so timeoutAfterExpire is not always invoked.
			select {
			case <-expired:
			case <-time.After(2 * time.Second):
			}
			if txnContext(h.tm) != nil {
				t.Fatal("expiry left a replacement or stale owner")
			}
		})
	}
}

func TestTransactionTimeoutExpiryAfterEntryRestoreBeforeBegin(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	h.attachSessionClient(session)
	mustExec(t, ctx, session, "SET TRANSACTION_TIMEOUT = '1h'")
	mustExec(t, ctx, session, "BEGIN")
	mustExec(t, ctx, session, "SET LOCAL TRANSACTION_TIMEOUT = '2h'")
	mustExec(t, ctx, session, "SET LOCAL CLI_VERBOSE = TRUE")
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	ownerA := requireOwner(t, h.tm)
	expireOwnerAfterEntryRestore(h.tm, ownerA)
	mustExec(t, ctx, session, "BEGIN")
	if txnContext(h.tm) == ownerA {
		t.Fatal("BEGIN reused expired owner A")
	}
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "FALSE" {
		t.Fatalf("B inherited A's LOCAL verbose: %s", got)
	}
	if got := mustGetVar(t, session, "TRANSACTION_TIMEOUT"); got != time.Hour.String() {
		t.Fatalf("B inherited A's LOCAL timeout: %s", got)
	}
	d, captured, armed := ownerTimeout(h.tm)
	if !captured || d != time.Hour || armed {
		t.Fatalf("B snapshot d=%s captured=%v armed=%v, want restored 1h pending", d, captured, armed)
	}
}

func TestTransactionTimeoutExpiryBeforeEntryRestore(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	h.attachSessionClient(session)
	mustExec(t, ctx, session, "SET TRANSACTION_TIMEOUT = '1h'")
	mustExec(t, ctx, session, "BEGIN")
	mustExec(t, ctx, session, "SET LOCAL TRANSACTION_TIMEOUT = '2h'")
	mustExec(t, ctx, session, "SET LOCAL CLI_VERBOSE = TRUE")
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	ownerA := requireOwner(t, h.tm)
	h.tm.retireMatchingOwner(ownerA)
	if txnContext(h.tm) != nil {
		t.Fatal("pre-entry expiry left owner A")
	}
	mustExec(t, ctx, session, "BEGIN")
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "FALSE" {
		t.Fatalf("entry restore missed LOCAL verbose: %s", got)
	}
	if got := mustGetVar(t, session, "TRANSACTION_TIMEOUT"); got != time.Hour.String() {
		t.Fatalf("entry restore missed LOCAL timeout: %s", got)
	}
	d, captured, armed := ownerTimeout(h.tm)
	if !captured || d != time.Hour || armed {
		t.Fatalf("B snapshot d=%s captured=%v armed=%v, want restored 1h pending", d, captured, armed)
	}
}

func TestTransactionTimeoutInFlightCancelRestoresLocalBeforeNextOwner(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	h.attachSessionClient(session)
	mustExec(t, ctx, session, "SET TRANSACTION_TIMEOUT = '1h'")
	mustExec(t, ctx, session, "BEGIN")
	mustExec(t, ctx, session, "SET LOCAL CLI_VERBOSE = TRUE")
	mustExec(t, ctx, session, "SET LOCAL TRANSACTION_TIMEOUT = '30ms'")
	ownerA := requireOwner(t, h.tm)
	expired := make(chan struct{})
	h.tm.timeoutAfterExpire = func(got *transactionContext) {
		if got == ownerA {
			close(expired)
		}
	}
	block := make(chan struct{})
	inFlight := make(chan struct{})
	h.server.blockBegin = block
	h.server.beginBlocked = sync.OnceFunc(func() { close(inFlight) })
	errCh := make(chan error, 1)
	go func() {
		errCh <- h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED)
	}()
	waitChan(t, inFlight, "constructor BeginTransaction while LOCAL timeout armed")
	err := waitTimeoutErr(t, errCh, "cancelled constructor")
	if !isTimeoutish(err) {
		t.Fatalf("in-flight constructor cancel: %v", err)
	}
	waitChan(t, expired, "owner A retire after in-flight cancel")
	mustExec(t, ctx, session, "BEGIN")
	if txnContext(h.tm) == ownerA {
		t.Fatal("next BEGIN reused expired owner A")
	}
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "FALSE" {
		t.Fatalf("next owner inherited expired LOCAL verbose: %s", got)
	}
	if got := mustGetVar(t, session, "TRANSACTION_TIMEOUT"); got != time.Hour.String() {
		t.Fatalf("next owner inherited expired LOCAL timeout: %s", got)
	}
	d, captured, armed := ownerTimeout(h.tm)
	if !captured || d != time.Hour || armed {
		t.Fatalf("next owner snapshot d=%s captured=%v armed=%v, want restored 1h pending", d, captured, armed)
	}
}

func TestTransactionTimeoutExpiryAfterEntryRestoreOrdinarySET(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	h.attachSessionClient(session)
	mustExec(t, ctx, session, "SET TRANSACTION_TIMEOUT = '1h'")
	mustExec(t, ctx, session, "BEGIN")
	mustExec(t, ctx, session, "SET LOCAL TRANSACTION_TIMEOUT = '2h'")
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	ownerA := requireOwner(t, h.tm)
	expireOwnerAfterEntryRestore(h.tm, ownerA)
	mustExec(t, ctx, session, "SET TRANSACTION_TIMEOUT = '3h'")
	if txnContext(h.tm) != nil {
		t.Fatal("ordinary SET left expired owner A")
	}
	if got := mustGetVar(t, session, "TRANSACTION_TIMEOUT"); got != (3 * time.Hour).String() {
		t.Fatalf("post-statement restore overwrote SET: %s", got)
	}
}

func TestTransactionTimeoutExpiryAfterEntryRestoreFrozenPriorityIsolation(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	h.attachSessionClient(session)
	mustExec(t, ctx, session, "SET RPC_PRIORITY = 'LOW'")
	mustExec(t, ctx, session, "SET DEFAULT_ISOLATION_LEVEL = 'SERIALIZABLE'")
	mustExec(t, ctx, session, "BEGIN")
	mustExec(t, ctx, session, "SET LOCAL RPC_PRIORITY = 'HIGH'")
	mustExec(t, ctx, session, "SET LOCAL DEFAULT_ISOLATION_LEVEL = 'REPEATABLE_READ'")
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	ownerA := requireOwner(t, h.tm)
	expireOwnerAfterEntryRestore(h.tm, ownerA)
	mustExec(t, ctx, session, "BEGIN")
	if txnContext(h.tm) == ownerA {
		t.Fatal("BEGIN reused expired owner A")
	}
	if got := mustGetVar(t, session, "RPC_PRIORITY"); got != "LOW" {
		t.Fatalf("registry kept A's LOCAL priority: %s", got)
	}
	if got := mustGetVar(t, session, "DEFAULT_ISOLATION_LEVEL"); got != "SERIALIZABLE" {
		t.Fatalf("registry kept A's LOCAL isolation: %s", got)
	}
	attrs := h.tm.TransactionAttrsWithLock()
	if attrs.priority != sppb.RequestOptions_PRIORITY_LOW {
		t.Fatalf("B froze A's LOCAL priority: %v", attrs.priority)
	}
	if attrs.isolationLevel != sppb.TransactionOptions_SERIALIZABLE {
		t.Fatalf("B froze A's LOCAL isolation: %v", attrs.isolationLevel)
	}
}

func TestTransactionTimeoutExpiryAfterEntryRestoreStatementTimeout(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	h.attachSessionClient(session)
	mustExec(t, ctx, session, "SET STATEMENT_TIMEOUT = '10m'")
	mustExec(t, ctx, session, "BEGIN")
	mustExec(t, ctx, session, "SET LOCAL STATEMENT_TIMEOUT = '1s'")
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	ownerA := requireOwner(t, h.tm)
	expireOwnerAfterEntryRestore(h.tm, ownerA)
	probe := &statementTimeoutProbe{}
	if _, err := session.ExecuteStatement(ctx, probe); err != nil {
		t.Fatal(err)
	}
	if !probe.ok {
		t.Fatal("statement context had no deadline")
	}
	if probe.remaining < time.Minute {
		t.Fatalf("statement used unrestored LOCAL timeout remaining=%s", probe.remaining)
	}
	if got := mustGetVar(t, session, "STATEMENT_TIMEOUT"); got != (10 * time.Minute).String() {
		t.Fatalf("STATEMENT_TIMEOUT after statement = %s", got)
	}
}

func TestTransactionTimeoutNestedExecutionRestoresBeforeInnerSET(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	h.attachSessionClient(session)
	mustExec(t, ctx, session, "SET TRANSACTION_TIMEOUT = '1h'")
	mustExec(t, ctx, session, "BEGIN")
	mustExec(t, ctx, session, "SET LOCAL TRANSACTION_TIMEOUT = '2h'")
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	ownerA := requireOwner(t, h.tm)
	expireOwnerAfterEntryRestore(h.tm, ownerA)
	if _, err := session.ExecuteStatement(ctx, &nestedSetTimeoutStatement{value: "'3h'"}); err != nil {
		t.Fatal(err)
	}
	if txnContext(h.tm) != nil {
		t.Fatal("nested SET left expired owner A")
	}
	if got := mustGetVar(t, session, "TRANSACTION_TIMEOUT"); got != (3 * time.Hour).String() {
		t.Fatalf("nested SET overwritten by restore: %s", got)
	}
}

type statementTimeoutProbe struct {
	remaining time.Duration
	ok        bool
}

func (s *statementTimeoutProbe) Execute(ctx context.Context, session *Session, out OperationOutput) (*Result, error) {
	if dl, ok := ctx.Deadline(); ok {
		s.remaining = time.Until(dl)
		s.ok = true
	}
	return &Result{KeepVariables: true}, nil
}

type nestedSetTimeoutStatement struct {
	value string
}

func (s *nestedSetTimeoutStatement) Execute(ctx context.Context, session *Session, out OperationOutput) (*Result, error) {
	return session.ExecuteStatement(ctx, &SetStatement{VarName: "TRANSACTION_TIMEOUT", Value: s.value})
}

func TestTransactionTimeoutExpiryAfterLeaveRestoreBeforeDepthDecrement(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	h.attachSessionClient(session)
	mustExec(t, ctx, session, "BEGIN")
	mustExec(t, ctx, session, "SET LOCAL CLI_VERBOSE = TRUE")
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	ownerA := requireOwner(t, h.tm)
	expireOwnerAfterLeaveRestore(h.tm, ownerA)
	mustExec(t, ctx, session, "SHOW VARIABLE CLI_VERBOSE")
	if got := statementDepth(h.tm); got != 0 {
		t.Fatalf("depth after final frame = %d, want 0", got)
	}
	if owner := txnContext(h.tm); owner != nil {
		t.Fatalf("final-frame expiry left owner expirePending=%v", owner.expirePending)
	}
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "FALSE" {
		t.Fatalf("CLI_VERBOSE after final-frame expiry = %s, want FALSE", got)
	}
}

func TestTransactionTimeoutNestedLeaveDoesNotRestoreUntilOuterFrame(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	h.attachSessionClient(session)
	mustExec(t, ctx, session, "BEGIN")
	mustExec(t, ctx, session, "SET LOCAL CLI_VERBOSE = TRUE")
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	ownerA := requireOwner(t, h.tm)
	probe := &nestedLeaveExpiryProbe{owner: ownerA}
	if _, err := session.ExecuteStatement(ctx, probe); err != nil {
		t.Fatal(err)
	}
	if !probe.sawInner {
		t.Fatal("inner leave hook did not run")
	}
	if probe.afterInnerDepth != 1 {
		t.Fatalf("depth after inner leave = %d, want 1", probe.afterInnerDepth)
	}
	if !probe.afterInnerPending || !probe.afterInnerOwner {
		t.Fatalf("inner leave retired A: pending=%v owner=%v", probe.afterInnerPending, probe.afterInnerOwner)
	}
	if probe.afterInnerVerbose != "TRUE" {
		t.Fatalf("inner leave restored LOCAL verbose: %s", probe.afterInnerVerbose)
	}
	if got := statementDepth(h.tm); got != 0 {
		t.Fatalf("depth after outer frame = %d, want 0", got)
	}
	if txnContext(h.tm) != nil {
		t.Fatal("outer leave left expired owner A")
	}
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "FALSE" {
		t.Fatalf("CLI_VERBOSE after outer leave = %s, want FALSE", got)
	}
}

type nestedLeaveExpiryProbe struct {
	owner             *transactionContext
	sawInner          bool
	afterInnerDepth   int
	afterInnerPending bool
	afterInnerOwner   bool
	afterInnerVerbose string
}

func (s *nestedLeaveExpiryProbe) Execute(ctx context.Context, session *Session, out OperationOutput) (*Result, error) {
	session.txn.afterLeaveRestore = func() {
		if statementDepth(session.txn) == 2 {
			s.sawInner = true
			session.txn.watchTransactionDeadline(s.owner, expiredContext())
		}
	}
	if _, err := session.ExecuteStatement(ctx, &ShowVariableStatement{VarName: "CLI_VERBOSE"}); err != nil {
		return nil, err
	}
	s.afterInnerDepth = statementDepth(session.txn)
	s.afterInnerPending, s.afterInnerOwner = ownerExpirePending(session.txn)
	s.afterInnerVerbose = mustGetVarFromSession(session, "CLI_VERBOSE")
	return &Result{KeepVariables: true}, nil
}

func mustGetVarFromSession(session *Session, name string) string {
	value, err := session.systemVariables.Registry.Get(name)
	if err != nil {
		return err.Error()
	}
	return value
}

func statementDepth(tm *TransactionManager) int {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.statementDepth
}

func ownerExpirePending(tm *TransactionManager) (pending, live bool) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if tm.tc == nil {
		return false, false
	}
	return tm.tc.expirePending, true
}

// captureUnarmedTransactionTimeout installs TRANSACTION_TIMEOUT on the live
// owner without starting the deadline. RunQuery's armAndBindDeadlineLocked then
// starts the budget at the blocked ExecuteSql, matching
// TestTransactionTimeoutConstructorCancelWhileMutexHeld (arm at the RPC under
// test, then wait for in-flight). Do not widen the duration or sleep.
func captureUnarmedTransactionTimeout(t *testing.T, tm *TransactionManager, d time.Duration) {
	t.Helper()
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if tm.tc == nil {
		t.Fatal("expected a live logical owner")
	}
	if !tm.tc.deadline.IsZero() {
		t.Fatal("setup started the transaction budget before the blocked query")
	}
	tm.tc.timeout = d
	tm.tc.timeoutCaptured = true
}

func expireOwnerAfterEntryRestore(tm *TransactionManager, owner *transactionContext) {
	tm.afterEntryRestore = func() {
		tm.watchTransactionDeadline(owner, expiredContext())
	}
}

func expireOwnerAfterLeaveRestore(tm *TransactionManager, owner *transactionContext) {
	tm.afterLeaveRestore = func() {
		tm.watchTransactionDeadline(owner, expiredContext())
	}
}

func expiredContext() context.Context {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	cancel()
	return ctx
}

func waitTimeoutErr(t *testing.T, errCh <-chan error, what string) error {
	t.Helper()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case err := <-errCh:
		return err
	case <-timer.C:
		t.Fatalf("timeout waiting for %s", what)
		return nil
	case <-t.Context().Done():
		t.Fatalf("test cancelled waiting for %s: %v", what, t.Context().Err())
		return nil
	}
}

func isTimeoutish(err error) bool {
	return err != nil && (errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, errTransactionTimeout) ||
		status.Code(err) == codes.DeadlineExceeded ||
		status.Code(err) == codes.Canceled ||
		strings.Contains(err.Error(), "deadline") ||
		strings.Contains(err.Error(), "TRANSACTION_TIMEOUT"))
}
