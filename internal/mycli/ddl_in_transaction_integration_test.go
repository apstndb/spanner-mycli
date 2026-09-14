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
)

// TestDDLInTransactionEmulatorCommitThenFailedDDL remains committed.
// Evidence is the shared spanemuboost emulator, not a managed Spanner instance.
func TestDDLInTransactionEmulatorCommitThenFailedDDL(t *testing.T) {
	skipIfShortIntegration(t)
	_, session := initializeWithRandomDB(t, []string{
		"CREATE TABLE DdlTxnSrc (Id INT64 NOT NULL, Flag BOOL) PRIMARY KEY (Id)",
	}, nil)
	ctx := t.Context()
	mustExec(t, ctx, session, "SET CLI_DDL_IN_TRANSACTION_MODE = 'AUTO_COMMIT_TRANSACTION'")
	mustExec(t, ctx, session, "SET CLI_SAVEPOINT_SUPPORT = 'ENABLED'")
	mustExec(t, ctx, session, "BEGIN")
	mustExec(t, ctx, session, "INSERT INTO DdlTxnSrc (Id, Flag) VALUES (1, TRUE)")
	_, err := execSQL(t, ctx, session, "CREATE TABLE")
	if err == nil {
		t.Fatal("expected failed DDL after auto-commit")
	}
	var after *ddlAfterCommitError
	if !errors.As(err, &after) && !strings.Contains(err.Error(), "committed") {
		t.Fatalf("want commit-then-DDL error, got %v", err)
	}
	if session.txn.InTransaction() {
		t.Fatal("original owner must be retired after the successful commit")
	}

	res := mustExec(t, ctx, session, "SELECT Id FROM DdlTxnSrc WHERE Id = 1")
	if res.AffectedRows != 1 && len(res.presentationRows()) != 1 {
		t.Fatalf("committed row missing after failed DDL: %+v", res)
	}

	mustExec(t, ctx, session, "BEGIN")
	mustExec(t, ctx, session, "SAVEPOINT keep")
	if _, rbErr := execSQL(t, ctx, session, "ROLLBACK"); rbErr != nil {
		t.Fatalf("ROLLBACK: %v", rbErr)
	}
	res = mustExec(t, ctx, session, "SELECT Id FROM DdlTxnSrc WHERE Id = 1")
	if res.AffectedRows != 1 && len(res.presentationRows()) != 1 {
		t.Fatalf("SAVEPOINT/ROLLBACK must not undo the earlier committed insert: %+v", res)
	}
}
