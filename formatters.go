package main

// This file contains output formatters for query results.
// It implements various output formats (TABLE, CSV, HTML, XML, etc.) with proper error handling.
// All formatters follow a consistent pattern where errors are propagated instead of logged and ignored.

import (
	"cmp"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"html"
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

// FormatFunc is a function type that formats and writes result data.
// It takes an output writer and result data, and returns an error if any write operation fails.
type FormatFunc func(out io.Writer, result *Result, columnNames []string, sysVars *systemVariables, screenWidth int) error

// formatTable formats output as an ASCII table.
func formatTable(mode DisplayMode) FormatFunc {
	return func(out io.Writer, result *Result, columnNames []string, sysVars *systemVariables, screenWidth int) error {
		return writeBuffered(out, func(out io.Writer) error {
			return writeTable(out, result, columnNames, sysVars, screenWidth, mode)
		})
	}
}

// writeTable writes the table to the provided writer.
func writeTable(w io.Writer, result *Result, columnNames []string, sysVars *systemVariables, screenWidth int, mode DisplayMode) error {
	rw := runewidthex.NewCondition()
	rw.TabWidth = cmp.Or(int(sysVars.TabWidth), 4)

	rows := result.Rows

	// For comment modes, we need to manipulate the output, so use a buffer
	var tableBuf strings.Builder
	tableWriter := w
	if mode == DisplayModeTableComment || mode == DisplayModeTableDetailComment {
		tableWriter = &tableBuf
	}

	// Create a table that writes to tableWriter
	table := tablewriter.NewTable(tableWriter,
		tablewriter.WithRenderer(
			renderer.NewBlueprint(tw.Rendition{Symbols: tw.NewSymbols(tw.StyleASCII)})),
		tablewriter.WithHeaderAlignment(tw.AlignLeft),
		tablewriter.WithTrimSpace(tw.Off),
		tablewriter.WithHeaderAutoFormat(tw.Off),
	).Configure(func(config *tablewriter.Config) {
		config.Row.ColumnAligns = result.ColumnAlign
		config.Row.Formatting.AutoWrap = tw.WrapNone
	})

	wc := &widthCalculator{Condition: rw}
	adjustedWidths := calculateWidth(result, wc, screenWidth, rows)

	headers := slices.Collect(hiter.Unify(
		rw.Wrap,
		hiter.Pairs(
			slices.Values(renderTableHeader(result.TableHeader, sysVars.Verbose)),
			slices.Values(adjustedWidths))))

	if !sysVars.SkipColumnNames {
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

	forceTableRender := sysVars.Verbose && len(headers) > 0

	if forceTableRender || len(rows) > 0 {
		if err := table.Render(); err != nil {
			return fmt.Errorf("failed to render table: %w", err)
		}
	}

	// Handle comment mode transformations
	if mode == DisplayModeTableComment || mode == DisplayModeTableDetailComment {
		s := strings.TrimSpace(tableBuf.String())
		s = strings.ReplaceAll(s, "\n", "\n ")
		s = topLeftRe.ReplaceAllLiteralString(s, "/*")

		if mode == DisplayModeTableComment {
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
// This is a streaming format that outputs row-by-row without buffering.
func formatVertical(out io.Writer, result *Result, columnNames []string, sysVars *systemVariables, screenWidth int) error {
	if len(columnNames) == 0 {
		return nil
	}

	maxLen := 0
	for _, columnName := range columnNames {
		if len(columnName) > maxLen {
			maxLen = len(columnName)
		}
	}

	format := fmt.Sprintf("%%%ds: %%s\n", maxLen)

	for i, row := range result.Rows {
		if _, err := fmt.Fprintf(out, "*************************** %d. row ***************************\n", i+1); err != nil {
			return err
		}
		for j, column := range row {
			var columnName string
			if j < len(columnNames) {
				columnName = columnNames[j]
			} else {
				// Use a default column name if row has more columns than headers
				columnName = fmt.Sprintf("Column_%d", j+1)
			}
			if _, err := fmt.Fprintf(out, format, columnName, column); err != nil {
				return err
			}
		}
	}

	return nil
}

// formatTab formats output as tab-separated values.
func formatTab(out io.Writer, result *Result, columnNames []string, sysVars *systemVariables, screenWidth int) error {
	if len(columnNames) == 0 {
		return nil
	}

	if !sysVars.SkipColumnNames {
		if _, err := fmt.Fprintln(out, strings.Join(columnNames, "\t")); err != nil {
			return fmt.Errorf("failed to write TAB header: %w", err)
		}
	}

	for i, row := range result.Rows {
		if _, err := fmt.Fprintln(out, strings.Join(row, "\t")); err != nil {
			return fmt.Errorf("failed to write TAB row %d: %w", i+1, err)
		}
	}

	return nil
}

// formatCSV formats output as comma-separated values following RFC 4180.
func formatCSV(out io.Writer, result *Result, columnNames []string, sysVars *systemVariables, screenWidth int) error {
	if len(columnNames) == 0 {
		return fmt.Errorf("no columns to output")
	}

	csvWriter := csv.NewWriter(out)
	defer csvWriter.Flush()

	if !sysVars.SkipColumnNames {
		if err := csvWriter.Write(columnNames); err != nil {
			return fmt.Errorf("failed to write CSV header: %w", err)
		}
	}

	for i, row := range result.Rows {
		if err := csvWriter.Write(row); err != nil {
			return fmt.Errorf("failed to write CSV row %d: %w", i+1, err)
		}
	}

	// Check for any error that occurred during writing
	if err := csvWriter.Error(); err != nil {
		return fmt.Errorf("CSV writer error: %w", err)
	}

	return nil
}

// formatHTML formats output as an HTML table.
// This is a streaming format that outputs row-by-row without buffering.
func formatHTML(out io.Writer, result *Result, columnNames []string, sysVars *systemVariables, screenWidth int) error {
	if len(columnNames) == 0 {
		return fmt.Errorf("no columns to output")
	}

	if _, err := fmt.Fprint(out, "<TABLE BORDER='1'>"); err != nil {
		return err
	}

	// Add header row unless skipping column names
	if !sysVars.SkipColumnNames {
		if _, err := fmt.Fprint(out, "<TR>"); err != nil {
			return err
		}
		for _, col := range columnNames {
			if _, err := fmt.Fprintf(out, "<TH>%s</TH>", html.EscapeString(col)); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprint(out, "</TR>"); err != nil {
			return err
		}
	}

	// Add data rows
	for _, row := range result.Rows {
		if _, err := fmt.Fprint(out, "<TR>"); err != nil {
			return err
		}
		for _, col := range row {
			if _, err := fmt.Fprintf(out, "<TD>%s</TD>", html.EscapeString(col)); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprint(out, "</TR>"); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintln(out, "</TABLE>"); err != nil {
		return err
	}
	return nil
}

// xmlField represents a field element in XML output.
type xmlField struct {
	XMLName xml.Name `xml:"field"`
	Value   string   `xml:",chardata"`
}

// xmlRow represents a row element containing multiple fields.
type xmlRow struct {
	XMLName xml.Name   `xml:"row"`
	Fields  []xmlField `xml:"field"`
}

// xmlHeader represents the optional header element containing column names.
type xmlHeader struct {
	XMLName xml.Name   `xml:"header"`
	Fields  []xmlField `xml:"field"`
}

// xmlResultSet represents the root element of the XML output.
type xmlResultSet struct {
	XMLName xml.Name   `xml:"resultset"`
	XMLNS   string     `xml:"xmlns:xsi,attr"`
	Header  *xmlHeader `xml:"header,omitempty"`
	Rows    []xmlRow   `xml:"row"`
}

// formatXML formats output as XML.
func formatXML(out io.Writer, result *Result, columnNames []string, sysVars *systemVariables, screenWidth int) error {
	return writeBuffered(out, func(out io.Writer) error {
		if len(columnNames) == 0 {
			return fmt.Errorf("no columns to output")
		}

		// Build the result set structure
		resultSet := xmlResultSet{
			XMLNS: "http://www.w3.org/2001/XMLSchema-instance",
			Rows:  make([]xmlRow, 0, len(result.Rows)),
		}

		// Add header fields only if not skipping column names
		if !sysVars.SkipColumnNames {
			header := &xmlHeader{Fields: make([]xmlField, 0, len(columnNames))}
			for _, col := range columnNames {
				header.Fields = append(header.Fields, xmlField{Value: col})
			}
			resultSet.Header = header
		}

		// Add rows
		for _, row := range result.Rows {
			xmlRow := xmlRow{Fields: make([]xmlField, 0, len(row))}
			for _, col := range row {
				xmlRow.Fields = append(xmlRow.Fields, xmlField{Value: col})
			}
			resultSet.Rows = append(resultSet.Rows, xmlRow)
		}

		// Write XML declaration
		if _, err := fmt.Fprintln(out, "<?xml version='1.0'?>"); err != nil {
			return err
		}

		// Marshal the result set
		encoder := xml.NewEncoder(out)
		encoder.Indent("", "\t")
		if err := encoder.Encode(resultSet); err != nil {
			return fmt.Errorf("xml encode failed: %w", err)
		}
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		} // Add final newline

		return nil
	})
}

// NewFormatter creates a new formatter function based on the display mode.
func NewFormatter(mode DisplayMode) (FormatFunc, error) {
	switch mode {
	case DisplayModeTable, DisplayModeTableComment, DisplayModeTableDetailComment:
		return formatTable(mode), nil
	case DisplayModeVertical:
		return formatVertical, nil
	case DisplayModeTab:
		return formatTab, nil
	case DisplayModeCSV:
		return formatCSV, nil
	case DisplayModeHTML:
		return formatHTML, nil
	case DisplayModeXML:
		return formatXML, nil
	default:
		return nil, fmt.Errorf("unsupported display mode: %v", mode)
	}
}
