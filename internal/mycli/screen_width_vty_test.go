package mycli

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanner-mycli/internal/mycli/format"
	"github.com/apstndb/spanner-mycli/internal/mycli/streamio"
	"github.com/creack/pty"
)

// TestGetTerminalSizeWithPty tests the GetTerminalSize function using a pseudoterminal
// with a controlled size.
func TestGetTerminalSizeWithPty(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
				Rows:        []Row{toRow("value")},
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
				Rows:        []Row{toRow("value")},
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
				Rows:        []Row{toRow("value")},
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
			sysVars := &systemVariables{
				Display: DisplayVars{
					AutoWrap:   tt.autowrap,
					FixedWidth: tt.fixedWidth,
					CLIFormat:  enums.DisplayModeTab, // Use TAB format for predictable output
				},
				StreamManager: streamio.NewStreamManager(io.NopCloser(bytes.NewReader(nil)), tty, os.Stderr),
			}
			// Set the TTY stream in StreamManager
			sysVars.StreamManager.SetTtyStream(tty)
			cli := &Cli{
				SystemVariables: sysVars,
			}

			// Call displayResult with a per-statement sink, as executeStatement does
			err = cli.displayResult(cli.newResultSink(tty, tt.input), tt.result, tt.interactive, tty)
			if err != nil {
				t.Fatalf("displayResult() failed: %v", err)
			}

			// Derive the expected rendered content from the test inputs. TAB
			// format prints the header row followed by each data row. The PTY may
			// translate "\n" to "\r\n" (ONLCR), so assert on the presence of each
			// header name and cell value rather than on exact bytes.
			var want []string
			want = append(want, tt.result.TableHeader.Render(false)...)
			for _, row := range tt.result.Rows {
				want = append(want, format.Texts(row)...)
			}

			// Read from the master side of the PTY until all expected fragments
			// appear. The read runs in a goroutine bounded by a timeout so the
			// test can never hang, even if displayResult produced no output.
			// PTY masters do not support SetReadDeadline on all platforms
			// (e.g. macOS), so on timeout we close the PTY to unblock Read.
			type readResult struct {
				out string
				err error
			}
			done := make(chan readResult, 1)
			go func() {
				var out bytes.Buffer
				buf := make([]byte, 1024)
				for {
					n, readErr := ptmx.Read(buf)
					if n > 0 {
						out.Write(buf[:n])
					}
					if containsAll(out.String(), want) {
						done <- readResult{out: out.String()}
						return
					}
					if readErr != nil {
						done <- readResult{out: out.String(), err: readErr}
						return
					}
				}
			}()

			select {
			case res := <-done:
				if !containsAll(res.out, want) {
					t.Fatalf("PTY output missing expected content: want all of %q, got %q (read error: %v)",
						want, res.out, res.err)
				}
			case <-time.After(5 * time.Second):
				// Unblock the reader goroutine's pending Read.
				ptmx.Close()
				t.Fatalf("timed out reading PTY output: want all of %q", want)
			}
		})
	}
}

// containsAll reports whether s contains every substring in subs.
func containsAll(s string, subs []string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

// isUnixLike returns true if the current system is Unix-like (Linux, macOS, etc.)
func isUnixLike() bool {
	// A simple check for Unix-like systems
	_, err := os.Stat("/dev/null")
	return err == nil
}
