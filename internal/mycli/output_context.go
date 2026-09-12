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
	"math"
)

// OperationOutput is the per-statement output destination for streamed results
// (streaming formats, DUMP, shell). It is resolved once at the statement entry
// point (Cli.executeStatement, which also serves the MCP handler through its
// writer argument, and Session.ExecuteStatement for direct callers) and
// carried through Execute and every nested execution path.
//
// Before this type existed, streaming paths wrote to the process-global
// StreamManager writer regardless of which writer the caller passed to
// Cli.executeStatement. Under --mcp that writer is the JSON-RPC stdout, so
// any streamed statement corrupted the protocol stream. The caller-provided
// writer is authoritative on every path. A statement that does not emit
// output ignores the value.
type OperationOutput struct {
	// w receives streamed statement output. When nil after entry-point
	// resolution, streaming paths treat it as "buffer instead".
	w io.Writer

	// screenWidth resolves the display width for streamed table rendering.
	// It is a function rather than a value so the terminal size is read when
	// rendering starts, not when the statement was submitted. Width is
	// resolved against the original destination, not a pager pipe.
	screenWidth func() int
}

// Writer returns the destination for streamed statement output. Nil means
// streaming paths should buffer instead.
func (o OperationOutput) Writer() io.Writer {
	return o.w
}

// ScreenWidth returns the screen width for streamed rendering. When no
// resolver is set, wrapping is disabled.
func (o OperationOutput) ScreenWidth() int {
	if o.screenWidth != nil {
		return o.screenWidth()
	}
	return math.MaxInt
}

// withWriter returns a copy that writes to w while keeping the original lazy
// width resolver. DUMP uses this for internal buffering so the outer
// publication destination and width stay those of the caller.
func (o OperationOutput) withWriter(w io.Writer) OperationOutput {
	o.w = w
	return o
}

// resolveOperationOutput fills a nil writer from StreamManager and a nil
// width resolver from the live session settings. Caller-provided fields win.
// Width stays lazy so the terminal is measured at render time against the
// original destination. Streaming helpers also call this so a direct
// Execute with a zero OperationOutput still sees StreamManager, matching
// the former session.outputWriter() fallback without mutating Session.
func (s *Session) resolveOperationOutput(out OperationOutput) OperationOutput {
	if out.w == nil && s != nil && s.systemVariables != nil && s.systemVariables.StreamManager != nil {
		out.w = s.systemVariables.StreamManager.GetWriter()
	}
	if out.screenWidth == nil {
		var sysVars *systemVariables
		if s != nil {
			sysVars = s.systemVariables
		}
		out.screenWidth = func() int {
			if sysVars == nil {
				return math.MaxInt
			}
			return displayScreenWidth(sysVars)
		}
	}
	return out
}

// ExecuteStatementWithOutput executes stmt with out as the per-statement
// output destination. Fallback writer and width are resolved here; nested
// execution (e.g. RUN BATCH) must forward the same value rather than re-entering
// ExecuteStatement, which would rebuild a default destination.
func (s *Session) ExecuteStatementWithOutput(ctx context.Context, stmt Statement, out OperationOutput) (*Result, error) {
	return s.executeStatement(ctx, stmt, s.resolveOperationOutput(out))
}

// ExecuteStatementWithOutput executes a statement like ExecuteStatement,
// routing streamed output to out. Session-changing statements (USE/DETACH)
// produce no streamed output and take their normal path.
func (h *SessionHandler) ExecuteStatementWithOutput(ctx context.Context, stmt Statement, out OperationOutput) (*Result, error) {
	switch stmt.(type) {
	case *UseStatement, *UseDatabaseMetaCommand, *DetachStatement:
		return h.ExecuteStatement(ctx, stmt)
	default:
		return h.Session.ExecuteStatementWithOutput(ctx, stmt, out)
	}
}
