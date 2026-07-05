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
	"context"
	"io"
)

// outputContext is the per-statement output destination for streamed results
// (streaming formats, DUMP). It is resolved once at the statement entry point
// (Cli.executeStatement, which also serves the MCP handler through its writer
// argument) and carried by the Session for the duration of one statement.
//
// Before this type existed, streaming paths wrote to the process-global
// StreamManager writer regardless of which writer the caller passed to
// Cli.executeStatement. Under --mcp that writer is the JSON-RPC stdout, so
// any streamed statement corrupted the protocol stream. Routing all statement
// output through the session's outputContext makes the caller-provided writer
// authoritative on every path.
type outputContext struct {
	// w receives streamed statement output. When nil, the session falls back
	// to the StreamManager writer (direct Session.ExecuteStatement callers,
	// e.g. tests).
	w io.Writer

	// screenWidth resolves the display width for streamed table rendering.
	// It is a function rather than a value so the terminal size is read when
	// rendering starts, not when the statement was submitted. When nil, the
	// session falls back to displayScreenWidth.
	screenWidth func() int
}

// outputWriter returns the destination for streamed statement output: the
// per-statement outputContext writer when one is set, otherwise the
// StreamManager writer. May return nil (no output destination); streaming
// paths treat nil as "buffer instead".
func (s *Session) outputWriter() io.Writer {
	if s.output.w != nil {
		return s.output.w
	}
	if s.systemVariables != nil && s.systemVariables.StreamManager != nil {
		return s.systemVariables.StreamManager.GetWriter()
	}
	return nil
}

// displayWidthFor returns the screen width for streamed rendering. sysVars is
// passed explicitly because query execution may run with a per-statement copy
// carrying format overrides (see executeSQLWithFormatAndTxn).
func (s *Session) displayWidthFor(sysVars *systemVariables) int {
	if s.output.screenWidth != nil {
		return s.output.screenWidth()
	}
	return displayScreenWidth(sysVars)
}

// withOutput runs fn with the session's per-statement output redirected to
// out, restoring the previous destination afterwards. Statement execution is
// serialized per session (single-goroutine REPL/batch loops; the MCP handler
// holds a mutex around each call), so plain save/restore is safe here.
func (s *Session) withOutput(out outputContext, fn func() error) error {
	prev := s.output
	s.output = out
	defer func() {
		s.output = prev
	}()
	return fn()
}

// ExecuteStatementWithOutput executes stmt with out as the per-statement
// output destination. Statements executed re-entrantly during stmt (e.g.
// RUN BATCH re-entering ExecuteStatement) inherit the same destination.
func (s *Session) ExecuteStatementWithOutput(ctx context.Context, stmt Statement, out outputContext) (*Result, error) {
	var result *Result
	err := s.withOutput(out, func() error {
		var err error
		result, err = s.ExecuteStatement(ctx, stmt)
		return err
	})
	return result, err
}

// ExecuteStatementWithOutput executes a statement like ExecuteStatement,
// routing streamed output to out. Session-changing statements (USE/DETACH)
// produce no streamed output and take their normal path.
func (h *SessionHandler) ExecuteStatementWithOutput(ctx context.Context, stmt Statement, out outputContext) (*Result, error) {
	switch stmt.(type) {
	case *UseStatement, *UseDatabaseMetaCommand, *DetachStatement:
		return h.ExecuteStatement(ctx, stmt)
	default:
		return h.Session.ExecuteStatementWithOutput(ctx, stmt, out)
	}
}
