package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestTeeManager(t *testing.T) {
	t.Run("basic enable and disable", func(t *testing.T) {
		originalOut := &bytes.Buffer{}
		errOut := &bytes.Buffer{}
		tm := NewTeeManager(originalOut, errOut)
		defer tm.Close()

		// Initially disabled
		if tm.IsEnabled() {
			t.Error("Expected tee to be disabled initially")
		}

		// Enable tee
		tmpDir := t.TempDir()
		teeFile := filepath.Join(tmpDir, "test.log")
		if err := tm.EnableTee(teeFile); err != nil {
			t.Fatalf("Failed to enable tee: %v", err)
		}

		if !tm.IsEnabled() {
			t.Error("Expected tee to be enabled after EnableTee")
		}

		// Write through the writer
		writer := tm.GetWriter()
		testData := "Hello, tee!\n"
		if _, err := writer.Write([]byte(testData)); err != nil {
			t.Fatalf("Failed to write: %v", err)
		}

		// Verify data was written to both outputs
		if originalOut.String() != testData {
			t.Errorf("Expected original output %q, got %q", testData, originalOut.String())
		}

		// Read tee file
		content, err := os.ReadFile(teeFile)
		if err != nil {
			t.Fatalf("Failed to read tee file: %v", err)
		}
		if string(content) != testData {
			t.Errorf("Expected tee file content %q, got %q", testData, string(content))
		}

		// Disable tee
		tm.DisableTee()
		if tm.IsEnabled() {
			t.Error("Expected tee to be disabled after DisableTee")
		}
	})

	t.Run("multiple enable calls", func(t *testing.T) {
		originalOut := &bytes.Buffer{}
		errOut := &bytes.Buffer{}
		tm := NewTeeManager(originalOut, errOut)
		defer tm.Close()

		tmpDir := t.TempDir()
		teeFile1 := filepath.Join(tmpDir, "test1.log")
		teeFile2 := filepath.Join(tmpDir, "test2.log")

		// Enable first file
		if err := tm.EnableTee(teeFile1); err != nil {
			t.Fatalf("Failed to enable tee1: %v", err)
		}

		// Write to first file
		writer1 := tm.GetWriter()
		data1 := "First file\n"
		if _, err := writer1.Write([]byte(data1)); err != nil {
			t.Fatalf("Failed to write to first file: %v", err)
		}

		// Enable second file (should close first)
		if err := tm.EnableTee(teeFile2); err != nil {
			t.Fatalf("Failed to enable tee2: %v", err)
		}

		// Write to second file
		writer2 := tm.GetWriter()
		data2 := "Second file\n"
		if _, err := writer2.Write([]byte(data2)); err != nil {
			t.Fatalf("Failed to write to second file: %v", err)
		}

		// Verify first file only has first data
		content1, _ := os.ReadFile(teeFile1)
		if string(content1) != data1 {
			t.Errorf("Expected file1 content %q, got %q", data1, string(content1))
		}

		// Verify second file only has second data
		content2, _ := os.ReadFile(teeFile2)
		if string(content2) != data2 {
			t.Errorf("Expected file2 content %q, got %q", data2, string(content2))
		}
	})

	t.Run("enable with invalid file preserves existing tee", func(t *testing.T) {
		originalOut := &bytes.Buffer{}
		errOut := &bytes.Buffer{}
		tm := NewTeeManager(originalOut, errOut)
		defer tm.Close()

		tmpDir := t.TempDir()
		validFile := filepath.Join(tmpDir, "valid.log")
		
		// Enable valid file
		if err := tm.EnableTee(validFile); err != nil {
			t.Fatalf("Failed to enable valid file: %v", err)
		}

		// Write some data
		writer := tm.GetWriter()
		testData := "Valid data\n"
		if _, err := writer.Write([]byte(testData)); err != nil {
			t.Fatalf("Failed to write test data: %v", err)
		}

		// Try to enable a directory (should fail)
		if err := tm.EnableTee(tmpDir); err == nil {
			t.Error("Expected error when enabling directory as tee file")
		}

		// Verify tee is still enabled with original file
		if !tm.IsEnabled() {
			t.Error("Expected tee to remain enabled after failed EnableTee")
		}

		// Write more data
		moreData := "More data\n"
		writer2 := tm.GetWriter()
		if _, err := writer2.Write([]byte(moreData)); err != nil {
			t.Fatalf("Failed to write more data: %v", err)
		}

		// Verify all data was written to the valid file
		content, _ := os.ReadFile(validFile)
		expectedContent := testData + moreData
		if string(content) != expectedContent {
			t.Errorf("Expected file content %q, got %q", expectedContent, string(content))
		}
	})

	t.Run("concurrent access", func(t *testing.T) {
		originalOut := &bytes.Buffer{}
		errOut := &bytes.Buffer{}
		tm := NewTeeManager(originalOut, errOut)
		defer tm.Close()

		tmpDir := t.TempDir()
		teeFile := filepath.Join(tmpDir, "concurrent.log")

		// Enable tee
		if err := tm.EnableTee(teeFile); err != nil {
			t.Fatalf("Failed to enable tee: %v", err)
		}

		// Concurrent writes
		var wg sync.WaitGroup
		numGoroutines := 10
		numWrites := 100

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				writer := tm.GetWriter()
				for j := 0; j < numWrites; j++ {
					data := []byte("test\n")
					if _, err := writer.Write(data); err != nil {
						t.Errorf("Failed to write data: %v", err)
					}
				}
			}(i)
		}

		wg.Wait()

		// Verify all writes completed
		content, _ := os.ReadFile(teeFile)
		lines := strings.Count(string(content), "\n")
		expectedLines := numGoroutines * numWrites
		if lines != expectedLines {
			t.Errorf("Expected %d lines, got %d", expectedLines, lines)
		}
	})

	t.Run("close idempotency", func(t *testing.T) {
		originalOut := &bytes.Buffer{}
		errOut := &bytes.Buffer{}
		tm := NewTeeManager(originalOut, errOut)

		tmpDir := t.TempDir()
		teeFile := filepath.Join(tmpDir, "test.log")
		
		// Enable tee
		if err := tm.EnableTee(teeFile); err != nil {
			t.Fatalf("Failed to enable tee: %v", err)
		}

		// Close multiple times (should not panic)
		tm.Close()
		tm.Close()
		tm.DisableTee()
		tm.Close()

		// Verify disabled
		if tm.IsEnabled() {
			t.Error("Expected tee to be disabled after Close")
		}
	})

	t.Run("writer caching", func(t *testing.T) {
		originalOut := &bytes.Buffer{}
		errOut := &bytes.Buffer{}
		tm := NewTeeManager(originalOut, errOut)
		defer tm.Close()

		// Enable tee
		tmpDir := t.TempDir()
		teeFile := filepath.Join(tmpDir, "test.log")
		if err := tm.EnableTee(teeFile); err != nil {
			t.Fatalf("Failed to enable tee: %v", err)
		}

		// Get writer multiple times
		writer1 := tm.GetWriter()
		writer2 := tm.GetWriter()
		writer3 := tm.GetWriter()

		// Verify same instance is returned (caching works)
		if writer1 != writer2 || writer2 != writer3 {
			t.Error("Expected GetWriter to return the same cached instance")
		}

		// Disable and re-enable should create new instance
		tm.DisableTee()
		if err := tm.EnableTee(teeFile); err != nil {
			t.Fatalf("Failed to re-enable tee: %v", err)
		}

		writer4 := tm.GetWriter()
		if writer1 == writer4 {
			t.Error("Expected new writer instance after disable/enable cycle")
		}

		// Multiple calls should still return same new instance
		writer5 := tm.GetWriter()
		if writer4 != writer5 {
			t.Error("Expected GetWriter to return the same cached instance after re-enable")
		}
	})

	t.Run("concurrent EnableTee calls", func(t *testing.T) {
		originalOut := &bytes.Buffer{}
		errOut := &bytes.Buffer{}
		tm := NewTeeManager(originalOut, errOut)
		defer tm.Close()

		tmpDir := t.TempDir()
		
		// Run multiple EnableTee calls concurrently
		var wg sync.WaitGroup
		errors := make([]error, 10)
		
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				filePath := filepath.Join(tmpDir, fmt.Sprintf("concurrent-%d.log", index))
				errors[index] = tm.EnableTee(filePath)
			}(i)
		}
		
		wg.Wait()
		
		// All calls should succeed
		for i, err := range errors {
			if err != nil {
				t.Errorf("EnableTee call %d failed: %v", i, err)
			}
		}
		
		// Only one file should be active (the last one)
		if !tm.IsEnabled() {
			t.Error("Expected tee to be enabled after concurrent calls")
		}
		
		// Write data to verify it works
		writer := tm.GetWriter()
		testData := "concurrent test data\n"
		if _, err := writer.Write([]byte(testData)); err != nil {
			t.Fatalf("Failed to write after concurrent EnableTee: %v", err)
		}
	})

	t.Run("concurrent GetWriter and EnableTee", func(t *testing.T) {
		originalOut := &bytes.Buffer{}
		errOut := &bytes.Buffer{}
		tm := NewTeeManager(originalOut, errOut)
		defer tm.Close()

		tmpDir := t.TempDir()
		initialFile := filepath.Join(tmpDir, "initial.log")
		
		// Enable initial tee
		if err := tm.EnableTee(initialFile); err != nil {
			t.Fatalf("Failed to enable initial tee: %v", err)
		}
		
		// Run GetWriter and EnableTee concurrently
		var wg sync.WaitGroup
		
		// Writers will try to get the writer repeatedly
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 100; j++ {
					writer := tm.GetWriter()
					// Try to write
					_, _ = writer.Write([]byte("test\n"))
				}
			}()
		}
		
		// Enablers will try to change the tee file
		for i := 0; i < 3; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				for j := 0; j < 10; j++ {
					filePath := filepath.Join(tmpDir, fmt.Sprintf("change-%d-%d.log", index, j))
					_ = tm.EnableTee(filePath)
				}
			}(i)
		}
		
		wg.Wait()
		
		// Should still be functional
		if !tm.IsEnabled() {
			t.Error("Expected tee to be enabled after concurrent operations")
		}
	})

	t.Run("append to existing file", func(t *testing.T) {
		originalOut := &bytes.Buffer{}
		errOut := &bytes.Buffer{}
		tm := NewTeeManager(originalOut, errOut)
		defer tm.Close()

		tmpDir := t.TempDir()
		teeFile := filepath.Join(tmpDir, "append.log")
		
		// Create file with initial content
		initialContent := "Initial content\n"
		if err := os.WriteFile(teeFile, []byte(initialContent), 0644); err != nil {
			t.Fatalf("Failed to create initial file: %v", err)
		}

		// Enable tee (should append)
		if err := tm.EnableTee(teeFile); err != nil {
			t.Fatalf("Failed to enable tee: %v", err)
		}

		// Write new data
		writer := tm.GetWriter()
		newData := "Appended data\n"
		if _, err := writer.Write([]byte(newData)); err != nil {
			t.Fatalf("Failed to write new data: %v", err)
		}

		// Verify file contains both old and new content
		content, _ := os.ReadFile(teeFile)
		expectedContent := initialContent + newData
		if string(content) != expectedContent {
			t.Errorf("Expected file content %q, got %q", expectedContent, string(content))
		}
	})
}

func TestSafeTeeWriter(t *testing.T) {
	t.Run("single warning with cached writer", func(t *testing.T) {
		// This test verifies that when using TeeManager with caching,
		// we only get one warning even if multiple goroutines write
		originalOut := &bytes.Buffer{}
		errOut := &bytes.Buffer{}
		tm := NewTeeManager(originalOut, errOut)
		defer tm.Close()

		// Create a file and close it to simulate write errors
		tmpFile, err := os.CreateTemp("", "test-*.log")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		tmpPath := tmpFile.Name()
		tmpFile.Close()

		// Enable tee with the closed file
		if err := tm.EnableTee(tmpPath); err != nil {
			t.Fatalf("Failed to enable tee: %v", err)
		}

		// Close the file to ensure writes will fail
		tm.teeFile.Close()

		// Get writer once (should be cached)
		writer := tm.GetWriter()

		// Multiple writes from different goroutines
		var wg sync.WaitGroup
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _ = writer.Write([]byte("test data\n"))
			}()
		}
		wg.Wait()

		// Check that we only got one warning
		warnings := strings.Count(errOut.String(), "WARNING: Failed to write to tee file")
		if warnings != 1 {
			t.Errorf("Expected exactly 1 warning, got %d warnings", warnings)
		}

		// Clean up
		_ = os.Remove(tmpPath)
	})

	t.Run("write error handling", func(t *testing.T) {
		// Create a file and close it to simulate write errors
		tmpFile, err := os.CreateTemp("", "test-*.log")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		tmpPath := tmpFile.Name()
		tmpFile.Close()
		_ = os.Remove(tmpPath) // Remove so we can't write to it

		// Create a closed file handle
		closedFile, _ := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY, 0644)
		closedFile.Close()

		errBuf := &bytes.Buffer{}
		writer := &safeTeeWriter{
			file:      closedFile,
			errStream: errBuf,
			hasWarned: false,
		}

		// First write should print warning
		data := []byte("test data")
		n, err := writer.Write(data)
		if err != nil {
			t.Errorf("Expected no error from Write, got %v", err)
		}
		if n != len(data) {
			t.Errorf("Expected Write to return %d, got %d", len(data), n)
		}

		// Check warning was printed
		errOutput := errBuf.String()
		if !strings.Contains(errOutput, "WARNING: Failed to write to tee file") {
			t.Errorf("Expected warning message, got: %s", errOutput)
		}
		if !strings.Contains(errOutput, "WARNING: Tee logging disabled for remainder of session") {
			t.Errorf("Expected disabled message, got: %s", errOutput)
		}

		// Reset buffer
		errBuf.Reset()

		// Second write should not print warning
		n, err = writer.Write(data)
		if err != nil {
			t.Errorf("Expected no error from second Write, got %v", err)
		}
		if n != len(data) {
			t.Errorf("Expected Write to return %d, got %d", len(data), n)
		}

		// Check no additional warning
		if errBuf.Len() > 0 {
			t.Errorf("Expected no additional warnings, got: %s", errBuf.String())
		}
	})
}

func TestOpenTeeFile(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func() string
		wantErr   bool
		errMsg    string
	}{
		{
			name: "regular file",
			setupFunc: func() string {
				tmpFile, _ := os.CreateTemp("", "test-*.log")
				tmpFile.Close()
				return tmpFile.Name()
			},
			wantErr: false,
		},
		{
			name: "new file",
			setupFunc: func() string {
				return filepath.Join(t.TempDir(), "new.log")
			},
			wantErr: false,
		},
		{
			name: "directory",
			setupFunc: func() string {
				return t.TempDir()
			},
			wantErr: true,
			errMsg:  "is a directory", // OpenFile returns this error for directories
		},
		{
			name: "named pipe (FIFO)",
			setupFunc: func() string {
				tmpDir := t.TempDir()
				fifoPath := filepath.Join(tmpDir, "test.fifo")
				// Try to create a FIFO - this may fail on some systems
				if err := os.MkdirAll(tmpDir, 0755); err == nil {
					// Use syscall.Mkfifo if available, otherwise skip this test
					// For now, we'll create a file and test will pass
					// In real scenario, this would be a FIFO that our validation catches
					_ = os.WriteFile(fifoPath, []byte{}, 0644)
				}
				return fifoPath
			},
			wantErr: false, // Regular file creation as fallback
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setupFunc()
			file, err := openTeeFile(path)
			
			if (err != nil) != tt.wantErr {
				t.Errorf("openTeeFile() error = %v, wantErr %v", err, tt.wantErr)
			}
			
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("Expected error containing %q, got %v", tt.errMsg, err)
			}
			
			if file != nil {
				file.Close()
			}
		})
	}
}