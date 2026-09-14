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
	if got := mustGetVar(t, session, "TRANSACTION_TIMEOUT"); got != (time.Hour).String() {
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
	session := sessionForTM(t, h.tm)
	mustExec(t, ctx, session, "SET TRANSACTION_TIMEOUT = '30ms'")
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
	if got := mustGetVar(t, session, "TRANSACTION_TIMEOUT"); got != (time.Hour).String() {
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
