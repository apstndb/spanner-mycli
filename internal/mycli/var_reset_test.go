// Copyright 2026 apstndb
//
// Licensed under the MIT License.

package mycli

import (
	"errors"
	"strings"
	"testing"
)

func TestResettableDefsHavePrepareSupport(t *testing.T) {
	t.Parallel()
	sv := newSystemVariablesWithDefaultsForTest()
	sv.ensureRegistry()
	for i := range varDefs {
		def := &varDefs[i]
		if !def.resettable() {
			continue
		}
		if _, ok := sv.Registry.GetVariable(def.name).(resetPreparer); !ok {
			t.Errorf("%s is resettable but has no PrepareReset", def.name)
		}
	}
}

func TestCaptureStartupSnapshotsRequiresPrepareSupport(t *testing.T) {
	t.Parallel()
	sv := newSystemVariablesWithDefaultsForTest()
	if err := sv.CaptureStartupSnapshots(); err != nil {
		t.Fatalf("CaptureStartupSnapshots: %v", err)
	}
	if len(sv.startupSnapshots) == 0 {
		t.Fatal("expected supported startup snapshots")
	}
	if _, ok := sv.startupSnapshots["CLI_VERBOSE"]; !ok {
		t.Fatal("CLI_VERBOSE missing from startup snapshots")
	}
	if got := sv.startupSnapshots["KEEP_TRANSACTION_ALIVE"]; got != "TRUE" {
		t.Fatalf("captured KEEP_TRANSACTION_ALIVE = %q, want TRUE", got)
	}
	for _, excluded := range []string{
		"PROTO_DESCRIPTORS_FILE_PATH",
		protoDescriptorsVarName,
		"CLI_OUTPUT_TEMPLATE_FILE",
		"AUTOCOMMIT",
		"RETRY_ABORTS_INTERNALLY",
		"CLI_PROJECT",
		"CLI_ENABLE_ADC_PLUS",
		"CLI_CA_CERT_FILE",
		"CLI_CLIENT_CERT_FILE",
		"CLI_CLIENT_CERT_KEY",
		"CLI_WITHOUT_AUTHENTICATION",
	} {
		if _, ok := sv.startupSnapshots[excluded]; ok {
			t.Errorf("%s should not be captured", excluded)
		}
	}
}

func TestInitializeSystemVariablesCapturesAfterSet(t *testing.T) {
	t.Parallel()
	sv, err := initializeSystemVariables(&spannerOptions{
		ProjectId:  "p",
		InstanceId: "i",
		DatabaseId: "d",
		Set:        map[string]string{"CLI_VERBOSE": "TRUE", "CLI_PROMPT": "startup> "},
	})
	if err != nil {
		t.Fatalf("initializeSystemVariables: %v", err)
	}
	if got := sv.startupSnapshots["CLI_VERBOSE"]; got != "TRUE" {
		t.Fatalf("captured CLI_VERBOSE = %q, want TRUE", got)
	}
	if got := sv.startupSnapshots["CLI_PROMPT"]; got != "startup> " {
		t.Fatalf("captured CLI_PROMPT = %q, want startup> ", got)
	}
	if err := sv.SetFromSimple("CLI_VERBOSE", "FALSE"); err != nil {
		t.Fatal(err)
	}
	if err := sv.SetFromSimple("CLI_PROMPT", "init> "); err != nil {
		t.Fatal(err)
	}
	if err := sv.ResetAll(); err != nil {
		t.Fatal(err)
	}
	if got, _ := sv.Registry.Get("CLI_VERBOSE"); got != "TRUE" {
		t.Errorf("after RESET ALL CLI_VERBOSE = %q, want TRUE", got)
	}
	if got, _ := sv.Registry.Get("CLI_PROMPT"); got != "startup> " {
		t.Errorf("after RESET ALL CLI_PROMPT = %q, want startup> ", got)
	}
}

func TestResetNameAliasResolves(t *testing.T) {
	t.Parallel()
	val := "init"
	sv := newSystemVariablesWithDefaultsForTest()
	sv.featureVarDefs = []varDef{{
		name:    "CLI_TEST_RESET_VAR",
		aliases: []string{"CLI_TEST_RESET_ALIAS"},
		desc:    "test",
		scope:   scopeSession,
		bind:    func(*systemVariables) Variable { return StringVar(&val) },
	}}
	sv.Registry = NewVarRegistry(sv)
	if err := sv.CaptureStartupSnapshots(); err != nil {
		t.Fatal(err)
	}
	if err := sv.SetFromSimple("CLI_TEST_RESET_VAR", "changed"); err != nil {
		t.Fatal(err)
	}
	if err := sv.Reset("CLI_TEST_RESET_ALIAS"); err != nil {
		t.Fatalf("Reset(alias): %v", err)
	}
	if val != "init" {
		t.Errorf("alias reset = %q, want init", val)
	}
}

func TestResetNameCaseAndUnknown(t *testing.T) {
	t.Parallel()
	sv := newSystemVariablesWithDefaultsForTest()
	if err := sv.CaptureStartupSnapshots(); err != nil {
		t.Fatal(err)
	}
	if err := sv.SetFromSimple("CLI_VERBOSE", "TRUE"); err != nil {
		t.Fatal(err)
	}
	if err := sv.Reset("cli_verbose"); err != nil {
		t.Fatalf("Reset(cli_verbose): %v", err)
	}
	if got, _ := sv.Registry.Get("CLI_VERBOSE"); got != "FALSE" {
		t.Errorf("CLI_VERBOSE = %q, want FALSE", got)
	}

	err := sv.Reset("NO_SUCH_VARIABLE")
	var unknown *ErrUnknownVariable
	if !errors.As(err, &unknown) {
		t.Fatalf("Reset unknown: %v, want ErrUnknownVariable", err)
	}
	for _, name := range []string{"AUTOCOMMIT", "CLI_VERSION", "CLI_ENABLE_ADC_PLUS", "PROTO_DESCRIPTORS_FILE_PATH"} {
		err := sv.Reset(name)
		if err == nil || !strings.Contains(err.Error(), "does not support RESET") {
			t.Errorf("Reset(%s) = %v, want does not support RESET", name, err)
		}
	}
}

func TestResetAllWithoutCaptureFails(t *testing.T) {
	t.Parallel()
	sv := newSystemVariablesWithDefaultsForTest()
	sv.ensureRegistry()
	if err := sv.ResetAll(); !errors.Is(err, errResetSnapshotsMissing) {
		t.Fatalf("ResetAll without capture: %v, want errResetSnapshotsMissing", err)
	}
}

func TestResetAllLeavesExcludedState(t *testing.T) {
	t.Parallel()
	sv := newSystemVariablesWithDefaultsForTest()
	if err := sv.CaptureStartupSnapshots(); err != nil {
		t.Fatal(err)
	}
	if err := sv.SetFromSimple("PROTO_DESCRIPTORS_FILE_PATH", "testdata/protos/order_descriptors.pb"); err != nil {
		t.Fatal(err)
	}
	if err := sv.SetFromSimple("CLI_VERBOSE", "TRUE"); err != nil {
		t.Fatal(err)
	}
	if err := sv.ResetAll(); err != nil {
		t.Fatal(err)
	}
	if got, _ := sv.Registry.Get("CLI_VERBOSE"); got != "FALSE" {
		t.Errorf("CLI_VERBOSE = %q, want FALSE", got)
	}
	if got, _ := sv.Registry.Get("PROTO_DESCRIPTORS_FILE_PATH"); !strings.Contains(got, "order_descriptors.pb") {
		t.Errorf("excluded file path reset unexpectedly: %q", got)
	}
}

func TestResetPreparesWithoutMutatingOnRejection(t *testing.T) {
	t.Parallel()
	sv := newSystemVariablesWithDefaultsForTest()
	if err := sv.CaptureStartupSnapshots(); err != nil {
		t.Fatal(err)
	}
	if err := sv.SetFromSimple("CLI_VERBOSE", "TRUE"); err != nil {
		t.Fatal(err)
	}
	if err := sv.SetFromSimple("READONLY", "TRUE"); err != nil {
		t.Fatal(err)
	}
	sv.inTransaction = func() bool { return true }

	if err := sv.ResetAll(); err == nil || !strings.Contains(err.Error(), "READONLY") {
		t.Fatalf("ResetAll in transaction: %v, want READONLY guard", err)
	}
	if got, _ := sv.Registry.Get("CLI_VERBOSE"); got != "TRUE" {
		t.Errorf("rejected RESET ALL mutated CLI_VERBOSE: %q", got)
	}
	if got, _ := sv.Registry.Get("READONLY"); got != "TRUE" {
		t.Errorf("rejected RESET ALL mutated READONLY: %q", got)
	}
}

func TestCaptureIncludesFeatureVar(t *testing.T) {
	t.Parallel()
	val := "init"
	sv, err := initializeSystemVariables(&spannerOptions{
		ProjectId:  "p",
		InstanceId: "i",
		DatabaseId: "d",
	}, Feature{
		Name: "TESTFEAT",
		Vars: []FeatureVar{{
			Name: "CLI_TEST_RESET_VAR",
			Desc: "test",
			Var:  StringVar(&val),
		}},
	})
	if err != nil {
		t.Fatalf("initializeSystemVariables: %v", err)
	}
	if got := sv.startupSnapshots["CLI_TEST_RESET_VAR"]; got != "init" {
		t.Fatalf("captured feature var = %q, want init", got)
	}
	if err := sv.SetFromSimple("CLI_TEST_RESET_VAR", "changed"); err != nil {
		t.Fatal(err)
	}
	if err := sv.Reset("CLI_TEST_RESET_VAR"); err != nil {
		t.Fatal(err)
	}
	if val != "init" {
		t.Errorf("feature var after Reset = %q, want init", val)
	}
}

func TestResetCustomDerivedState(t *testing.T) {
	t.Parallel()
	sv := newSystemVariablesWithDefaultsForTest()
	if err := sv.CaptureStartupSnapshots(); err != nil {
		t.Fatal(err)
	}
	startupStyles := sv.Display.TypeStylesRaw
	if err := sv.SetFromSimple("CLI_TYPE_STYLES", "STRING=red"); err != nil {
		t.Fatal(err)
	}
	if len(sv.typeStyles) == 0 {
		t.Fatal("SET CLI_TYPE_STYLES did not update derived state")
	}
	if err := sv.Reset("CLI_TYPE_STYLES"); err != nil {
		t.Fatal(err)
	}
	if sv.Display.TypeStylesRaw != startupStyles {
		t.Errorf("TypeStylesRaw = %q, want %q", sv.Display.TypeStylesRaw, startupStyles)
	}
}

func TestResetDirectedReadWhenIdle(t *testing.T) {
	t.Parallel()
	sv := newSystemVariablesWithDefaultsForTest()
	if err := sv.CaptureStartupSnapshots(); err != nil {
		t.Fatal(err)
	}
	if err := sv.SetFromSimple("DIRECTED_READ", "us-east1"); err != nil {
		t.Fatal(err)
	}
	if err := sv.Reset("DIRECTED_READ"); err != nil {
		t.Fatal(err)
	}
	if got, _ := sv.Registry.Get("DIRECTED_READ"); got != "" {
		t.Errorf("DIRECTED_READ = %q, want empty", got)
	}
}

func TestResetKeepTransactionAlive(t *testing.T) {
	t.Parallel()
	sv := newSystemVariablesWithDefaultsForTest()
	if err := sv.CaptureStartupSnapshots(); err != nil {
		t.Fatal(err)
	}
	if err := sv.SetFromSimple("KEEP_TRANSACTION_ALIVE", "FALSE"); err != nil {
		t.Fatal(err)
	}
	if err := sv.Reset("KEEP_TRANSACTION_ALIVE"); err != nil {
		t.Fatal(err)
	}
	if got, _ := sv.Registry.Get("KEEP_TRANSACTION_ALIVE"); got != "TRUE" {
		t.Errorf("KEEP_TRANSACTION_ALIVE = %q, want TRUE", got)
	}
}

func TestResetCommitPriority(t *testing.T) {
	t.Parallel()
	sv := newSystemVariablesWithDefaultsForTest()
	if err := sv.CaptureStartupSnapshots(); err != nil {
		t.Fatal(err)
	}
	if err := sv.SetFromSimple("COMMIT_PRIORITY", "HIGH"); err != nil {
		t.Fatal(err)
	}
	if err := sv.Reset("COMMIT_PRIORITY"); err != nil {
		t.Fatal(err)
	}
	if got, _ := sv.Registry.Get("COMMIT_PRIORITY"); got != "UNSPECIFIED" {
		t.Errorf("COMMIT_PRIORITY = %q, want UNSPECIFIED", got)
	}
}
