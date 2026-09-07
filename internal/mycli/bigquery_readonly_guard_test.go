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
	"strings"
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
		{desc: "SELECT then DELETE blocked", sql: "SELECT 1; DELETE FROM dataset.table WHERE TRUE"},
		{desc: "dash CR DELETE blocked", sql: "SELECT 1; -- comment\rDELETE FROM dataset.table WHERE TRUE"},
		{desc: "CALL root blocked", sql: "CALL dataset.proc()"},
		{desc: "EXPORT root blocked", sql: "EXPORT DATA OPTIONS(uri='gs://b/p') AS SELECT 1"},
		{desc: "EXECUTE IMMEDIATE blocked", sql: "EXECUTE IMMEDIATE 'SELECT 1'"},
		{desc: "truncated octal blocked", sql: "SELECT \"\\0"},
		{desc: "empty payload blocked", sql: "/* comment only */"},
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
		{desc: "read-only script", sql: "SELECT 1; SELECT 2"},
		{desc: "comment then SELECT", sql: "-- comment\nSELECT 1"},
		{desc: "parenthesized SELECT", sql: "(SELECT 1)"},
		{desc: "FROM pipe", sql: "FROM dataset.table |> SELECT *"},
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
			if !isMissingBigQueryProject(err) {
				t.Errorf("%s: got error %v, want missing BigQuery project", tt.desc, err)
			}
		})
	}
}

func TestReadOnlyFalseReachesMissingProject(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		desc string
		sql  string
	}{
		{desc: "SELECT", sql: "SELECT 1"},
		{desc: "SELECT then DELETE", sql: "SELECT 1; DELETE FROM dataset.table WHERE TRUE"},
		{desc: "dash CR DELETE", sql: "SELECT 1; -- comment\rDELETE FROM dataset.table WHERE TRUE"},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			t.Parallel()
			session := mycli.NewSessionForTest(t)
			_, err := session.ExecuteStatement(t.Context(), buildBigQuery(t, tt.sql))
			if mycli.IsReadOnlyError(err) {
				t.Errorf("%s READONLY=false wrongly blocked by READONLY guard: %v", tt.desc, err)
			}
			if !isMissingBigQueryProject(err) {
				t.Errorf("%s: got error %v, want missing BigQuery project", tt.desc, err)
			}
		})
	}
}

func isMissingBigQueryProject(err error) bool {
	return err != nil && strings.Contains(err.Error(), "BigQuery project not configured")
}
