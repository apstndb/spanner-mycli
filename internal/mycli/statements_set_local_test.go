// Copyright 2026 apstndb
//
// Licensed under the MIT License.

package mycli

import (
	"strings"
	"testing"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
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
		{desc: "virtual variable", varName: "COMMIT_RESPONSE", value: "1", wantErr: "unimplemented setter"},
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
