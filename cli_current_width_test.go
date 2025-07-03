package main

import (
	"bytes"
	"os"
	"strconv"
	"testing"
)

func TestCliCurrentWidthWithTee(t *testing.T) {
	// Test that CLI_CURRENT_WIDTH works correctly when --tee is enabled
	// and TeeManager is used
	
	t.Run("with TtyOutStream", func(t *testing.T) {
		// Setup TeeManager with a buffer for tee output
		teeManager := NewTeeManager(os.Stdout, os.Stderr)
		teeManager.SetTtyStream(os.Stdout)
		
		sysVars := &systemVariables{
			TeeManager:   teeManager,
			TtyOutStream: os.Stdout,     // This should be used for terminal size
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
	
	t.Run("without TtyOutStream and non-file stream", func(t *testing.T) {
		// Setup TeeManager with non-TTY output
		consoleBuf := &bytes.Buffer{}
		teeManager := NewTeeManager(consoleBuf, consoleBuf)
		
		sysVars := &systemVariables{
			TeeManager:   teeManager,
			TtyOutStream: nil,          // Not set
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