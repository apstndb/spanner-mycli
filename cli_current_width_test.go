package main

import (
	"bytes"
	"io"
	"os"
	"strconv"
	"testing"
)

func TestCliCurrentWidthWithTee(t *testing.T) {
	// Test that CLI_CURRENT_WIDTH works correctly when --tee is enabled
	// and StreamManager is used

	t.Run("with TtyStream in StreamManager", func(t *testing.T) {
		// Setup StreamManager with a buffer for tee output
		teeManager := NewStreamManager(os.Stdin, os.Stdout, os.Stderr)
		teeManager.SetTtyStream(os.Stdout)

		sysVars := &systemVariables{
			StreamManager: teeManager,
		}

		// Get the accessor for CLI_CURRENT_WIDTH
		metadata := systemVariableDefMap["CLI_CURRENT_WIDTH"]
		getter := metadata.Accessor.Getter

		// Call the getter
		result, err := getter(sysVars, "CLI_CURRENT_WIDTH")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// In CI environment without TTY, this might return "NULL"
		// But we should not get a panic or type assertion error
		value := result["CLI_CURRENT_WIDTH"]
		if value != "NULL" {
			// If we got a value, it should be a valid integer
			if _, err := strconv.Atoi(value); err != nil {
				t.Errorf("Expected numeric width or NULL, got: %s", value)
			}
		}
	})

	t.Run("without TtyStream and non-file stream", func(t *testing.T) {
		// Setup StreamManager with non-TTY output
		consoleBuf := &bytes.Buffer{}
		teeManager := NewStreamManager(io.NopCloser(bytes.NewReader(nil)), consoleBuf, consoleBuf)
		// Do not set TTY stream

		sysVars := &systemVariables{
			StreamManager: teeManager,
		}

		// Get the accessor for CLI_CURRENT_WIDTH
		metadata := systemVariableDefMap["CLI_CURRENT_WIDTH"]
		getter := metadata.Accessor.Getter

		// Call the getter
		result, err := getter(sysVars, "CLI_CURRENT_WIDTH")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// Should return NULL when no terminal is available
		value := result["CLI_CURRENT_WIDTH"]
		if value != "NULL" {
			t.Errorf("Expected NULL when no terminal available, got: %s", value)
		}
	})
}
