package streamio

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

// supportsFIFO returns true if the current platform supports FIFOs (named pipes).
// We use runtime.GOOS instead of build tags for several reasons:
// 1. Code compiles on all platforms, allowing for better cross-platform testing
// 2. Compile-time errors are caught even on platforms that don't support FIFOs
// 3. Test coverage tools can see the code even when tests are skipped
// 4. Simpler build process without platform-specific build constraints
func supportsFIFO() bool {
	switch runtime.GOOS {
	case "darwin", "linux", "freebsd", "netbsd", "openbsd":
		return true
	default:
		return false
	}
}

func TestOpenTeeFile_FIFO(t *testing.T) {
	t.Parallel()
	// Skip test on platforms that don't support FIFOs reliably
	if !supportsFIFO() {
		t.Skipf("FIFO test not supported on %s", runtime.GOOS)
	}

	// Additional check for CI environments
	if os.Getenv("CI") != "" {
		t.Skip("Skipping FIFO test in CI environment")
	}

	tmpDir := t.TempDir()
	fifoPath := filepath.Join(tmpDir, "test.fifo")

	// Create a FIFO
	if err := syscall.Mkfifo(fifoPath, 0o644); err != nil {
		t.Skipf("Failed to create FIFO (may not be supported): %v", err)
	}

	// Test that openTeeFile rejects the FIFO without hanging
	done := make(chan struct{})
	var openErr error

	go func() {
		_, openErr = openTeeFile(fifoPath)
		close(done)
	}()

	// Wait for the function to complete or timeout
	select {
	case <-done:
		// Good, it didn't hang
		if openErr == nil {
			t.Error("Expected error when opening FIFO, got nil")
		}
		if !strings.Contains(openErr.Error(), "non-regular file") {
			t.Errorf("Expected 'non-regular file' error, got: %v", openErr)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("openTeeFile hung when attempting to open FIFO - the protection is not working")
	}
}

func TestStreamManager_FIFO(t *testing.T) {
	t.Parallel()
	// Skip test on platforms that don't support FIFOs reliably
	if !supportsFIFO() {
		t.Skipf("FIFO test not supported on %s", runtime.GOOS)
	}

	// Additional check for CI environments
	if os.Getenv("CI") != "" {
		t.Skip("Skipping FIFO test in CI environment")
	}

	tmpDir := t.TempDir()
	fifoPath := filepath.Join(tmpDir, "test.fifo")

	// Create a FIFO
	if err := syscall.Mkfifo(fifoPath, 0o644); err != nil {
		t.Skipf("Failed to create FIFO (may not be supported): %v", err)
	}

	sm := NewStreamManager(os.Stdin, os.Stdout, os.Stderr)
	defer sm.Close()

	// Test that EnableTee rejects the FIFO without hanging
	done := make(chan struct{})
	var enableErr error

	go func() {
		enableErr = sm.EnableTee(fifoPath, false)
		close(done)
	}()

	// Wait for the function to complete or timeout
	select {
	case <-done:
		// Good, it didn't hang
		if enableErr == nil {
			t.Error("Expected error when enabling tee to FIFO, got nil")
		}
		if !strings.Contains(enableErr.Error(), "non-regular file") {
			t.Errorf("Expected 'non-regular file' error, got: %v", enableErr)
		}
		// Verify that tee is not enabled
		if sm.IsEnabled() {
			t.Error("Tee should not be enabled after failed FIFO attempt")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("EnableTee hung when attempting to use FIFO - the protection is not working")
	}
}
