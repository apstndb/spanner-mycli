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

package llm

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/apstndb/spanner-mycli/internal/mycli"
)

func TestFeatureDispatchAndEnumVars(t *testing.T) {
	t.Parallel()

	feat := Feature()
	if feat.Name != "GEMINI" {
		t.Fatalf("Name = %q, want GEMINI", feat.Name)
	}
	if feat.KongVars["defaultVertexAIModel"] != defaultVertexAIModel {
		t.Errorf("KongVars defaultVertexAIModel = %q, want %q", feat.KongVars["defaultVertexAIModel"], defaultVertexAIModel)
	}
	if feat.KongVars["defaultVertexAILocation"] != defaultVertexAILocation {
		t.Errorf("KongVars defaultVertexAILocation = %q, want %q", feat.KongVars["defaultVertexAILocation"], defaultVertexAILocation)
	}

	defs := mycli.MergedStatementDefs(feat)
	stmt, err := mycli.BuildStatementWithDefs(defs, `GEMINI "list tables"`)
	if err != nil {
		t.Fatalf("BuildStatementWithDefs() error = %v", err)
	}
	gs, ok := stmt.(*GeminiStatement)
	if !ok {
		t.Fatalf("dispatch returned %T, want *GeminiStatement", stmt)
	}
	if gs.Text != "list tables" {
		t.Fatalf("Text = %q, want unquoted prompt", gs.Text)
	}
	if gs.cfg == nil {
		t.Fatal("statement config is nil")
	}

	vars := featureVars(t, feat)
	backend := vars["CLI_GENAI_BACKEND"]
	if got, err := backend.Get(); err != nil || got != genAIBackendEnterprise {
		t.Fatalf("CLI_GENAI_BACKEND Get() = %q, %v, want %q", got, err, genAIBackendEnterprise)
	}
	if err := backend.Set("vertex_ai"); err != nil {
		t.Fatalf("Set(vertex_ai) error = %v", err)
	}
	if got, _ := backend.Get(); got != genAIBackendEnterprise {
		t.Errorf("CLI_GENAI_BACKEND after alias = %q, want %s", got, genAIBackendEnterprise)
	}
	if err := backend.Set("gemini_api"); err != nil {
		t.Fatalf("Set(gemini_api) error = %v", err)
	}
	if got, _ := backend.Get(); got != genAIBackendGeminiAPI {
		t.Errorf("CLI_GENAI_BACKEND = %q, want %s", got, genAIBackendGeminiAPI)
	}
	if err := backend.Set("OTHER"); err == nil {
		t.Fatal("Set(OTHER) error = nil")
	} else if !strings.Contains(err.Error(), "GEMINI_ENTERPRISE") {
		t.Errorf("invalid Set error = %v, want listed values", err)
	}

	enum, ok := backend.(interface{ ValidValues() []string })
	if !ok {
		t.Fatal("CLI_GENAI_BACKEND does not expose ValidValues")
	}
	wantValues := []string{"'GEMINI_ENTERPRISE'", "'GEMINI_API'"}
	if diff := cmp.Diff(wantValues, enum.ValidValues()); diff != "" {
		t.Errorf("ValidValues mismatch (-want +got):\n%s", diff)
	}

	thinking := vars["CLI_GENAI_THINKING_LEVEL"]
	if err := thinking.Set("high"); err != nil {
		t.Fatalf("Set(high) error = %v", err)
	}
	if got, _ := thinking.Get(); got != "HIGH" {
		t.Errorf("CLI_GENAI_THINKING_LEVEL = %q, want HIGH", got)
	}
}

func TestFeatureApplyFlags(t *testing.T) {
	t.Parallel()

	t.Run("unset pointers skip model and location", func(t *testing.T) {
		t.Parallel()
		feat := Feature()
		got := map[string]string{}
		if err := feat.ApplyFlags(func(name, value string) error {
			got[name] = value
			return nil
		}); err != nil {
			t.Fatalf("ApplyFlags() error = %v", err)
		}
		want := map[string]string{"CLI_VERTEXAI_PROJECT": ""}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("ApplyFlags set calls mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("explicit flags route through setter", func(t *testing.T) {
		t.Parallel()
		feat := Feature()
		f := feat.Flags.(*flags)
		model := "gemini-test"
		location := "us-central1"
		f.VertexAIProject = "override-proj"
		f.VertexAIModel = &model
		f.VertexAILocation = &location

		got := map[string]string{}
		var order []string
		if err := feat.ApplyFlags(func(name, value string) error {
			got[name] = value
			order = append(order, name)
			return nil
		}); err != nil {
			t.Fatalf("ApplyFlags() error = %v", err)
		}
		want := map[string]string{
			"CLI_VERTEXAI_PROJECT":  "override-proj",
			"CLI_VERTEXAI_MODEL":    "gemini-test",
			"CLI_VERTEXAI_LOCATION": "us-central1",
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("ApplyFlags values mismatch (-want +got):\n%s", diff)
		}
		if !slices.Equal(order, []string{"CLI_VERTEXAI_PROJECT", "CLI_VERTEXAI_MODEL", "CLI_VERTEXAI_LOCATION"}) {
			t.Errorf("ApplyFlags order = %v", order)
		}
	})

	t.Run("setter error is returned", func(t *testing.T) {
		t.Parallel()
		feat := Feature()
		err := feat.ApplyFlags(func(name, value string) error {
			return errors.New("set failed")
		})
		if err == nil || err.Error() != "set failed" {
			t.Fatalf("ApplyFlags() error = %v, want set failed", err)
		}
	})
}

func featureVars(t *testing.T, feat mycli.Feature) map[string]mycli.Variable {
	t.Helper()
	vars := make(map[string]mycli.Variable, len(feat.Vars))
	for _, fv := range feat.Vars {
		vars[fv.Name] = fv.Var
	}
	return vars
}
