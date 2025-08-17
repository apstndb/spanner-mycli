package main

import (
	"fmt"
	"io"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
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

	// Try to get complete field information with types from TableHeader
	var metadata *sppb.ResultSetMetadata
	if result.TableHeader != nil {
		if fields, ok := result.TableHeader.structFields(); ok {
			// Type information is available - use complete metadata
			metadata = &sppb.ResultSetMetadata{
				RowType: &sppb.StructType{
					Fields: fields,
				},
			}
		}
		// If ok is false, metadata remains nil (for simpleTableHeader)
		// This matches the actual usage pattern where formatters pass nil for metadata
	}

	if err := formatter.InitFormat(columnNames, metadata, sysVars, nil); err != nil {
		return err
	}

	// Write all rows
	for _, row := range result.Rows {
		if err := formatter.WriteRow(row); err != nil {
			return err
		}
	}

	// Finish formatting
	return formatter.FinishFormat(QueryStats{}, int64(len(result.Rows)))
}
