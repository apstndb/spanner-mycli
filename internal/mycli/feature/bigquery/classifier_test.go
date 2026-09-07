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
		{desc: "read-only script", sql: "SELECT 1; WITH x AS (SELECT 2) SELECT * FROM x;", want: false},
		{desc: "empty separators", sql: "; ; SELECT 1;;; # tail", want: false},
		{desc: "comment then SELECT", sql: "-- comment\nSELECT 1", want: false},
		{desc: "block comment then SELECT", sql: "/* x */ SELECT 1", want: false},
		{desc: "parenthesized SELECT", sql: "(SELECT 1)", want: false},
		{desc: "nested parentheses", sql: "((SELECT 1))", want: false},
		{desc: "FROM pipe", sql: "FROM d.t |> SELECT *", want: false},
		{desc: "pipe CALL SET DROP", sql: "FROM d.t |> CALL d.f() |> SET x=1 |> DROP y; SELECT 2", want: false},
		{desc: "quoted DELETE letters", sql: "SELECT 'DELETE FROM t'", want: false},
		{desc: "quoted semicolon", sql: "SELECT `semi;colon` FROM `p.d.t`; SELECT 2", want: false},
		{desc: "raw escaped quote", sql: "SELECT r'a\\'; DELETE x;'; SELECT 2", want: false},
		{desc: "triple quoted", sql: "SELECT '''a\n'; DELETE x;'''; SELECT 2", want: false},
		{desc: "bytes octal then DELETE", sql: "SELECT b'\\073'; DELETE d.t WHERE TRUE", want: true},
		{desc: "dash CR then DELETE", sql: "SELECT 1; -- comment\rDELETE d.t WHERE TRUE", want: true},
		{desc: "pound CR then DELETE", sql: "SELECT 1; # comment\rDELETE d.t WHERE TRUE", want: true},
		{desc: "backspace whitespace", sql: "SELECT\b1; SELECT 2", want: false},
		{desc: "INSERT", sql: "INSERT dataset.table VALUES (1)", want: true},
		{desc: "UPDATE", sql: "UPDATE dataset.table SET c = 1 WHERE TRUE", want: true},
		{desc: "DELETE", sql: "DELETE FROM dataset.table WHERE TRUE", want: true},
		{desc: "CREATE", sql: "CREATE TABLE dataset.table AS SELECT 1", want: true},
		{desc: "DROP", sql: "DROP TABLE dataset.table", want: true},
		{desc: "MERGE", sql: "MERGE dataset.t USING dataset.s ON FALSE WHEN MATCHED THEN DELETE", want: true},
		{desc: "CALL root", sql: "CALL dataset.proc()", want: true},
		{desc: "EXPORT root", sql: "EXPORT DATA OPTIONS(uri='gs://b/p') AS SELECT 1", want: true},
		{desc: "EXECUTE IMMEDIATE", sql: "EXECUTE IMMEDIATE 'SELECT 1'", want: true},
		{desc: "script control", sql: "BEGIN SELECT 1; END", want: true},
		{desc: "SELECT then DELETE", sql: "SELECT 1; DELETE FROM dataset.table WHERE TRUE", want: true},
		{desc: "quoted SELECT ident", sql: "`SELECT` 1; DELETE d.t WHERE TRUE", want: true},
		{desc: "parenthesized DELETE", sql: "(DELETE FROM dataset.table WHERE TRUE)", want: true},
		{desc: "unrecognized keyword", sql: "FROBNICATE dataset.table", want: true},
		{desc: "empty", sql: "", want: true},
		{desc: "whitespace only", sql: "   ", want: true},
		{desc: "comments only", sql: "/* x */ -- y\n# z", want: true},
		{desc: "unterminated string", sql: "SELECT 1; SELECT 'x", want: true},
		{desc: "unterminated comment", sql: "SELECT 1; /* x", want: true},
		{desc: "invalid escape", sql: "SELECT '\\z'", want: true},
		{desc: "literal CR in ordinary string", sql: "SELECT 'x\ry'", want: true},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			t.Parallel()
			if got := bigQueryStatementMutates(tt.sql); got != tt.want {
				t.Errorf("bigQueryStatementMutates(%q) = %v, want %v", tt.sql, got, tt.want)
			}
		})
	}
}

func TestBigQueryStatementMutatesTruncatedEscapes(t *testing.T) {
	t.Parallel()

	for _, sql := range []string{
		"\"\\0",
		"SELECT \"\\0",
		"SELECT 1; SELECT \"\\0",
		"\"\\x",
		"\"\\u",
	} {
		t.Run(sql, func(t *testing.T) {
			t.Parallel()
			if !bigQueryStatementMutates(sql) {
				t.Errorf("bigQueryStatementMutates(%q) = false, want mutating fail-closed", sql)
			}
		})
	}
}

func TestBigQueryClassificationLeavesOriginalSQL(t *testing.T) {
	t.Parallel()

	sql := "SELECT 1; -- comment\rDELETE FROM t WHERE TRUE"
	stmt := newBigQueryStatement(sql, &config{})
	if !stmt.Classify() {
		t.Fatal("mixed CR script must classify as mutating")
	}
	if stmt.SQL != sql {
		t.Fatalf("classification mutated SQL: got %q want %q", stmt.SQL, sql)
	}
}
