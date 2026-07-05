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
	"github.com/apstndb/spancodec"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanner-mycli/internal/mycli/format"
	"github.com/apstndb/spanvalue/writer"
	"github.com/google/go-cmp/cmp"
)

// identRow exercises several value types so the byte-identity check covers
// number/string/bool/float rendering across every CLI_FORMAT.
type identRow struct {
	N int64   `spanner:"n"`
	S string  `spanner:"s"`
	B bool    `spanner:"b"`
	F float64 `spanner:"f"`
}

// TestTypedRowsByteIdentity is the byte-identity regression suite for the typed
// buffered producer (issue #738 section 6). For every CLI_FORMAT, rendering a
// typed buffered result (Result.Typed) must be byte-identical to the canonical
// emitter for that format:
//
//   - Export formats (CSV/JSONL/SQL_INSERT*): the streaming writer that a live
//     query uses (newSpanvalueRowIteratorWriterFor + writer.WriteRowSeq over the
//     same raw rows). This is the single source of truth those formats stream
//     through; the buffered typed path must produce the same bytes.
//   - Table-family formats: the same raw rows eagerly transformed to display
//     cells (Result.Rows) with the execution-time FormatConfig, then rendered as
//     a presentation table. Table rendering is unchanged by the decomposition.
//
// (Before PR5 the export reference was also eager display cells replayed through
// a pass-through-GCV writer; PR5 deleted that replay, so export formats now
// compare against the streaming writer directly.)
func TestTypedRowsByteIdentity(t *testing.T) {
	t.Parallel()

	enc := spancodec.MustNewRowEncoder[identRow]()
	items := []identRow{
		{N: 1, S: "Alice", B: true, F: 1.5},
		{N: 2, S: "Bob,\"quoted\"\nline2", B: false, F: -3.25},
	}
	md, err := enc.ResultSetMetadata()
	if err != nil {
		t.Fatalf("ResultSetMetadata: %v", err)
	}
	var rawRows []*spanner.Row
	for row, err := range enc.Rows(items) {
		if err != nil {
			t.Fatalf("encode row: %v", err)
		}
		rawRows = append(rawRows, row)
	}
	fields := md.GetRowType().GetFields()
	header := toTableHeader(fields)

	for _, mode := range enums.DisplayModeValues() {
		if mode == enums.DisplayModeUnspecified {
			continue
		}
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()

			sv := newSystemVariablesWithDefaults()
			sv.Display.CLIFormat = mode
			sv.Display.SQLTableName = "Items" // required by SQL export modes

			fc, vfm, sv2, err := prepareFormatConfig("SELECT * FROM Items", &sv)
			if err != nil {
				t.Fatalf("prepareFormatConfig: %v", err)
			}
			sqlExport := vfm == format.SQLLiteralValues

			// Reference bytes for the format's canonical emitter.
			var wantBuf bytes.Buffer
			if usesSpanvalueWriter(mode) {
				// Export formats stream through this writer for live queries.
				w, handled, err := newSpanvalueRowIteratorWriterFor(&wantBuf, sv2, fc)
				if err != nil || !handled {
					t.Fatalf("newSpanvalueRowIteratorWriterFor: handled=%v err=%v", handled, err)
				}
				if _, err := writer.WriteRowSeq(md, rowSeq(rawRows), w); err != nil {
					t.Fatalf("WriteRowSeq: %v", err)
				}
			} else {
				// Table-family formats: eager display cells rendered as a table.
				transform := spannerRowToRow(fc, sv2.typeStyles, sv2.nullStyle)
				if vfm == format.JSONValues {
					transform = withRawJSONMarker(transform)
				}
				var oldRows []Row
				for _, row := range rawRows {
					cells, err := transform(row)
					if err != nil {
						t.Fatalf("transform: %v", err)
					}
					oldRows = append(oldRows, cells)
				}
				oldResult := &Result{
					Rows:         oldRows,
					TableHeader:  header,
					AffectedRows: len(rawRows),
				}
				if err := printTableData(&sv, 0, &wantBuf, oldResult); err != nil {
					t.Fatalf("printTableData(reference): %v", err)
				}
			}

			newResult := &Result{
				Typed:                 &TypedRows{Metadata: md, Rows: rawRows, SQLExportAllowed: sqlExport},
				TableHeader:           header,
				AffectedRows:          len(rawRows),
				SQLTableNameForExport: sv2.Display.SQLTableName,
			}

			var newBuf bytes.Buffer
			if err := printTableData(&sv, 0, &newBuf, newResult); err != nil {
				t.Fatalf("printTableData(new): %v", err)
			}
			if wantBuf.Len() == 0 {
				t.Fatalf("expected non-empty output for %s", mode)
			}
			if diff := cmp.Diff(wantBuf.String(), newBuf.String()); diff != "" {
				t.Errorf("typed replay is not byte-identical for %s (-want +got):\n%s", mode, diff)
			}
		})
	}
}

// bodyPayloadCount reports how many mutually-exclusive body payloads a Result
// carries. The exclusivity invariant (issue #738 section 2) requires at most
// one of Rows, Typed, RenderedOutput to be set.
func bodyPayloadCount(r *Result) int {
	n := 0
	if len(r.Rows) > 0 {
		n++
	}
	if r.Typed != nil {
		n++
	}
	if len(r.RenderedOutput) > 0 {
		n++
	}
	return n
}

// TestResultBodyPayloadExclusive verifies the at-most-one-body-payload
// invariant: the typed buffered producer sets only Typed, and a Result that
// sets two payloads is detectable as a violation.
func TestResultBodyPayloadExclusive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		result *Result
		want   int
	}{
		{
			name:   "typed buffered query result sets only Typed",
			result: &Result{Typed: &TypedRows{Rows: []*spanner.Row{}}, TableHeader: toTableHeader("n")},
			want:   1,
		},
		{
			name:   "presentation table sets only Rows",
			result: &Result{Rows: []Row{toRow("1")}, TableHeader: toTableHeader("n")},
			want:   1,
		},
		{
			name:   "rendered output only",
			result: &Result{RenderedOutput: []byte("x")},
			want:   1,
		},
		{
			name:   "streamed result carries no body payload",
			result: &Result{Streamed: true, TableHeader: toTableHeader("n")},
			want:   0,
		},
		{
			name:   "two payloads violate exclusivity",
			result: &Result{Rows: []Row{toRow("1")}, Typed: &TypedRows{}},
			want:   2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := bodyPayloadCount(tt.result)
			if got != tt.want {
				t.Errorf("bodyPayloadCount = %d, want %d", got, tt.want)
			}
			if tt.want <= 1 && got > 1 {
				t.Errorf("result violates at-most-one-body-payload invariant")
			}
		})
	}
}
