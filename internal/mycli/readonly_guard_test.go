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
		{desc: "SYNC PROTO BUNDLE", stmt: &SyncProtoStatement{UpsertPaths: []string{"examples.ProtoType"}}},
		{desc: "ADD SPLIT POINTS", stmt: &AddSplitPointsStatement{}},
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

// TestReadOnlyGuardPreservesActiveBatch verifies RUN BATCH rejects in READONLY
// mode without consuming the buffered batch (issue from FEEDBACK.md 10.2 item 5).
func TestReadOnlyGuardPreservesActiveBatch(t *testing.T) {
	t.Parallel()

	session := newSessionForLocalVarTest(t)
	if err := session.batch.Start(batchModeDDL); err != nil {
		t.Fatalf("batch.Start: %v", err)
	}
	bulk, ok := session.batch.Current().(*BulkDdlStatement)
	if !ok {
		t.Fatalf("batch.Current() = %T, want *BulkDdlStatement", session.batch.Current())
	}
	bulk.Ddls = append(bulk.Ddls, "CREATE TABLE t (id INT64) PRIMARY KEY (id)")
	session.systemVariables.Transaction.ReadOnly = true

	_, err := session.ExecuteStatement(t.Context(), &RunBatchStatement{})
	if !errors.Is(err, errReadOnly) {
		t.Fatalf("RUN BATCH in READONLY mode: got %v, want errReadOnly", err)
	}
	if !session.batch.IsActive() {
		t.Fatal("RUN BATCH readonly rejection should leave the active batch intact")
	}
	if len(bulk.Ddls) != 1 {
		t.Fatalf("batch DDLs = %d, want 1", len(bulk.Ddls))
	}
}

// CQL READONLY guard coverage moved with the family to
// internal/mycli/feature/cql (#778): the fail-closed classifier is unit tested
// there (TestCQLStatementMutates), the wired read-only/mutating classification
// is asserted directly in TestCQLStatementConditionalMutation, and the
// dispatch-level guard for mutating CQL is proven through real dispatch in the
// external mycli_test package (cql_readonly_guard_test.go), which can import the
// feature package without an import cycle.

// BIGQUERY READONLY guard coverage moved with the family to
// internal/mycli/feature/bigquery (#778): the fail-closed classifier is unit
// tested there (TestBigQueryStatementMutates), and the dispatch-level guard is
// proven through real dispatch in the external mycli_test package
// (bigquery_readonly_guard_test.go), which can import the feature package
// without an import cycle.
