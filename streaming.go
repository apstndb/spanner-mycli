package main

import (
	"fmt"
	"iter"
	"log/slog"

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

		var firstIteration = true
		
		// Process all rows using Do()
		err := rowIter.Do(func(row *spanner.Row) error {
			// On first iteration, capture metadata
			if firstIteration {
				firstIteration = false
				// Metadata should be available after the first call to Next() (which Do() calls internally)
				result.Metadata = rowIter.Metadata
				if result.Metadata == nil {
					slog.Debug("rowIter.Metadata is nil after first row in Do()")
				} else if result.Metadata.RowType == nil {
					slog.Debug("Metadata.RowType is nil")
				} else {
					fields := result.Metadata.RowType.GetFields()
					slog.Debug("Metadata retrieved", "fieldCount", len(fields))
					for i, field := range fields {
						slog.Debug("Field info", "index", i, "name", field.GetName())
					}
				}
			}
			
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

		// Handle empty result set - metadata might still be available
		if firstIteration && err == nil {
			// No rows were processed, but Do() completed successfully
			result.Metadata = rowIter.Metadata
			slog.Debug("Empty result set in Do()", "metadataAvailable", result.Metadata != nil)
		}

		// Handle errors
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
	initialized := false
	for row := range result.Rows {
		// Initialize processor with metadata on first row
		if !initialized && result.Metadata != nil {
			slog.Debug("Initializing processor with metadata", 
				"fieldCount", len(result.Metadata.GetRowType().GetFields()))
			if err := processor.Init(result.Metadata, sysVars); err != nil {
				return nil, 0, result.Metadata, nil, fmt.Errorf("failed to initialize processor: %w", err)
			}
			initialized = true
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
	if !initialized && result.Metadata != nil {
		if err := processor.Init(result.Metadata, sysVars); err != nil {
			return nil, 0, result.Metadata, nil, fmt.Errorf("failed to initialize processor: %w", err)
		}
		initialized = true
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
	// Check if streaming is enabled globally
	if !sysVars.StreamingEnabled {
		return false
	}

	// Check if the current format supports streaming
	return isStreamingSupported(sysVars.CLIFormat)
}

// isStreamingSupported checks if a specific display mode supports streaming.
func isStreamingSupported(mode enums.DisplayMode) bool {
	switch mode {
	case enums.DisplayModeCSV,
		enums.DisplayModeTab,
		enums.DisplayModeVertical,
		enums.DisplayModeHTML,
		enums.DisplayModeXML:
		// These formats support streaming
		return true
	case enums.DisplayModeTable,
		enums.DisplayModeTableComment,
		enums.DisplayModeTableDetailComment:
		// Table formats support streaming with preview
		return true
	default:
		return false
	}
}
