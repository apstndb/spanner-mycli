// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package format

import (
	"fmt"
	"io"

	"github.com/go-json-experiment/json/jsontext"
)

// JSONLFormatter provides JSONL (JSON Lines) formatting logic.
// Each row is output as a single JSON object with column names as keys.
// The jsontext.Encoder writes a newline after each top-level JSON value,
// producing valid JSONL output. Column order is preserved.
//
// When cells are RawJSONCell, their text is written as raw JSON values
// (e.g., ARRAY as JSON array, INT64 as JSON number).
// Otherwise, values are output as JSON strings (fallback for client-side statements).
type JSONLFormatter struct {
	enc         *jsontext.Encoder
	columns     []string
	initialized bool
}

// NewJSONLFormatter creates a new JSONL formatter.
func NewJSONLFormatter(out io.Writer) *JSONLFormatter {
	return &JSONLFormatter{
		enc: jsontext.NewEncoder(out),
	}
}

// InitFormat stores column names for use as JSON keys.
func (f *JSONLFormatter) InitFormat(columnNames []string, config FormatConfig, previewRows []Row) error {
	if f.initialized {
		return nil
	}

	f.columns = columnNames
	f.initialized = true
	return nil
}

// WriteRow writes a single row as a JSON object on one line.
func (f *JSONLFormatter) WriteRow(row Row) error {
	if !f.initialized {
		return fmt.Errorf("JSONL formatter not initialized")
	}

	if err := f.enc.WriteToken(jsontext.BeginObject); err != nil {
		return fmt.Errorf("failed to write JSONL row: %w", err)
	}

	for i, cell := range row {
		var columnName string
		if i < len(f.columns) {
			columnName = f.columns[i]
		} else {
			columnName = fmt.Sprintf("Column_%d", i+1)
		}

		if err := f.enc.WriteToken(jsontext.String(columnName)); err != nil {
			return fmt.Errorf("failed to write JSONL key: %w", err)
		}

		if err := f.writeValue(cell); err != nil {
			return fmt.Errorf("failed to write JSONL value: %w", err)
		}
	}

	if err := f.enc.WriteToken(jsontext.EndObject); err != nil {
		return fmt.Errorf("failed to write JSONL row: %w", err)
	}

	return nil
}

// writeValue writes a cell's value to the encoder.
// RawJSONCell text is written as raw JSON values (the text is valid JSON).
// Other cells are written as quoted JSON strings.
func (f *JSONLFormatter) writeValue(cell Cell) error {
	if IsRawJSON(cell) {
		return f.enc.WriteValue(jsontext.Value(cell.RawText()))
	}
	return f.enc.WriteToken(jsontext.String(cell.RawText()))
}

// FinishFormat completes JSONL output.
func (f *JSONLFormatter) FinishFormat() error {
	return nil
}
