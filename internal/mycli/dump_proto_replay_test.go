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
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"cloud.google.com/go/spanner"
	adminpb "cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
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
		if _, err := source.ExecuteStatement(t.Context(), cmd.stmt); err != nil {
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
							_, _ = out.Write(result.preparedOutput())
						}
					}
					if err != nil {
						t.Fatalf("dump failed: %v", err)
					}
					if result.alreadyDelivered() != streamed {
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
					if _, ok := commands[0].stmt.(*SetStatement); !ok {
						t.Fatalf("first command = %T, want SET", commands[0].stmt)
					}
					_, target := initializeWithRandomDB(t, nil, nil)
					for _, command := range commands {
						if _, err := target.ExecuteStatement(t.Context(), command.stmt); err != nil {
							t.Fatalf("unassisted replay: %v", err)
						}
					}
					if mode == dumpModeDatabase {
						assertA20Row(t, target)
					}
				})
			}
		}
	}
}

func TestDumpProtoDescriptorImportedNestedReplay(t *testing.T) {
	skipIfShortIntegration(t)
	fds := a20ImportedNestedDescriptorSet(t)
	_, source := initializeWithRandomDB(t, nil, nil)
	source.systemVariables.Internal.ProtoDescriptor = fds
	if _, err := executeDdlStatements(t.Context(), source, []string{
		"CREATE PROTO BUNDLE (`a20g.Root`, `a20g.Root.Payload`, `a20g.Child`, `google.protobuf.Timestamp`)",
		"CREATE TABLE A20N (Id INT64 NOT NULL, P a20g.Root) PRIMARY KEY(Id)",
	}); err != nil {
		t.Fatal(err)
	}
	cmds, err := buildCommands(`INSERT INTO A20N (Id,P) VALUES (1,CAST(b"\x0a\x03\x0a\x01x\x12\x03\x0a\x01y" AS a20g.Root))`, source.systemVariables.Query.BuildStatementMode)
	if err != nil {
		t.Fatal(err)
	}
	for _, cmd := range cmds {
		if _, err := source.ExecuteStatement(t.Context(), cmd.stmt); err != nil {
			t.Fatal(err)
		}
	}
	wantB64 := queryProtoBytesB64(t, source, "SELECT TO_BASE64(CAST(P AS BYTES)) FROM A20N")

	for _, mode := range []dumpMode{dumpModeDatabase, dumpModeSchema} {
		for _, streamed := range []bool{false, true} {
			t.Run(fmt.Sprintf("mode=%d/stream=%v", mode, streamed), func(t *testing.T) {
				before := source.systemVariables.Internal.ProtoDescriptor
				out, result, err := tryDumpProto(t, source, mode, streamed, nil)
				if err != nil {
					t.Fatalf("dump failed: %v", err)
				}
				if result.alreadyDelivered() != streamed {
					t.Fatalf("wrong output path: %+v", result)
				}
				if source.systemVariables.Internal.ProtoDescriptor != before {
					t.Fatal("dump changed source graph")
				}
				if !strings.Contains(out, "SET PROTO_DESCRIPTORS = '") {
					t.Fatalf("missing preamble: %s", out)
				}
				target := initializeEmptyTarget(t)
				replayCommands(t, target, out)
				if mode == dumpModeDatabase {
					got := queryProtoBytesB64(t, target, "SELECT TO_BASE64(CAST(P AS BYTES)) FROM A20N")
					if got != wantB64 {
						t.Fatalf("imported/nested wire got %q want %q", got, wantB64)
					}
				}
			})
		}
	}
}

func TestDumpProtoReplayWithoutPreamble(t *testing.T) {
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
		if _, err := source.ExecuteStatement(t.Context(), cmd.stmt); err != nil {
			t.Fatal(err)
		}
	}
	out, _, err := tryDumpProto(t, source, dumpModeDatabase, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	commands, err := buildCommands(out, source.systemVariables.Query.BuildStatementMode)
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) < 2 {
		t.Fatalf("replay commands = %d, want SET plus DDL/data", len(commands))
	}
	if _, ok := commands[0].stmt.(*SetStatement); !ok {
		t.Fatalf("first command = %T, want SET", commands[0].stmt)
	}

	_, stripped := initializeWithRandomDB(t, nil, nil)
	var strippedErr error
	for _, command := range commands[1:] {
		if _, strippedErr = stripped.ExecuteStatement(t.Context(), command.stmt); strippedErr != nil {
			break
		}
	}
	if strippedErr == nil {
		t.Fatal("empty-target replay without preamble succeeded")
	}
	if !strings.Contains(strippedErr.Error(), "proto_descriptor") {
		t.Fatalf("stripped replay error = %v, want missing proto descriptors", strippedErr)
	}

	_, control := initializeWithRandomDB(t, nil, nil)
	for _, command := range commands {
		if _, err := control.ExecuteStatement(t.Context(), command.stmt); err != nil {
			t.Fatalf("descriptor-supplied control: %v", err)
		}
	}
	assertA20Row(t, control)
}

func TestDumpProtoFailClosedAndOrdinaryControls(t *testing.T) {
	skipIfShortIntegration(t)
	_, session := initializeWithRandomDB(t, []string{
		"CREATE TABLE Plain (Id INT64 NOT NULL, Value STRING(16)) PRIMARY KEY(Id)",
	}, []string{"INSERT INTO Plain (Id, Value) VALUES (1, 'x')"})
	validRaw, err := proto.Marshal(a20DescriptorSet(t))
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name    string
		raw     []byte
		stmts   []string
		wantErr string
	}{
		{"missing", nil, []string{"CREATE PROTO BUNDLE (a20.Root)", "CREATE TABLE T (Id INT64 NOT NULL) PRIMARY KEY (Id)"}, "dump requires proto descriptors"},
		{"malformed", []byte("not-a-proto"), []string{"CREATE PROTO BUNDLE (a20.Root)"}, "dump proto descriptors"},
		{"wrong-member", validRaw, []string{"CREATE PROTO BUNDLE (a20.Missing)"}, "missing PROTO BUNDLE type"},
	} {
		for _, mode := range []dumpMode{dumpModeSchema, dumpModeDatabase} {
			for _, streamed := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/mode=%d/stream=%v", tc.name, mode, streamed), func(t *testing.T) {
					session.dumpDDLOverride = func(context.Context) (*adminpb.GetDatabaseDdlResponse, error) {
						return &adminpb.GetDatabaseDdlResponse{Statements: tc.stmts, ProtoDescriptors: tc.raw}, nil
					}
					out, _, err := tryDumpProto(t, session, mode, streamed, nil)
					if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
						t.Fatalf("error = %v, want %q", err, tc.wantErr)
					}
					if out != "" {
						t.Fatalf("wrote bytes before fail-closed: %q", out)
					}
				})
			}
		}
	}

	session.dumpDDLOverride = func(context.Context) (*adminpb.GetDatabaseDdlResponse, error) {
		t.Fatal("GetDatabaseDdlFresh should not be called for TABLES-only dumps")
		return nil, fmt.Errorf("fail-if-called")
	}
	tablesOut, result, err := tryDumpProto(t, session, dumpModeTables, false, []tableID{tid("Plain")})
	if err != nil {
		t.Fatal(err)
	}
	if result.alreadyDelivered() {
		t.Fatal("TABLES-only dump used streaming unexpectedly")
	}
	if strings.Contains(tablesOut, "SET PROTO_DESCRIPTORS") {
		t.Fatalf("TABLES-only dump emitted preamble: %s", tablesOut)
	}

	session.dumpDDLOverride = nil
	for _, mode := range []dumpMode{dumpModeSchema, dumpModeDatabase} {
		out, _, err := tryDumpProto(t, session, mode, false, nil)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out, "SET PROTO_DESCRIPTORS") {
			t.Fatalf("ordinary non-PROTO dump emitted preamble: %s", out)
		}
	}
}

func a20ImportedNestedDescriptorSet(t *testing.T) *descriptorpb.FileDescriptorSet {
	t.Helper()
	dir := t.TempDir()
	child := writeProtoSource(t, filepath.Join(dir, "child.proto"), `syntax="proto3"; package a20g; message Child { string value=1; }`)
	root := writeProtoSource(t, filepath.Join(dir, "root.proto"), fmt.Sprintf(
		`syntax="proto3"; package a20g; import %q; import "google/protobuf/timestamp.proto"; message Root { message Payload { string value=1; } Payload inner=1; Child child=2; google.protobuf.Timestamp ts=3; }`,
		child,
	))
	fds, err := readFileDescriptorProtoFromFile(root)
	if err != nil {
		t.Fatal(err)
	}
	files := requireUsableDescriptor(t, fds)
	for _, name := range []protoreflect.FullName{"a20g.Root", "a20g.Root.Payload", "a20g.Child", "google.protobuf.Timestamp"} {
		if _, err := files.FindDescriptorByName(name); err != nil {
			t.Fatal(err)
		}
	}
	return fds
}

func tryDumpProto(t *testing.T, session *Session, mode dumpMode, streamed bool, tables []tableID) (string, *Result, error) {
	t.Helper()
	var out bytes.Buffer
	var result *Result
	var err error
	run := func() error {
		var inner error
		result, inner = executeDump(t.Context(), session, mode, tables)
		return inner
	}
	if streamed {
		err = session.withOutput(outputContext{w: &out}, run)
	} else {
		err = run()
		if result != nil {
			_, _ = out.Write(result.preparedOutput())
		}
	}
	return out.String(), result, err
}

func initializeEmptyTarget(t *testing.T) *Session {
	t.Helper()
	_, target := initializeWithRandomDB(t, nil, nil)
	return target
}

func replayCommands(t *testing.T, target *Session, sql string) {
	t.Helper()
	commands, err := buildCommands(sql, target.systemVariables.Query.BuildStatementMode)
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) == 0 {
		t.Fatal("no replay commands")
	}
	if _, ok := commands[0].stmt.(*SetStatement); !ok {
		t.Fatalf("first command = %T, want SET", commands[0].stmt)
	}
	for _, command := range commands {
		if _, err := target.ExecuteStatement(t.Context(), command.stmt); err != nil {
			t.Fatalf("unassisted replay: %v", err)
		}
	}
}

func assertA20Row(t *testing.T, session *Session) {
	t.Helper()
	it := session.client.Single().Query(t.Context(), spanner.Statement{SQL: "SELECT TO_BASE64(CAST(P AS BYTES)), CAST(E AS INT64) FROM A20"})
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

func queryProtoBytesB64(t *testing.T, session *Session, sql string) string {
	t.Helper()
	it := session.client.Single().Query(t.Context(), spanner.Statement{SQL: sql})
	defer it.Stop()
	row, err := it.Next()
	if err != nil {
		t.Fatal(err)
	}
	var value string
	if err := row.Columns(&value); err != nil {
		t.Fatal(err)
	}
	return value
}
