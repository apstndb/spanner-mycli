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

// TestWriteBufferedRowsWithSpanvalueWriter pins the byte output of buffered
// Result replay through the spanvalue writers, which is the single emitter
// shared with the streaming paths for CSV, JSONL, and SQL_INSERT*.
func TestWriteBufferedRowsWithSpanvalueWriter(t *testing.T) {
	t.Parallel()

	rawJSONRow := func(texts ...string) Row {
		row := make(Row, len(texts))
		for i, text := range texts {
			row[i] = format.RawJSONCell{Cell: format.PlainCell{Text: text}}
		}
		return row
	}

	tests := []struct {
		name            string
		mode            enums.DisplayMode
		skipColumnNames bool
		sqlTableName    string // CLI_SQL_TABLE_NAME
		tableOverride   string // Result.SQLTableNameForExport
		sqlBatchSize    int64
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
			name:        "JSONL raw JSON cells pass through",
			mode:        enums.DisplayModeJSONL,
			columns:     []string{"n", "arr", "null"},
			rows:        []Row{rawJSONRow("42", "[1,2,3]", "null")},
			want:        `{"n":42,"arr":[1,2,3],"null":null}` + "\n",
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
			name:         "SQL_INSERT single rows",
			mode:         enums.DisplayModeSQLInsert,
			sqlTableName: "Users",
			columns:      []string{"id", "name"},
			rows:         []Row{toRow("1", "'Alice'"), toRow("2", "NULL")},
			want: "INSERT INTO `Users` (`id`, `name`) VALUES (1, 'Alice');\n" +
				"INSERT INTO `Users` (`id`, `name`) VALUES (2, NULL);\n",
			wantHandled: true,
		},
		{
			name:          "SQL_INSERT table override wins",
			mode:          enums.DisplayModeSQLInsert,
			sqlTableName:  "",
			tableOverride: "Detected",
			columns:       []string{"id"},
			rows:          []Row{toRow("1")},
			want:          "INSERT INTO `Detected` (`id`) VALUES (1);\n",
			wantHandled:   true,
		},
		{
			name:         "SQL_INSERT_OR_IGNORE batched with remainder",
			mode:         enums.DisplayModeSQLInsertOrIgnore,
			sqlTableName: "Users",
			sqlBatchSize: 2,
			columns:      []string{"id"},
			rows:         []Row{toRow("1"), toRow("2"), toRow("3")},
			want: "INSERT OR IGNORE INTO `Users` (`id`) VALUES\n  (1),\n  (2);\n" +
				"INSERT OR IGNORE INTO `Users` (`id`) VALUES\n  (3);\n",
			wantHandled: true,
		},
		{
			name:         "SQL_INSERT_OR_UPDATE qualified reserved table name",
			mode:         enums.DisplayModeSQLInsertOrUpdate,
			sqlTableName: "myschema.Order",
			columns:      []string{"id"},
			rows:         []Row{toRow("1")},
			want:         "INSERT OR UPDATE INTO `myschema`.`Order` (`id`) VALUES (1);\n",
			wantHandled:  true,
		},
		{
			name:         "SQL_INSERT missing table name",
			mode:         enums.DisplayModeSQLInsert,
			sqlTableName: "",
			columns:      []string{"id"},
			rows:         []Row{toRow("1")},
			wantHandled:  true,
			wantErr:      "SQL export requires a table name",
		},
		{
			name:         "SQL_INSERT empty column name",
			mode:         enums.DisplayModeSQLInsert,
			sqlTableName: "Users",
			columns:      []string{""},
			rows:         []Row{toRow("1")},
			wantHandled:  true,
			wantErr:      "SQL export requires all columns to have names",
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
			sysVars.Display.SQLTableName = tt.sqlTableName
			if tt.sqlBatchSize > 0 {
				sysVars.Display.SQLBatchSize = tt.sqlBatchSize
			}

			var buf bytes.Buffer
			handled, err := writeBufferedRowsWithSpanvalueWriter(&buf, &sysVars, tt.tableOverride, tt.columns, tt.rows)
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

// TestPrintTableDataBufferedSpanvalueReplay verifies that printTableData
// routes buffered non-table results through the spanvalue writers, including
// the SQL export mode gate on SQLExportAllowed.
func TestPrintTableDataBufferedSpanvalueReplay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mode   enums.DisplayMode
		result *Result
		want   string
	}{
		{
			name: "JSONL raw JSON cells",
			mode: enums.DisplayModeJSONL,
			result: &Result{
				TableHeader: toTableHeader("n"),
				Rows:        []Row{{format.RawJSONCell{Cell: format.PlainCell{Text: "42"}}}},
			},
			want: `{"n":42}` + "\n",
		},
		{
			name: "SQL export with SQL literals and detected table",
			mode: enums.DisplayModeSQLInsert,
			result: &Result{
				TableHeader:           toTableHeader("id"),
				Rows:                  []Row{toRow("1")},
				SQLExportAllowed:      true,
				SQLTableNameForExport: "Users",
			},
			want: "INSERT INTO `Users` (`id`) VALUES (1);\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := runPrintTableData(t, tt.mode, false, tt.result)
			if err != nil {
				t.Fatalf("printTableData() error = %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("output mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
