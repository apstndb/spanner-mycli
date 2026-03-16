# Column Width Calculation Improvement: Proposed Issues

These issues were discussed in the context of improving table column width
calculation in `internal/mycli/format/width.go`. They should be created as
GitHub issues.

## Issue 1: Introduce `WidthStrategy` interface for pluggable column width algorithms

**Labels**: `enhancement`, `refactoring`

### Summary

Refactor the column width calculation in `internal/mycli/format/width.go` to use a
`WidthStrategy` interface, enabling multiple algorithms behind a common interface.

### Motivation

The current `calculateOptimalWidth()` is a single monolithic function combining several
heuristics (header-proportional allocation, frequency-based greedy expansion, remainder
assignment). Extracting it behind an interface enables:

- Comparing alternative algorithms (proportional, marginal cost minimization) with
  benchmarks
- Selecting strategies via system variable at runtime
- Clear separation between "what inputs the algorithm receives" and "how it allocates
  width"

### Design

```go
// WidthStrategy calculates column widths for table rendering.
type WidthStrategy interface {
    CalculateWidths(wc *widthCalculator, availableWidth int,
        headers []string, rows []Row, hints []ColumnHint) []int
}
```

### Scope (single PR)

- [ ] Define `WidthStrategy` interface and `ColumnHint` struct
- [ ] Extract current algorithm as `GreedyFrequencyStrategy`
- [ ] Wire through `TableStreamingFormatter` (strategy selection)
- [ ] Add `ProportionalStrategy` (natural width proportional allocation)
- [ ] Add `MarginalCostStrategy` (wrap-line minimization via marginal benefit heap)
- [ ] Benchmark suite comparing all three strategies
- [ ] `TestCompareStrategies` outputting width arrays and total wrap lines for review
- [ ] System variable `CLI_WIDTH_STRATEGY` to select at runtime

### Notes

- `CalculateWidth()` public function signature can remain as a convenience wrapper
- Overhead calculation (borders/separators) stays outside the strategy
- This is prerequisite for Issue 2 (NoWrapCell)

---

## Issue 2: Introduce `NoWrapCell` to generalize `minColumnWidth` hardcoding

**Labels**: `enhancement`

### Summary

Replace the hardcoded `minColumnWidth = 4` (for NULL marker) with a general mechanism:
`NoWrapCell` wrapper that marks cells whose content should preferably not be wrapped.

### Motivation

- `minColumnWidth = 4` is implicit knowledge that NULL is 4 characters
- Other values may also benefit from no-wrap: `<nil>` (5 chars), `true`/`false`, short
  numeric values
- The constraint should be soft (prefer no wrap, but allow it if screen is too narrow)
  rather than a hard minimum that can starve other columns
- ANSI sequence carry-over support means wrapping mid-NULL no longer breaks display,
  so a hard minimum is unnecessarily restrictive

### Design

```go
// NoWrapCell wraps any Cell to indicate its content should preferably not be wrapped.
type NoWrapCell struct {
    Cell
}

// ColumnHint holds per-column metadata derived from cell attributes.
type ColumnHint struct {
    // PreferredMinWidth: width needed to avoid wrapping all NoWrap cells.
    // Strategies should try to satisfy this but may go below if space is tight.
    PreferredMinWidth int
}
```

### Key behaviors

- `deriveColumnHints()` scans rows and computes `PreferredMinWidth` per column from
  `NoWrapCell` instances
- Strategies treat `PreferredMinWidth` as soft: satisfy if budget allows, degrade
  gracefully otherwise
- `minColumnWidth` constant can be reduced to 1 (or a small hard floor) since the
  meaningful minimum is now data-driven

### Scope (single PR)

- [ ] Add `NoWrapCell` type implementing `Cell` via composition
- [ ] Add `deriveColumnHints()` function
- [ ] Update `spannerRowToRow()` to wrap NULL cells as `NoWrapCell{StyledCell{...}}`
- [ ] Update all strategies to respect `ColumnHint.PreferredMinWidth` as soft constraint
- [ ] Remove or reduce `minColumnWidth` constant
- [ ] Tests: NoWrap cells respected when space allows, gracefully wrapped when not

### Depends on

- Issue 1 (WidthStrategy interface)

---

## Issue 3: Tab character visualization in cell values

**Labels**: `enhancement`

### Summary

Replace tab characters in cell display with a visible symbol (e.g., `→`, `⇥`, or `␉`)
styled in dim/gray, followed by padding spaces to the tab stop.

### Motivation

- Tab characters in column values are rare but when present cause display ambiguity
- Tab stops in the middle of a wrapped line have undefined visual behavior
- Replacing tabs with "symbol + spaces" makes wrapping at those positions well-defined
  and gives the user visual feedback that a tab was present

### Design

A preprocessing step in the cell rendering pipeline:

```
Raw value → tab visualization → width calculation → wrap → display
```

Example: `"abc\tdef"` with tabstop=8 → `"abc→    def"` (→ is dim-styled)

### Scope

- [ ] Implement tab visualization as a cell preprocessor
- [ ] Respect `FormatConfig.Styled` (plain mode: use simple character, styled: dim color)
- [ ] Consider system variable to toggle behavior
- [ ] After visualization, tabs become regular characters → wrap works normally

### Notes

- Independent of WidthStrategy refactoring
- Low priority: tabs in Spanner data are rare in practice

---

## Discussion Context

### Alternative approaches considered for NoWrap + tab handling

When `NoWrapCell` constraints cannot be satisfied, three fallback levels were discussed:

1. **Level 0**: Normal tab stops, respect NoWrap → default
2. **Level 1**: Expand tabs to spaces → allows wrapping at tab boundaries without
   changing visual width
3. **Level 2**: Recalculate with tabWidth=1 → reduces NoWrap cell required widths

Since tabs in Spanner data are rare, and tab visualization (Issue 3) would eliminate
the tab-specific wrapping problem entirely, the fallback chain was deemed unnecessary
as a separate feature. If needed, it can be incorporated into the NoWrapCell strategy
implementation.

### Algorithm comparison

| Algorithm | Approach | Strength |
|-----------|----------|----------|
| GreedyFrequency (current) | Expand column benefiting most cells | Frequency-aware |
| Proportional | Allocate proportional to natural width | Simple, predictable |
| MarginalCost | Minimize total wrap lines via heap | Optimal for wrap minimization |
