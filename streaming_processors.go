package main

import (
	"io"

	"github.com/apstndb/spanner-mycli/enums"
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
		formatter = NewHTMLFormatter(out, sysVars.SkipColumnNames)
		return &StreamingProcessor{
			formatter:   formatter,
			out:         out,
			screenWidth: screenWidth,
		}

	case enums.DisplayModeXML:
		formatter = NewXMLFormatter(out, sysVars.SkipColumnNames)
		return &StreamingProcessor{
			formatter:   formatter,
			out:         out,
			screenWidth: screenWidth,
		}

	case enums.DisplayModeTable, enums.DisplayModeTableComment, enums.DisplayModeTableDetailComment:
		// Table formats use preview processor for width calculation
		// TablePreviewRows: positive = preview N rows, 0 = headers only, negative = all rows
		previewSize := int(sysVars.TablePreviewRows)
		if previewSize < 0 {
			// Negative means collect all rows (non-streaming)
			previewSize = 0 // 0 in TablePreviewProcessor means collect all
		}
		tableFormatter := NewTableStreamingFormatter(out, sysVars, screenWidth, previewSize)
		return NewTablePreviewProcessor(tableFormatter, previewSize)

	default:
		return nil
	}
}

// The HTML and XML formatters have been moved to formatters_html_xml_streaming.go
// to avoid duplication and use better implementations with standard library functions
// (html.EscapeString for HTML and proper XML encoding).