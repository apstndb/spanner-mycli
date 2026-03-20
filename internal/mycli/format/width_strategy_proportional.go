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

	"github.com/samber/lo"
)

// ProportionalStrategy allocates column widths proportionally to each column's
// natural width (the maximum width across header and all rows).
// It starts with header-proportional + minColumnWidth floor, then distributes
// remaining space proportionally to each column's deficit.
type ProportionalStrategy struct{}

func (ProportionalStrategy) CalculateWidths(wc *widthCalculator, availableWidth int,
	headers []string, rows []Row, hints []ColumnHint,
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

	// Start with header-proportional allocation + preferred/min width floor.
	adjustedWidths := adjustByHeader(headers, availableWidth)
	applyColumnFloors(adjustedWidths, hints, availableWidth)

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
	if remaining > 0 {
		distributed := 0
		if totalDeficit > 0 {
			for i := range numCols {
				if deficits[i] > 0 {
					share := remaining * deficits[i] / totalDeficit
					// Don't exceed the natural width.
					share = min(share, deficits[i])
					adjustedWidths[i] += share
					distributed += share
				}
			}
		}

		// Assign leftover (from integer division rounding, or all of remaining
		// if totalDeficit was 0) to the column with the largest remaining
		// deficit. Fall back to column 0 to ensure the full available width
		// is always used.
		leftover := remaining - distributed
		if leftover > 0 && numCols > 0 {
			remainingDeficits := make([]int, numCols)
			for i := range numCols {
				remainingDeficits[i] = naturalWidths[i] - adjustedWidths[i]
			}
			idx, _ := MaxWithIdx(0, slices.Values(remainingDeficits))
			if idx < 0 {
				idx = 0
			}
			adjustedWidths[idx] += leftover
		}
	}

	return adjustedWidths
}
