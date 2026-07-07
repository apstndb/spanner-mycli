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

// TestCQLStatementMutates exhaustively checks the fail-closed classification of
// CQL statements: only SELECT is treated as read-only; every other verb
// (mutations, DDL, permissions) and any unrecognized or empty keyword is
// treated as mutating.
func TestCQLStatementMutates(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		cql  string
		want bool
	}{
		{cql: "SELECT * FROM t", want: false},
		{cql: "select * from t", want: false},
		{cql: "  SELECT * FROM t", want: false},
		{cql: "INSERT INTO t (id) VALUES (1)", want: true},
		{cql: "UPDATE t SET c = 1 WHERE id = 1", want: true},
		{cql: "DELETE FROM t WHERE id = 1", want: true},
		{cql: "BEGIN BATCH INSERT INTO t (id) VALUES (1) APPLY BATCH", want: true},
		{cql: "TRUNCATE t", want: true},
		{cql: "CREATE TABLE t (id int PRIMARY KEY)", want: true},
		{cql: "DROP TABLE t", want: true},
		{cql: "ALTER TABLE t ADD c int", want: true},
		{cql: "GRANT SELECT ON t TO role", want: true},
		{cql: "REVOKE SELECT ON t FROM role", want: true},
		{cql: "BOGUS not a real verb", want: true}, // fail-closed on unknown keyword
		{cql: "", want: true},                      // fail-closed on empty
		{cql: "   ", want: true},                   // fail-closed on whitespace-only
	} {
		t.Run(tt.cql, func(t *testing.T) {
			t.Parallel()
			if got := cqlStatementMutates(tt.cql); got != tt.want {
				t.Errorf("cqlStatementMutates(%q) = %v, want %v", tt.cql, got, tt.want)
			}
		})
	}
}

// TestReadOnlyGuardBlocksMutatingCQL verifies that mutating CQL is rejected by
// Session.ExecuteStatement in READONLY mode. These statements short-circuit at
// the guard before CQLStatement.Execute, so no Cassandra adapter is needed.
//
// The complementary case (read-only SELECT is NOT blocked) is covered by
// TestReadOnlyGuardAllowsReadOnlyCQL and TestCQLStatementMutates; it is not
// exercised through ExecuteStatement here because a passing SELECT proceeds to
// Execute, which would attempt a real Cassandra/Spanner connection.
func TestReadOnlyGuardBlocksMutatingCQL(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		desc string
		cql  string
	}{
		{desc: "DELETE blocked", cql: "DELETE FROM t WHERE id = 1"},
		{desc: "INSERT blocked", cql: "INSERT INTO t (id) VALUES (1)"},
		{desc: "unrecognized keyword blocked", cql: "FROBNICATE t"},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			t.Parallel()
			session := newSessionForLocalVarTest(t)
			session.systemVariables.Transaction.ReadOnly = true

			_, err := session.ExecuteStatement(t.Context(), &CQLStatement{CQL: tt.cql})
			if !errors.Is(err, errReadOnly) {
				t.Errorf("%s in READONLY mode: got error %v, want errReadOnly", tt.desc, err)
			}
		})
	}
}

// TestReadOnlyGuardAllowsReadOnlyCQL verifies that a read-only CQL statement is
// not classified as mutating, so the READONLY guard lets it through. The guard
// input is isConditionallyMutating; asserting it directly avoids running
// CQLStatement.Execute, which would need a live Cassandra adapter.
func TestReadOnlyGuardAllowsReadOnlyCQL(t *testing.T) {
	t.Parallel()

	stmt := &CQLStatement{CQL: "SELECT * FROM t"}
	if _, isMutation := any(stmt).(MutationStatement); isMutation {
		t.Fatal("CQLStatement must not be a static MutationStatement")
	}
	if stmt.isConditionallyMutating() {
		t.Error("read-only SELECT CQL classified as mutating; READONLY guard would wrongly block it")
	}
}

// BIGQUERY READONLY guard coverage moved with the family to
// internal/mycli/feature/bigquery (#778): the fail-closed classifier is unit
// tested there (TestBigQueryStatementMutates), and the dispatch-level guard is
// proven through real dispatch in the external mycli_test package
// (bigquery_readonly_guard_test.go), which can import the feature package
// without an import cycle.
