package main

import (
	"fmt"
	"io"
	"slices"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
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
	sysVars      *systemVariables
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
func NewTableStreamingFormatter(out io.Writer, sysVars *systemVariables, screenWidth int, previewSize int) *TableStreamingFormatter {
	return &TableStreamingFormatter{
		out:         out,
		sysVars:     sysVars,
		screenWidth: screenWidth,
		previewSize: previewSize,
		previewRows: []Row{},
	}
}

// InitFormat initializes the table with preview rows for width calculation.
func (f *TableStreamingFormatter) InitFormat(columns []string, metadata *sppb.ResultSetMetadata, sysVars *systemVariables, previewRows []Row) error {
	if f.initialized {
		return nil
	}

	f.columns = columns
	f.sysVars = sysVars
	f.previewRows = previewRows

	// Calculate optimal widths using preview rows
	f.calculateWidths(columns, previewRows)

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
	).Configure(func(config *tablewriter.Config) {
		// Note: Column alignment is not set here because:
		// 1. Regular SQL queries don't specify column alignment (defaults to left)
		// 2. EXPLAIN/DESCRIBE statements that use custom alignment don't use streaming mode
		// They return a complete Result with ColumnAlign set and use buffered formatting
		config.Row.Formatting.AutoWrap = tw.WrapNone
	})

	// Start streaming table
	if err := f.table.Start(); err != nil {
		return fmt.Errorf("failed to start table streaming: %w", err)
	}

	// Set headers
	if !f.sysVars.SkipColumnNames && len(columns) > 0 {
		// Apply calculated widths to headers
		headers := f.wrapHeaders(columns)
		// Header method doesn't return an error in tablewriter v1.0.9
		f.table.Header(headers)
	}

	f.initialized = true

	// Don't write preview rows here - they will be written by the TablePreviewProcessor
	// after this initialization completes

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
			return f.InitFormat(f.columns, nil, f.sysVars, f.previewRows)
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
func (f *TableStreamingFormatter) FinishFormat(stats QueryStats, rowCount int64) error {
	// Initialize if not done yet (e.g., fewer rows than preview size)
	if !f.initialized && len(f.columns) > 0 {
		if err := f.InitFormat(f.columns, nil, f.sysVars, f.previewRows); err != nil {
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
// If previewRows is empty, it uses only header names for width calculation.
func (f *TableStreamingFormatter) calculateWidths(columns []string, previewRows []Row) {
	rw := runewidthex.NewCondition()
	rw.TabWidth = 4
	if f.sysVars != nil && f.sysVars.TabWidth > 0 {
		rw.TabWidth = int(f.sysVars.TabWidth)
	}

	wc := &widthCalculator{Condition: rw}

	// If no preview rows, calculate based on headers only
	rowsForCalculation := previewRows
	if len(previewRows) == 0 {
		// Use empty rows for header-only calculation
		// calculateWidth will use header widths as the baseline
		rowsForCalculation = []Row{}
	}

	// Create a mock result for width calculation
	mockResult := &Result{
		TableHeader: &streamingTableHeader{columns: columns},
		Rows:        rowsForCalculation,
	}

	// Calculate optimal widths
	f.widths = calculateWidth(mockResult, wc, f.screenWidth, rowsForCalculation)
}

// wrapHeaders wraps headers according to calculated widths.
func (f *TableStreamingFormatter) wrapHeaders(headers []string) []string {
	if len(f.widths) == 0 {
		return headers
	}

	rw := runewidthex.NewCondition()
	rw.TabWidth = 4
	if f.sysVars != nil && f.sysVars.TabWidth > 0 {
		rw.TabWidth = int(f.sysVars.TabWidth)
	}

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
	rw.TabWidth = 4
	if f.sysVars != nil && f.sysVars.TabWidth > 0 {
		rw.TabWidth = int(f.sysVars.TabWidth)
	}

	return slices.Collect(hiter.Unify(
		rw.Wrap,
		hiter.Pairs(slices.Values(row), slices.Values(f.widths))))
}

// streamingTableHeader implements TableHeader for width calculation in streaming mode.
type streamingTableHeader struct {
	columns []string
}

func (h *streamingTableHeader) internalRender(verbose bool) []string {
	return h.columns
}
