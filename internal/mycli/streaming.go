package mycli

import (
	"io"

	"github.com/apstndb/spanner-mycli/enums"
)

// NewStreamingProcessorForMode creates a streaming processor for the given display mode.
// Returns nil if the mode doesn't support streaming yet.
// This is primarily used for testing - production code uses createStreamingProcessor.
func NewStreamingProcessorForMode(mode enums.DisplayMode, out io.Writer, sysVars *systemVariables, screenWidth int) RowProcessor {
	// Use the shared implementation that avoids duplication
	processor, err := createStreamingProcessorForMode(mode, out, sysVars, screenWidth)
	if err != nil {
		return nil
	}
	return processor
}
