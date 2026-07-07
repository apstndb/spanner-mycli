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

// Dispatch-level READONLY guard regression for the extracted BIGQUERY family
// (#775, re-homed by #778). These live in the external mycli_test package so
// they can import feature/bigquery (which imports mycli) without a cycle, and
// prove the guard fires through the real dispatch path: build the statement from
// the merged def table exactly as production does, then execute it against a
// READONLY session.

import (
	"testing"

	"github.com/apstndb/spanner-mycli/internal/mycli"
	"github.com/apstndb/spanner-mycli/internal/mycli/feature/bigquery"
)

func buildBigQuery(t *testing.T, sql string) mycli.Statement {
	t.Helper()
	defs := mycli.MergedStatementDefs(bigquery.Feature())
	stmt, err := mycli.BuildStatementWithDefs(defs, "BIGQUERY "+sql)
	if err != nil {
		t.Fatalf("BuildStatementWithDefs(BIGQUERY %s) error = %v", sql, err)
	}
	return stmt
}

// TestReadOnlyGuardBlocksMutatingBigQuery verifies that mutating BIGQUERY SQL is
// rejected by Session.ExecuteStatement in READONLY mode before the statement can
// reach BigQuery (no client is built).
func TestReadOnlyGuardBlocksMutatingBigQuery(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		desc string
		sql  string
	}{
		{desc: "DELETE blocked", sql: "DELETE FROM dataset.table WHERE TRUE"},
		{desc: "CREATE blocked", sql: "CREATE TABLE dataset.table AS SELECT 1"},
		{desc: "unrecognized keyword blocked", sql: "FROBNICATE dataset.table"},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			t.Parallel()
			session := mycli.NewReadOnlySessionForTest(t)
			_, err := session.ExecuteStatement(t.Context(), buildBigQuery(t, tt.sql))
			if !mycli.IsReadOnlyError(err) {
				t.Errorf("%s in READONLY mode: got error %v, want READONLY error", tt.desc, err)
			}
		})
	}
}

// TestReadOnlyGuardAllowsReadOnlyBigQuery verifies that read-only BIGQUERY SQL
// passes the READONLY guard: dispatch reaches Execute, which then fails for an
// unrelated reason (no BigQuery project configured), NOT with the READONLY
// error. This proves, through the real guard, that a SELECT/WITH is not wrongly
// blocked. A statically-mutating classification is also ruled out.
func TestReadOnlyGuardAllowsReadOnlyBigQuery(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		desc string
		sql  string
	}{
		{desc: "SELECT", sql: "SELECT 1"},
		{desc: "WITH", sql: "WITH cte AS (SELECT 1) SELECT * FROM cte"},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			t.Parallel()
			stmt := buildBigQuery(t, tt.sql)
			if _, isMutation := stmt.(mycli.MutationStatement); isMutation {
				t.Fatal("BigQueryStatement must not be a static MutationStatement")
			}

			session := mycli.NewReadOnlySessionForTest(t)
			_, err := session.ExecuteStatement(t.Context(), stmt)
			// The guard let it through; Execute then fails on missing project
			// config, which must NOT be the READONLY sentinel.
			if err == nil {
				t.Fatalf("%s: expected a non-READONLY error from Execute (no project configured), got nil", tt.desc)
			}
			if mycli.IsReadOnlyError(err) {
				t.Errorf("%s BIGQUERY wrongly blocked by READONLY guard: %v", tt.desc, err)
			}
		})
	}
}
