// Copyright 2025 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package format

import (
	"strings"

	"github.com/apstndb/go-tabwrap"
)

// visualizedCell is a Cell that has had tab characters replaced with a visible
// arrow (→) plus padding spaces. It maintains both a plain version (for width
// calculation) and a styled version (with dim arrow markers for display).
type visualizedCell struct {
	plain     string // e.g. "abc→   def" — no ANSI codes
	styled    string // e.g. "abc\033[2m→\033[22m   def" — dim arrow
	cellStyle string // original cell's ANSI style (e.g. "\033[32m")
}

func (c visualizedCell) RawText() string { return c.plain }

func (c visualizedCell) Format() string {
	if c.cellStyle == "" {
		return c.styled
	}
	return c.cellStyle + c.styled + ansiReset
}

func (c visualizedCell) WithText(s string) Cell { return PlainCell{Text: s} }

// visualizeTab replaces tab characters in text with a visible arrow (→) plus
// padding spaces, matching the same column positions as real tab expansion.
// cond is used to compute grapheme display widths for accurate column tracking.
func visualizeTab(text string, cond *tabwrap.Condition) (plain, styled string) {
	plain = cond.ExpandTabFunc(text, func(nSpaces int) string {
		return "→" + strings.Repeat(" ", nSpaces-1)
	})
	styled = cond.ExpandTabFunc(text, func(nSpaces int) string {
		return "\033[2m→\033[22m" + strings.Repeat(" ", nSpaces-1)
	})
	return plain, styled
}

// visualizeTabsInRow replaces tab characters in each cell of a row with visible
// arrow markers. Cells without tabs pass through unchanged. When styled is true
// and the cell is a StyledCell, a visualizedCell is created that carries both
// plain and styled representations. When styled is false, the plain text is
// injected via WithText to preserve the original Cell type.
func visualizeTabsInRow(row Row, cond *tabwrap.Condition, styled bool) Row {
	result := make(Row, len(row))
	changed := false
	for i, cell := range row {
		raw := cell.RawText()
		if !strings.Contains(raw, "\t") {
			result[i] = cell
			continue
		}
		changed = true
		plain, styledText := visualizeTab(raw, cond)
		if styled {
			if sc, ok := cell.(StyledCell); ok {
				result[i] = visualizedCell{
					plain:     plain,
					styled:    styledText,
					cellStyle: sc.Style,
				}
			} else {
				result[i] = visualizedCell{
					plain:  plain,
					styled: styledText,
				}
			}
		} else {
			result[i] = cell.WithText(plain)
		}
	}
	if !changed {
		return row
	}
	return result
}
