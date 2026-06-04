package mycli

import (
	"io"

	"github.com/apstndb/spanner-mycli/enums"
)

// shouldUseStreaming determines whether to use streaming mode based on system variables and format.
// This delegates to createStreamingProcessor to maintain a single source of truth for the streaming decision logic.
func shouldUseStreaming(sysVars *systemVariables) bool {
	// Use a dummy writer to test if streaming would be enabled
	processor, err := createStreamingProcessor(sysVars, io.Discard, 80)
	if err != nil {
		return false
	}
	return processor != nil
}

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
