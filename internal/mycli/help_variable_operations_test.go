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
	"slices"
	"testing"
)

// helpOperations looks up one public HELP VARIABLES / generated-docs row.
func helpOperations(t *testing.T, sv *systemVariables, name string) string {
	t.Helper()
	for _, row := range helpVariableRows(sv) {
		if row.Name == name {
			return row.Operations
		}
	}
	t.Fatalf("%s missing from HELP VARIABLES", name)
	return ""
}

// TestHelpVariableOperationsPublicRows pins representative registered
// capability labels. It does not re-derive localAllowed/resettable for every
// variable.
func TestHelpVariableOperationsPublicRows(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		want string
	}{
		{name: "CLI_FORMAT", want: "read,write,local,reset"},
		{name: "COMMIT_RESPONSE", want: "read"},
		{name: "CLI_HOST", want: "read"},
		{name: "CLI_ENABLE_ADC_PLUS", want: "read,write"},
		{name: "READONLY", want: "read,write,reset"},
		{name: "AUTOCOMMIT", want: "read,write,reset"},
		{name: "PROTO_DESCRIPTORS_FILE_PATH", want: "read,write,add"},
		{name: "CLI_OUTPUT_TEMPLATE_FILE", want: "read,write"},
		{name: "DIRECTED_READ", want: "read,write,reset"},
		{name: "TRANSACTION_TIMEOUT", want: "read,write,local,reset"},
		{name: "RETRY_ABORTS_INTERNALLY", want: "read,write,local,reset"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sv := newSystemVariablesWithDefaultsForTest()
			if got := helpOperations(t, sv, tt.name); got != tt.want {
				t.Errorf("operations = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestHelpVariableOperationsIndependentOfSessionState asserts static support
// labels do not follow the current value or startup snapshot.
func TestHelpVariableOperationsIndependentOfSessionState(t *testing.T) {
	t.Parallel()

	sv := newSystemVariablesWithDefaultsForTest()
	if err := sv.CaptureStartupSnapshots(); err != nil {
		t.Fatalf("CaptureStartupSnapshots: %v", err)
	}
	before := helpOperations(t, sv, "CLI_FORMAT")
	if before != "read,write,local,reset" {
		t.Fatalf("startup operations = %q", before)
	}
	if err := sv.SetFromSimple("CLI_FORMAT", "VERTICAL"); err != nil {
		t.Fatalf("SET CLI_FORMAT: %v", err)
	}
	after := helpOperations(t, sv, "CLI_FORMAT")
	if after != before {
		t.Errorf("operations changed after SET: before %q after %q", before, after)
	}
}

type opsMetaSpyVar struct {
	gets, sets int
}

func (v *opsMetaSpyVar) Get() (string, error) {
	v.gets++
	return "x", nil
}

func (v *opsMetaSpyVar) Set(string) error {
	v.sets++
	return nil
}

// TestHelpVariableOperationsDoesNotReadLiveValues guards the metadata path:
// ListVariableInfo / HELP rows must not Get or Set merely to render labels.
func TestHelpVariableOperationsDoesNotReadLiveValues(t *testing.T) {
	t.Parallel()

	spy := &opsMetaSpyVar{}
	sv := newSystemVariablesWithDefaults()
	sv.featureVarDefs = featureVarDefs([]Feature{{
		Name: "TESTOPS",
		Vars: []FeatureVar{{Name: "CLI_TEST_OPS_SPY", Desc: "spy", Var: spy}},
	}})
	sv.ensureRegistry()

	if _, ok := sv.ListVariableInfo()["CLI_TEST_OPS_SPY"]; !ok {
		t.Fatal("CLI_TEST_OPS_SPY missing from ListVariableInfo")
	}
	if got := helpOperations(t, &sv, "CLI_TEST_OPS_SPY"); got != "read,write,local,reset" {
		t.Errorf("operations = %q", got)
	}
	if spy.gets != 0 || spy.sets != 0 {
		t.Fatalf("metadata path called Get/Set: gets=%d sets=%d", spy.gets, spy.sets)
	}
}

// TestHelpVariableOperationsFeatureParity confirms feature-contributed vars
// use the same metadata generator, including distinct noLocal/noReset and a
// single unimplemented marker.
func TestHelpVariableOperationsFeatureParity(t *testing.T) {
	t.Parallel()

	sv := newSystemVariablesWithDefaults()
	sv.featureVarDefs = featureVarDefs([]Feature{{
		Name: "TESTOPS",
		Vars: []FeatureVar{
			{Name: "CLI_TEST_OPS_DEFAULT", Desc: "default", Var: &stubVar{val: "x"}},
			{Name: "CLI_TEST_OPS_RO", Desc: "ro", Var: &stubVar{val: "x"}, ReadOnly: true},
			{Name: "CLI_TEST_OPS_INIT", Desc: "init", Var: &stubVar{val: "x"}, InitOnly: true},
			{Name: "CLI_TEST_OPS_TXN", Desc: "txn", Var: &stubVar{val: "x"}, TxnGuard: true},
			{Name: "CLI_TEST_OPS_NOLOCAL", Desc: "nolocal", Var: &stubVar{val: "x"}, NoLocal: true},
			{Name: "CLI_TEST_OPS_NORESET", Desc: "noreset", Var: &stubVar{val: "x"}, NoReset: true},
			{Name: "CLI_TEST_OPS_FILE", Desc: "file", Var: &stubVar{val: "x"}, NoLocal: true, NoReset: true},
			{Name: "CLI_TEST_OPS_UNIMPL", Desc: "placeholder", Var: &UnimplementedVar{name: "CLI_TEST_OPS_UNIMPL"}},
		},
	}})
	sv.ensureRegistry()

	for _, tt := range []struct {
		name string
		want string
	}{
		{name: "CLI_TEST_OPS_DEFAULT", want: "read,write,local,reset"},
		{name: "CLI_TEST_OPS_RO", want: "read"},
		{name: "CLI_TEST_OPS_INIT", want: "read,write"},
		{name: "CLI_TEST_OPS_TXN", want: "read,write,reset"},
		{name: "CLI_TEST_OPS_NOLOCAL", want: "read,write,reset"},
		{name: "CLI_TEST_OPS_NORESET", want: "read,write,local"},
		{name: "CLI_TEST_OPS_FILE", want: "read,write"},
		{name: "CLI_TEST_OPS_UNIMPL", want: "unimplemented"},
	} {
		if got := helpOperations(t, &sv, tt.name); got != tt.want {
			t.Errorf("%s operations = %q, want %q", tt.name, got, tt.want)
		}
	}

	unimpl := sv.ListVariableInfo()["CLI_TEST_OPS_UNIMPL"]
	if !unimpl.Unimplemented || unimpl.LocalAllowed || unimpl.Resettable || unimpl.CanAdd {
		t.Errorf("unimplemented info = %+v", unimpl)
	}
}

// TestHelpVariableOperationsExcludesAliases asserts listings stay on
// canonical names even when an alias is registered.
func TestHelpVariableOperationsExcludesAliases(t *testing.T) {
	t.Parallel()

	val := "init"
	sv := newSystemVariablesWithDefaults()
	sv.featureVarDefs = []varDef{{
		name:    "CLI_TEST_OPS_CANON",
		aliases: []string{"CLI_TEST_OPS_ALIAS"},
		desc:    "alias source",
		scope:   scopeSession,
		bind:    func(*systemVariables) Variable { return StringVar(&val) },
	}}
	sv.ensureRegistry()

	info := sv.ListVariableInfo()
	if _, ok := info["CLI_TEST_OPS_ALIAS"]; ok {
		t.Fatal("alias listed in ListVariableInfo")
	}
	if _, ok := info["CLI_TEST_OPS_CANON"]; !ok {
		t.Fatal("canonical name missing from ListVariableInfo")
	}

	rows := helpVariableRows(&sv)
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.Name)
	}
	if slices.Contains(names, "CLI_TEST_OPS_ALIAS") {
		t.Fatal("alias listed in HELP VARIABLES")
	}
	if !slices.Contains(names, "CLI_TEST_OPS_CANON") {
		t.Fatal("canonical name missing from HELP VARIABLES")
	}
	if !slices.IsSorted(names) {
		t.Fatal("HELP VARIABLES rows are not sorted")
	}
}
