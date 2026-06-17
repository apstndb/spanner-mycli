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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// JSONLFormatter provides JSONL (JSON Lines) formatting logic.
// Each row is output as a single JSON object with column names as keys.
// WriteRow writes one top-level JSON object plus a newline, producing valid
// JSONL output. Column order is preserved.
//
// When cells are RawJSONCell, their text is written as raw JSON values
// (e.g., ARRAY as JSON array, INT64 as JSON number).
// Otherwise, values are output as JSON strings (fallback for client-side statements).
type JSONLFormatter struct {
	out         io.Writer
	columns     []string
	initialized bool
}

// NewJSONLFormatter creates a new JSONL formatter.
func NewJSONLFormatter(out io.Writer) *JSONLFormatter {
	return &JSONLFormatter{
		out: out,
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

	if _, err := io.WriteString(f.out, "{"); err != nil {
		return fmt.Errorf("failed to write JSONL row: %w", err)
	}

	for i, cell := range row {
		if i > 0 {
			if _, err := io.WriteString(f.out, ","); err != nil {
				return fmt.Errorf("failed to write JSONL separator: %w", err)
			}
		}

		var columnName string
		if i < len(f.columns) {
			columnName = f.columns[i]
		} else {
			columnName = fmt.Sprintf("Column_%d", i+1)
		}

		if err := writeJSONString(f.out, columnName); err != nil {
			return fmt.Errorf("failed to write JSONL key: %w", err)
		}
		if _, err := io.WriteString(f.out, ":"); err != nil {
			return fmt.Errorf("failed to write JSONL separator: %w", err)
		}

		if err := f.writeValue(cell); err != nil {
			return fmt.Errorf("failed to write JSONL value: %w", err)
		}
	}

	if _, err := io.WriteString(f.out, "}\n"); err != nil {
		return fmt.Errorf("failed to write JSONL row: %w", err)
	}

	return nil
}

// writeValue writes a cell's value to the encoder.
// RawJSONCell text is written as raw JSON values (the text is valid JSON).
// Other cells are written as quoted JSON strings.
func (f *JSONLFormatter) writeValue(cell Cell) error {
	if IsRawJSON(cell) {
		raw := []byte(cell.RawText())
		if !json.Valid(raw) {
			return fmt.Errorf("invalid raw JSON value: %q", cell.RawText())
		}
		_, err := f.out.Write(raw)
		return err
	}
	return writeJSONString(f.out, cell.RawText())
}

// FinishFormat completes JSONL output.
func (f *JSONLFormatter) FinishFormat() error {
	return nil
}

func writeJSONString(w io.Writer, s string) error {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return err
	}

	encoded := b.Bytes()
	if len(encoded) > 0 && encoded[len(encoded)-1] == '\n' {
		encoded = encoded[:len(encoded)-1]
	}
	_, err := w.Write(encoded)
	return err
}
