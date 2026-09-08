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
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apstndb/spanner-mycli/internal/mycli"
	"github.com/apstndb/spanner-mycli/internal/mycli/feature/all"
)

func writeTempConfig(t *testing.T, extra string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".spanner_mycli.toml")
	body := "project = \"p\"\ninstance = \"i\"\ndatabase = \"d\"\n" + extra
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFullEntrypointAcceptsOptionalFlagAndConfig(t *testing.T) {
	t.Parallel()
	cfg := writeTempConfig(t, "")
	if err := mycli.ParseFlagsArgsForTest(
		[]string{"--vertexai-model=gemini-test"},
		"test",
		[]string{cfg},
		io.Discard,
		io.Discard,
		all.All()...,
	); err != nil {
		t.Fatalf("full --vertexai-model: %v", err)
	}

	cfgTOML := writeTempConfig(t, "vertexai-model = \"gemini-test\"\n")
	if err := mycli.ParseFlagsArgsForTest(
		nil,
		"test",
		[]string{cfgTOML},
		io.Discard,
		io.Discard,
		all.All()...,
	); err != nil {
		t.Fatalf("full vertexai-model TOML: %v", err)
	}
}

func TestSlimEntrypointRejectsOptionalFlagAndConfig(t *testing.T) {
	t.Parallel()
	cfg := writeTempConfig(t, "")
	err := mycli.ParseFlagsArgsForTest(
		[]string{"--vertexai-model=gemini-test"},
		"test",
		[]string{cfg},
		io.Discard,
		io.Discard,
	)
	if err == nil || !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("slim --vertexai-model error = %v, want unknown flag", err)
	}

	cfgTOML := writeTempConfig(t, "vertexai-model = \"gemini-test\"\n")
	err = mycli.ParseFlagsArgsForTest(
		nil,
		"test",
		[]string{cfgTOML},
		io.Discard,
		io.Discard,
	)
	if err == nil || !strings.Contains(err.Error(), "unknown configuration keys") {
		t.Fatalf("slim vertexai-model TOML error = %v, want unknown configuration keys", err)
	}
}

func TestSlimEntrypointKeepsCoreFlagAndStatements(t *testing.T) {
	t.Parallel()
	cfg := writeTempConfig(t, "")
	if err := mycli.ParseFlagsArgsForTest(
		[]string{"--mcp"},
		"test",
		[]string{cfg},
		io.Discard,
		io.Discard,
	); err != nil {
		t.Fatalf("slim --mcp: %v", err)
	}

	defs := mycli.MergedStatementDefs()
	for _, input := range []string{"HELP", "SHOW VARIABLES"} {
		if _, err := mycli.BuildStatementWithDefs(defs, input); err != nil {
			t.Errorf("slim core statement %q: %v", input, err)
		}
	}
}

func TestOptionalVariablesPresentOnlyWithFeatures(t *testing.T) {
	t.Parallel()
	full := mycli.NewSessionWithFeaturesForTest(t, all.All()...)
	if _, ok := mycli.ListVariablesForTest(full)["CLI_VERTEXAI_MODEL"]; !ok {
		t.Fatal("full listing missing CLI_VERTEXAI_MODEL")
	}

	slim := mycli.NewSessionWithFeaturesForTest(t)
	if _, ok := mycli.ListVariablesForTest(slim)["CLI_VERTEXAI_MODEL"]; ok {
		t.Fatal("slim listing includes CLI_VERTEXAI_MODEL")
	}
	if _, ok := mycli.ListVariablesForTest(slim)["CLI_FORMAT"]; !ok {
		t.Fatal("slim listing missing core CLI_FORMAT")
	}
}
