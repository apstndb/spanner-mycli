package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
)

// CSVFormatter provides shared CSV formatting logic for both buffered and streaming modes.
type CSVFormatter struct {
	writer      *csv.Writer
	out         io.Writer
	skipHeaders bool
	initialized bool
}

// NewCSVFormatter creates a new CSV formatter.
func NewCSVFormatter(out io.Writer, skipHeaders bool) *CSVFormatter {
	return &CSVFormatter{
		out:         out,
		writer:      csv.NewWriter(out),
		skipHeaders: skipHeaders,
	}
}

// InitFormat writes CSV headers if needed.
func (f *CSVFormatter) InitFormat(columns []string, metadata *sppb.ResultSetMetadata, sysVars *systemVariables, previewRows []Row) error {
	if f.initialized {
		return nil
	}

	if len(columns) == 0 {
		return nil
	}

	// Write headers unless skipping
	if !f.skipHeaders {
		if err := f.writer.Write(columns); err != nil {
			return fmt.Errorf("failed to write CSV header: %w", err)
		}
	}

	f.initialized = true
	return nil
}

// WriteRow writes a single CSV row.
func (f *CSVFormatter) WriteRow(row Row) error {
	if !f.initialized {
		return fmt.Errorf("CSV formatter not initialized")
	}

	if err := f.writer.Write(row); err != nil {
		return fmt.Errorf("failed to write CSV row: %w", err)
	}

	// Flush after each row for streaming
	f.writer.Flush()
	return f.writer.Error()
}

// FinishFormat completes CSV output.
func (f *CSVFormatter) FinishFormat(stats QueryStats, rowCount int64) error {
	f.writer.Flush()
	if err := f.writer.Error(); err != nil {
		return fmt.Errorf("CSV writer error: %w", err)
	}
	return nil
}

// TabFormatter provides shared tab-separated formatting logic.
type TabFormatter struct {
	out         io.Writer
	skipHeaders bool
	columns     []string
	initialized bool
}

// NewTabFormatter creates a new tab-separated formatter.
func NewTabFormatter(out io.Writer, skipHeaders bool) *TabFormatter {
	return &TabFormatter{
		out:         out,
		skipHeaders: skipHeaders,
	}
}

// InitFormat writes tab-separated headers if needed.
func (f *TabFormatter) InitFormat(columns []string, metadata *sppb.ResultSetMetadata, sysVars *systemVariables, previewRows []Row) error {
	if f.initialized {
		return nil
	}

	f.columns = columns

	if len(columns) == 0 {
		return nil
	}

	// Write headers unless skipping
	if !f.skipHeaders {
		if _, err := fmt.Fprintln(f.out, strings.Join(columns, "\t")); err != nil {
			return fmt.Errorf("failed to write TAB header: %w", err)
		}
	}

	f.initialized = true
	return nil
}

// WriteRow writes a single tab-separated row.
func (f *TabFormatter) WriteRow(row Row) error {
	if !f.initialized {
		return fmt.Errorf("TAB formatter not initialized")
	}

	if _, err := fmt.Fprintln(f.out, strings.Join(row, "\t")); err != nil {
		return fmt.Errorf("failed to write TAB row: %w", err)
	}

	return nil
}

// FinishFormat completes tab-separated output.
func (f *TabFormatter) FinishFormat(stats QueryStats, rowCount int64) error {
	// Nothing to do for tab format
	return nil
}

// VerticalFormatter provides shared vertical formatting logic.
type VerticalFormatter struct {
	out         io.Writer
	columns     []string
	maxLen      int
	format      string
	rowNumber   int64
	initialized bool
}

// NewVerticalFormatter creates a new vertical formatter.
func NewVerticalFormatter(out io.Writer) *VerticalFormatter {
	return &VerticalFormatter{
		out: out,
	}
}

// InitFormat prepares vertical format output.
func (f *VerticalFormatter) InitFormat(columns []string, metadata *sppb.ResultSetMetadata, sysVars *systemVariables, previewRows []Row) error {
	if f.initialized {
		return nil
	}

	f.columns = columns

	if len(columns) == 0 {
		return nil
	}

	// Calculate max column name length for alignment
	f.maxLen = 0
	for _, col := range columns {
		if len(col) > f.maxLen {
			f.maxLen = len(col)
		}
	}

	f.format = fmt.Sprintf("%%%ds: %%s\n", f.maxLen)
	f.initialized = true
	return nil
}

// WriteRow writes a single row in vertical format.
func (f *VerticalFormatter) WriteRow(row Row) error {
	if !f.initialized {
		return fmt.Errorf("VERTICAL formatter not initialized")
	}

	f.rowNumber++

	// Print row separator
	if _, err := fmt.Fprintf(f.out, "*************************** %d. row ***************************\n", f.rowNumber); err != nil {
		return err
	}

	// Print each column
	for i, value := range row {
		columnName := f.columns[i]
		if i >= len(f.columns) {
			columnName = fmt.Sprintf("Column_%d", i+1)
		}
		if _, err := fmt.Fprintf(f.out, f.format, columnName, value); err != nil {
			return err
		}
	}

	return nil
}

// FinishFormat completes vertical format output.
func (f *VerticalFormatter) FinishFormat(stats QueryStats, rowCount int64) error {
	// Nothing to do for vertical format
	return nil
}
