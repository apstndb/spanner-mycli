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
		{name: "invalid flag", args: []string{"--directed-read=us-east1:NOPE"}, wantErr: true},
		{name: "invalid set", args: []string{"--set=DIRECTED_READ=us-east1:READ_ONLY:extra"}, wantErr: true},
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
			if err != nil || got["DIRECTED_READ"] != tt.want {
				t.Fatalf("SHOW=%v error=%v want=%q", got, err, tt.want)
			}
			if tt.want == "" && vars.Query.DirectedRead != nil {
				t.Fatal("clear left nonnil selection")
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
