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

import "strings"

// firstCQLKeyword returns the uppercased first whitespace-delimited token of a
// CQL statement, or "" if there is none.
func firstCQLKeyword(cql string) string {
	fields := strings.Fields(cql)
	if len(fields) == 0 {
		return ""
	}
	return strings.ToUpper(fields[0])
}

// cqlStatementMutates reports whether a CQL statement should be treated as
// mutating for the purpose of the READONLY guard.
//
// It classifies by the statement's first keyword and is deliberately
// fail-closed: only statements whose first keyword is a known read-only verb
// are allowed under READONLY. Everything else - data mutations
// (INSERT/UPDATE/DELETE), batches (BATCH), DDL (CREATE/DROP/ALTER/TRUNCATE),
// permission changes (GRANT/REVOKE), and any unrecognized or empty keyword - is
// treated as mutating so the guard blocks it. This avoids silently allowing an
// unknown CQL verb to bypass READONLY.
//
// SELECT is the only clearly read-only data verb in the Spanner Cassandra
// adapter's supported surface. Other Cassandra read verbs (e.g. LIST, DESCRIBE)
// are not part of that surface, so they are intentionally not allow-listed.
func cqlStatementMutates(cql string) bool {
	switch firstCQLKeyword(cql) {
	case "SELECT":
		return false
	default:
		return true
	}
}
