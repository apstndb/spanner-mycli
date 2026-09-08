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

	"github.com/google/go-cmp/cmp"
)

func TestBuildStatementMutatePrefix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		input  string
		want   *MutateStatement
		errSub string
	}{
		{
			name:  "bare table insert",
			input: `MUTATE MutationTest INSERT STRUCT(1 AS PK)`,
			want:  &MutateStatement{Table: "MutationTest", Operation: "INSERT", Body: "STRUCT(1 AS PK)"},
		},
		{
			name:  "bare qualified",
			input: `MUTATE AuditSchema.Target INSERT STRUCT(1 AS Id)`,
			want:  &MutateStatement{Table: "AuditSchema.Target", Operation: "INSERT", Body: "STRUCT(1 AS Id)"},
		},
		{
			name:  "quoted components",
			input: "MUTATE `AuditSchema`.`Target` INSERT STRUCT(1 AS Id)",
			want:  &MutateStatement{Table: "AuditSchema.Target", Operation: "INSERT", Body: "STRUCT(1 AS Id)"},
		},
		{
			name:  "quoted reserved table",
			input: "MUTATE `select`.`Order` INSERT STRUCT(1 AS Id)",
			want:  &MutateStatement{Table: "select.Order", Operation: "INSERT", Body: "STRUCT(1 AS Id)"},
		},
		{
			name:  "quoted table named INSERT",
			input: "MUTATE `INSERT` INSERT STRUCT(1 AS Id)",
			want:  &MutateStatement{Table: "INSERT", Operation: "INSERT", Body: "STRUCT(1 AS Id)"},
		},
		{
			name:  "quoted component with dot",
			input: "MUTATE `a.b` INSERT STRUCT(1 AS Id)",
			want:  &MutateStatement{Table: "a.b", Operation: "INSERT", Body: "STRUCT(1 AS Id)"},
		},
		{
			name:  "whitespace around dot",
			input: "MUTATE AuditSchema . Target INSERT STRUCT(1 AS Id)",
			want:  &MutateStatement{Table: "AuditSchema.Target", Operation: "INSERT", Body: "STRUCT(1 AS Id)"},
		},
		{
			name:  "quoted whitespace component",
			input: "MUTATE `Audit Schema`.`Target` INSERT STRUCT(1 AS Id)",
			want:  &MutateStatement{Table: "Audit Schema.Target", Operation: "INSERT", Body: "STRUCT(1 AS Id)"},
		},
		{
			name:  "lowercase mutate insert",
			input: `mutate Target insert STRUCT(1 AS Id)`,
			want:  &MutateStatement{Table: "Target", Operation: "INSERT", Body: "STRUCT(1 AS Id)"},
		},
		{
			name:  "mixed insert_or_update",
			input: `MUTATE Target Insert_Or_Update STRUCT(1 AS Id)`,
			want:  &MutateStatement{Table: "Target", Operation: "INSERT_OR_UPDATE", Body: "STRUCT(1 AS Id)"},
		},
		{
			name:  "update",
			input: `MUTATE Target UPDATE STRUCT(1 AS Id)`,
			want:  &MutateStatement{Table: "Target", Operation: "UPDATE", Body: "STRUCT(1 AS Id)"},
		},
		{
			name:  "replace",
			input: `MUTATE Target REPLACE STRUCT(1 AS Id)`,
			want:  &MutateStatement{Table: "Target", Operation: "REPLACE", Body: "STRUCT(1 AS Id)"},
		},
		{
			name:  "uppercase delete",
			input: `MUTATE Target DELETE ALL`,
			want:  &MutateStatement{Table: "Target", Operation: "DELETE", Body: "ALL"},
		},
		{
			name:  "lowercase delete",
			input: `MUTATE Target delete ALL`,
			want:  &MutateStatement{Table: "Target", Operation: "DELETE", Body: "ALL"},
		},
		{
			name:  "mixed delete",
			input: `MUTATE Target Delete ALL`,
			want:  &MutateStatement{Table: "Target", Operation: "DELETE", Body: "ALL"},
		},
		{
			name:  "newline separator",
			input: "MUTATE Target INSERT\nSTRUCT(1 AS Id)",
			want:  &MutateStatement{Table: "Target", Operation: "INSERT", Body: "STRUCT(1 AS Id)"},
		},
		{
			name:  "tab separator",
			input: "MUTATE Target DELETE\tALL",
			want:  &MutateStatement{Table: "Target", Operation: "DELETE", Body: "ALL"},
		},
		{
			name:  "internal newline and escaped literal",
			input: "MUTATE Target INSERT STRUCT(\n\"a\\nb\" AS s)",
			want:  &MutateStatement{Table: "Target", Operation: "INSERT", Body: "STRUCT(\n\"a\\nb\" AS s)"},
		},
		{
			name:  "unicode quoted table component",
			input: "MUTATE `名`.`表` INSERT STRUCT(1 AS Id)",
			want:  &MutateStatement{Table: "名.表", Operation: "INSERT", Body: "STRUCT(1 AS Id)"},
		},
		{
			name:  "unicode body",
			input: `MUTATE Target INSERT STRUCT("こんにちは" AS s)`,
			want:  &MutateStatement{Table: "Target", Operation: "INSERT", Body: `STRUCT("こんにちは" AS s)`},
		},
		{
			name:  "operation-like text in body",
			input: `MUTATE Target INSERT STRUCT("DELETE ALL" AS s)`,
			want:  &MutateStatement{Table: "Target", Operation: "INSERT", Body: `STRUCT("DELETE ALL" AS s)`},
		},
		{name: "missing args", input: `MUTATE`, errSub: "requires"},
		{name: "missing operation", input: `MUTATE Target`, errSub: "operation"},
		{name: "missing body", input: `MUTATE Target INSERT`, errSub: "body"},
		{name: "unknown operation", input: `MUTATE Target FOO STRUCT(1 AS Id)`, errSub: "operation"},
		{name: "quoted operation", input: "MUTATE Target `DELETE` ALL", errSub: "unquoted"},
		{name: "three parts", input: `MUTATE a.b.c INSERT STRUCT(1 AS Id)`, errSub: "components"},
		{name: "empty component", input: `MUTATE a..b INSERT STRUCT(1 AS Id)`, errSub: "invalid MUTATE table"},
		{name: "adjacent quoted", input: "MUTATE A``B INSERT STRUCT(1 AS Id)", errSub: "invalid"},
		{name: "adjacent delete bracket", input: `MUTATE Target DELETE[1]`, errSub: "whitespace after operation"},
		{name: "adjacent delete paren", input: `MUTATE Target DELETE(1)`, errSub: "whitespace after operation"},
		{name: "adjacent insert paren", input: `MUTATE Target INSERT(1)`, errSub: "whitespace after operation"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := BuildStatement(tt.input)
			if tt.errSub != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errSub) {
					t.Fatalf("error = %v, want substring %q", err, tt.errSub)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			stmt, ok := got.(*MutateStatement)
			if !ok {
				t.Fatalf("type %T", got)
			}
			if diff := cmp.Diff(tt.want, stmt); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestParseMutationDeleteCase(t *testing.T) {
	t.Parallel()
	for _, op := range []string{"DELETE", "delete", "Delete"} {
		t.Run(op, func(t *testing.T) {
			t.Parallel()
			got, err := parseMutation("t", op, "ALL")
			if err != nil {
				t.Fatalf("parseMutation(%q) error = %v", op, err)
			}
			if len(got) != 1 {
				t.Fatalf("count = %d", len(got))
			}
		})
	}
}
