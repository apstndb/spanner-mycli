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

// Mechanism tests for feature-contributed CLI flags (issue #778 §4.5): flags
// carried via kong.Plugins must participate in the TOML config resolver and in
// CLI-flag precedence exactly like core flags. First user is GEMINI's
// --vertexai-* family (feature/llm); these tests pin the seam with a synthetic
// feature so the guarantee outlives any single feature.

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

type featureFlagProbe struct {
	Scalar  string  `name:"probe-scalar" help:"probe scalar flag"`
	Pointer *string `name:"probe-pointer" help:"probe pointer flag"`
}

func parseProbeFeature(t *testing.T, args []string, tomlBody string) *featureFlagProbe {
	t.Helper()
	pf := &featureFlagProbe{}
	feat := Feature{Name: "PROBE", Flags: pf}

	var configFiles []string
	if tomlBody != "" {
		cfg := filepath.Join(t.TempDir(), ".spanner_mycli.toml")
		if err := os.WriteFile(cfg, []byte(tomlBody), 0o644); err != nil {
			t.Fatal(err)
		}
		configFiles = []string{cfg}
	}
	if _, _, err := parseFlagsArgs(args, "", configFiles, io.Discard, io.Discard, feat); err != nil {
		t.Fatalf("parseFlagsArgs: %v", err)
	}
	return pf
}

const probeTOMLBase = "project = \"p\"\ninstance = \"i\"\ndatabase = \"d\"\n"

func TestFeatureFlagsResolveFromTOML(t *testing.T) {
	t.Parallel()

	pf := parseProbeFeature(t, nil,
		probeTOMLBase+"probe-scalar = \"toml-scalar\"\nprobe-pointer = \"toml-pointer\"\n")
	if pf.Scalar != "toml-scalar" {
		t.Errorf("TOML did not populate plugin scalar flag: %q", pf.Scalar)
	}
	if pf.Pointer == nil || *pf.Pointer != "toml-pointer" {
		t.Errorf("TOML did not populate plugin pointer flag: %v", pf.Pointer)
	}
}

func TestFeatureFlagsCLIOverridesTOML(t *testing.T) {
	t.Parallel()

	pf := parseProbeFeature(t,
		[]string{"--probe-scalar=cli-scalar", "--probe-pointer=cli-pointer"},
		probeTOMLBase+"probe-scalar = \"toml-scalar\"\nprobe-pointer = \"toml-pointer\"\n")
	if pf.Scalar != "cli-scalar" {
		t.Errorf("CLI flag did not override TOML for plugin scalar flag: %q", pf.Scalar)
	}
	if pf.Pointer == nil || *pf.Pointer != "cli-pointer" {
		t.Errorf("CLI flag did not override TOML for plugin pointer flag: %v", pf.Pointer)
	}
}

func TestFeatureFlagsUnsetPointerStaysNil(t *testing.T) {
	t.Parallel()

	// The "unset flags must not clobber TOML/--set values" contract relies on
	// pointer flags remaining nil when neither the CLI nor the config provides
	// a value.
	pf := parseProbeFeature(t, nil, probeTOMLBase)
	if pf.Pointer != nil {
		t.Errorf("unset pointer plugin flag = %v, want nil", pf.Pointer)
	}
}
