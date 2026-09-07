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
	"testing"

	dbadminpb "cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/google/go-cmp/cmp"
)

func TestParseDumpTableID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		want    tableID
		wantErr bool
	}{
		{name: "unqualified", input: "Users", want: tableID{Name: "Users"}},
		{name: "qualified", input: "Alpha.Users", want: tableID{Schema: "Alpha", Name: "Users"}},
		{name: "quoted reserved", input: "`select`.`Order`", want: tableID{Schema: "select", Name: "Order"}},
		{name: "quoted dotted name is one component", input: "`a.b`", want: tableID{Name: "a.b"}},
		{name: "three components", input: "a.b.c", wantErr: true},
		{name: "empty", input: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDumpTableID(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseDumpTableID(%q) = %v, want error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestTableIDCompareEmptySchemaFirst(t *testing.T) {
	t.Parallel()
	ids := []tableID{tidn("Alpha", "Users"), tid("Users"), tidn("Beta", "Users")}
	if ids[0].compare(ids[1]) <= 0 {
		t.Fatalf("named schema %q should sort after default %q", ids[0].FQN(), ids[1].FQN())
	}
	if ids[1].compare(ids[0]) >= 0 {
		t.Fatalf("default schema should sort first")
	}
	if ids[0].compare(ids[2]) >= 0 {
		t.Fatalf("Alpha should sort before Beta")
	}
}

func TestQuoteTableID(t *testing.T) {
	t.Parallel()
	dialect := dbadminpb.DatabaseDialect_GOOGLE_STANDARD_SQL
	if got := quoteTableID(dialect, tid("Users")); got != "`Users`" {
		t.Fatalf("got %q", got)
	}
	if got := quoteTableID(dialect, tidn("select", "Order")); got != "`select`.`Order`" {
		t.Fatalf("got %q", got)
	}
}

func TestUserSchemaExcluded(t *testing.T) {
	t.Parallel()
	if !userSchemaExcluded("INFORMATION_SCHEMA") || !userSchemaExcluded("information_schema") || !userSchemaExcluded("SPANNER_SYS") {
		t.Fatal("expected system schemas excluded")
	}
	if userSchemaExcluded("") || userSchemaExcluded("Alpha") {
		t.Fatal("user schemas must not be excluded")
	}
}
