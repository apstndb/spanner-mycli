package main

import (
	"bytes"
	"os"
	"testing"
)

func TestStreamManager_TerminalFunctions(t *testing.T) {
	t.Run("GetTerminalWidth with TTY", func(t *testing.T) {
		// This test will be skipped in CI environments without TTY
		if !isatty(os.Stdout) {
			t.Skip("Test requires TTY")
		}
		
		tm := NewTeeManager(os.Stdout, os.Stderr)
		
		width, err := tm.GetTerminalWidth()
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
		tm := NewTeeManager(buf, os.Stderr)
		
		_, err := tm.GetTerminalWidth()
		if err == nil {
			t.Error("Expected error for non-TTY output")
		}
	})
	
	t.Run("GetTerminalWidthString without TTY", func(t *testing.T) {
		buf := &bytes.Buffer{}
		tm := NewTeeManager(buf, os.Stderr)
		
		result := tm.GetTerminalWidthString()
		if result != "NULL" {
			t.Errorf("Expected 'NULL', got %s", result)
		}
	})
	
	t.Run("SetTtyStream", func(t *testing.T) {
		buf := &bytes.Buffer{}
		tm := NewTeeManager(buf, os.Stderr)
		
		// Initially should not be a terminal
		if tm.IsTerminal() {
			t.Error("Expected IsTerminal to be false with buffer output")
		}
		
		// Set TTY stream (if available)
		if isatty(os.Stdout) {
			tm.SetTtyStream(os.Stdout)
			
			// Now it should report as terminal
			if !tm.IsTerminal() {
				t.Error("Expected IsTerminal to be true after SetTtyStream")
			}
			
			// Should be able to get terminal width
			width, err := tm.GetTerminalWidth()
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
		tm1 := NewTeeManager(buf, os.Stderr)
		if tm1.IsTerminal() {
			t.Error("Expected IsTerminal to be false for buffer")
		}
		
		// Test with actual terminal (if available)
		if isatty(os.Stdout) {
			tm2 := NewTeeManager(os.Stdout, os.Stderr)
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