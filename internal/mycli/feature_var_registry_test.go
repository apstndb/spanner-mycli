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

// Internal tests for the var-registry uniqueness invariant and the FeatureVar
// conversion path (issue #778). Internal package to reach varDefs, the
// unexported registry construction, and FeatureVar.toVarDef.

import (
	"strings"
	"testing"
)

// stubVar is a minimal Variable for exercising the FeatureVar conversion path.
type stubVar struct{ val string }

func (v *stubVar) Get() (string, error) { return v.val, nil }
func (v *stubVar) Set(s string) error   { v.val = s; return nil }

// TestVarDefsNoCaseFoldedNameCollisions asserts the core varDefs table has no
// case-folded name or alias collisions. registerAll now panics on a collision,
// so this also guards against a silent handler overwrite (var_registry.go).
func TestVarDefsNoCaseFoldedNameCollisions(t *testing.T) {
	t.Parallel()

	seen := make(map[string]string) // upper name -> source canonical name
	check := func(key, source string) {
		upper := strings.ToUpper(key)
		if prev, dup := seen[upper]; dup {
			t.Errorf("case-folded name collision on %q: %q and %q", upper, prev, source)
			return
		}
		seen[upper] = source
	}
	for i := range varDefs {
		check(varDefs[i].name, varDefs[i].name)
		for _, alias := range varDefs[i].aliases {
			check(alias, varDefs[i].name)
		}
	}
}

// TestRegistryBuildsWithoutCollision confirms the real registry construction
// (core varDefs) does not panic; it is the runtime counterpart to the table
// scan above.
func TestRegistryBuildsWithoutCollision(t *testing.T) {
	t.Parallel()

	sv := newSystemVariablesWithDefaults()
	_ = NewVarRegistry(&sv) // must not panic
}

// TestFeatureVarConversionRegisters exercises the FeatureVar -> varDef path: a
// non-colliding feature var is registered and resolvable through the registry
// with its declared policy.
func TestFeatureVarConversionRegisters(t *testing.T) {
	t.Parallel()

	sv := newSystemVariablesWithDefaults()
	sv.featureVarDefs = featureVarDefs([]Feature{
		{
			Name: "TESTFEAT",
			Vars: []FeatureVar{
				{Name: "CLI_TEST_FEATURE_VAR", Desc: "test", Var: &stubVar{val: "init"}},
			},
		},
	})
	r := NewVarRegistry(&sv)

	if got, err := r.Get("CLI_TEST_FEATURE_VAR"); err != nil || got != "init" {
		t.Fatalf("Get(CLI_TEST_FEATURE_VAR) = %q, %v; want \"init\", nil", got, err)
	}
	if err := r.Set("cli_test_feature_var", "changed", false); err != nil {
		t.Fatalf("Set feature var (case-insensitive) error: %v", err)
	}
	if got, _ := r.Get("CLI_TEST_FEATURE_VAR"); got != "changed" {
		t.Fatalf("after Set, Get = %q, want \"changed\"", got)
	}
}

// TestFeatureVarReadOnlyPolicyApplies confirms the converted varDef carries the
// FeatureVar policy flags (ReadOnly here) through to registry enforcement.
func TestFeatureVarReadOnlyPolicyApplies(t *testing.T) {
	t.Parallel()

	sv := newSystemVariablesWithDefaults()
	sv.featureVarDefs = featureVarDefs([]Feature{
		{
			Name: "TESTFEAT",
			Vars: []FeatureVar{
				{Name: "CLI_TEST_RO_VAR", Desc: "ro", Var: &stubVar{val: "x"}, ReadOnly: true},
			},
		},
	})
	r := NewVarRegistry(&sv)

	if ro, err := r.IsReadOnly("CLI_TEST_RO_VAR"); err != nil || !ro {
		t.Fatalf("IsReadOnly = %v, %v; want true, nil", ro, err)
	}
	if err := r.Set("CLI_TEST_RO_VAR", "y", false); err == nil {
		t.Fatalf("Set on read-only feature var succeeded; want rejection")
	}
}

// TestFeatureVarCollisionPanics asserts a feature var whose name collides
// (case-folded) with a core variable makes registry construction fail loudly
// instead of silently overwriting the core handler.
func TestFeatureVarCollisionWithCorePanics(t *testing.T) {
	t.Parallel()

	sv := newSystemVariablesWithDefaults()
	sv.featureVarDefs = featureVarDefs([]Feature{
		{
			Name: "TESTFEAT",
			// READONLY is a core variable; lowercase to prove case-folding.
			Vars: []FeatureVar{
				{Name: "readonly", Desc: "collide", Var: &stubVar{}},
			},
		},
	})

	assertPanics(t, "core/feature name collision", func() {
		_ = NewVarRegistry(&sv)
	})
}

// TestFeatureVarCollisionBetweenFeaturesPanics asserts two feature vars with
// the same case-folded name also collide loudly.
func TestFeatureVarCollisionBetweenFeaturesPanics(t *testing.T) {
	t.Parallel()

	sv := newSystemVariablesWithDefaults()
	sv.featureVarDefs = featureVarDefs([]Feature{
		{Name: "F1", Vars: []FeatureVar{{Name: "CLI_DUP_VAR", Var: &stubVar{}}}},
		{Name: "F2", Vars: []FeatureVar{{Name: "cli_dup_var", Var: &stubVar{}}}},
	})

	assertPanics(t, "feature/feature name collision", func() {
		_ = NewVarRegistry(&sv)
	})
}

func assertPanics(t *testing.T, what string, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("%s: expected panic, got none", what)
		}
	}()
	fn()
}
