// Copyright 2026 apstndb
//
// Licensed under the MIT License.

package mycli

import (
	"errors"
	"strings"
	"testing"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
)

func TestBuildStatement_ResetLocalRejected(t *testing.T) {
	t.Parallel()
	for _, input := range []string{"RESET LOCAL", "reset local"} {
		_, err := BuildStatement(input)
		if err == nil || !strings.Contains(err.Error(), "RESET LOCAL is not supported") {
			t.Errorf("%q: %v, want RESET LOCAL is not supported", input, err)
		}
	}
}

func TestResetStatementRestoresStartupAndIgnoresInitCommand(t *testing.T) {
	t.Parallel()
	sv, err := initializeSystemVariables(&spannerOptions{
		ProjectId:  "p",
		InstanceId: "i",
		DatabaseId: "d",
		Set: map[string]string{
			"CLI_VERBOSE":            "TRUE",
			"CLI_PROMPT":             "startup> ",
			"DDL_EXECUTION_MODE":     "ASYNC",
			"DDL_ASYNC_WAIT_TIMEOUT": "15s",
		},
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
		t.Fatalf("init-command SET CLI_VERBOSE: %v", err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "CLI_PROMPT", Value: "'init> '"}); err != nil {
		t.Fatalf("init-command SET CLI_PROMPT: %v", err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "DDL_EXECUTION_MODE", Value: "'SYNC'"}); err != nil {
		t.Fatalf("init-command SET DDL_EXECUTION_MODE: %v", err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "DDL_ASYNC_WAIT_TIMEOUT", Value: "'30s'"}); err != nil {
		t.Fatalf("init-command SET DDL_ASYNC_WAIT_TIMEOUT: %v", err)
	}

	res, err := session.ExecuteStatement(ctx, &ResetStatement{VarName: "cli_verbose"})
	if err != nil {
		t.Fatalf("RESET CLI_VERBOSE: %v", err)
	}
	if res == nil || !res.KeepVariables {
		t.Fatal("RESET KeepVariables=false")
	}
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "TRUE" {
		t.Errorf("after RESET CLI_VERBOSE = %q, want TRUE (--set snapshot)", got)
	}
	if got := mustGetVar(t, session, "CLI_PROMPT"); got != "init> " {
		t.Errorf("RESET CLI_VERBOSE changed CLI_PROMPT: %q", got)
	}
	if got := mustGetVar(t, session, "DDL_EXECUTION_MODE"); got != "SYNC" {
		t.Errorf("RESET CLI_VERBOSE changed DDL_EXECUTION_MODE: %q", got)
	}
	if got := mustGetVar(t, session, "DDL_ASYNC_WAIT_TIMEOUT"); got != "30s" {
		t.Errorf("RESET CLI_VERBOSE changed DDL_ASYNC_WAIT_TIMEOUT: %q", got)
	}

	if _, err := session.ExecuteStatement(ctx, &ResetStatement{VarName: "DDL_EXECUTION_MODE"}); err != nil {
		t.Fatalf("RESET DDL_EXECUTION_MODE: %v", err)
	}
	if got := mustGetVar(t, session, "DDL_EXECUTION_MODE"); got != "ASYNC" {
		t.Errorf("after RESET DDL_EXECUTION_MODE = %q, want ASYNC", got)
	}
	if got := mustGetVar(t, session, "DDL_ASYNC_WAIT_TIMEOUT"); got != "30s" {
		t.Errorf("RESET DDL_EXECUTION_MODE changed timeout: %q", got)
	}

	if _, err := session.ExecuteStatement(ctx, &ResetStatement{VarName: "DDL_ASYNC_WAIT_TIMEOUT"}); err != nil {
		t.Fatalf("RESET DDL_ASYNC_WAIT_TIMEOUT: %v", err)
	}
	if got := mustGetVar(t, session, "DDL_ASYNC_WAIT_TIMEOUT"); got != "15s" {
		t.Errorf("after RESET DDL_ASYNC_WAIT_TIMEOUT = %q, want 15s", got)
	}
}

func TestResetStatementAliasResolves(t *testing.T) {
	t.Parallel()
	val := "init"
	session := newSessionForLocalVarTest(t)
	session.systemVariables.featureVarDefs = []varDef{{
		name:    "CLI_TEST_RESET_VAR",
		aliases: []string{"CLI_TEST_RESET_ALIAS"},
		desc:    "test",
		scope:   scopeSession,
		bind:    func(*systemVariables) Variable { return StringVar(&val) },
	}}
	session.systemVariables.Registry = NewVarRegistry(session.systemVariables)
	session.systemVariables.inManualBatch = session.batch.IsActive
	if err := session.systemVariables.CaptureStartupSnapshots(); err != nil {
		t.Fatalf("CaptureStartupSnapshots: %v", err)
	}
	if err := session.systemVariables.SetFromSimple("CLI_TEST_RESET_VAR", "changed"); err != nil {
		t.Fatal(err)
	}

	if _, err := session.ExecuteStatement(t.Context(), &ResetStatement{VarName: "cli_test_reset_alias"}); err != nil {
		t.Fatalf("RESET alias: %v", err)
	}
	if val != "init" {
		t.Errorf("alias reset = %q, want init", val)
	}
}

func TestResetStatementUnknownAndExcluded(t *testing.T) {
	t.Parallel()
	session := newSessionForResetTest(t)
	if err := session.systemVariables.SetFromSimple("CLI_VERBOSE", "TRUE"); err != nil {
		t.Fatal(err)
	}

	_, err := session.ExecuteStatement(t.Context(), &ResetStatement{VarName: "NO_SUCH_VARIABLE"})
	var unknown *ErrUnknownVariable
	if !errors.As(err, &unknown) {
		t.Fatalf("unknown: %v, want ErrUnknownVariable", err)
	}
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "TRUE" {
		t.Errorf("unknown RESET mutated CLI_VERBOSE: %q", got)
	}

	for _, name := range []string{"AUTOCOMMIT", "CLI_VERSION", "CLI_ENABLE_ADC_PLUS", "PROTO_DESCRIPTORS_FILE_PATH"} {
		_, err := session.ExecuteStatement(t.Context(), &ResetStatement{VarName: name})
		if err == nil || !strings.Contains(err.Error(), "does not support RESET") {
			t.Errorf("RESET %s: %v, want does not support RESET", name, err)
		}
	}
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "TRUE" {
		t.Errorf("excluded RESET mutated CLI_VERBOSE: %q", got)
	}
}

func TestResetStatementChangedGuardLeavesOthers(t *testing.T) {
	t.Parallel()
	session := newSessionForResetTest(t)
	ctx := t.Context()
	if err := session.systemVariables.SetFromSimple("CLI_VERBOSE", "TRUE"); err != nil {
		t.Fatal(err)
	}
	if err := session.systemVariables.SetFromSimple("READONLY", "TRUE"); err != nil {
		t.Fatal(err)
	}
	// READONLY=TRUE plus a real BEGIN would need a database client. The
	// txnGuard consults inTransaction, so a fake flag is enough.
	session.systemVariables.inTransaction = func() bool { return true }

	if _, err := session.ExecuteStatement(ctx, &ResetStatement{VarName: "CLI_VERBOSE"}); err != nil {
		t.Fatalf("RESET CLI_VERBOSE during txn: %v", err)
	}
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "FALSE" {
		t.Errorf("CLI_VERBOSE = %q, want FALSE", got)
	}
	if got := mustGetVar(t, session, "READONLY"); got != "TRUE" {
		t.Errorf("RESET CLI_VERBOSE changed READONLY: %q", got)
	}

	_, err := session.ExecuteStatement(ctx, &ResetStatement{VarName: "READONLY"})
	if err == nil || !strings.Contains(err.Error(), "READONLY") {
		t.Fatalf("RESET READONLY: %v, want guard", err)
	}
	if got := mustGetVar(t, session, "READONLY"); got != "TRUE" {
		t.Errorf("rejected RESET READONLY mutated value: %q", got)
	}
}

func TestResetStatementEqualValueRetiresOnlyTargetedUndo(t *testing.T) {
	t.Parallel()
	session := newSessionForResetTest(t)
	ctx := t.Context()

	if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "CLI_VERBOSE", Value: "TRUE"}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "CLI_PROMPT", Value: "'changed> '"}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &BeginStatement{Priority: sppb.RequestOptions_PRIORITY_UNSPECIFIED}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "CLI_VERBOSE", Value: "FALSE"}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "CLI_FORMAT", Value: "'CSV'"}); err != nil {
		t.Fatal(err)
	}
	if outstandingLocalUndo(session.txn) != 2 {
		t.Fatalf("LOCAL undo = %d, want 2", outstandingLocalUndo(session.txn))
	}

	if _, err := session.ExecuteStatement(ctx, &ResetStatement{VarName: "CLI_VERBOSE"}); err != nil {
		t.Fatalf("RESET CLI_VERBOSE: %v", err)
	}
	if outstandingLocalUndo(session.txn) != 1 {
		t.Fatalf("after targeted RESET LOCAL undo = %d, want 1", outstandingLocalUndo(session.txn))
	}
	if got := mustGetVar(t, session, "CLI_FORMAT"); got != "CSV" {
		t.Errorf("unrelated LOCAL CLI_FORMAT = %q, want CSV", got)
	}
	if got := mustGetVar(t, session, "CLI_PROMPT"); got != "changed> " {
		t.Errorf("unrelated CLI_PROMPT = %q, want changed> ", got)
	}

	if _, err := session.ExecuteStatement(ctx, &CommitStatement{}); err != nil {
		t.Fatal(err)
	}
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "FALSE" {
		t.Errorf("after COMMIT CLI_VERBOSE = %q, want FALSE (startup snapshot)", got)
	}
	if got := mustGetVar(t, session, "CLI_FORMAT"); got != "TABLE" {
		t.Errorf("after COMMIT CLI_FORMAT = %q, want TABLE (unrelated LOCAL undo)", got)
	}
}

func TestResetWrapperDoesNotRetireLocalUndo(t *testing.T) {
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
	if err := session.systemVariables.Reset("CLI_VERBOSE"); err != nil {
		t.Fatal(err)
	}
	if outstandingLocalUndo(session.txn) == 0 {
		t.Fatal("Reset(name) wrapper retired LOCAL undo; SQL RESET must use commitPersistentReset")
	}
}

func TestResetStatementRejectedDoesNotRetireUnrelatedUndo(t *testing.T) {
	t.Parallel()
	session := newSessionForResetTest(t)
	ctx := t.Context()
	if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "DIRECTED_READ", Value: "'us-east1'"}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &BeginStatement{Priority: sppb.RequestOptions_PRIORITY_UNSPECIFIED}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "CLI_FORMAT", Value: "'CSV'"}); err != nil {
		t.Fatal(err)
	}

	_, err := session.ExecuteStatement(ctx, &ResetStatement{VarName: "DIRECTED_READ"})
	if err == nil || !strings.Contains(err.Error(), "DIRECTED_READ") {
		t.Fatalf("RESET DIRECTED_READ: %v, want guard", err)
	}
	if got := mustGetVar(t, session, "DIRECTED_READ"); got != "us-east1" {
		t.Errorf("rejected RESET DIRECTED_READ mutated: %q", got)
	}
	if outstandingLocalUndo(session.txn) == 0 {
		t.Fatal("rejected RESET retired unrelated LOCAL undo")
	}
}

func TestResetStatementTransactionTagUsesWritableSlot(t *testing.T) {
	t.Parallel()

	t.Run("consumed nonempty startup is changed and rejected", func(t *testing.T) {
		t.Parallel()
		session := newSessionForResetTestWithTag(t, "startup-tag")
		withFakeReadWriteOwner(t, session, "startup-tag")
		assertTagSurfaces(t, session, "startup-tag")
		if got := nextOwnerTagSlot(session); got != "" {
			t.Fatalf("consumed slot = %q, want empty", got)
		}

		_, err := session.ExecuteStatement(t.Context(), &ResetStatement{VarName: "TRANSACTION_TAG"})
		if err == nil || !errors.Is(err, errTransactionTagInReadWrite) || !strings.Contains(err.Error(), "TRANSACTION_TAG") {
			t.Fatalf("RESET TRANSACTION_TAG: %v, want slot guard", err)
		}
		assertTagSurfaces(t, session, "startup-tag")
		if got := nextOwnerTagSlot(session); got != "" {
			t.Fatalf("rejected RESET restored slot: %q", got)
		}
	})

	t.Run("empty startup already matches consumed slot", func(t *testing.T) {
		t.Parallel()
		session := newSessionForResetTest(t)
		if err := session.systemVariables.SetFromSimple("CLI_VERBOSE", "TRUE"); err != nil {
			t.Fatal(err)
		}
		withFakeReadWriteOwner(t, session, "applied-tag")
		if _, err := session.ExecuteStatement(t.Context(), &ResetStatement{VarName: "TRANSACTION_TAG"}); err != nil {
			t.Fatalf("RESET TRANSACTION_TAG: %v", err)
		}
		assertTagSurfaces(t, session, "applied-tag")
		if got := nextOwnerTagSlot(session); got != "" {
			t.Fatalf("slot mutated: %q", got)
		}
		if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "TRUE" {
			t.Errorf("RESET TRANSACTION_TAG changed CLI_VERBOSE: %q", got)
		}
	})

	t.Run("pending local tag restores slot and retires only tag undo", func(t *testing.T) {
		t.Parallel()
		session := newSessionForResetTestWithTag(t, "startup-tag")
		ctx := t.Context()
		if _, err := session.ExecuteStatement(ctx, &BeginStatement{Priority: sppb.RequestOptions_PRIORITY_UNSPECIFIED}); err != nil {
			t.Fatal(err)
		}
		if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "TRANSACTION_TAG", Value: "'local-tag'"}); err != nil {
			t.Fatal(err)
		}
		if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "CLI_VERBOSE", Value: "TRUE"}); err != nil {
			t.Fatal(err)
		}
		if outstandingLocalUndo(session.txn) != 2 {
			t.Fatalf("LOCAL undo = %d, want 2", outstandingLocalUndo(session.txn))
		}

		if _, err := session.ExecuteStatement(ctx, &ResetStatement{VarName: "TRANSACTION_TAG"}); err != nil {
			t.Fatalf("RESET TRANSACTION_TAG: %v", err)
		}
		if outstandingLocalUndo(session.txn) != 1 {
			t.Fatalf("after RESET tag LOCAL undo = %d, want 1", outstandingLocalUndo(session.txn))
		}
		assertTagSurfaces(t, session, "startup-tag")
		if got := nextOwnerTagSlot(session); got != "startup-tag" {
			t.Fatalf("slot after RESET = %q, want startup-tag", got)
		}
		if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "TRUE" {
			t.Errorf("RESET TRANSACTION_TAG changed CLI_VERBOSE: %q", got)
		}
	})
}
