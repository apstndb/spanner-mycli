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
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func ownerIdle(tm *TransactionManager) (d time.Duration, captured, userWork, armed bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	if tm.tc == nil {
		return 0, false, false, false
	}
	return tm.tc.idle, tm.tc.idleCaptured, tm.tc.idleUserWork, tm.tc.idleCancel != nil
}

func ownerIdleGen(tm *TransactionManager) uint64 {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	if tm.tc == nil {
		return 0
	}
	return tm.tc.idleGen
}

func fireIdleNow(tm *TransactionManager) {
	tm.mu.RLock()
	owner := tm.tc
	var gen uint64
	if owner != nil {
		gen = owner.idleGen
	}
	tm.mu.RUnlock()
	if owner == nil {
		return
	}
	tm.watchIdleDeadline(owner, gen, expiredContext())
}

func waitIdleExpire(t *testing.T, expired <-chan struct{}, what string) {
	t.Helper()
	waitChan(t, expired, what)
}

func armIdleOwner(t *testing.T, h *heartbeatHarness, session *Session, keepalive bool) *transactionContext {
	t.Helper()
	ctx := t.Context()
	mustExec(t, ctx, session, "SET CLI_IDLE_TRANSACTION_TIMEOUT = '1h'")
	if !keepalive {
		mustExec(t, ctx, session, "SET KEEP_TRANSACTION_ALIVE = FALSE")
	}
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	d, captured, userWork, armed := ownerIdle(h.tm)
	if !captured || d != time.Hour || !userWork || !armed {
		t.Fatalf("BEGIN RW idle d=%s captured=%v userWork=%v armed=%v", d, captured, userWork, armed)
	}
	return requireOwner(t, h.tm)
}

func TestIdleTransactionTimeoutRegistryAndLocal(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	ctx := t.Context()
	if got := mustGetVar(t, session, "CLI_IDLE_TRANSACTION_TIMEOUT"); got != "NULL" {
		t.Fatalf("default = %q, want NULL", got)
	}
	mustExec(t, ctx, session, "SET CLI_IDLE_TRANSACTION_TIMEOUT = '30s'")
	if got := mustGetVar(t, session, "CLI_IDLE_TRANSACTION_TIMEOUT"); got != "30s" {
		t.Fatalf("SET 30s = %q", got)
	}
	mustExec(t, ctx, session, "SET CLI_IDLE_TRANSACTION_TIMEOUT = NULL")
	if got := mustGetVar(t, session, "CLI_IDLE_TRANSACTION_TIMEOUT"); got != "NULL" {
		t.Fatalf("SET NULL = %q", got)
	}
}

func TestIdleTransactionTimeoutPendingDoesNotArm(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	ctx := t.Context()
	mustExec(t, ctx, session, "SET CLI_IDLE_TRANSACTION_TIMEOUT = '10s'")
	mustExec(t, ctx, session, "BEGIN")
	mustExec(t, ctx, session, "SHOW VARIABLE CLI_IDLE_TRANSACTION_TIMEOUT")
	mustExec(t, ctx, session, "SET LOCAL CLI_VERBOSE = TRUE")
	d, captured, userWork, armed := ownerIdle(session.txn)
	if !captured || d != 10*time.Second || userWork || armed {
		t.Fatalf("pending idle d=%s captured=%v userWork=%v armed=%v", d, captured, userWork, armed)
	}
	fireIdleNow(session.txn)
	if txnContext(session.txn) == nil {
		t.Fatal("unarmed pending owner was retired")
	}
}

func TestIdleTransactionTimeoutSessionSETAfterBeginDoesNotChangeOwner(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	ctx := t.Context()
	mustExec(t, ctx, session, "SET CLI_IDLE_TRANSACTION_TIMEOUT = '10s'")
	mustExec(t, ctx, session, "BEGIN")
	mustExec(t, ctx, session, "SET CLI_IDLE_TRANSACTION_TIMEOUT = '1h'")
	d, captured, _, _ := ownerIdle(session.txn)
	if !captured || d != 10*time.Second {
		t.Fatalf("session SET mutated owner snapshot d=%s captured=%v", d, captured)
	}
	if got := mustGetVar(t, session, "CLI_IDLE_TRANSACTION_TIMEOUT"); got != time.Hour.String() {
		t.Fatalf("session value = %q", got)
	}
}

func TestIdleTransactionTimeoutPendingLocalSelectsDuration(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	ctx := t.Context()
	mustExec(t, ctx, session, "SET CLI_IDLE_TRANSACTION_TIMEOUT = '1h'")
	mustExec(t, ctx, session, "BEGIN")
	mustExec(t, ctx, session, "SET LOCAL CLI_IDLE_TRANSACTION_TIMEOUT = '45s'")
	d, captured, userWork, armed := ownerIdle(session.txn)
	if !captured || d != 45*time.Second || userWork || armed {
		t.Fatalf("pending local snapshot d=%s captured=%v userWork=%v armed=%v", d, captured, userWork, armed)
	}
}

func TestIdleTransactionTimeoutRejectsLocalChangeAfterWork(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	armIdleOwner(t, h, session, true)
	_, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "CLI_IDLE_TRANSACTION_TIMEOUT", Value: "'10s'"})
	if err == nil || !errors.Is(err, errIdleTransactionFrozen) {
		t.Fatalf("SET LOCAL after work: %v", err)
	}
	mustExec(t, ctx, session, "SET LOCAL CLI_IDLE_TRANSACTION_TIMEOUT = '1h'")
}

func TestIdleTransactionTimeoutBeginRWAndROArmQuietInterval(t *testing.T) {
	t.Parallel()
	t.Run("rw", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := sessionForTM(t, h.tm)
		armIdleOwner(t, h, session, true)
	})
	t.Run("ro", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		ctx := t.Context()
		session := sessionForTM(t, h.tm)
		mustExec(t, ctx, session, "SET CLI_IDLE_TRANSACTION_TIMEOUT = '1h'")
		if _, err := h.tm.BeginReadOnlyTransaction(ctx, timestampBoundUnspecified, 0, time.Time{}, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
			t.Fatal(err)
		}
		d, captured, userWork, armed := ownerIdle(h.tm)
		if !captured || d != time.Hour || !userWork || !armed {
			t.Fatalf("BEGIN RO idle d=%s captured=%v userWork=%v armed=%v", d, captured, userWork, armed)
		}
	})
}

func TestIdleTransactionTimeoutFailedConstructorPreservesPending(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	mustExec(t, ctx, session, "SET CLI_IDLE_TRANSACTION_TIMEOUT = '1h'")
	mustExec(t, ctx, session, "BEGIN")
	pending := requireOwner(t, h.tm)
	h.server.failBegin = status.Error(codes.PermissionDenied, "injected constructor failure")
	err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED)
	if err == nil || !strings.Contains(err.Error(), "injected constructor failure") {
		t.Fatalf("constructor: %v", err)
	}
	if txnContext(h.tm) != pending {
		t.Fatal("failed constructor replaced the pending owner")
	}
	_, _, userWork, armed := ownerIdle(h.tm)
	if userWork || armed {
		t.Fatalf("failed pending constructor armed idle userWork=%v armed=%v", userWork, armed)
	}
}

func TestIdleTransactionTimeoutFailedIdleConstructorLeavesNoOwner(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	mustExec(t, ctx, session, "SET CLI_IDLE_TRANSACTION_TIMEOUT = '1h'")
	h.server.failBegin = status.Error(codes.PermissionDenied, "idle constructor failure")
	err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED)
	if err == nil || !strings.Contains(err.Error(), "idle constructor failure") {
		t.Fatalf("idle constructor: %v", err)
	}
	if txnContext(h.tm) != nil {
		t.Fatal("failed idle constructor left a logical owner")
	}
}

func TestIdleTransactionTimeoutZeroAndNullDoNotArm(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"NULL", "0s"} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			h := newHeartbeatHarness(t)
			ctx := t.Context()
			session := sessionForTM(t, h.tm)
			if value == "NULL" {
				mustExec(t, ctx, session, "SET CLI_IDLE_TRANSACTION_TIMEOUT = NULL")
			} else {
				mustExec(t, ctx, session, "SET CLI_IDLE_TRANSACTION_TIMEOUT = '0s'")
			}
			if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
				t.Fatal(err)
			}
			_, captured, userWork, armed := ownerIdle(h.tm)
			if !captured || !userWork || armed {
				t.Fatalf("disabled idle captured=%v userWork=%v armed=%v", captured, userWork, armed)
			}
			fireIdleNow(h.tm)
			if txnContext(h.tm) == nil {
				t.Fatal("disabled idle retired the owner")
			}
		})
	}
}

func TestIdleTransactionTimeoutExpiresWithKeepaliveDisabled(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	owner := armIdleOwner(t, h, session, false)
	if h.tm.heartbeatEnabled() {
		t.Fatal("keepalive-disabled owner scheduled a heartbeat")
	}
	expired := make(chan struct{})
	h.tm.idleAfterExpire = func(got *transactionContext) {
		if got == owner {
			close(expired)
		}
	}
	fireIdleNow(h.tm)
	waitIdleExpire(t, expired, "idle expire with keepalive disabled")
	if txnContext(h.tm) != nil {
		t.Fatal("idle expiry left an owner")
	}
}

func TestIdleTransactionTimeoutHeartbeatDoesNotReset(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	owner := armIdleOwner(t, h, session, true)
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1 AS keep", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	gen := ownerIdleGen(h.tm)
	h.installBeforeAcquireBarrier()
	sendTick(t, h.ticks)
	waitChan(t, h.arrived, "heartbeat acquire")
	h.releaseBarriers()
	if txnContext(h.tm) != owner {
		t.Fatal("heartbeat replaced the owner")
	}
	if ownerIdleGen(h.tm) != gen {
		t.Fatal("heartbeat rearmed idle")
	}
	expired := make(chan struct{})
	h.tm.idleAfterExpire = func(got *transactionContext) {
		if got == owner {
			close(expired)
		}
	}
	fireIdleNow(h.tm)
	waitIdleExpire(t, expired, "idle expire after heartbeat")
}

func TestIdleTransactionTimeoutShowAndSetDoNotReset(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	armIdleOwner(t, h, session, true)
	gen := ownerIdleGen(h.tm)
	mustExec(t, ctx, session, "SHOW VARIABLE CLI_IDLE_TRANSACTION_TIMEOUT")
	mustExec(t, ctx, session, "SHOW TRANSACTION ISOLATION LEVEL")
	mustExec(t, ctx, session, "SET CLI_VERBOSE = TRUE")
	if ownerIdleGen(h.tm) != gen {
		t.Fatal("SHOW/SET rearmed idle")
	}
}

func TestIdleTransactionTimeoutBufferedWorkRearms(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	h.tm.enableSavepointCaptureForTest()
	armIdleOwner(t, h, session, true)
	gen := ownerIdleGen(h.tm)
	ok, err := h.tm.TryEnqueueAutomaticDML(spanner.NewStatement("INSERT INTO t (id) VALUES (1)"))
	if err != nil || !ok {
		t.Fatalf("TryEnqueueAutomaticDML: ok=%v err=%v", ok, err)
	}
	if ownerIdleGen(h.tm) == gen {
		t.Fatal("buffered DML did not rearm idle")
	}
	gen = ownerIdleGen(h.tm)
	if err := h.tm.CreateSavepoint(ctx, "sp1"); err != nil {
		t.Fatal(err)
	}
	if ownerIdleGen(h.tm) == gen {
		t.Fatal("SAVEPOINT did not rearm idle")
	}
}

func TestIdleTransactionTimeoutRejectedAdmissionDoesNotReset(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	h.tm.enableSavepointCaptureForTest()
	armIdleOwner(t, h, session, true)
	if err := h.tm.CreateSavepoint(ctx, "keep"); err != nil {
		t.Fatal(err)
	}
	gen := ownerIdleGen(h.tm)
	if err := h.tm.RollbackToSavepoint(ctx, "unknown"); err == nil {
		t.Fatal("unknown savepoint succeeded")
	}
	if ownerIdleGen(h.tm) != gen {
		t.Fatal("rejected SAVEPOINT admission rearmed idle")
	}
}

func TestIdleTransactionTimeoutResultHoldAndLongRPC(t *testing.T) {
	t.Parallel()
	t.Run("hold without fire keeps deadline", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := sessionForTM(t, h.tm)
		owner := armIdleOwner(t, h, session, true)
		gen := ownerIdleGen(h.tm)
		h.tm.beginIdleResultHold()
		if txnContext(h.tm) != owner {
			t.Fatal("result-hold dropped the owner")
		}
		h.tm.endIdleResultHold()
		if txnContext(h.tm) != owner {
			t.Fatal("result-hold release retired the owner")
		}
		if ownerIdleGen(h.tm) != gen {
			t.Fatal("non-activity hold rearmed idle")
		}
		_, _, _, armed := ownerIdle(h.tm)
		if !armed {
			t.Fatal("non-activity hold disarmed idle")
		}
	})
	t.Run("elapsed hold retires at barrier", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := sessionForTM(t, h.tm)
		owner := armIdleOwner(t, h, session, true)
		expired := make(chan struct{})
		h.tm.idleAfterExpire = func(got *transactionContext) {
			if got == owner {
				close(expired)
			}
		}
		h.tm.beginIdleResultHold()
		fireIdleNow(h.tm)
		if txnContext(h.tm) != owner {
			t.Fatal("result-hold allowed idle retire")
		}
		select {
		case <-expired:
			t.Fatal("result-hold retired the owner")
		default:
		}
		h.tm.endIdleResultHold()
		waitIdleExpire(t, expired, "idle expire at result-hold barrier")
		if txnContext(h.tm) != nil {
			t.Fatal("elapsed non-activity hold left an owner")
		}
	})
	t.Run("elapsed hold with work rearms", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		session := sessionForTM(t, h.tm)
		owner := armIdleOwner(t, h, session, true)
		gen := ownerIdleGen(h.tm)
		h.tm.beginIdleResultHold()
		h.tm.noteIdleUserWork(true)
		fireIdleNow(h.tm)
		if txnContext(h.tm) != owner {
			t.Fatal("held work allowed idle retire")
		}
		h.tm.endIdleResultHold()
		if txnContext(h.tm) != owner {
			t.Fatal("completed work expired at the hold barrier")
		}
		if ownerIdleGen(h.tm) == gen {
			t.Fatal("completed work did not rearm idle")
		}
	})
	t.Run("long rpc expires at leave", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		ctx := t.Context()
		session := sessionForTM(t, h.tm)
		owner := armIdleOwner(t, h, session, true)
		expired := make(chan struct{})
		h.tm.idleAfterExpire = func(got *transactionContext) {
			if got == owner {
				close(expired)
			}
		}
		h.tm.enterStatement()
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
		waitChan(t, inFlight, "query RPC")
		fireIdleNow(h.tm)
		if txnContext(h.tm) != owner {
			t.Fatal("in-flight query allowed idle retire")
		}
		close(block)
		if err := waitTimeoutErr(t, errCh, "held query"); err != nil {
			t.Fatal(err)
		}
		h.tm.leaveStatement()
		waitIdleExpire(t, expired, "idle expire after RPC hold")
	})
}

func TestIdleTransactionTimeoutTombstoneIsOneShot(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	owner := armIdleOwner(t, h, session, true)
	expired := make(chan struct{})
	h.tm.idleAfterExpire = func(got *transactionContext) {
		if got == owner {
			close(expired)
		}
	}
	fireIdleNow(h.tm)
	waitIdleExpire(t, expired, "idle expire")

	_, err := execSQL(t, ctx, session, "SELECT 1")
	if err == nil || !errors.Is(err, errIdleTransactionTimeout) {
		t.Fatalf("first command after idle: %v", err)
	}
	_, err = execSQL(t, ctx, session, "SELECT 1")
	if errors.Is(err, errIdleTransactionTimeout) {
		t.Fatalf("tombstone leaked to a later command: %v", err)
	}

	mustExec(t, ctx, session, "BEGIN")
	ownerB := requireOwner(t, h.tm)
	if ownerB == owner {
		t.Fatal("BEGIN reused expired owner")
	}
	_, err = execSQL(t, ctx, session, "SELECT 1")
	if errors.Is(err, errIdleTransactionTimeout) {
		t.Fatalf("replacement owner inherited idle notice: %v", err)
	}
}

func TestIdleTransactionTimeoutAckStatements(t *testing.T) {
	t.Parallel()
	t.Run("rollback", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		ctx := t.Context()
		session := sessionForTM(t, h.tm)
		owner := armIdleOwner(t, h, session, true)
		expired := make(chan struct{})
		h.tm.idleAfterExpire = func(got *transactionContext) {
			if got == owner {
				close(expired)
			}
		}
		fireIdleNow(h.tm)
		waitIdleExpire(t, expired, "idle expire")
		_, err := execSQL(t, ctx, session, "ROLLBACK")
		if errors.Is(err, errIdleTransactionTimeout) {
			t.Fatalf("ROLLBACK inherited idle notice: %v", err)
		}
	})
	t.Run("begin", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		ctx := t.Context()
		session := sessionForTM(t, h.tm)
		owner := armIdleOwner(t, h, session, true)
		expired := make(chan struct{})
		h.tm.idleAfterExpire = func(got *transactionContext) {
			if got == owner {
				close(expired)
			}
		}
		fireIdleNow(h.tm)
		waitIdleExpire(t, expired, "idle expire")
		mustExec(t, ctx, session, "BEGIN RW")
		if txnContext(h.tm) == owner {
			t.Fatal("BEGIN RW reused expired owner")
		}
	})
	t.Run("detach", func(t *testing.T) {
		t.Parallel()
		h := newHeartbeatHarness(t)
		ctx := t.Context()
		session := sessionForTM(t, h.tm)
		owner := armIdleOwner(t, h, session, true)
		expired := make(chan struct{})
		h.tm.idleAfterExpire = func(got *transactionContext) {
			if got == owner {
				close(expired)
			}
		}
		fireIdleNow(h.tm)
		waitIdleExpire(t, expired, "idle expire")
		handler := NewSessionHandler(session)
		handler.constructCandidate = func(context.Context, ConnectionVars) (*Session, error) {
			return nil, errors.New("stop after idle ack")
		}
		_, err := handler.ExecuteStatement(ctx, &DetachStatement{})
		if errors.Is(err, errIdleTransactionTimeout) {
			t.Fatalf("DETACH inherited idle notice: %v", err)
		}
		if err == nil || !strings.Contains(err.Error(), "stop after idle ack") {
			t.Fatalf("DETACH after idle: %v", err)
		}
	})
}

func TestIdleTransactionTimeoutStaleCallbackDoesNotRetireReplacement(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	ownerA := armIdleOwner(t, h, session, true)
	if err := h.tm.RollbackReadWriteTransaction(ctx); err != nil {
		t.Fatal(err)
	}
	h.tm.restoreLocalVarsIfIdle()
	mustExec(t, ctx, session, "SET CLI_IDLE_TRANSACTION_TIMEOUT = '2h'")
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	ownerB := requireOwner(t, h.tm)
	if ownerB == ownerA {
		t.Fatal("replacement reused owner A")
	}
	h.tm.watchIdleDeadline(ownerA, 1, expiredContext())
	if txnContext(h.tm) != ownerB {
		t.Fatal("stale idle callback retired replacement")
	}
}

func TestIdleTransactionTimeoutSavepointPreservesOwner(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	h.tm.enableSavepointCaptureForTest()
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	owner := armIdleOwner(t, h, session, true)
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1 AS keep", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	if err := h.tm.CreateSavepoint(ctx, "keep"); err != nil {
		t.Fatal(err)
	}
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1 AS extra", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	mustExec(t, ctx, session, "SET CLI_IDLE_TRANSACTION_TIMEOUT = '10s'")
	if err := h.tm.RollbackToSavepoint(ctx, "keep"); err != nil {
		t.Fatal(err)
	}
	if txnContext(h.tm) != owner {
		t.Fatal("ROLLBACK TO replaced the logical owner")
	}
	d, captured, userWork, armed := ownerIdle(h.tm)
	if !captured || d != time.Hour || !userWork || !armed {
		t.Fatalf("reconstructed idle d=%s captured=%v userWork=%v armed=%v", d, captured, userWork, armed)
	}
}

func TestIdleTransactionTimeoutDoesNotCancelInFlightWhenTotalTimeoutWins(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	mustExec(t, ctx, session, "SET CLI_IDLE_TRANSACTION_TIMEOUT = '1h'")
	mustExec(t, ctx, session, "SET TRANSACTION_TIMEOUT = '1h'")
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	owner := requireOwner(t, h.tm)
	totalExpired := make(chan struct{})
	h.tm.timeoutAfterExpire = func(got *transactionContext) {
		if got == owner {
			close(totalExpired)
		}
	}
	idleExpired := make(chan struct{})
	h.tm.idleAfterExpire = func(got *transactionContext) {
		if got == owner {
			close(idleExpired)
		}
	}

	h.tm.enterStatement()
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
	waitChan(t, inFlight, "query RPC")
	fireIdleNow(h.tm)
	select {
	case <-idleExpired:
		t.Fatal("idle retired an in-flight owner")
	default:
	}
	h.tm.watchTransactionDeadline(owner, expiredContext())
	waitChan(t, totalExpired, "TRANSACTION_TIMEOUT retire")
	close(block)
	_ = waitTimeoutErr(t, errCh, "cancelled query")
	h.tm.leaveStatement()
	select {
	case <-idleExpired:
		t.Fatal("idle tombstone after TRANSACTION_TIMEOUT precedence")
	default:
	}
	if h.tm.consumeIdleNotice(false) != nil {
		t.Fatal("idle notice set after total-timeout retire")
	}
}

type idleFireWriter struct {
	tm    *TransactionManager
	t     *testing.T
	once  sync.Once
	buf   bytes.Buffer
	wrote bool
	live  bool
}

func (w *idleFireWriter) Write(p []byte) (int, error) {
	w.once.Do(func() {
		w.wrote = true
		if w.t != nil && txnContext(w.tm) != nil {
			w.live = true
		}
		fireIdleNow(w.tm)
		if w.t != nil && txnContext(w.tm) == nil {
			w.live = false
		}
	})
	return w.buf.Write(p)
}

func testCLI(session *Session) *Cli {
	session.systemVariables.Display.Verbose = true
	return &Cli{
		SessionHandler:  NewSessionHandler(session),
		SystemVariables: session.systemVariables,
	}
}

func TestIdleTransactionTimeoutCLIShowDoesNotRestartIdle(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	owner := armIdleOwner(t, h, session, true)
	expired := make(chan struct{})
	h.tm.idleAfterExpire = func(got *transactionContext) {
		if got == owner {
			close(expired)
		}
	}
	w := &idleFireWriter{tm: h.tm}
	cli := testCLI(session)
	if _, err := cli.executeStatement(ctx, &ShowVariableStatement{VarName: "CLI_IDLE_TRANSACTION_TIMEOUT"}, false, "SHOW VARIABLE CLI_IDLE_TRANSACTION_TIMEOUT", w); err != nil {
		t.Fatal(err)
	}
	if !w.wrote {
		t.Fatal("expected SHOW result output")
	}
	waitIdleExpire(t, expired, "idle expire after client-only SHOW")
	if txnContext(h.tm) != nil {
		t.Fatalf("SHOW crossing the old deadline left owner gen=%d", ownerIdleGen(h.tm))
	}
}

func TestIdleTransactionTimeoutCLIBeginRWHoldCoversNewOwner(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	mustExec(t, ctx, session, "SET CLI_IDLE_TRANSACTION_TIMEOUT = '1h'")
	w := &idleFireWriter{tm: h.tm, t: t}
	cli := testCLI(session)
	if _, err := cli.executeStatement(ctx, &BeginRwStatement{}, false, "BEGIN RW", w); err != nil {
		t.Fatal(err)
	}
	if !w.wrote {
		t.Fatal("expected BEGIN RW result output")
	}
	if !w.live {
		t.Fatal("BEGIN RW owner expired while CLI result output was still running")
	}
	if txnContext(h.tm) == nil {
		t.Fatal("BEGIN RW owner was retired after CLI output")
	}
	_, _, userWork, armed := ownerIdle(h.tm)
	if !userWork || !armed {
		t.Fatalf("BEGIN RW after CLI output userWork=%v armed=%v", userWork, armed)
	}
}

func TestIdleTransactionTimeoutManualBufferedDMLRearms(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	armIdleOwner(t, h, session, true)
	mustExec(t, ctx, session, "START BATCH DML")
	gen := ownerIdleGen(h.tm)
	mustExec(t, ctx, session, "INSERT INTO t (id) VALUES (1)")
	if ownerIdleGen(h.tm) == gen {
		t.Fatal("manual buffered DML did not rearm idle")
	}

	gen = ownerIdleGen(h.tm)
	_, err := execSQL(t, ctx, session, "INSERT INTO t (id) VALUES (1) THEN RETURN id")
	if err == nil || !errors.Is(err, errReturningDMLNotSupportedInBatch) {
		t.Fatalf("THEN RETURN: %v", err)
	}
	if ownerIdleGen(h.tm) != gen {
		t.Fatal("THEN RETURN rearmed idle")
	}

	_, err = session.ExecuteStatement(ctx, &DmlStatement{Dml: "INSERT INTO t (id) VALUES ('"})
	if err == nil {
		t.Fatal("rejected DML input succeeded")
	}
	if ownerIdleGen(h.tm) != gen {
		t.Fatal("rejected DML input rearmed idle")
	}
}
