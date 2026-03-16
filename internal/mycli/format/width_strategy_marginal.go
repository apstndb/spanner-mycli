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
	"container/heap"

	"github.com/samber/lo"
)

// MarginalCostStrategy minimizes total wrap-lines across all cells using a
// greedy max-heap approach. At each step, it allocates one extra character
// to the column that yields the largest reduction in total wrap-lines.
//
// Complexity: O(availableWidth * log(numCols)).
type MarginalCostStrategy struct{}

func (MarginalCostStrategy) CalculateWidths(wc *widthCalculator, availableWidth int,
	headers []string, rows []Row, _ []ColumnHint,
) []int {
	numCols := len(headers)

	// Collect all cell texts per column (header + rows).
	columnTexts := make([][]string, numCols)
	for i, h := range headers {
		columnTexts[i] = append(columnTexts[i], h)
	}
	for _, row := range rows {
		for i := range numCols {
			if i < len(row) {
				columnTexts[i] = append(columnTexts[i], row[i].RawText())
			}
		}
	}

	// Start with header-proportional allocation + minColumnWidth floor.
	adjustedWidths := adjustByHeader(headers, availableWidth)
	for i := range adjustedWidths {
		adjustedWidths[i] = max(adjustedWidths[i], minColumnWidth)
	}

	remaining := availableWidth - lo.Sum(adjustedWidths)
	if remaining <= 0 {
		return adjustedWidths
	}

	// Compute current total wrap-lines per column.
	currentWrapLines := make([]int, numCols)
	for i := range numCols {
		currentWrapLines[i] = totalWrapLines(wc, columnTexts[i], adjustedWidths[i])
	}

	// Build max-heap keyed on marginal benefit.
	h := make(marginalHeap, 0, numCols)
	for i := range numCols {
		benefit := currentWrapLines[i] - totalWrapLines(wc, columnTexts[i], adjustedWidths[i]+1)
		if benefit > 0 {
			h = append(h, marginalEntry{col: i, benefit: benefit})
		}
	}
	heap.Init(&h)

	// Greedily allocate one character at a time to the highest-benefit column.
	for remaining > 0 && h.Len() > 0 {
		top := heap.Pop(&h).(marginalEntry)
		if top.benefit <= 0 {
			break
		}

		adjustedWidths[top.col]++
		remaining--
		currentWrapLines[top.col] -= top.benefit

		// Recompute benefit for the next increment.
		newBenefit := currentWrapLines[top.col] - totalWrapLines(wc, columnTexts[top.col], adjustedWidths[top.col]+1)
		if newBenefit > 0 {
			top.benefit = newBenefit
			heap.Push(&h, top)
		}
	}

	// Distribute any leftover to the column with most remaining wrap-lines.
	if remaining > 0 {
		maxIdx := 0
		maxWrap := currentWrapLines[0]
		for i := 1; i < numCols; i++ {
			if currentWrapLines[i] > maxWrap {
				maxWrap = currentWrapLines[i]
				maxIdx = i
			}
		}
		adjustedWidths[maxIdx] += remaining
	}

	return adjustedWidths
}

// totalWrapLines computes the sum of wrap-lines for all texts at the given width.
func totalWrapLines(wc *widthCalculator, texts []string, colWidth int) int {
	total := 0
	for _, t := range texts {
		total += wrapLinesForWidth(wc, t, colWidth)
	}
	return total
}

// marginalEntry holds a column index and its current marginal benefit.
type marginalEntry struct {
	col     int
	benefit int
}

// marginalHeap implements heap.Interface for max-heap by benefit.
type marginalHeap []marginalEntry

func (h marginalHeap) Len() int           { return len(h) }
func (h marginalHeap) Less(i, j int) bool { return h[i].benefit > h[j].benefit } // max-heap
func (h marginalHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *marginalHeap) Push(x any)        { *h = append(*h, x.(marginalEntry)) }
func (h *marginalHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}
