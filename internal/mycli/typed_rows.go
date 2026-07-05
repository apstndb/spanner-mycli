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
	"fmt"
	"io"
	"iter"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/internal/mycli/decoder"
	"github.com/apstndb/spanner-mycli/internal/mycli/format"
	"github.com/apstndb/spanvalue"
	"github.com/apstndb/spanvalue/writer"
)

// typedReplayFormatConfig picks the FormatConfig used to render a typed
// buffered result (Result.Typed) for the current CLI_FORMAT. It mirrors
// prepareFormatConfig's format decision without the SQL table-name
// auto-detection (that ran at execution time and is stored in
// Result.SQLTableNameForExport), so replay produces the same bytes the
// streaming path would have produced from the same raw rows.
func typedReplayFormatConfig(sysVars *systemVariables) (*spanvalue.FormatConfig, format.ValueFormatMode, error) {
	vfm := format.ValueFormatModeFor(format.Mode(sysVars.Display.CLIFormat.String()))
	switch vfm {
	case format.SQLLiteralValues:
		return spanvalue.LiteralFormatConfig(), vfm, nil
	case format.JSONValues:
		return decoder.JSONFormatConfig(), vfm, nil
	default:
		fc, err := decoder.FormatConfigWithProto(sysVars.Internal.ProtoDescriptor, sysVars.Display.MultilineProtoText)
		return fc, vfm, err
	}
}

// rowSeq adapts a materialized slice of rows to the iter.Seq2 shape consumed by
// writer.WriteRowSeq. The rows are already decoded, so iteration never errors.
func rowSeq(rows []*spanner.Row) iter.Seq2[*spanner.Row, error] {
	return func(yield func(*spanner.Row, error) bool) {
		for _, r := range rows {
			if !yield(r, nil) {
				return
			}
		}
	}
}

// writeTypedRows replays a typed buffered result's raw rows through the single
// spanvalue writer for the current export format (CSV/JSONL/SQL_INSERT*), the
// same emitters used by the streaming and client-side paths. The caller
// (printTableData) has already verified the format has a spanvalue writer and
// that SQL export is allowed, so handled is guaranteed true.
func writeTypedRows(out io.Writer, sysVars *systemVariables, result *Result) error {
	fc, _, err := typedReplayFormatConfig(sysVars)
	if err != nil {
		return err
	}

	// Apply the per-query auto-detected table name (Result.SQLTableNameForExport)
	// for SQL export without mutating the caller's systemVariables.
	sv := sysVars
	if n := result.SQLTableNameForExport; n != "" && sv.Display.SQLTableName != n {
		tmp := *sv
		tmp.Display.SQLTableName = n
		sv = &tmp
	}

	w, handled, err := newSpanvalueRowIteratorWriterFor(out, sv, fc)
	if err != nil {
		return err
	}
	if !handled {
		// printTableData only calls writeTypedRows for usesSpanvalueWriter
		// formats with a non-nil out, so newSpanvalueRowIteratorWriterFor must
		// handle them. Fail loudly rather than silently drop rows if the two
		// format sets ever diverge.
		return fmt.Errorf("no spanvalue writer for typed replay in format %v", sv.Display.CLIFormat)
	}
	if _, err := writer.WriteRowSeq(result.Typed.Metadata, rowSeq(result.Typed.Rows), w); err != nil {
		return normalizeSpanvalueWriterError(err)
	}
	return nil
}

// deriveDisplayRows converts a typed buffered result to display-text cells using
// the same transform as the buffered query path, so table-family formats render
// identically whether the rows arrived as Result.Rows or Result.Typed.
func deriveDisplayRows(sysVars *systemVariables, t *TypedRows) ([]Row, error) {
	fc, vfm, err := clientSideFormatContext(sysVars)
	if err != nil {
		return nil, err
	}
	transform := spannerRowToRow(fc, sysVars.typeStyles, sysVars.nullStyle)
	if vfm == format.JSONValues {
		transform = withRawJSONMarker(transform)
	}
	rows := make([]Row, 0, len(t.Rows))
	for _, r := range t.Rows {
		cells, err := transform(r)
		if err != nil {
			return nil, err
		}
		rows = append(rows, cells)
	}
	return rows, nil
}

// writeDisplayRows replays a presentation Result's display-text cells (kind (a):
// EXPLAIN trees, SHOW OPERATION, batch summaries, ...) through the single
// spanvalue writer for the current export format. It wraps the display texts as
// STRING-typed synthetic rows and routes them through writeTypedRows, so CSV,
// JSONL, and SQL_INSERT* bytes are emitted by exactly one implementation shared
// with the streaming and typed-buffered paths (issue #738 PR5 removes the former
// pass-through-GCV replay).
//
// Presentation tables never allow SQL export (Result.SQLExportAllowed is false
// and they carry no Typed payload), so printTableData falls back to TABLE before
// a SQL mode reaches here; only CSV and JSONL are handled in practice. Under
// those modes a STRING value renders identically to the previous SimpleFormatConfig
// (CSV: raw text) / JSONFormatConfig (JSONL: JSON string) replay. Returns
// handled=false when the current format has no spanvalue writer (TAB/VERTICAL/
// HTML/XML), matching the prior contract so the caller falls back to the
// streaming formatter.
func writeDisplayRows(out io.Writer, sysVars *systemVariables, columnNames []string, rows []Row) (bool, error) {
	if out == nil || !usesSpanvalueWriter(sysVars.Display.CLIFormat) {
		return false, nil
	}
	typed, err := stringRowsToTyped(columnNames, rows)
	if err != nil {
		return true, err
	}
	return true, writeTypedRows(out, sysVars, &Result{Typed: typed})
}

// stringRowsToTyped builds a TypedRows whose columns are all STRING and whose
// values are the raw display texts of the given cells, so display-text rows can
// reuse the typed replay writers. It errors if any row's cell count does not
// match columnNames.
func stringRowsToTyped(columnNames []string, rows []Row) (*TypedRows, error) {
	fields := make([]*sppb.StructType_Field, len(columnNames))
	for i, name := range columnNames {
		fields[i] = &sppb.StructType_Field{Name: name, Type: &sppb.Type{Code: sppb.TypeCode_STRING}}
	}
	metadata := &sppb.ResultSetMetadata{RowType: &sppb.StructType{Fields: fields}}

	typedRows := make([]*spanner.Row, len(rows))
	for i, row := range rows {
		if len(row) != len(columnNames) {
			return nil, fmt.Errorf("row %d has %d cells, want %d columns", i+1, len(row), len(columnNames))
		}
		values := make([]any, len(row))
		for j, cell := range row {
			values[j] = cell.RawText()
		}
		r, err := spanner.NewRow(columnNames, values)
		if err != nil {
			return nil, err
		}
		typedRows[i] = r
	}
	return &TypedRows{Metadata: metadata, Rows: typedRows}, nil
}
