package format

// This file contains output formatters for query results.
// It implements various output formats (TABLE, CSV, HTML, XML, etc.) with proper error handling.
// All formatters follow a consistent pattern where errors are propagated instead of logged and ignored.

import (
	"cmp"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"

	"github.com/apstndb/go-runewidthex"
	"github.com/ngicks/go-iterator-helper/hiter"
	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/renderer"
	"github.com/olekukonko/tablewriter/tw"
)

var (
	topLeftRe     = regexp.MustCompile(`^\+`)
	bottomRightRe = regexp.MustCompile(`\+$`)
)

// writeBuffered writes to a temporary buffer first, and only writes to out if no error occurs.
// This is useful for formats that need to build the entire output before writing.
func writeBuffered(out io.Writer, buildFunc func(out io.Writer) error) error {
	var buf strings.Builder
	err := buildFunc(&buf)
	if err != nil {
		return err
	}

	output := buf.String()
	if output != "" {
		_, err = fmt.Fprint(out, output)
		return err
	}
	return nil
}

// formatTable formats output as an ASCII table.
// verboseNames provides the verbose header names (with type info) when Verbose is true.
// columnAlign provides per-column alignment for special statements like EXPLAIN.
func formatTable(mode Mode) FormatFunc {
	return func(out io.Writer, rows []Row, columnNames []string, config FormatConfig, screenWidth int) error {
		return writeBuffered(out, func(out io.Writer) error {
			return WriteTable(out, rows, columnNames, config, screenWidth, mode)
		})
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
// verboseNames and columnAlign are passed separately because they are specific to table formatting
// and not available in the generic FormatConfig.
func WriteTable(w io.Writer, rows []Row, columnNames []string, config FormatConfig, screenWidth int, mode Mode) error {
	return WriteTableWithParams(w, rows, columnNames, config, screenWidth, mode, TableParams{})
}

// WriteTableWithParams writes the table with additional table-specific parameters.
func WriteTableWithParams(w io.Writer, rows []Row, columnNames []string, config FormatConfig, screenWidth int, mode Mode, params TableParams) error {
	rw := runewidthex.NewCondition()
	rw.TabWidth = cmp.Or(config.TabWidth, 4)

	// For comment modes, we need to manipulate the output, so use a buffer
	var tableBuf strings.Builder
	tableWriter := w
	if mode == ModeTableComment || mode == ModeTableDetailComment {
		tableWriter = &tableBuf
	}

	// Create a table that writes to tableWriter
	table := tablewriter.NewTable(tableWriter,
		tablewriter.WithRenderer(
			renderer.NewBlueprint(tw.Rendition{Symbols: tw.NewSymbols(tw.StyleASCII)})),
		tablewriter.WithHeaderAlignment(tw.AlignLeft),
		tablewriter.WithTrimSpace(tw.Off),
		tablewriter.WithHeaderAutoFormat(tw.Off),
	).Configure(func(twConfig *tablewriter.Config) {
		if len(params.ColumnAlign) > 0 {
			twConfig.Row.Alignment.PerColumn = params.ColumnAlign
		}
		twConfig.Row.Formatting.AutoWrap = tw.WrapNone
	})

	wc := &widthCalculator{Condition: rw}

	// Use verbose names for width calculation if available
	headerForWidth := columnNames
	if config.Verbose && len(params.VerboseHeaders) > 0 {
		headerForWidth = params.VerboseHeaders
	}
	adjustedWidths := CalculateWidth(columnNames, headerForWidth, wc, screenWidth, rows)

	// Determine display headers
	displayHeaders := columnNames
	if config.Verbose && len(params.VerboseHeaders) > 0 {
		displayHeaders = params.VerboseHeaders
	}

	headers := slices.Collect(hiter.Unify(
		rw.Wrap,
		hiter.Pairs(
			slices.Values(displayHeaders),
			slices.Values(adjustedWidths))))

	if !config.SkipColumnNames {
		table.Header(headers)
	}

	for _, row := range rows {
		wrappedColumns := slices.Collect(hiter.Unify(
			rw.Wrap,
			hiter.Pairs(slices.Values(row), slices.Values(adjustedWidths))))
		if err := table.Append(wrappedColumns); err != nil {
			return fmt.Errorf("failed to append row: %w", err)
		}
	}

	forceTableRender := config.Verbose && len(headers) > 0

	if forceTableRender || len(rows) > 0 {
		if err := table.Render(); err != nil {
			return fmt.Errorf("failed to render table: %w", err)
		}
	}

	// Handle comment mode transformations
	if mode == ModeTableComment || mode == ModeTableDetailComment {
		s := strings.TrimSpace(tableBuf.String())
		// Sanitize */ in table content to prevent premature SQL comment closure.
		s = strings.ReplaceAll(s, "*/", "* /")
		s = strings.ReplaceAll(s, "\n", "\n ")
		s = topLeftRe.ReplaceAllLiteralString(s, "/*")

		if mode == ModeTableComment {
			s = bottomRightRe.ReplaceAllLiteralString(s, "*/")
		}

		if s != "" {
			if _, err := fmt.Fprintln(w, s); err != nil {
				return err
			}
		}
	}

	return nil
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

// NewFormatter creates a new formatter function based on the display mode.
// Built-in modes (TABLE, CSV, etc.) are handled directly.
// Custom modes are looked up in the registry (see RegisterFormatFunc).
func NewFormatter(mode Mode) (FormatFunc, error) {
	switch mode {
	case "UNSPECIFIED", "":
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
	default:
		// Look up in registry for custom modes
		if factory, ok := lookupFormatFunc(mode); ok {
			return factory(mode)
		}
		return nil, errUnsupportedMode("display", mode)
	}
}

// ExecuteWithFormatter executes buffered formatting using a streaming formatter.
// This reduces duplication in formatCSV, formatTab, formatVertical, etc.
func ExecuteWithFormatter(formatter StreamingFormatter, rows []Row, columnNames []string, config FormatConfig) error {
	if len(columnNames) == 0 {
		return nil
	}

	if err := formatter.InitFormat(columnNames, config, nil); err != nil {
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
	case ModeTable, ModeTableComment, ModeTableDetailComment:
		// Table formats need screenWidth, so they must be created by the caller
		// Return a dummy formatter for isStreamingSupported check
		if out == io.Discard {
			return NewTableStreamingFormatter(out, config, 0, 0), nil
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
