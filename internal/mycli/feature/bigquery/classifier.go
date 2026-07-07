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

import "strings"

// firstBigQueryKeyword returns the uppercased first whitespace-delimited token
// of a BigQuery statement, or "" if there is none.
func firstBigQueryKeyword(sql string) string {
	start := -1
	for i := range len(sql) {
		if !isBigQueryKeywordWhitespace(sql[i]) {
			start = i
			break
		}
	}
	if start == -1 {
		return ""
	}

	end := len(sql)
	for i := start; i < len(sql); i++ {
		if isBigQueryKeywordWhitespace(sql[i]) {
			end = i
			break
		}
	}
	return strings.ToUpper(sql[start:end])
}

func isBigQueryKeywordWhitespace(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	default:
		return false
	}
}

// bigQueryStatementMutates reports whether a BIGQUERY statement should be
// treated as mutating for the READONLY guard.
//
// The classifier is deliberately fail-closed: only BigQuery statements whose
// first keyword is a known read-only query verb are allowed under READONLY.
// Everything else, including unrecognized, empty, DML, DDL, and script-control
// statements, is treated as mutating and blocked before it can reach BigQuery.
func bigQueryStatementMutates(sql string) bool {
	switch firstBigQueryKeyword(sql) {
	case "SELECT", "WITH":
		return false
	default:
		return true
	}
}
