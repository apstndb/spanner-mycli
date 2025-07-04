package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestStreamManager_LargeFileWarning tests that we handle large files gracefully
func TestStreamManager_LargeFileWarning(t *testing.T) {
	// Note: We don't actually enforce file size limits in StreamManager
	// This test documents the current behavior
	originalOut := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	sm := NewStreamManager(os.Stdin, originalOut, errOut)
	defer sm.Close()
	
	tmpDir := t.TempDir()
	teeFile := filepath.Join(tmpDir, "large.log")
	
	// Enable tee
	if err := sm.EnableTee(teeFile); err != nil {
		t.Fatalf("Failed to enable tee: %v", err)
	}
	
	// Write a large amount of data (1MB)
	largeData := strings.Repeat("x", 1024*1024)
	writer := sm.GetWriter()
	if _, err := writer.Write([]byte(largeData)); err != nil {
		t.Fatalf("Failed to write large data: %v", err)
	}
	
	// Verify file was created and contains data
	info, err := os.Stat(teeFile)
	if err != nil {
		t.Fatalf("Failed to stat tee file: %v", err)
	}
	if info.Size() != int64(len(largeData)) {
		t.Errorf("Expected file size %d, got %d", len(largeData), info.Size())
	}
}

// TestStreamManager_DirectoryError tests that specifying a directory returns appropriate error
func TestStreamManager_DirectoryError(t *testing.T) {
	originalOut := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	sm := NewStreamManager(os.Stdin, originalOut, errOut)
	defer sm.Close()
	
	tmpDir := t.TempDir()
	
	// Try to enable tee with a directory path
	err := sm.EnableTee(tmpDir)
	if err == nil {
		t.Error("Expected error when enabling tee with directory path")
	}
	if !strings.Contains(err.Error(), "non-regular file") {
		t.Errorf("Expected 'non-regular file' error, got: %v", err)
	}
	
	// Verify tee is not enabled
	if sm.IsEnabled() {
		t.Error("Tee should not be enabled after error")
	}
}

// TestStreamManager_NoWritePermission tests graceful handling of permission errors
func TestStreamManager_NoWritePermission(t *testing.T) {
	// Skip if running as root
	if os.Geteuid() == 0 {
		t.Skip("Test requires non-root user")
	}
	
	originalOut := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	sm := NewStreamManager(os.Stdin, originalOut, errOut)
	defer sm.Close()
	
	tmpDir := t.TempDir()
	
	// Create a read-only directory
	readOnlyDir := filepath.Join(tmpDir, "readonly")
	if err := os.Mkdir(readOnlyDir, 0555); err != nil {
		t.Fatalf("Failed to create read-only directory: %v", err)
	}
	
	// Try to create a file in the read-only directory
	teeFile := filepath.Join(readOnlyDir, "test.log")
	err := sm.EnableTee(teeFile)
	if err == nil {
		t.Error("Expected error when creating file in read-only directory")
	}
	
	// Verify tee is not enabled
	if sm.IsEnabled() {
		t.Error("Tee should not be enabled after permission error")
	}
}

// TestStreamManager_ManyFileSwitches tests that file handles don't leak
func TestStreamManager_ManyFileSwitches(t *testing.T) {
	originalOut := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	sm := NewStreamManager(os.Stdin, originalOut, errOut)
	defer sm.Close()
	
	tmpDir := t.TempDir()
	
	// Switch between many files
	for i := 0; i < 100; i++ {
		teeFile := filepath.Join(tmpDir, fmt.Sprintf("file-%d.log", i))
		if err := sm.EnableTee(teeFile); err != nil {
			t.Fatalf("Failed to enable tee for file %d: %v", i, err)
		}
		
		// Write some data
		writer := sm.GetWriter()
		data := fmt.Sprintf("test data %d\n", i)
		if _, err := writer.Write([]byte(data)); err != nil {
			t.Fatalf("Failed to write to file %d: %v", i, err)
		}
	}
	
	// Disable tee
	sm.DisableTee()
	
	// Try to open many files to ensure we didn't leak file descriptors
	var files []*os.File
	defer func() {
		for _, f := range files {
			f.Close()
		}
	}()
	
	for i := 0; i < 50; i++ {
		f, err := os.Open(filepath.Join(tmpDir, fmt.Sprintf("file-%d.log", i)))
		if err != nil {
			t.Fatalf("Failed to open file %d (possible fd leak): %v", i, err)
		}
		files = append(files, f)
	}
}

// TestStreamManager_CloseIdempotency tests that Close() can be called multiple times
func TestStreamManager_CloseIdempotency(t *testing.T) {
	originalOut := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	sm := NewStreamManager(os.Stdin, originalOut, errOut)
	
	tmpDir := t.TempDir()
	teeFile := filepath.Join(tmpDir, "test.log")
	
	// Enable tee
	if err := sm.EnableTee(teeFile); err != nil {
		t.Fatalf("Failed to enable tee: %v", err)
	}
	
	// Call Close multiple times - should not panic
	sm.Close()
	sm.Close()
	sm.Close()
	
	// Verify tee is disabled
	if sm.IsEnabled() {
		t.Error("Expected tee to be disabled after Close")
	}
}

// TestStreamManager_WriteAfterError tests behavior after write errors
func TestStreamManager_WriteAfterError(t *testing.T) {
	originalOut := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	sm := NewStreamManager(os.Stdin, originalOut, errOut)
	defer sm.Close()
	
	// Create and immediately close a file to simulate write errors
	tmpFile, err := os.CreateTemp("", "test-*.log")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	
	// Enable tee with the file
	if err := sm.EnableTee(tmpPath); err != nil {
		t.Fatalf("Failed to enable tee: %v", err)
	}
	
	// Manually close the tee file to cause write errors
	sm.mu.Lock()
	sm.teeFile.Close()
	sm.mu.Unlock()
	
	// First write should generate warning
	writer := sm.GetWriter()
	data1 := "first write\n"
	if _, err := writer.Write([]byte(data1)); err != nil {
		t.Errorf("Write should not return error: %v", err)
	}
	
	// Check warning was printed
	if !strings.Contains(errOut.String(), "WARNING: Failed to write to tee file") {
		t.Error("Expected warning message not found")
	}
	
	// Clear error buffer
	errOut.Reset()
	
	// Subsequent writes should not generate additional warnings
	data2 := "second write\n"
	if _, err := writer.Write([]byte(data2)); err != nil {
		t.Errorf("Write should not return error: %v", err)
	}
	
	// No new warning should be printed
	if errOut.Len() > 0 {
		t.Errorf("Expected no additional warnings, got: %s", errOut.String())
	}
	
	// Main output should contain all data
	output := originalOut.String()
	if !strings.Contains(output, data1) || !strings.Contains(output, data2) {
		t.Errorf("Main output missing data: %q", output)
	}
	
	// Clean up
	_ = os.Remove(tmpPath)
}