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
	"sync"
	"testing"
)

func assertTagSurfaces(t *testing.T, session *Session, want string) {
	t.Helper()
	got := mustGetVar(t, session, "TRANSACTION_TAG")
	if got != want {
		t.Fatalf("Get TRANSACTION_TAG=%q want=%q", got, want)
	}
	if listed := session.systemVariables.ListVariables()["TRANSACTION_TAG"]; listed != want {
		t.Fatalf("ListVariables TRANSACTION_TAG=%q want=%q", listed, want)
	}
	res, err := session.ExecuteStatement(t.Context(), &ShowVariableStatement{VarName: "TRANSACTION_TAG"})
	if err != nil {
		t.Fatalf("SHOW VARIABLE TRANSACTION_TAG: %v", err)
	}
	if len(res.Rows) != 1 || len(res.Rows[0]) != 1 || res.Rows[0][0].RawText() != want {
		t.Fatalf("SHOW VARIABLE TRANSACTION_TAG rows=%v want %q", res.Rows, want)
	}
}

func withFakeReadWriteOwner(t *testing.T, session *Session, tag string) {
	t.Helper()
	session.txn.mu.Lock()
	defer session.txn.mu.Unlock()
	session.txn.tc = &transactionContext{
		attrs: transactionAttributes{mode: transactionModeReadWrite, tag: tag},
	}
	if session.systemVariables != nil {
		session.systemVariables.Transaction.TransactionTag = ""
	}
}

func TestTransactionTagSlotWithoutSession(t *testing.T) {
	t.Parallel()
	sv := newSystemVariablesWithDefaultsForTest()
	sv.ensureRegistry()
	if err := sv.SetFromSimple("TRANSACTION_TAG", "pre-session"); err != nil {
		t.Fatal(err)
	}
	got, err := sv.Get("TRANSACTION_TAG")
	if err != nil || got["TRANSACTION_TAG"] != "pre-session" {
		t.Fatalf("Get: %v %v", got, err)
	}
	if sv.ListVariables()["TRANSACTION_TAG"] != "pre-session" {
		t.Fatalf("ListVariables: %q", sv.ListVariables()["TRANSACTION_TAG"])
	}
}

func TestTransactionTagPendingSetAndLocal(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	ctx := t.Context()
	if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "TRANSACTION_TAG", Value: "'nope'"}); err == nil {
		t.Fatal("idle SET LOCAL succeeded")
	}
	if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "TRANSACTION_TAG", Value: "'pending-tag'"}); err != nil {
		t.Fatal(err)
	}
	if got := mustGetVar(t, session, "TRANSACTION_TAG"); got != "pending-tag" {
		t.Fatalf("pending SET: %q", got)
	}
	assertTagSurfaces(t, session, "pending-tag")
	if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "TRANSACTION_TAG", Value: "'local-tag'"}); err != nil {
		t.Fatal(err)
	}
	if got := mustGetVar(t, session, "TRANSACTION_TAG"); got != "local-tag" {
		t.Fatalf("pending LOCAL: %q", got)
	}
	if _, err := session.ExecuteStatement(ctx, &RollbackStatement{}); err != nil {
		t.Fatal(err)
	}
	if got := mustGetVar(t, session, "TRANSACTION_TAG"); got != "pending-tag" {
		t.Fatalf("pending rollback restore: %q", got)
	}
}

func TestTransactionTagConcurrentSlotAccess(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			_ = session.systemVariables.SetFromSimple("TRANSACTION_TAG", "race")
			_, _ = session.systemVariables.Get("TRANSACTION_TAG")
			_ = session.systemVariables.ListVariables()["TRANSACTION_TAG"]
		})
	}
	wg.Wait()
}

func TestTransactionTagAppliedOwnerRejectsSet(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	if err := session.systemVariables.SetFromSimple("TRANSACTION_TAG", "applied"); err != nil {
		t.Fatal(err)
	}
	withFakeReadWriteOwner(t, session, "applied")
	assertTagSurfaces(t, session, "applied")
	err := session.systemVariables.SetFromSimple("TRANSACTION_TAG", "too-late")
	if !errors.Is(err, errTransactionTagInReadWrite) {
		t.Fatalf("late SET: %v", err)
	}
	err = session.systemVariables.SetFromSimple("TRANSACTION_TAG", "")
	if !errors.Is(err, errTransactionTagInReadWrite) {
		t.Fatalf("late empty SET: %v", err)
	}
	if _, err := session.ExecuteStatement(t.Context(), &SetLocalStatement{VarName: "TRANSACTION_TAG", Value: "'local'"}); err == nil {
		t.Fatal("late LOCAL succeeded")
	}
	assertTagSurfaces(t, session, "applied")
}

func TestTransactionTagFailedBeginWithoutClientPreservesSlot(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	ctx := t.Context()
	if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "TRANSACTION_TAG", Value: "'keep'"}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &BeginRwStatement{}); err == nil {
		t.Fatal("BEGIN RW with nil client succeeded")
	}
	assertTagSurfaces(t, session, "keep")
}

func TestTransactionTagCloseRestoresPendingLocal(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	ctx := t.Context()
	if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "TRANSACTION_TAG", Value: "'A'"}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "TRANSACTION_TAG", Value: "'B'"}); err != nil {
		t.Fatal(err)
	}
	assertTagSurfaces(t, session, "B")
	session.Close()
	assertTagSurfaces(t, session, "A")
}

func TestTransactionTagConcurrentAppliedOwnerAccess(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	withFakeReadWriteOwner(t, session, "applied")
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			_, _ = session.systemVariables.Get("TRANSACTION_TAG")
			_ = session.systemVariables.ListVariables()["TRANSACTION_TAG"]
			err := session.systemVariables.SetFromSimple("TRANSACTION_TAG", "race")
			if !errors.Is(err, errTransactionTagInReadWrite) {
				t.Errorf("SET during physical RW: %v", err)
			}
		})
	}
	wg.Wait()
	assertTagSurfaces(t, session, "applied")
}
