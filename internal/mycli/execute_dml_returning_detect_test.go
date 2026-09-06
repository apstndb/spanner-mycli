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

package mycli

import "testing"

func TestDmlHasReturningClause(t *testing.T) {
	t.Parallel()

	tests := []struct {
		sql  string
		want bool
	}{
		{sql: "INSERT INTO t (id) VALUES (1)", want: false},
		{sql: "INSERT INTO t (id) VALUES (1) THEN RETURN id", want: true},
		{sql: "UPDATE t SET id = 1 WHERE TRUE THEN RETURN *", want: true},
		{sql: "DELETE FROM t WHERE TRUE THEN RETURN id", want: true},
		{sql: "INSERT INTO t (id) VALUES (1) -- THEN RETURN id", want: false},
		{sql: "INSERT INTO t (id) VALUES ('THEN RETURN')", want: false},
		{sql: "INSERT INTO t (id) VALUES (1) /* THEN RETURN id */", want: false},
		{sql: "UPDATE t SET x = CASE WHEN a THEN b ELSE c END WHERE TRUE", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.sql, func(t *testing.T) {
			t.Parallel()
			if got := dmlHasReturningClause(tt.sql); got != tt.want {
				t.Errorf("dmlHasReturningClause() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDmlHasReturningClauseLexicalFallback(t *testing.T) {
	t.Parallel()

	if !dmlHasReturningClauseLexical("INSERT INTO t (id) VALUES (1) THEN RETURN id") {
		t.Error("lexical scanner missed THEN RETURN")
	}
	if dmlHasReturningClauseLexical("INSERT INTO t (id) VALUES (1) -- THEN RETURN id") {
		t.Error("lexical scanner treated commented THEN RETURN as returning")
	}
	if dmlHasReturningClauseLexical("INSERT INTO t (id) VALUES ('THEN RETURN')") {
		t.Error("lexical scanner treated string literal THEN RETURN as returning")
	}
	// Unclosed string: fail closed toward the row-producing path.
	if !dmlHasReturningClauseLexical("INSERT INTO t (id) VALUES ('oops") {
		t.Error("lexer error should fail closed to executeDML")
	}
}
