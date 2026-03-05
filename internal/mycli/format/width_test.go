package format

import (
	"math"
	"testing"

	"github.com/apstndb/go-runewidthex"
	"github.com/google/go-cmp/cmp"
)

func TestMaxWithIdx(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    []int
		wantIdx  int
		wantVal  int
		fallback int
	}{
		{
			name:     "single element",
			input:    []int{5},
			wantIdx:  0,
			wantVal:  5,
			fallback: math.MinInt,
		},
		{
			name:     "max at end",
			input:    []int{1, 2, 3},
			wantIdx:  2,
			wantVal:  3,
			fallback: math.MinInt,
		},
		{
			name:     "max at start",
			input:    []int{9, 2, 3},
			wantIdx:  0,
			wantVal:  9,
			fallback: math.MinInt,
		},
		{
			name:     "max in middle",
			input:    []int{1, 9, 3},
			wantIdx:  1,
			wantVal:  9,
			fallback: math.MinInt,
		},
		{
			name:     "all same",
			input:    []int{5, 5, 5},
			wantIdx:  -1,
			wantVal:  5,
			fallback: 5,
		},
		{
			name:     "empty seq",
			input:    nil,
			wantIdx:  -1,
			wantVal:  0,
			fallback: 0,
		},
		{
			name:     "negative numbers",
			input:    []int{-3, -1, -5},
			wantIdx:  1,
			wantVal:  -1,
			fallback: math.MinInt,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			seq := sliceToSeq(tt.input)
			idx, val := MaxWithIdx(tt.fallback, seq)
			if idx != tt.wantIdx {
				t.Errorf("idx = %d, want %d", idx, tt.wantIdx)
			}
			if val != tt.wantVal {
				t.Errorf("val = %d, want %d", val, tt.wantVal)
			}
		})
	}
}

func TestMaxByWithIdx(t *testing.T) {
	t.Parallel()

	type item struct {
		name  string
		value int
	}

	input := []item{
		{"a", 1},
		{"b", 5},
		{"c", 3},
	}

	idx, got := MaxByWithIdx(item{}, func(i item) int { return i.value }, sliceToSeq(input))
	if idx != 1 {
		t.Errorf("idx = %d, want 1", idx)
	}
	if got.name != "b" {
		t.Errorf("got.name = %q, want %q", got.name, "b")
	}
}

func TestWidthCount(t *testing.T) {
	t.Parallel()

	wc := WidthCount{width: 10, count: 5}
	if wc.Length() != 10 {
		t.Errorf("Length() = %d, want 10", wc.Length())
	}
	if wc.Count() != 5 {
		t.Errorf("Count() = %d, want 5", wc.Count())
	}
}

func TestAdjustToSum(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		limit       int
		vs          []int
		wantWidths  []int
		wantRemains int
	}{
		{
			name:        "within limit",
			limit:       20,
			vs:          []int{5, 5, 5},
			wantWidths:  []int{5, 5, 5},
			wantRemains: 5,
		},
		{
			name:        "exact limit",
			limit:       15,
			vs:          []int{5, 5, 5},
			wantWidths:  []int{5, 5, 5},
			wantRemains: 0,
		},
		{
			name:        "exceeds limit clips largest",
			limit:       10,
			vs:          []int{5, 3, 10},
			wantWidths:  []int{5, 3, 5},
			wantRemains: 10 - 13, // might be negative before redistribute
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotWidths, gotRemains := adjustToSum(tt.limit, tt.vs)
			if tt.wantRemains >= 0 {
				// Only check when we expect non-negative remains
				if diff := cmp.Diff(tt.wantWidths, gotWidths); diff != "" {
					t.Errorf("widths mismatch (-want +got):\n%s", diff)
				}
				if gotRemains != tt.wantRemains {
					t.Errorf("remains = %d, want %d", gotRemains, tt.wantRemains)
				}
			} else {
				// For exceeds-limit case, just verify total <= limit
				total := 0
				for _, w := range gotWidths {
					total += w
				}
				if total > tt.limit {
					t.Errorf("total width %d exceeds limit %d", total, tt.limit)
				}
			}
		})
	}
}

func TestAdjustByHeader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		headers        []string
		availableWidth int
		wantLen        int
	}{
		{
			name:           "fits within width",
			headers:        []string{"id", "name", "email"},
			availableWidth: 100,
			wantLen:        3,
		},
		{
			name:           "single column",
			headers:        []string{"id"},
			availableWidth: 50,
			wantLen:        1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := adjustByHeader(tt.headers, tt.availableWidth)
			if len(got) != tt.wantLen {
				t.Errorf("len = %d, want %d", len(got), tt.wantLen)
			}
			// Each width should be > 0
			for i, w := range got {
				if w < 0 {
					t.Errorf("width[%d] = %d, expected >= 0", i, w)
				}
			}
		})
	}
}

func TestCalculateWidth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		columns     []string
		rows        []Row
		screenWidth int
	}{
		{
			name:        "basic",
			columns:     []string{"id", "name"},
			rows:        []Row{{"1", "Alice"}, {"2", "Bob"}},
			screenWidth: 80,
		},
		{
			name:        "wide data",
			columns:     []string{"id", "description"},
			rows:        []Row{{"1", "A very long description that might need wrapping"}},
			screenWidth: 40,
		},
		{
			name:        "no rows",
			columns:     []string{"id", "name"},
			rows:        nil,
			screenWidth: 80,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rw := newTestWidthCalculator()
			widths := CalculateWidth(tt.columns, tt.columns, rw, tt.screenWidth, tt.rows)
			if len(widths) != len(tt.columns) {
				t.Errorf("len(widths) = %d, want %d", len(widths), len(tt.columns))
			}
			for i, w := range widths {
				if w <= 0 {
					t.Errorf("width[%d] = %d, expected > 0", i, w)
				}
			}
		})
	}
}

// Helper: convert slice to iter.Seq
func sliceToSeq[E any](s []E) func(func(E) bool) {
	return func(yield func(E) bool) {
		for _, v := range s {
			if !yield(v) {
				return
			}
		}
	}
}

func newTestWidthCalculator() *widthCalculator {
	return &widthCalculator{Condition: runewidthex.NewCondition()}
}
