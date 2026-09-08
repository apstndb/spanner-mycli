// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mycli

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestProtoDescriptorsFilePathSQL(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	sv := session.systemVariables
	path := writeProtoSource(t, filepath.Join(t.TempDir(), "root.proto"), `syntax="proto3"; package renamed; message Root { string value=1; }`)
	execute := func(sql string) *Result {
		t.Helper()
		stmt, err := BuildStatement(sql)
		if err != nil {
			t.Fatal(err)
		}
		result, err := session.ExecuteStatement(t.Context(), stmt)
		if err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return result
	}
	execute(fmt.Sprintf("SET PROTO_DESCRIPTORS_FILE_PATH = %q", path))
	files := requireUsableDescriptor(t, sv.Internal.ProtoDescriptor)
	if _, err := files.FindDescriptorByName("renamed.Root"); err != nil {
		t.Fatal(err)
	}
	result := execute("SHOW VARIABLE PROTO_DESCRIPTORS_FILE_PATH")
	if got := result.TableHeader.Render(false); !slices.Equal(got, []string{"PROTO_DESCRIPTORS_FILE_PATH"}) {
		t.Fatalf("SHOW header = %v", got)
	}
	if len(result.Rows) != 1 || len(result.Rows[0]) != 1 || result.Rows[0][0].RawText() != path {
		t.Fatalf("SHOW rows = %v, want path %q", result.Rows, path)
	}
	added := writeDescriptorSet(t, filepath.Join(t.TempDir(), "added.pb"), descriptorFile("added.proto", "added"))
	execute(fmt.Sprintf("SET PROTO_DESCRIPTORS_FILE_PATH += %q", added))
	files = requireUsableDescriptor(t, sv.Internal.ProtoDescriptor)
	for _, name := range []protoreflect.FullName{"renamed.Root", "added.Root"} {
		if _, err := files.FindDescriptorByName(name); err != nil {
			t.Fatal(err)
		}
	}
	if got := mustGetVar(t, session, "PROTO_DESCRIPTORS_FILE_PATH"); got != path+","+added {
		t.Fatalf("ADD provenance = %q", got)
	}
	beforeGraph, beforePaths := cloneDescriptorState(sv)
	for _, sql := range []string{
		"SET CLI_PROTO_DESCRIPTOR_FILE = ''",
		"SET CLI_PROTO_DESCRIPTOR_FILE += ''",
		"SHOW VARIABLE CLI_PROTO_DESCRIPTOR_FILE",
	} {
		stmt, err := BuildStatement(sql)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := session.ExecuteStatement(t.Context(), stmt); err == nil || !strings.Contains(err.Error(), "unknown variable") {
			t.Fatalf("old-name execution %q: %v", sql, err)
		}
		assertDescriptorState(t, sv, beforeGraph, beforePaths)
	}
	execute("BEGIN")
	_, err := session.ExecuteStatement(t.Context(), &SetLocalStatement{VarName: "PROTO_DESCRIPTORS_FILE_PATH", Value: "''"})
	if err == nil || err.Error() != "PROTO_DESCRIPTORS_FILE_PATH does not support SET LOCAL" {
		t.Fatalf("SET LOCAL: %v", err)
	}
	assertDescriptorState(t, sv, beforeGraph, beforePaths)
	execute("ROLLBACK")
	execute("SET PROTO_DESCRIPTORS_FILE_PATH = ''")
	if sv.Internal.ProtoDescriptor != nil || len(sv.Internal.ProtoDescriptorFile) != 0 {
		t.Fatal("empty SET did not clear graph and paths")
	}
}

func TestProtoDescriptorsFilePathDiscovery(t *testing.T) {
	t.Parallel()
	sv := newSystemVariablesWithDefaultsForTest()
	finder := &fuzzyFinderCommand{cli: &Cli{SystemVariables: sv}}
	names := finder.fetchVariableCandidates()
	if !slices.Contains(names, "PROTO_DESCRIPTORS_FILE_PATH") || slices.Contains(names, "CLI_PROTO_DESCRIPTOR_FILE") {
		t.Fatalf("variable completion: %v", names)
	}
	found := false
	for _, row := range helpVariableRows(sv) {
		if row.Name == "CLI_PROTO_DESCRIPTOR_FILE" {
			t.Fatal("HELP still advertises removed name")
		}
		if row.Name == "PROTO_DESCRIPTORS_FILE_PATH" {
			found = true
			if row.Operations != "read,write,add" {
				t.Fatalf("HELP operations = %q", row.Operations)
			}
		}
	}
	if !found {
		t.Fatal("HELP omitted new name")
	}
}

func TestProtoDescriptorsFilePathStartup(t *testing.T) {
	oldLogger, oldLevel := slog.Default(), cliLogLevel.Level()
	t.Cleanup(func() { slog.SetDefault(oldLogger); cliLogLevel.Set(oldLevel) })
	first := writeDescriptorSet(t, filepath.Join(t.TempDir(), "first.pb"), descriptorFile("first.proto", "first"))
	second := writeDescriptorSet(t, filepath.Join(t.TempDir(), "second.pb"), descriptorFile("second.proto", "second"))
	for _, tt := range []struct {
		name, config, want string
		args               []string
		wantErr            bool
	}{
		{name: "flag unchanged", args: []string{"--proto-descriptor-file=" + first}, want: first},
		{name: "config unchanged", config: fmt.Sprintf("proto-descriptor-file = '%s'\n", first), want: first},
		{name: "new set", args: []string{"--set=PROTO_DESCRIPTORS_FILE_PATH=" + second}, want: second},
		{name: "set overrides flag", args: []string{"--proto-descriptor-file=" + first, "--set=PROTO_DESCRIPTORS_FILE_PATH=" + second}, want: second},
		{name: "set clears flag", args: []string{"--proto-descriptor-file=" + first, "--set=PROTO_DESCRIPTORS_FILE_PATH="}},
		{name: "old set rejected", args: []string{"--set=CLI_PROTO_DESCRIPTOR_FILE=" + first}, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var configFiles []string
			if tt.config != "" {
				path := filepath.Join(t.TempDir(), "config.toml")
				if err := os.WriteFile(path, []byte(tt.config), 0o600); err != nil {
					t.Fatal(err)
				}
				configFiles = []string{path}
			}
			opts, _, err := parseFlagsArgs(tt.args, "test", configFiles, io.Discard, io.Discard)
			if err != nil {
				t.Fatal(err)
			}
			sv, err := initializeSystemVariables(&opts.Spanner)
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "unknown variable") || sv != nil {
					t.Fatalf("startup state nil=%v, err=%v", sv == nil, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got, err := sv.Registry.Get("PROTO_DESCRIPTORS_FILE_PATH"); err != nil || got != tt.want {
				t.Fatalf("startup paths = %q, err=%v; want %q", got, err, tt.want)
			}
			if tt.want == "" {
				if sv.Internal.ProtoDescriptor != nil {
					t.Fatal("cleared startup graph remains")
				}
				return
			}
			requireUsableDescriptor(t, sv.Internal.ProtoDescriptor)
			wantPackage := strings.TrimSuffix(filepath.Base(tt.want), ".pb")
			if len(sv.Internal.ProtoDescriptor.File) != 1 || sv.Internal.ProtoDescriptor.File[0].GetPackage() != wantPackage {
				t.Fatalf("startup installed wrong graph: %v", sv.Internal.ProtoDescriptor)
			}
		})
	}
}
