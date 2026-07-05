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
	"bytes"
	"strings"
	"testing"

	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanner-mycli/internal/mycli/format"
	"github.com/google/go-cmp/cmp"
)

// TestWriteDisplayRows pins the byte output of the display-text presentation
// replay (kind (a) results: EXPLAIN trees, SHOW OPERATION, batch summaries),
// which routes plain cell texts through the single spanvalue writers as
// STRING-typed synthetic rows. This is the successor of the deleted
// pass-through-GCV replay (issue #738 PR5); typed producers are covered by
// TestTypedRowsByteIdentity, so only CSV and JSONL (the formats a presentation
// table can reach) are exercised here.
func TestWriteDisplayRows(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		mode            enums.DisplayMode
		skipColumnNames bool
		columns         []string
		rows            []Row
		want            string
		wantHandled     bool
		wantErr         string
	}{
		{
			name:        "CSV basic",
			mode:        enums.DisplayModeCSV,
			columns:     []string{"id", "name"},
			rows:        []Row{toRow("1", "Alice"), toRow("2", "Bob")},
			want:        "id,name\n1,Alice\n2,Bob\n",
			wantHandled: true,
		},
		{
			name:            "CSV skip headers",
			mode:            enums.DisplayModeCSV,
			skipColumnNames: true,
			columns:         []string{"id", "name"},
			rows:            []Row{toRow("1", "Alice")},
			want:            "1,Alice\n",
			wantHandled:     true,
		},
		{
			name:        "CSV special characters",
			mode:        enums.DisplayModeCSV,
			columns:     []string{"col"},
			rows:        []Row{toRow("value with, comma"), toRow("value with \"quotes\"")},
			want:        "col\n\"value with, comma\"\n\"value with \"\"quotes\"\"\"\n",
			wantHandled: true,
		},
		{
			name:        "CSV zero rows still writes header",
			mode:        enums.DisplayModeCSV,
			columns:     []string{"id"},
			rows:        nil,
			want:        "id\n",
			wantHandled: true,
		},
		{
			name:        "CSV styled cell uses raw text",
			mode:        enums.DisplayModeCSV,
			columns:     []string{"id", "value"},
			rows:        []Row{{format.PlainCell{Text: "1"}, format.StyledCell{Text: "NULL", Style: "\033[2m"}}},
			want:        "id,value\n1,NULL\n",
			wantHandled: true,
		},
		{
			name:        "JSONL plain cells become JSON strings",
			mode:        enums.DisplayModeJSONL,
			columns:     []string{"id", "name"},
			rows:        []Row{toRow("1", "Alice"), toRow("2", "Bob")},
			want:        `{"id":"1","name":"Alice"}` + "\n" + `{"id":"2","name":"Bob"}` + "\n",
			wantHandled: true,
		},
		{
			name:        "JSONL string escaping",
			mode:        enums.DisplayModeJSONL,
			columns:     []string{"col"},
			rows:        []Row{toRow("value with \"quotes\""), toRow("line1\nline2")},
			want:        `{"col":"value with \"quotes\""}` + "\n" + `{"col":"line1\nline2"}` + "\n",
			wantHandled: true,
		},
		{
			name:        "JSONL does not escape HTML characters in strings",
			mode:        enums.DisplayModeJSONL,
			columns:     []string{"SPANNER_TYPE"},
			rows:        []Row{toRow("ARRAY<STRUCT<COLUMN STRING(MAX), LOCK_MODE STRING(MAX)>>")},
			want:        `{"SPANNER_TYPE":"ARRAY<STRUCT<COLUMN STRING(MAX), LOCK_MODE STRING(MAX)>>"}` + "\n",
			wantHandled: true,
		},
		{
			name:        "JSONL zero rows writes nothing",
			mode:        enums.DisplayModeJSONL,
			columns:     []string{"id"},
			rows:        nil,
			want:        "",
			wantHandled: true,
		},
		{
			name:        "row and column count mismatch",
			mode:        enums.DisplayModeCSV,
			columns:     []string{"id", "name"},
			rows:        []Row{toRow("1")},
			wantHandled: true,
			wantErr:     "has 1 cells, want 2 columns",
		},
		{
			name:        "table mode is not handled",
			mode:        enums.DisplayModeTable,
			columns:     []string{"id"},
			rows:        []Row{toRow("1")},
			wantHandled: false,
		},
		{
			name:        "vertical mode is not handled",
			mode:        enums.DisplayModeVertical,
			columns:     []string{"id"},
			rows:        []Row{toRow("1")},
			wantHandled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sysVars := newSystemVariablesWithDefaults()
			sysVars.Display.CLIFormat = tt.mode
			sysVars.Display.SkipColumnNames = tt.skipColumnNames

			var buf bytes.Buffer
			handled, err := writeDisplayRows(&buf, &sysVars, tt.columns, tt.rows)
			if handled != tt.wantHandled {
				t.Fatalf("handled = %v, want %v", handled, tt.wantHandled)
			}
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil (output %q)", tt.wantErr, buf.String())
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !handled {
				return
			}
			if diff := cmp.Diff(tt.want, buf.String()); diff != "" {
				t.Errorf("output mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestPrintTableDataDisplayReplay verifies that printTableData routes a
// presentation Result (display-text Rows, no Typed payload) through the display
// replay for non-table export formats.
func TestPrintTableDataDisplayReplay(t *testing.T) {
	t.Parallel()

	result := &Result{
		TableHeader: toTableHeader("id", "name"),
		Rows:        []Row{toRow("1", "Alice")},
	}

	got, err := runPrintTableData(t, enums.DisplayModeJSONL, false, result)
	if err != nil {
		t.Fatalf("printTableData() error = %v", err)
	}
	want := `{"id":"1","name":"Alice"}` + "\n"
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("output mismatch (-want +got):\n%s", diff)
	}
}
