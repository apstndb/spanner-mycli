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

import "testing"

// TestCQLStatementMutates exhaustively checks the fail-closed classification of
// CQL statements: only SELECT is treated as read-only; every other verb
// (mutations, DDL, permissions) and any unrecognized or empty keyword is
// treated as mutating. Preserves the #757 coverage moved from the core
// readonly_guard_test.go (#778).
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
