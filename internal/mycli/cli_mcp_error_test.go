// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mycli

import (
	"io"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPExecuteStatementIsError(t *testing.T) {
	t.Parallel()
	session := newDetachedTestSession(io.Discard)
	client, _, err := setupMCPClientServer(t, t.Context(), session)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name      string
		sql       string
		wantError bool
	}{
		{name: "success HELP", sql: "HELP", wantError: false},
		{name: "parse failure", sql: "SHOW QUERY PROFILE nope", wantError: true},
		{name: "meta command policy", sql: `\q`, wantError: true},
		{name: "execution failure", sql: "SET CLI_FORMAT = 'not-a-format'", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := client.CallTool(t.Context(), &mcp.CallToolParams{
				Name:      "execute_statement",
				Arguments: map[string]any{"statement": tc.sql},
			})
			if err != nil {
				t.Fatalf("unexpected protocol error: %v", err)
			}
			if result == nil {
				t.Fatal("nil tool result")
			}
			text := mcpResultText(t, result)
			if result.IsError != tc.wantError {
				t.Fatalf("IsError=%v want %v; text=%q", result.IsError, tc.wantError, text)
			}
			if tc.wantError {
				if !strings.HasPrefix(text, "ERROR:") {
					t.Fatalf("error text %q, want ERROR: prefix", text)
				}
			} else if strings.HasPrefix(text, "ERROR:") {
				t.Fatalf("success text unexpectedly starts with ERROR: %q", text)
			}
		})
	}

	t.Run("unknown tool is protocol error", func(t *testing.T) {
		_, err := client.CallTool(t.Context(), &mcp.CallToolParams{
			Name:      "audit_nonexistent_tool",
			Arguments: map[string]any{},
		})
		if err == nil {
			t.Fatal("expected protocol error for missing tool")
		}
	})
}
