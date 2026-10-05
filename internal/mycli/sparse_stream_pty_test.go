//go:build unix

// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mycli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanner-mycli/internal/mycli/streamio"
	"github.com/creack/pty"
	"golang.org/x/term"
)

const sparsePTYHelperEnv = "SPANNER_MYCLI_SPARSE_PTY_HELPER"

type sparsePTYServer struct {
	streamWidthRPCServer
	deliverRow bool
}

func (s *sparsePTYServer) ExecuteStreamingSql(req *sppb.ExecuteSqlRequest, stream sppb.Spanner_ExecuteStreamingSqlServer) error {
	if !strings.Contains(req.Sql, "RECEIVE_") {
		return s.streamWidthRPCServer.ExecuteStreamingSql(req, stream)
	}
	if s.deliverRow {
		row := sparseStreamRow("pty-message-delivered", "one")
		row.Metadata = sparseStreamMetadata()
		if err := stream.Send(row); err != nil {
			return err
		}
	}
	// This marker is only synchronization for the no-row case, not evidence of
	// rendered output. The row case waits for the actual formatted message.
	fmt.Fprintln(os.Stderr, "[receive-rpc-open]")
	<-stream.Context().Done()
	fmt.Fprintln(os.Stderr, "[receive-rpc-cancelled]")
	return stream.Context().Err()
}

// RunInteractive owns process-wide terminal and signal state. Run it in a child
// with a real controlling terminal rather than sending SIGINT to the test runner.
func TestSparseStreamPTYHelper(t *testing.T) {
	mode := os.Getenv(sparsePTYHelperEnv)
	if mode == "" {
		t.Skip("PTY subprocess helper")
	}
	parts := strings.SplitN(mode, "/", 2)
	if len(parts) != 2 {
		t.Fatal("invalid helper mode")
	}
	server := &sparsePTYServer{
		streamWidthRPCServer: streamWidthRPCServer{value: "next-query-ok"},
		deliverRow:           parts[1] == "after-row",
	}
	session, live := newBufconnQuerySession(t, server)
	live.StreamManager = streamio.NewStreamManager(os.Stdin, os.Stdout, os.Stderr)
	live.Display.Prompt = "queue-pty> "
	live.Display.HistoryFile = filepath.Join(t.TempDir(), "history")
	live.Display.EnableHighlight = false
	live.Display.EnableProgressBar = false
	live.Display.UsePager = false
	live.Query.StreamingMode = enums.StreamingModeTrue
	live.Query.TablePreviewRows = 1
	if err := live.SetFromSimple("CLI_FORMAT", parts[0]); err != nil {
		t.Fatal(err)
	}
	cli := &Cli{SessionHandler: NewSessionHandler(session), SystemVariables: live}
	if err := cli.RunInteractive(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestSparseStreamPTY(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"JSONL", "TABLE"} {
		for _, phase := range []string{"before-row", "after-row", "parameters"} {
			t.Run(mode+"/"+phase, func(t *testing.T) {
				t.Parallel()
				exe, err := os.Executable()
				if err != nil {
					t.Fatal(err)
				}
				cmd := exec.CommandContext(t.Context(), exe, "-test.run=^TestSparseStreamPTYHelper$", "-test.timeout=30s")
				cmd.Env = append(os.Environ(), sparsePTYHelperEnv+"="+mode+"/"+phase, "TERM=xterm", "NO_COLOR=1")
				terminal, tty, err := pty.Open()
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = terminal.Close(); _ = tty.Close() })
				if err := pty.Setsize(terminal, &pty.Winsize{Rows: 30, Cols: 120}); err != nil {
					t.Fatal(err)
				}
				// Capture this PTY's raw state before the child starts. The
				// editor uses term.MakeRaw only while reading input, so prompt
				// bytes alone do not mean Enter will survive CR-to-LF mapping.
				original, err := term.MakeRaw(int(tty.Fd()))
				if err != nil {
					t.Fatal(err)
				}
				raw, err := term.GetState(int(tty.Fd()))
				if err != nil {
					t.Fatal(err)
				}
				if err := term.Restore(int(tty.Fd()), original); err != nil {
					t.Fatal(err)
				}
				cmd.Stdin, cmd.Stdout, cmd.Stderr = tty, tty, tty
				cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
				if err := cmd.Start(); err != nil {
					t.Fatal(err)
				}
				_ = tty.Close()
				out := &observedStreamWriter{changed: make(chan struct{}, 1)}
				readDone := make(chan struct{})
				go func() { defer close(readDone); _, _ = io.Copy(out, terminal) }()
				exited := make(chan struct{})
				var exitErr error
				go func() { defer close(exited); exitErr = cmd.Wait() }()
				t.Cleanup(func() {
					_ = cmd.Process.Kill()
					_ = terminal.Close()
					select {
					case <-exited:
					case <-time.After(5 * time.Second):
						t.Error("PTY child did not exit")
					}
					select {
					case <-readDone:
					case <-time.After(5 * time.Second):
						t.Error("PTY reader did not stop")
					}
					if t.Failed() {
						out.mu.Lock()
						defer out.mu.Unlock()
						t.Logf("PTY transcript: %q", out.buf.String())
					}
				})
				// Consume matches in order so a prompt from an earlier edit cannot satisfy
				// the post-cancellation assertion.
				offset := 0
				expect := func(text string) {
					t.Helper()
					watchdog := time.NewTimer(10 * time.Second)
					defer watchdog.Stop()
					for {
						out.mu.Lock()
						got := out.buf.String()
						out.mu.Unlock()
						if i := strings.Index(got[offset:], text); i >= 0 {
							offset += i + len(text)
							return
						}
						select {
						case <-out.changed:
						case <-watchdog.C:
							t.Fatalf("waiting for %q in PTY output %q", text, got[offset:])
						}
					}
				}
				send := func(text string) {
					t.Helper()
					if _, err := io.WriteString(terminal, text); err != nil {
						t.Fatal(err)
					}
				}
				waitPrompt := func() {
					t.Helper()
					expect("queue-pty> ")
					// Poll observable OS state, not an arbitrary typing delay.
					ticker := time.NewTicker(time.Millisecond)
					defer ticker.Stop()
					watchdog := time.NewTimer(5 * time.Second)
					defer watchdog.Stop()
					for {
						state, err := term.GetState(int(terminal.Fd()))
						if err != nil {
							t.Fatal(err)
						}
						if reflect.DeepEqual(state, raw) {
							return
						}
						select {
						case <-ticker.C:
						case <-watchdog.C:
							t.Fatal("PTY did not enter raw input mode")
						}
					}
				}
				waitPrompt()
				if phase == "parameters" {
					send("SET PARAM saved = STRUCT<Id INT64, Name STRING>(1, 'Alice');\r")
					expect("Query OK")
					waitPrompt()
					send("SHOW PARAMS;\r")
					expect("Param_Type")
					waitPrompt()
					send("SHOW PARAM SAVED;\r")
					expect("(1, 'Alice')")
					waitPrompt()
				}
				send("SELECT * FROM RECEIVE_Tasks(max_duration => '20m');\r")
				if phase == "after-row" {
					expect("pty-message-delivered")
				} else {
					expect("[receive-rpc-open]")
				}
				// Write the terminal's interrupt character, not Process.Signal: this tests
				// terminal mode restoration and the normal OS/CLI interrupt path together.
				send("\x03")
				out.waitFor(t, "[receive-rpc-cancelled]")
				waitPrompt()
				send("SELECT 1;\r")
				expect("next-query-ok")
				waitPrompt()
				send("EXIT;\r")
				select {
				case <-exited:
					if exitErr != nil {
						t.Fatalf("PTY child: %v", exitErr)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("PTY child did not exit after EXIT")
				}
				t.Log("Observed open RECEIVE, terminal Ctrl+C, RPC cancellation, prompt recovery, successful next query, and clean EXIT")
			})
		}
	}
}
