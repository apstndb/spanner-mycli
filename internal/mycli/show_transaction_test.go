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
	"errors"
	"io"
	"testing"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanner-mycli/internal/mycli/streamio"
)

func showTransactionIsolationCell(t *testing.T, res *Result) string {
	t.Helper()
	if names := extractTableColumnNames(res.TableHeader); len(names) != 1 || names[0] != "isolation_level" {
		t.Fatalf("SHOW TRANSACTION ISOLATION LEVEL columns=%v, want [isolation_level]", names)
	}
	typed := res.typedPayload()
	if typed == nil || len(typed.Rows) != 1 {
		t.Fatalf("SHOW TRANSACTION ISOLATION LEVEL typed rows=%v, want 1", typed)
	}
	var got string
	if err := typed.Rows[0].Column(0, &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func showTransactionReadOnlyCell(t *testing.T, res *Result) bool {
	t.Helper()
	if names := extractTableColumnNames(res.TableHeader); len(names) != 1 || names[0] != "transaction_read_only" {
		t.Fatalf("SHOW TRANSACTION READ ONLY columns=%v, want [transaction_read_only]", names)
	}
	fields, ok := res.TableHeader.structFields()
	if !ok || len(fields) != 1 || fields[0].GetType().GetCode() != sppb.TypeCode_BOOL {
		t.Fatalf("SHOW TRANSACTION READ ONLY metadata=%v ok=%v, want BOOL", fields, ok)
	}
	typed := res.typedPayload()
	if typed == nil || len(typed.Rows) != 1 {
		t.Fatalf("SHOW TRANSACTION READ ONLY typed rows=%v, want 1", typed)
	}
	var got bool
	if err := typed.Rows[0].Column(0, &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func mustShowTransactionIsolationLevel(t *testing.T, session *Session) string {
	t.Helper()
	const sql = "SHOW TRANSACTION ISOLATION LEVEL"
	res := mustExec(t, t.Context(), session, sql)
	if !res.KeepVariables {
		t.Fatalf("%s KeepVariables=false", sql)
	}
	return showTransactionIsolationCell(t, res)
}

func mustShowTransactionReadOnly(t *testing.T, session *Session) bool {
	t.Helper()
	const sql = "SHOW TRANSACTION READ ONLY"
	res := mustExec(t, t.Context(), session, sql)
	if !res.KeepVariables {
		t.Fatalf("%s KeepVariables=false", sql)
	}
	return showTransactionReadOnlyCell(t, res)
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
	if got := mustShowTransactionIsolationLevel(t, session); got != "UNSPECIFIED" {
		t.Fatalf("idle isolation = %q, want UNSPECIFIED", got)
	}
	if got := mustShowTransactionReadOnly(t, session); got {
		t.Fatalf("idle read only = %v, want false", got)
	}

	mustExec(t, t.Context(), session, "SET DEFAULT_ISOLATION_LEVEL = 'SERIALIZABLE'")
	mustExec(t, t.Context(), session, "SET READONLY = TRUE")
	if got := mustShowTransactionIsolationLevel(t, session); got != "SERIALIZABLE" {
		t.Fatalf("idle isolation after SET = %q, want SERIALIZABLE", got)
	}
	if got := mustShowTransactionReadOnly(t, session); !got {
		t.Fatalf("idle read only after SET = %v, want true", got)
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
	if got := mustShowTransactionIsolationLevel(t, session); got != "SERIALIZABLE" {
		t.Fatalf("pending isolation = %q, want SERIALIZABLE owner snapshot", got)
	}
	if got := mustShowTransactionReadOnly(t, session); got {
		t.Fatalf("pending read only = %v, want false", got)
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
	if got := mustShowTransactionIsolationLevel(t, session); got != "REPEATABLE_READ" {
		t.Fatalf("BEGIN override isolation = %q, want REPEATABLE_READ", got)
	}
	mustExec(t, ctx, session, "SET DEFAULT_ISOLATION_LEVEL = 'SERIALIZABLE'")
	if got := mustGetVar(t, session, "DEFAULT_ISOLATION_LEVEL"); got != "SERIALIZABLE" {
		t.Fatalf("DEFAULT_ISOLATION_LEVEL = %q", got)
	}
	if got := mustShowTransactionIsolationLevel(t, session); got != "REPEATABLE_READ" {
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
	if got := mustShowTransactionIsolationLevel(t, session); got != "SERIALIZABLE" {
		t.Fatalf("post-COMMIT isolation = %q, want SERIALIZABLE next-transaction default", got)
	}
	if got := mustShowTransactionReadOnly(t, session); got {
		t.Fatalf("post-COMMIT read only = %v, want false", got)
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
		if got := mustShowTransactionIsolationLevel(t, session); got != "REPEATABLE_READ" {
			t.Fatalf("RW isolation = %q", got)
		}
		if got := mustShowTransactionReadOnly(t, session); got {
			t.Fatalf("RW read only = %v, want false", got)
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
		if got := mustShowTransactionIsolationLevel(t, session); got != "SERIALIZABLE" {
			t.Fatalf("RO isolation = %q", got)
		}
		if got := mustShowTransactionReadOnly(t, session); !got {
			t.Fatalf("RO read only = %v, want true", got)
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
	if got := mustShowTransactionIsolationLevel(t, session); got != "REPEATABLE_READ" {
		t.Fatalf("recovery isolation = %q", got)
	}
	if got := mustShowTransactionReadOnly(t, session); got {
		t.Fatalf("recovery read only = %v, want false", got)
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
	if got := mustShowTransactionIsolationLevel(t, session); got != "UNSPECIFIED" {
		t.Fatalf("idle unspecified = %q", got)
	}
	mustExec(t, t.Context(), session, "BEGIN")
	if got := mustShowTransactionIsolationLevel(t, session); got != "UNSPECIFIED" {
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

func TestShowTransactionReadOnlyJSONLBoolean(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name     string
		readOnly bool
		wantJSON string
	}{
		{name: "false", readOnly: false, wantJSON: "{\"transaction_read_only\":false}\n"},
		{name: "true", readOnly: true, wantJSON: "{\"transaction_read_only\":true}\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			session := newSessionForLocalVarTest(t)
			if tt.readOnly {
				mustExec(t, t.Context(), session, "SET READONLY = TRUE")
			}
			session.systemVariables.Display.CLIFormat = enums.DisplayModeJSONL
			var buf bytes.Buffer
			session.systemVariables.StreamManager = streamio.NewStreamManager(io.NopCloser(bytes.NewReader(nil)), &buf, &buf)

			res := mustExec(t, t.Context(), session, "SHOW TRANSACTION READ ONLY")
			if !res.KeepVariables {
				t.Fatal("SHOW TRANSACTION READ ONLY KeepVariables=false")
			}
			fields, ok := res.TableHeader.structFields()
			if !ok || len(fields) != 1 || fields[0].GetType().GetCode() != sppb.TypeCode_BOOL {
				t.Fatalf("JSONL metadata=%v ok=%v, want BOOL", fields, ok)
			}
			if !res.alreadyDelivered() {
				t.Fatal("JSONL SHOW TRANSACTION READ ONLY should stream")
			}
			if got := buf.String(); got != tt.wantJSON {
				t.Fatalf("JSONL = %q, want %q", got, tt.wantJSON)
			}
			if _, active := session.txn.TransactionState(); active {
				t.Fatal("JSONL SHOW TRANSACTION READ ONLY activated a transaction")
			}
		})
	}
}
