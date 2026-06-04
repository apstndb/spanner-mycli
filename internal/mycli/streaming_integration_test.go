package mycli

import (
	"testing"

	"github.com/apstndb/spanner-mycli/enums"
)

func TestShouldUseStreaming(t *testing.T) {
	t.Parallel()

	sysVars := &systemVariables{
		Query:   QueryVars{StreamingMode: enums.StreamingModeFalse},
		Display: DisplayVars{CLIFormat: enums.DisplayModeTable},
	}

	if shouldUseStreaming(sysVars) {
		t.Error("Streaming should be disabled for table output when StreamingMode is FALSE")
	}

	sysVars.Display.CLIFormat = enums.DisplayModeCSV
	if !shouldUseStreaming(sysVars) {
		t.Error("Streaming should stay enabled for non-table output when StreamingMode is FALSE")
	}

	sysVars.Query.StreamingMode = enums.StreamingModeTrue
	if !shouldUseStreaming(sysVars) {
		t.Error("Streaming should be enabled when StreamingMode is TRUE")
	}

	// Test AUTO mode behavior
	sysVars.Query.StreamingMode = enums.StreamingModeAuto
	sysVars.Display.CLIFormat = enums.DisplayModeTable
	if shouldUseStreaming(sysVars) {
		t.Error("AUTO mode should disable streaming for Table format")
	}

	sysVars.Display.CLIFormat = enums.DisplayModeCSV
	if !shouldUseStreaming(sysVars) {
		t.Error("AUTO mode should enable streaming for CSV format")
	}
}
