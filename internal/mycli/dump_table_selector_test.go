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
	"errors"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseDumpTablesTailPatterns(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		want    *DumpTablesStatement
		wantErr string
	}{
		{
			name:  "like double-quoted string",
			input: `LIKE "User%"`,
			want:  &DumpTablesStatement{Selector: &dumpTableSelector{Like: []string{"User%"}}},
		},
		{
			name:  "like triple-quoted",
			input: "LIKE '''User%'''",
			want:  &DumpTablesStatement{Selector: &dumpTableSelector{Like: []string{"User%"}}},
		},
		{
			name:  "except only",
			input: "EXCEPT '%Tmp'",
			want:  &DumpTablesStatement{Selector: &dumpTableSelector{Except: []string{"%Tmp"}}},
		},
		{
			name:  "like lowercase keyword",
			input: "like 'User%'",
			want:  &DumpTablesStatement{Selector: &dumpTableSelector{Like: []string{"User%"}}},
		},
		{
			name:    "bytes literal rejected",
			input:   "LIKE b'User%'",
			wantErr: "string literal",
		},
		{
			name:    "trailing except leftover",
			input:   "EXCEPT 'Tmp%' LIKE 'User%'",
			wantErr: "unexpected input",
		},
		{
			name:    "empty like list",
			input:   "LIKE",
			wantErr: "string literal",
		},
		{
			name:    "trailing comma",
			input:   "LIKE 'User%',",
			wantErr: "trailing comma",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseDumpTablesTail(tt.input)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err=%v, want %q", err, tt.wantErr)
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

func TestBuildDumpTableSelectorQueryUsesParams(t *testing.T) {
	t.Parallel()
	sel := &dumpTableSelector{
		Like:   []string{"User%", "'); DROP TABLE Users; --"},
		Except: []string{"UserTest%"},
	}
	stmt, err := buildDumpTableSelectorQuery(sel)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stmt.SQL, "User%") || strings.Contains(stmt.SQL, "DROP TABLE") {
		t.Fatalf("interpolated pattern into SQL:\n%s", stmt.SQL)
	}
	if !strings.Contains(stmt.SQL, dumpTableDisplayIdentityExpr+" LIKE @like0") {
		t.Fatalf("missing parameterized LIKE:\n%s", stmt.SQL)
	}
	if strings.Contains(stmt.SQL, "ESCAPE") {
		t.Fatalf("unexpected ESCAPE clause:\n%s", stmt.SQL)
	}
	if stmt.Params["like0"] != "User%" || stmt.Params["like1"] != "'); DROP TABLE Users; --" || stmt.Params["except0"] != "UserTest%" {
		t.Fatalf("params=%v", stmt.Params)
	}

	if _, err := buildDumpTableSelectorQuery(&dumpTableSelector{}); !errors.Is(err, errDumpTablesMissingSelection) {
		t.Fatalf("empty selector: %v", err)
	}
}

func TestDumpTablesStatementRejectsMixedSelector(t *testing.T) {
	t.Parallel()
	stmt := &DumpTablesStatement{
		Tables:   []tableID{{Name: "Users"}},
		Selector: &dumpTableSelector{Like: []string{"User%"}},
	}
	if _, err := stmt.Execute(t.Context(), nil, OperationOutput{}); !errors.Is(err, errDumpTablesMixedSelector) {
		t.Fatalf("mixed: %v", err)
	}
}
