package format

import (
	"cmp"
	"fmt"
	"io"
	"slices"

	"github.com/apstndb/go-runewidthex"
	"github.com/ngicks/go-iterator-helper/hiter"
	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/renderer"
	"github.com/olekukonko/tablewriter/tw"
)

// TableStreamingFormatter provides streaming table output using tablewriter v1.0.9.
// It uses a configurable number of preview rows to calculate optimal column widths.
type TableStreamingFormatter struct {
	out          io.Writer
	config       FormatConfig
	screenWidth  int
	table        *tablewriter.Table
	columns      []string
	widths       []int
	initialized  bool
	previewRows  []Row
	previewSize  int
	rowsBuffered int
}

// NewTableStreamingFormatter creates a new table streaming formatter.
// previewSize determines how many rows to use for width calculation (0 = all rows).
func NewTableStreamingFormatter(out io.Writer, config FormatConfig, screenWidth int, previewSize int) *TableStreamingFormatter {
	return &TableStreamingFormatter{
		out:         out,
		config:      config,
		screenWidth: screenWidth,
		previewSize: previewSize,
		previewRows: []Row{},
	}
}

// InitFormat initializes the table with preview rows for width calculation.
func (f *TableStreamingFormatter) InitFormat(columnNames []string, config FormatConfig, previewRows []Row) error {
	if f.initialized {
		return nil
	}

	f.columns = columnNames
	f.config = config
	f.previewRows = previewRows

	// Calculate optimal widths using preview rows
	f.calculateWidths(columnNames, previewRows)

	// Create table with streaming configuration
	f.table = tablewriter.NewTable(f.out,
		tablewriter.WithRenderer(
			renderer.NewBlueprint(tw.Rendition{Symbols: tw.NewSymbols(tw.StyleASCII)})),
		tablewriter.WithHeaderAlignment(tw.AlignLeft),
		tablewriter.WithTrimSpace(tw.Off),
		tablewriter.WithHeaderAutoFormat(tw.Off),
		tablewriter.WithStreaming(tw.StreamConfig{
			Enable: true,
		}),
	).Configure(func(twConfig *tablewriter.Config) {
		// Note: Column alignment is not set here because:
		// 1. Regular SQL queries don't specify column alignment (defaults to left)
		// 2. EXPLAIN/DESCRIBE statements that use custom alignment don't use streaming mode
		// They return a complete Result with ColumnAlign set and use buffered formatting
		twConfig.Row.Formatting.AutoWrap = tw.WrapNone
	})

	// Start streaming table
	if err := f.table.Start(); err != nil {
		return fmt.Errorf("failed to start table streaming: %w", err)
	}

	// Set headers
	if !f.config.SkipColumnNames && len(columnNames) > 0 {
		// Apply calculated widths to headers
		headers := f.wrapHeaders(columnNames)
		f.table.Header(headers)
	}

	f.initialized = true

	return nil
}

// WriteRow writes a single table row.
func (f *TableStreamingFormatter) WriteRow(row Row) error {
	if !f.initialized {
		// Buffer rows until initialization
		f.previewRows = append(f.previewRows, row)
		f.rowsBuffered++

		// Check if we have enough preview rows
		if f.previewSize > 0 && f.rowsBuffered >= f.previewSize {
			// Initialize with buffered rows
			return f.InitFormat(f.columns, f.config, f.previewRows)
		}
		return nil
	}

	return f.writeRowInternal(row)
}

// writeRowInternal writes a row to the table stream.
func (f *TableStreamingFormatter) writeRowInternal(row Row) error {
	// Apply calculated widths to row
	wrappedRow := f.wrapRow(row)
	if err := f.table.Append(wrappedRow); err != nil {
		return fmt.Errorf("failed to append row to table: %w", err)
	}
	return nil
}

// FinishFormat completes the table output.
func (f *TableStreamingFormatter) FinishFormat() error {
	// Initialize if not done yet (e.g., fewer rows than preview size)
	if !f.initialized && len(f.columns) > 0 {
		if err := f.InitFormat(f.columns, f.config, f.previewRows); err != nil {
			return err
		}
	}

	if f.table != nil {
		// Close the streaming table
		if err := f.table.Close(); err != nil {
			return fmt.Errorf("failed to close table stream: %w", err)
		}
	}

	return nil
}

// calculateWidths calculates optimal column widths based on preview rows.
func (f *TableStreamingFormatter) calculateWidths(columns []string, previewRows []Row) {
	rw := runewidthex.NewCondition()
	rw.TabWidth = cmp.Or(f.config.TabWidth, 4)

	wc := &widthCalculator{Condition: rw}

	// Calculate optimal widths using column names as header
	f.widths = CalculateWidth(columns, columns, wc, f.screenWidth, previewRows)
}

// wrapHeaders wraps headers according to calculated widths.
func (f *TableStreamingFormatter) wrapHeaders(headers []string) []string {
	if len(f.widths) == 0 {
		return headers
	}

	rw := runewidthex.NewCondition()
	rw.TabWidth = cmp.Or(f.config.TabWidth, 4)

	return slices.Collect(hiter.Unify(
		rw.Wrap,
		hiter.Pairs(slices.Values(headers), slices.Values(f.widths))))
}

// wrapRow wraps row columns according to calculated widths.
func (f *TableStreamingFormatter) wrapRow(row Row) []string {
	if len(f.widths) == 0 {
		return row
	}

	rw := runewidthex.NewCondition()
	rw.TabWidth = cmp.Or(f.config.TabWidth, 4)

	return slices.Collect(hiter.Unify(
		rw.Wrap,
		hiter.Pairs(slices.Values(row), slices.Values(f.widths))))
}
