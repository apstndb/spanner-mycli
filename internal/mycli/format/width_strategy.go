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

import "github.com/apstndb/spanner-mycli/enums"

// ColumnHint carries per-column metadata that strategies can use for allocation.
type ColumnHint struct {
	NoWrap bool // Reserved for #567: if true, the column should not be wrapped.
}

// WidthStrategy defines the interface for pluggable column width algorithms.
// Strategies receive availableWidth already overhead-subtracted (caller handles
// table border overhead). The returned slice must have len(headers) elements,
// each >= minColumnWidth.
type WidthStrategy interface {
	CalculateWidths(wc *widthCalculator, availableWidth int,
		headers []string, rows []Row, hints []ColumnHint) []int
}

// NewWidthStrategy returns a WidthStrategy for the given enum value.
func NewWidthStrategy(ws enums.WidthStrategy) WidthStrategy {
	switch ws {
	case enums.WidthStrategyProportional:
		return ProportionalStrategy{}
	case enums.WidthStrategyMarginalCost:
		return MarginalCostStrategy{}
	default:
		return GreedyFrequencyStrategy{}
	}
}
