// Copyright 2026 apstndb
//
// Licensed under the MIT License.

package mycli

import (
	"errors"
	"strings"
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
)

func newSessionForResetTestWithTag(t *testing.T, tag string) *Session {
	t.Helper()
	session := newSessionForLocalVarTest(t)
	session.systemVariables.inManualBatch = session.batch.IsActive
	if tag != "" {
		if err := session.systemVariables.SetFromSimple("TRANSACTION_TAG", tag); err != nil {
			t.Fatal(err)
		}
	}
	if err := session.systemVariables.CaptureStartupSnapshots(); err != nil {
		t.Fatalf("CaptureStartupSnapshots: %v", err)
	}
	return session
}

func nextOwnerTagSlot(session *Session) string {
	if session.systemVariables.transactionTagSlot != nil {
		return session.systemVariables.transactionTagSlot()
	}
	return session.systemVariables.Transaction.TransactionTag
}

// withFakeReadWriteOwnerPreserveUndo is the RESET-test variant of
// withFakeReadWriteOwner: it consumes the next-owner slot and installs a
// physical RW owner while keeping any SET LOCAL undo already on tc.
func withFakeReadWriteOwnerPreserveUndo(t *testing.T, session *Session, tag string) {
	t.Helper()
	session.txn.mu.Lock()
	defer session.txn.mu.Unlock()
	var undo []savedLocalVar
	if session.txn.tc != nil {
		undo = session.txn.tc.localVarUndo
	}
	session.txn.tc = &transactionContext{
		attrs:        transactionAttributes{mode: transactionModeReadWrite, tag: tag},
		localVarUndo: undo,
	}
	if session.systemVariables != nil {
		session.systemVariables.Transaction.TransactionTag = ""
	}
}

func withFakeReadOnlyOwnerPreserveUndo(t *testing.T, session *Session) {
	t.Helper()
	session.txn.mu.Lock()
	defer session.txn.mu.Unlock()
	var undo []savedLocalVar
	if session.txn.tc != nil {
		undo = session.txn.tc.localVarUndo
	}
	session.txn.tc = &transactionContext{
		attrs:        transactionAttributes{mode: transactionModeReadOnly},
		localVarUndo: undo,
	}
}

func TestResetAllConsumedStartupTagRejectedDuringRW(t *testing.T) {
	t.Parallel()
	session := newSessionForResetTestWithTag(t, "startup-tag")
	if err := session.systemVariables.SetFromSimple("CLI_VERBOSE", "TRUE"); err != nil {
		t.Fatal(err)
	}
	withFakeReadWriteOwner(t, session, "startup-tag")
	assertTagSurfaces(t, session, "startup-tag")
	if got := nextOwnerTagSlot(session); got != "" {
		t.Fatalf("consumed slot = %q, want empty", got)
	}

	_, err := session.ExecuteStatement(t.Context(), &ResetAllStatement{})
	if err == nil || !errors.Is(err, errTransactionTagInReadWrite) || !strings.Contains(err.Error(), "TRANSACTION_TAG") {
		t.Fatalf("RESET ALL: %v, want TRANSACTION_TAG guard", err)
	}
	assertTagSurfaces(t, session, "startup-tag")
	if got := nextOwnerTagSlot(session); got != "" {
		t.Fatalf("rejected RESET ALL restored slot: %q", got)
	}
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "TRUE" {
		t.Errorf("rejected RESET ALL changed CLI_VERBOSE: %q", got)
	}
}

func TestResetAllEmptyStartupSlotUnchangedDuringRWAllowsUnrelated(t *testing.T) {
	t.Parallel()
	session := newSessionForResetTest(t)
	if err := session.systemVariables.SetFromSimple("CLI_VERBOSE", "TRUE"); err != nil {
		t.Fatal(err)
	}
	withFakeReadWriteOwner(t, session, "applied-tag")
	assertTagSurfaces(t, session, "applied-tag")
	if got := nextOwnerTagSlot(session); got != "" {
		t.Fatalf("slot = %q, want empty startup", got)
	}

	if _, err := session.ExecuteStatement(t.Context(), &ResetAllStatement{}); err != nil {
		t.Fatalf("RESET ALL: %v, slot already equals empty startup", err)
	}
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "FALSE" {
		t.Errorf("CLI_VERBOSE = %q, want FALSE", got)
	}
	assertTagSurfaces(t, session, "applied-tag")
	if got := nextOwnerTagSlot(session); got != "" {
		t.Fatalf("RESET ALL mutated equal slot: %q", got)
	}
}

func TestResetAllConsumedStartupTagRejectedPreservesLocalUndo(t *testing.T) {
	t.Parallel()
	session := newSessionForResetTestWithTag(t, "startup-tag")
	ctx := t.Context()
	if _, err := session.ExecuteStatement(ctx, &BeginStatement{Priority: sppb.RequestOptions_PRIORITY_UNSPECIFIED}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "CLI_VERBOSE", Value: "TRUE"}); err != nil {
		t.Fatal(err)
	}
	undoBefore := outstandingLocalUndo(session.txn)
	if undoBefore == 0 {
		t.Fatal("expected LOCAL undo")
	}
	withFakeReadWriteOwnerPreserveUndo(t, session, "startup-tag")
	if outstandingLocalUndo(session.txn) != undoBefore {
		t.Fatalf("fake RW dropped LOCAL undo: %d -> %d", undoBefore, outstandingLocalUndo(session.txn))
	}

	_, err := session.ExecuteStatement(ctx, &ResetAllStatement{})
	if err == nil || !errors.Is(err, errTransactionTagInReadWrite) {
		t.Fatalf("RESET ALL: %v, want transaction-tag guard", err)
	}
	if outstandingLocalUndo(session.txn) != undoBefore {
		t.Fatal("rejected RESET ALL retired LOCAL undo")
	}
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "TRUE" {
		t.Errorf("rejected RESET ALL changed CLI_VERBOSE: %q", got)
	}
	assertTagSurfaces(t, session, "startup-tag")
	if got := nextOwnerTagSlot(session); got != "" {
		t.Fatalf("rejected RESET ALL restored slot: %q", got)
	}
}

func TestResetNameTransactionTagBothDirectionsDuringRW(t *testing.T) {
	t.Parallel()

	t.Run("consumed nonempty startup is changed and rejected", func(t *testing.T) {
		t.Parallel()
		session := newSessionForResetTestWithTag(t, "startup-tag")
		withFakeReadWriteOwner(t, session, "startup-tag")
		err := session.systemVariables.Reset("TRANSACTION_TAG")
		if err == nil || !errors.Is(err, errTransactionTagInReadWrite) {
			t.Fatalf("Reset(TRANSACTION_TAG): %v, want guard", err)
		}
		assertTagSurfaces(t, session, "startup-tag")
		if got := nextOwnerTagSlot(session); got != "" {
			t.Fatalf("slot restored: %q", got)
		}
	})

	t.Run("empty startup already matches consumed slot", func(t *testing.T) {
		t.Parallel()
		session := newSessionForResetTest(t)
		withFakeReadWriteOwner(t, session, "applied-tag")
		if err := session.systemVariables.Reset("TRANSACTION_TAG"); err != nil {
			t.Fatalf("Reset(TRANSACTION_TAG): %v", err)
		}
		assertTagSurfaces(t, session, "applied-tag")
		if got := nextOwnerTagSlot(session); got != "" {
			t.Fatalf("slot mutated: %q", got)
		}
	})
}

func TestResetAllPendingChangedTagRestoresSlot(t *testing.T) {
	t.Parallel()
	session := newSessionForResetTestWithTag(t, "startup-tag")
	ctx := t.Context()
	if _, err := session.ExecuteStatement(ctx, &BeginStatement{Priority: sppb.RequestOptions_PRIORITY_UNSPECIFIED}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "TRANSACTION_TAG", Value: "'pending-tag'"}); err != nil {
		t.Fatal(err)
	}
	assertTagSurfaces(t, session, "pending-tag")
	if got := nextOwnerTagSlot(session); got != "pending-tag" {
		t.Fatalf("pending slot = %q", got)
	}

	if _, err := session.ExecuteStatement(ctx, &ResetAllStatement{}); err != nil {
		t.Fatalf("RESET ALL: %v", err)
	}
	assertTagSurfaces(t, session, "startup-tag")
	if got := nextOwnerTagSlot(session); got != "startup-tag" {
		t.Fatalf("slot after RESET ALL = %q, want startup-tag", got)
	}
}

func TestResetAllPendingLocalTagRestoresAndRetiresUndo(t *testing.T) {
	t.Parallel()
	session := newSessionForResetTestWithTag(t, "startup-tag")
	ctx := t.Context()
	if _, err := session.ExecuteStatement(ctx, &BeginStatement{Priority: sppb.RequestOptions_PRIORITY_UNSPECIFIED}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "TRANSACTION_TAG", Value: "'local-tag'"}); err != nil {
		t.Fatal(err)
	}
	assertTagSurfaces(t, session, "local-tag")
	if outstandingLocalUndo(session.txn) == 0 {
		t.Fatal("expected LOCAL undo")
	}

	if _, err := session.ExecuteStatement(ctx, &ResetAllStatement{}); err != nil {
		t.Fatalf("RESET ALL: %v", err)
	}
	if outstandingLocalUndo(session.txn) != 0 {
		t.Fatal("RESET ALL left TRANSACTION_TAG LOCAL undo")
	}
	assertTagSurfaces(t, session, "startup-tag")
	if _, err := session.ExecuteStatement(ctx, &CommitStatement{}); err != nil {
		t.Fatal(err)
	}
	assertTagSurfaces(t, session, "startup-tag")
}

func TestResetAllPendingEqualTagRetiresLocalUndo(t *testing.T) {
	t.Parallel()
	session := newSessionForResetTestWithTag(t, "startup-tag")
	ctx := t.Context()
	if _, err := session.ExecuteStatement(ctx, &BeginStatement{Priority: sppb.RequestOptions_PRIORITY_UNSPECIFIED}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "TRANSACTION_TAG", Value: "'startup-tag'"}); err != nil {
		t.Fatal(err)
	}
	if outstandingLocalUndo(session.txn) == 0 {
		t.Fatal("expected equal-value LOCAL undo")
	}

	if _, err := session.ExecuteStatement(ctx, &ResetAllStatement{}); err != nil {
		t.Fatalf("RESET ALL: %v", err)
	}
	if outstandingLocalUndo(session.txn) != 0 {
		t.Fatal("equal-value RESET ALL left TRANSACTION_TAG LOCAL undo")
	}
	assertTagSurfaces(t, session, "startup-tag")
	if _, err := session.ExecuteStatement(ctx, &CommitStatement{}); err != nil {
		t.Fatal(err)
	}
	assertTagSurfaces(t, session, "startup-tag")
}

func TestResetAllReadOnlyChangedTagRestoresSlot(t *testing.T) {
	t.Parallel()
	session := newSessionForResetTest(t)
	ctx := t.Context()
	if _, err := session.ExecuteStatement(ctx, &BeginStatement{Priority: sppb.RequestOptions_PRIORITY_UNSPECIFIED}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "TRANSACTION_TAG", Value: "'ro-tag'"}); err != nil {
		t.Fatal(err)
	}
	withFakeReadOnlyOwnerPreserveUndo(t, session)
	assertTagSurfaces(t, session, "ro-tag")
	if got := nextOwnerTagSlot(session); got != "ro-tag" {
		t.Fatalf("RO must not consume slot: %q", got)
	}

	if _, err := session.ExecuteStatement(ctx, &ResetAllStatement{}); err != nil {
		t.Fatalf("RESET ALL during RO: %v", err)
	}
	assertTagSurfaces(t, session, "")
	if got := nextOwnerTagSlot(session); got != "" {
		t.Fatalf("slot after RESET ALL = %q, want empty", got)
	}
}

func TestResetAllReadOnlyUnchangedSlotAllowsUnrelated(t *testing.T) {
	t.Parallel()
	session := newSessionForResetTestWithTag(t, "startup-tag")
	if err := session.systemVariables.SetFromSimple("CLI_VERBOSE", "TRUE"); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(t.Context(), &BeginStatement{Priority: sppb.RequestOptions_PRIORITY_UNSPECIFIED}); err != nil {
		t.Fatal(err)
	}
	withFakeReadOnlyOwnerPreserveUndo(t, session)
	assertTagSurfaces(t, session, "startup-tag")
	if got := nextOwnerTagSlot(session); got != "startup-tag" {
		t.Fatalf("RO consumed slot: %q", got)
	}

	if _, err := session.ExecuteStatement(t.Context(), &ResetAllStatement{}); err != nil {
		t.Fatalf("RESET ALL during RO: %v", err)
	}
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "FALSE" {
		t.Errorf("CLI_VERBOSE = %q, want FALSE", got)
	}
	assertTagSurfaces(t, session, "startup-tag")
	if got := nextOwnerTagSlot(session); got != "startup-tag" {
		t.Fatalf("unchanged RO slot mutated: %q", got)
	}
}

func TestResetAllReadOnlyLocalTagRestoresAndRetiresUndo(t *testing.T) {
	t.Parallel()
	session := newSessionForResetTest(t)
	ctx := t.Context()
	if _, err := session.ExecuteStatement(ctx, &BeginStatement{Priority: sppb.RequestOptions_PRIORITY_UNSPECIFIED}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "TRANSACTION_TAG", Value: "'ro-local'"}); err != nil {
		t.Fatal(err)
	}
	withFakeReadOnlyOwnerPreserveUndo(t, session)
	if outstandingLocalUndo(session.txn) == 0 {
		t.Fatal("expected LOCAL undo")
	}
	assertTagSurfaces(t, session, "ro-local")

	if _, err := session.ExecuteStatement(ctx, &ResetAllStatement{}); err != nil {
		t.Fatalf("RESET ALL: %v", err)
	}
	if outstandingLocalUndo(session.txn) != 0 {
		t.Fatal("RESET ALL left TRANSACTION_TAG LOCAL undo")
	}
	assertTagSurfaces(t, session, "")
	if got := nextOwnerTagSlot(session); got != "" {
		t.Fatalf("slot = %q, want empty startup", got)
	}
}

func TestCaptureStartupSnapshotsRecordsTransactionTagSlot(t *testing.T) {
	t.Parallel()
	session := newSessionForResetTestWithTag(t, "startup-tag")
	if got := session.systemVariables.startupSnapshots["TRANSACTION_TAG"]; got != "startup-tag" {
		t.Fatalf("captured TRANSACTION_TAG = %q, want startup-tag", got)
	}
	withFakeReadWriteOwner(t, session, "startup-tag")
	assertTagSurfaces(t, session, "startup-tag")
	got, err := variableResetValue(session.systemVariables.Registry.GetVariable("TRANSACTION_TAG"))
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("ResetSnapshot after consume = %q, want empty slot", got)
	}
	show, err := session.systemVariables.Registry.Get("TRANSACTION_TAG")
	if err != nil || show != "startup-tag" {
		t.Fatalf("SHOW after consume = %q (%v), want startup-tag", show, err)
	}
}
