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

package bigquery

import "testing"

// TestBigQueryStatementMutates exhaustively checks the fail-closed
// classification of BIGQUERY statements: only SELECT and WITH are treated as
// read-only; every other verb, comment-prefixed statement, unrecognized token,
// or empty statement is treated as mutating.
func TestBigQueryStatementMutates(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		desc string
		sql  string
		want bool
	}{
		{desc: "SELECT", sql: "SELECT 1", want: false},
		{desc: "lowercase select", sql: "select 1", want: false},
		{desc: "leading whitespace SELECT", sql: "  SELECT 1", want: false},
		{desc: "WITH", sql: "WITH cte AS (SELECT 1) SELECT * FROM cte", want: false},
		{desc: "lowercase with", sql: "with cte as (select 1) select * from cte", want: false},
		{desc: "INSERT", sql: "INSERT dataset.table VALUES (1)", want: true},
		{desc: "UPDATE", sql: "UPDATE dataset.table SET c = 1 WHERE TRUE", want: true},
		{desc: "DELETE", sql: "DELETE FROM dataset.table WHERE TRUE", want: true},
		{desc: "CREATE", sql: "CREATE TABLE dataset.table AS SELECT 1", want: true},
		{desc: "script control", sql: "BEGIN SELECT 1; END", want: true},
		{desc: "comment prefix", sql: "-- comment\nSELECT 1", want: true},
		{desc: "unrecognized keyword", sql: "FROBNICATE dataset.table", want: true},
		{desc: "empty", sql: "", want: true},
		{desc: "whitespace only", sql: "   ", want: true},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			t.Parallel()
			if got := bigQueryStatementMutates(tt.sql); got != tt.want {
				t.Errorf("bigQueryStatementMutates(%q) = %v, want %v", tt.sql, got, tt.want)
			}
		})
	}
}
