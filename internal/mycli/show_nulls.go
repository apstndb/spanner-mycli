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
	"strconv"
	"strings"

	"cloud.google.com/go/spanner"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanvalue"
)

// showNullsLiteral is the exact STRING value that CLI_SHOW_NULLS always quotes.
// SQL NULL keeps the existing NullString spelling ("NULL").
const showNullsLiteral = "NULL"

func isShowNullsDisplayMode(mode enums.DisplayMode) bool {
	switch mode {
	case enums.DisplayModeTable, enums.DisplayModeTableComment, enums.DisplayModeTableDetailComment, enums.DisplayModeVertical:
		return true
	default:
		return false
	}
}

func needsShowNullsQuote(s string) bool {
	return s == showNullsLiteral || strings.HasPrefix(s, `"`)
}

// applyShowNullsDisplay returns a display FormatConfig that, when CLI_SHOW_NULLS
// is enabled and mode is TABLE / TABLE_COMMENT / TABLE_DETAIL_COMMENT /
// VERTICAL, quotes a non-NULL STRING with strconv.Quote only when its value is
// exactly NULL or starts with a double quote. SQL NULL keeps the existing
// NullString spelling, including nested ARRAY/STRUCT NULLs. Other strings,
// including <NULL>, stay unchanged. The input config is not mutated. Shared
// presets stay untouched. Excluded formats, including SQL-export fallback that
// still renders as a table, keep their existing bytes.
func applyShowNullsDisplay(fc *spanvalue.FormatConfig, showNulls bool, mode enums.DisplayMode) (*spanvalue.FormatConfig, error) {
	if fc == nil || !showNulls || !isShowNullsDisplayMode(mode) {
		return fc, nil
	}

	updated := fc.WithComplexPlugin(spanvalue.PluginFromNullable(quoteShowNullsString))
	if err := updated.Validate(); err != nil {
		return nil, err
	}
	return updated, nil
}

func quoteShowNullsString(v spanvalue.NullableValue) (string, error) {
	ns, ok := v.(spanner.NullString)
	if !ok || !ns.Valid || !needsShowNullsQuote(ns.StringVal) {
		return "", spanvalue.ErrFallthrough
	}
	return strconv.Quote(ns.StringVal), nil
}
