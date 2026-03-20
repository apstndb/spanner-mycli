package format

import (
	"testing"

	"github.com/apstndb/spanner-mycli/enums"
)

func TestGreedyFrequencyStrategy(t *testing.T) {
	t.Parallel()

	wc := newTestWidthCalculator()
	strategy := GreedyFrequencyStrategy{}

	headers := []string{"id", "name", "description"}
	rows := []Row{
		StringsToRow("1", "Alice", "Software Engineer"),
		StringsToRow("2", "Bob", "Data Scientist with a longer title"),
	}

	widths := strategy.CalculateWidths(wc, 60, headers, rows, make([]ColumnHint, 3))
	if len(widths) != 3 {
		t.Fatalf("len(widths) = %d, want 3", len(widths))
	}
	for i, w := range widths {
		if w < minColumnWidth {
			t.Errorf("widths[%d] = %d, want >= %d", i, w, minColumnWidth)
		}
	}
}

func TestProportionalStrategy(t *testing.T) {
	t.Parallel()

	wc := newTestWidthCalculator()
	strategy := ProportionalStrategy{}

	headers := []string{"id", "name", "description"}
	rows := []Row{
		StringsToRow("1", "Alice", "Software Engineer"),
		StringsToRow("2", "Bob", "Data Scientist with a longer title"),
	}

	widths := strategy.CalculateWidths(wc, 60, headers, rows, make([]ColumnHint, 3))
	if len(widths) != 3 {
		t.Fatalf("len(widths) = %d, want 3", len(widths))
	}
	for i, w := range widths {
		if w < minColumnWidth {
			t.Errorf("widths[%d] = %d, want >= %d", i, w, minColumnWidth)
		}
	}
}

func TestMarginalCostStrategy(t *testing.T) {
	t.Parallel()

	wc := newTestWidthCalculator()
	strategy := MarginalCostStrategy{}

	headers := []string{"id", "name", "description"}
	rows := []Row{
		StringsToRow("1", "Alice", "Software Engineer"),
		StringsToRow("2", "Bob", "Data Scientist with a longer title"),
	}

	widths := strategy.CalculateWidths(wc, 60, headers, rows, make([]ColumnHint, 3))
	if len(widths) != 3 {
		t.Fatalf("len(widths) = %d, want 3", len(widths))
	}
	for i, w := range widths {
		if w < minColumnWidth {
			t.Errorf("widths[%d] = %d, want >= %d", i, w, minColumnWidth)
		}
	}
}

func TestNewWidthStrategy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		strategy enums.WidthStrategy
		wantType string
	}{
		{"default", enums.WidthStrategyGreedyFrequency, "GreedyFrequencyStrategy"},
		{"proportional", enums.WidthStrategyProportional, "ProportionalStrategy"},
		{"marginal_cost", enums.WidthStrategyMarginalCost, "MarginalCostStrategy"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := NewWidthStrategy(tt.strategy)
			if s == nil {
				t.Fatal("NewWidthStrategy returned nil")
			}
		})
	}
}

func TestCalculateWidthWithStrategy(t *testing.T) {
	t.Parallel()

	columns := []string{"id", "name"}
	rows := []Row{StringsToRow("1", "Alice"), StringsToRow("2", "Bob")}

	for _, ws := range enums.WidthStrategyValues() {
		t.Run(ws.String(), func(t *testing.T) {
			t.Parallel()
			wc := newTestWidthCalculator()
			widths := CalculateWidthWithStrategy(ws, columns, columns, wc, 80, rows)
			if len(widths) != len(columns) {
				t.Errorf("len(widths) = %d, want %d", len(widths), len(columns))
			}
			for i, w := range widths {
				if w < minColumnWidth {
					t.Errorf("width[%d] = %d, expected >= %d (minColumnWidth)", i, w, minColumnWidth)
				}
			}
		})
	}
}

// TestCompareStrategies logs width arrays for all three strategies
// to allow manual comparison. Not asserting equality — strategies
// intentionally differ in allocation.
func TestCompareStrategies(t *testing.T) {
	t.Parallel()

	wc := newTestWidthCalculator()
	headers := []string{"id", "name", "email", "bio"}
	rows := []Row{
		StringsToRow("1", "Alice Johnson", "alice@example.com", "Software engineer with 10 years of experience"),
		StringsToRow("2", "Bob", "bob@test.org", "Student"),
		StringsToRow("123", "Charlie Brown", "charlie.b@longdomain.example.com", "A very detailed biography that spans multiple words"),
		StringsToRow("4", "Diana", "d@x.co", "NULL"),
	}
	hints := make([]ColumnHint, len(headers))

	for _, ws := range enums.WidthStrategyValues() {
		s := NewWidthStrategy(ws)
		widths := s.CalculateWidths(wc, 80, headers, rows, hints)

		// Count total wrap-lines for comparison.
		totalWraps := 0
		for i, h := range headers {
			totalWraps += wrapLinesForWidth(wc, h, widths[i])
		}
		for _, row := range rows {
			for i, cell := range row {
				totalWraps += wrapLinesForWidth(wc, cell.RawText(), widths[i])
			}
		}

		t.Logf("%-20s widths=%v totalWraps=%d", ws.String(), widths, totalWraps)
	}
}

func TestWrapLinesForWidth(t *testing.T) {
	t.Parallel()

	wc := newTestWidthCalculator()

	tests := []struct {
		name    string
		text    string
		width   int
		wantMin int
	}{
		{"fits", "hello", 10, 1},
		{"exact", "hello", 5, 1},
		{"wraps", "hello world", 5, 2},
		{"multiline", "hello\nworld", 10, 2},
		{"zero width", "hello", 0, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := wrapLinesForWidth(wc, tt.text, tt.width)
			if got < tt.wantMin {
				t.Errorf("wrapLinesForWidth(%q, %d) = %d, want >= %d", tt.text, tt.width, got, tt.wantMin)
			}
		})
	}
}

// TestGreedyFrequencyStrategy_ShortRows ensures GreedyFrequencyStrategy
// handles rows shorter than headers without panicking.
func TestGreedyFrequencyStrategy_ShortRows(t *testing.T) {
	t.Parallel()

	wc := newTestWidthCalculator()
	strategy := GreedyFrequencyStrategy{}

	headers := []string{"id", "name", "description"}
	rows := []Row{
		StringsToRow("1", "Alice"), // only 2 columns, header has 3
		StringsToRow("2"),          // only 1 column
	}

	widths := strategy.CalculateWidths(wc, 60, headers, rows, make([]ColumnHint, 3))
	if len(widths) != 3 {
		t.Fatalf("len(widths) = %d, want 3", len(widths))
	}
	for i, w := range widths {
		if w < minColumnWidth {
			t.Errorf("widths[%d] = %d, want >= %d", i, w, minColumnWidth)
		}
	}
}

// TestProportionalStrategy_ZeroDeficit ensures ProportionalStrategy uses the
// full available width even when all columns already fit (totalDeficit == 0).
func TestProportionalStrategy_ZeroDeficit(t *testing.T) {
	t.Parallel()

	wc := newTestWidthCalculator()
	strategy := ProportionalStrategy{}

	// Very short data with large available width — all columns fit easily.
	headers := []string{"a", "b"}
	rows := []Row{StringsToRow("1", "2")}

	availableWidth := 80
	widths := strategy.CalculateWidths(wc, availableWidth, headers, rows, make([]ColumnHint, 2))

	totalWidth := 0
	for _, w := range widths {
		totalWidth += w
	}
	if totalWidth != availableWidth {
		t.Errorf("total width = %d, want %d (full available width)", totalWidth, availableWidth)
	}
}

// TestGreedyFrequencyStrategy_FullWidth ensures GreedyFrequencyStrategy uses
// the full available width even when all columns fit.
func TestGreedyFrequencyStrategy_FullWidth(t *testing.T) {
	t.Parallel()

	wc := newTestWidthCalculator()
	strategy := GreedyFrequencyStrategy{}

	headers := []string{"a", "b"}
	rows := []Row{StringsToRow("1", "2")}

	availableWidth := 80
	widths := strategy.CalculateWidths(wc, availableWidth, headers, rows, make([]ColumnHint, 2))

	totalWidth := 0
	for _, w := range widths {
		totalWidth += w
	}
	if totalWidth != availableWidth {
		t.Errorf("total width = %d, want %d (full available width)", totalWidth, availableWidth)
	}
}

// TestStrategyMinColumnWidth ensures all strategies respect minColumnWidth.
func TestStrategyMinColumnWidth(t *testing.T) {
	t.Parallel()

	wc := newTestWidthCalculator()
	headers := []string{"a", "b"} // very short headers
	rows := []Row{StringsToRow("1", "2")}
	hints := make([]ColumnHint, 2)

	for _, ws := range enums.WidthStrategyValues() {
		t.Run(ws.String(), func(t *testing.T) {
			t.Parallel()
			s := NewWidthStrategy(ws)
			widths := s.CalculateWidths(wc, 40, headers, rows, hints)
			for i, w := range widths {
				if w < minColumnWidth {
					t.Errorf("widths[%d] = %d, want >= %d", i, w, minColumnWidth)
				}
			}
		})
	}
}

// TestStrategyRespectsPreferredMinWidth ensures all strategies respect PreferredMinWidth
// from hints when space allows.
func TestStrategyRespectsPreferredMinWidth(t *testing.T) {
	t.Parallel()

	wc := newTestWidthCalculator()
	headers := []string{"a", "b"}
	rows := []Row{
		{NoWrapCell{Cell: PlainCell{Text: "NULL"}}, PlainCell{Text: "x"}},
	}
	// hints[0] has PreferredMinWidth=4 (NULL), hints[1] has 0 (no NoWrap)
	hints := []ColumnHint{{PreferredMinWidth: 4}, {}}

	for _, ws := range enums.WidthStrategyValues() {
		t.Run(ws.String(), func(t *testing.T) {
			t.Parallel()
			s := NewWidthStrategy(ws)
			// Wide screen: enough space to satisfy preferred min
			widths := s.CalculateWidths(wc, 40, headers, rows, hints)
			if widths[0] < 4 {
				t.Errorf("widths[0] = %d, want >= 4 (PreferredMinWidth for NULL)", widths[0])
			}
		})
	}
}

// TestStrategyGracefulDegradation ensures strategies degrade gracefully
// when screen is too narrow to satisfy PreferredMinWidth for all columns.
func TestStrategyGracefulDegradation(t *testing.T) {
	t.Parallel()

	wc := newTestWidthCalculator()
	headers := []string{"a", "b", "c", "d", "e"}
	rows := []Row{StringsToRow("x", "y", "z", "w", "v")}
	// All columns want 5 chars (total=25), but we only give 10.
	hints := []ColumnHint{
		{PreferredMinWidth: 5},
		{PreferredMinWidth: 5},
		{PreferredMinWidth: 5},
		{PreferredMinWidth: 5},
		{PreferredMinWidth: 5},
	}

	for _, ws := range enums.WidthStrategyValues() {
		t.Run(ws.String(), func(t *testing.T) {
			t.Parallel()
			s := NewWidthStrategy(ws)
			widths := s.CalculateWidths(wc, 10, headers, rows, hints)
			for i, w := range widths {
				// Hard floor is minColumnWidth=1, should not panic or go below.
				if w < minColumnWidth {
					t.Errorf("widths[%d] = %d, want >= %d", i, w, minColumnWidth)
				}
			}
		})
	}
}
