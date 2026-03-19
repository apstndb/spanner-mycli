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

// tabVisualizedCell is a Cell that has had tab characters replaced with a visible
// arrow (→) plus padding spaces. It stores the styled version (with dim arrow
// markers) and lazily derives the plain version (for width calculation) on first
// access via stripANSI.
type tabVisualizedCell struct {
	styled    string // e.g. "abc\033[2m→\033[22m   def" — dim arrow
	cellStyle string // original cell's ANSI style (e.g. "\033[32m")
	plain     string // cached; lazily derived from stripANSI(styled)
}

func (c *tabVisualizedCell) RawText() string {
	if c.plain == "" {
		c.plain = stripANSI(c.styled)
	}
	return c.plain
}

func (c *tabVisualizedCell) Format() string {
	if c.cellStyle == "" {
		return c.styled
	}
	return c.cellStyle + c.styled + ansiReset
}

func (c *tabVisualizedCell) WithText(s string) Cell {
	// Re-apply dim styling to any → characters in the wrapped segment.
	return &tabVisualizedCell{
		styled:    strings.ReplaceAll(s, "→", "\033[2m→\033[22m"),
		cellStyle: c.cellStyle,
	}
}

// visualizeTab replaces tab characters in text with a visible arrow (→) plus
// padding spaces, matching the same column positions as real tab expansion.
// When styled is true, the arrow is wrapped with dim ANSI codes; otherwise
// a plain arrow is used. cond is used to compute grapheme display widths for
// accurate column tracking.
func visualizeTab(text string, cond *tabwrap.Condition, styled bool) string {
	if styled {
		return cond.ExpandTabFunc(text, func(nSpaces int) string {
			return "\033[2m→\033[22m" + strings.Repeat(" ", nSpaces-1)
		})
	}
	return cond.ExpandTabFunc(text, func(nSpaces int) string {
		return "→" + strings.Repeat(" ", nSpaces-1)
	})
}

// stripANSI removes ANSI SGR escape sequences from a string.
func stripANSI(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\033' && i+1 < len(s) && s[i+1] == '[' {
			// Skip until 'm' (end of SGR sequence).
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			if j < len(s) {
				i = j + 1
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// visualizeTabsInRow replaces tab characters in each cell of a row with visible
// arrow markers. Cells without tabs pass through unchanged. When styled is true
// and the cell is a StyledCell, a tabVisualizedCell is created that carries the
// styled representation and lazily derives plain. When styled is false, the plain
// text is injected via WithText to preserve the original Cell type.
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
		expanded := visualizeTab(raw, cond, styled)
		if styled {
			var cellStyle string
			if sc, ok := cell.(StyledCell); ok {
				cellStyle = sc.Style
			}
			result[i] = &tabVisualizedCell{
				styled:    expanded,
				cellStyle: cellStyle,
			}
		} else {
			result[i] = cell.WithText(expanded)
		}
	}
	if !changed {
		return row
	}
	return result
}
