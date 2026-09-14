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

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/enums"
)

func TestCLIDdlInTransactionModeDefaultIsFail(t *testing.T) {
	t.Parallel()
	sv := newSystemVariablesWithDefaultsForTest()
	sv.ensureRegistry()
	got, err := sv.Registry.Get("CLI_DDL_IN_TRANSACTION_MODE")
	if err != nil {
		t.Fatal(err)
	}
	if got != "FAIL" {
		t.Fatalf("default = %q, want FAIL", got)
	}
}

func TestDdlInTransactionSnapshotIgnoresLaterSessionSet(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	ctx := t.Context()
	if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "CLI_DDL_IN_TRANSACTION_MODE", Value: "'AUTO_COMMIT_TRANSACTION'"}); err != nil {
		t.Fatal(err)
	}
	if session.txn.effectiveDdlInTransactionMode() != enums.DdlInTransactionModeFail {
		t.Fatalf("owner snapshot = %v, want FAIL", session.txn.effectiveDdlInTransactionMode())
	}
	if got := mustGetVar(t, session, "CLI_DDL_IN_TRANSACTION_MODE"); got != "AUTO_COMMIT_TRANSACTION" {
		t.Fatalf("session value = %q, want AUTO_COMMIT_TRANSACTION", got)
	}
	_, err := session.ExecuteStatement(ctx, &DdlStatement{Ddl: "CREATE TABLE t (id INT64) PRIMARY KEY (id)"})
	if !errors.Is(err, errDDLInTransaction) {
		t.Fatalf("DDL after mid-owner SET: %v, want FAIL", err)
	}
	if !session.txn.InPendingTransaction() {
		t.Fatal("FAIL must leave the pending owner in place")
	}
}

func TestSetLocalDdlInTransactionModeBeforeAndAfterUserWork(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	ctx := t.Context()
	if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "CLI_DDL_IN_TRANSACTION_MODE", Value: "'ALLOW_IN_EMPTY_TRANSACTION'"}); err != nil {
		t.Fatal(err)
	}
	if session.txn.effectiveDdlInTransactionMode() != enums.DdlInTransactionModeAllowInEmptyTransaction {
		t.Fatal("SET LOCAL before user work must update the captured policy")
	}

	session.txn.mu.Lock()
	session.txn.markUserWorkLocked()
	session.txn.mu.Unlock()

	_, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "CLI_DDL_IN_TRANSACTION_MODE", Value: "'AUTO_COMMIT_TRANSACTION'"})
	if !errors.Is(err, errDdlInTransactionModeFrozen) {
		t.Fatalf("SET LOCAL after user work: %v, want frozen", err)
	}
	if got := mustGetVar(t, session, "CLI_DDL_IN_TRANSACTION_MODE"); got != "ALLOW_IN_EMPTY_TRANSACTION" {
		t.Fatalf("registry after rejected SET LOCAL = %q, want ALLOW_IN_EMPTY_TRANSACTION", got)
	}
	if session.txn.effectiveDdlInTransactionMode() != enums.DdlInTransactionModeAllowInEmptyTransaction {
		t.Fatal("rejected SET LOCAL must not change the captured policy")
	}
}

func TestSetLocalDdlInTransactionModeRestoresOnRetire(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	ctx := t.Context()
	if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "CLI_DDL_IN_TRANSACTION_MODE", Value: "'FAIL'"}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "CLI_DDL_IN_TRANSACTION_MODE", Value: "'ALLOW_IN_EMPTY_TRANSACTION'"}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &RollbackStatement{}); err != nil {
		t.Fatal(err)
	}
	if got := mustGetVar(t, session, "CLI_DDL_IN_TRANSACTION_MODE"); got != "FAIL" {
		t.Fatalf("after rollback = %q, want FAIL", got)
	}
}

func TestEmptyBulkDdlDoesNotRetirePending(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	ctx := t.Context()
	if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "CLI_DDL_IN_TRANSACTION_MODE", Value: "'AUTO_COMMIT_TRANSACTION'"}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
		t.Fatal(err)
	}
	if _, err := executeDdlStatements(ctx, session, nil); err != nil {
		t.Fatal(err)
	}
	if !session.txn.InPendingTransaction() {
		t.Fatal("empty BulkDdl must not retire a pending owner")
	}
}

func TestStartBatchDDLAdmitsBeforeChangingBatchState(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	ctx := t.Context()
	if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
		t.Fatal(err)
	}
	_, err := session.ExecuteStatement(ctx, &StartBatchStatement{Mode: batchModeDDL})
	if !errors.Is(err, errDDLInTransaction) {
		t.Fatalf("START BATCH DDL: %v, want FAIL", err)
	}
	if session.batch.IsActive() {
		t.Fatal("rejected START BATCH DDL must not change batch state")
	}
	if !session.txn.InPendingTransaction() {
		t.Fatal("FAIL START BATCH DDL must not retire the pending owner")
	}
}

func TestStartBatchDDLRejectsExistingManualBatchBeforeRetire(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	ctx := t.Context()
	if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "CLI_DDL_IN_TRANSACTION_MODE", Value: "'AUTO_COMMIT_TRANSACTION'"}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &StartBatchStatement{Mode: batchModeDML}); err != nil {
		t.Fatal(err)
	}
	_, err := session.ExecuteStatement(ctx, &StartBatchStatement{Mode: batchModeDDL})
	if err == nil || !strings.Contains(err.Error(), "already in batch") {
		t.Fatalf("START BATCH DDL: %v, want already in batch", err)
	}
	if !session.txn.InPendingTransaction() {
		t.Fatal("existing manual batch must be rejected before retirement")
	}
	if _, ok := session.batch.Current().(*BatchDMLStatement); !ok {
		t.Fatal("manual DML batch must remain")
	}
}

func TestAllowPendingRetiresWithoutDetermine(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	ctx := t.Context()
	if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "CLI_DDL_IN_TRANSACTION_MODE", Value: "'ALLOW_IN_EMPTY_TRANSACTION'"}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareDDLInTransaction(ctx, session); err != nil {
		t.Fatal(err)
	}
	if session.txn.InTransaction() {
		t.Fatal("ALLOW pending must retire without constructing")
	}
}

func TestAutoCommitPendingIsNoopRetire(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	ctx := t.Context()
	if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "CLI_DDL_IN_TRANSACTION_MODE", Value: "'AUTO_COMMIT_TRANSACTION'"}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareDDLInTransaction(ctx, session); err != nil {
		t.Fatal(err)
	}
	if session.txn.InTransaction() {
		t.Fatal("AUTO_COMMIT pending must no-op retire")
	}
}

func TestDDLRejectedInReadOnlyTransaction(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	session.systemVariables.Transaction.ReadOnly = true
	_, err := session.ExecuteStatement(t.Context(), &DdlStatement{Ddl: "CREATE TABLE t (id INT64) PRIMARY KEY (id)"})
	if !errors.Is(err, errReadOnly) {
		t.Fatalf("READONLY session: %v, want errReadOnly", err)
	}
}

func TestHasUserWorkSurvivesRollbackToJournal(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	if err := session.txn.BeginPendingTransaction(t.Context(), sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	session.txn.mu.Lock()
	session.txn.markUserWorkLocked()
	session.txn.tc.replay = &replayState{}
	session.txn.tc.replay.savepoints = []savepoint{{name: "keep"}}
	session.txn.tc.replay.entries = nil
	session.txn.mu.Unlock()

	if !session.txn.HasUserWork() {
		t.Fatal("hasUserWork should stay set")
	}
	state := session.txn.ddlOwnerSnapshot()
	if state.empty {
		t.Fatal("ROLLBACK TO / empty journal must not make a used owner empty")
	}
}
