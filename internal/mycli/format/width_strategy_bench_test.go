package format

import (
	"fmt"
	"testing"

	"github.com/apstndb/spanner-mycli/enums"
)

func generateRows(numRows, numCols int) ([]string, []Row) {
	headers := make([]string, numCols)
	for i := range numCols {
		headers[i] = fmt.Sprintf("column_%d", i)
	}

	rows := make([]Row, numRows)
	for r := range numRows {
		cells := make([]string, numCols)
		for c := range numCols {
			switch c % 3 {
			case 0:
				cells[c] = fmt.Sprintf("%d", r*numCols+c)
			case 1:
				cells[c] = fmt.Sprintf("value_%d_%d", r, c)
			case 2:
				cells[c] = fmt.Sprintf("This is a longer text for row %d column %d that may wrap", r, c)
			}
		}
		rows[r] = StringsToRow(cells...)
	}
	return headers, rows
}

func BenchmarkWidthStrategies(b *testing.B) {
	sizes := []struct {
		name  string
		rows  int
		cols  int
		width int
	}{
		{"small_3x5", 5, 3, 80},
		{"medium_5x50", 50, 5, 120},
		{"large_10x200", 200, 10, 200},
	}

	for _, size := range sizes {
		headers, rows := generateRows(size.rows, size.cols)
		hints := make([]ColumnHint, size.cols)
		allRows := append([]Row{StringsToRow(headers...)}, rows...)

		for _, ws := range enums.WidthStrategyValues() {
			strategy := NewWidthStrategy(ws)
			name := fmt.Sprintf("%s/%s", size.name, ws.String())

			b.Run(name, func(b *testing.B) {
				wc := newTestWidthCalculator()
				overheadWidth := 4 + 3*(size.cols-1)
				availableWidth := size.width - overheadWidth

				b.ResetTimer()
				for range b.N {
					strategy.CalculateWidths(wc, availableWidth, headers, allRows, hints)
				}
			})
		}
	}
}
