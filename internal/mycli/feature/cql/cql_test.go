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

package cql

import (
	"testing"

	"github.com/apstndb/spanner-mycli/internal/mycli"
)

// TestFeatureDefDispatch proves the CQL def, when merged into the statement
// table, dispatches "CQL <text>" to a *CQLStatement carrying the CQL remainder.
// Moved from the core statement_processing_test.go "CQL statement" case (#778).
func TestFeatureDefDispatch(t *testing.T) {
	t.Parallel()

	defs := mycli.MergedStatementDefs(Feature())
	stmt, err := mycli.BuildStatementWithDefs(defs, "CQL SELECT id, active, username FROM users")
	if err != nil {
		t.Fatalf("BuildStatementWithDefs() error = %v", err)
	}
	cs, ok := stmt.(*CQLStatement)
	if !ok {
		t.Fatalf("dispatch returned %T, want *CQLStatement", stmt)
	}
	if cs.CQL != "SELECT id, active, username FROM users" {
		t.Fatalf("CQL = %q, want %q", cs.CQL, "SELECT id, active, username FROM users")
	}
}

// TestCQLStatementConditionalMutation preserves the classification coverage of
// the core TestReadOnlyGuardAllowsReadOnlyCQL (#757, re-homed by #778): a
// read-only SELECT is not a static MutationStatement and its wired classifier
// reports non-mutating, so the READONLY guard lets it through; a mutating
// statement's classifier reports mutating, so the guard blocks it.
//
// This asserts the wired classification directly (via the exported Classify
// field) rather than through Session.ExecuteStatement, because a passing SELECT
// proceeds to Execute, which spins up the Cassandra adapter against a live
// Spanner backend. The dispatch-level guard is proven for the blocked (mutating)
// path — which short-circuits before Execute — in the external mycli_test
// package (cql_readonly_guard_test.go).
func TestCQLStatementConditionalMutation(t *testing.T) {
	t.Parallel()

	readOnly := newCQLStatement("SELECT * FROM t")
	if _, isMutation := any(readOnly).(mycli.MutationStatement); isMutation {
		t.Fatal("CQLStatement must not be a static MutationStatement")
	}
	if readOnly.Classify() {
		t.Error("read-only SELECT CQL classified as mutating; READONLY guard would wrongly block it")
	}

	mutating := newCQLStatement("DELETE FROM t WHERE id = 1")
	if !mutating.Classify() {
		t.Error("mutating DELETE CQL classified as read-only; READONLY guard would wrongly allow it")
	}
}
