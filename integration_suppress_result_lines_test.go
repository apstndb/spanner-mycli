package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/apstndb/spanner-mycli/enums"
)

func TestSuppressResultLines(t *testing.T) {
	tests := []struct {
		name                string
		suppressResultLines bool
		verbose             bool
		interactive         bool
		expectedHasResult   bool
	}{
		{
			name:                "Normal mode - show result line",
			suppressResultLines: false,
			interactive:         true,
			expectedHasResult:   true,
		},
		{
			name:                "Suppress enabled - no result line",
			suppressResultLines: true,
			interactive:         true,
			expectedHasResult:   false,
		},
		{
			name:                "Batch mode without verbose - no result line",
			suppressResultLines: false,
			interactive:         false,
			verbose:             false,
			expectedHasResult:   false,
		},
		{
			name:                "Batch mode with verbose - show result line",
			suppressResultLines: false,
			interactive:         false,
			verbose:             true,
			expectedHasResult:   true,
		},
		{
			name:                "Batch mode with verbose but suppressed - no result line",
			suppressResultLines: true,
			interactive:         false,
			verbose:             true,
			expectedHasResult:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a simple result
			result := &Result{
				Rows: []Row{
					{"value1", "value2"},
					{"value3", "value4"},
				},
				TableHeader: toTableHeader("Column1", "Column2"),
				AffectedRows: 2,
				Stats: QueryStats{
					ElapsedTime: "0.5 sec",
				},
			}

			// Create system variables with test settings
			sysVars := newSystemVariablesWithDefaults()
			sysVars.SuppressResultLines = tt.suppressResultLines
			sysVars.Verbose = tt.verbose
			sysVars.CLIFormat = enums.DisplayModeTable

			// Capture output
			var buf bytes.Buffer
			err := printResult(&sysVars, 80, &buf, result, tt.interactive, "")
			if err != nil {
				t.Fatalf("printResult failed: %v", err)
			}

			output := buf.String()

			// Check if result line is present (look for "rows" or "sec")
			hasResultLine := strings.Contains(output, "rows") || strings.Contains(output, "sec")

			if hasResultLine != tt.expectedHasResult {
				t.Errorf("Result line presence mismatch: got %v, want %v\nOutput:\n%s", 
					hasResultLine, tt.expectedHasResult, output)
			}
		})
	}
}

func TestCLISuppressResultLinesSystemVariable(t *testing.T) {
	// Test that the system variable can be set and retrieved
	sysVars := newSystemVariablesWithDefaults()
	sysVars.ensureRegistry()

	// Test setting to TRUE
	err := sysVars.SetFromGoogleSQL("CLI_SUPPRESS_RESULT_LINES", "TRUE")
	if err != nil {
		t.Fatalf("Failed to set CLI_SUPPRESS_RESULT_LINES: %v", err)
	}

	if !sysVars.SuppressResultLines {
		t.Error("CLI_SUPPRESS_RESULT_LINES should be true after setting to TRUE")
	}

	// Test setting to FALSE
	err = sysVars.SetFromGoogleSQL("CLI_SUPPRESS_RESULT_LINES", "FALSE")
	if err != nil {
		t.Fatalf("Failed to set CLI_SUPPRESS_RESULT_LINES: %v", err)
	}

	if sysVars.SuppressResultLines {
		t.Error("CLI_SUPPRESS_RESULT_LINES should be false after setting to FALSE")
	}

	// Test GET
	values, err := sysVars.Get("CLI_SUPPRESS_RESULT_LINES")
	if err != nil {
		t.Fatalf("Failed to get CLI_SUPPRESS_RESULT_LINES: %v", err)
	}

	if values["CLI_SUPPRESS_RESULT_LINES"] != "FALSE" {
		t.Errorf("Expected CLI_SUPPRESS_RESULT_LINES to be FALSE, got %s", values["CLI_SUPPRESS_RESULT_LINES"])
	}
}