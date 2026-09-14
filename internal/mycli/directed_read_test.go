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
	"io"
	"log/slog"
	"strings"
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestFormatDirectedReadOptionRoundTrip(t *testing.T) {
	t.Parallel()
	for _, input := range []string{"us-east1", "us-east1:READ_ONLY", "us-west1:READ_WRITE", "asia-northeast2:read_only"} {
		parsed, err := parseDirectedReadOption(input)
		if err != nil {
			t.Fatalf("parseDirectedReadOption(%q): %v", input, err)
		}
		got := formatDirectedReadOption(parsed)
		again, err := parseDirectedReadOption(got)
		if err != nil {
			t.Fatalf("reparse %q from %q: %v", got, input, err)
		}
		if diff := cmp.Diff(parsed, again, protocmp.Transform()); diff != "" {
			t.Errorf("round-trip %q mismatch (-want +got):\n%s", input, diff)
		}
	}
	if got := formatDirectedReadOption(nil); got != "" {
		t.Errorf("nil format = %q, want empty", got)
	}
}

func TestDirectedReadSetShowClearAndUnknownNames(t *testing.T) {
	t.Parallel()
	parsed, err := parseDirectedReadOption("asia-northeast2:READ_WRITE")
	if err != nil {
		t.Fatal(err)
	}
	preloaded := newTestSysVars().withDirectedRead(parsed).build()
	preloaded.ensureRegistry()
	gotInit, err := preloaded.Get("DIRECTED_READ")
	if err != nil {
		t.Fatal(err)
	}
	if gotInit["DIRECTED_READ"] != "asia-northeast2:READ_WRITE" {
		t.Errorf("SHOW of flag-initialized value = %q", gotInit["DIRECTED_READ"])
	}

	sysVars := newSystemVariablesWithDefaultsForTest()
	sysVars.ensureRegistry()

	for _, name := range []string{"CLI_DIRECT_READ", "CLI_DIRECTED_READ"} {
		if err := sysVars.SetFromGoogleSQL(name, "'us-east1'"); err == nil {
			t.Errorf("SET %s succeeded, want unknown", name)
		}
	}

	if err := sysVars.SetFromGoogleSQL("DIRECTED_READ", "'us-east1'"); err != nil {
		t.Fatalf("SET DIRECTED_READ us-east1: %v", err)
	}
	got, err := sysVars.Get("DIRECTED_READ")
	if err != nil {
		t.Fatal(err)
	}
	if got["DIRECTED_READ"] != "us-east1" {
		t.Errorf("SHOW after region-only SET = %q, want us-east1", got["DIRECTED_READ"])
	}

	original := sysVars.Query.DirectedRead
	originalValue := proto.CloneOf(original)
	if err := sysVars.SetFromSimple("DIRECTED_READ", "us-east1:NOT_A_TYPE"); err == nil {
		t.Fatal("invalid SET succeeded")
	}
	if sysVars.Query.DirectedRead != original {
		t.Fatal("invalid SET replaced the DirectedRead pointer")
	}
	if !proto.Equal(originalValue, original) {
		t.Fatal("invalid SET mutated the existing DirectedRead value")
	}
	if err := sysVars.SetFromSimple("DIRECTED_READ", "us-east1:READ_ONLY:extra"); err == nil {
		t.Fatal("SET with extra separators succeeded")
	}
	if sysVars.Query.DirectedRead != original || !proto.Equal(originalValue, original) {
		t.Fatal("extra-separator SET changed the current selection")
	}
	if err := sysVars.SetFromSimple("DIRECTED_READ", "us-east1:"); err == nil {
		t.Fatal("SET with empty replica type succeeded")
	}
	if sysVars.Query.DirectedRead != original || !proto.Equal(originalValue, original) {
		t.Fatal("empty replica-type SET changed the current selection")
	}

	if err := sysVars.SetFromGoogleSQL("DIRECTED_READ", "''"); err != nil {
		t.Fatalf("empty SET: %v", err)
	}
	got, err = sysVars.Get("DIRECTED_READ")
	if err != nil {
		t.Fatal(err)
	}
	if got["DIRECTED_READ"] != "" {
		t.Errorf("SHOW after clear = %q, want empty", got["DIRECTED_READ"])
	}
	if sysVars.Query.DirectedRead != nil {
		t.Errorf("cleared pointer = %v, want nil", sysVars.Query.DirectedRead)
	}
}

func TestDirectedReadTxnGuardAndNoLocal(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	ctx := t.Context()

	if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
		t.Fatalf("BEGIN: %v", err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "DIRECTED_READ", Value: "'us-east1'"}); err == nil || !strings.Contains(err.Error(), "active transaction") {
		t.Fatalf("SET during pending: err=%v, want active transaction", err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "DIRECTED_READ", Value: "'us-east1'"}); err == nil || !strings.Contains(err.Error(), "cannot be changed within a transaction") {
		t.Fatalf("SET LOCAL during pending: err=%v", err)
	}

	if _, err := session.ExecuteStatement(ctx, &RollbackStatement{}); err != nil {
		t.Fatalf("ROLLBACK: %v", err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "DIRECTED_READ", Value: "'us-east1:READ_ONLY'"}); err != nil {
		t.Fatalf("SET outside txn: %v", err)
	}
	got, err := session.systemVariables.Get("DIRECTED_READ")
	if err != nil {
		t.Fatal(err)
	}
	if got["DIRECTED_READ"] != "us-east1:READ_ONLY" {
		t.Errorf("SHOW = %q", got["DIRECTED_READ"])
	}
}

func TestCloneDirectedReadIsReplacement(t *testing.T) {
	t.Parallel()
	src, err := parseDirectedReadOption("us-east1:READ_ONLY")
	if err != nil {
		t.Fatal(err)
	}
	cloned := cloneDirectedRead(src)
	src.GetIncludeReplicas().ReplicaSelections[0].Location = "mutated"
	if cloned.GetIncludeReplicas().GetReplicaSelections()[0].GetLocation() == "mutated" {
		t.Fatal("clone shared the replica selection with the source")
	}
	if cloneDirectedRead(nil) != nil {
		t.Fatal("clone nil")
	}
}

func TestDirectedReadStartupFlagThenSet(t *testing.T) {
	oldLogger, oldLevel := slog.Default(), cliLogLevel.Level()
	t.Cleanup(func() { slog.SetDefault(oldLogger); cliLogLevel.Set(oldLevel) })
	for _, tt := range []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{name: "location", args: []string{"--directed-read=us-east1"}, want: "us-east1"},
		{name: "mixed case", args: []string{"--directed-read=us-east1:read_only"}, want: "us-east1:READ_ONLY"},
		{name: "set overrides", args: []string{"--directed-read=us-east1:READ_ONLY", "--set=DIRECTED_READ=us-west1:READ_WRITE"}, want: "us-west1:READ_WRITE"},
		{name: "set clears", args: []string{"--directed-read=us-east1", "--set=DIRECTED_READ="}},
		{name: "json exclude", args: []string{`--directed-read={"excludeReplicas":{"replicaSelections":[{"location":"us-east1","type":"READ_WRITE"}]}}`}, want: `{"excludeReplicas":{"replicaSelections":[{"location":"us-east1","type":"READ_WRITE"}]}}`},
		{name: "json set overrides shorthand", args: []string{"--directed-read=us-east1", `--set=DIRECTED_READ={"includeReplicas":{"replicaSelections":[{"location":"us-west1","type":"READ_ONLY"}],"autoFailoverDisabled":false}}`}, want: `{"includeReplicas":{"replicaSelections":[{"location":"us-west1","type":"READ_ONLY"}]}}`},
		{name: "invalid flag", args: []string{"--directed-read=us-east1:NOPE"}, wantErr: true},
		{name: "invalid set", args: []string{"--set=DIRECTED_READ=us-east1:READ_ONLY:extra"}, wantErr: true},
		{name: "invalid json", args: []string{`--set=DIRECTED_READ={"notAField":true}`}, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, err := parseFlagsArgs(tt.args, "test", nil, io.Discard, io.Discard)
			if err != nil {
				t.Fatal(err)
			}
			vars, err := initializeSystemVariables(&opts.Spanner)
			if tt.wantErr {
				if err == nil || vars != nil {
					t.Fatalf("startup state=%v error=%v", vars, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := vars.Get("DIRECTED_READ")
			if err != nil {
				t.Fatal(err)
			}
			if tt.want == "" {
				if got["DIRECTED_READ"] != "" || vars.Query.DirectedRead != nil {
					t.Fatalf("SHOW=%v directed=%v, want clear", got, vars.Query.DirectedRead)
				}
				return
			}
			wantDRO, err := parseDirectedReadOption(tt.want)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(wantDRO, vars.Query.DirectedRead, protocmp.Transform()); diff != "" {
				t.Fatalf("startup DirectedRead mismatch (-want +got):\n%s", diff)
			}
			showDRO, err := parseDirectedReadOption(got["DIRECTED_READ"])
			if err != nil {
				t.Fatalf("SHOW %q: %v", got["DIRECTED_READ"], err)
			}
			if diff := cmp.Diff(wantDRO, showDRO, protocmp.Transform()); diff != "" {
				t.Fatalf("SHOW round-trip mismatch (-want +got):\n%s", diff)
			}
			if !strings.HasPrefix(strings.TrimSpace(tt.want), "{") && got["DIRECTED_READ"] != tt.want {
				t.Fatalf("SHOW=%q want shorthand %q", got["DIRECTED_READ"], tt.want)
			}
		})
	}
}

func TestDirectedReadSetLocalOutsideTransaction(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	_, err := session.ExecuteStatement(t.Context(), &SetLocalStatement{VarName: "DIRECTED_READ", Value: "'us-east1'"})
	if err == nil || !strings.Contains(err.Error(), "requires an active transaction") {
		t.Fatalf("SET LOCAL outside txn: %v", err)
	}
}

func TestDirectedReadHelpAndUnknownNames(t *testing.T) {
	t.Parallel()
	rows := helpVariableRows(newSystemVariablesWithDefaultsForTest())
	var saw bool
	for _, row := range rows {
		if row.Name == "CLI_DIRECT_READ" || row.Name == "CLI_DIRECTED_READ" {
			t.Errorf("obsolete name listed: %s", row.Name)
		}
		if row.Name == "DIRECTED_READ" {
			saw = true
			if !strings.Contains(row.Operations, "write") {
				t.Errorf("operations=%q", row.Operations)
			}
		}
	}
	if !saw {
		t.Fatal("DIRECTED_READ missing from HELP VARIABLES")
	}
}

func replicaSel(location string, typ sppb.DirectedReadOptions_ReplicaSelection_Type) *sppb.DirectedReadOptions_ReplicaSelection {
	return &sppb.DirectedReadOptions_ReplicaSelection{Location: location, Type: typ}
}

func includeDirectedRead(autoFailover bool, sels ...*sppb.DirectedReadOptions_ReplicaSelection) *sppb.DirectedReadOptions {
	return &sppb.DirectedReadOptions{
		Replicas: &sppb.DirectedReadOptions_IncludeReplicas_{
			IncludeReplicas: &sppb.DirectedReadOptions_IncludeReplicas{
				ReplicaSelections:    sels,
				AutoFailoverDisabled: autoFailover,
			},
		},
	}
}

func excludeDirectedRead(sels ...*sppb.DirectedReadOptions_ReplicaSelection) *sppb.DirectedReadOptions {
	return &sppb.DirectedReadOptions{
		Replicas: &sppb.DirectedReadOptions_ExcludeReplicas_{
			ExcludeReplicas: &sppb.DirectedReadOptions_ExcludeReplicas{
				ReplicaSelections: sels,
			},
		},
	}
}

func TestParseFormatDirectedReadCompleteJSON(t *testing.T) {
	t.Parallel()
	type tc struct {
		name      string
		input     string
		want      *sppb.DirectedReadOptions
		wantShow  string
		errSubstr string
	}
	cases := []tc{
		{
			name:     "include json lossless shorthand",
			input:    `{"includeReplicas":{"replicaSelections":[{"location":"us-east1","type":"READ_ONLY"}],"autoFailoverDisabled":true}}`,
			want:     includeDirectedRead(true, replicaSel("us-east1", sppb.DirectedReadOptions_ReplicaSelection_READ_ONLY)),
			wantShow: "us-east1:READ_ONLY",
		},
		{
			name:     "include json location only lossless shorthand",
			input:    `{"includeReplicas":{"replicaSelections":[{"location":"us-east1"}],"autoFailoverDisabled":true}}`,
			want:     includeDirectedRead(true, replicaSel("us-east1", sppb.DirectedReadOptions_ReplicaSelection_TYPE_UNSPECIFIED)),
			wantShow: "us-east1",
		},
		{
			name:  "include autoFailoverDisabled false",
			input: `{"includeReplicas":{"replicaSelections":[{"location":"us-east1","type":"READ_ONLY"}],"autoFailoverDisabled":false}}`,
			want:  includeDirectedRead(false, replicaSel("us-east1", sppb.DirectedReadOptions_ReplicaSelection_READ_ONLY)),
		},
		{
			name:  "include autoFailoverDisabled omitted is false",
			input: `{"includeReplicas":{"replicaSelections":[{"location":"us-east1","type":"READ_ONLY"}]}}`,
			want:  includeDirectedRead(false, replicaSel("us-east1", sppb.DirectedReadOptions_ReplicaSelection_READ_ONLY)),
		},
		{
			name:  "exclude single replica",
			input: `{"excludeReplicas":{"replicaSelections":[{"location":"us-east1","type":"READ_WRITE"}]}}`,
			want:  excludeDirectedRead(replicaSel("us-east1", sppb.DirectedReadOptions_ReplicaSelection_READ_WRITE)),
		},
		{
			name: "multiple include selections",
			input: `{"includeReplicas":{"replicaSelections":[` +
				`{"location":"us-east1","type":"READ_ONLY"},` +
				`{"location":"us-west1","type":"READ_WRITE"}` +
				`],"autoFailoverDisabled":true}}`,
			want: includeDirectedRead(true,
				replicaSel("us-east1", sppb.DirectedReadOptions_ReplicaSelection_READ_ONLY),
				replicaSel("us-west1", sppb.DirectedReadOptions_ReplicaSelection_READ_WRITE)),
		},
		{
			name: "multiple exclude selections",
			input: `{"excludeReplicas":{"replicaSelections":[` +
				`{"location":"europe-west1","type":"READ_ONLY"},` +
				`{"location":"asia-northeast1"}]}}`,
			want: excludeDirectedRead(
				replicaSel("europe-west1", sppb.DirectedReadOptions_ReplicaSelection_READ_ONLY),
				replicaSel("asia-northeast1", sppb.DirectedReadOptions_ReplicaSelection_TYPE_UNSPECIFIED)),
		},
		{
			name:  "empty object",
			input: `{}`,
			want:  &sppb.DirectedReadOptions{},
		},
		{
			name:  "empty include replica stays json",
			input: `{"includeReplicas":{"replicaSelections":[{}],"autoFailoverDisabled":true}}`,
			want:  includeDirectedRead(true, replicaSel("", sppb.DirectedReadOptions_ReplicaSelection_TYPE_UNSPECIFIED)),
		},
		{
			name:  "unknown replica type number stays json",
			input: `{"includeReplicas":{"replicaSelections":[{"location":"us-east1","type":99}],"autoFailoverDisabled":true}}`,
			want:  includeDirectedRead(true, replicaSel("us-east1", 99)),
		},
		{
			name:      "unknown field",
			input:     `{"includeReplicas":{"replicaSelections":[{"location":"us-east1"}]},"notAField":true}`,
			errSubstr: "invalid directed read protobuf JSON",
		},
		{
			name:      "malformed json",
			input:     `{"includeReplicas":`,
			errSubstr: "invalid directed read protobuf JSON",
		},
		{
			name:      "include and exclude together",
			input:     `{"includeReplicas":{"replicaSelections":[{"location":"us-east1"}]},"excludeReplicas":{"replicaSelections":[{"location":"us-west1"}]}}`,
			errSubstr: "invalid directed read protobuf JSON",
		},
		{
			name:      "unknown enum",
			input:     `{"includeReplicas":{"replicaSelections":[{"location":"us-east1","type":"NOT_A_TYPE"}]}}`,
			errSubstr: "invalid directed read protobuf JSON",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseDirectedReadOption(tt.input)
			if tt.errSubstr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errSubstr) {
					t.Fatalf("error=%v, want substring %q", err, tt.errSubstr)
				}
				if got != nil {
					t.Fatal("parse returned a value on error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tt.want, got, protocmp.Transform()); diff != "" {
				t.Fatalf("parse mismatch (-want +got):\n%s", diff)
			}
			show := formatDirectedReadOption(got)
			if tt.wantShow != "" && show != tt.wantShow {
				t.Fatalf("SHOW=%q want shorthand %q", show, tt.wantShow)
			}
			if tt.wantShow == "" {
				if show == "" {
					t.Fatal("SHOW returned empty; SET would clear")
				}
				if !strings.HasPrefix(strings.TrimSpace(show), "{") {
					t.Fatalf("SHOW=%q, want protobuf JSON", show)
				}
			}
			again, err := parseDirectedReadOption(show)
			if err != nil {
				t.Fatalf("reparse SHOW %q: %v", show, err)
			}
			if diff := cmp.Diff(got, again, protocmp.Transform()); diff != "" {
				t.Fatalf("SHOW round-trip mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDirectedReadCompleteJSONSetShowClearAndInvalidAtomic(t *testing.T) {
	t.Parallel()
	sysVars := newSystemVariablesWithDefaultsForTest()
	sysVars.ensureRegistry()

	excludeJSON := `{"excludeReplicas":{"replicaSelections":[{"location":"us-east1","type":"READ_WRITE"}]}}`
	if err := sysVars.SetFromSimple("DIRECTED_READ", excludeJSON); err != nil {
		t.Fatal(err)
	}
	wantExclude := excludeDirectedRead(replicaSel("us-east1", sppb.DirectedReadOptions_ReplicaSelection_READ_WRITE))
	if diff := cmp.Diff(wantExclude, sysVars.Query.DirectedRead, protocmp.Transform()); diff != "" {
		t.Fatalf("SET exclude mismatch (-want +got):\n%s", diff)
	}
	got, err := sysVars.Get("DIRECTED_READ")
	if err != nil {
		t.Fatal(err)
	}
	show := got["DIRECTED_READ"]
	if show == "" || !strings.Contains(show, "excludeReplicas") {
		t.Fatalf("SHOW exclude = %q, want protobuf JSON", show)
	}
	again, err := parseDirectedReadOption(show)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(wantExclude, again, protocmp.Transform()); diff != "" {
		t.Fatalf("SHOW exclude round-trip mismatch (-want +got):\n%s", diff)
	}

	failoverFalse := `{"includeReplicas":{"replicaSelections":[{"location":"us-west1","type":"READ_ONLY"}],"autoFailoverDisabled":false}}`
	if err := sysVars.SetFromGoogleSQL("DIRECTED_READ", "'"+failoverFalse+"'"); err != nil {
		t.Fatalf("GoogleSQL SET JSON: %v", err)
	}
	wantFalse := includeDirectedRead(false, replicaSel("us-west1", sppb.DirectedReadOptions_ReplicaSelection_READ_ONLY))
	if diff := cmp.Diff(wantFalse, sysVars.Query.DirectedRead, protocmp.Transform()); diff != "" {
		t.Fatalf("SET failover false mismatch (-want +got):\n%s", diff)
	}
	got, err = sysVars.Get("DIRECTED_READ")
	if err != nil {
		t.Fatal(err)
	}
	if got["DIRECTED_READ"] == "us-west1:READ_ONLY" {
		t.Fatal("SHOW used shorthand for autoFailoverDisabled false")
	}

	original := sysVars.Query.DirectedRead
	originalValue := proto.CloneOf(original)
	for _, invalid := range []string{
		`{"notAField":1}`,
		`{"includeReplicas":`,
		`{"includeReplicas":{"replicaSelections":[{"location":"us-east1"}],"extra":true}}`,
		`{"includeReplicas":{"replicaSelections":[{"location":"us-east1"}]},"excludeReplicas":{"replicaSelections":[{"location":"us-west1"}]}}`,
	} {
		if err := sysVars.SetFromSimple("DIRECTED_READ", invalid); err == nil {
			t.Fatalf("invalid SET %q succeeded", invalid)
		}
		if sysVars.Query.DirectedRead != original {
			t.Fatalf("invalid SET %q replaced the DirectedRead pointer", invalid)
		}
		if !proto.Equal(originalValue, original) {
			t.Fatalf("invalid SET %q mutated the existing DirectedRead value", invalid)
		}
	}

	if err := sysVars.SetFromSimple("DIRECTED_READ", ""); err != nil {
		t.Fatal(err)
	}
	if sysVars.Query.DirectedRead != nil {
		t.Fatal("empty SET left a non-nil selection")
	}
	got, err = sysVars.Get("DIRECTED_READ")
	if err != nil {
		t.Fatal(err)
	}
	if got["DIRECTED_READ"] != "" {
		t.Fatalf("SHOW after clear = %q", got["DIRECTED_READ"])
	}
}

func TestDirectedReadSetShowSetShorthandBoundaries(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name      string
		set       string
		wantJSON  bool
		wantEqual *sppb.DirectedReadOptions
	}{
		{
			name:      "lossless include still shorthand",
			set:       `{"includeReplicas":{"replicaSelections":[{"location":"us-east1","type":"READ_ONLY"}],"autoFailoverDisabled":true}}`,
			wantEqual: includeDirectedRead(true, replicaSel("us-east1", sppb.DirectedReadOptions_ReplicaSelection_READ_ONLY)),
		},
		{
			name:      "empty include replica stays json",
			set:       `{"includeReplicas":{"replicaSelections":[{}],"autoFailoverDisabled":true}}`,
			wantJSON:  true,
			wantEqual: includeDirectedRead(true, replicaSel("", sppb.DirectedReadOptions_ReplicaSelection_TYPE_UNSPECIFIED)),
		},
		{
			name:      "unknown replica type number stays json",
			set:       `{"includeReplicas":{"replicaSelections":[{"location":"us-east1","type":99}],"autoFailoverDisabled":true}}`,
			wantJSON:  true,
			wantEqual: includeDirectedRead(true, replicaSel("us-east1", 99)),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sysVars := newSystemVariablesWithDefaultsForTest()
			sysVars.ensureRegistry()
			if err := sysVars.SetFromSimple("DIRECTED_READ", tt.set); err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tt.wantEqual, sysVars.Query.DirectedRead, protocmp.Transform()); diff != "" {
				t.Fatalf("SET mismatch (-want +got):\n%s", diff)
			}
			original := proto.CloneOf(sysVars.Query.DirectedRead)
			got, err := sysVars.Get("DIRECTED_READ")
			if err != nil {
				t.Fatal(err)
			}
			show := got["DIRECTED_READ"]
			if show == "" {
				t.Fatal("SHOW returned empty; SET would clear")
			}
			if tt.wantJSON {
				if !strings.HasPrefix(strings.TrimSpace(show), "{") {
					t.Fatalf("SHOW=%q, want protobuf JSON", show)
				}
				if show == "us-east1:99" {
					t.Fatal("SHOW used invalid numeric-type shorthand")
				}
			} else if show != "us-east1:READ_ONLY" {
				t.Fatalf("SHOW=%q, want lossless shorthand", show)
			}
			if err := sysVars.SetFromSimple("DIRECTED_READ", show); err != nil {
				t.Fatalf("SET of SHOW %q: %v", show, err)
			}
			if !proto.Equal(original, sysVars.Query.DirectedRead) {
				t.Fatalf("SET/SHOW/SET lost the message\nbefore=%v\nafter=%v\nSHOW=%q", original, sysVars.Query.DirectedRead, show)
			}
		})
	}
}

func TestDirectedReadJSONShorthandIndependence(t *testing.T) {
	t.Parallel()
	a := newSystemVariablesWithDefaultsForTest()
	a.ensureRegistry()
	b := newSystemVariablesWithDefaultsForTest()
	b.ensureRegistry()

	excludeJSON := `{"excludeReplicas":{"replicaSelections":[{"location":"us-east1","type":"READ_WRITE"}]}}`
	multiJSON := `{"includeReplicas":{"replicaSelections":[{"location":"us-east1","type":"READ_ONLY"},{"location":"us-west1","type":"READ_WRITE"}],"autoFailoverDisabled":false}}`
	if err := a.SetFromSimple("DIRECTED_READ", excludeJSON); err != nil {
		t.Fatal(err)
	}
	if err := b.SetFromSimple("DIRECTED_READ", multiJSON); err != nil {
		t.Fatal(err)
	}
	wantA := excludeDirectedRead(replicaSel("us-east1", sppb.DirectedReadOptions_ReplicaSelection_READ_WRITE))
	wantB := includeDirectedRead(false,
		replicaSel("us-east1", sppb.DirectedReadOptions_ReplicaSelection_READ_ONLY),
		replicaSel("us-west1", sppb.DirectedReadOptions_ReplicaSelection_READ_WRITE))
	if diff := cmp.Diff(wantA, a.Query.DirectedRead, protocmp.Transform()); diff != "" {
		t.Fatalf("session A mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(wantB, b.Query.DirectedRead, protocmp.Transform()); diff != "" {
		t.Fatalf("session B mismatch (-want +got):\n%s", diff)
	}
	a.Query.DirectedRead.GetExcludeReplicas().ReplicaSelections[0].Location = "mutated"
	if b.Query.DirectedRead.GetIncludeReplicas().GetReplicaSelections()[0].GetLocation() == "mutated" {
		t.Fatal("sessions shared DirectedRead state")
	}
}
