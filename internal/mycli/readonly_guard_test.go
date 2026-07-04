// Copyright 2026 apstndb
//
// Licensed under the MIT License.

package mycli

import (
	"errors"
	"testing"

	"cloud.google.com/go/spanner"
)

// TestReadOnlyGuardCoversBatchStatements is the regression test for issue
// #695: CreateDatabaseStatement, BulkDdlStatement, and BatchDMLStatement
// declared an exported IsMutationStatement method instead of the unexported
// marker, silently dropping them out of the MutationStatement interface and
// bypassing the READONLY guard in Session.ExecuteStatement.
func TestReadOnlyGuardCoversBatchStatements(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		desc string
		stmt Statement
	}{
		{desc: "single DDL", stmt: &DdlStatement{Ddl: "CREATE TABLE t (id INT64) PRIMARY KEY (id)"}},
		{desc: "batch DDL", stmt: &BulkDdlStatement{Ddls: []string{"CREATE TABLE t (id INT64) PRIMARY KEY (id)"}}},
		{desc: "batch DML", stmt: &BatchDMLStatement{DMLs: []spanner.Statement{spanner.NewStatement("UPDATE t SET id = 1 WHERE TRUE")}}},
		{desc: "CREATE DATABASE", stmt: &CreateDatabaseStatement{CreateStatement: "CREATE DATABASE d"}},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			t.Parallel()
			session := newSessionForLocalVarTest(t)
			session.systemVariables.Transaction.ReadOnly = true

			_, err := session.ExecuteStatement(t.Context(), tt.stmt)
			if !errors.Is(err, errReadOnly) {
				t.Errorf("%s in READONLY mode: got error %v, want errReadOnly", tt.desc, err)
			}
		})
	}
}
