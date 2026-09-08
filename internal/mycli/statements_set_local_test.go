// Copyright 2026 apstndb
//
// Licensed under the MIT License.

package mycli

import (
	"context"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// newSessionForLocalVarTest builds a Session wired for the pending-transaction
// lifecycle (BEGIN -> ... -> COMMIT/ROLLBACK on a pending transaction performs
// no RPCs), so SET LOCAL semantics can be tested without an emulator.
func newSessionForLocalVarTest(t *testing.T) *Session {
	t.Helper()
	sysVars := newSystemVariablesWithDefaultsForTest()
	sysVars.ensureRegistry()
	session := &Session{
		mode:            DatabaseConnected,
		systemVariables: sysVars,
		txn:             NewTransactionManager(nil, sysVars, spanner.ClientConfig{}),
	}
	sysVars.inTransaction = session.txn.InTransaction
	return session
}

func mustGetVar(t *testing.T, session *Session, name string) string {
	t.Helper()
	value, err := session.systemVariables.Registry.Get(name)
	if err != nil {
		t.Fatalf("Registry.Get(%q) error: %v", name, err)
	}
	return value
}

func TestSetLocalRequiresTransaction(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	ctx := t.Context()

	_, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "CLI_VERBOSE", Value: "TRUE"})
	if err == nil || !strings.Contains(err.Error(), "requires an active transaction") {
		t.Errorf("SET LOCAL outside transaction: got error %v, want 'requires an active transaction'", err)
	}
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "FALSE" {
		t.Errorf("CLI_VERBOSE changed by failed SET LOCAL: got %q, want FALSE", got)
	}
}

func TestSetLocalRevertsOnTransactionEnd(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		desc    string
		endStmt Statement
	}{
		{desc: "COMMIT", endStmt: &CommitStatement{}},
		{desc: "ROLLBACK", endStmt: &RollbackStatement{}},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			t.Parallel()
			session := newSessionForLocalVarTest(t)
			ctx := t.Context()

			if _, err := session.ExecuteStatement(ctx, &BeginStatement{Priority: sppb.RequestOptions_PRIORITY_UNSPECIFIED}); err != nil {
				t.Fatalf("BEGIN error: %v", err)
			}

			if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "CLI_VERBOSE", Value: "TRUE"}); err != nil {
				t.Fatalf("SET LOCAL error: %v", err)
			}
			if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "TRUE" {
				t.Errorf("CLI_VERBOSE during transaction: got %q, want TRUE", got)
			}

			if _, err := session.ExecuteStatement(ctx, tt.endStmt); err != nil {
				t.Fatalf("%s error: %v", tt.desc, err)
			}
			if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "FALSE" {
				t.Errorf("CLI_VERBOSE after %s: got %q, want FALSE (reverted)", tt.desc, got)
			}
		})
	}
}

func TestSetLocalTwiceRestoresOriginalValue(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	ctx := t.Context()

	if err := session.systemVariables.SetFromSimple("CLI_PROMPT", "original> "); err != nil {
		t.Fatalf("SET CLI_PROMPT error: %v", err)
	}

	if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
		t.Fatalf("BEGIN error: %v", err)
	}
	for _, value := range []string{`'first> '`, `'second> '`} {
		if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "CLI_PROMPT", Value: value}); err != nil {
			t.Fatalf("SET LOCAL CLI_PROMPT = %s error: %v", value, err)
		}
	}
	if got := mustGetVar(t, session, "CLI_PROMPT"); got != "second> " {
		t.Errorf("CLI_PROMPT during transaction: got %q, want %q", got, "second> ")
	}

	if _, err := session.ExecuteStatement(ctx, &RollbackStatement{}); err != nil {
		t.Fatalf("ROLLBACK error: %v", err)
	}
	if got := mustGetVar(t, session, "CLI_PROMPT"); got != "original> " {
		t.Errorf("CLI_PROMPT after ROLLBACK: got %q, want %q (first saved value wins)", got, "original> ")
	}
}

func TestSetLocalRejectsUnsupportedVariables(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		desc    string
		varName string
		value   string
		wantErr string
	}{
		{desc: "read-only variable", varName: "CLI_VERSION", value: "'v1'", wantErr: "does not support SET LOCAL"},
		{desc: "unknown variable", varName: "NO_SUCH_VARIABLE", value: "1", wantErr: "unknown variable name"},
		// COMMIT_RESPONSE is now a read-only registry def (scopeResult), so SET
		// LOCAL rejects it via localAllowed() rather than the old name check.
		{desc: "multi-valued read-only variable", varName: "COMMIT_RESPONSE", value: "1", wantErr: "does not support SET LOCAL"},
		{desc: "transaction-guarded variable", varName: "DIRECTED_READ", value: "'us-east1'", wantErr: "cannot be changed within a transaction"},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			t.Parallel()
			session := newSessionForLocalVarTest(t)
			ctx := t.Context()

			if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
				t.Fatalf("BEGIN error: %v", err)
			}
			_, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: tt.varName, Value: tt.value})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("SET LOCAL %s: got error %v, want containing %q", tt.varName, err, tt.wantErr)
			}
		})
	}
}

func TestOrdinarySetPersistsAfterSetLocal(t *testing.T) {
	t.Parallel()
	for _, variable := range []struct{ name, initial, local, session string }{
		{"CLI_PROMPT", "original> ", "local> ", "session> "},
		{"CLI_FORMAT", "TABLE", "CSV", "JSONL"},
	} {
		for _, end := range []Statement{&CommitStatement{}, &RollbackStatement{}} {
			for _, tc := range []struct {
				name        string
				kinds       []string
				wantSession bool
			}{
				{"local_only_control", []string{"LOCAL"}, false},
				{"session_only_control", []string{"SESSION"}, true},
				{"session_then_local_control", []string{"SESSION", "LOCAL"}, true},
				{"local_then_session", []string{"LOCAL", "SESSION"}, true},
				{"local_session_local", []string{"LOCAL", "SESSION", "LOCAL"}, true},
			} {
				t.Run(variable.name+"/"+endName(end)+"/"+tc.name, func(t *testing.T) {
					t.Parallel()
					session := newSessionForLocalVarTest(t)
					ctx := t.Context()
					if err := session.systemVariables.SetFromSimple(variable.name, variable.initial); err != nil {
						t.Fatal(err)
					}
					if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
						t.Fatal(err)
					}
					for _, kind := range tc.kinds {
						if kind == "LOCAL" {
							if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: variable.name, Value: quoteGoogleSQL(variable.local)}); err != nil {
								t.Fatal(err)
							}
						} else if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: variable.name, Value: quoteGoogleSQL(variable.session)}); err != nil {
							t.Fatal(err)
						}
					}
					if _, err := session.ExecuteStatement(ctx, end); err != nil {
						t.Fatal(err)
					}
					want := variable.initial
					if tc.wantSession {
						want = variable.session
					}
					if got := mustGetVar(t, session, variable.name); got != want {
						t.Errorf("got %q want %q", got, want)
					}
				})
			}
		}
	}
}

func TestOrdinarySetFailedLeavesLocalUndo(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	ctx := t.Context()
	if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "CLI_VERBOSE", Value: "TRUE"}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "CLI_VERBOSE", Value: "'not-a-bool'"}); err == nil {
		t.Fatal("invalid SET succeeded")
	}
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "TRUE" {
		t.Fatalf("failed SET changed current value: %s", got)
	}
	if _, err := session.ExecuteStatement(ctx, &RollbackStatement{}); err != nil {
		t.Fatal(err)
	}
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "FALSE" {
		t.Fatalf("failed SET retired LOCAL undo: %s", got)
	}
}

func TestOrdinarySetRetiresCanonicalAliasUndo(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	rv := session.systemVariables.Registry.vars["CLI_FORMAT"]
	session.systemVariables.Registry.putUnique("CLI_FMT", rv.def.name, rv)
	ctx := t.Context()
	if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "cli_fmt", Value: "'CSV'"}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "CLI_FORMAT", Value: "'JSONL'"}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &RollbackStatement{}); err != nil {
		t.Fatal(err)
	}
	if got := mustGetVar(t, session, "CLI_FORMAT"); got != "JSONL" {
		t.Fatalf("alias/case SET did not persist: %s", got)
	}
}

func TestOrdinarySetDoesNotRetireUnrelatedLocal(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	ctx := t.Context()
	if err := session.systemVariables.SetFromSimple("CLI_PROMPT", "original> "); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "CLI_PROMPT", Value: "'local> '"}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "CLI_VERBOSE", Value: "TRUE"}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "CLI_PROMPT", Value: "'session> '"}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &RollbackStatement{}); err != nil {
		t.Fatal(err)
	}
	if got := mustGetVar(t, session, "CLI_PROMPT"); got != "session> " {
		t.Fatalf("PROMPT = %s", got)
	}
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "FALSE" {
		t.Fatalf("VERBOSE undo was retired: %s", got)
	}
}

func TestOrdinarySetPersistsOnSessionClose(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	ctx := t.Context()
	if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "CLI_TYPE_STYLES", Value: "'STRING=green'"}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "CLI_TYPE_STYLES", Value: "'INT64=bold'"}); err != nil {
		t.Fatal(err)
	}
	session.Close()
	if got := mustGetVar(t, session, "CLI_TYPE_STYLES"); got != "INT64=bold" {
		t.Fatalf("Close restored LOCAL over SET: %s", got)
	}
	if session.systemVariables.typeStyles[sppb.TypeCode_INT64] == "" {
		t.Fatal("derived typeStyles not kept after Close")
	}
}

func TestSessionCloseRestoresOutstandingLocal(t *testing.T) {
	t.Parallel()

	t.Run("mixed remaining LOCAL restores while ordinary SET remains", func(t *testing.T) {
		t.Parallel()
		session := newSessionForLocalVarTest(t)
		ctx := t.Context()
		wantPrompt := mustGetVar(t, session, "CLI_PROMPT")
		wantStyles := mustGetVar(t, session, "CLI_TYPE_STYLES")
		wantDerived := session.systemVariables.typeStyles[sppb.TypeCode_STRING]
		if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
			t.Fatal(err)
		}
		if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "CLI_PROMPT", Value: "'local> '"}); err != nil {
			t.Fatal(err)
		}
		if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "CLI_TYPE_STYLES", Value: "'STRING=green'"}); err != nil {
			t.Fatal(err)
		}
		if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "CLI_FORMAT", Value: "'CSV'"}); err != nil {
			t.Fatal(err)
		}
		if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "CLI_FORMAT", Value: "'JSONL'"}); err != nil {
			t.Fatal(err)
		}
		session.Close()
		if got := mustGetVar(t, session, "CLI_PROMPT"); got != wantPrompt {
			t.Fatalf("Close left unrelated LOCAL CLI_PROMPT: got %q want %q", got, wantPrompt)
		}
		if got := mustGetVar(t, session, "CLI_TYPE_STYLES"); got != wantStyles {
			t.Fatalf("Close left LOCAL CLI_TYPE_STYLES: got %q want %q", got, wantStyles)
		}
		if session.systemVariables.typeStyles[sppb.TypeCode_STRING] != wantDerived {
			t.Fatal("Close did not restore derived typeStyles")
		}
		if got := mustGetVar(t, session, "CLI_FORMAT"); got != "JSONL" {
			t.Fatalf("Close lost session SET CLI_FORMAT: %q", got)
		}
		if n := len(session.txn.localVarUndo); n != 0 {
			t.Fatalf("Close left %d stale undo entries", n)
		}
		session.Close()
		if n := len(session.txn.localVarUndo); n != 0 {
			t.Fatalf("repeated Close left %d undo entries", n)
		}
		if got := mustGetVar(t, session, "CLI_FORMAT"); got != "JSONL" {
			t.Fatalf("repeated Close changed session SET: %q", got)
		}
	})

	t.Run("LOCAL after ordinary SET restores the session value", func(t *testing.T) {
		t.Parallel()
		session := newSessionForLocalVarTest(t)
		ctx := t.Context()
		if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
			t.Fatal(err)
		}
		if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "CLI_FORMAT", Value: "'JSONL'"}); err != nil {
			t.Fatal(err)
		}
		if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "CLI_FORMAT", Value: "'CSV'"}); err != nil {
			t.Fatal(err)
		}
		session.Close()
		if got := mustGetVar(t, session, "CLI_FORMAT"); got != "JSONL" {
			t.Fatalf("Close left LOCAL over session SET: %q", got)
		}
		if n := len(session.txn.localVarUndo); n != 0 {
			t.Fatalf("Close left %d stale undo entries", n)
		}
	})
}

func TestOrdinarySetSurvivesAutomaticQueryAbort(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping emulator integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
	defer cancel()
	_, session := initializeWithRandomDB(t, []string{testTableSimpleDDL}, nil)
	wantPrompt := mustGetVar(t, session, "CLI_PROMPT")

	mustExec(t, ctx, session, "BEGIN RW")
	mustExec(t, ctx, session, "SET LOCAL CLI_PROMPT = 'local> '")
	mustExec(t, ctx, session, "SET LOCAL CLI_FORMAT = 'CSV'")
	mustExec(t, ctx, session, "SET CLI_FORMAT = 'JSONL'")
	const injectedCollectorAbortMsg = "injected collector abort for SET LOCAL"
	var hookFired bool
	session.txn.queryAfterCollectHook = func() error {
		hookFired = true
		return status.Error(codes.Aborted, injectedCollectorAbortMsg)
	}
	t.Cleanup(func() { session.txn.queryAfterCollectHook = nil })
	_, err := execSQL(t, ctx, session, "SELECT 1")
	session.txn.queryAfterCollectHook = nil
	if !hookFired {
		t.Fatal("queryAfterCollectHook did not run")
	}
	if spanner.ErrCode(err) != codes.Aborted || err == nil || !strings.Contains(err.Error(), injectedCollectorAbortMsg) {
		t.Fatalf("SELECT abort: %v", err)
	}
	if session.txn.InTransaction() {
		t.Fatal("query abort left an active transaction")
	}
	if n := len(session.txn.localVarUndo); n != 0 {
		t.Fatalf("query abort left %d stale undo entries", n)
	}
	if got := mustGetVar(t, session, "CLI_PROMPT"); got != wantPrompt {
		t.Fatalf("query abort left LOCAL CLI_PROMPT: got %q want %q", got, wantPrompt)
	}
	if got := mustGetVar(t, session, "CLI_FORMAT"); got != "JSONL" {
		t.Fatalf("query abort lost session SET CLI_FORMAT: %q", got)
	}

	if err := session.RecreateClient(ctx); err != nil {
		t.Fatalf("RecreateClient after abort: %v", err)
	}
	mustExec(t, ctx, session, "BEGIN RW")
	mustExec(t, ctx, session, "SET LOCAL CLI_VERBOSE = TRUE")
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "TRUE" {
		t.Fatalf("later LOCAL CLI_VERBOSE: %q", got)
	}
	mustExec(t, ctx, session, "ROLLBACK")
	if session.txn.InTransaction() {
		t.Fatal("later ROLLBACK left an active transaction")
	}
	if n := len(session.txn.localVarUndo); n != 0 {
		t.Fatalf("later transaction left %d undo entries", n)
	}
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "FALSE" {
		t.Fatalf("later LOCAL contaminated session: CLI_VERBOSE=%q", got)
	}
	if got := mustGetVar(t, session, "CLI_FORMAT"); got != "JSONL" {
		t.Fatalf("later transaction lost prior session SET: %q", got)
	}
}

func TestSetLocalTypeStylesRestoreDerivedState(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	ctx := t.Context()
	wantRaw := mustGetVar(t, session, "CLI_TYPE_STYLES")
	wantDerived := session.systemVariables.typeStyles[sppb.TypeCode_STRING]
	if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "CLI_TYPE_STYLES", Value: "'STRING=green'"}); err != nil {
		t.Fatal(err)
	}
	if session.systemVariables.typeStyles[sppb.TypeCode_STRING] == "" {
		t.Fatal("SET LOCAL did not update derived typeStyles")
	}
	if _, err := session.ExecuteStatement(ctx, &RollbackStatement{}); err != nil {
		t.Fatal(err)
	}
	if got := mustGetVar(t, session, "CLI_TYPE_STYLES"); got != wantRaw {
		t.Fatalf("raw value after rollback: %s", got)
	}
	if session.systemVariables.typeStyles[sppb.TypeCode_STRING] != wantDerived {
		t.Fatal("derived typeStyles not restored on LOCAL rollback")
	}
}

func endName(end Statement) string {
	switch end.(type) {
	case *CommitStatement:
		return "COMMIT"
	default:
		return "ROLLBACK"
	}
}

func quoteGoogleSQL(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "\\'") + "'"
}

func TestSessionSwitchGuards(t *testing.T) {
	t.Parallel()

	t.Run("USE rejected while in transaction", func(t *testing.T) {
		t.Parallel()
		session := newSessionForLocalVarTest(t)
		handler := NewSessionHandler(session)
		ctx := t.Context()

		if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
			t.Fatalf("BEGIN error: %v", err)
		}
		_, err := handler.ExecuteStatement(ctx, &UseStatement{Database: "other"})
		if err == nil || !strings.Contains(err.Error(), "transaction is active") {
			t.Errorf("USE in transaction: got error %v, want 'transaction is active'", err)
		}
	})

	t.Run("DETACH rejected while batch is active", func(t *testing.T) {
		t.Parallel()
		session := newSessionForLocalVarTest(t)
		handler := NewSessionHandler(session)
		ctx := t.Context()

		if err := session.batch.Start(batchModeDDL); err != nil {
			t.Fatalf("batch.Start error: %v", err)
		}
		_, err := handler.ExecuteStatement(ctx, &DetachStatement{})
		if err == nil || !strings.Contains(err.Error(), "batch is active") {
			t.Errorf("DETACH with active batch: got error %v, want 'batch is active'", err)
		}
	})
}
