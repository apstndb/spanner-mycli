package main

import (
	"fmt"
	"io"
	"iter"
	"log/slog"
	"time"

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
// If metrics is non-nil, timing information will be collected.
func rowIterToSeq(
	rowIter *spanner.RowIterator,
	rowTransform func(*spanner.Row) (Row, error),
	metrics *ExecutionMetrics,
) *RowIterResult {
	result := &RowIterResult{}

	// Create an iterator sequence that handles metadata extraction
	result.Rows = func(yield func(Row) bool) {
		defer rowIter.Stop()

		firstIteration := true
		var rowCount int64

		// Process all rows using Do()
		err := rowIter.Do(func(row *spanner.Row) error {
			now := time.Now()

			// On first iteration, capture metadata and TTFB
			if firstIteration {
				firstIteration = false
				// Metadata should be available after the first call to Next() (which Do() calls internally)
				result.Metadata = rowIter.Metadata

				// Record TTFB if metrics collection is enabled
				if metrics != nil && metrics.FirstRowTime == nil {
					metrics.FirstRowTime = &now
					slog.Debug("Metadata retrieved (TTFB)",
						"fieldCount", len(result.Metadata.GetRowType().GetFields()),
						"ttfb", now.Sub(metrics.QueryStartTime))
				} else if result.Metadata != nil && result.Metadata.RowType != nil {
					fields := result.Metadata.RowType.GetFields()
					slog.Debug("Metadata retrieved",
						"fieldCount", len(fields),
						"firstRowTime", now.Format(time.RFC3339Nano))
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

			rowCount++
			if metrics != nil {
				metrics.LastRowTime = &now
				metrics.RowCount = rowCount
			}

			return nil
		})

		// Handle empty result set - metadata might still be available
		if firstIteration && err == nil {
			// No rows were processed, but Do() completed successfully
			result.Metadata = rowIter.Metadata
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
// If metrics is non-nil, timing information will be collected during processing.
func consumeRowIterWithProcessor(
	iter *spanner.RowIterator,
	processor RowProcessor,
	rowTransform func(*spanner.Row) (Row, error),
	sysVars *systemVariables,
	metrics *ExecutionMetrics,
) (queryStats map[string]interface{}, rowCount int64, metadata *sppb.ResultSetMetadata, queryPlan *sppb.QueryPlan, err error) {
	// Convert RowIterator to iter.Seq with metadata extraction and metrics
	result := rowIterToSeq(iter, rowTransform, metrics)

	// Process rows through the sequence
	rowCount = 0
	initialized := false
	for row := range result.Rows {
		// Initialize processor with metadata on first row
		if !initialized && result.Metadata != nil {
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
	}

	// Parse stats for processor
	parsedStats, _ := parseQueryStats(result.QueryStats)

	// Finish processing
	if err := processor.Finish(parsedStats, rowCount); err != nil {
		return result.QueryStats, rowCount, result.Metadata, result.QueryPlan, fmt.Errorf("failed to finish processing: %w", err)
	}

	return result.QueryStats, rowCount, result.Metadata, result.QueryPlan, nil
}

// errStopIteration is a sentinel error used to stop iteration.
var errStopIteration = fmt.Errorf("stop iteration")

// shouldUseStreaming determines whether to use streaming mode based on system variables and format.
// This delegates to createStreamingProcessor to maintain a single source of truth for the streaming decision logic.
func shouldUseStreaming(sysVars *systemVariables) bool {
	// Use a dummy writer to test if streaming would be enabled
	processor, _ := createStreamingProcessor(sysVars, io.Discard, 80)
	return processor != nil
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

// NewStreamingProcessorForMode creates a streaming processor for the given display mode.
// Returns nil if the mode doesn't support streaming yet.
// This is primarily used for testing - production code uses createStreamingProcessor.
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
