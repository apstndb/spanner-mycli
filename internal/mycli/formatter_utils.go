package mycli

import (
	"io"

	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanner-mycli/internal/mycli/format"
)

// createStreamingProcessorForMode creates a streaming processor for the given display mode.
// This is the single source of truth for streaming processor creation logic,
// used by both execute_sql.go and streaming.go to avoid duplication.
func createStreamingProcessorForMode(mode enums.DisplayMode, out io.Writer, sysVars *systemVariables, screenWidth int) (RowProcessor, error) {
	config := sysVars.toFormatConfig()

	// Special handling for table formats with preview (need screenWidth)
	if mode == enums.DisplayModeTable || mode == enums.DisplayModeTableComment || mode == enums.DisplayModeTableDetailComment {
		previewSize := int(sysVars.TablePreviewRows)
		if previewSize < 0 {
			previewSize = 0 // 0 means headers-only preview (stream all rows)
		}
		tableFormatter := format.NewTableStreamingFormatter(out, config, screenWidth, previewSize)
		return NewTablePreviewProcessor(tableFormatter, previewSize), nil
	}

	// For non-table formats, use unified creation
	formatter, err := format.NewStreamingFormatter(mode, out, config)
	if err != nil {
		return nil, err
	}
	return NewStreamingProcessor(formatter, out, screenWidth), nil
}
