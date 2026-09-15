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
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apstndb/spanner-mycli/enums"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

func TestPullRemoteProtoImportedBundle(t *testing.T) {
	t.Parallel()
	skipIfShortIntegration(t)
	dir := t.TempDir()
	child := writeProtoSource(t, filepath.Join(dir, "child.proto"), `syntax="proto3"; package imported; message Child { string value=1; }`)
	root := writeProtoSource(t, filepath.Join(dir, "root.proto"), fmt.Sprintf(`syntax="proto3"; package imported; import %q; import "google/protobuf/timestamp.proto"; message Root { message Payload { string note=1; } Child child=1; Payload payload=2; google.protobuf.Timestamp ts=3; }`, child))
	_, session := initializeWithRandomDB(t, nil, nil)
	if session.dumpDDLOverride != nil {
		t.Fatal("integration PULL must use the real Admin GetDatabaseDdl path")
	}
	handler := NewSessionHandler(session)
	execute := func(sql string) *Result {
		t.Helper()
		stmt, err := BuildStatementWithCommentsWithMode(sql, sql, enums.ParseModeNoMemefish)
		if err != nil {
			t.Fatal(err)
		}
		result, err := handler.ExecuteStatement(t.Context(), stmt)
		if err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return result
	}
	execute(fmt.Sprintf("SET PROTO_DESCRIPTORS_FILE_PATH = %q", root))
	execute("CREATE PROTO BUNDLE (`imported.Root`, `imported.Root.Payload`, `imported.Child`, `google.protobuf.Timestamp`)")

	resp, err := session.GetDatabaseDdlFresh(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var remote descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(resp.GetProtoDescriptors(), &remote); err != nil {
		t.Fatal(err)
	}
	if len(remote.GetFile()) < 2 {
		t.Fatalf("emulator complete bundle returned %d files, want the imported graph", len(remote.GetFile()))
	}

	execute("SET PROTO_DESCRIPTORS = ''")
	if session.systemVariables.Internal.ProtoDescriptor != nil {
		t.Fatal("local store was not cleared")
	}
	result := execute("PULL REMOTE PROTO ALL")
	if result.AffectedRows == 0 || !result.KeepVariables {
		t.Fatalf("PULL ALL result=%+v", result)
	}
	requireUsableDescriptor(t, session.systemVariables.Internal.ProtoDescriptor)
	if _, err := requireUsableDescriptor(t, session.systemVariables.Internal.ProtoDescriptor).FindDescriptorByName("imported.Child"); err != nil {
		t.Fatal(err)
	}

	execute("CREATE TABLE ProtoRows (Id INT64 NOT NULL, P imported.Root) PRIMARY KEY (Id)")
	execute(`INSERT INTO ProtoRows (Id, P) VALUES (1, CAST('child { value: "kept" } payload { note: "n" }' AS imported.Root))`)
	query := execute("SELECT P FROM ProtoRows WHERE Id = 1")
	rows, err := deriveDisplayRows(session.systemVariables, query.typedPayload())
	if err != nil || len(rows) != 1 {
		t.Fatalf("decode rows=%v err=%v", rows, err)
	}
	files := requireUsableDescriptor(t, session.systemVariables.Internal.ProtoDescriptor)
	desc, err := files.FindDescriptorByName("imported.Root")
	if err != nil {
		t.Fatal(err)
	}
	message := dynamicpb.NewMessage(desc.(protoreflect.MessageDescriptor))
	if err := prototext.Unmarshal([]byte(rows[0][0].RawText()), message); err != nil {
		t.Fatalf("decoded proto text = %q: %v", rows[0][0].RawText(), err)
	}
	execute("SYNC PROTO BUNDLE UPSERT (`imported.Root`)")
}

func TestPullRemoteProtoRootOnlyMissingImport(t *testing.T) {
	t.Parallel()
	skipIfShortIntegration(t)
	dir := t.TempDir()
	child := writeProtoSource(t, filepath.Join(dir, "child.proto"), `syntax="proto3"; package imported; message Child { string value=1; }`)
	root := writeProtoSource(t, filepath.Join(dir, "root.proto"), fmt.Sprintf(`syntax="proto3"; package imported; import %q; import "google/protobuf/timestamp.proto"; message Root { Child child=1; google.protobuf.Timestamp ts=2; }`, child))
	_, session := initializeWithRandomDB(t, nil, nil)
	if session.dumpDDLOverride != nil {
		t.Fatal("integration PULL must use the real Admin GetDatabaseDdl path")
	}
	handler := NewSessionHandler(session)
	mustExec := func(sql string) *Result {
		t.Helper()
		stmt, err := BuildStatementWithCommentsWithMode(sql, sql, enums.ParseModeNoMemefish)
		if err != nil {
			t.Fatal(err)
		}
		result, err := handler.ExecuteStatement(t.Context(), stmt)
		if err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return result
	}
	mustExec(fmt.Sprintf("SET PROTO_DESCRIPTORS_FILE_PATH = %q", root))
	mustExec("CREATE PROTO BUNDLE (`imported.Root`)")

	resp, err := session.GetDatabaseDdlFresh(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var remote descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(resp.GetProtoDescriptors(), &remote); err != nil {
		t.Fatal(err)
	}
	if len(remote.GetFile()) != 1 || !strings.HasSuffix(remote.File[0].GetName(), "root.proto") {
		names := make([]string, 0, len(remote.GetFile()))
		for _, file := range remote.GetFile() {
			names = append(names, file.GetName())
		}
		t.Fatalf("root-only emulator response files = %v", names)
	}

	mustExec("SET PROTO_DESCRIPTORS = ''")
	stmt, err := BuildStatement("PULL REMOTE PROTO imported.Root")
	if err != nil {
		t.Fatal(err)
	}
	_, err = handler.ExecuteStatement(t.Context(), stmt)
	if err == nil || !strings.Contains(err.Error(), "missing import") {
		t.Fatalf("empty local pull err=%v", err)
	}
	if session.systemVariables.Internal.ProtoDescriptor != nil || len(session.systemVariables.Internal.ProtoDescriptorFile) != 0 {
		t.Fatal("failed root-only pull mutated local state")
	}

	mustExec(fmt.Sprintf("SET PROTO_DESCRIPTORS_FILE_PATH = %q", root))
	result := mustExec("PULL REMOTE PROTO imported.Root")
	if result.AffectedRows != 1 {
		t.Fatalf("local-reuse pull rows=%d", result.AffectedRows)
	}
	if _, err := requireUsableDescriptor(t, session.systemVariables.Internal.ProtoDescriptor).FindDescriptorByName("imported.Child"); err != nil {
		t.Fatal(err)
	}
}
