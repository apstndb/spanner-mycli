package format

import (
	"cmp"
	"iter"
	"log/slog"
	"math"
	"slices"
	"strings"

	"github.com/apstndb/go-tabwrap"
	"github.com/apstndb/lox"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/ngicks/go-iterator-helper/hiter"
	"github.com/ngicks/go-iterator-helper/hiter/stringsiter"
	"github.com/samber/lo"
)

// minColumnWidth is the hard minimum width for any column.
// The meaningful minimum is now data-driven via NoWrapCell and PreferredMinWidth.
const minColumnWidth = 1

// CalculateWidth calculates optimal column widths for table rendering using the default
// GreedyFrequencyStrategy. columnNames are the plain column names, verboseHeaders are
// optionally the verbose header strings (with type info, may contain newlines).
func CalculateWidth(columnNames []string, verboseHeaders []string, wc *widthCalculator, screenWidth int, rows []Row) []int {
	return CalculateWidthWithStrategy(enums.WidthStrategyGreedyFrequency, columnNames, verboseHeaders, wc, screenWidth, rows)
}

// CalculateWidthWithStrategy calculates optimal column widths using the specified strategy.
// verboseHeaders are passed as the headers parameter so that strategies count them exactly once.
func CalculateWidthWithStrategy(ws enums.WidthStrategy, columnNames []string, verboseHeaders []string, wc *widthCalculator, screenWidth int, rows []Row) []int {
	// table overhead is:
	// len(`|  |`) +
	// len(` | `) * len(columns) - 1
	overheadWidth := 4 + 3*(len(columnNames)-1)
	availableWidth := screenWidth - overheadWidth

	slog.Debug("screen width info", "screenWidth", screenWidth, "availableWidth", availableWidth)

	hints := deriveColumnHints(wc, len(columnNames), rows)
	strategy := NewWidthStrategy(ws)
	return strategy.CalculateWidths(wc, availableWidth, verboseHeaders, rows, hints)
}

// deriveColumnHints scans rows and computes PreferredMinWidth per column
// from NoWrapCell instances. The preferred min width is the maximum width
// among all NoWrapCell values in that column.
func deriveColumnHints(wc *widthCalculator, numCols int, rows []Row) []ColumnHint {
	hints := make([]ColumnHint, numCols)
	for _, row := range rows {
		for i, cell := range row {
			if i >= numCols {
				break
			}
			if IsNoWrap(cell) {
				w := wc.maxWidth(cell.RawText())
				hints[i].PreferredMinWidth = max(hints[i].PreferredMinWidth, w)
			}
		}
	}
	return hints
}

// applyColumnFloors applies per-column minimum widths using hints.
// When budget allows, it uses PreferredMinWidth from hints (soft constraint).
// When the total would exceed availableWidth, it falls back to minColumnWidth only.
func applyColumnFloors(widths []int, hints []ColumnHint, availableWidth int) {
	origWidths := slices.Clone(widths)

	for i := range widths {
		preferred := minColumnWidth
		if i < len(hints) {
			preferred = max(preferred, hints[i].PreferredMinWidth)
		}
		widths[i] = max(widths[i], preferred)
	}

	// Soft degradation: if preferred mins exceed budget, fall back to hard minimum only.
	if lo.Sum(widths) > availableWidth {
		copy(widths, origWidths)
		for i := range widths {
			widths[i] = max(widths[i], minColumnWidth)
		}
	}
}

// MaxWithIdx returns the index and value of the maximum element in seq.
func MaxWithIdx[E cmp.Ordered](fallback E, seq iter.Seq[E]) (int, E) {
	return MaxByWithIdx(fallback, lox.Identity, seq)
}

// MaxByWithIdx returns the index and value of the element with the maximum key.
func MaxByWithIdx[O cmp.Ordered, E any](fallback E, f func(E) O, seq iter.Seq[E]) (int, E) {
	val := fallback
	idx := -1
	current := -1
	for v := range seq {
		current++
		if f(val) < f(v) {
			val = v
			idx = current
		}
	}
	return idx, val
}

type widthCalculator struct{ Condition *tabwrap.Condition }

func (wc *widthCalculator) StringWidth(s string) int {
	return wc.Condition.StringWidth(s)
}

func (wc *widthCalculator) maxWidth(s string) int {
	return hiter.Max(hiter.Map(
		wc.StringWidth,
		stringsiter.SplitFunc(s, 0, stringsiter.CutNewLine)))
}

func asc[T cmp.Ordered](left, right T) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func desc[T cmp.Ordered](left, right T) int {
	return asc(right, left)
}

func adjustToSum(limit int, vs []int) ([]int, int) {
	sumVs := lo.Sum(vs)
	remains := limit - sumVs
	if remains >= 0 {
		return vs, remains
	}

	// Build sorted unique thresholds (descending) once.
	rev := slices.SortedFunc(slices.Values(lo.Uniq(vs)), desc)

	curVs := vs
	for i := 1; i < len(rev); i++ {
		threshold := rev[i]
		clipped := make([]int, len(vs))
		total := 0
		for j, v := range vs {
			clipped[j] = min(v, threshold)
			total += clipped[j]
		}
		curVs = clipped
		if total <= limit {
			break
		}
	}

	total := 0
	for _, v := range curVs {
		total += v
	}
	return curVs, limit - total
}

var invalidWidthCount = WidthCount{
	// impossible to fit any width
	width: math.MaxInt,
	// least significant
	count: math.MinInt,
}

func (wc *widthCalculator) maxIndex(ignoreMax int, adjustWidths []int, seq iter.Seq[WidthCount]) (int, WidthCount) {
	return MaxByWithIdx(
		invalidWidthCount,
		WidthCount.Count,
		hiter.Unify(
			func(adjustWidth int, wc WidthCount) WidthCount {
				return lo.Ternary(wc.Length()-adjustWidth <= ignoreMax, wc, invalidWidthCount)
			},
			hiter.Pairs(slices.Values(adjustWidths), seq)))
}

func (wc *widthCalculator) countWidth(ss []string) iter.Seq[WidthCount] {
	return hiter.Map(
		func(e lo.Entry[int, int]) WidthCount {
			return WidthCount{
				width: e.Key,
				count: e.Value,
			}
		},
		slices.Values(lox.EntriesSortedByKey(lo.CountValuesBy(ss, wc.maxWidth))))
}

func (wc *widthCalculator) calculateWidthCounts(currentWidths []int, rows [][]string) [][]WidthCount {
	var result [][]WidthCount
	for columnNo := range len(currentWidths) {
		currentWidth := currentWidths[columnNo]
		columnValues := rows[columnNo]
		largerWidthCounts := slices.Collect(
			hiter.Filter(
				func(v WidthCount) bool {
					return v.Length() > currentWidth
				},
				wc.countWidth(columnValues),
			))
		result = append(result, largerWidthCounts)
	}
	return result
}

// WidthCount tracks the width and frequency of column values.
type WidthCount struct{ width, count int }

// Length returns the width value.
func (wc WidthCount) Length() int { return wc.width }

// Count returns the frequency count.
func (wc WidthCount) Count() int { return wc.count }

func adjustByHeader(headers []string, availableWidth int) []int {
	nameWidths := slices.Collect(hiter.Map(tabwrap.StringWidth, slices.Values(headers)))

	adjustWidths, _ := adjustToSum(availableWidth, nameWidths)

	return adjustWidths
}

// wrapLinesForWidth counts how many visual lines a string occupies at the given column width.
// Used by MarginalCostStrategy and tests. Returns at least 1.
func wrapLinesForWidth(wc *widthCalculator, s string, colWidth int) int {
	if colWidth <= 0 {
		return 1
	}
	total := 0
	for line := range splitLines(s) {
		w := wc.StringWidth(line)
		if w <= colWidth {
			total++
		} else {
			total += (w + colWidth - 1) / colWidth
		}
	}
	return max(total, 1)
}

// splitLines splits s on newlines, returning an iterator.
func splitLines(s string) iter.Seq[string] {
	return func(yield func(string) bool) {
		for {
			i := strings.IndexByte(s, '\n')
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
