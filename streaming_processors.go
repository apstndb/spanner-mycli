package main

import (
	"io"
	"strings"

	"github.com/apstndb/spanner-mycli/enums"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
)

// NewStreamingProcessorForMode creates a streaming processor for the given display mode.
// Returns nil if the mode doesn't support streaming yet.
func NewStreamingProcessorForMode(mode enums.DisplayMode, out io.Writer, sysVars *systemVariables, screenWidth int) RowProcessor {
	var formatter StreamingFormatter
	
	switch mode {
	case enums.DisplayModeCSV:
		formatter = NewCSVFormatter(out, sysVars.SkipColumnNames)
		return &StreamingProcessor{
			formatter:   formatter,
			out:         out,
			screenWidth: screenWidth,
		}
		
	case enums.DisplayModeTab:
		formatter = NewTabFormatter(out, sysVars.SkipColumnNames)
		return &StreamingProcessor{
			formatter:   formatter,
			out:         out,
			screenWidth: screenWidth,
		}
		
	case enums.DisplayModeVertical:
		formatter = NewVerticalFormatter(out)
		return &StreamingProcessor{
			formatter:   formatter,
			out:         out,
			screenWidth: screenWidth,
		}
		
	case enums.DisplayModeHTML:
		formatter = NewHTMLStreamingFormatter(out, sysVars.SkipColumnNames)
		return &StreamingProcessor{
			formatter:   formatter,
			out:         out,
			screenWidth: screenWidth,
		}
		
	case enums.DisplayModeXML:
		formatter = NewXMLStreamingFormatter(out, sysVars.SkipColumnNames)
		return &StreamingProcessor{
			formatter:   formatter,
			out:         out,
			screenWidth: screenWidth,
		}
		
	case enums.DisplayModeTable, enums.DisplayModeTableComment, enums.DisplayModeTableDetailComment:
		// Table formats use preview processor for width calculation
		// TODO: Implement table streaming formatter
		return nil
		
	default:
		return nil
	}
}

// HTMLStreamingFormatter provides streaming HTML table output.
type HTMLStreamingFormatter struct {
	out         io.Writer
	skipHeaders bool
	columns     []string
	initialized bool
}

// NewHTMLStreamingFormatter creates a new HTML streaming formatter.
func NewHTMLStreamingFormatter(out io.Writer, skipHeaders bool) *HTMLStreamingFormatter {
	return &HTMLStreamingFormatter{
		out:         out,
		skipHeaders: skipHeaders,
	}
}

// InitFormat starts the HTML table and writes headers.
func (f *HTMLStreamingFormatter) InitFormat(columns []string, metadata *sppb.ResultSetMetadata, sysVars *systemVariables, previewRows []Row) error {
	if f.initialized {
		return nil
	}
	
	f.columns = columns
	
	// Start the table
	if _, err := f.out.Write([]byte("<TABLE BORDER='1'>")); err != nil {
		return err
	}
	
	// Write header row unless skipping
	if !f.skipHeaders && len(columns) > 0 {
		if _, err := f.out.Write([]byte("<TR>")); err != nil {
			return err
		}
		for _, col := range columns {
			if _, err := f.out.Write([]byte("<TH>" + htmlEscape(col) + "</TH>")); err != nil {
				return err
			}
		}
		if _, err := f.out.Write([]byte("</TR>")); err != nil {
			return err
		}
	}
	
	f.initialized = true
	return nil
}

// WriteRow writes a single HTML table row.
func (f *HTMLStreamingFormatter) WriteRow(row Row) error {
	if !f.initialized {
		return nil
	}
	
	if _, err := f.out.Write([]byte("<TR>")); err != nil {
		return err
	}
	
	for _, col := range row {
		if _, err := f.out.Write([]byte("<TD>" + htmlEscape(col) + "</TD>")); err != nil {
			return err
		}
	}
	
	if _, err := f.out.Write([]byte("</TR>")); err != nil {
		return err
	}
	
	return nil
}

// FinishFormat closes the HTML table.
func (f *HTMLStreamingFormatter) FinishFormat(stats QueryStats, rowCount int64) error {
	_, err := f.out.Write([]byte("</TABLE>\n"))
	return err
}

// htmlEscape escapes HTML special characters.
func htmlEscape(s string) string {
	// Simple HTML escaping - could use html.EscapeString for more complete escaping
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}

// XMLStreamingFormatter provides streaming XML output.
type XMLStreamingFormatter struct {
	out         io.Writer
	skipHeaders bool
	columns     []string
	initialized bool
}

// NewXMLStreamingFormatter creates a new XML streaming formatter.
func NewXMLStreamingFormatter(out io.Writer, skipHeaders bool) *XMLStreamingFormatter {
	return &XMLStreamingFormatter{
		out:         out,
		skipHeaders: skipHeaders,
	}
}

// InitFormat starts the XML document and writes headers if needed.
func (f *XMLStreamingFormatter) InitFormat(columns []string, metadata *sppb.ResultSetMetadata, sysVars *systemVariables, previewRows []Row) error {
	if f.initialized {
		return nil
	}
	
	f.columns = columns
	
	// Start XML document
	if _, err := f.out.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")); err != nil {
		return err
	}
	
	if _, err := f.out.Write([]byte(`<resultset xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">` + "\n")); err != nil {
		return err
	}
	
	// Write header if not skipping
	if !f.skipHeaders && len(columns) > 0 {
		if _, err := f.out.Write([]byte("  <header>\n")); err != nil {
			return err
		}
		for _, col := range columns {
			if _, err := f.out.Write([]byte("    <field>" + xmlEscape(col) + "</field>\n")); err != nil {
				return err
			}
		}
		if _, err := f.out.Write([]byte("  </header>\n")); err != nil {
			return err
		}
	}
	
	f.initialized = true
	return nil
}

// WriteRow writes a single XML row element.
func (f *XMLStreamingFormatter) WriteRow(row Row) error {
	if !f.initialized {
		return nil
	}
	
	if _, err := f.out.Write([]byte("  <row>\n")); err != nil {
		return err
	}
	
	for i, value := range row {
		name := "field"
		if i < len(f.columns) && f.columns[i] != "" {
			name = xmlEscape(f.columns[i])
		}
		if _, err := f.out.Write([]byte("    <field name=\"" + name + "\">" + xmlEscape(value) + "</field>\n")); err != nil {
			return err
		}
	}
	
	if _, err := f.out.Write([]byte("  </row>\n")); err != nil {
		return err
	}
	
	return nil
}

// FinishFormat closes the XML document.
func (f *XMLStreamingFormatter) FinishFormat(stats QueryStats, rowCount int64) error {
	_, err := f.out.Write([]byte("</resultset>\n"))
	return err
}

// xmlEscape escapes XML special characters.
func xmlEscape(s string) string {
	// Simple XML escaping - could use xml.EscapeText for more complete escaping
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}