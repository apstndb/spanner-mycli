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
	"bytes"
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

func TestSlimAndFullAcceptCustomTLSFlags(t *testing.T) {
	t.Parallel()
	cfg := writeTempConfig(t, "")
	args := []string{
		"--endpoint=omni.example:443",
		"--ca-cert-file=/tmp/ca.pem",
		"--client-cert-file=/tmp/client.pem",
		"--client-cert-key=/tmp/client.key",
		"--without-authentication",
	}
	if err := mycli.ParseFlagsArgsForTest(args, "test", []string{cfg}, io.Discard, io.Discard); err != nil {
		t.Fatalf("slim custom TLS flags: %v", err)
	}
	if err := mycli.ParseFlagsArgsForTest(args, "test", []string{cfg}, io.Discard, io.Discard, all.All()...); err != nil {
		t.Fatalf("full custom TLS flags: %v", err)
	}

	cfgTOML := writeTempConfig(t, "endpoint = \"omni.example:443\"\nca-cert-file = \"/tmp/ca.pem\"\nclient-cert-file = \"/tmp/client.pem\"\nclient-cert-key = \"/tmp/client.key\"\nwithout-authentication = true\n")
	if err := mycli.ParseFlagsArgsForTest(nil, "test", []string{cfgTOML}, io.Discard, io.Discard); err != nil {
		t.Fatalf("slim custom TLS TOML: %v", err)
	}
	if err := mycli.ParseFlagsArgsForTest(nil, "test", []string{cfgTOML}, io.Discard, io.Discard, all.All()...); err != nil {
		t.Fatalf("full custom TLS TOML: %v", err)
	}

	for _, name := range []string{"slim", "full"} {
		var stdout bytes.Buffer
		var features []mycli.Feature
		if name == "full" {
			features = all.All()
		}
		err := mycli.ParseFlagsArgsForTest([]string{"--help"}, "test", []string{cfg}, &stdout, io.Discard, features...)
		if stdout.Len() == 0 {
			t.Fatalf("%s help produced no output: %v", name, err)
		}
		help := stdout.String()
		for _, want := range []string{"--ca-cert-file", "--client-cert-file", "--client-cert-key", "--without-authentication", "Permit plaintext gRPC"} {
			if !strings.Contains(help, want) {
				t.Errorf("%s help missing %q", name, want)
			}
		}
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
	for _, input := range []string{"HELP", "SHOW VARIABLES", "RESET ALL", "RESET CLI_VERBOSE", "SHOW TRANSACTION ISOLATION LEVEL", "SHOW TRANSACTION READ ONLY"} {
		if _, err := mycli.BuildStatementWithDefs(defs, input); err != nil {
			t.Errorf("slim core statement %q: %v", input, err)
		}
	}
}

func TestInitializeSystemVariablesCapturesFeatureVars(t *testing.T) {
	t.Parallel()
	if err := mycli.InitializeSystemVariablesForTest(all.All()...); err != nil {
		t.Fatalf("initializeSystemVariables with features: %v", err)
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
