package main

import (
	"bytes"
	"os"
	"testing"
)

func TestStreamManager_TerminalFunctions(t *testing.T) {
	t.Parallel()
	t.Run("GetTerminalWidth with TTY", func(t *testing.T) {
		// This test will be skipped in CI environments without TTY
		if !isatty(os.Stdout) {
			t.Skip("Test requires TTY")
		}

		sm := NewStreamManager(os.Stdin, os.Stdout, os.Stderr)

		width, err := sm.GetTerminalWidth()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Terminal width should be positive
		if width <= 0 {
			t.Errorf("Expected positive width, got %d", width)
		}
	})

	t.Run("GetTerminalWidth without TTY", func(t *testing.T) {
		buf := &bytes.Buffer{}
		sm := NewStreamManager(os.Stdin, buf, os.Stderr)

		_, err := sm.GetTerminalWidth()
		if err == nil {
			t.Error("Expected error for non-TTY output")
		}
	})

	t.Run("GetTerminalWidthString without TTY", func(t *testing.T) {
		buf := &bytes.Buffer{}
		sm := NewStreamManager(os.Stdin, buf, os.Stderr)

		result := sm.GetTerminalWidthString()
		if result != "NULL" {
			t.Errorf("Expected 'NULL', got %s", result)
		}
	})

	t.Run("SetTtyStream", func(t *testing.T) {
		buf := &bytes.Buffer{}
		sm := NewStreamManager(os.Stdin, buf, os.Stderr)

		// Initially should not be a terminal
		if sm.IsTerminal() {
			t.Error("Expected IsTerminal to be false with buffer output")
		}

		// Set TTY stream (if available)
		if isatty(os.Stdout) {
			sm.SetTtyStream(os.Stdout)

			// Now it should report as terminal
			if !sm.IsTerminal() {
				t.Error("Expected IsTerminal to be true after SetTtyStream")
			}

			// Should be able to get terminal width
			width, err := sm.GetTerminalWidth()
			if err != nil {
				t.Errorf("Expected no error after SetTtyStream, got %v", err)
			}
			if width <= 0 {
				t.Errorf("Expected positive width after SetTtyStream, got %d", width)
			}
		}
	})

	t.Run("IsTerminal", func(t *testing.T) {
		// Test with buffer (not a terminal)
		buf := &bytes.Buffer{}
		tm1 := NewStreamManager(os.Stdin, buf, os.Stderr)
		if tm1.IsTerminal() {
			t.Error("Expected IsTerminal to be false for buffer")
		}

		// Test with actual terminal (if available)
		if isatty(os.Stdout) {
			tm2 := NewStreamManager(os.Stdin, os.Stdout, os.Stderr)
			if !tm2.IsTerminal() {
				t.Error("Expected IsTerminal to be true for os.Stdout")
			}
		}
	})
}

// isatty is a simple helper to check if a file is a terminal
func isatty(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
