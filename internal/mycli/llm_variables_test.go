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

// External-package coverage for the extracted GEMINI/LLM variables (#778 PR3),
// replacing the CLI_VERTEXAI_* cases that lived in the core
// system_variables_test.go before the move: enumeration (SHOW VARIABLES /
// completion source), defaults, and Set/Get round-trips with the real llm
// feature registered.

import (
	"testing"

	"github.com/apstndb/spanner-mycli/internal/mycli"
	"github.com/apstndb/spanner-mycli/internal/mycli/feature/llm"
)

func TestLLMVariables(t *testing.T) {
	t.Parallel()

	session := mycli.NewSessionWithFeaturesForTest(t, llm.Feature())
	listed := mycli.ListVariablesForTest(session)

	// Enumeration + defaults: the pre-extraction defaults must be reported
	// unchanged (SHOW VARIABLES and generated docs depend on this).
	for name, wantDefault := range map[string]string{
		"CLI_VERTEXAI_PROJECT":  "",
		"CLI_VERTEXAI_MODEL":    "gemini-3-flash-preview",
		"CLI_VERTEXAI_LOCATION": "global",
	} {
		got, ok := listed[name]
		if !ok {
			t.Errorf("variable listing does not include %s", name)
			continue
		}
		if got != wantDefault {
			t.Errorf("%s default = %q, want %q", name, got, wantDefault)
		}
	}

	// Set/Get round-trip through the registry (replaces the removed core
	// stringTests cases for CLI_VERTEXAI_PROJECT/MODEL).
	for name, value := range map[string]string{
		"CLI_VERTEXAI_PROJECT":  "example-project",
		"CLI_VERTEXAI_MODEL":    "test",
		"CLI_VERTEXAI_LOCATION": "us-central1",
	} {
		if err := mycli.SetVariableForTest(session, name, value); err != nil {
			t.Errorf("Set(%s, %q) error: %v", name, value, err)
			continue
		}
		if got := mycli.ListVariablesForTest(session)[name]; got != value {
			t.Errorf("after Set, %s = %q, want %q", name, got, value)
		}
	}
}

// TestGeminiStatementDispatch relocates the pre-extraction GEMINI parse case
// (statement_processing_test.go) to the merged def table: the dispatch-built
// statement must be an *llm.GeminiStatement with the prompt unquoted, and it
// must keep implementing NO marker interfaces — not a static MutationStatement,
// not conditionally mutating, and not detached-compatible — exactly like the
// pre-extraction in-core type.
func TestGeminiStatementDispatch(t *testing.T) {
	t.Parallel()

	feat := llm.Feature()
	defs := mycli.MergedStatementDefs(feat)
	stmt, err := mycli.BuildStatementWithDefs(defs, `GEMINI "make a query"`)
	if err != nil {
		t.Fatalf("BuildStatementWithDefs(GEMINI ...) error: %v", err)
	}
	gs, ok := stmt.(*llm.GeminiStatement)
	if !ok {
		t.Fatalf("dispatch returned %T, want *llm.GeminiStatement", stmt)
	}
	if gs.Text != "make a query" {
		t.Errorf("GeminiStatement.Text = %q, want unquoted prompt", gs.Text)
	}

	if _, isMutation := stmt.(mycli.MutationStatement); isMutation {
		t.Error("GeminiStatement must not be a static MutationStatement")
	}
	if conditional, _ := mycli.ClassifyForTest(stmt); conditional {
		t.Error("GeminiStatement must not be a ConditionallyMutatingStatement")
	}
	if _, detached := stmt.(mycli.DetachedCompatible); detached {
		t.Error("GeminiStatement must not be DetachedCompatible")
	}
}
