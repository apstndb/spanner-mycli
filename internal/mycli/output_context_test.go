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

// newClientlessDatabaseSession is a client-less DatabaseConnected session.
// RUN BATCH is not DetachedCompatible, so nested-dispatch tests use this.
func newClientlessDatabaseSession(global io.Writer) *Session {
	session := newDetachedTestSession(global)
	session.mode = DatabaseConnected
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

	result, err := session.ExecuteStatementWithOutput(context.Background(), &ShowVariablesStatement{}, OperationOutput{w: &perCall})
	if err != nil {
		t.Fatalf("ExecuteStatementWithOutput: %v", err)
	}

	if !result.alreadyDelivered() {
		t.Errorf("Streamed = false, want true (CSV should stream)")
	}
	if perCall.Len() == 0 || !strings.Contains(perCall.String(), "CLI_FORMAT") {
		t.Errorf("per-statement writer got %q, want CSV rows including CLI_FORMAT", perCall.String())
	}
	if global.Len() != 0 {
		t.Errorf("StreamManager writer got %q, want empty (rows must not leak to the global stream)", global.String())
	}
}

// TestExecuteStatement_fallsBackToStreamManager pins the fallback behavior:
// without a per-statement OperationOutput writer (direct Session.ExecuteStatement
// callers), streamed output still goes to the StreamManager writer.
func TestExecuteStatement_fallsBackToStreamManager(t *testing.T) {
	t.Parallel()

	var global bytes.Buffer
	session := newDetachedTestSession(&global)

	result, err := session.ExecuteStatement(context.Background(), &ShowVariablesStatement{})
	if err != nil {
		t.Fatalf("ExecuteStatement: %v", err)
	}

	if !result.alreadyDelivered() {
		t.Errorf("Streamed = false, want true (CSV should stream)")
	}
	if global.Len() == 0 || !strings.Contains(global.String(), "CLI_FORMAT") {
		t.Errorf("StreamManager writer got %q, want CSV rows including CLI_FORMAT", global.String())
	}
}

// TestOperationOutput_nestedRunBatchDispatch covers the #918 forwarding
// boundary: RUN BATCH must re-enter ExecuteStatementWithOutput with the
// caller destination. Dropping that value (ExecuteStatement) sends streamed
// rows to StreamManager instead. Subtests share no session; each is sequential
// so the next-statement default destination is deterministic.
func TestOperationOutput_nestedRunBatchDispatch(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	t.Run("success then default next statement", func(t *testing.T) {
		t.Parallel()

		var global, caller bytes.Buffer
		session := newClientlessDatabaseSession(&global)
		t.Cleanup(session.Close)
		session.batch.SetCurrent(&ShowVariablesStatement{})
		callerOut := OperationOutput{w: &caller}

		result, err := session.ExecuteStatementWithOutput(ctx, &RunBatchStatement{}, callerOut)
		if err != nil {
			t.Fatalf("RUN BATCH: %v", err)
		}
		if !result.alreadyDelivered() {
			t.Errorf("Streamed = false, want true (CSV should stream)")
		}
		if caller.Len() == 0 || !strings.Contains(caller.String(), "CLI_FORMAT") {
			t.Errorf("caller writer got %q, want CSV rows including CLI_FORMAT", caller.String())
		}
		if global.Len() != 0 {
			t.Errorf("StreamManager writer got %q, want empty during nested RUN BATCH", global.String())
		}

		callerLen := caller.Len()
		next, err := session.ExecuteStatement(ctx, &ShowVariablesStatement{})
		if err != nil {
			t.Fatalf("next ExecuteStatement: %v", err)
		}
		if !next.alreadyDelivered() {
			t.Errorf("next Streamed = false, want true")
		}
		if global.Len() == 0 || !strings.Contains(global.String(), "CLI_FORMAT") {
			t.Errorf("next statement StreamManager writer got %q, want CSV rows", global.String())
		}
		if caller.Len() != callerLen {
			t.Errorf("caller writer grew from %d to %d after next statement", callerLen, caller.Len())
		}
	})

	t.Run("nested error then default next statement", func(t *testing.T) {
		t.Parallel()

		var global, caller bytes.Buffer
		session := newClientlessDatabaseSession(&global)
		t.Cleanup(session.Close)
		session.batch.SetCurrent(&SetLocalStatement{VarName: "CLI_VERBOSE", Value: "TRUE"})
		callerOut := OperationOutput{w: &caller}

		_, err := session.ExecuteStatementWithOutput(ctx, &RunBatchStatement{}, callerOut)
		if err == nil || !strings.Contains(err.Error(), "SET LOCAL requires an active transaction") {
			t.Fatalf("RUN BATCH nested error = %v, want SET LOCAL transaction error", err)
		}
		if global.Len() != 0 {
			t.Errorf("StreamManager writer got %q, want empty on nested error", global.String())
		}
		if caller.Len() != 0 {
			t.Errorf("caller writer got %q, want empty on nested SET LOCAL error", caller.String())
		}
		if session.batch.IsActive() {
			t.Error("batch still active after nested error; TakeForExecution should have consumed it")
		}

		next, err := session.ExecuteStatement(ctx, &ShowVariablesStatement{})
		if err != nil {
			t.Fatalf("next ExecuteStatement: %v", err)
		}
		if !next.alreadyDelivered() {
			t.Errorf("next Streamed = false, want true")
		}
		if global.Len() == 0 || !strings.Contains(global.String(), "CLI_FORMAT") {
			t.Errorf("next statement StreamManager writer got %q, want CSV rows", global.String())
		}
		if caller.Len() != 0 {
			t.Errorf("caller writer got %q after next statement, want unchanged empty", caller.String())
		}
	})
}

// TestDumpBuffered_isolatesLocalWriter checks DUMP internal buffering: the
// caller's OperationOutput writer stays unused and unmutated while the
// prepared body receives the dumped bytes.
func TestDumpBuffered_isolatesLocalWriter(t *testing.T) {
	t.Parallel()

	var caller bytes.Buffer
	session := newDetachedTestSession(io.Discard)
	t.Cleanup(session.Close)
	out := OperationOutput{w: &caller}
	ddl := []byte("CREATE TABLE T (Id INT64) PRIMARY KEY(Id);\n")
	result, err := executeDumpBufferedWithTxn(t.Context(), session, dumpModeSchema, &dumpPlan{DDL: ddl}, nil, nil, out)
	if err != nil {
		t.Fatalf("executeDumpBufferedWithTxn: %v", err)
	}
	if caller.Len() != 0 {
		t.Errorf("caller writer got %q, want empty (DUMP buffers locally)", caller.String())
	}
	if got := out.Writer(); got != &caller {
		t.Errorf("caller OperationOutput.Writer() = %v after DUMP, want original buffer", got)
	}
	got, ok := result.Body.PreparedBytes()
	if !ok {
		t.Fatalf("result body kind = %v, want prepared bytes", result.Body)
	}
	if !bytes.Equal(got, ddl) {
		t.Errorf("prepared body = %q, want %q", got, ddl)
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

func TestMCPHandler_rejectsMetaCommands(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		statement         string
		skipSystemCommand bool
	}{
		{
			name:      "shell command",
			statement: `\! echo mcp-shell-probe`,
		},
		{
			name:              "shell command with skip system command",
			statement:         `\! echo mcp-shell-probe`,
			skipSystemCommand: true,
		},
		{
			name:      "output redirect",
			statement: `\o out.log`,
		},
		{
			name:      "tee output",
			statement: `\T out.log`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var global bytes.Buffer
			session := newDetachedTestSession(&global)
			session.systemVariables.Config.SkipSystemCommand = tt.skipSystemCommand
			cli := &Cli{
				SessionHandler:  NewSessionHandler(session),
				SystemVariables: session.systemVariables,
			}

			handler := executeStatementHandler(cli)
			result, _, err := handler(context.Background(), nil, ExecuteStatementArgs{Statement: tt.statement})
			if err != nil {
				t.Fatalf("handler: %v", err)
			}

			text := mcpResultText(t, result)
			if !strings.Contains(text, "ERROR: meta commands are not supported by MCP execute_statement") {
				t.Errorf("tool result = %q, want meta-command rejection", text)
			}
			if strings.Contains(text, "mcp-shell-probe") {
				t.Errorf("tool result = %q, shell command appears to have executed", text)
			}
			if global.Len() != 0 {
				t.Errorf("StreamManager writer got %q, want empty (would corrupt the MCP protocol stream)", global.String())
			}
		})
	}
}

func TestMCPOutputCaptureTruncates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		limit      int
		chunks     []string
		wantPrefix string
		notWant    string
	}{
		{
			name:       "partial chunk at limit",
			limit:      8,
			chunks:     []string{"12345", "67890"},
			wantPrefix: "12345678",
			notWant:    "90",
		},
		{
			name:       "discard writes after truncation",
			limit:      10,
			chunks:     []string{"12345", "67890", "overflow", "more"},
			wantPrefix: "1234567890",
			notWant:    "overflow",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			capture := newMCPOutputCapture(tt.limit)
			for _, chunk := range tt.chunks {
				n, err := capture.Write([]byte(chunk))
				if err != nil {
					t.Fatalf("Write(%q): %v", chunk, err)
				}
				if n != len(chunk) {
					t.Fatalf("Write(%q) = %d, want %d", chunk, n, len(chunk))
				}
			}

			got := capture.String()
			if !strings.HasPrefix(got, tt.wantPrefix) {
				t.Errorf("capture prefix = %q, want %q", got, tt.wantPrefix)
			}
			if !strings.Contains(got, "spanner-mycli MCP output truncated after") {
				t.Errorf("capture = %q, want truncation marker", got)
			}
			if strings.Contains(got, tt.notWant) {
				t.Errorf("capture = %q, should not contain discarded content %q", got, tt.notWant)
			}
		})
	}
}

func TestProgressWithTTYDisabledWithoutTTY(t *testing.T) {
	t.Parallel()

	var global bytes.Buffer
	session := newDetachedTestSession(&global)
	session.systemVariables.Display.EnableProgressBar = true

	progress := newProgressWithTTY(context.Background(), session)
	if progress != nil {
		progress.Wait()
		t.Fatal("newProgressWithTTY returned a progress container without a TTY")
	}
	if global.Len() != 0 {
		t.Errorf("StreamManager writer got %q, want empty", global.String())
	}
}
