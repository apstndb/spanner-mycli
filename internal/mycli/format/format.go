package format

// This file contains output formatters for query results.
// It implements the display-oriented output formats (TABLE, TAB, TSV, VERTICAL,
// HTML, XML) with proper error handling. CSV, JSONL, and the SQL export modes
// are emitted by the spanvalue writers instead (see the mycli package).
// All formatters follow a consistent pattern where errors are propagated instead of logged and ignored.

import (
	"fmt"
	"io"
	"slices"

	"github.com/apstndb/go-tabwrap"
	"github.com/apstndb/spanner-mycli/internal/mycli/iterutil"
	"github.com/olekukonko/tablewriter/tw"
)

// TableParams holds additional parameters for table formatting that are not
// part of the standard FormatConfig (used only by table format).
type TableParams struct {
	// VerboseHeaders contains header strings rendered with type information.
	// These may include newlines (e.g., "Name\nSTRING") and are used for display
	// when Verbose mode is enabled.
	VerboseHeaders []string
	ColumnAlign    []tw.Align
}

// WriteTable writes the table to the provided writer.
func WriteTable(w io.Writer, rows []Row, columnNames []string, config FormatConfig, screenWidth int, mode Mode) error {
	return WriteTableWithParams(w, rows, columnNames, config, screenWidth, mode, TableParams{})
}

// WriteTableWithParams writes the table with additional table-specific parameters.
// This delegates to TableStreamingFormatter (non-streaming mode) as the single table
// rendering engine, eliminating duplicate table rendering code.
func WriteTableWithParams(w io.Writer, rows []Row, columnNames []string, config FormatConfig, screenWidth int, mode Mode, params TableParams) error {
	formatter := NewTableFormatterForBuffered(w, config, screenWidth, mode, params)
	return ExecuteWithFormatter(formatter, rows, columnNames, config)
}

// wrapRowStyled wraps styled (ANSI-coded) cell text with ControlSequences-aware Wrap.
// SGR state is carried across line breaks, so each output line is independently styled.
// The result is PlainCell containing pre-styled text — no further Format() needed.
func wrapRowStyled(row Row, widths []int, rw *tabwrap.Condition) Row {
	if len(widths) == 0 {
		return row
	}
	rwCSI := &tabwrap.Condition{
		TabWidth:         rw.TabWidth,
		EastAsianWidth:   rw.EastAsianWidth,
		ControlSequences: true,
	}
	// Wrap the styled (Format()) text with ANSI-aware wrapping.
	// The result is a PlainCell containing pre-styled text with SGR carry-over,
	// so writeRowInternal's Formatters() path calls Format() which returns text as-is.
	styledTexts := make([]string, len(row))
	for i, c := range row {
		styledTexts[i] = c.Format()
	}
	result := make(Row, len(row))
	for i, text := range styledTexts {
		wrapped := rwCSI.Wrap(text, widths[i])
		result[i] = PlainCell{Text: wrapped}
	}
	return result
}

// wrapRowPreserving wraps row text to calculated widths while preserving Cell metadata.
// This is the key function that enables per-cell styling through the width-wrapping step:
// the wrapped text replaces the original, but the Cell adapter type is preserved via WithText().
func wrapRowPreserving(row Row, widths []int, rw *tabwrap.Condition) Row {
	if len(widths) == 0 {
		return row
	}
	wrappedTexts := slices.Collect(iterutil.ZipShortestBy(slices.Values(Texts(row)), slices.Values(widths), func(text string, width int) string {
		return rw.Wrap(text, width)
	}))
	result := make(Row, len(wrappedTexts))
	for i, text := range wrappedTexts {
		if i < len(row) {
			result[i] = row[i].WithText(text)
		} else {
			result[i] = PlainCell{Text: text}
		}
	}
	return result
}

// ExecuteWithFormatter executes buffered formatting using a streaming formatter.
// Rows are passed as previewRows to InitFormat, enabling formatters like table
// to calculate optimal column widths from all rows before rendering.
// Non-table formatters ignore previewRows.
func ExecuteWithFormatter(formatter StreamingFormatter, rows []Row, columnNames []string, config FormatConfig) error {
	if len(columnNames) == 0 {
		return nil
	}

	if err := formatter.InitFormat(columnNames, config, rows); err != nil {
		return err
	}

	for i, row := range rows {
		if err := formatter.WriteRow(row); err != nil {
			return fmt.Errorf("failed to write row %d: %w", i+1, err)
		}
	}

	return formatter.FinishFormat()
}

// NewStreamingFormatter creates a streaming formatter for the given display mode.
// Note: Table formats (Table, TableComment, TableDetailComment) require screenWidth
// and should be created with NewTableStreamingFormatter directly by the caller.
// CSV, JSONL, and the SQL export modes are not handled here: they are emitted
// by the spanvalue writers on both the streaming and buffered paths.
func NewStreamingFormatter(mode Mode, out io.Writer, config FormatConfig) (StreamingFormatter, error) {
	switch mode {
	case ModeTab:
		return NewTabFormatter(out, config.SkipColumnNames), nil
	case ModeTSV:
		return NewTSVFormatter(out, config.SkipColumnNames), nil
	case ModeVertical:
		return NewVerticalFormatter(out), nil
	case ModeHTML:
		return NewHTMLFormatter(out, config.SkipColumnNames), nil
	case ModeXML:
		return NewXMLFormatter(out, config.SkipColumnNames), nil
	case ModeTable, ModeTableComment, ModeTableDetailComment:
		// Table formats need screenWidth, so they must be created by the caller
		// Return a dummy formatter for capability checks.
		if out == io.Discard {
			return NewTableStreamingFormatter(out, config, 0, 0, mode), nil
		}
		return nil, fmt.Errorf("table formats require screenWidth - use NewTableStreamingFormatter directly")
	default:
		return nil, fmt.Errorf("unsupported streaming mode: %s", mode)
	}
}
