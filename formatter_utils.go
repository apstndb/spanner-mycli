package main

import (
	"fmt"
	"io"

	"github.com/apstndb/spanner-mycli/enums"
)

// createStreamingFormatter creates a streaming formatter for the given display mode.
// This is the single source of truth for formatter creation logic.
// Note: Table formats (Table, TableComment, TableDetailComment) require screenWidth
// and should be created with NewTableStreamingFormatter directly by the caller.
func createStreamingFormatter(mode enums.DisplayMode, out io.Writer, sysVars *systemVariables) (StreamingFormatter, error) {
	switch mode {
	case enums.DisplayModeCSV:
		return NewCSVFormatter(out, sysVars.SkipColumnNames), nil
	case enums.DisplayModeTab:
		return NewTabFormatter(out, sysVars.SkipColumnNames), nil
	case enums.DisplayModeVertical:
		return NewVerticalFormatter(out), nil
	case enums.DisplayModeHTML:
		return NewHTMLFormatter(out, sysVars.SkipColumnNames), nil
	case enums.DisplayModeXML:
		return NewXMLFormatter(out, sysVars.SkipColumnNames), nil
	case enums.DisplayModeSQLInsert, enums.DisplayModeSQLInsertOrIgnore, enums.DisplayModeSQLInsertOrUpdate:
		return NewSQLStreamingFormatter(out, sysVars, mode)
	case enums.DisplayModeTable, enums.DisplayModeTableComment, enums.DisplayModeTableDetailComment:
		// Table formats need screenWidth, so they must be created by the caller
		// Return a dummy formatter for isStreamingSupported check
		if out == io.Discard {
			// This is just for checking support
			return NewTableStreamingFormatter(out, sysVars, 0, 0), nil
		}
		return nil, fmt.Errorf("table formats require screenWidth - use NewTableStreamingFormatter directly")
	default:
		return nil, fmt.Errorf("unsupported streaming format: %v", mode)
	}
}

// executeWithFormatter executes buffered formatting using a streaming formatter.
// This reduces duplication in formatCSV, formatTab, formatVertical, etc.
func executeWithFormatter(formatter StreamingFormatter, result *Result, columnNames []string, sysVars *systemVariables) error {
	if len(columnNames) == 0 {
		return nil
	}

	// Pass TableHeader directly - formatters can extract what they need
	if err := formatter.InitFormat(result.TableHeader, sysVars, nil); err != nil {
		return err
	}

	// Write all rows
	for i, row := range result.Rows {
		if err := formatter.WriteRow(row); err != nil {
			return fmt.Errorf("failed to write row %d: %w", i+1, err)
		}
	}

	// Finish formatting
	return formatter.FinishFormat(QueryStats{}, int64(len(result.Rows)))
}

// createStreamingProcessorForMode creates a streaming processor for the given display mode.
// This is the single source of truth for streaming processor creation logic,
// used by both execute_sql.go and streaming.go to avoid duplication.
func createStreamingProcessorForMode(mode enums.DisplayMode, out io.Writer, sysVars *systemVariables, screenWidth int) (RowProcessor, error) {
	// Special handling for table formats with preview (need screenWidth)
	if mode == enums.DisplayModeTable || mode == enums.DisplayModeTableComment || mode == enums.DisplayModeTableDetailComment {
		previewSize := int(sysVars.TablePreviewRows)
		if previewSize < 0 {
			previewSize = 0 // 0 means headers-only preview (stream all rows)
		}
		tableFormatter := NewTableStreamingFormatter(out, sysVars, screenWidth, previewSize)
		return NewTablePreviewProcessor(tableFormatter, previewSize), nil
	}

	// For non-table formats, use unified creation
	formatter, err := createStreamingFormatter(mode, out, sysVars)
	if err != nil {
		return nil, err
	}
	return NewStreamingProcessor(formatter, out, screenWidth), nil
}
