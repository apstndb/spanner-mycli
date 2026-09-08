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

import (
	"strings"
	"testing"
)

func TestMutateQuotedNamedSchemaAndLowercaseDelete(t *testing.T) {
	t.Parallel()
	skipIfShortIntegration(t)
	_, session := initializeWithRandomDB(t, []string{
		"CREATE SCHEMA AuditSchema",
		"CREATE TABLE AuditSchema.Target (Id INT64 NOT NULL) PRIMARY KEY (Id)",
		"CREATE TABLE Target (Id INT64 NOT NULL) PRIMARY KEY (Id)",
	}, nil)
	mustExec := func(sql string) {
		t.Helper()
		stmt, err := BuildStatement(sql)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := stmt.Execute(t.Context(), session); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	mustExec("BEGIN RW")
	mustExec("MUTATE `AuditSchema`.`Target` INSERT STRUCT(1 AS Id)")
	mustExec("MUTATE AuditSchema.Target INSERT STRUCT(2 AS Id)")
	mustExec("MUTATE Target INSERT STRUCT(3 AS Id)")
	mustExec("COMMIT")

	got := func(sql string) string {
		t.Helper()
		stmt, err := BuildStatement(sql)
		if err != nil {
			t.Fatal(err)
		}
		res, err := stmt.Execute(t.Context(), session)
		if err != nil {
			t.Fatal(err)
		}
		res = normalizeResultForCompare(t, res)
		if len(res.Rows) == 0 {
			return ""
		}
		var out []string
		for _, row := range res.Rows {
			out = append(out, row[0].RawText())
		}
		return strings.Join(out, ",")
	}
	if g := got("SELECT Id FROM AuditSchema.Target ORDER BY Id"); g != "1,2" {
		t.Fatalf("named schema rows = %q", g)
	}
	if g := got("SELECT Id FROM Target ORDER BY Id"); g != "3" {
		t.Fatalf("default table rows = %q", g)
	}

	mustExec("BEGIN RW")
	mustExec("MUTATE `AuditSchema`.`Target` delete ALL")
	mustExec("COMMIT")
	if g := got("SELECT Id FROM AuditSchema.Target ORDER BY Id"); g != "" {
		t.Fatalf("after lowercase delete rows = %q", g)
	}
}
