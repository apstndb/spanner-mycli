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

package mycli_test

import (
	"strings"
	"testing"

	"github.com/apstndb/spanner-mycli/internal/mycli"
	"github.com/apstndb/spanner-mycli/internal/mycli/feature/bigquery"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPReadOnlyGuardBigQuery(t *testing.T) {
	mycli.UseStatementDefsForTest(t, mycli.MergedStatementDefs(bigquery.Feature()))

	const (
		readOnlyErr    = "can't execute this statement in READONLY mode"
		missingProject = "BigQuery project not configured"
		mutatingScript = "BIGQUERY SELECT 1; DELETE FROM dataset.table WHERE TRUE;"
		readOnlyScript = "BIGQUERY SELECT 1;"
		overlapScript  = "BIGQUERY SELECT 1 /*/ ' */; DELETE FROM `dataset.table` WHERE TRUE; -- '"
	)

	t.Run("READONLY mutating script", func(t *testing.T) {
		session := mycli.NewReadOnlySessionForTest(t)
		got, isError := callMCPExecuteStatement(t, session, mutatingScript)
		if !isError {
			t.Fatalf("IsError=false, want true; got %q", got)
		}
		if !strings.Contains(got, readOnlyErr) {
			t.Fatalf("got %q, want READONLY error", got)
		}
		if strings.Contains(got, missingProject) {
			t.Fatalf("mutating script reached feature execution: %q", got)
		}
	})

	t.Run("READONLY query script", func(t *testing.T) {
		session := mycli.NewReadOnlySessionForTest(t)
		got, isError := callMCPExecuteStatement(t, session, readOnlyScript)
		if !isError {
			t.Fatalf("IsError=false, want true; got %q", got)
		}
		if strings.Contains(got, readOnlyErr) {
			t.Fatalf("query script wrongly blocked by READONLY: %q", got)
		}
		if !strings.Contains(got, missingProject) {
			t.Fatalf("got %q, want missing BigQuery project", got)
		}
	})

	t.Run("READONLY false mutating script", func(t *testing.T) {
		session := mycli.NewSessionForTest(t)
		got, isError := callMCPExecuteStatement(t, session, mutatingScript)
		if !isError {
			t.Fatalf("IsError=false, want true; got %q", got)
		}
		if strings.Contains(got, readOnlyErr) {
			t.Fatalf("READONLY=false wrongly blocked: %q", got)
		}
		if !strings.Contains(got, missingProject) {
			t.Fatalf("got %q, want missing BigQuery project", got)
		}
	})

	t.Run("READONLY overlapping comment", func(t *testing.T) {
		session := mycli.NewReadOnlySessionForTest(t)
		got, isError := callMCPExecuteStatement(t, session, overlapScript)
		if !isError {
			t.Fatalf("IsError=false, want true; got %q", got)
		}
		if !strings.Contains(got, readOnlyErr) {
			t.Fatalf("got %q, want READONLY error", got)
		}
		if strings.Contains(got, missingProject) {
			t.Fatalf("overlapping comment reached feature execution: %q", got)
		}
	})

	t.Run("READONLY false overlapping comment", func(t *testing.T) {
		session := mycli.NewSessionForTest(t)
		got, isError := callMCPExecuteStatement(t, session, overlapScript)
		if !isError {
			t.Fatalf("IsError=false, want true; got %q", got)
		}
		if strings.Contains(got, readOnlyErr) {
			t.Fatalf("READONLY=false wrongly blocked: %q", got)
		}
		if !strings.Contains(got, missingProject) {
			t.Fatalf("got %q, want missing BigQuery project", got)
		}
	})

	t.Run("READONLY local HELP", func(t *testing.T) {
		session := mycli.NewReadOnlySessionForTest(t)
		got, isError := callMCPExecuteStatement(t, session, "HELP")
		if isError {
			t.Fatalf("HELP IsError=true, want false; got %q", got)
		}
		if !strings.Contains(got, "Usage") {
			t.Fatalf("HELP output missing Usage: %q", got)
		}
	})
}

func callMCPExecuteStatement(t *testing.T, session *mycli.Session, statement string) (string, bool) {
	t.Helper()
	ctx := t.Context()
	client, _, err := mycli.SetupMCPClientServerForTest(t, ctx, session)
	if err != nil {
		t.Fatalf("SetupMCPClientServerForTest: %v", err)
	}
	result, err := client.CallTool(ctx, &mcp.CallToolParams{
		Name: "execute_statement",
		Arguments: map[string]any{
			"statement": statement,
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	var got string
	isError := result != nil && result.IsError
	if result != nil {
		for _, content := range result.Content {
			if text, ok := content.(*mcp.TextContent); ok {
				got = text.Text
				break
			}
		}
	}
	return got, isError
}
