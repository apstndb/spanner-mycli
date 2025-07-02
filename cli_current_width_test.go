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
	// and CurrentOutStream is an io.MultiWriter
	
	t.Run("with TtyOutStream", func(t *testing.T) {
		// Create a buffer to simulate tee file
		var teeBuf bytes.Buffer
		
		// Simulate --tee setup with MultiWriter
		multiWriter := io.MultiWriter(os.Stdout, &teeBuf)
		
		sysVars := &systemVariables{
			CurrentOutStream: multiWriter,  // This is an io.MultiWriter, not *os.File
			TtyOutStream:     os.Stdout,     // This should be used for terminal size
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
		// Create a buffer to simulate tee file
		var teeBuf bytes.Buffer
		
		// Simulate --tee setup with MultiWriter
		multiWriter := io.MultiWriter(&bytes.Buffer{}, &teeBuf)
		
		sysVars := &systemVariables{
			CurrentOutStream: multiWriter,  // This is an io.MultiWriter, not *os.File
			TtyOutStream:     nil,          // Not set
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