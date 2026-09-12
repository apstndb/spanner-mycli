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
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/kballard/go-shellquote"
)

// resultSink is the per-statement output sink shared by streamed rows (via
// outputContext) and buffered display (via printResult). It owns the output
// decorations (CLI_MARKDOWN_CODEBLOCK fence, CLI_ECHO_INPUT echo) and the
// CLI_USE_PAGER pager so both apply in the correct order regardless of
// whether the statement streams.
//
// Whether a statement will stream is not knowable with certainty before
// execution (Result.Streamed is the truth only afterwards). Instead of
// predicting, the sink binds lazily: the pager is started and the pre-result
// decorations are emitted on the first byte written through it, wherever that
// byte comes from. For streamed statements the first byte is the first row,
// so the fence/echo precede the rows and the rows flow through the pager; for
// buffered statements the first byte arrives at display time, reproducing the
// historical post-execution behavior byte for byte (including SET statements
// whose own result reflects the just-changed decoration flags, because the
// flags are read at emission time, not at statement submission).
type resultSink struct {
	c     *Cli
	ctx   context.Context // statement context; PrintResult uses Background
	dst   io.Writer       // caller-provided destination
	input string          // already prepared CLI_ECHO_INPUT text; no extra semicolon is appended

	beforeStart func()       // optional; invoked once before pager or any bytes
	out         io.Writer    // active destination once started (pager pipe or dst)
	startErr    error        // sticky pager start failure
	fenceOpen   bool         // opening ```sql fence was written
	stopPager   func() error // closes pager stdin and waits once; nil without pager
	done        bool
}

// newResultSink returns a sink writing to dst. input is already prepared echo
// text: the sink writes it as-is when CLI_ECHO_INPUT is enabled and does not
// append another semicolon. A display newline is added only when the text
// does not already end with one, so subsequent output stays separated.
// ctx bounds the pager child only; it cannot interrupt an independently
// blocking dst (unpaged output).
func (c *Cli) newResultSink(ctx context.Context, dst io.Writer, input string) *resultSink {
	if ctx == nil {
		ctx = context.Background()
	}
	return &resultSink{c: c, ctx: ctx, dst: dst, input: input}
}

// start lazily initializes the sink: it starts the pager when CLI_USE_PAGER
// is enabled and emits the pre-result decorations. Decoration flags are read
// here (at first output) rather than at sink construction so statements that
// change them (SET CLI_MARKDOWN_CODEBLOCK=TRUE) decorate their own result,
// matching the historical post-execution evaluation.
func (s *resultSink) start() error {
	if s.out != nil || s.startErr != nil {
		return s.startErr
	}
	// Join progress (or any other pre-output work) before pager startup or
	// any fence, echo, or result bytes. finish() also comes through here so
	// empty successful results stop the spinner before emitting decorations.
	if s.beforeStart != nil {
		stop := s.beforeStart
		s.beforeStart = nil
		stop()
	}
	if err := s.ctx.Err(); err != nil {
		s.startErr = err
		return err
	}

	out := s.dst
	if s.c.SystemVariables.Display.UsePager {
		pw, stop, err := startPager(s.ctx, s.dst)
		if err != nil {
			s.startErr = err
			return err
		}
		out = pw
		s.stopPager = stop
	}
	s.out = out

	// Test seam: fail after the pager is owned and before decorations so
	// finish/abort must still Wait. Production leaves this nil.
	if testFailPagerDecorate != nil {
		err := testFailPagerDecorate()
		s.startErr = err
		return err
	}

	if s.c.SystemVariables.Display.MarkdownCodeblock {
		if _, err := fmt.Fprintln(out, "```sql"); err != nil {
			s.startErr = s.wrapPagerWrite(err)
			return s.startErr
		}
		s.fenceOpen = true
	}
	// The echo is intentionally sent through the sink (not TtyOutStream) so it
	// is captured in tee files, providing complete context in logs showing
	// which queries produced which results.
	if s.c.SystemVariables.Feature.EchoInput && s.input != "" {
		if _, err := io.WriteString(out, s.input); err != nil {
			s.startErr = s.wrapPagerWrite(err)
			return s.startErr
		}
		if !strings.HasSuffix(s.input, "\n") {
			if _, err := fmt.Fprint(out, "\n"); err != nil {
				s.startErr = s.wrapPagerWrite(err)
				return s.startErr
			}
		}
	}
	return nil
}

// interactiveEchoText prepares CLI_ECHO_INPUT text for interactive and MCP
// callers using the historical convention: the statement with a semicolon
// terminator. Empty input stays empty so the sink skips the echo.
func interactiveEchoText(statement string) string {
	if statement == "" {
		return ""
	}
	return statement + delimiterHorizontal
}

func (s *resultSink) Write(p []byte) (int, error) {
	if err := s.ctx.Err(); err != nil {
		return 0, err
	}
	if err := s.start(); err != nil {
		return 0, err
	}
	n, err := s.out.Write(p)
	if err != nil {
		return n, s.wrapPagerWrite(err)
	}
	return n, nil
}

func (s *resultSink) wrapPagerWrite(err error) error {
	if err == nil {
		return nil
	}
	if ctxErr := s.ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if s.stopPager == nil {
		return err
	}
	return fmt.Errorf("pager write: %w", err)
}

// finish emits the closing decoration and shuts the pager down. It
// force-starts the sink so statements that produced no other output still
// emit the decoration pair (e.g. an empty ```sql fence), matching the
// historical printResult output byte for byte. Call it only on the success
// path; use abort for error unwinding. The pager is reaped even when start
// or the closing fence fails, and done is set only after that reap.
func (s *resultSink) finish() error {
	if s.done {
		return nil
	}
	err := s.start()
	if err == nil && s.fenceOpen {
		if _, ferr := fmt.Fprintln(s.out, "```"); ferr != nil {
			err = s.wrapPagerWrite(ferr)
		}
	}
	err = errors.Join(err, s.reapPager())
	s.done = true
	return err
}

// abort unwinds the sink on the error path without forcing decorations: a
// statement that failed before writing anything keeps the historical silent
// error path, while a failure after rows already streamed still closes the
// fence (keeping the markdown block well-formed) and releases the pager.
func (s *resultSink) abort() {
	if s.done {
		return
	}
	if s.out != nil && s.fenceOpen {
		fmt.Fprintln(s.out, "```")
	}
	_ = s.reapPager()
	s.done = true
}

func (s *resultSink) reapPager() error {
	stop := s.stopPager
	s.stopPager = nil
	if stop == nil {
		return nil
	}
	return stop()
}

// startPager launches $PAGER (default "less") writing to w and returns the
// stdin writer plus a stop func that closes stdin and Wait()s exactly once.
//
// Ownership uses exec.StdinPipe, not an unmanaged io.Pipe as Cmd.Stdin.
// Cmd.Wait waits for the extra copy goroutine that io.Copy's a non-file
// Stdin into the child; if the child exits first that copy returns while the
// PipeWriter stays open, so a later Write blocks forever. StdinPipe is an
// *os.File, so the child closing stdin yields EPIPE and unblocks writers.
// CommandContext(ctx) kills the child on statement cancel, which is the same
// unblock path. stop Wait()s exactly once and returns close/wait errors:
// canceled contexts map to ctx.Err(), and a nonzero child exit after stdin
// EOF is an unexpected pager failure (not full success). Interactive user
// quit while bytes are still being written already fails at Write (EPIPE).
// If the entire payload fit in the OS pipe and the pager exited 0 after
// reading only a prefix, that incomplete display is not detectable from Wait.
func startPager(ctx context.Context, w io.Writer) (io.Writer, func() error, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	pagerpath := cmp.Or(os.Getenv("PAGER"), "less")

	split, err := shellquote.Split(pagerpath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse pager command: %w", err)
	}
	// A whitespace-only PAGER yields an empty slice; reject it instead of
	// panicking on split[0].
	if len(split) == 0 {
		return nil, nil, fmt.Errorf("invalid pager command: %q", pagerpath)
	}
	cmd := exec.CommandContext(ctx, split[0], split[1:]...)
	cmd.Stdout = w

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to start pager: %w", err)
	}
	if err := cmd.Start(); err != nil {
		slog.Error("failed to start pager command", "err", err)
		_ = stdin.Close()
		return nil, nil, fmt.Errorf("failed to start pager: %w", err)
	}

	var once sync.Once
	var stopErr error
	stop := func() error {
		once.Do(func() {
			if err := stdin.Close(); err != nil && !ignorePagerCloseErr(err) {
				slog.Error("failed to close pager stdin", "err", err)
				stopErr = err
			}
			if err := cmd.Wait(); err != nil {
				if ctx.Err() != nil {
					stopErr = ctx.Err()
					return
				}
				slog.Error("failed to wait for pager command", "err", err)
				if stopErr == nil {
					stopErr = fmt.Errorf("pager wait: %w", err)
				}
			}
		})
		return stopErr
	}
	return stdin, stop, nil
}

func ignorePagerCloseErr(err error) bool {
	return errors.Is(err, os.ErrClosed) || errors.Is(err, io.ErrClosedPipe)
}

// testFailPagerDecorate is invoked after a pager is acquired and before
// decorations. Production leaves it nil. Tests must not run in parallel
// with other pager tests while it is set.
var testFailPagerDecorate func() error
