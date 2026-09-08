// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mycli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestDumpProtoDescriptorReplay(t *testing.T) {
	skipIfShortIntegration(t)
	fds := a20DescriptorSet(t)
	_, source := initializeWithRandomDB(t, nil, nil)
	source.systemVariables.Internal.ProtoDescriptor = fds
	if _, err := executeDdlStatements(t.Context(), source, []string{
		"CREATE PROTO BUNDLE (a20.Root, a20.State)",
		"CREATE TABLE A20 (Id INT64 NOT NULL, P a20.Root, E a20.State) PRIMARY KEY(Id)",
	}); err != nil {
		t.Fatal(err)
	}
	cmds, err := buildCommands(`INSERT INTO A20 (Id,P,E) VALUES (1,CAST(b"\x0a\x01x" AS a20.Root),CAST(42 AS a20.State))`, source.systemVariables.Query.BuildStatementMode)
	if err != nil {
		t.Fatal(err)
	}
	for _, cmd := range cmds {
		if _, err := source.ExecuteStatement(t.Context(), cmd); err != nil {
			t.Fatal(err)
		}
	}

	for _, stale := range []bool{false, true} {
		for _, mode := range []dumpMode{dumpModeDatabase, dumpModeSchema} {
			for _, streamed := range []bool{false, true} {
				t.Run(fmt.Sprintf("stale=%v/mode=%d/stream=%v", stale, mode, streamed), func(t *testing.T) {
					source.systemVariables.Internal.ProtoDescriptor = nil
					if stale {
						source.systemVariables.Internal.ProtoDescriptor = &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{{
							Name: proto.String("broken.proto"), Dependency: []string{"missing.proto"},
						}}}
					}
					before := source.systemVariables.Internal.ProtoDescriptor
					var out bytes.Buffer
					var result *Result
					var err error
					if streamed {
						err = source.withOutput(outputContext{w: &out}, func() error {
							result, err = executeDump(t.Context(), source, mode, nil)
							return err
						})
					} else {
						result, err = executeDump(t.Context(), source, mode, nil)
						if err == nil {
							_, _ = out.Write(result.RenderedOutput)
						}
					}
					if err != nil {
						t.Fatalf("dump failed: %v", err)
					}
					if result.Streamed != streamed {
						t.Fatalf("wrong output path: %+v", result)
					}
					if source.systemVariables.Internal.ProtoDescriptor != before {
						t.Fatal("dump changed source graph")
					}
					if !strings.Contains(out.String(), "SET PROTO_DESCRIPTORS = '") {
						t.Fatalf("missing preamble: %s", out.String())
					}
					commands, err := buildCommands(out.String(), source.systemVariables.Query.BuildStatementMode)
					if err != nil {
						t.Fatal(err)
					}
					if len(commands) == 0 {
						t.Fatal("no replay commands")
					}
					if _, ok := commands[0].(*SetStatement); !ok {
						t.Fatalf("first command = %T, want SET", commands[0])
					}
					_, target := initializeWithRandomDB(t, nil, nil)
					for _, command := range commands {
						if _, err := target.ExecuteStatement(t.Context(), command); err != nil {
							t.Fatalf("unassisted replay: %v", err)
						}
					}
					if mode == dumpModeDatabase {
						it := target.client.Single().Query(t.Context(), spanner.Statement{SQL: "SELECT TO_BASE64(CAST(P AS BYTES)), CAST(E AS INT64) FROM A20"})
						defer it.Stop()
						row, err := it.Next()
						if err != nil {
							t.Fatal(err)
						}
						var p string
						var e int64
						if err := row.Columns(&p, &e); err != nil {
							t.Fatal(err)
						}
						if p != "CgF4" || e != 42 {
							t.Fatalf("wire fidelity got %q,%d", p, e)
						}
					}
				})
			}
		}
	}
}
