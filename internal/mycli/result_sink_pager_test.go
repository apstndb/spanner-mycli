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
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanner-mycli/internal/mycli/streamio"
)

const pagerHelperEnv = "SPANNER_MYCLI_PAGER_HELPER"

func init() {
	mode := os.Getenv(pagerHelperEnv)
	if mode == "" {
		return
	}
	os.Exit(runPagerHelper(mode))
}

func runPagerHelper(mode string) int {
	switch mode {
	case "cat":
		_, _ = io.Copy(os.Stdout, os.Stdin)
		return 0
	case "head1":
		line, err := bufio.NewReader(os.Stdin).ReadBytes('\n')
		if len(line) == 0 && err != nil && !errors.Is(err, io.EOF) {
			return 1
		}
		_, _ = os.Stdout.Write(line)
		return 0
	case "exit2":
		return 2
	case "hang":
		fmt.Println("pager-hang-ready")
		_ = os.Stdout.Sync()
		select {}
	default:
		fmt.Fprintf(os.Stderr, "unknown pager helper %q\n", mode)
		return 2
	}
}

func setPagerHelper(t *testing.T, mode string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PAGER", exe)
	t.Setenv(pagerHelperEnv, mode)
}

func waitErr(t *testing.T, done <-chan error, d time.Duration, what string) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(d):
		t.Fatalf("watchdog: %s did not finish", what)
		return nil
	}
}

func largePagerInput() string {
	return strings.Repeat("audit line\n", 1<<16)
}

func TestStartPagerCompleteConsumption(t *testing.T) {
	setPagerHelper(t, "cat")
	var buf bytes.Buffer
	out, stop, err := startPager(context.Background(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	payload := "hello pager\nsecond line\n"
	done := make(chan error, 1)
	go func() {
		_, err := io.WriteString(out, payload)
		done <- err
	}()
	if err := waitErr(t, done, 3*time.Second, "cat pager write"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if err := stop(); err != nil {
		t.Fatalf("repeated stop: %v", err)
	}
	if got := buf.String(); got != payload {
		t.Fatalf("pager stdout = %q, want %q", got, payload)
	}
}

func TestStartPagerEarlyExit(t *testing.T) {
	setPagerHelper(t, "head1")
	out, stop, err := startPager(context.Background(), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stop() }()
	done := make(chan error, 1)
	go func() {
		_, err := io.WriteString(out, largePagerInput())
		done <- err
	}()
	err = waitErr(t, done, 3*time.Second, "early-exit pager write")
	if err == nil {
		t.Fatal("large write succeeded after pager early exit; want write error")
	}
}

func TestStartPagerEarlyExitHeadUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses head(1)")
	}
	t.Setenv("PAGER", "head -n 1")
	out, stop, err := startPager(context.Background(), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stop() }()
	done := make(chan error, 1)
	go func() {
		_, err := io.WriteString(out, largePagerInput())
		done <- err
	}()
	err = waitErr(t, done, 3*time.Second, "head -n 1 pager write")
	if err == nil {
		t.Fatal("large write succeeded after head -n 1 exit; want write error")
	}
}

func TestStartPagerFailingChild(t *testing.T) {
	setPagerHelper(t, "exit2")
	out, stop, err := startPager(context.Background(), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stop() }()
	done := make(chan error, 1)
	go func() {
		_, err := io.WriteString(out, largePagerInput())
		done <- err
	}()
	if err := waitErr(t, done, 3*time.Second, "failing pager write"); err == nil {
		t.Fatal("write succeeded through pager that exited 2")
	}
	if err := stop(); err != nil {
		t.Fatalf("stop after failing child: %v", err)
	}
}

func TestStartPagerInvalidAndMissingCommand(t *testing.T) {
	t.Run("whitespace", func(t *testing.T) {
		t.Setenv("PAGER", "   ")
		_, _, err := startPager(context.Background(), io.Discard)
		if err == nil || !strings.Contains(err.Error(), "invalid pager command") {
			t.Fatalf("err = %v, want invalid pager command", err)
		}
	})
	t.Run("missing", func(t *testing.T) {
		t.Setenv("PAGER", "spanner-mycli-pager-command-missing")
		_, stop, err := startPager(context.Background(), io.Discard)
		if err == nil {
			_ = stop()
			t.Fatal("missing pager command started")
		}
		if !strings.Contains(err.Error(), "failed to start pager") {
			t.Fatalf("err = %v, want failed to start pager", err)
		}
	})
}

type readyWriter struct {
	ready chan struct{}
	once  sync.Once
}

func (w *readyWriter) Write(p []byte) (int, error) {
	if bytes.Contains(p, []byte("pager-hang-ready")) {
		w.once.Do(func() { close(w.ready) })
	}
	return len(p), nil
}

func TestStartPagerCancelDuringBlockedWrite(t *testing.T) {
	setPagerHelper(t, "hang")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stdout := &readyWriter{ready: make(chan struct{})}
	out, stop, err := startPager(ctx, stdout)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stop() }()

	select {
	case <-stdout.ready:
	case <-time.After(3 * time.Second):
		t.Fatal("hang pager did not print ready")
	}

	done := make(chan error, 1)
	go func() {
		_, err := out.Write(bytes.Repeat([]byte("x"), 1<<20))
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("blocked write returned before cancel: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	cancel()
	err = waitErr(t, done, 3*time.Second, "canceled pager write")
	if err == nil {
		t.Fatal("canceled write succeeded")
	}
	if !errors.Is(err, context.Canceled) && !isBrokenPipe(err) {
		t.Logf("canceled write error: %v", err)
	}
}

func TestResultSinkCancelBeforeLazyStart(t *testing.T) {
	setPagerHelper(t, "hang")
	cli := newDecorationTestCli()
	cli.SystemVariables.Display.UsePager = true
	cli.SystemVariables.Display.MarkdownCodeblock = true
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var buf bytes.Buffer
	sink := cli.newResultSink(ctx, &buf, "SHOW VARIABLES")
	n, err := sink.Write([]byte("should-not-write"))
	if n != 0 {
		t.Fatalf("wrote %d bytes before lazy start", n)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Write err = %v, want context.Canceled", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("canceled sink started pager/decorations: %q", buf.String())
	}
	sink.abort()
	sink.abort()
	if err := sink.finish(); err != nil {
		t.Fatalf("finish after abort: %v", err)
	}
}

func TestResultSinkRepeatedFinishAbort(t *testing.T) {
	setPagerHelper(t, "cat")
	cli := newDecorationTestCli()
	cli.SystemVariables.Display.UsePager = true
	var buf bytes.Buffer
	sink := cli.newResultSink(context.Background(), &buf, "")
	if _, err := sink.Write([]byte("row\n")); err != nil {
		t.Fatal(err)
	}
	if err := sink.finish(); err != nil {
		t.Fatal(err)
	}
	if err := sink.finish(); err != nil {
		t.Fatalf("repeated finish: %v", err)
	}
	sink.abort()
	sink.abort()
	if !strings.Contains(buf.String(), "row") {
		t.Fatalf("missing payload: %q", buf.String())
	}
}

// largeStreamStatement writes a payload larger than a typical OS pipe
// buffer through the session output writer, which executeStatement binds
// to the resultSink. It exists only to exercise the streamed pager path.
type largeStreamStatement struct{}

func (largeStreamStatement) isDetachedCompatible() {}

func (largeStreamStatement) Execute(_ context.Context, session *Session) (*Result, error) {
	w := session.outputWriter()
	if w == nil {
		return nil, errors.New("no output writer")
	}
	if _, err := io.WriteString(w, largePagerInput()); err != nil {
		return nil, err
	}
	return &Result{Streamed: true, KeepVariables: true}, nil
}

func TestExecuteStatementPagerEarlyExit(t *testing.T) {
	setPagerHelper(t, "head1")
	cli := newDecorationTestCli()
	cli.SystemVariables.Display.UsePager = true
	var buf bytes.Buffer
	done := make(chan error, 1)
	go func() {
		_, err := cli.executeStatement(context.Background(), largeStreamStatement{}, false, "SELECT large", &buf)
		done <- err
	}()
	err := waitErr(t, done, 5*time.Second, "executeStatement pager early exit")
	if err == nil {
		t.Fatal("executeStatement succeeded after pager early exit; incomplete output must not be full success")
	}
}

func TestExecuteStatementPagerCancelBeforeStart(t *testing.T) {
	setPagerHelper(t, "hang")
	cli := newDecorationTestCli()
	cli.SystemVariables.Display.UsePager = true
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var buf bytes.Buffer
	done := make(chan error, 1)
	go func() {
		_, err := cli.executeStatement(ctx, &ShowVariablesStatement{}, false, "SHOW VARIABLES", &buf)
		done <- err
	}()
	err := waitErr(t, done, 5*time.Second, "executeStatement cancel before pager start")
	if err == nil {
		t.Fatal("executeStatement succeeded on canceled context")
	}
	if strings.Contains(buf.String(), "pager-hang-ready") {
		t.Fatalf("canceled executeStatement started hang pager: %q", buf.String())
	}
}

func TestPrintResultPagerEarlyExit(t *testing.T) {
	setPagerHelper(t, "head1")
	outBuf := &bytes.Buffer{}
	sysVars := &systemVariables{
		Display: DisplayVars{
			UsePager:  true,
			CLIFormat: enums.DisplayModeCSV,
		},
		StreamManager: streamio.NewStreamManager(io.NopCloser(bytes.NewReader(nil)), outBuf, outBuf),
	}
	cli := &Cli{SystemVariables: sysVars}
	result := &Result{TableHeader: toTableHeader("col1"), Rows: []Row{toRow(largePagerInput())}}
	done := make(chan error, 1)
	go func() {
		done <- cli.PrintResult(80, result, false, "", outBuf)
	}()
	err := waitErr(t, done, 5*time.Second, "PrintResult pager early exit")
	if err == nil {
		t.Fatal("PrintResult succeeded after pager early exit")
	}
}

func isBrokenPipe(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.ErrClosedPipe) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "broken pipe") || strings.Contains(msg, "pipe")
}
