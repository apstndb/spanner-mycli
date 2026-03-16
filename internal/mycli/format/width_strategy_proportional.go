// Copyright 2025 apstndb
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
	"slices"

	"github.com/ngicks/go-iterator-helper/hiter"
	"github.com/samber/lo"
)

// ProportionalStrategy allocates column widths proportionally to each column's
// natural width (the maximum width across header and all rows).
// It starts with header-proportional + minColumnWidth floor, then distributes
// remaining space proportionally to each column's deficit.
type ProportionalStrategy struct{}

func (ProportionalStrategy) CalculateWidths(wc *widthCalculator, availableWidth int,
	headers []string, rows []Row, _ []ColumnHint,
) []int {
	numCols := len(headers)

	// Compute natural width per column (max across header + all rows).
	naturalWidths := make([]int, numCols)
	for i, h := range headers {
		naturalWidths[i] = wc.maxWidth(h)
	}
	for _, row := range rows {
		for i := range numCols {
			if i < len(row) {
				w := wc.maxWidth(row[i].RawText())
				naturalWidths[i] = max(naturalWidths[i], w)
			}
		}
	}

	// Start with header-proportional allocation + minColumnWidth floor.
	adjustedWidths := adjustByHeader(headers, availableWidth)
	for i := range adjustedWidths {
		adjustedWidths[i] = max(adjustedWidths[i], minColumnWidth)
	}

	// Compute deficit per column: how much more each column wants.
	deficits := make([]int, numCols)
	totalDeficit := 0
	for i := range numCols {
		d := naturalWidths[i] - adjustedWidths[i]
		if d > 0 {
			deficits[i] = d
			totalDeficit += d
		}
	}

	// Distribute remaining space proportionally to deficit.
	remaining := availableWidth - lo.Sum(adjustedWidths)
	if remaining > 0 && totalDeficit > 0 {
		distributed := 0
		for i := range numCols {
			if deficits[i] > 0 {
				share := remaining * deficits[i] / totalDeficit
				// Don't exceed the natural width.
				share = min(share, deficits[i])
				adjustedWidths[i] += share
				distributed += share
			}
		}

		// Assign leftover to column with largest remaining deficit.
		leftover := remaining - distributed
		if leftover > 0 {
			remainingDeficits := make([]int, numCols)
			for i := range numCols {
				remainingDeficits[i] = naturalWidths[i] - adjustedWidths[i]
			}
			idx, _ := MaxWithIdx(0, slices.Values(remainingDeficits))
			if idx >= 0 {
				adjustedWidths[idx] += leftover
			}
		}
	}

	return adjustedWidths
}

// wrapLinesForWidth counts how many visual lines a string occupies at the given column width.
// Used by MarginalCostStrategy. Returns at least 1.
func wrapLinesForWidth(wc *widthCalculator, s string, colWidth int) int {
	if colWidth <= 0 {
		return 1
	}
	lines := slices.Collect(hiter.Map(
		func(line string) int {
			w := wc.StringWidth(line)
			if w <= colWidth {
				return 1
			}
			return (w + colWidth - 1) / colWidth
		},
		splitLines(s),
	))
	return max(lo.Sum(lines), 1)
}

// splitLines splits s on newlines, returning an iterator.
func splitLines(s string) func(func(string) bool) {
	return func(yield func(string) bool) {
		for {
			i := indexOf(s, '\n')
			if i < 0 {
				yield(s)
				return
			}
			if !yield(s[:i]) {
				return
			}
			s = s[i+1:]
		}
	}
}

func indexOf(s string, b byte) int {
	for i := range len(s) {
		if s[i] == b {
			return i
		}
	}
	return -1
}
