package format

import (
	"io"

	"github.com/apstndb/spanner-mycli/enums"
)

// Cell is the interface for a single formatted cell.
// The format package is agnostic to the concrete type — it only calls these methods.
// Concrete adapters (PlainCell, StyledCell) implement styling logic.
//
// Cell also satisfies tw.Formatter (via Format()), enabling per-cell styling
// when passed to tablewriter.
type Cell interface {
	// Format returns styled text for display (may include ANSI codes).
	// Called by tablewriter via tw.Formatter interface when FormatConfig.Styled is true.
	// When Styled is false, callers should use RawText() instead.
	Format() string

	// RawText returns plain text without styling.
	// Used by non-table formatters (CSV, XML, etc.) that must not contain ANSI codes.
	RawText() string

	// WithText creates a new Cell with replaced text but preserved styling behavior.
	// Used by table formatters after width-wrapping to carry styling through.
	WithText(text string) Cell
}

// Row is a slice of Cell representing one row of formatted data.
type Row = []Cell

// PlainCell is the simplest Cell implementation — no styling.
// Used by client-side statements (SHOW, DESCRIBE, DUMP) and tests.
type PlainCell struct {
	Text string
}

func (c PlainCell) Format() string         { return c.Text }
func (c PlainCell) RawText() string        { return c.Text }
func (c PlainCell) WithText(s string) Cell { return PlainCell{Text: s} }

const ansiReset = "\033[0m"

// StyledCell renders values with a configurable ANSI SGR sequence.
// Used for type-based styling (e.g., STRING → green, INT64 → bold, NULL → dim).
// The Style field holds an ANSI SGR sequence (e.g., "\033[32m" for green).
// In the styled path, wrapRowStyled handles SGR carry-over across line breaks,
// so Format() only needs to wrap the entire text — no per-line logic needed.
type StyledCell struct {
	Text  string
	Style string // ANSI SGR sequence, e.g. "\033[32m" for green, "\033[1m" for bold
}

func (c StyledCell) Format() string {
	if c.Style == "" {
		return c.Text
	}
	return c.Style + c.Text + ansiReset
}
func (c StyledCell) RawText() string        { return c.Text }
func (c StyledCell) WithText(s string) Cell { return StyledCell{Text: s, Style: c.Style} }

// NoWrapCell wraps any Cell to indicate its content should preferably not be wrapped.
// Strategies use this to compute PreferredMinWidth per column so that short values
// like NULL, true, false are kept intact when space allows.
type NoWrapCell struct {
	Cell
}

func (c NoWrapCell) WithText(s string) Cell { return NoWrapCell{Cell: c.Cell.WithText(s)} }

// IsNoWrap reports whether c is a NoWrapCell.
func IsNoWrap(c Cell) bool {
	_, ok := c.(NoWrapCell)
	return ok
}

// StringsToRow converts a slice of strings to a Row of PlainCell.
// Used by client-side statements and tests that construct rows from plain strings.
func StringsToRow(ss ...string) Row {
	row := make(Row, len(ss))
	for i, s := range ss {
		row[i] = PlainCell{Text: s}
	}
	return row
}

// Texts extracts the raw text from each Cell, returning a plain string slice.
// Used when a formatter needs to pass values to APIs that only accept []string.
func Texts(row Row) []string {
	ss := make([]string, len(row))
	for i, c := range row {
		ss[i] = c.RawText()
	}
	return ss
}

// Formatters converts a Row to []any for tablewriter per-cell formatting.
// Each Cell implements tw.Formatter, so tablewriter calls Format() per cell.
func Formatters(row Row) []any {
	fs := make([]any, len(row))
	for i, c := range row {
		fs[i] = c
	}
	return fs
}

// toAnySlice converts []string to []any.
// Used to pass plain text to tablewriter without triggering tw.Formatter.
func toAnySlice(ss []string) []any {
	result := make([]any, len(ss))
	for i, s := range ss {
		result[i] = s
	}
	return result
}

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
	Styled          bool                // When true, table output uses Cell.Format() (may include ANSI codes). When false, uses RawText().
	WidthStrategy   enums.WidthStrategy // Column width allocation algorithm. Zero value = GreedyFrequency (default).
	TabVisualize    bool                // When true, visualize tab characters with → symbol in table output.
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
