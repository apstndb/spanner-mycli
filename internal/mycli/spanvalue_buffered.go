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

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanner-mycli/internal/mycli/decoder"
	"github.com/apstndb/spanner-mycli/internal/mycli/format"
	"github.com/apstndb/spanvalue"
	"google.golang.org/protobuf/types/known/structpb"
)

// spanvalueValuesWriter is the subset of the spanvalue writer API used to
// replay buffered rows. Every writer returned by
// newSpanvalueRowIteratorWriterFor (DelimitedWriter, JSONLWriter,
// SQLInsertWriter) implements it.
type spanvalueValuesWriter interface {
	PrepareColumnNames(names []string) error
	WriteValues(columnNames []string, values []spanner.GenericColumnValue) error
	Flush() error
}

// writeBufferedRowsWithSpanvalueWriter replays a buffered Result's
// pre-formatted cells through the spanvalue writer for the current CLI_FORMAT,
// so CSV, JSONL, and SQL_INSERT* bytes are emitted by exactly one
// implementation on both the streaming and buffered paths.
//
// Buffered Results only carry display-formatted cell text; the raw
// *spanner.Row values were consumed by the producing path (client-side
// statements build cells directly, DML THEN RETURN and partitioned-query
// buffering convert rows to cells at collection time). The cells are therefore
// re-encoded as pass-through GCVs (see preformattedCellGCV) instead of being
// re-formatted. Making buffered Results carry typed rows end-to-end is a
// separate redesign (typed buffered results); until then this replay keeps the
// emitters single-sourced.
//
// sqlTableName overrides CLI_SQL_TABLE_NAME for SQL export modes (the
// per-query auto-detected table stored in Result.SQLTableNameForExport).
// Returns handled=false when the current format has no spanvalue writer.
func writeBufferedRowsWithSpanvalueWriter(out io.Writer, sysVars *systemVariables, sqlTableName string, columnNames []string, rows []Row) (bool, error) {
	if out == nil || !usesSpanvalueWriter(sysVars.Display.CLIFormat) {
		return false, nil
	}

	fc := bufferedReplayFormatConfig(sysVars.Display.CLIFormat)

	if sqlTableName != "" && sysVars.Display.SQLTableName != sqlTableName {
		tempVars := *sysVars
		tempVars.Display.SQLTableName = sqlTableName
		sysVars = &tempVars
	}

	w, handled, err := newSpanvalueRowIteratorWriterFor(out, sysVars, fc)
	if err != nil || !handled {
		return handled, err
	}
	vw, ok := w.(spanvalueValuesWriter)
	if !ok {
		return true, fmt.Errorf("spanvalue writer %T does not support buffered row replay", w)
	}

	// Register the schema up front so header output (and zero-row exports)
	// does not depend on the first data row.
	if err := vw.PrepareColumnNames(columnNames); err != nil {
		return true, normalizeSpanvalueWriterError(err)
	}

	values := make([]spanner.GenericColumnValue, len(columnNames))
	for i, row := range rows {
		if len(row) != len(columnNames) {
			return true, fmt.Errorf("row %d has %d cells, want %d columns", i+1, len(row), len(columnNames))
		}
		for j, cell := range row {
			values[j] = preformattedCellGCV(cell)
		}
		if err := vw.WriteValues(columnNames, values); err != nil {
			return true, normalizeSpanvalueWriterError(err)
		}
	}
	if err := vw.Flush(); err != nil {
		return true, normalizeSpanvalueWriterError(err)
	}
	return true, nil
}

// bufferedReplayFormatConfig picks the FormatConfig that renders the
// pass-through GCVs built by preformattedCellGCV without altering their text.
func bufferedReplayFormatConfig(mode enums.DisplayMode) *spanvalue.FormatConfig {
	if mode == enums.DisplayModeJSONL {
		// JSON formatting: STRING GCVs become JSON strings and JSON-typed
		// (RawJSONCell) GCVs pass their wire text through unchanged.
		return decoder.JSONFormatConfig()
	}
	// CSV cells are display text and SQL export cells are already SQL
	// literals; SimpleFormatConfig renders STRING GCVs as their raw text.
	return spanvalue.SimpleFormatConfig()
}

// preformattedCellGCV re-encodes a pre-formatted cell as a GCV whose
// formatting under bufferedReplayFormatConfig reproduces the cell text:
// RawJSONCell text (a JSON fragment produced by withRawJSONMarker) becomes a
// JSON-typed GCV that spanvalue's JSON formatting passes through as-is; any
// other cell becomes a STRING GCV.
func preformattedCellGCV(cell format.Cell) spanner.GenericColumnValue {
	code := sppb.TypeCode_STRING
	if format.IsRawJSON(cell) {
		code = sppb.TypeCode_JSON
	}
	return spanner.GenericColumnValue{
		Type:  &sppb.Type{Code: code},
		Value: structpb.NewStringValue(cell.RawText()),
	}
}
