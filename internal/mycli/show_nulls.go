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
	"cloud.google.com/go/spanner"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanvalue"
)

// showNullsMarker is the fixed SQL NULL spelling used when CLI_SHOW_NULLS is
// enabled in an accepted human table format. It is not user-configurable.
const showNullsMarker = "<NULL>"

func isShowNullsDisplayMode(mode enums.DisplayMode) bool {
	switch mode {
	case enums.DisplayModeTable, enums.DisplayModeTableComment, enums.DisplayModeTableDetailComment, enums.DisplayModeVertical:
		return true
	default:
		return false
	}
}

// applyShowNullsDisplay returns a display FormatConfig that, when CLI_SHOW_NULLS
// is enabled and mode is TABLE / TABLE_COMMENT / TABLE_DETAIL_COMMENT /
// VERTICAL, renders SQL NULL as <NULL> (including nested ARRAY/STRUCT NULLs
// via NullString) and quote-wraps an ordinary STRING whose current formatted
// text is exactly that marker. The input config is not mutated. Shared
// presets stay untouched. Excluded formats, including SQL-export fallback
// that still renders as a table, keep their existing bytes.
func applyShowNullsDisplay(fc *spanvalue.FormatConfig, showNulls bool, mode enums.DisplayMode) (*spanvalue.FormatConfig, error) {
	if fc == nil || !showNulls || !isShowNullsDisplayMode(mode) {
		return fc, nil
	}

	updated := fc.WithComplexPlugin(spanvalue.PluginFromNullable(quoteShowNullsMarkerString))
	updated.NullString = showNullsMarker
	if err := updated.Validate(); err != nil {
		return nil, err
	}
	return updated, nil
}

func quoteShowNullsMarkerString(v spanvalue.NullableValue) (string, error) {
	ns, ok := v.(spanner.NullString)
	if !ok || !ns.Valid || ns.StringVal != showNullsMarker {
		return "", spanvalue.ErrFallthrough
	}
	return `"` + showNullsMarker + `"`, nil
}
