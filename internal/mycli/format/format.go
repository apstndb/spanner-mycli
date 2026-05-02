package format

// This file contains output formatters for query results.
// It implements various output formats (TABLE, CSV, HTML, XML, etc.) with proper error handling.
// All formatters follow a consistent pattern where errors are propagated instead of logged and ignored.

import (
	"fmt"
	"io"
	"slices"

	"github.com/apstndb/go-tabwrap"
	"github.com/apstndb/spanner-mycli/internal/mycli/iterutil"
	"github.com/olekukonko/tablewriter/tw"
)

// formatTable formats output as an ASCII table.
// No writeBuffered wrapping needed: non-streaming TableStreamingFormatter defers
// all output to Render() in FinishFormat(), so no partial output on error.
func formatTable(mode Mode) FormatFunc {
	return func(out io.Writer, rows []Row, columnNames []string, config FormatConfig, screenWidth int) error {
		return WriteTable(out, rows, columnNames, config, screenWidth, mode)
	}
}

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

// formatVertical formats output in vertical format where each row is displayed
// with column names on the left and values on the right.
func formatVertical(out io.Writer, rows []Row, columnNames []string, config FormatConfig, screenWidth int) error {
	return ExecuteWithFormatter(NewVerticalFormatter(out), rows, columnNames, config)
}

// formatTab formats output as tab-separated values.
func formatTab(out io.Writer, rows []Row, columnNames []string, config FormatConfig, screenWidth int) error {
	return ExecuteWithFormatter(NewTabFormatter(out, config.SkipColumnNames), rows, columnNames, config)
}

// formatCSV formats output as comma-separated values following RFC 4180.
func formatCSV(out io.Writer, rows []Row, columnNames []string, config FormatConfig, screenWidth int) error {
	return ExecuteWithFormatter(NewCSVFormatter(out, config.SkipColumnNames), rows, columnNames, config)
}

// formatHTML formats output as an HTML table.
func formatHTML(out io.Writer, rows []Row, columnNames []string, config FormatConfig, screenWidth int) error {
	return ExecuteWithFormatter(NewHTMLFormatter(out, config.SkipColumnNames), rows, columnNames, config)
}

// formatXML formats output as XML.
func formatXML(out io.Writer, rows []Row, columnNames []string, config FormatConfig, screenWidth int) error {
	return ExecuteWithFormatter(NewXMLFormatter(out, config.SkipColumnNames), rows, columnNames, config)
}

// formatJSONL formats output as JSON Lines (one JSON object per row).
func formatJSONL(out io.Writer, rows []Row, columnNames []string, config FormatConfig, screenWidth int) error {
	return ExecuteWithFormatter(NewJSONLFormatter(out), rows, columnNames, config)
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

// NewFormatter creates a new formatter function based on the display mode.
// Built-in modes (TABLE, CSV, etc.) are handled directly.
// Custom modes are looked up in the registry (see RegisterFormatFunc).
func NewFormatter(mode Mode) (FormatFunc, error) {
	switch mode {
	case ModeUnspecified, "":
		return formatTable(ModeTable), nil
	case ModeTable, ModeTableComment, ModeTableDetailComment:
		return formatTable(mode), nil
	case ModeVertical:
		return formatVertical, nil
	case ModeTab:
		return formatTab, nil
	case ModeCSV:
		return formatCSV, nil
	case ModeHTML:
		return formatHTML, nil
	case ModeXML:
		return formatXML, nil
	case ModeJSONL:
		return formatJSONL, nil
	default:
		// Look up in registry for custom modes
		if factory, ok := lookupFormatFunc(mode); ok {
			return factory(mode)
		}
		return nil, errUnsupportedMode("display", mode)
	}
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
// Built-in modes are handled directly. Custom modes are looked up in the registry.
func NewStreamingFormatter(mode Mode, out io.Writer, config FormatConfig) (StreamingFormatter, error) {
	switch mode {
	case ModeCSV:
		return NewCSVFormatter(out, config.SkipColumnNames), nil
	case ModeTab:
		return NewTabFormatter(out, config.SkipColumnNames), nil
	case ModeVertical:
		return NewVerticalFormatter(out), nil
	case ModeHTML:
		return NewHTMLFormatter(out, config.SkipColumnNames), nil
	case ModeXML:
		return NewXMLFormatter(out, config.SkipColumnNames), nil
	case ModeJSONL:
		return NewJSONLFormatter(out), nil
	case ModeTable, ModeTableComment, ModeTableDetailComment:
		// Table formats need screenWidth, so they must be created by the caller
		// Return a dummy formatter for isStreamingSupported check
		if out == io.Discard {
			return NewTableStreamingFormatter(out, config, 0, 0, mode), nil
		}
		return nil, fmt.Errorf("table formats require screenWidth - use NewTableStreamingFormatter directly")
	default:
		// Look up in registry for custom modes
		if factory, ok := lookupStreamingFormatter(mode); ok {
			return factory(mode, out, config)
		}
		return nil, errUnsupportedMode("streaming", mode)
	}
}
