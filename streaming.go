package main

import (
	"fmt"
	"iter"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/enums"
)

// RowIterResult contains metadata and a row iterator sequence.
// The metadata is available immediately after the first row is fetched.
type RowIterResult struct {
	Metadata   *sppb.ResultSetMetadata
	Rows       iter.Seq[Row]
	QueryStats map[string]interface{}
	RowCount   int64
	QueryPlan  *sppb.QueryPlan
	Error      error
}

// rowIterToSeq converts a spanner.RowIterator to an iter.Seq[Row] with metadata extraction.
// It fetches the first row to obtain metadata, then yields all rows including the first.
// This unified approach allows both streaming and buffered processing through the same interface.
func rowIterToSeq(
	rowIter *spanner.RowIterator,
	rowTransform func(*spanner.Row) (Row, error),
) *RowIterResult {
	result := &RowIterResult{}
	
	// Create an iterator sequence that handles metadata extraction
	result.Rows = func(yield func(Row) bool) {
		defer rowIter.Stop()
		
		// First row handling - needed to get metadata
		var firstRowFetched bool
		var firstRow Row
		
		// Try to fetch the first row to get metadata
		err := rowIter.Do(func(row *spanner.Row) error {
			transformedRow, err := rowTransform(row)
			if err != nil {
				result.Error = fmt.Errorf("failed to transform first row: %w", err)
				return err
			}
			firstRow = transformedRow
			firstRowFetched = true
			// Stop after first row to extract metadata
			return errStopIteration
		})
		
		// Handle errors from first row fetch
		if err != nil && err != errStopIteration {
			result.Error = fmt.Errorf("failed to fetch first row: %w", err)
			return
		}
		
		// Metadata is now available
		result.Metadata = rowIter.Metadata
		
		// Yield the first row if it exists
		if firstRowFetched {
			if !yield(firstRow) {
				// Consumer stopped iteration
				return
			}
		}
		
		// Process remaining rows
		err = rowIter.Do(func(row *spanner.Row) error {
			transformedRow, err := rowTransform(row)
			if err != nil {
				return fmt.Errorf("failed to transform row: %w", err)
			}
			
			// Yield the row; stop if consumer returns false
			if !yield(transformedRow) {
				return errStopIteration
			}
			return nil
		})
		
		// Handle errors from remaining rows
		if err != nil && err != errStopIteration {
			result.Error = fmt.Errorf("failed to process rows: %w", err)
			return
		}
		
		// Capture final statistics
		result.QueryStats = rowIter.QueryStats
		result.RowCount = rowIter.RowCount
		result.QueryPlan = rowIter.QueryPlan
	}
	
	return result
}

// consumeRowIterWithProcessor processes rows using the provided RowProcessor.
// This unified function handles both buffered and streaming modes through the same interface.
func consumeRowIterWithProcessor(
	iter *spanner.RowIterator,
	processor RowProcessor,
	rowTransform func(*spanner.Row) (Row, error),
	sysVars *systemVariables,
) (queryStats map[string]interface{}, rowCount int64, metadata *sppb.ResultSetMetadata, queryPlan *sppb.QueryPlan, err error) {
	// Convert RowIterator to iter.Seq with metadata extraction
	result := rowIterToSeq(iter, rowTransform)
	
	// Process rows through the sequence
	rowCount = 0
	for row := range result.Rows {
		// Initialize processor with metadata on first row
		if rowCount == 0 && result.Metadata != nil {
			if err := processor.Init(result.Metadata, sysVars); err != nil {
				return nil, 0, result.Metadata, nil, fmt.Errorf("failed to initialize processor: %w", err)
			}
		}
		
		// Process the row
		if err := processor.ProcessRow(row); err != nil {
			return nil, rowCount, result.Metadata, nil, fmt.Errorf("failed to process row %d: %w", rowCount+1, err)
		}
		rowCount++
	}
	
	// Check for errors during iteration
	if result.Error != nil {
		return nil, rowCount, result.Metadata, nil, result.Error
	}
	
	// Initialize processor if no rows (metadata might still be available)
	if rowCount == 0 && result.Metadata != nil {
		if err := processor.Init(result.Metadata, sysVars); err != nil {
			return nil, 0, result.Metadata, nil, fmt.Errorf("failed to initialize processor: %w", err)
		}
	}
	
	// Parse stats for processor
	parsedStats, _ := parseQueryStats(result.QueryStats)
	
	// Finish processing
	if err := processor.Finish(parsedStats, result.RowCount); err != nil {
		return result.QueryStats, result.RowCount, result.Metadata, result.QueryPlan, fmt.Errorf("failed to finish processing: %w", err)
	}
	
	return result.QueryStats, result.RowCount, result.Metadata, result.QueryPlan, nil
}

// errStopIteration is a sentinel error used to stop iteration.
var errStopIteration = fmt.Errorf("stop iteration")

// shouldUseStreaming determines whether to use streaming mode based on system variables and format.
func shouldUseStreaming(sysVars *systemVariables) bool {
	// For now, return false until streaming formatters are implemented
	// This will be updated as we implement streaming support for each format
	return false
}

// isStreamingSupported checks if a specific display mode supports streaming.
func isStreamingSupported(mode enums.DisplayMode) bool {
	// Will be updated as we implement streaming for each format
	switch mode {
	case enums.DisplayModeCSV,
		enums.DisplayModeTab,
		enums.DisplayModeVertical,
		enums.DisplayModeHTML,
		enums.DisplayModeXML:
		// These formats can support streaming
		return false // Will change to true as implemented
	case enums.DisplayModeTable,
		enums.DisplayModeTableComment,
		enums.DisplayModeTableDetailComment:
		// Table formats need special handling with tablewriter streaming
		return false // Will be implemented with tablewriter v1.0.9 streaming
	default:
		return false
	}
}