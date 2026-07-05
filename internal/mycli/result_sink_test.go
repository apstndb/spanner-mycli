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
	"io"
	"runtime"
	"strings"
	"testing"
)

// newDecorationTestCli builds a Cli around a detached-mode session (CSV
// format, streaming-capable) suitable for exercising Cli.executeStatement
// with client-side statements and no emulator.
func newDecorationTestCli() *Cli {
	session := newDetachedTestSession(io.Discard)
	return &Cli{
		SessionHandler:  NewSessionHandler(session),
		SystemVariables: session.systemVariables,
	}
}

// TestExecuteStatement_decorationsPrecedeStreamedRows is the regression test
// for FEEDBACK C2 (ordering): with CLI_MARKDOWN_CODEBLOCK and CLI_ECHO_INPUT
// enabled, the opening ```sql fence and the echoed input must appear BEFORE
// streamed rows, and the closing fence after them. Before the fix, both
// decorations were emitted post-hoc by printResult, so streamed rows came
// first.
func TestExecuteStatement_decorationsPrecedeStreamedRows(t *testing.T) {
	t.Parallel()

	cli := newDecorationTestCli()
	cli.SystemVariables.Display.MarkdownCodeblock = true
	cli.SystemVariables.Feature.EchoInput = true

	var buf bytes.Buffer
	if _, err := cli.executeStatement(context.Background(), &ShowVariablesStatement{}, false, "SHOW VARIABLES", &buf); err != nil {
		t.Fatalf("executeStatement: %v", err)
	}

	out := buf.String()
	if !strings.HasPrefix(out, "```sql\nSHOW VARIABLES;\n") {
		t.Errorf("output must start with the opening fence and the echoed input, got prefix %q", firstLines(out, 3))
	}
	rowIdx := strings.Index(out, "CLI_FORMAT")
	echoIdx := strings.Index(out, "SHOW VARIABLES;\n")
	if rowIdx < 0 {
		t.Fatalf("streamed CSV rows missing from output:\n%s", out)
	}
	if echoIdx < 0 || echoIdx > rowIdx {
		t.Errorf("echoed input must precede streamed rows (echo at %d, rows at %d)", echoIdx, rowIdx)
	}
	if !strings.HasSuffix(out, "```\n") {
		t.Errorf("output must end with the closing fence, got suffix %q", lastLines(out, 2))
	}
	closeIdx := strings.LastIndex(out, "```\n")
	if closeIdx < rowIdx {
		t.Errorf("closing fence must come after streamed rows (close at %d, rows at %d)", closeIdx, rowIdx)
	}
}

// TestExecuteStatement_pagerCoversStreamedRows is the regression test for
// FEEDBACK C2 (pager coverage): with CLI_USE_PAGER enabled, streamed rows
// must flow through the pager process, not bypass it. The pager is a sed
// command that prefixes every line, so any unprefixed output proves a bypass.
func TestExecuteStatement_pagerCoversStreamedRows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test pager command uses sed")
	}
	t.Setenv("PAGER", "sed s/^/paged:/")

	cli := newDecorationTestCli()
	cli.SystemVariables.Display.UsePager = true

	var buf bytes.Buffer
	if _, err := cli.executeStatement(context.Background(), &ShowVariablesStatement{}, false, "SHOW VARIABLES", &buf); err != nil {
		t.Fatalf("executeStatement: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "CLI_FORMAT") {
		t.Fatalf("streamed CSV rows missing from output:\n%s", out)
	}
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		if !strings.HasPrefix(line, "paged:") {
			t.Errorf("line bypassed the pager: %q", line)
		}
	}
}

// TestExecuteStatement_errorBeforeOutputEmitsNoDecorations pins the error
// path: a statement that fails before writing anything must not leave a
// dangling opening fence (or any decoration) behind.
func TestExecuteStatement_errorBeforeOutputEmitsNoDecorations(t *testing.T) {
	t.Parallel()

	cli := newDecorationTestCli()
	cli.SystemVariables.Display.MarkdownCodeblock = true
	cli.SystemVariables.Feature.EchoInput = true

	var buf bytes.Buffer
	// SELECT requires a database connection; the detached session rejects it
	// before producing any output.
	_, err := cli.executeStatement(context.Background(), &SelectStatement{Query: "SELECT 1"}, false, "SELECT 1", &buf)
	if err == nil {
		t.Fatal("executeStatement succeeded, want detached-mode rejection")
	}
	if buf.Len() != 0 {
		t.Errorf("failed statement wrote decorations: %q", buf.String())
	}
}

// TestExecuteStatement_setMarkdownDecoratesOwnResult pins that decoration
// flags are read at emission time, not before execution: SET
// CLI_MARKDOWN_CODEBLOCK=TRUE fences its own result line, matching the
// historical post-execution flag evaluation.
func TestExecuteStatement_setMarkdownDecoratesOwnResult(t *testing.T) {
	t.Parallel()

	cli := newDecorationTestCli()
	if cli.SystemVariables.Display.MarkdownCodeblock {
		t.Fatal("precondition: CLI_MARKDOWN_CODEBLOCK must start disabled")
	}

	var buf bytes.Buffer
	stmt := &SetStatement{VarName: "CLI_MARKDOWN_CODEBLOCK", Value: "TRUE"}
	if _, err := cli.executeStatement(context.Background(), stmt, true, "SET CLI_MARKDOWN_CODEBLOCK = TRUE", &buf); err != nil {
		t.Fatalf("executeStatement: %v", err)
	}

	out := buf.String()
	if !strings.HasPrefix(out, "```sql\n") {
		t.Errorf("SET result must be fenced with the post-execution flag value, got %q", out)
	}
	if !strings.Contains(out, "Query OK") {
		t.Errorf("interactive SET result line missing, got %q", out)
	}
	if !strings.HasSuffix(out, "```\n\n") {
		t.Errorf("output must end with closing fence plus interactive newline, got %q", out)
	}
}

// firstLines returns up to n leading lines of s for readable test failures.
func firstLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

// lastLines returns up to n trailing lines of s for readable test failures.
func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSuffix(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
