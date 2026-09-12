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
	"fmt"
	"io"
	"strings"
	"testing"
	"text/template"

	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanner-mycli/internal/mycli/streamio"
)

// resultFailureWriter accepts a prefix, then returns the supplied error. Calls
// after that failure are counted so a renderer cannot silently keep writing.
type resultFailureWriter struct {
	accepted     bytes.Buffer
	remaining    int
	err          error
	failed       bool
	afterFailure int
}

func (w *resultFailureWriter) Write(p []byte) (int, error) {
	if w.failed {
		w.afterFailure++
		return 0, w.err
	}
	if len(p) > w.remaining {
		n, _ := w.accepted.Write(p[:w.remaining])
		w.remaining = 0
		w.failed = true
		return n, w.err
	}
	w.remaining -= len(p)
	return w.accepted.Write(p)
}

func (w *resultFailureWriter) String() string { return w.accepted.String() }

func TestPrintResultOutputErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		result   Result
		mode     enums.DisplayMode
		suppress bool
		want     string
	}{
		{name: "summary", want: "Query OK\n"},
		{name: "body and summary", result: Result{Body: PreparedBody([]byte("BODY\n"))}, want: "BODY\nQuery OK\n"},
		{name: "streamed summary", result: Result{Body: DeliveredBody()}, want: "Query OK\n"},
		{name: "appendices", result: Result{Appendices: []ResultAppendix{{Title: "Empty"}, {Title: "Appendix", Lines: []string{"one", "two"}}, {Title: "Next", Lines: []string{"three"}}}, Predicates: []string{"hidden"}, Body: PreparedBody([]byte("BODY\n"))}, suppress: true, want: "BODY\nAppendix\n one\n two\n\nNext\n three\n\n"},
		{name: "predicates", result: Result{Predicates: []string{"one", "two"}}, suppress: true, want: "Predicates(identified by ID):\n one\n two\n\n"},
		{name: "lint", result: Result{LintResults: []string{"one", "two"}}, suppress: true, want: "Experimental Lint Result:\n one\n two\n\n"},
		{name: "advice", result: Result{IndexAdvice: []QueryIndexAdvice{{DDL: []string{"first", "second"}, ImprovementFactor: 2}, {DDL: []string{"third"}}}}, suppress: true, want: "Query Advisor Recommendations:\n  first  -- Est. improvement: 50.00%\n  second  -- Est. improvement: 50.00%\n  third\n\n"},
		{name: "detail comment", mode: enums.DisplayModeTableDetailComment, suppress: true, want: "*/\n"},
		{name: "empty appendix and suppressed summary", result: Result{Appendices: []ResultAppendix{{Title: "Empty"}}}, suppress: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sv := &systemVariables{Display: DisplayVars{CLIFormat: tc.mode, SuppressResultLines: tc.suppress, OutputTemplate: template.Must(template.New("empty").Parse(""))}}
			var success bytes.Buffer
			if err := printResult(sv, 80, &success, &tc.result, true); err != nil {
				t.Fatal(err)
			}
			if success.String() != tc.want {
				t.Fatalf("output = %q, want %q", success.String(), tc.want)
			}
			// Every byte boundary includes partial writes, titles, each line,
			// separators and suffixes, without depending on fmt's call count.
			for limit := range len(tc.want) {
				t.Run(fmt.Sprint(limit), func(t *testing.T) {
					cause := errors.New("result destination failed")
					w := &resultFailureWriter{remaining: limit, err: cause}
					err := printResult(sv, 80, w, &tc.result, true)
					if !errors.Is(err, cause) || !w.failed {
						t.Fatalf("error = %v, failure reached = %v", err, w.failed)
					}
					if w.String() != tc.want[:limit] || w.afterFailure != 0 {
						t.Errorf("accepted = %q, calls after failure = %d", w.String(), w.afterFailure)
					}
				})
			}
		})
	}
}

func TestPrintResultPublicOutputError(t *testing.T) {
	t.Parallel()
	for _, streamed := range []bool{false, true} {
		t.Run(fmt.Sprint(streamed), func(t *testing.T) {
			session := newDetachedTestSession(io.Discard)
			t.Cleanup(session.Close)
			c := &Cli{SessionHandler: NewSessionHandler(session), SystemVariables: session.systemVariables}
			cause := errors.New("public result destination failed")
			w := &resultFailureWriter{err: cause}
			err := c.PrintResult(80, &Result{Body: deliveredBodyIf(streamed)}, true, "", w)
			if !errors.Is(err, cause) || !w.failed || w.afterFailure != 0 {
				t.Fatalf("error=%v, writer=%+v", err, w)
			}
		})
	}
}

func TestRunBatchResultOutputError(t *testing.T) {
	for _, sql := range []string{"SET CLI_PROMPT = 'changed';", "SHOW VARIABLE CLI_PROMPT;"} {
		t.Run(sql, func(t *testing.T) {
			var stderr bytes.Buffer
			cause := errors.New("batch result destination failed")
			w := &resultFailureWriter{err: cause}
			session := newDetachedTestSession(io.Discard)
			t.Cleanup(session.Close)
			c := &Cli{SessionHandler: NewSessionHandler(session), SystemVariables: session.systemVariables}
			c.SystemVariables.StreamManager = streamio.NewStreamManager(nil, w, &stderr)
			c.SystemVariables.Display.Verbose = true
			err := c.RunBatch(t.Context(), sql)
			var exitErr *ExitCodeError
			if !errors.As(err, &exitErr) || GetExitCode(err) != 1 || !strings.Contains(stderr.String(), cause.Error()) || !w.failed {
				t.Fatalf("error=%v, stderr=%q, failure reached=%v", err, stderr.String(), w.failed)
			}
			if strings.HasPrefix(sql, "SET") && c.SystemVariables.Display.Prompt != "changed" {
				t.Fatal("output failure must not undo successful SET")
			}
		})
	}
}

// Output succeeds during execution, but abort's closing fence fails. That
// cleanup failure must not replace the statement's original error.
type resultErrorStatement struct{ cause error }

func (resultErrorStatement) isDetachedCompatible() {}

func (s resultErrorStatement) Execute(_ context.Context, _ *Session, out OperationOutput) (*Result, error) {
	if _, err := io.WriteString(out.Writer(), "BODY\n"); err != nil {
		return nil, err
	}
	return nil, s.cause
}

func TestExecuteStatementPreservesErrorAfterOutput(t *testing.T) {
	t.Parallel()
	session := newDetachedTestSession(io.Discard)
	t.Cleanup(session.Close)
	c := &Cli{SessionHandler: NewSessionHandler(session), SystemVariables: session.systemVariables}
	c.SystemVariables.Display.MarkdownCodeblock = true
	cause := errors.New("statement execution failed")
	cleanup := errors.New("cleanup output failed")
	w := &resultFailureWriter{remaining: len("```sql\nBODY\n"), err: cleanup}
	_, err := c.executeStatement(t.Context(), resultErrorStatement{cause}, false, "", w)
	if !errors.Is(err, cause) || errors.Is(err, cleanup) || !w.failed || w.String() != "```sql\nBODY\n" {
		t.Fatalf("error=%v, output=%q, cleanup failure reached=%v", err, w.String(), w.failed)
	}
}
