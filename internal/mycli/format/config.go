package format

import "io"

// Row is a type alias for a row of string values.
// Using a type alias (not a new type) ensures zero breaking change at call sites.
type Row = []string

// FormatConfig holds configuration values needed by formatters.
// This replaces the dependency on *systemVariables, exposing only the fields
// that formatters actually use.
type FormatConfig struct {
	TabWidth        int
	Verbose         bool
	SkipColumnNames bool
	SQLTableName    string
	SQLBatchSize    int64
	PreviewRows     int64
}

// FormatFunc is a function type that formats and writes result data.
// It takes an output writer, rows, column names, config, and screen width.
type FormatFunc func(out io.Writer, rows []Row, columnNames []string, config FormatConfig, screenWidth int) error

// StreamingFormatter defines the interface for format-specific streaming output.
// Each format (CSV, TAB, etc.) implements this interface to handle streaming output.
type StreamingFormatter interface {
	// InitFormat is called once with column names and configuration.
	// For table formats, previewRows contains the first N rows for width calculation.
	// For other formats, previewRows may be empty as they don't need preview.
	InitFormat(columnNames []string, config FormatConfig, previewRows []Row) error

	// WriteRow outputs a single row.
	WriteRow(row Row) error

	// FinishFormat completes the output (e.g., closing tags, final flush).
	FinishFormat() error
}
