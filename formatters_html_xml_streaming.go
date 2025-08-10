package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"html"
	"io"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
)

// HTMLFormatter provides streaming HTML table output.
type HTMLFormatter struct {
	out         io.Writer
	skipHeaders bool
	columns     []string
	initialized bool
	rowCount    int64
}

// NewHTMLFormatter creates a new HTML streaming formatter.
func NewHTMLFormatter(out io.Writer, skipHeaders bool) *HTMLFormatter {
	return &HTMLFormatter{
		out:         out,
		skipHeaders: skipHeaders,
	}
}

// InitFormat writes the HTML table opening and headers.
func (f *HTMLFormatter) InitFormat(columns []string, metadata *sppb.ResultSetMetadata, sysVars *systemVariables, previewRows []Row) error {
	if f.initialized {
		return nil
	}

	f.columns = columns

	if len(columns) == 0 {
		return fmt.Errorf("no columns to output")
	}

	// Start HTML table
	if _, err := fmt.Fprint(f.out, "<TABLE BORDER='1'>"); err != nil {
		return err
	}

	// Write headers unless skipping
	if !f.skipHeaders {
		if _, err := fmt.Fprint(f.out, "<TR>"); err != nil {
			return err
		}
		for _, col := range columns {
			if _, err := fmt.Fprintf(f.out, "<TH>%s</TH>", html.EscapeString(col)); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprint(f.out, "</TR>"); err != nil {
			return err
		}
	}

	f.initialized = true
	return nil
}

// WriteRow writes a single HTML table row.
func (f *HTMLFormatter) WriteRow(row Row) error {
	if !f.initialized {
		return fmt.Errorf("HTML formatter not initialized")
	}

	f.rowCount++

	if _, err := fmt.Fprint(f.out, "<TR>"); err != nil {
		return err
	}
	for _, col := range row {
		if _, err := fmt.Fprintf(f.out, "<TD>%s</TD>", html.EscapeString(col)); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprint(f.out, "</TR>"); err != nil {
		return err
	}

	return nil
}

// FinishFormat completes the HTML table.
func (f *HTMLFormatter) FinishFormat(stats QueryStats, rowCount int64) error {
	if _, err := fmt.Fprintln(f.out, "</TABLE>"); err != nil {
		return err
	}
	return nil
}

// XMLFormatter provides streaming XML output.
type XMLFormatter struct {
	out         io.Writer
	skipHeaders bool
	columns     []string
	encoder     *xml.Encoder
	initialized bool
	rowCount    int64
}

// NewXMLFormatter creates a new XML streaming formatter.
func NewXMLFormatter(out io.Writer, skipHeaders bool) *XMLFormatter {
	return &XMLFormatter{
		out:         out,
		skipHeaders: skipHeaders,
	}
}

// InitFormat writes the XML declaration and starts the result set.
func (f *XMLFormatter) InitFormat(columns []string, metadata *sppb.ResultSetMetadata, sysVars *systemVariables, previewRows []Row) error {
	if f.initialized {
		return nil
	}

	f.columns = columns

	if len(columns) == 0 {
		return fmt.Errorf("no columns to output")
	}

	// Write XML declaration
	if _, err := fmt.Fprintln(f.out, "<?xml version='1.0'?>"); err != nil {
		return err
	}

	// Start resultset element
	if _, err := fmt.Fprint(f.out, `<resultset xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">`); err != nil {
		return err
	}

	// Write header if not skipping
	if !f.skipHeaders {
		if _, err := fmt.Fprint(f.out, "<header>"); err != nil {
			return err
		}
		for _, col := range columns {
			if _, err := fmt.Fprintf(f.out, "<field>%s</field>", xmlEscape(col)); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprint(f.out, "</header>"); err != nil {
			return err
		}
	}

	f.encoder = xml.NewEncoder(f.out)
	f.initialized = true
	return nil
}

// WriteRow writes a single XML row.
func (f *XMLFormatter) WriteRow(row Row) error {
	if !f.initialized {
		return fmt.Errorf("XML formatter not initialized")
	}

	f.rowCount++

	// Write row opening tag
	if _, err := fmt.Fprint(f.out, "<row>"); err != nil {
		return err
	}

	// Write fields
	for _, value := range row {
		if _, err := fmt.Fprintf(f.out, "<field>%s</field>", xmlEscape(value)); err != nil {
			return err
		}
	}

	// Write row closing tag
	if _, err := fmt.Fprint(f.out, "</row>"); err != nil {
		return err
	}

	return nil
}

// FinishFormat completes the XML output.
func (f *XMLFormatter) FinishFormat(stats QueryStats, rowCount int64) error {
	// Close resultset
	if _, err := fmt.Fprintln(f.out, "</resultset>"); err != nil {
		return err
	}
	return nil
}

// xmlEscape escapes special XML characters.
func xmlEscape(s string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(s)) // Error ignored: EscapeText only errors on Write failure
	return buf.String()
}
