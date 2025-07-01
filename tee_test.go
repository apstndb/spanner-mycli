package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTeeOption(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir := t.TempDir()
	teeFile := filepath.Join(tmpDir, "output.log")

	// Test cases
	tests := []struct {
		name         string
		setupTee     bool
		expectedInTee bool
		content      string
	}{
		{
			name:         "output with tee enabled",
			setupTee:     true,
			expectedInTee: true,
			content:      "Hello, World!",
		},
		{
			name:         "output without tee",
			setupTee:     false,
			expectedInTee: false,
			content:      "This should not be in tee file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup output streams
			var outBuf bytes.Buffer
			var outStream io.Writer = &outBuf

			if tt.setupTee {
				// Open tee file
				f, err := os.OpenFile(teeFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND|os.O_TRUNC, 0644)
				if err != nil {
					t.Fatalf("Failed to open tee file: %v", err)
				}
				defer f.Close()
				outStream = io.MultiWriter(&outBuf, f)
			}

			// Write content
			_, err := outStream.Write([]byte(tt.content))
			if err != nil {
				t.Fatalf("Failed to write: %v", err)
			}

			// Check output buffer
			if !strings.Contains(outBuf.String(), tt.content) {
				t.Errorf("Expected output buffer to contain %q, got %q", tt.content, outBuf.String())
			}

			// Check tee file
			if tt.expectedInTee {
				teeContent, err := os.ReadFile(teeFile)
				if err != nil {
					t.Fatalf("Failed to read tee file: %v", err)
				}
				if !strings.Contains(string(teeContent), tt.content) {
					t.Errorf("Expected tee file to contain %q, got %q", tt.content, string(teeContent))
				}
			}

			// Clean up tee file for next test
			_ = os.Remove(teeFile)
		})
	}
}

func TestProgressMarkNotInTee(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir := t.TempDir()
	teeFile := filepath.Join(tmpDir, "output.log")

	// Open tee file
	f, err := os.OpenFile(teeFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatalf("Failed to open tee file: %v", err)
	}
	defer f.Close()

	// Create output streams
	var outBuf bytes.Buffer
	outStream := io.MultiWriter(&outBuf, f)
	var errBuf bytes.Buffer

	// Create system variables with TTY stream
	sysVars := &systemVariables{
		TtyOutStream:     os.Stdout,
		CurrentOutStream: outStream,
		CurrentErrStream: &errBuf,
		EnableProgressBar: true,
	}

	// Create CLI instance
	cli := &Cli{
		OutStream:       outStream,
		ErrStream:       &errBuf,
		SystemVariables: sysVars,
	}

	// Test progress mark - it should go to TtyOutStream, not to tee file
	stop := cli.PrintProgressingMark(nil)
	stop()

	// Check that tee file doesn't contain carriage return
	teeContent, err := os.ReadFile(teeFile)
	if err != nil {
		t.Fatalf("Failed to read tee file: %v", err)
	}
	
	if strings.Contains(string(teeContent), "\r") {
		t.Errorf("Tee file should not contain carriage return characters from progress marks")
	}
}

func TestReadlinePromptsNotInTee(t *testing.T) {
	// Skip this test if not in a TTY environment
	if _, err := os.Stdout.Stat(); err != nil {
		t.Skip("Not in a TTY environment")
	}
	
	// This test verifies that readline is configured to use TtyOutStream
	tmpDir := t.TempDir()
	teeFile := filepath.Join(tmpDir, "output.log")
	
	f, err := os.OpenFile(teeFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatalf("Failed to open tee file: %v", err)
	}
	defer f.Close()
	
	// Create output stream with tee
	var outBuf bytes.Buffer
	outStream := io.MultiWriter(&outBuf, f)
	
	
	// Create system variables with TTY stream
	sysVars := &systemVariables{
		TtyOutStream:     os.Stdout,  // In real usage, this is always os.Stdout
		CurrentOutStream: outStream,
		HistoryFile:      filepath.Join(tmpDir, "history"),
		Prompt:           "test> ",
		Prompt2:          "    > ",
	}
	
	// Create CLI instance
	cli := &Cli{
		OutStream:       outStream,
		InStream:        os.Stdin,
		ErrStream:       os.Stderr,
		SystemVariables: sysVars,
	}
	
	// Try to initialize multiline editor
	// This may fail in non-TTY environments, which is expected
	ed, _, err := initializeMultilineEditor(cli)
	if err != nil {
		// In CI or non-TTY environments, this is expected
		t.Logf("Failed to initialize editor (expected in non-TTY environment): %v", err)
		return
	}
	
	// Verify that LineEditor.Writer is set to TtyOutStream
	if ed.LineEditor.Writer != sysVars.TtyOutStream {
		t.Errorf("Expected LineEditor.Writer to be TtyOutStream, got %v", ed.LineEditor.Writer)
	}
}

func TestGetTerminalSizeWithTee(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir := t.TempDir()
	teeFile := filepath.Join(tmpDir, "output.log")

	// Open tee file
	f, err := os.OpenFile(teeFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatalf("Failed to open tee file: %v", err)
	}
	defer f.Close()

	// Create output stream with MultiWriter
	outStream := io.MultiWriter(os.Stdout, f)

	// Create system variables with TTY stream
	sysVars := &systemVariables{
		TtyOutStream:     os.Stdout,
		CurrentOutStream: outStream,
	}

	// Create CLI instance
	cli := &Cli{
		OutStream:       outStream,
		SystemVariables: sysVars,
	}

	// Test GetTerminalSizeWithTty - should succeed even with MultiWriter
	// because it uses TtyOutStream
	_, err = cli.GetTerminalSizeWithTty(outStream)
	if err != nil {
		// This might fail in CI environment without TTY, which is expected
		t.Logf("GetTerminalSizeWithTty failed (expected in non-TTY environment): %v", err)
	}

	// Test regular GetTerminalSize with MultiWriter - should fail
	_, err = GetTerminalSize(outStream)
	if err == nil {
		t.Error("Expected GetTerminalSize to fail with MultiWriter")
	}
}