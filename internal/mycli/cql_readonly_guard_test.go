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

package mycli_test

// Dispatch-level READONLY guard regression for the extracted CQL family (#757,
// re-homed by #778). These live in the external mycli_test package so they can
// import feature/cql (which imports mycli) without a cycle, and prove the guard
// fires through the real dispatch path: build the statement from the merged def
// table exactly as production does, then execute it against a READONLY session.

import (
	"testing"

	"github.com/apstndb/spanner-mycli/internal/mycli"
	"github.com/apstndb/spanner-mycli/internal/mycli/feature/cql"
)

func buildCQL(t *testing.T, cqlText string) mycli.Statement {
	t.Helper()
	defs := mycli.MergedStatementDefs(cql.Feature())
	stmt, err := mycli.BuildStatementWithDefs(defs, "CQL "+cqlText)
	if err != nil {
		t.Fatalf("BuildStatementWithDefs(CQL %s) error = %v", cqlText, err)
	}
	return stmt
}

// TestReadOnlyGuardBlocksMutatingCQL verifies that mutating CQL is rejected by
// Session.ExecuteStatement in READONLY mode before the statement can reach the
// Cassandra adapter. These statements short-circuit at the guard before
// CQLStatement.Execute, so no adapter/proxy is built.
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
			session := mycli.NewReadOnlySessionForTest(t)
			_, err := session.ExecuteStatement(t.Context(), buildCQL(t, tt.cql))
			if !mycli.IsReadOnlyError(err) {
				t.Errorf("%s in READONLY mode: got error %v, want READONLY error", tt.desc, err)
			}
		})
	}
}

// TestReadOnlyGuardAllowsReadOnlyCQL verifies, at the dispatch level, that a
// read-only CQL SELECT is not classified as a static MutationStatement, so the
// READONLY guard does not statically block it.
//
// Unlike the BigQuery counterpart, this does NOT run the SELECT through
// Session.ExecuteStatement: a passing SELECT proceeds to CQLStatement.Execute,
// which builds the Cassandra adapter via spancql.NewCluster. That call panics
// when the adapter client cannot be constructed (no live Spanner backend in
// tests), so it cannot "fail later for a non-READONLY reason" the way BigQuery's
// project-not-configured guard does. The complementary proof that a wired SELECT
// classifier reports non-mutating (so the conditional guard lets it through)
// lives in feature/cql (TestCQLStatementConditionalMutation), which asserts the
// classification directly without touching the adapter.
func TestReadOnlyGuardAllowsReadOnlyCQL(t *testing.T) {
	t.Parallel()

	stmt := buildCQL(t, "SELECT * FROM t")
	if _, isMutation := stmt.(mycli.MutationStatement); isMutation {
		t.Fatal("CQLStatement must not be a static MutationStatement")
	}
}
