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
	"testing"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spancodec"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanner-mycli/internal/mycli/decoder"
	"github.com/apstndb/spanvalue"
	"github.com/apstndb/spanvalue/writer"
	"github.com/google/go-cmp/cmp"
)

type exportIdentRow struct {
	ID   int64  `spanner:"id"`
	Name string `spanner:"name"`
}

const sqlExportMissingTableNameErr = "SQL export requires a table name. Auto-detection failed (query may be too complex).\n" +
	"Options:\n" +
	"  1. Use DUMP TABLE for full table exports\n" +
	"  2. Set CLI_SQL_TABLE_NAME explicitly for complex queries\n" +
	"  3. Ensure your query matches: SELECT * FROM table_name [WHERE/ORDER BY/LIMIT]"

func TestNewSpanvalueRowIteratorWriterForContract(t *testing.T) {
	t.Parallel()

	fc, err := decoder.FormatConfigWithProto(nil, false)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("nil destination is unhandled buffered fallback", func(t *testing.T) {
		t.Parallel()
		w, handled, err := newSpanvalueRowIteratorWriterFor(nil, exportWriterOptions{CLIFormat: enums.DisplayModeCSV}, fc)
		if w != nil || handled || err != nil {
			t.Fatalf("got writer=%v handled=%v err=%v, want nil, false, nil", w, handled, err)
		}
	})

	t.Run("unsupported format is unhandled", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		w, handled, err := newSpanvalueRowIteratorWriterFor(&buf, exportWriterOptions{CLIFormat: enums.DisplayModeTable}, fc)
		if w != nil || handled || err != nil {
			t.Fatalf("got writer=%v handled=%v err=%v, want nil, false, nil", w, handled, err)
		}
	})

	t.Run("SQL export missing table name is handled with exact error text", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		w, handled, err := newSpanvalueRowIteratorWriterFor(&buf, exportWriterOptions{CLIFormat: enums.DisplayModeSQLInsert}, sqlLiteralFormatConfig())
		if w != nil || !handled {
			t.Fatalf("got writer=%v handled=%v, want nil, true", w, handled)
		}
		if err == nil || err.Error() != sqlExportMissingTableNameErr {
			t.Fatalf("error mismatch (-want +got):\n%s", cmp.Diff(sqlExportMissingTableNameErr, errString(err)))
		}
		if buf.Len() != 0 {
			t.Fatalf("writer wrote %q before failing", buf.String())
		}
	})

	t.Run("negative SQL batch size is handled with error", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		_, handled, err := newSpanvalueRowIteratorWriterFor(&buf, exportWriterOptions{
			CLIFormat:    enums.DisplayModeSQLInsert,
			SQLTableName: "Items",
			SQLBatchSize: -1,
		}, sqlLiteralFormatConfig())
		if !handled {
			t.Fatal("negative batch size should be handled")
		}
		want := "CLI_SQL_BATCH_SIZE cannot be negative: -1"
		if err == nil || err.Error() != want {
			t.Fatalf("error mismatch (-want +got):\n%s", cmp.Diff(want, errString(err)))
		}
	})

	t.Run("SQL batch size above maximum is handled with error", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		_, handled, err := newSpanvalueRowIteratorWriterFor(&buf, exportWriterOptions{
			CLIFormat:    enums.DisplayModeSQLInsert,
			SQLTableName: "Items",
			SQLBatchSize: 10001,
		}, sqlLiteralFormatConfig())
		if !handled {
			t.Fatal("oversized batch size should be handled")
		}
		want := "CLI_SQL_BATCH_SIZE 10001 exceeds maximum supported value of 10000 (limited for Spanner mutation constraints)"
		if err == nil || err.Error() != want {
			t.Fatalf("error mismatch (-want +got):\n%s", cmp.Diff(want, errString(err)))
		}
	})
}

func TestNewSpanvalueRowIteratorWriterForOutput(t *testing.T) {
	t.Parallel()

	md, rows := mustExportIdentRows(t, []exportIdentRow{
		{ID: 1, Name: "Alice"},
		{ID: 2, Name: "Bob"},
		{ID: 3, Name: "Carol"},
	})

	tests := []struct {
		name string
		opts exportWriterOptions
		fc   *spanvalue.FormatConfig
		want string
	}{
		{
			name: "CSV includes headers",
			opts: exportWriterOptions{CLIFormat: enums.DisplayModeCSV},
			want: "id,name\n1,Alice\n2,Bob\n3,Carol\n",
		},
		{
			name: "CSV skip column names",
			opts: exportWriterOptions{CLIFormat: enums.DisplayModeCSV, SkipColumnNames: true},
			want: "1,Alice\n2,Bob\n3,Carol\n",
		},
		{
			name: "JSONL values",
			opts: exportWriterOptions{CLIFormat: enums.DisplayModeJSONL},
			want: "{\"id\":1,\"name\":\"Alice\"}\n{\"id\":2,\"name\":\"Bob\"}\n{\"id\":3,\"name\":\"Carol\"}\n",
		},
		{
			name: "SQL INSERT uses table name and GoogleSQL string quotes",
			opts: exportWriterOptions{CLIFormat: enums.DisplayModeSQLInsert, SQLTableName: "Items"},
			fc:   sqlLiteralFormatConfig(),
			want: "INSERT INTO `Items` (`id`, `name`) VALUES (1, \"Alice\");\nINSERT INTO `Items` (`id`, `name`) VALUES (2, \"Bob\");\nINSERT INTO `Items` (`id`, `name`) VALUES (3, \"Carol\");\n",
		},
		{
			name: "SQL INSERT OR IGNORE",
			opts: exportWriterOptions{CLIFormat: enums.DisplayModeSQLInsertOrIgnore, SQLTableName: "Items"},
			fc:   sqlLiteralFormatConfig(),
			want: "INSERT OR IGNORE INTO `Items` (`id`, `name`) VALUES (1, \"Alice\");\nINSERT OR IGNORE INTO `Items` (`id`, `name`) VALUES (2, \"Bob\");\nINSERT OR IGNORE INTO `Items` (`id`, `name`) VALUES (3, \"Carol\");\n",
		},
		{
			name: "SQL INSERT OR UPDATE",
			opts: exportWriterOptions{CLIFormat: enums.DisplayModeSQLInsertOrUpdate, SQLTableName: "Items"},
			fc:   sqlLiteralFormatConfig(),
			want: "INSERT OR UPDATE INTO `Items` (`id`, `name`) VALUES (1, \"Alice\");\nINSERT OR UPDATE INTO `Items` (`id`, `name`) VALUES (2, \"Bob\");\nINSERT OR UPDATE INTO `Items` (`id`, `name`) VALUES (3, \"Carol\");\n",
		},
		{
			name: "SQL batch size groups VALUES lists",
			opts: exportWriterOptions{CLIFormat: enums.DisplayModeSQLInsert, SQLTableName: "Items", SQLBatchSize: 2},
			fc:   sqlLiteralFormatConfig(),
			want: "INSERT INTO `Items` (`id`, `name`) VALUES\n  (1, \"Alice\"),\n  (2, \"Bob\");\nINSERT INTO `Items` (`id`, `name`) VALUES\n  (3, \"Carol\");\n",
		},
		{
			name: "PostgreSQL dialect quotes identifiers",
			opts: exportWriterOptions{
				CLIFormat:       enums.DisplayModeSQLInsert,
				SQLTableName:    "Items",
				DatabaseDialect: databasepb.DatabaseDialect_POSTGRESQL,
			},
			fc:   sqlLiteralFormatConfig(),
			want: "INSERT INTO \"Items\" (\"id\", \"name\") VALUES (1, \"Alice\");\nINSERT INTO \"Items\" (\"id\", \"name\") VALUES (2, \"Bob\");\nINSERT INTO \"Items\" (\"id\", \"name\") VALUES (3, \"Carol\");\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fc := tt.fc
			if fc == nil {
				var err error
				if tt.opts.CLIFormat == enums.DisplayModeJSONL {
					fc = decoder.JSONFormatConfig()
				} else {
					fc, err = decoder.FormatConfigWithProto(nil, false)
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			var buf bytes.Buffer
			w, handled, err := newSpanvalueRowIteratorWriterFor(&buf, tt.opts, fc)
			if err != nil || !handled {
				t.Fatalf("newSpanvalueRowIteratorWriterFor: handled=%v err=%v", handled, err)
			}
			if _, err := writer.WriteRowSeq(md, writer.RowSeq(rows...), w); err != nil {
				t.Fatalf("WriteRowSeq: %v", err)
			}
			if diff := cmp.Diff(tt.want, buf.String()); diff != "" {
				t.Errorf("output mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func mustExportIdentRows(t *testing.T, items []exportIdentRow) (*sppb.ResultSetMetadata, []*spanner.Row) {
	t.Helper()
	enc := spancodec.MustNewRowEncoder[exportIdentRow]()
	md, err := enc.ResultSetMetadata()
	if err != nil {
		t.Fatal(err)
	}
	var rows []*spanner.Row
	for row, err := range enc.Rows(items) {
		if err != nil {
			t.Fatal(err)
		}
		rows = append(rows, row)
	}
	return md, rows
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
