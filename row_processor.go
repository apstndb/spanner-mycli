package main

import (
	"io"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
)

// RowProcessor handles rows either in buffered or streaming mode.
// It provides a unified interface for processing query results regardless
// of whether rows are collected first or streamed directly.
type RowProcessor interface {
	// Init is called once after metadata becomes available (after first row fetch).
	// This is where headers are written for formats like CSV, or table initialization occurs.
	Init(metadata *sppb.ResultSetMetadata, sysVars *systemVariables) error

	// ProcessRow is called for each row in the result set.
	// In buffered mode, rows are collected. In streaming mode, rows are output immediately.
	ProcessRow(row Row) error

	// Finish is called after all rows have been processed.
	// It receives final statistics and row count for summary output.
	Finish(stats QueryStats, rowCount int64) error
}

// BufferedProcessor collects all rows in memory before formatting.
// This is the traditional approach used by spanner-mycli.
type BufferedProcessor struct {
	rows        []Row
	metadata    *sppb.ResultSetMetadata
	sysVars     *systemVariables
	formatter   FormatFunc
	out         io.Writer
	screenWidth int
	result      *Result // Accumulates the complete result
}

// NewBufferedProcessor creates a processor that collects all rows before formatting.
func NewBufferedProcessor(formatter FormatFunc, out io.Writer, screenWidth int) *BufferedProcessor {
	return &BufferedProcessor{
		formatter:   formatter,
		out:         out,
		screenWidth: screenWidth,
		rows:        []Row{},
		result:      &Result{},
	}
}

// Init initializes the buffered processor with metadata.
func (p *BufferedProcessor) Init(metadata *sppb.ResultSetMetadata, sysVars *systemVariables) error {
	p.metadata = metadata
	p.sysVars = sysVars
	p.result.TableHeader = toTableHeader(metadata.GetRowType().GetFields())
	return nil
}

// ProcessRow adds a row to the buffer.
func (p *BufferedProcessor) ProcessRow(row Row) error {
	p.rows = append(p.rows, row)
	return nil
}

// Finish formats and outputs all collected rows.
func (p *BufferedProcessor) Finish(stats QueryStats, rowCount int64) error {
	p.result.Rows = p.rows
	p.result.Stats = stats
	p.result.AffectedRows = len(p.rows)

	// Extract column names for formatting
	columnNames := extractTableColumnNames(p.result.TableHeader)

	// Use the existing formatter
	return p.formatter(p.out, p.result, columnNames, p.sysVars, p.screenWidth)
}

// StreamingProcessor processes rows immediately as they arrive.
// This reduces memory usage and improves Time To First Byte for large result sets.
type StreamingProcessor struct {
	metadata    *sppb.ResultSetMetadata
	sysVars     *systemVariables
	formatter   StreamingFormatter
	out         io.Writer
	screenWidth int
	rowCount    int64
	initialized bool
}

// StreamingFormatter defines the interface for format-specific streaming output.
// Each format (CSV, TAB, etc.) implements this interface to handle streaming output.
type StreamingFormatter interface {
	// InitFormat is called once with table header information to output headers.
	// TableHeader provides both column names and type information (if available).
	// For table formats, previewRows contains the first N rows for width calculation.
	// For other formats, previewRows may be empty as they don't need preview.
	InitFormat(header TableHeader, sysVars *systemVariables, previewRows []Row) error

	// WriteRow outputs a single row.
	WriteRow(row Row) error

	// FinishFormat completes the output (e.g., closing tags, final flush).
	FinishFormat(stats QueryStats, rowCount int64) error
}

// TablePreviewProcessor collects a configurable number of rows for table width calculation.
// This allows table formats to determine optimal column widths before starting output.
type TablePreviewProcessor struct {
	previewSize int   // Number of rows to preview (0 = all rows for non-streaming)
	previewRows []Row // Collected preview rows
	formatter   StreamingFormatter
	metadata    *sppb.ResultSetMetadata
	sysVars     *systemVariables
	initialized bool
	rowCount    int64
}

// NewTablePreviewProcessor creates a processor that previews rows for width calculation.
// previewSize of 0 means collect all rows (non-streaming mode).
func NewTablePreviewProcessor(formatter StreamingFormatter, previewSize int) *TablePreviewProcessor {
	return &TablePreviewProcessor{
		formatter:   formatter,
		previewSize: previewSize,
		previewRows: []Row{},
	}
}

// Init stores metadata for later use.
func (p *TablePreviewProcessor) Init(metadata *sppb.ResultSetMetadata, sysVars *systemVariables) error {
	p.metadata = metadata
	p.sysVars = sysVars
	return nil
}

// ProcessRow collects rows for preview or passes them through after initialization.
func (p *TablePreviewProcessor) ProcessRow(row Row) error {
	p.rowCount++

	// If not initialized yet and still collecting preview
	if !p.initialized {
		// Special case: previewSize = 0 means headers only (no row preview)
		if p.previewSize == 0 && p.metadata != nil {
			// Initialize immediately with no preview rows
			if err := p.initializeFormatter(); err != nil {
				return err
			}
			// Write this first row
			return p.formatter.WriteRow(row)
		}

		p.previewRows = append(p.previewRows, row)

		// Check if we've collected enough preview rows
		if p.previewSize > 0 && len(p.previewRows) >= p.previewSize {
			// Initialize formatter with preview rows
			if err := p.initializeFormatter(); err != nil {
				return err
			}
			// Don't write preview rows yet - they'll be written in the formatter
		}
		return nil
	}

	// After initialization, pass rows directly to formatter
	return p.formatter.WriteRow(row)
}

// Finish ensures formatter is initialized and completes output.
func (p *TablePreviewProcessor) Finish(stats QueryStats, rowCount int64) error {
	// Initialize if not done yet (e.g., fewer rows than preview size)
	if !p.initialized {
		if err := p.initializeFormatter(); err != nil {
			return err
		}
	}

	return p.formatter.FinishFormat(stats, rowCount)
}

// initializeFormatter initializes the formatter with preview rows.
func (p *TablePreviewProcessor) initializeFormatter() error {
	if p.initialized {
		return nil
	}

	header := toTableHeader(p.metadata.GetRowType().GetFields())

	// Initialize formatter with preview rows for width calculation
	if err := p.formatter.InitFormat(header, p.sysVars, p.previewRows); err != nil {
		return err
	}

	p.initialized = true

	// Write all preview rows
	for _, row := range p.previewRows {
		if err := p.formatter.WriteRow(row); err != nil {
			return err
		}
	}

	return nil
}

// NewStreamingProcessor creates a processor that outputs rows immediately.
func NewStreamingProcessor(formatter StreamingFormatter, out io.Writer, screenWidth int) *StreamingProcessor {
	return &StreamingProcessor{
		formatter:   formatter,
		out:         out,
		screenWidth: screenWidth,
	}
}

// Init initializes the streaming processor and writes headers if needed.
func (p *StreamingProcessor) Init(metadata *sppb.ResultSetMetadata, sysVars *systemVariables) error {
	p.metadata = metadata
	p.sysVars = sysVars

	// Get header from metadata
	header := toTableHeader(metadata.GetRowType().GetFields())

	// Initialize the format (e.g., write CSV headers)
	// For streaming formats, we don't need preview rows
	if err := p.formatter.InitFormat(header, sysVars, nil); err != nil {
		return err
	}

	p.initialized = true
	return nil
}

// ProcessRow immediately outputs a row.
func (p *StreamingProcessor) ProcessRow(row Row) error {
	if !p.initialized {
		return nil // Skip if not initialized (shouldn't happen)
	}
	p.rowCount++
	return p.formatter.WriteRow(row)
}

// Finish completes the streaming output.
func (p *StreamingProcessor) Finish(stats QueryStats, rowCount int64) error {
	return p.formatter.FinishFormat(stats, rowCount)
}
