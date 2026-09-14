// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mycli

import (
	"errors"
	"testing"
)

func TestAutocommitFalseEmulatorCommittedContents(t *testing.T) {
	skipIfShortIntegration(t)
	_, session := initializeWithRandomDB(t, []string{
		"CREATE TABLE AutocommitSrc (Id INT64 NOT NULL, Flag BOOL) PRIMARY KEY (Id)",
	}, nil)
	ctx := t.Context()

	mustExec(t, ctx, session, "SET AUTOCOMMIT = FALSE")
	mustExec(t, ctx, session, "INSERT INTO AutocommitSrc (Id, Flag) VALUES (1, TRUE)")
	if !session.txn.InTransaction() {
		t.Fatal("INSERT under AUTOCOMMIT=false must keep a logical owner")
	}

	res := mustExec(t, ctx, session, "SELECT Id FROM AutocommitSrc WHERE Id = 1")
	if res.AffectedRows != 1 && len(res.presentationRows()) != 1 {
		t.Fatalf("same-owner SELECT missing uncommitted insert: %+v", res)
	}

	mustExec(t, ctx, session, "ROLLBACK")
	assertIdle(t, session, "ROLLBACK must leave the session idle")

	res = mustExec(t, ctx, session, "SELECT Id FROM AutocommitSrc WHERE Id = 1")
	if res.AffectedRows != 0 && len(res.presentationRows()) != 0 {
		t.Fatalf("ROLLBACK must not leave committed contents: %+v", res)
	}
	if !session.txn.InTransaction() {
		t.Fatal("next SELECT after ROLLBACK must create a new owner")
	}
	mustExec(t, ctx, session, "ROLLBACK")

	mustExec(t, ctx, session, "INSERT INTO AutocommitSrc (Id, Flag) VALUES (2, TRUE)")
	mustExec(t, ctx, session, "COMMIT")
	assertIdle(t, session, "COMMIT must leave the session idle")

	res = mustExec(t, ctx, session, "SELECT Id FROM AutocommitSrc WHERE Id = 2")
	if res.AffectedRows != 1 && len(res.presentationRows()) != 1 {
		t.Fatalf("COMMIT must persist contents: %+v", res)
	}
	mustExec(t, ctx, session, "ROLLBACK")
}

func TestAutocommitFalseEmulatorReadonlyAndSavepoint(t *testing.T) {
	skipIfShortIntegration(t)
	_, session := initializeWithRandomDB(t, []string{
		"CREATE TABLE AutocommitRo (Id INT64 NOT NULL) PRIMARY KEY (Id)",
	}, nil)
	ctx := t.Context()

	mustExec(t, ctx, session, "SET AUTOCOMMIT = FALSE")
	mustExec(t, ctx, session, "SET READONLY = TRUE")
	_, err := execSQL(t, ctx, session, "INSERT INTO AutocommitRo (Id) VALUES (1)")
	if !errors.Is(err, errReadOnly) {
		t.Fatalf("READONLY DML: %v, want %v", err, errReadOnly)
	}
	assertIdle(t, session, "READONLY DML acquired an owner")

	mustExec(t, ctx, session, "SET READONLY = FALSE")
	mustExec(t, ctx, session, "SET CLI_SAVEPOINT_SUPPORT = 'ENABLED'")
	mustExec(t, ctx, session, "SAVEPOINT keep")
	if !session.txn.InTransaction() {
		t.Fatal("enabled SAVEPOINT must acquire an owner")
	}
	mustExec(t, ctx, session, "INSERT INTO AutocommitRo (Id) VALUES (1)")
	mustExec(t, ctx, session, "ROLLBACK TO SAVEPOINT keep")
	mustExec(t, ctx, session, "COMMIT")

	res := mustExec(t, ctx, session, "SELECT Id FROM AutocommitRo WHERE Id = 1")
	if res.AffectedRows != 0 && len(res.presentationRows()) != 0 {
		t.Fatalf("ROLLBACK TO SAVEPOINT must undo the insert: %+v", res)
	}
}

func TestAutocommitFalseEmulatorLocalRestoreOnce(t *testing.T) {
	skipIfShortIntegration(t)
	_, session := initializeWithRandomDB(t, nil, nil)
	ctx := t.Context()

	mustExec(t, ctx, session, "SET AUTOCOMMIT = FALSE")
	mustExec(t, ctx, session, "SET CLI_VERBOSE = FALSE")
	mustExec(t, ctx, session, "SELECT 1")
	mustExec(t, ctx, session, "SET LOCAL CLI_VERBOSE = TRUE")
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "TRUE" {
		t.Fatalf("LOCAL CLI_VERBOSE = %q, want TRUE", got)
	}
	mustExec(t, ctx, session, "COMMIT")
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "FALSE" {
		t.Fatalf("COMMIT LOCAL restore = %q, want FALSE", got)
	}
	assertIdle(t, session, "COMMIT must restore LOCAL only at the idle boundary")
}
