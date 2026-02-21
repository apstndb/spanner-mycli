package mycli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestStreamManager(t *testing.T) {
	t.Parallel()
	t.Run("basic enable and disable", func(t *testing.T) {
		originalOut := &bytes.Buffer{}
		errOut := &bytes.Buffer{}
		sm := NewStreamManager(os.Stdin, originalOut, errOut)
		defer sm.Close()

		// Initially disabled
		if sm.IsEnabled() {
			t.Error("Expected tee to be disabled initially")
		}

		// Enable tee
		tmpDir := t.TempDir()
		teeFile := filepath.Join(tmpDir, "test.log")
		if err := sm.EnableTee(teeFile, false); err != nil {
			t.Fatalf("Failed to enable tee: %v", err)
		}

		if !sm.IsEnabled() {
			t.Error("Expected tee to be enabled after EnableTee")
		}

		// Write through the writer
		writer := sm.GetWriter()
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
		sm.DisableTee()
		if sm.IsEnabled() {
			t.Error("Expected tee to be disabled after DisableTee")
		}

		// Write after disable - should only go to original
		originalOut.Reset()
		testData2 := "After disable\n"
		writer2 := sm.GetWriter()
		if _, err := writer2.Write([]byte(testData2)); err != nil {
			t.Fatalf("Failed to write after disable: %v", err)
		}

		if originalOut.String() != testData2 {
			t.Errorf("Expected output after disable %q, got %q", testData2, originalOut.String())
		}

		// Verify tee file wasn't updated
		content2, err := os.ReadFile(teeFile)
		if err != nil {
			t.Fatalf("Failed to read tee file after disable: %v", err)
		}
		if string(content2) != testData {
			t.Errorf("Expected tee file unchanged, got %q", string(content2))
		}
	})

	t.Run("switching tee files", func(t *testing.T) {
		originalOut := &bytes.Buffer{}
		errOut := &bytes.Buffer{}
		sm := NewStreamManager(os.Stdin, originalOut, errOut)
		defer sm.Close()

		tmpDir := t.TempDir()

		// Enable first tee file
		teeFile1 := filepath.Join(tmpDir, "first.log")
		if err := sm.EnableTee(teeFile1, false); err != nil {
			t.Fatalf("Failed to enable first tee: %v", err)
		}

		// Write to first file
		writer := sm.GetWriter()
		data1 := "First file data\n"
		if _, err := writer.Write([]byte(data1)); err != nil {
			t.Fatalf("Failed to write to first file: %v", err)
		}

		// Switch to second file
		teeFile2 := filepath.Join(tmpDir, "second.log")
		if err := sm.EnableTee(teeFile2, false); err != nil {
			t.Fatalf("Failed to switch to second tee: %v", err)
		}

		// Write to second file
		writer2 := sm.GetWriter()
		data2 := "Second file data\n"
		if _, err := writer2.Write([]byte(data2)); err != nil {
			t.Fatalf("Failed to write to second file: %v", err)
		}

		// Verify first file has only first data
		content1, err := os.ReadFile(teeFile1)
		if err != nil {
			t.Fatalf("Failed to read first file: %v", err)
		}
		if string(content1) != data1 {
			t.Errorf("Expected first file content %q, got %q", data1, string(content1))
		}

		// Verify second file has only second data
		content2, err := os.ReadFile(teeFile2)
		if err != nil {
			t.Fatalf("Failed to read second file: %v", err)
		}
		if string(content2) != data2 {
			t.Errorf("Expected second file content %q, got %q", data2, string(content2))
		}

		// Verify stdout has both
		expectedOut := data1 + data2
		if originalOut.String() != expectedOut {
			t.Errorf("Expected stdout %q, got %q", expectedOut, originalOut.String())
		}
	})

	t.Run("enable same file twice", func(t *testing.T) {
		originalOut := &bytes.Buffer{}
		errOut := &bytes.Buffer{}
		sm := NewStreamManager(os.Stdin, originalOut, errOut)
		defer sm.Close()

		tmpDir := t.TempDir()
		teeFile := filepath.Join(tmpDir, "test.log")

		// Enable tee
		if err := sm.EnableTee(teeFile, false); err != nil {
			t.Fatalf("Failed to enable tee: %v", err)
		}

		// Write first data
		writer := sm.GetWriter()
		data1 := "First write\n"
		if _, err := writer.Write([]byte(data1)); err != nil {
			t.Fatalf("Failed to write first data: %v", err)
		}

		// Enable same file again
		if err := sm.EnableTee(teeFile, false); err != nil {
			t.Fatalf("Failed to re-enable same file: %v", err)
		}

		// Write second data
		writer2 := sm.GetWriter()
		data2 := "Second write\n"
		if _, err := writer2.Write([]byte(data2)); err != nil {
			t.Fatalf("Failed to write second data: %v", err)
		}

		// File should have both (append mode)
		content, err := os.ReadFile(teeFile)
		if err != nil {
			t.Fatalf("Failed to read tee file: %v", err)
		}
		expectedContent := data1 + data2
		if string(content) != expectedContent {
			t.Errorf("Expected file content %q, got %q", expectedContent, string(content))
		}
	})

	t.Run("GetWriter caching", func(t *testing.T) {
		originalOut := &bytes.Buffer{}
		errOut := &bytes.Buffer{}
		sm := NewStreamManager(os.Stdin, originalOut, errOut)
		defer sm.Close()

		// Without tee, should return outStream directly
		writer1 := sm.GetWriter()
		writer2 := sm.GetWriter()
		if writer1 != writer2 {
			t.Error("Expected GetWriter to return same instance without tee")
		}

		// Enable tee
		tmpDir := t.TempDir()
		teeFile := filepath.Join(tmpDir, "test.log")
		if err := sm.EnableTee(teeFile, false); err != nil {
			t.Fatalf("Failed to enable tee: %v", err)
		}

		// Should return cached writer
		writer3 := sm.GetWriter()
		writer4 := sm.GetWriter()
		if writer3 != writer4 {
			t.Error("Expected GetWriter to return cached instance with tee")
		}

		// After disable, should return original stream again
		sm.DisableTee()
		writer5 := sm.GetWriter()
		if writer5 != originalOut {
			t.Error("Expected GetWriter to return original stream after disable")
		}
	})

	t.Run("invalid file paths", func(t *testing.T) {
		originalOut := &bytes.Buffer{}
		errOut := &bytes.Buffer{}
		sm := NewStreamManager(os.Stdin, originalOut, errOut)
		defer sm.Close()

		// Try to enable tee with non-existent directory
		invalidPath := "/non/existent/path/file.log"
		err := sm.EnableTee(invalidPath, false)
		if err == nil {
			t.Error("Expected error for invalid path")
		}

		// Tee should remain disabled
		if sm.IsEnabled() {
			t.Error("Expected tee to remain disabled after error")
		}
	})

	t.Run("writer caching invalidation", func(t *testing.T) {
		originalOut := &bytes.Buffer{}
		errOut := &bytes.Buffer{}
		sm := NewStreamManager(os.Stdin, originalOut, errOut)
		defer sm.Close()

		tmpDir := t.TempDir()
		teeFile := filepath.Join(tmpDir, "test.log")

		// Enable tee
		if err := sm.EnableTee(teeFile, false); err != nil {
			t.Fatalf("Failed to enable tee: %v", err)
		}

		// Get cached writer
		writer1 := sm.GetWriter()
		writer2 := sm.GetWriter()
		if writer1 != writer2 {
			t.Error("Expected same cached writer")
		}

		// Disable tee (should invalidate cache)
		sm.DisableTee()

		// Re-enable tee
		if err := sm.EnableTee(teeFile, false); err != nil {
			t.Fatalf("Failed to re-enable tee: %v", err)
		}

		// Should get new writer instance
		writer3 := sm.GetWriter()
		if writer3 == writer1 {
			t.Error("Expected new writer instance after re-enable")
		}

		// But multiple calls should still return same new instance
		writer4 := sm.GetWriter()
		if writer3 != writer4 {
			t.Error("Expected GetWriter to return the same cached instance after re-enable")
		}

		// Multiple calls should still return same new instance
		writer5 := sm.GetWriter()
		if writer4 != writer5 {
			t.Error("Expected GetWriter to return the same cached instance after re-enable")
		}
	})

	t.Run("concurrent EnableTee calls", func(t *testing.T) {
		originalOut := &bytes.Buffer{}
		errOut := &bytes.Buffer{}
		sm := NewStreamManager(os.Stdin, originalOut, errOut)
		defer sm.Close()

		tmpDir := t.TempDir()

		// Run multiple EnableTee calls concurrently
		var wg sync.WaitGroup
		errors := make([]error, 10)

		for i := 0; i < 10; i++ {
			wg.Go(func() {
				filePath := filepath.Join(tmpDir, fmt.Sprintf("concurrent-%d.log", i))
				errors[i] = sm.EnableTee(filePath, false)
			})
		}

		wg.Wait()

		// All calls should succeed
		for i, err := range errors {
			if err != nil {
				t.Errorf("EnableTee call %d failed: %v", i, err)
			}
		}

		// Only one file should be active (the last one)
		if !sm.IsEnabled() {
			t.Error("Expected tee to be enabled after concurrent calls")
		}

		// Write data to verify it works
		writer := sm.GetWriter()
		testData := "concurrent test data\n"
		if _, err := writer.Write([]byte(testData)); err != nil {
			t.Fatalf("Failed to write after concurrent EnableTee: %v", err)
		}
	})

	t.Run("concurrent read/write operations", func(t *testing.T) {
		originalOut := &syncBuffer{}
		errOut := &syncBuffer{}
		sm := NewStreamManager(os.Stdin, originalOut, errOut)
		defer sm.Close()

		tmpDir := t.TempDir()
		teeFile := filepath.Join(tmpDir, "concurrent.log")

		// Enable tee
		if err := sm.EnableTee(teeFile, false); err != nil {
			t.Fatalf("Failed to enable tee: %v", err)
		}

		// Run concurrent operations
		var wg sync.WaitGroup
		iterations := 100

		// Writers
		for i := 0; i < 5; i++ {
			wg.Go(func() {
				writer := sm.GetWriter()
				for j := 0; j < iterations; j++ {
					data := fmt.Sprintf("Writer %d iteration %d\n", i, j)
					_, _ = writer.Write([]byte(data))
				}
			})
		}

		// Status checkers
		for i := 0; i < 3; i++ {
			wg.Go(func() {
				for j := 0; j < iterations; j++ {
					_ = sm.IsEnabled()
				}
			})
		}

		wg.Wait()

		// Verify file has content
		content, err := os.ReadFile(teeFile)
		if err != nil {
			t.Fatalf("Failed to read tee file: %v", err)
		}

		lines := strings.Split(string(content), "\n")
		expectedLines := 5*iterations + 1 // +1 for trailing newline
		if len(lines) != expectedLines {
			t.Errorf("Expected %d lines in file, got %d", expectedLines, len(lines))
		}

		// Verify stdout also has content
		// Note: With concurrent writes, lines might appear in different order between stdout and file
		// due to scheduling, so we compare sets of lines using go-cmp with sorting
		stdoutLines := strings.Split(strings.TrimSpace(originalOut.String()), "\n")
		fileLines := strings.Split(strings.TrimSpace(string(content)), "\n")

		// Use go-cmp with SortSlices to compare lines regardless of order
		less := func(a, b string) bool { return a < b }
		if diff := cmp.Diff(stdoutLines, fileLines, cmpopts.SortSlices(less)); diff != "" {
			t.Errorf("stdout and file content mismatch (-stdout +file):\n%s", diff)
		}
	})

	t.Run("append to existing file", func(t *testing.T) {
		originalOut := &bytes.Buffer{}
		errOut := &bytes.Buffer{}
		sm := NewStreamManager(os.Stdin, originalOut, errOut)
		defer sm.Close()

		tmpDir := t.TempDir()
		teeFile := filepath.Join(tmpDir, "append.log")

		// Create file with initial content
		initialContent := "Initial content\n"
		if err := os.WriteFile(teeFile, []byte(initialContent), 0o644); err != nil {
			t.Fatalf("Failed to create initial file: %v", err)
		}

		// Enable tee (should append)
		if err := sm.EnableTee(teeFile, false); err != nil {
			t.Fatalf("Failed to enable tee: %v", err)
		}

		// Write new data
		writer := sm.GetWriter()
		newData := "Appended data\n"
		if _, err := writer.Write([]byte(newData)); err != nil {
			t.Fatalf("Failed to write: %v", err)
		}

		// File should have both initial and new content
		content, err := os.ReadFile(teeFile)
		if err != nil {
			t.Fatalf("Failed to read file: %v", err)
		}

		expectedContent := initialContent + newData
		if string(content) != expectedContent {
			t.Errorf("Expected file content %q, got %q", expectedContent, string(content))
		}
	})
}

// syncBuffer is a thread-safe wrapper around bytes.Buffer for testing
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (sb *syncBuffer) Write(p []byte) (n int, err error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Write(p)
}

func (sb *syncBuffer) String() string {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.String()
}

func TestSafeTeeWriter(t *testing.T) {
	t.Parallel()
	t.Run("single warning with cached writer", func(t *testing.T) {
		// This test verifies that when using StreamManager with caching,
		// we only get one warning even if multiple goroutines write
		originalOut := &syncBuffer{}
		errOut := &syncBuffer{}
		sm := NewStreamManager(os.Stdin, originalOut, errOut)
		defer sm.Close()

		// Create a file and close it to simulate write errors
		tmpFile, err := os.CreateTemp("", "test-*.log")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		tmpPath := tmpFile.Name()
		tmpFile.Close()

		// Enable tee with the closed file
		if err := sm.EnableTee(tmpPath, false); err != nil {
			t.Fatalf("Failed to enable tee: %v", err)
		}

		// Close the file to ensure writes will fail
		sm.teeFile.Close()

		// Get writer once (should be cached)
		writer := sm.GetWriter()

		// Concurrent writes
		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Go(func() {
				data := fmt.Sprintf("Test data from goroutine %d\n", i)
				_, _ = writer.Write([]byte(data))
			})
		}

		wg.Wait()

		// Verify only one warning was printed (important for concurrent writes)
		warningCount := strings.Count(errOut.String(), "WARNING: Failed to write to tee file")
		if warningCount != 1 {
			t.Errorf("Expected exactly 1 warning, got %d warnings", warningCount)
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
		closedFile, _ := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY, 0o644)
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

func TestStreamManagerSilentMode(t *testing.T) {
	t.Parallel()
	t.Run("silent mode functionality", func(t *testing.T) {
		originalOut := &bytes.Buffer{}
		errOut := &bytes.Buffer{}
		sm := NewStreamManager(os.Stdin, originalOut, errOut)
		defer sm.Close()

		// Initially not in silent mode
		if sm.IsInSilentTeeMode() {
			t.Error("Expected not to be in silent tee mode initially")
		}

		// Enable silent tee
		tmpDir := t.TempDir()
		teeFile := filepath.Join(tmpDir, "silent.log")
		if err := sm.EnableTee(teeFile, true); err != nil {
			t.Fatalf("Failed to enable silent tee: %v", err)
		}

		if !sm.IsEnabled() {
			t.Error("Expected tee to be enabled")
		}

		if !sm.IsInSilentTeeMode() {
			t.Error("Expected to be in silent tee mode")
		}

		// Write through the writer
		writer := sm.GetWriter()
		testData := "Silent mode test\n"
		if _, err := writer.Write([]byte(testData)); err != nil {
			t.Fatalf("Failed to write: %v", err)
		}

		// In silent mode, original output should NOT receive data
		if originalOut.String() != "" {
			t.Errorf("Expected no output to stdout in silent mode, got %q", originalOut.String())
		}

		// But tee file should have the data
		content, err := os.ReadFile(teeFile)
		if err != nil {
			t.Fatalf("Failed to read tee file: %v", err)
		}
		if string(content) != testData {
			t.Errorf("Expected tee file content %q, got %q", testData, string(content))
		}

		// Disable tee
		sm.DisableTee()
		if sm.IsInSilentTeeMode() {
			t.Error("Expected not to be in silent tee mode after disable")
		}

		// Re-enable in normal mode
		teeFile2 := filepath.Join(tmpDir, "normal.log")
		if err := sm.EnableTee(teeFile2, false); err != nil {
			t.Fatalf("Failed to re-enable tee: %v", err)
		}

		if sm.IsInSilentTeeMode() {
			t.Error("Expected not to be in silent mode when enabled with false")
		}

		// Write more data
		writer2 := sm.GetWriter()
		testData2 := "Normal mode test\n"
		if _, err := writer2.Write([]byte(testData2)); err != nil {
			t.Fatalf("Failed to write: %v", err)
		}

		// In normal mode, both should receive data
		if originalOut.String() != testData2 {
			t.Errorf("Expected stdout output %q, got %q", testData2, originalOut.String())
		}

		content2, err := os.ReadFile(teeFile2)
		if err != nil {
			t.Fatalf("Failed to read tee file: %v", err)
		}
		if string(content2) != testData2 {
			t.Errorf("Expected tee file content %q, got %q", testData2, string(content2))
		}
	})

	t.Run("GetWriter vs GetOutStream usage", func(t *testing.T) {
		originalOut := &bytes.Buffer{}
		errOut := &bytes.Buffer{}
		sm := NewStreamManager(os.Stdin, originalOut, errOut)
		defer sm.Close()

		tmpDir := t.TempDir()
		outputFile := filepath.Join(tmpDir, "output.sql")

		// Enable output redirect (silent mode)
		if err := sm.EnableTee(outputFile, true); err != nil {
			t.Fatalf("Failed to enable output redirect: %v", err)
		}

		// GetWriter should write to file only (respects redirect)
		writer := sm.GetWriter()
		dataOutput := "INSERT INTO table VALUES (1, 'data');\n"
		if _, err := writer.Write([]byte(dataOutput)); err != nil {
			t.Fatalf("Failed to write data: %v", err)
		}

		// GetOutStream should still write to stdout (for progress/UI)
		outStream := sm.GetOutStream()
		progressMsg := "Processing... 50%\n"
		if _, err := outStream.Write([]byte(progressMsg)); err != nil {
			t.Fatalf("Failed to write progress: %v", err)
		}

		// Verify stdout has only progress message (not data)
		if originalOut.String() != progressMsg {
			t.Errorf("Expected stdout to have only progress %q, got %q", progressMsg, originalOut.String())
		}

		// Verify file has only data output (not progress)
		content, err := os.ReadFile(outputFile)
		if err != nil {
			t.Fatalf("Failed to read output file: %v", err)
		}
		if string(content) != dataOutput {
			t.Errorf("Expected file to have only data %q, got %q", dataOutput, string(content))
		}
	})
}
