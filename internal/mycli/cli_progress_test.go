// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mycli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/apstndb/spanner-mycli/enums"
)

// barrierWriter holds the first Write until release is closed. Later writes
// pass through immediately so the progress clear sequence is not deadlocked.
type barrierWriter struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	mu      sync.Mutex
	buf     bytes.Buffer
}

func (w *barrierWriter) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.entered) })
	select {
	case <-w.release:
	case <-time.After(10 * time.Second):
		return 0, errors.New("barrierWriter: release not received")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *barrierWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

type concurrentBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (w *concurrentBuffer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *concurrentBuffer) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

type failWriter struct {
	err error
}

func (w failWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func shortenProgressInterval(t *testing.T) {
	t.Helper()
	orig := progressingMarkInterval
	progressingMarkInterval = time.Millisecond
	t.Cleanup(func() { progressingMarkInterval = orig })
}

func attachTTYPipe(t *testing.T, cli *Cli) *os.File {
	t.Helper()
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = pr.Close()
		_ = pw.Close()
	})
	cli.SystemVariables.StreamManager.SetTtyStream(pw)
	return pr
}

func readPipeDeadline(t *testing.T, r *os.File, d time.Duration) string {
	t.Helper()
	_ = r.SetReadDeadline(time.Now().Add(d))
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

func TestStartProgressingMark_stopJoinsInFlightWrite(t *testing.T) {
	shortenProgressInterval(t)
	progress := &barrierWriter{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	t.Cleanup(func() {
		select {
		case <-progress.release:
		default:
			close(progress.release)
		}
	})

	stop := startProgressingMark(progress)
	select {
	case <-progress.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("progress write did not start")
	}

	stopped := make(chan struct{})
	go func() {
		stop()
		close(stopped)
	}()
	select {
	case <-stopped:
		t.Fatal("stop returned while a progress write was in flight")
	case <-time.After(50 * time.Millisecond):
	}

	close(progress.release)
	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("stop did not join after the in-flight write was released")
	}

	got := progress.String()
	if !strings.HasSuffix(got, "\r \r") {
		t.Fatalf("clear sequence missing after join, got %q", got)
	}
	stop()
	stop()
}

func TestResultSink_noBytesUntilProgressJoined(t *testing.T) {
	shortenProgressInterval(t)
	progress := &barrierWriter{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	t.Cleanup(func() {
		select {
		case <-progress.release:
		default:
			close(progress.release)
		}
	})

	stop := startProgressingMark(progress)
	select {
	case <-progress.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("progress write did not start")
	}

	var dest concurrentBuffer
	cli := newDecorationTestCli()
	cli.SystemVariables.Display.MarkdownCodeblock = true
	cli.SystemVariables.Feature.EchoInput = true
	sink := cli.newResultSink(context.Background(), &dest, "SELECT 1;")
	sink.beforeStart = stop

	done := make(chan error, 1)
	go func() {
		_, err := sink.Write([]byte("body-row\n"))
		done <- err
	}()

	select {
	case err := <-done:
		t.Fatalf("sink write finished before progress was released: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if got := dest.String(); got != "" {
		t.Fatalf("sink wrote %q before progress stopped and cleared", got)
	}

	close(progress.release)
	if err := waitErr(t, done, 3*time.Second, "sink write after progress join"); err != nil {
		t.Fatalf("sink write: %v", err)
	}
	if err := sink.finish(); err != nil {
		t.Fatal(err)
	}

	out := dest.String()
	if strings.Contains(out, "\r") {
		t.Fatalf("result destination received progress control characters: %q", out)
	}
	if !strings.HasPrefix(out, "```sql\nSELECT 1;\n") {
		t.Fatalf("decorations missing or out of order, got %q", out)
	}
	if !strings.Contains(out, "body-row\n") {
		t.Fatalf("body missing, got %q", out)
	}
	if strings.Index(out, "```sql") > strings.Index(out, "body-row") {
		t.Fatalf("body preceded decorations: %q", out)
	}
}

func TestStartProgressingMark_repeatedStopAndNilWriter(t *testing.T) {
	t.Parallel()

	stopNil := startProgressingMark(nil)
	stopNil()
	stopNil()

	var buf bytes.Buffer
	stop := startProgressingMark(&buf)
	stop()
	stop()
	stop()
	if buf.String() != "\r \r" {
		t.Fatalf("stop before first tick should clear once, got %q", buf.String())
	}

	cli := newDecorationTestCli()
	nop := cli.PrintProgressingMark(io.Discard)
	nop()
	nop()
}

func TestSetupProgressMark_selectNotPredictedDumpStillExcluded(t *testing.T) {
	cli := newDecorationTestCli()
	tty := attachTTYPipe(t, cli)

	stopSelect := cli.setupProgressMark(&SelectStatement{Query: "SELECT 1"}, io.Discard)
	stopSelect()
	stopSelect()
	if got := readPipeDeadline(t, tty, 200*time.Millisecond); !strings.Contains(got, "\r") {
		t.Fatalf("SELECT should start progress regardless of format, TTY got %q", got)
	}

	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = pr.Close()
		_ = pw.Close()
	})
	cli.SystemVariables.StreamManager.SetTtyStream(pw)
	stopDump := cli.setupProgressMark(&DumpDatabaseStatement{}, io.Discard)
	stopDump()
	stopDump()
	if got := readPipeDeadline(t, pr, 50*time.Millisecond); got != "" {
		t.Fatalf("DUMP should not start progress, TTY got %q", got)
	}
}

func TestResultSink_emptyFinishStopsProgress(t *testing.T) {
	t.Parallel()

	var stopped atomic.Bool
	cli := newDecorationTestCli()
	cli.SystemVariables.Display.MarkdownCodeblock = true
	var dest bytes.Buffer
	sink := cli.newResultSink(context.Background(), &dest, "")
	sink.beforeStart = func() { stopped.Store(true) }
	if err := sink.finish(); err != nil {
		t.Fatal(err)
	}
	if !stopped.Load() {
		t.Fatal("empty finish must invoke beforeStart")
	}
	if dest.String() != "```sql\n```\n" {
		t.Fatalf("empty fenced result = %q", dest.String())
	}
	if err := sink.finish(); err != nil {
		t.Fatalf("repeated finish: %v", err)
	}
	sink.abort()
}

func TestResultSink_cancelBeforeWriteLeavesStopToCaller(t *testing.T) {
	t.Parallel()

	var stopped atomic.Bool
	cli := newDecorationTestCli()
	cli.SystemVariables.Display.MarkdownCodeblock = true
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var dest bytes.Buffer
	sink := cli.newResultSink(ctx, &dest, "SELECT 1;")
	sink.beforeStart = func() { stopped.Store(true) }
	n, err := sink.Write([]byte("nope"))
	if n != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("Write = %d, %v, want canceled", n, err)
	}
	if stopped.Load() {
		t.Fatal("canceled Write must not start the sink")
	}
	if dest.Len() != 0 {
		t.Fatalf("canceled sink wrote %q", dest.String())
	}
	sink.abort()
	if stopped.Load() {
		t.Fatal("abort must not force beforeStart")
	}
}

func TestResultSink_firstWriteFailureAfterStop(t *testing.T) {
	t.Parallel()

	var stopped atomic.Bool
	cli := newDecorationTestCli()
	writeErr := errors.New("first write failed")
	sink := cli.newResultSink(context.Background(), failWriter{err: writeErr}, "")
	sink.beforeStart = func() { stopped.Store(true) }
	_, err := sink.Write([]byte("row\n"))
	if !stopped.Load() {
		t.Fatal("beforeStart must run before the first dest write")
	}
	if !errors.Is(err, writeErr) {
		t.Fatalf("Write err = %v, want %v", err, writeErr)
	}
	sink.abort()
	sink.abort()
	if err := sink.finish(); err != nil {
		t.Fatalf("finish after abort: %v", err)
	}
}

func TestResultSink_pagerStartFailureStopsProgress(t *testing.T) {
	t.Setenv("PAGER", "spanner-mycli-pager-command-missing")

	var stopped atomic.Bool
	cli := newDecorationTestCli()
	cli.SystemVariables.Display.UsePager = true
	var dest bytes.Buffer
	sink := cli.newResultSink(context.Background(), &dest, "SELECT 1;")
	sink.beforeStart = func() { stopped.Store(true) }
	_, err := sink.Write([]byte("row\n"))
	if !stopped.Load() {
		t.Fatal("beforeStart must run before pager startup")
	}
	if err == nil || !strings.Contains(err.Error(), "failed to start pager") {
		t.Fatalf("Write err = %v, want pager start failure", err)
	}
	sink.abort()
	sink.abort()
	if err := sink.finish(); err != nil {
		t.Fatalf("finish after abort: %v", err)
	}
	if dest.Len() != 0 {
		t.Fatalf("pager start failure wrote %q", dest.String())
	}
}

func TestResultSink_failureAfterStreamedRows(t *testing.T) {
	t.Parallel()

	var stops atomic.Int32
	cli := newDecorationTestCli()
	cli.SystemVariables.Display.MarkdownCodeblock = true
	var dest bytes.Buffer
	sink := cli.newResultSink(context.Background(), &dest, "")
	sink.beforeStart = func() { stops.Add(1) }
	if _, err := sink.Write([]byte("row\n")); err != nil {
		t.Fatal(err)
	}
	sink.abort()
	sink.abort()
	if err := sink.finish(); err != nil {
		t.Fatalf("finish after abort: %v", err)
	}
	if stops.Load() != 1 {
		t.Fatalf("beforeStart calls = %d, want 1", stops.Load())
	}
	out := dest.String()
	if !strings.HasPrefix(out, "```sql\n") || !strings.Contains(out, "row\n") || !strings.HasSuffix(out, "```\n") {
		t.Fatalf("streamed failure output = %q", out)
	}
}

func TestExecuteStatement_progressStaysOffTeeAndMCPCapture(t *testing.T) {
	var dest bytes.Buffer
	session := newDetachedTestSession(&dest)
	cli := &Cli{
		SessionHandler:  NewSessionHandler(session),
		SystemVariables: session.systemVariables,
	}
	tty := attachTTYPipe(t, cli)
	teePath := filepath.Join(t.TempDir(), "tee.log")
	if err := cli.SystemVariables.StreamManager.EnableTee(teePath, false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cli.SystemVariables.StreamManager.DisableTee)

	if _, err := cli.executeStatement(context.Background(), &ShowVariablesStatement{}, true, "SHOW VARIABLES;", nil); err != nil {
		t.Fatal(err)
	}
	ttyOut := readPipeDeadline(t, tty, 200*time.Millisecond)
	if !strings.Contains(ttyOut, "\r") {
		t.Fatalf("interactive progress should clear on TTY, got %q", ttyOut)
	}
	if strings.Contains(dest.String(), "\r") {
		t.Fatalf("result dest received progress control characters: %q", dest.String())
	}
	teeBytes, err := os.ReadFile(teePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(teeBytes), "\r") {
		t.Fatalf("tee file received progress control characters: %q", teeBytes)
	}

	var mcp bytes.Buffer
	if _, err := cli.executeStatement(context.Background(), &ShowVariablesStatement{}, false, "SHOW VARIABLES;", &mcp); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(mcp.String(), "\r") {
		t.Fatalf("MCP capture received progress control characters: %q", mcp.String())
	}
}

func TestExecuteStatement_tableAndNonTableStopBeforeOutput(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode enums.DisplayMode
		stmt Statement
		in   string
	}{
		{name: "csv-stream", mode: enums.DisplayModeCSV, stmt: &ShowVariablesStatement{}, in: "SHOW VARIABLES;"},
		{name: "table-set", mode: enums.DisplayModeTable, stmt: &SetStatement{VarName: "CLI_VERBOSE", Value: "TRUE"}, in: "SET CLI_VERBOSE = TRUE;"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cli := newDecorationTestCli()
			cli.SystemVariables.Display.CLIFormat = tc.mode
			cli.SystemVariables.Display.MarkdownCodeblock = true
			tty := attachTTYPipe(t, cli)
			var dest bytes.Buffer
			if _, err := cli.executeStatement(context.Background(), tc.stmt, true, tc.in, &dest); err != nil {
				t.Fatalf("executeStatement: %v", err)
			}
			if strings.Contains(dest.String(), "\r") {
				t.Fatalf("dest received progress control characters: %q", dest.String())
			}
			if !strings.HasPrefix(dest.String(), "```sql\n") {
				t.Fatalf("output must start with fence after progress join, got %q", dest.String())
			}
			ttyOut := readPipeDeadline(t, tty, 200*time.Millisecond)
			if !strings.Contains(ttyOut, "\r") {
				t.Fatalf("progress should have cleared on TTY, got %q", ttyOut)
			}
		})
	}
}

func TestExecuteStatement_errorBeforeOutputStopsProgress(t *testing.T) {
	cli := newDecorationTestCli()
	tty := attachTTYPipe(t, cli)
	cli.SystemVariables.Display.MarkdownCodeblock = true
	var dest bytes.Buffer
	_, err := cli.executeStatement(context.Background(), &SelectStatement{Query: "SELECT 1"}, true, "SELECT 1;", &dest)
	if err == nil {
		t.Fatal("executeStatement succeeded, want detached-mode rejection")
	}
	if dest.Len() != 0 {
		t.Fatalf("error-before-output wrote %q", dest.String())
	}
	if got := readPipeDeadline(t, tty, 200*time.Millisecond); !strings.Contains(got, "\r") {
		t.Fatalf("progress must be stopped and cleared on the error path, TTY got %q", got)
	}
}

type streamThenErrorStatement struct{}

func (streamThenErrorStatement) isDetachedCompatible() {}

func (streamThenErrorStatement) Execute(_ context.Context, _ *Session, out OperationOutput) (*Result, error) {
	w := out.Writer()
	if w == nil {
		return nil, errors.New("no output writer")
	}
	if _, err := io.WriteString(w, "streamed-row\n"); err != nil {
		return nil, err
	}
	return nil, errors.New("after streamed rows")
}

func TestExecuteStatement_failureAfterStreamedRows(t *testing.T) {
	cli := newDecorationTestCli()
	tty := attachTTYPipe(t, cli)
	cli.SystemVariables.Display.MarkdownCodeblock = true
	var dest bytes.Buffer
	_, err := cli.executeStatement(context.Background(), streamThenErrorStatement{}, true, "SELECT streamed;", &dest)
	if err == nil {
		t.Fatal("executeStatement succeeded, want error after streamed rows")
	}
	out := dest.String()
	if strings.Contains(out, "\r") {
		t.Fatalf("dest received progress control characters: %q", out)
	}
	if !strings.Contains(out, "streamed-row\n") {
		t.Fatalf("streamed row missing: %q", out)
	}
	if !strings.HasSuffix(out, "```\n") {
		t.Fatalf("closing fence missing after streamed failure: %q", out)
	}
	if got := readPipeDeadline(t, tty, 200*time.Millisecond); !strings.Contains(got, "\r") {
		t.Fatalf("progress must be cleared, TTY got %q", got)
	}
}
