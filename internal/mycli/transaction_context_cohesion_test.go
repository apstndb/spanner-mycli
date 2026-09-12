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
	"strings"
	"testing"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestTransactionContextIdentityMatrix(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	t.Run("idle to pending allocates", func(t *testing.T) {
		t.Parallel()
		session := newSessionForLocalVarTest(t)
		if txnContext(session.txn) != nil {
			t.Fatal("idle session already had a transactionContext")
		}
		if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
			t.Fatal(err)
		}
		owner := txnContext(session.txn)
		if owner == nil {
			t.Fatal("BEGIN did not allocate a transactionContext")
		}
		if owner.attrs.mode != transactionModePending {
			t.Fatalf("mode = %q, want pending", owner.attrs.mode)
		}
	})

	t.Run("failed activation preserves pending identity state tag and undo", func(t *testing.T) {
		t.Parallel()
		session := newSessionForLocalVarTest(t)
		if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "TRANSACTION_TAG", Value: "'keep'"}); err != nil {
			t.Fatal(err)
		}
		if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
			t.Fatal(err)
		}
		pending := txnContext(session.txn)
		if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "CLI_VERBOSE", Value: "TRUE"}); err != nil {
			t.Fatal(err)
		}
		undoBefore := outstandingLocalUndo(session.txn)
		if undoBefore == 0 {
			t.Fatal("SET LOCAL did not record undo on the pending context")
		}

		err := session.txn.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED)
		if err == nil || !strings.Contains(err.Error(), "database operation requires a database connection") {
			t.Fatalf("BEGIN RW: %v", err)
		}
		if txnContext(session.txn) != pending {
			t.Fatal("failed RW activation replaced pending identity")
		}
		if pending.attrs.mode != transactionModePending {
			t.Fatalf("mode after failed RW = %q, want pending", pending.attrs.mode)
		}
		if pending.txn != nil {
			t.Fatal("failed RW activation installed an SDK handle")
		}
		if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "TRUE" {
			t.Fatalf("failed RW activation restored LOCAL: %s", got)
		}
		if outstandingLocalUndo(session.txn) != undoBefore {
			t.Fatal("failed RW activation changed SET LOCAL undo")
		}
		assertTagSurfaces(t, session, "keep")

		_, err = session.txn.BeginReadOnlyTransaction(ctx, timestampBoundUnspecified, 0, time.Time{}, sppb.RequestOptions_PRIORITY_UNSPECIFIED)
		if err == nil || !strings.Contains(err.Error(), "database operation requires a database connection") {
			t.Fatalf("BEGIN RO: %v", err)
		}
		if txnContext(session.txn) != pending {
			t.Fatal("failed RO activation replaced pending identity")
		}
		if pending.attrs.mode != transactionModePending {
			t.Fatalf("mode after failed RO = %q, want pending", pending.attrs.mode)
		}
		if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "TRUE" {
			t.Fatalf("failed RO activation restored LOCAL: %s", got)
		}
		assertTagSurfaces(t, session, "keep")

		if _, err := session.ExecuteStatement(ctx, &RollbackStatement{}); err != nil {
			t.Fatal(err)
		}
		if txnContext(session.txn) != nil {
			t.Fatal("ROLLBACK left a transactionContext")
		}
		if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "FALSE" {
			t.Fatalf("ROLLBACK after failed activation: %s", got)
		}
		assertTagSurfaces(t, session, "keep")
	})

	t.Run("active to idle retires exactly once and next begin is a new identity", func(t *testing.T) {
		t.Parallel()
		session := newSessionForLocalVarTest(t)
		if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
			t.Fatal(err)
		}
		ownerA := txnContext(session.txn)
		if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "CLI_VERBOSE", Value: "TRUE"}); err != nil {
			t.Fatal(err)
		}
		if _, err := session.ExecuteStatement(ctx, &RollbackStatement{}); err != nil {
			t.Fatal(err)
		}
		if txnContext(session.txn) != nil {
			t.Fatal("terminal path left transactionContext")
		}
		if outstandingLocalUndo(session.txn) != 0 {
			t.Fatal("terminal path left SET LOCAL undo")
		}
		session.txn.clearTransactionContext()
		if txnContext(session.txn) != nil || outstandingLocalUndo(session.txn) != 0 {
			t.Fatal("repeated retire was not a no-op")
		}
		if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "FALSE" {
			t.Fatalf("repeated retire changed restored value: %s", got)
		}

		if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
			t.Fatal(err)
		}
		ownerB := txnContext(session.txn)
		if ownerB == nil || ownerB == ownerA {
			t.Fatal("idle-to-pending reused the previous identity")
		}
	})
}

func TestTransactionContextPendingActivationKeepsIdentityAndUndo(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	session := &Session{
		mode:            DatabaseConnected,
		systemVariables: h.tm.sysVars,
		connection:      h.tm.sysVars.Connection,
		txn:             h.tm,
	}
	h.tm.sysVars.ensureRegistry()
	h.tm.sysVars.inTransaction = h.tm.InTransaction

	if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
		t.Fatal(err)
	}
	pending := txnContext(h.tm)
	if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "CLI_VERBOSE", Value: "TRUE"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.tm.DetermineTransaction(ctx); err != nil {
		t.Fatalf("pending activation: %v", err)
	}
	active := txnContext(h.tm)
	if active != pending {
		t.Fatal("pending activation replaced transactionContext")
	}
	if active.attrs.mode != transactionModeReadWrite {
		t.Fatalf("mode after activation = %q, want read-write", active.attrs.mode)
	}
	if active.txn == nil {
		t.Fatal("activation did not install an SDK handle")
	}
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "TRUE" {
		t.Fatalf("SET LOCAL did not survive pending activation: %s", got)
	}
	if outstandingLocalUndo(h.tm) == 0 {
		t.Fatal("pending activation dropped SET LOCAL undo")
	}

	if err := h.tm.RollbackReadWriteTransaction(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	h.tm.restoreLocalVarsIfIdle()
	if txnContext(h.tm) == pending {
		t.Fatal("terminal path reused the retired identity")
	}
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "FALSE" {
		t.Fatalf("rollback after activation: %s", got)
	}
}

func TestTransactionContextPendingActivationReadOnlyKeepsIdentity(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	if err := h.tm.BeginPendingTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	pending := txnContext(h.tm)
	if _, err := h.tm.BeginReadOnlyTransaction(ctx, timestampBoundUnspecified, 0, time.Time{}, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatalf("pending RO activation: %v", err)
	}
	active := txnContext(h.tm)
	if active != pending {
		t.Fatal("pending RO activation replaced transactionContext")
	}
	if active.attrs.mode != transactionModeReadOnly {
		t.Fatalf("mode after RO activation = %q, want read-only", active.attrs.mode)
	}
	if err := h.tm.CloseReadOnlyTransaction(); err != nil {
		t.Fatalf("close RO: %v", err)
	}
	if txnContext(h.tm) == pending {
		t.Fatal("RO close reused the retired identity")
	}
}

func sessionForTM(t *testing.T, tm *TransactionManager) *Session {
	t.Helper()
	tm.sysVars.ensureRegistry()
	tm.sysVars.inTransaction = tm.InTransaction
	return &Session{
		mode:            DatabaseConnected,
		systemVariables: tm.sysVars,
		connection:      tm.sysVars.Connection,
		txn:             tm,
	}
}

func TestTransactionContextFailedROActivationPreservesPending(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	const injected = "injected RO activation failure for pending identity"

	if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "TRANSACTION_TAG", Value: "'keep'"}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
		t.Fatal(err)
	}
	pending := txnContext(h.tm)
	if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "CLI_VERBOSE", Value: "TRUE"}); err != nil {
		t.Fatal(err)
	}
	undoBefore := outstandingLocalUndo(h.tm)
	if undoBefore == 0 {
		t.Fatal("SET LOCAL did not record undo on the pending context")
	}

	h.server.setFailROQuery(status.Error(codes.PermissionDenied, injected))
	_, err := h.tm.BeginReadOnlyTransaction(ctx, timestampBoundUnspecified, 0, time.Time{}, sppb.RequestOptions_PRIORITY_UNSPECIFIED)
	if err == nil || !strings.Contains(err.Error(), injected) {
		t.Fatalf("RO activation: %v", err)
	}
	if txnContext(h.tm) != pending {
		t.Fatal("failed RO activation replaced pending identity")
	}
	if pending.attrs.mode != transactionModePending {
		t.Fatalf("mode after failed RO = %q, want pending", pending.attrs.mode)
	}
	if pending.txn != nil {
		t.Fatal("failed RO activation installed an SDK handle")
	}
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "TRUE" {
		t.Fatalf("failed RO activation restored LOCAL: %s", got)
	}
	if outstandingLocalUndo(h.tm) != undoBefore {
		t.Fatal("failed RO activation changed SET LOCAL undo")
	}
	assertTagSurfaces(t, session, "keep")

	h.server.setFailROQuery(nil)
	if _, err := h.tm.BeginReadOnlyTransaction(ctx, timestampBoundUnspecified, 0, time.Time{}, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatalf("RO retry: %v", err)
	}
	active := txnContext(h.tm)
	if active != pending {
		t.Fatal("successful RO retry replaced pending identity")
	}
	if active.attrs.mode != transactionModeReadOnly {
		t.Fatalf("mode after RO retry = %q, want read-only", active.attrs.mode)
	}
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "TRUE" {
		t.Fatalf("SET LOCAL did not survive RO retry: %s", got)
	}
	assertTagSurfaces(t, session, "keep")
}
