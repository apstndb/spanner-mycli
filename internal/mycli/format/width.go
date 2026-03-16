package format

import (
	"cmp"
	"iter"
	"log/slog"
	"math"
	"slices"

	"github.com/apstndb/go-tabwrap"
	"github.com/apstndb/lox"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/ngicks/go-iterator-helper/hiter"
	"github.com/ngicks/go-iterator-helper/hiter/stringsiter"
	"github.com/samber/lo"
)

// minColumnWidth is the minimum width for any column.
// Prevents very short columns from splitting common short values (NULL, true, false, etc.).
const minColumnWidth = 4

// CalculateWidth calculates optimal column widths for table rendering using the default
// GreedyFrequencyStrategy. columnNames are the plain column names, verboseHeaders are
// optionally the verbose header strings (with type info, may contain newlines).
func CalculateWidth(columnNames []string, verboseHeaders []string, wc *widthCalculator, screenWidth int, rows []Row) []int {
	return CalculateWidthWithStrategy(enums.WidthStrategyGreedyFrequency, columnNames, verboseHeaders, wc, screenWidth, rows)
}

// CalculateWidthWithStrategy calculates optimal column widths using the specified strategy.
func CalculateWidthWithStrategy(ws enums.WidthStrategy, columnNames []string, verboseHeaders []string, wc *widthCalculator, screenWidth int, rows []Row) []int {
	allRows := slices.Concat([]Row{StringsToRow(verboseHeaders...)}, rows)

	// table overhead is:
	// len(`|  |`) +
	// len(` | `) * len(columns) - 1
	overheadWidth := 4 + 3*(len(columnNames)-1)
	availableWidth := screenWidth - overheadWidth

	slog.Debug("screen width info", "screenWidth", screenWidth, "availableWidth", availableWidth)

	hints := make([]ColumnHint, len(columnNames))
	strategy := NewWidthStrategy(ws)
	return strategy.CalculateWidths(wc, availableWidth, columnNames, allRows, hints)
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

func clipToMax[S interface{ ~[]E }, E cmp.Ordered](s S, maxValue E) iter.Seq[E] {
	return hiter.Map(
		func(in E) E {
			return min(in, maxValue)
		},
		slices.Values(s),
	)
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

	curVs := vs
	for i := 1; ; i++ {
		rev := slices.SortedFunc(slices.Values(lo.Uniq(vs)), desc)
		v, ok := hiter.Nth(i, slices.Values(rev))
		if !ok {
			break
		}
		curVs = slices.Collect(clipToMax(vs, v))
		if lo.Sum(curVs) <= limit {
			break
		}
	}
	return curVs, limit - lo.Sum(curVs)
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
