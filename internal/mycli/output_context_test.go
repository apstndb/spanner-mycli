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

	"cloud.google.com/go/spanner"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanner-mycli/internal/mycli/streamio"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// mcpResultText extracts the single text content from a tool result.
func mcpResultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if result == nil || len(result.Content) == 0 {
		t.Fatal("tool result has no content")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("tool result content is %T, want *mcp.TextContent", result.Content[0])
	}
	return text.Text
}

// newDetachedTestSession builds a minimal detached-mode session whose
// StreamManager writes to global. Suitable for client-side statements only.
func newDetachedTestSession(global io.Writer) *Session {
	sysVars := newSystemVariablesWithDefaults()
	sysVars.Display.CLIFormat = enums.DisplayModeCSV
	sysVars.StreamManager = streamio.NewStreamManager(io.NopCloser(strings.NewReader("")), global, io.Discard)

	session := &Session{
		mode:            Detached,
		systemVariables: &sysVars,
	}
	// Zero-value &Session{} needs a real (client-less) manager now that
	// callers dispatch to session.txn directly instead of the deleted mirror.
	session.txn = NewTransactionManager(nil, &sysVars, spanner.ClientConfig{})
	sysVars.inTransaction = session.txn.InTransaction
	return session
}

// TestExecuteStatementWithOutput_routesStreamedOutput is the regression test
// for the MCP protocol corruption bug: streamed statement output (here, SHOW
// VARIABLES under CLI_FORMAT=CSV) must go to the per-statement writer, not to
// the process-global StreamManager writer.
func TestExecuteStatementWithOutput_routesStreamedOutput(t *testing.T) {
	t.Parallel()

	var global, perCall bytes.Buffer
	session := newDetachedTestSession(&global)

	result, err := session.ExecuteStatementWithOutput(context.Background(), &ShowVariablesStatement{}, outputContext{w: &perCall})
	if err != nil {
		t.Fatalf("ExecuteStatementWithOutput: %v", err)
	}

	if !result.Streamed {
		t.Errorf("Streamed = false, want true (CSV should stream)")
	}
	if perCall.Len() == 0 || !strings.Contains(perCall.String(), "CLI_FORMAT") {
		t.Errorf("per-statement writer got %q, want CSV rows including CLI_FORMAT", perCall.String())
	}
	if global.Len() != 0 {
		t.Errorf("StreamManager writer got %q, want empty (rows must not leak to the global stream)", global.String())
	}
	if session.output.w != nil || session.output.screenWidth != nil {
		t.Errorf("session.output not restored after execution: %+v", session.output)
	}
}

// TestExecuteStatement_fallsBackToStreamManager pins the fallback behavior:
// without a per-statement outputContext (direct Session.ExecuteStatement
// callers), streamed output still goes to the StreamManager writer.
func TestExecuteStatement_fallsBackToStreamManager(t *testing.T) {
	t.Parallel()

	var global bytes.Buffer
	session := newDetachedTestSession(&global)

	result, err := session.ExecuteStatement(context.Background(), &ShowVariablesStatement{})
	if err != nil {
		t.Fatalf("ExecuteStatement: %v", err)
	}

	if !result.Streamed {
		t.Errorf("Streamed = false, want true (CSV should stream)")
	}
	if global.Len() == 0 || !strings.Contains(global.String(), "CLI_FORMAT") {
		t.Errorf("StreamManager writer got %q, want CSV rows including CLI_FORMAT", global.String())
	}
}

// TestWithOutput_nestedRestore verifies that nested withOutput calls (e.g.
// buffered DUMP capturing SQL export while a per-statement destination is
// active) restore the previous destination on unwind.
func TestWithOutput_nestedRestore(t *testing.T) {
	t.Parallel()

	var outer, inner bytes.Buffer
	session := newDetachedTestSession(io.Discard)

	err := session.withOutput(outputContext{w: &outer}, func() error {
		if got := session.outputWriter(); got != &outer {
			t.Errorf("outputWriter() = %v, want outer buffer", got)
		}
		return session.withOutput(outputContext{w: &inner}, func() error {
			if got := session.outputWriter(); got != &inner {
				t.Errorf("outputWriter() = %v, want inner buffer", got)
			}
			return nil
		})
	})
	if err != nil {
		t.Fatalf("withOutput: %v", err)
	}
	if session.output.w != nil || session.output.screenWidth != nil {
		t.Errorf("session.output not restored after nested withOutput: %+v", session.output)
	}
}

// TestMCPHandler_streamedFormatDoesNotLeakToGlobalStream exercises the full
// MCP handler path: with a streaming CLI_FORMAT, the tool result must contain
// the rows and nothing may be written to the StreamManager writer (which is
// the JSON-RPC stdout under --mcp).
func TestMCPHandler_streamedFormatDoesNotLeakToGlobalStream(t *testing.T) {
	t.Parallel()

	var global bytes.Buffer
	session := newDetachedTestSession(&global)
	cli := &Cli{
		SessionHandler:  NewSessionHandler(session),
		SystemVariables: session.systemVariables,
	}

	handler := executeStatementHandler(cli)
	result, _, err := handler(context.Background(), nil, ExecuteStatementArgs{Statement: "SHOW VARIABLES"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	text := mcpResultText(t, result)
	if !strings.Contains(text, "CLI_FORMAT") {
		t.Errorf("tool result %q does not contain streamed CSV rows", text)
	}
	if strings.HasPrefix(text, "ERROR:") {
		t.Errorf("tool result is an error: %q", text)
	}
	if global.Len() != 0 {
		t.Errorf("StreamManager writer got %q, want empty (would corrupt the MCP protocol stream)", global.String())
	}
}

// TestMCPHandler_shellMetaCommandDoesNotLeakToGlobalStream pins that \!
// shell command stdout goes to the per-statement writer, not the global
// stream: the MCP handler can parse and execute meta commands, so shell
// output written to the StreamManager writer would corrupt the JSON-RPC
// stream under --mcp.
func TestMCPHandler_shellMetaCommandDoesNotLeakToGlobalStream(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("shell command syntax differs on Windows")
	}

	var global bytes.Buffer
	session := newDetachedTestSession(&global)
	// ShellMetaCommand is not detached-compatible; the shell command itself
	// never touches the (nil) client.
	session.mode = DatabaseConnected
	cli := &Cli{
		SessionHandler:  NewSessionHandler(session),
		SystemVariables: session.systemVariables,
	}

	handler := executeStatementHandler(cli)
	result, _, err := handler(context.Background(), nil, ExecuteStatementArgs{Statement: `\! echo mcp-shell-probe`})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	text := mcpResultText(t, result)
	if !strings.Contains(text, "mcp-shell-probe") {
		t.Errorf("tool result %q does not contain shell output", text)
	}
	if global.Len() != 0 {
		t.Errorf("StreamManager writer got %q, want empty (would corrupt the MCP protocol stream)", global.String())
	}
}
