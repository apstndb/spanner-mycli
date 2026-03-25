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
// Each row is output as a single JSON object with column names as keys
// and all values as JSON strings. The jsontext.Encoder writes a newline
// after each top-level JSON value, producing valid JSONL output.
// Column order is preserved (unlike encoding/json with map[string]string).
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

		if err := f.enc.WriteToken(jsontext.String(cell.RawText())); err != nil {
			return fmt.Errorf("failed to write JSONL value: %w", err)
		}
	}

	if err := f.enc.WriteToken(jsontext.EndObject); err != nil {
		return fmt.Errorf("failed to write JSONL row: %w", err)
	}

	return nil
}

// FinishFormat completes JSONL output.
func (f *JSONLFormatter) FinishFormat() error {
	return nil
}
