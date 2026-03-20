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
	"math"
	"slices"

	"github.com/ngicks/go-iterator-helper/hiter"
	"github.com/samber/lo"
)

// GreedyFrequencyStrategy implements the original frequency-based greedy expansion
// algorithm. It starts with header-proportional widths, then greedily expands
// columns that would benefit the most cells (highest frequency count).
type GreedyFrequencyStrategy struct{}

func (GreedyFrequencyStrategy) CalculateWidths(wc *widthCalculator, availableWidth int,
	headers []string, rows []Row, hints []ColumnHint,
) []int {
	formatIntermediate := func(remainsWidth int, adjustedWidths []int) string {
		return fmt.Sprintf("remaining %v, adjustedWidths: %v", remainsWidth-lo.Sum(adjustedWidths), adjustedWidths)
	}

	adjustedWidths := adjustByHeader(headers, availableWidth)

	applyColumnFloors(adjustedWidths, hints, availableWidth)

	slog.Debug("adjustByName", "info", formatIntermediate(availableWidth, adjustedWidths))

	var transposedRows [][]string
	for columnIdx := range len(headers) {
		transposedRows = append(transposedRows, slices.Collect(
			hiter.Map(
				func(in Row) string {
					return lo.Must(lo.Nth(in, columnIdx)).RawText()
				},
				hiter.Concat(hiter.Once(StringsToRow(headers...)), slices.Values(rows)),
			)))
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
		idx, target := wc.maxIndex(availableWidth-lo.Sum(adjustedWidths), adjustedWidths, firstCounts)
		if idx < 0 || target.Count() < 1 {
			break
		}

		widthCounts[idx] = widthCounts[idx][1:]
		adjustedWidths[idx] = target.Length()

		slog.Debug("adjusting", "info", formatIntermediate(availableWidth, adjustedWidths))
	}

	slog.Debug("semi final", "info", formatIntermediate(availableWidth, adjustedWidths))

	// Add rest to the longest shortage column.
	// NOTE: When all columns fit within their allocated width (no remaining
	// widthCounts), idx will be -1 and the remainder is not distributed.
	// This matches the original calculateOptimalWidth behavior. Improving this
	// to always use the full available width is left for a future refactor.
	longestWidths := lo.Map(widthCounts, func(item []WidthCount, _ int) int {
		return hiter.Max(hiter.Map(WidthCount.Length, slices.Values(item)))
	})

	idx, _ := MaxWithIdx(math.MinInt, hiter.Unify(
		func(longestWidth, adjustedWidth int) int {
			return longestWidth - adjustedWidth
		},
		hiter.Pairs(slices.Values(longestWidths), slices.Values(adjustedWidths))))

	if idx != -1 {
		adjustedWidths[idx] += availableWidth - lo.Sum(adjustedWidths)
	}

	slog.Debug("final", "info", formatIntermediate(availableWidth, adjustedWidths))

	return adjustedWidths
}
