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
	"fmt"
	"log/slog"
	"slices"

	"github.com/ngicks/go-iterator-helper/hiter"
	"github.com/samber/lo"
)

// GreedyFrequencyStrategy implements the original frequency-based greedy expansion
// algorithm. It starts with header-proportional widths, then greedily expands
// columns that would benefit the most cells (highest frequency count).
type GreedyFrequencyStrategy struct{}

func (GreedyFrequencyStrategy) CalculateWidths(wc *widthCalculator, availableWidth int,
	headers []string, rows []Row, _ []ColumnHint,
) []int {
	if len(headers) == 0 {
		return []int{}
	}

	sumWidths := func(ws []int) int {
		total := 0
		for _, w := range ws {
			total += w
		}
		return total
	}
	formatIntermediate := func(remainsWidth int, adjustedWidths []int) string {
		return fmt.Sprintf("remaining %v, adjustedWidths: %v", remainsWidth-sumWidths(adjustedWidths), adjustedWidths)
	}

	adjustedWidths := adjustByHeader(headers, availableWidth)

	// Enforce minimum column width for readability.
	for i := range adjustedWidths {
		adjustedWidths[i] = max(adjustedWidths[i], minColumnWidth)
	}

	slog.Debug("adjustByName", "info", formatIntermediate(availableWidth, adjustedWidths))

	transposedRows := make([][]string, len(headers))
	headerRow := StringsToRow(headers...)
	for columnIdx := range len(headers) {
		col := make([]string, 0, 1+len(rows))
		col = append(col, headerRow[columnIdx].RawText())
		for _, row := range rows {
			if columnIdx < len(row) {
				col = append(col, row[columnIdx].RawText())
			}
		}
		transposedRows[columnIdx] = col
	}

	widthCounts := wc.calculateWidthCounts(adjustedWidths, transposedRows)
	for {
		slog.Debug("widthCounts", "counts", widthCounts)

		firstCounts := hiter.Map(
			func(wcs []WidthCount) WidthCount {
				return lo.FirstOr(wcs, invalidWidthCount)
			},
			slices.Values(widthCounts))

		// find the largest count idx within available width
		idx, target := wc.maxIndex(availableWidth-sumWidths(adjustedWidths), adjustedWidths, firstCounts)
		if idx < 0 || target.Count() < 1 {
			break
		}

		widthCounts[idx] = widthCounts[idx][1:]
		adjustedWidths[idx] = target.Length()

		slog.Debug("adjusting", "info", formatIntermediate(availableWidth, adjustedWidths))
	}

	slog.Debug("semi final", "info", formatIntermediate(availableWidth, adjustedWidths))

	// Add rest to the longest shortage column.
	// Fall back to column 0 when all columns fit (no remaining widthCounts).
	longestWidths := make([]int, len(headers))
	for i, wcs := range widthCounts {
		for _, wc := range wcs {
			longestWidths[i] = max(longestWidths[i], wc.Length())
		}
	}

	bestIdx := 0
	bestShortage := longestWidths[0] - adjustedWidths[0]
	for i := 1; i < len(headers); i++ {
		shortage := longestWidths[i] - adjustedWidths[i]
		if shortage > bestShortage {
			bestShortage = shortage
			bestIdx = i
		}
	}
	adjustedWidths[bestIdx] += availableWidth - sumWidths(adjustedWidths)

	slog.Debug("final", "info", formatIntermediate(availableWidth, adjustedWidths))

	return adjustedWidths
}
