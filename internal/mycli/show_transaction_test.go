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
	"strings"
	"testing"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
)

func showTransactionCell(t *testing.T, res *Result, wantColumn string) string {
	t.Helper()
	if names := extractTableColumnNames(res.TableHeader); len(names) != 1 || names[0] != wantColumn {
		t.Fatalf("SHOW TRANSACTION columns=%v, want [%s]", names, wantColumn)
	}
	typed := res.typedPayload()
	if typed == nil || len(typed.Rows) != 1 {
		t.Fatalf("SHOW TRANSACTION typed rows=%v, want 1", typed)
	}
	var got string
	if err := typed.Rows[0].Column(0, &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func mustShowTransaction(t *testing.T, session *Session, sql string) string {
	t.Helper()
	wantColumn := "isolation_level"
	if strings.Contains(strings.ToUpper(sql), "READ ONLY") {
		wantColumn = "transaction_read_only"
	}
	res := mustExec(t, t.Context(), session, sql)
	if !res.KeepVariables {
		t.Fatalf("%s KeepVariables=false", sql)
	}
	return showTransactionCell(t, res, wantColumn)
}

func plantOwner(t *testing.T, session *Session, attrs transactionAttributes, recovery error) *transactionContext {
	t.Helper()
	session.txn.mu.Lock()
	defer session.txn.mu.Unlock()
	owner := &transactionContext{attrs: attrs}
	if recovery != nil {
		owner.replay = &replayState{recoveryRequired: recovery}
	}
	session.txn.tc = owner
	return owner
}

func autoDMLLen(tm *TransactionManager) int {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	if tm.tc == nil {
		return 0
	}
	return len(tm.tc.autoDML)
}

func TestParseShowTransactionOption(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		in      string
		want    showTransactionKind
		wantErr string
	}{
		{in: "ISOLATION LEVEL", want: showTransactionIsolationLevel},
		{in: "  isolation   level  ", want: showTransactionIsolationLevel},
		{in: "READ ONLY", want: showTransactionReadOnly},
		{in: "read  only", want: showTransactionReadOnly},
		{in: "", wantErr: "syntax error: missing TRANSACTION option, expected ISOLATION LEVEL or READ ONLY"},
		{in: "ISOLATION", wantErr: `invalid TRANSACTION option "ISOLATION", expected ISOLATION LEVEL or READ ONLY`},
		{in: "ISOLATION LEVEL leftover", wantErr: `invalid TRANSACTION option "ISOLATION LEVEL leftover", expected ISOLATION LEVEL or READ ONLY`},
		{in: "READ ONLY leftover", wantErr: `invalid TRANSACTION option "READ ONLY leftover", expected ISOLATION LEVEL or READ ONLY`},
		{in: "DEFERRABLE", wantErr: `invalid TRANSACTION option "DEFERRABLE", expected ISOLATION LEVEL or READ ONLY`},
		{in: "READ WRITE", wantErr: `invalid TRANSACTION option "READ WRITE", expected ISOLATION LEVEL or READ ONLY`},
	} {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			got, err := parseShowTransactionOption(tt.in)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("kind = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildStatementShowTransactionInvalid(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		input   string
		wantErr string
	}{
		{input: "SHOW TRANSACTION", wantErr: "syntax error: missing TRANSACTION option, expected ISOLATION LEVEL or READ ONLY"},
		{input: "SHOW TRANSACTION ISOLATION LEVEL leftover", wantErr: `invalid TRANSACTION option "ISOLATION LEVEL leftover", expected ISOLATION LEVEL or READ ONLY`},
		{input: "SHOW TRANSACTION DEFERRABLE", wantErr: `invalid TRANSACTION option "DEFERRABLE", expected ISOLATION LEVEL or READ ONLY`},
	} {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			_, err := BuildStatement(tt.input)
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("BuildStatement(%q) error = %v, want %q", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestShowTransactionIdleDefaults(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	if got := mustShowTransaction(t, session, "SHOW TRANSACTION ISOLATION LEVEL"); got != "UNSPECIFIED" {
		t.Fatalf("idle isolation = %q, want UNSPECIFIED", got)
	}
	if got := mustShowTransaction(t, session, "SHOW TRANSACTION READ ONLY"); got != "FALSE" {
		t.Fatalf("idle read only = %q, want FALSE", got)
	}

	mustExec(t, t.Context(), session, "SET DEFAULT_ISOLATION_LEVEL = 'SERIALIZABLE'")
	mustExec(t, t.Context(), session, "SET READONLY = TRUE")
	if got := mustShowTransaction(t, session, "SHOW TRANSACTION ISOLATION LEVEL"); got != "SERIALIZABLE" {
		t.Fatalf("idle isolation after SET = %q, want SERIALIZABLE", got)
	}
	if got := mustShowTransaction(t, session, "SHOW TRANSACTION READ ONLY"); got != "TRUE" {
		t.Fatalf("idle read only after SET = %q, want TRUE", got)
	}
}

func TestShowTransactionPendingOwnerIgnoresNextTransactionDefault(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	ctx := t.Context()
	mustExec(t, ctx, session, "SET DEFAULT_ISOLATION_LEVEL = 'SERIALIZABLE'")
	mustExec(t, ctx, session, "SET CLI_SAVEPOINT_SUPPORT = 'ENABLED'")
	mustExec(t, ctx, session, "BEGIN")
	if mode, _ := session.txn.TransactionState(); mode != transactionModePending {
		t.Fatalf("mode = %q, want pending", mode)
	}
	mustExec(t, ctx, session, "SET DEFAULT_ISOLATION_LEVEL = 'REPEATABLE_READ'")
	if got := mustShowTransaction(t, session, "SHOW TRANSACTION ISOLATION LEVEL"); got != "SERIALIZABLE" {
		t.Fatalf("pending isolation = %q, want SERIALIZABLE owner snapshot", got)
	}
	if got := mustShowTransaction(t, session, "SHOW TRANSACTION READ ONLY"); got != "FALSE" {
		t.Fatalf("pending read only = %q, want FALSE", got)
	}
	if mode, _ := session.txn.TransactionState(); mode != transactionModePending {
		t.Fatalf("SHOW activated pending transaction: mode = %q", mode)
	}
	if n := autoDMLLen(session.txn); n != 0 {
		t.Fatalf("SHOW flushed or queued automatic DML: %d", n)
	}
	if journal := replayJournal(session.txn); len(journal) != 0 {
		t.Fatalf("SHOW appended journal: %+v", journal)
	}
}

func TestShowTransactionBeginOverrideNotDefaultIsolation(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	ctx := t.Context()
	mustExec(t, ctx, session, "SET DEFAULT_ISOLATION_LEVEL = 'SERIALIZABLE'")
	mustExec(t, ctx, session, "BEGIN ISOLATION LEVEL REPEATABLE READ")
	if got := mustShowTransaction(t, session, "SHOW TRANSACTION ISOLATION LEVEL"); got != "REPEATABLE_READ" {
		t.Fatalf("BEGIN override isolation = %q, want REPEATABLE_READ", got)
	}
	mustExec(t, ctx, session, "SET DEFAULT_ISOLATION_LEVEL = 'SERIALIZABLE'")
	if got := mustGetVar(t, session, "DEFAULT_ISOLATION_LEVEL"); got != "SERIALIZABLE" {
		t.Fatalf("DEFAULT_ISOLATION_LEVEL = %q", got)
	}
	if got := mustShowTransaction(t, session, "SHOW TRANSACTION ISOLATION LEVEL"); got != "REPEATABLE_READ" {
		t.Fatalf("SHOW echoed DEFAULT_ISOLATION_LEVEL: %q", got)
	}
}

func TestShowTransactionPostCommitUsesNextTransactionDefault(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	ctx := t.Context()
	mustExec(t, ctx, session, "SET DEFAULT_ISOLATION_LEVEL = 'SERIALIZABLE'")
	mustExec(t, ctx, session, "BEGIN ISOLATION LEVEL REPEATABLE READ")
	mustExec(t, ctx, session, "COMMIT")
	if _, active := session.txn.TransactionState(); active {
		t.Fatal("expected idle after COMMIT of pending transaction")
	}
	if got := mustShowTransaction(t, session, "SHOW TRANSACTION ISOLATION LEVEL"); got != "SERIALIZABLE" {
		t.Fatalf("post-COMMIT isolation = %q, want SERIALIZABLE next-transaction default", got)
	}
	if got := mustShowTransaction(t, session, "SHOW TRANSACTION READ ONLY"); got != "FALSE" {
		t.Fatalf("post-COMMIT read only = %q, want FALSE", got)
	}
}

func TestShowTransactionFakeOwners(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	mustExec(t, t.Context(), session, "SET DEFAULT_ISOLATION_LEVEL = 'SERIALIZABLE'")

	t.Run("read-write", func(t *testing.T) {
		owner := plantOwner(t, session, transactionAttributes{
			mode:           transactionModeReadWrite,
			isolationLevel: sppb.TransactionOptions_REPEATABLE_READ,
		}, nil)
		mustExec(t, t.Context(), session, "SET DEFAULT_ISOLATION_LEVEL = 'SERIALIZABLE'")
		if got := mustShowTransaction(t, session, "SHOW TRANSACTION ISOLATION LEVEL"); got != "REPEATABLE_READ" {
			t.Fatalf("RW isolation = %q", got)
		}
		if got := mustShowTransaction(t, session, "SHOW TRANSACTION READ ONLY"); got != "FALSE" {
			t.Fatalf("RW read only = %q", got)
		}
		if txnContext(session.txn) != owner {
			t.Fatal("SHOW replaced the RW owner")
		}
	})

	t.Run("read-only", func(t *testing.T) {
		owner := plantOwner(t, session, transactionAttributes{
			mode:           transactionModeReadOnly,
			isolationLevel: sppb.TransactionOptions_SERIALIZABLE,
		}, nil)
		if got := mustShowTransaction(t, session, "SHOW TRANSACTION ISOLATION LEVEL"); got != "SERIALIZABLE" {
			t.Fatalf("RO isolation = %q", got)
		}
		if got := mustShowTransaction(t, session, "SHOW TRANSACTION READ ONLY"); got != "TRUE" {
			t.Fatalf("RO read only = %q", got)
		}
		if txnContext(session.txn) != owner {
			t.Fatal("SHOW replaced the RO owner")
		}
	})
}

func TestShowTransactionRecoveryRequiredOwner(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	injected := errors.New("injected recovery")
	owner := plantOwner(t, session, transactionAttributes{
		mode:           transactionModeReadWrite,
		isolationLevel: sppb.TransactionOptions_REPEATABLE_READ,
	}, injected)
	if !session.txn.NeedsRecovery() {
		t.Fatal("expected recovery-required owner")
	}
	if _, err := execSQL(t, t.Context(), session, "SELECT 1"); !errors.Is(err, errSavepointRecovery) {
		t.Fatalf("SELECT during recovery: %v", err)
	}
	if got := mustShowTransaction(t, session, "SHOW TRANSACTION ISOLATION LEVEL"); got != "REPEATABLE_READ" {
		t.Fatalf("recovery isolation = %q", got)
	}
	if got := mustShowTransaction(t, session, "SHOW TRANSACTION READ ONLY"); got != "FALSE" {
		t.Fatalf("recovery read only = %q", got)
	}
	if txnContext(session.txn) != owner {
		t.Fatal("SHOW retired the recovery owner")
	}
	if !session.txn.NeedsRecovery() {
		t.Fatal("SHOW cleared recovery-required")
	}
}

func TestShowTransactionUnspecifiedMeansDatabaseDefault(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	mustExec(t, t.Context(), session, "SET DEFAULT_ISOLATION_LEVEL = 'UNSPECIFIED'")
	if got := mustShowTransaction(t, session, "SHOW TRANSACTION ISOLATION LEVEL"); got != "UNSPECIFIED" {
		t.Fatalf("idle unspecified = %q", got)
	}
	mustExec(t, t.Context(), session, "BEGIN")
	if got := mustShowTransaction(t, session, "SHOW TRANSACTION ISOLATION LEVEL"); got != "UNSPECIFIED" {
		t.Fatalf("pending unspecified = %q, want database default not a guessed server isolation", got)
	}
}

func TestShowTransactionStateWithoutSystemVariables(t *testing.T) {
	t.Parallel()
	var tm *TransactionManager
	isolation, readOnly := tm.showTransactionState()
	if isolation != sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED || readOnly {
		t.Fatalf("nil TM state = (%v, %v)", isolation, readOnly)
	}

	tm = NewTransactionManager(nil, nil, spanner.ClientConfig{})
	isolation, readOnly = tm.showTransactionState()
	if isolation != sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED || readOnly {
		t.Fatalf("nil sysVars state = (%v, %v)", isolation, readOnly)
	}

	sysVars := newSystemVariablesWithDefaultsForTest()
	sysVars.Transaction.DefaultIsolationLevel = sppb.TransactionOptions_SERIALIZABLE
	sysVars.Transaction.ReadOnly = true
	isolation, readOnly = showTransactionStateFromSession(&Session{systemVariables: sysVars})
	if isolation != sppb.TransactionOptions_SERIALIZABLE || !readOnly {
		t.Fatalf("sysVars-only state = (%v, %v)", isolation, readOnly)
	}

	session := &Session{}
	isolation, readOnly = showTransactionStateFromSession(session)
	if isolation != sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED || readOnly {
		t.Fatalf("empty session state = (%v, %v)", isolation, readOnly)
	}
	isolation, readOnly = showTransactionStateFromSession(nil)
	if isolation != sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED || readOnly {
		t.Fatalf("nil session state = (%v, %v)", isolation, readOnly)
	}

	res, err := (&ShowTransactionStatement{Kind: 0}).Execute(t.Context(), session, OperationOutput{})
	if err == nil || err.Error() != "invalid SHOW TRANSACTION kind" {
		t.Fatalf("invalid kind error = %v, result=%v", err, res)
	}
}
