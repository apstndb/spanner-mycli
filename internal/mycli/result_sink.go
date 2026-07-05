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
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"

	"github.com/kballard/go-shellquote"
)

// resultSink is the per-statement output sink shared by streamed rows (via
// outputContext) and buffered display (via printResult). It owns the output
// decorations (CLI_MARKDOWN_CODEBLOCK fence, CLI_ECHO_INPUT echo) and the
// CLI_USE_PAGER pager so both apply in the correct order regardless of
// whether the statement streams.
//
// Whether a statement will stream is not knowable with certainty before
// execution (shouldUseStreaming is a heuristic probe; Result.Streamed is the
// truth only afterwards). Instead of predicting, the sink binds lazily: the
// pager is started and the pre-result decorations are emitted on the first
// byte written through it, wherever that byte comes from. For streamed
// statements the first byte is the first row, so the fence/echo precede the
// rows and the rows flow through the pager; for buffered statements the first
// byte arrives at display time, reproducing the historical post-execution
// behavior byte for byte (including SET statements whose own result reflects
// the just-changed decoration flags, because the flags are read at emission
// time, not at statement submission).
type resultSink struct {
	c     *Cli
	dst   io.Writer // caller-provided destination
	input string    // raw statement text for CLI_ECHO_INPUT

	out       io.Writer    // active destination once started (pager pipe or dst)
	startErr  error        // sticky pager start failure
	fenceOpen bool         // opening ```sql fence was written
	stopPager func() error // closes the pager pipe and waits; nil without pager
	done      bool
}

// newResultSink returns a sink writing to dst. input is echoed when
// CLI_ECHO_INPUT is enabled.
func (c *Cli) newResultSink(dst io.Writer, input string) *resultSink {
	return &resultSink{c: c, dst: dst, input: input}
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

	out := s.dst
	if s.c.SystemVariables.Display.UsePager {
		pw, stop, err := startPager(s.dst)
		if err != nil {
			s.startErr = err
			return err
		}
		out = pw
		s.stopPager = stop
	}
	s.out = out

	if s.c.SystemVariables.Display.MarkdownCodeblock {
		fmt.Fprintln(out, "```sql")
		s.fenceOpen = true
	}
	// The echo is intentionally sent through the sink (not TtyOutStream) so it
	// is captured in tee files, providing complete context in logs showing
	// which queries produced which results.
	if s.c.SystemVariables.Feature.EchoInput && s.input != "" {
		fmt.Fprintln(out, s.input+";")
	}
	return nil
}

func (s *resultSink) Write(p []byte) (int, error) {
	if err := s.start(); err != nil {
		return 0, err
	}
	return s.out.Write(p)
}

// finish emits the closing decoration and shuts the pager down. It
// force-starts the sink so statements that produced no other output still
// emit the decoration pair (e.g. an empty ```sql fence), matching the
// historical printResult output byte for byte. Call it only on the success
// path; use abort for error unwinding.
func (s *resultSink) finish() error {
	if s.done {
		return nil
	}
	s.done = true
	if err := s.start(); err != nil {
		return err
	}
	if s.fenceOpen {
		fmt.Fprintln(s.out, "```")
	}
	if s.stopPager != nil {
		return s.stopPager()
	}
	return nil
}

// abort unwinds the sink on the error path without forcing decorations: a
// statement that failed before writing anything keeps the historical silent
// error path, while a failure after rows already streamed still closes the
// fence (keeping the markdown block well-formed) and releases the pager.
func (s *resultSink) abort() {
	if s.done {
		return
	}
	s.done = true
	if s.out == nil {
		return // never started; nothing to unwind
	}
	if s.fenceOpen {
		fmt.Fprintln(s.out, "```")
	}
	if s.stopPager != nil {
		_ = s.stopPager() // errors already logged by stopPager
	}
}

// startPager launches the pager command from $PAGER (default "less") writing
// to w and returns the pipe feeding it plus a function that closes the pipe
// and waits for the pager to exit. Close/wait failures are logged, not
// returned, preserving the historical behavior of the pager teardown.
func startPager(w io.Writer) (io.Writer, func() error, error) {
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
	cmd := exec.CommandContext(context.Background(), split[0], split[1:]...)

	pr, pw := io.Pipe()
	cmd.Stdin = pr
	cmd.Stdout = w

	if err := cmd.Start(); err != nil {
		slog.Error("failed to start pager command", "err", err)
		return nil, nil, fmt.Errorf("failed to start pager: %w", err)
	}

	stop := func() error {
		if err := pw.Close(); err != nil {
			slog.Error("failed to close pipe", "err", err)
		}
		if err := cmd.Wait(); err != nil {
			slog.Error("failed to wait for pager command", "err", err)
		}
		return nil
	}
	return pw, stop, nil
}
