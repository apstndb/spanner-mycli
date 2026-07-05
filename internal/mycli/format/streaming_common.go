package format

import (
	"fmt"
	"io"
	"strings"
)

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
func (f *TabFormatter) InitFormat(columnNames []string, config FormatConfig, previewRows []Row) error {
	if f.initialized {
		return nil
	}

	f.columns = columnNames

	if len(columnNames) == 0 {
		return nil
	}

	// Write headers unless skipping
	if !f.skipHeaders {
		if _, err := fmt.Fprintln(f.out, strings.Join(columnNames, "\t")); err != nil {
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

	if _, err := fmt.Fprintln(f.out, strings.Join(Texts(row), "\t")); err != nil {
		return fmt.Errorf("failed to write TAB row: %w", err)
	}

	return nil
}

// FinishFormat completes tab-separated output.
func (f *TabFormatter) FinishFormat() error {
	return nil
}

// tsvEscaper escapes characters that would break the TSV row/field structure.
// This is the conventional lossless TSV escaping (as used by MySQL's tab output,
// Postgres COPY text format, etc.): backslash, tab, newline, and carriage return
// are replaced by two-character backslash sequences. Quotes are NOT special in
// TSV and are emitted verbatim.
var tsvEscaper = strings.NewReplacer(
	"\\", `\\`,
	"\t", `\t`,
	"\n", `\n`,
	"\r", `\r`,
)

// escapeTSVFields escapes each field with tsvEscaper.
func escapeTSVFields(fields []string) []string {
	escaped := make([]string, len(fields))
	for i, field := range fields {
		escaped[i] = tsvEscaper.Replace(field)
	}
	return escaped
}

// TSVFormatter provides tab-separated formatting with escaping.
// Unlike TabFormatter (which joins raw cell text and is kept for backward
// compatibility), TSVFormatter guarantees one row per line and one field per
// tab-separated column by escaping tab, newline, carriage return, and
// backslash within values (and header names).
type TSVFormatter struct {
	out         io.Writer
	skipHeaders bool
	initialized bool
}

// NewTSVFormatter creates a new escaping tab-separated (TSV) formatter.
func NewTSVFormatter(out io.Writer, skipHeaders bool) *TSVFormatter {
	return &TSVFormatter{
		out:         out,
		skipHeaders: skipHeaders,
	}
}

// InitFormat writes escaped tab-separated headers if needed.
func (f *TSVFormatter) InitFormat(columnNames []string, config FormatConfig, previewRows []Row) error {
	if f.initialized {
		return nil
	}

	if len(columnNames) == 0 {
		return nil
	}

	// Write headers unless skipping
	if !f.skipHeaders {
		if _, err := fmt.Fprintln(f.out, strings.Join(escapeTSVFields(columnNames), "\t")); err != nil {
			return fmt.Errorf("failed to write TSV header: %w", err)
		}
	}

	f.initialized = true
	return nil
}

// WriteRow writes a single escaped tab-separated row.
func (f *TSVFormatter) WriteRow(row Row) error {
	if !f.initialized {
		return fmt.Errorf("TSV formatter not initialized")
	}

	if _, err := fmt.Fprintln(f.out, strings.Join(escapeTSVFields(Texts(row)), "\t")); err != nil {
		return fmt.Errorf("failed to write TSV row: %w", err)
	}

	return nil
}

// FinishFormat completes tab-separated output.
func (f *TSVFormatter) FinishFormat() error {
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
func (f *VerticalFormatter) InitFormat(columnNames []string, config FormatConfig, previewRows []Row) error {
	if f.initialized {
		return nil
	}

	f.columns = columnNames

	if len(columnNames) == 0 {
		return nil
	}

	// Calculate max column name length for alignment
	f.maxLen = 0
	for _, col := range columnNames {
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
		var columnName string
		if i < len(f.columns) {
			columnName = f.columns[i]
		} else {
			columnName = fmt.Sprintf("Column_%d", i+1)
		}
		if _, err := fmt.Fprintf(f.out, f.format, columnName, value.RawText()); err != nil {
			return err
		}
	}

	return nil
}

// FinishFormat completes vertical format output.
func (f *VerticalFormatter) FinishFormat() error {
	return nil
}
