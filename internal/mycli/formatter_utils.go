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
	render := queryRenderingFrom(sysVars)
	render.CLIFormat = mode
	render.Export.CLIFormat = mode
	return streamingProcessorForMode(render, out, screenWidth)
}

func streamingProcessorForMode(render queryRendering, out io.Writer, screenWidth int) (RowProcessor, error) {
	config := render.Formatter

	// Convert enums.DisplayMode to format.Mode
	fmtMode := format.Mode(render.CLIFormat.String())

	// Special handling for table formats with preview (need screenWidth)
	if fmtMode.IsTableMode() {
		previewSize := int(config.PreviewRows)
		if previewSize < 0 {
			previewSize = 0 // 0 means headers-only preview (stream all rows)
		}
		tableFormatter := format.NewTableStreamingFormatter(out, config, screenWidth, previewSize, fmtMode)
		return NewTablePreviewProcessor(tableFormatter, previewSize), nil
	}

	// For non-table formats, use unified creation
	formatter, err := format.NewStreamingFormatter(fmtMode, out, config)
	if err != nil {
		return nil, err
	}
	return NewStreamingProcessor(formatter, out, screenWidth), nil
}
