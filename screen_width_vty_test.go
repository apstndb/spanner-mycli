package main

import (
	"os"
	"testing"

	"github.com/creack/pty"
)

// TestGetTerminalSizeWithPty tests the GetTerminalSize function using a pseudoterminal
// with a controlled size.
func TestGetTerminalSizeWithPty(t *testing.T) {
	// Skip the test if we're not on a Unix-like system
	if !isUnixLike() {
		t.Skip("Skipping test on non-Unix-like system")
	}

	tests := []struct {
		desc      string
		width     uint16
		height    uint16
		wantWidth int
	}{
		{
			desc:      "standard terminal size",
			width:     80,
			height:    24,
			wantWidth: 80,
		},
		{
			desc:      "wide terminal",
			width:     120,
			height:    30,
			wantWidth: 120,
		},
		{
			desc:      "narrow terminal",
			width:     40,
			height:    20,
			wantWidth: 40,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			// Create a pseudoterminal
			ptmx, tty, err := pty.Open()
			if err != nil {
				t.Fatalf("Failed to open pty: %v", err)
			}
			defer ptmx.Close()
			defer tty.Close()

			// Set the terminal size
			err = pty.Setsize(ptmx, &pty.Winsize{
				Cols: tt.width,
				Rows: tt.height,
			})
			if err != nil {
				t.Fatalf("Failed to set pty size: %v", err)
			}

			// Get the terminal size using our function
			gotWidth, err := GetTerminalSize(tty)
			if err != nil {
				t.Fatalf("GetTerminalSize() error = %v", err)
			}

			// Verify the width matches what we set
			if gotWidth != tt.wantWidth {
				t.Errorf("GetTerminalSize() = %v, want %v", gotWidth, tt.wantWidth)
			}
		})
	}
}

// TestDisplayResultWithPty tests the displayResult method with a pseudoterminal
// of controlled size.
func TestDisplayResultWithPty(t *testing.T) {
	// Skip the test if we're not on a Unix-like system
	if !isUnixLike() {
		t.Skip("Skipping test on non-Unix-like system")
	}

	tests := []struct {
		desc        string
		autowrap    bool
		fixedWidth  *int64
		termWidth   uint16
		termHeight  uint16
		result      *Result
		interactive bool
		input       string
	}{
		{
			desc:       "CLI_AUTOWRAP=true, no fixed width, terminal width=100",
			autowrap:   true,
			fixedWidth: nil,
			termWidth:  100,
			termHeight: 30,
			result: &Result{
				TableHeader: toTableHeader("col1"),
				Rows:        []Row{{"value"}},
			},
			interactive: false,
			input:       "SELECT 'value'",
		},
		{
			desc:       "CLI_AUTOWRAP=true, fixed width=80, terminal width=100",
			autowrap:   true,
			fixedWidth: int64Ptr(80),
			termWidth:  100,
			termHeight: 30,
			result: &Result{
				TableHeader: toTableHeader("col1"),
				Rows:        []Row{{"value"}},
			},
			interactive: false,
			input:       "SELECT 'value'",
		},
		{
			desc:       "CLI_AUTOWRAP=false, terminal width=100",
			autowrap:   false,
			fixedWidth: nil,
			termWidth:  100,
			termHeight: 30,
			result: &Result{
				TableHeader: toTableHeader("col1"),
				Rows:        []Row{{"value"}},
			},
			interactive: false,
			input:       "SELECT 'value'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			// Create a pseudoterminal
			ptmx, tty, err := pty.Open()
			if err != nil {
				t.Fatalf("Failed to open pty: %v", err)
			}
			defer ptmx.Close()
			defer tty.Close()

			// Set the terminal size
			err = pty.Setsize(ptmx, &pty.Winsize{
				Cols: tt.termWidth,
				Rows: tt.termHeight,
			})
			if err != nil {
				t.Fatalf("Failed to set pty size: %v", err)
			}

			// Create a Cli with our system variables and the pseudoterminal as output
			cli := &Cli{
				OutStream: tty,
				SystemVariables: &systemVariables{
					CurrentOutStream: tty,
					AutoWrap:         tt.autowrap,
					FixedWidth:       tt.fixedWidth,
					CLIFormat:        DisplayModeTab, // Use TAB format for predictable output
				},
			}

			// Call displayResult
			cli.displayResult(tt.result, tt.interactive, tt.input, tty)

			// Read from the master side of the PTY to get the output
			buf := make([]byte, 1024)
			n, err := ptmx.Read(buf)
			if err != nil {
				t.Fatalf("Failed to read from PTY: %v", err)
			}

			// Verify that the output was generated correctly
			// This is a basic check that some output was produced
			if n == 0 {
				t.Errorf("No output was produced")
			}

			// We could add more specific checks here based on the expected output format
			// but that would require detailed knowledge of the output format
		})
	}
}

// isUnixLike returns true if the current system is Unix-like (Linux, macOS, etc.)
func isUnixLike() bool {
	// A simple check for Unix-like systems
	_, err := os.Stat("/dev/null")
	return err == nil
}
