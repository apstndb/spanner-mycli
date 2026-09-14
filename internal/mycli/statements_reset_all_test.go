// Copyright 2026 apstndb
//
// Licensed under the MIT License.

package mycli

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/internal/mycli/streamio"
)

func newSessionForResetTest(t *testing.T) *Session {
	t.Helper()
	session := newSessionForLocalVarTest(t)
	session.systemVariables.inManualBatch = session.batch.IsActive
	if err := session.systemVariables.CaptureStartupSnapshots(); err != nil {
		t.Fatalf("CaptureStartupSnapshots: %v", err)
	}
	return session
}

func TestBuildStatement_ResetSingleVariableNotResetAll(t *testing.T) {
	t.Parallel()
	got, err := BuildStatement("RESET CLI_VERBOSE")
	if err == nil {
		if _, ok := got.(*ResetAllStatement); ok {
			t.Fatal("RESET CLI_VERBOSE parsed as RESET ALL; single-variable RESET belongs to #960")
		}
	}
}

func TestResetAllStatementRestoresStartupAndIgnoresInitCommand(t *testing.T) {
	t.Parallel()
	sv, err := initializeSystemVariables(&spannerOptions{
		ProjectId:  "p",
		InstanceId: "i",
		DatabaseId: "d",
		Set:        map[string]string{"CLI_VERBOSE": "TRUE"},
	})
	if err != nil {
		t.Fatalf("initializeSystemVariables: %v", err)
	}
	session := &Session{
		mode:            DatabaseConnected,
		systemVariables: sv,
		connection:      sv.Connection,
		txn:             NewTransactionManager(nil, sv, spanner.ClientConfig{}),
	}
	sv.inTransaction = session.txn.InTransaction
	sv.inManualBatch = session.batch.IsActive
	ctx := t.Context()

	if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "CLI_VERBOSE", Value: "FALSE"}); err != nil {
		t.Fatalf("init-command SET: %v", err)
	}
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "FALSE" {
		t.Fatalf("after init-command CLI_VERBOSE = %q, want FALSE", got)
	}

	res, err := session.ExecuteStatement(ctx, &ResetAllStatement{})
	if err != nil {
		t.Fatalf("RESET ALL: %v", err)
	}
	if res == nil || !res.KeepVariables {
		t.Fatal("RESET ALL KeepVariables=false")
	}
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "TRUE" {
		t.Errorf("after RESET ALL CLI_VERBOSE = %q, want TRUE (--set snapshot)", got)
	}
}

func TestResetAllEqualValueRetiresLocalUndo(t *testing.T) {
	t.Parallel()
	session := newSessionForResetTest(t)
	ctx := t.Context()

	if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "CLI_VERBOSE", Value: "TRUE"}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &BeginStatement{Priority: sppb.RequestOptions_PRIORITY_UNSPECIFIED}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "CLI_VERBOSE", Value: "FALSE"}); err != nil {
		t.Fatal(err)
	}
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "FALSE" {
		t.Fatalf("during LOCAL CLI_VERBOSE = %q, want FALSE", got)
	}
	if outstandingLocalUndo(session.txn) == 0 {
		t.Fatal("expected LOCAL undo before RESET ALL")
	}

	if _, err := session.ExecuteStatement(ctx, &ResetAllStatement{}); err != nil {
		t.Fatalf("RESET ALL: %v", err)
	}
	if outstandingLocalUndo(session.txn) != 0 {
		t.Fatal("equal-value RESET ALL left LOCAL undo")
	}
	if _, err := session.ExecuteStatement(ctx, &CommitStatement{}); err != nil {
		t.Fatal(err)
	}
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "FALSE" {
		t.Errorf("after COMMIT CLI_VERBOSE = %q, want FALSE (startup snapshot)", got)
	}
}

func TestResetAllRejectedDoesNotRetireUndoOrValues(t *testing.T) {
	t.Parallel()
	session := newSessionForResetTest(t)
	ctx := t.Context()

	if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "CLI_VERBOSE", Value: "TRUE"}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "CLI_SAVEPOINT_SUPPORT", Value: "'ENABLED'"}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &BeginStatement{Priority: sppb.RequestOptions_PRIORITY_UNSPECIFIED}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "CLI_VERBOSE", Value: "FALSE"}); err != nil {
		t.Fatal(err)
	}
	if outstandingLocalUndo(session.txn) == 0 {
		t.Fatal("expected LOCAL undo")
	}

	_, err := session.ExecuteStatement(ctx, &ResetAllStatement{})
	if err == nil || !strings.Contains(err.Error(), "CLI_SAVEPOINT_SUPPORT") {
		t.Fatalf("RESET ALL: %v, want CLI_SAVEPOINT_SUPPORT guard", err)
	}
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "FALSE" {
		t.Errorf("rejected RESET ALL changed CLI_VERBOSE: %q", got)
	}
	if got := mustGetVar(t, session, "CLI_SAVEPOINT_SUPPORT"); got != "ENABLED" {
		t.Errorf("rejected RESET ALL changed CLI_SAVEPOINT_SUPPORT: %q", got)
	}
	if outstandingLocalUndo(session.txn) == 0 {
		t.Fatal("rejected RESET ALL retired LOCAL undo")
	}

	if _, err := session.ExecuteStatement(ctx, &CommitStatement{}); err != nil {
		t.Fatal(err)
	}
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "TRUE" {
		t.Errorf("after COMMIT CLI_VERBOSE = %q, want TRUE (LOCAL undo still applied)", got)
	}
}

func TestResetAllManualBatchGuard(t *testing.T) {
	t.Parallel()
	session := newSessionForResetTest(t)
	ctx := t.Context()

	if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "CLI_SAVEPOINT_SUPPORT", Value: "'ENABLED'"}); err != nil {
		t.Fatal(err)
	}
	if err := session.batch.Start(batchModeDML); err != nil {
		t.Fatal(err)
	}
	_, err := session.ExecuteStatement(ctx, &ResetAllStatement{})
	if err == nil || !strings.Contains(err.Error(), "CLI_SAVEPOINT_SUPPORT") {
		t.Fatalf("RESET ALL during batch: %v, want savepoint batch guard", err)
	}
	if got := mustGetVar(t, session, "CLI_SAVEPOINT_SUPPORT"); got != "ENABLED" {
		t.Errorf("rejected RESET ALL changed CLI_SAVEPOINT_SUPPORT: %q", got)
	}
	session.batch.Abort()
	if _, err := session.ExecuteStatement(ctx, &ResetAllStatement{}); err != nil {
		t.Fatalf("RESET ALL after abort: %v", err)
	}
	if got := mustGetVar(t, session, "CLI_SAVEPOINT_SUPPORT"); got != "DISABLED" {
		t.Errorf("after RESET ALL CLI_SAVEPOINT_SUPPORT = %q, want DISABLED", got)
	}
}

func TestResetAllPreservesOutputStream(t *testing.T) {
	t.Parallel()
	session := newSessionForResetTest(t)
	var out bytes.Buffer
	sm := streamio.NewStreamManager(io.NopCloser(strings.NewReader("")), &out, io.Discard)
	session.systemVariables.StreamManager = sm
	if err := session.systemVariables.SetFromSimple("CLI_VERBOSE", "TRUE"); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(t.Context(), &ResetAllStatement{}); err != nil {
		t.Fatal(err)
	}
	if session.systemVariables.StreamManager != sm {
		t.Fatal("RESET ALL replaced StreamManager")
	}
	if _, err := session.systemVariables.StreamManager.GetWriter().Write([]byte("ok")); err != nil {
		t.Fatal(err)
	}
	if out.String() != "ok" {
		t.Errorf("stream output = %q, want ok", out.String())
	}
}

func TestResetAllDirectedReadAndUnrelatedUndo(t *testing.T) {
	t.Parallel()
	session := newSessionForResetTest(t)
	ctx := t.Context()

	if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "DIRECTED_READ", Value: "'us-east1'"}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "CLI_PROMPT", Value: "'changed> '"}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &BeginStatement{Priority: sppb.RequestOptions_PRIORITY_UNSPECIFIED}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "CLI_FORMAT", Value: "'CSV'"}); err != nil {
		t.Fatal(err)
	}

	_, err := session.ExecuteStatement(ctx, &ResetAllStatement{})
	if err == nil || !strings.Contains(err.Error(), "DIRECTED_READ") {
		t.Fatalf("RESET ALL: %v, want DIRECTED_READ guard", err)
	}
	if got := mustGetVar(t, session, "CLI_PROMPT"); got != "changed> " {
		t.Errorf("rejected RESET ALL changed CLI_PROMPT: %q", got)
	}
	if outstandingLocalUndo(session.txn) == 0 {
		t.Fatal("rejected RESET ALL retired unrelated LOCAL undo")
	}
}

func TestResetNameRetiresEqualValueUndo(t *testing.T) {
	t.Parallel()
	session := newSessionForResetTest(t)
	ctx := t.Context()

	if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "CLI_VERBOSE", Value: "TRUE"}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &BeginStatement{Priority: sppb.RequestOptions_PRIORITY_UNSPECIFIED}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "CLI_VERBOSE", Value: "FALSE"}); err != nil {
		t.Fatal(err)
	}

	prep, err := session.systemVariables.Registry.prepareReset([]string{"CLI_VERBOSE"})
	if err != nil {
		t.Fatal(err)
	}
	if len(prep.assignments) != 0 {
		t.Fatalf("equal-value Reset prepared %d assignments, want 0", len(prep.assignments))
	}
	if err := commitPersistentReset(session, prep); err != nil {
		t.Fatal(err)
	}
	if outstandingLocalUndo(session.txn) != 0 {
		t.Fatal("equal-value Reset(name) left LOCAL undo")
	}
	if _, err := session.ExecuteStatement(ctx, &CommitStatement{}); err != nil {
		t.Fatal(err)
	}
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "FALSE" {
		t.Errorf("after COMMIT CLI_VERBOSE = %q, want FALSE", got)
	}
}
