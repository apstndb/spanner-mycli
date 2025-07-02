package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	
	"golang.org/x/term"
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
	t.Run("with TTY", func(t *testing.T) {
		// Skip if stdout is not a terminal
		if !term.IsTerminal(int(os.Stdout.Fd())) {
			t.Skip("stdout is not a terminal")
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
			TtyOutStream:     os.Stdout,  // Should be set when stdout is a TTY
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
		
		// Initialize multiline editor
		ed, _, err := initializeMultilineEditor(cli)
		if err != nil {
			t.Fatalf("Failed to initialize editor: %v", err)
		}
		
		// Verify that LineEditor.Writer is set to TtyOutStream
		if ed.LineEditor.Writer != sysVars.TtyOutStream {
			t.Errorf("Expected LineEditor.Writer to be TtyOutStream, got %v", ed.LineEditor.Writer)
		}
	})
	
	t.Run("without TTY", func(t *testing.T) {
		// This test verifies that interactive mode fails when no TTY is available
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
		
		// Create system variables WITHOUT TTY stream
		sysVars := &systemVariables{
			TtyOutStream:     nil,  // No TTY available
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
		
		// Try to initialize multiline editor - should fail
		_, _, err = initializeMultilineEditor(cli)
		if err == nil {
			t.Error("Expected error when initializing editor without TTY")
		}
		if !strings.Contains(err.Error(), "stdout is not a terminal") {
			t.Errorf("Expected error about stdout not being a terminal, got: %v", err)
		}
	})
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

func TestSafeTeeWriter(t *testing.T) {
	t.Run("normal write", func(t *testing.T) {
		tmpDir := t.TempDir()
		teeFile := filepath.Join(tmpDir, "output.log")
		
		f, err := os.OpenFile(teeFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			t.Fatalf("Failed to open file: %v", err)
		}
		defer f.Close()
		
		var errBuf bytes.Buffer
		writer := &safeTeeWriter{
			file:      f,
			errStream: &errBuf,
			hasWarned: false,
		}
		
		// Normal write should succeed
		data := []byte("Hello, World!\n")
		n, err := writer.Write(data)
		if err != nil {
			t.Errorf("Write should not return error: %v", err)
		}
		if n != len(data) {
			t.Errorf("Write returned %d, expected %d", n, len(data))
		}
		
		// No warning should be printed
		if errBuf.Len() > 0 {
			t.Errorf("No warning expected, got: %s", errBuf.String())
		}
		
		// Verify data was written
		content, _ := os.ReadFile(teeFile)
		if string(content) != string(data) {
			t.Errorf("File content mismatch: got %q, want %q", content, data)
		}
	})
	
	t.Run("write error handling", func(t *testing.T) {
		// Create a file that we'll close to simulate write error
		tmpDir := t.TempDir()
		teeFile := filepath.Join(tmpDir, "output.log")
		
		f, err := os.OpenFile(teeFile, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			t.Fatalf("Failed to open file: %v", err)
		}
		f.Close() // Close to simulate write error
		
		var errBuf bytes.Buffer
		writer := &safeTeeWriter{
			file:      f,
			errStream: &errBuf,
			hasWarned: false,
		}
		
		// First write should fail but return success
		data1 := []byte("First write\n")
		n, err := writer.Write(data1)
		if err != nil {
			t.Errorf("Write should not return error even on failure: %v", err)
		}
		if n != len(data1) {
			t.Errorf("Write should return full length even on failure: got %d, want %d", n, len(data1))
		}
		
		// Warning should be printed
		warnings := errBuf.String()
		if !strings.Contains(warnings, "WARNING: Failed to write to tee file") {
			t.Errorf("Expected warning about write failure, got: %s", warnings)
		}
		if !strings.Contains(warnings, "WARNING: Tee logging disabled") {
			t.Errorf("Expected warning about tee being disabled, got: %s", warnings)
		}
		
		// Clear error buffer
		errBuf.Reset()
		
		// Second write should be discarded without warning
		data2 := []byte("Second write\n")
		n, err = writer.Write(data2)
		if err != nil {
			t.Errorf("Second write should not return error: %v", err)
		}
		if n != len(data2) {
			t.Errorf("Second write should return full length: got %d, want %d", n, len(data2))
		}
		
		// No additional warning should be printed
		if errBuf.Len() > 0 {
			t.Errorf("No additional warning expected, got: %s", errBuf.String())
		}
	})
}

func TestTeeNonRegularFile(t *testing.T) {
	t.Run("named pipe detection", func(t *testing.T) {
		// Skip if we can't create named pipes on this system
		if runtime.GOOS == "windows" {
			t.Skip("Named pipes not supported on Windows")
		}
		
		tmpDir := t.TempDir()
		pipePath := filepath.Join(tmpDir, "test.fifo")
		
		// Create a named pipe
		err := syscall.Mkfifo(pipePath, 0644)
		if err != nil {
			t.Skipf("Cannot create named pipe: %v", err)
		}
		
		// Verify the file exists and is not regular
		fi, err := os.Stat(pipePath)
		if err != nil {
			t.Fatalf("Failed to stat pipe: %v", err)
		}
		if fi.Mode().IsRegular() {
			t.Fatal("Expected pipe to not be a regular file")
		}
		
		// The actual validation would happen in main.go run() function
		// Here we just test the detection logic
		if !fi.Mode().IsRegular() {
			// This is the expected case - pipe is not a regular file
			t.Logf("Correctly detected non-regular file: %s", pipePath)
		}
	})
	
	t.Run("directory detection", func(t *testing.T) {
		tmpDir := t.TempDir()
		
		// Try to use a directory as tee file
		fi, err := os.Stat(tmpDir)
		if err != nil {
			t.Fatalf("Failed to stat directory: %v", err)
		}
		
		if fi.Mode().IsRegular() {
			t.Fatal("Expected directory to not be a regular file")
		}
		
		// The actual validation would happen in main.go run() function
		// Here we just test the detection logic
		if !fi.Mode().IsRegular() {
			// This is the expected case - directory is not a regular file
			t.Logf("Correctly detected non-regular file: %s", tmpDir)
		}
	})
}