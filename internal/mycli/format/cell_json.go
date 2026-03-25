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

// RawJSONCell wraps a Cell to signal that its RawText() is a valid JSON value.
// JSON-aware formatters (like JSONL) write the text directly as raw JSON
// instead of quoting it as a JSON string. For cells without this wrapper
// (e.g., client-side statement results), the JSONL formatter quotes the
// text as a JSON string.
type RawJSONCell struct {
	Cell
}

func (c RawJSONCell) WithText(s string) Cell {
	return RawJSONCell{Cell: c.Cell.WithText(s)}
}

// IsRawJSON reports whether c is a RawJSONCell.
func IsRawJSON(c Cell) bool {
	_, ok := c.(RawJSONCell)
	return ok
}
