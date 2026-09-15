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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/nyaosorg/go-readline-ny/simplehistory"
	"github.com/spf13/afero"

	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanner-mycli/internal/mycli/streamio"
)

func TestBuildStatement_ShowHistoryNotClientSide(t *testing.T) {
	t.Parallel()
	for _, input := range []string{
		"CLEAR HISTORY",
		"SEARCH HISTORY",
		"SHOW HISTORY FOO",
		"SHOW HISTORY LIMIT",
		"SHOW HISTORY LIMIT -1",
		"SHOW HISTORY LIMIT 1.5",
	} {
		t.Run(input, func(t *testing.T) {
			got, err := BuildStatement(input)
			if err == nil {
				if _, ok := got.(*ShowHistoryStatement); ok {
					t.Fatalf("BuildStatement(%q) = %#v, want non-SHOW-HISTORY", input, got)
				}
			}
		})
	}
}

func TestShowHistory_liveInteractiveIncludesSelfRow(t *testing.T) {
	t.Parallel()

	session := newShowHistoryTestSession(t, "")
	h := mustMemHistory(t)
	if err := seedHistory(h, "SELECT 1;", "SHOW HISTORY;"); err != nil {
		t.Fatal(err)
	}
	session.systemVariables.interactiveHistory = h

	result, err := (&ShowHistoryStatement{}).Execute(t.Context(), session, OperationOutput{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	assertHistoryResult(t, result, []string{"SELECT 1;", "SHOW HISTORY;"})
}

func TestShowHistory_limitIsRecentTail(t *testing.T) {
	t.Parallel()

	session := newShowHistoryTestSession(t, "")
	h := mustMemHistory(t)
	if err := seedHistory(h, "SELECT 1;", "SELECT 2;", "SELECT 3;", "SELECT 4;", "SHOW HISTORY LIMIT 2;"); err != nil {
		t.Fatal(err)
	}
	session.systemVariables.interactiveHistory = h

	result, err := (&ShowHistoryStatement{Limit: 2}).Execute(t.Context(), session, OperationOutput{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	assertHistoryResult(t, result, []string{"SELECT 4;", "SHOW HISTORY LIMIT 2;"})
}

func TestShowHistory_missingFileEmpty(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "missing-history")
	session := newShowHistoryTestSession(t, path)

	result, err := (&ShowHistoryStatement{}).Execute(t.Context(), session, OperationOutput{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	assertHistoryResult(t, result, []string{})
}

func TestShowHistory_fileLoadChronologicalNoRewrite(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "history")
	original := []byte("\n\"SELECT 1;\"\n\n\"SELECT 2;\"\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	session := newShowHistoryTestSession(t, path)

	result, err := (&ShowHistoryStatement{}).Execute(t.Context(), session, OperationOutput{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	assertHistoryResult(t, result, []string{"SELECT 1;", "SELECT 2;"})

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(original, got); diff != "" {
		t.Fatalf("history file rewritten (-want +got):\n%s", diff)
	}
}

func TestShowHistory_malformedFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "history")
	original := []byte("not-a-quoted-record\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	session := newShowHistoryTestSession(t, path)

	_, err := (&ShowHistoryStatement{}).Execute(t.Context(), session, OperationOutput{})
	if err == nil {
		t.Fatal("Execute: want history file format error")
	}
	if !strings.Contains(err.Error(), "history file format error") {
		t.Fatalf("Execute error = %v, want format error", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(original, got); diff != "" {
		t.Fatalf("malformed history file rewritten (-want +got):\n%s", diff)
	}
}

func TestShowHistory_detachedCompatible(t *testing.T) {
	t.Parallel()

	session := newShowHistoryTestSession(t, filepath.Join(t.TempDir(), "history"))
	if err := session.ValidateStatementExecution(&ShowHistoryStatement{}); err != nil {
		t.Fatalf("ValidateStatementExecution: %v", err)
	}
}

func TestShowHistory_csvStream(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	session := newShowHistoryTestSession(t, filepath.Join(t.TempDir(), "history"))
	session.systemVariables.Display.CLIFormat = enums.DisplayModeCSV
	session.systemVariables.StreamManager = streamio.NewStreamManager(nil, &out, ioDiscard())

	h := mustMemHistory(t)
	if err := seedHistory(h, "SELECT 1;"); err != nil {
		t.Fatal(err)
	}
	session.systemVariables.interactiveHistory = h

	result, err := session.ExecuteStatementWithOutput(context.Background(), &ShowHistoryStatement{}, OperationOutput{w: &out})
	if err != nil {
		t.Fatalf("ExecuteStatementWithOutput: %v", err)
	}
	if result == nil || !result.KeepVariables {
		t.Fatal("expected KeepVariables")
	}
	if result.AffectedRows != 1 {
		t.Fatalf("AffectedRows = %d, want 1", result.AffectedRows)
	}
	got := out.String()
	if !strings.Contains(got, "statement") || !strings.Contains(got, "SELECT 1;") {
		t.Fatalf("CSV output %q, want header and row", got)
	}
}

func newShowHistoryTestSession(t *testing.T, historyFile string) *Session {
	t.Helper()
	session := newDetachedTestSession(ioDiscard())
	if historyFile == "" {
		historyFile = filepath.Join(t.TempDir(), "unused-history")
	}
	session.systemVariables.Display.HistoryFile = historyFile
	session.systemVariables.Display.CLIFormat = enums.DisplayModeTable
	return session
}

func mustMemHistory(t *testing.T) History {
	t.Helper()
	h, err := newPersistentHistoryWithFS("history", simplehistory.New(), afero.NewMemMapFs())
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func seedHistory(h History, stmts ...string) error {
	for _, stmt := range stmts {
		h.Add(stmt)
	}
	return nil
}

func assertHistoryResult(t *testing.T, result *Result, want []string) {
	t.Helper()
	if result == nil {
		t.Fatal("nil result")
	}
	if !result.KeepVariables {
		t.Fatal("KeepVariables = false")
	}
	if result.AffectedRows != len(want) {
		t.Fatalf("AffectedRows = %d, want %d", result.AffectedRows, len(want))
	}
	if result.TableHeader == nil {
		t.Fatal("nil TableHeader")
	}
	names := result.TableHeader.Render(false)
	if len(names) != 1 || names[0] != "statement" {
		t.Fatalf("columns = %v, want [statement]", names)
	}
	got := historyStatementsFromResult(t, result)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("rows mismatch (-want +got):\n%s", diff)
	}
}

func historyStatementsFromResult(t *testing.T, result *Result) []string {
	t.Helper()
	typed, ok := result.Body.Typed()
	if !ok || typed == nil {
		if result.AffectedRows == 0 {
			return nil
		}
		t.Fatal("expected typed body")
	}
	got := make([]string, 0, len(typed.Rows))
	for _, row := range typed.Rows {
		var s string
		if err := row.Column(0, &s); err != nil {
			t.Fatal(err)
		}
		got = append(got, s)
	}
	return got
}

func ioDiscard() *bytes.Buffer {
	return &bytes.Buffer{}
}
