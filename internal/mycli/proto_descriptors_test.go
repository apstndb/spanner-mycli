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
	"encoding/base64"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

func a20DescriptorSet(t *testing.T) *descriptorpb.FileDescriptorSet {
	t.Helper()
	file := descriptorFile("a20.proto", "a20")
	file.MessageType[0].Field = []*descriptorpb.FieldDescriptorProto{{
		Name: proto.String("value"), Number: proto.Int32(1),
		Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
	}}
	file.EnumType = []*descriptorpb.EnumDescriptorProto{{
		Name: proto.String("State"),
		Value: []*descriptorpb.EnumValueDescriptorProto{
			{Name: proto.String("ZERO"), Number: proto.Int32(0)},
			{Name: proto.String("ANSWER"), Number: proto.Int32(42)},
		},
	}}
	return &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{file}}
}

func TestProtoDescriptorsSetShowClear(t *testing.T) {
	t.Parallel()
	sv := newSystemVariablesWithDefaultsForTest()
	sv.ensureRegistry()
	fds := a20DescriptorSet(t)
	encoded, err := encodeProtoDescriptors(fds)
	if err != nil {
		t.Fatal(err)
	}
	if err := sv.SetFromSimple(protoDescriptorsVarName, encoded); err != nil {
		t.Fatal(err)
	}
	if sv.Internal.ProtoDescriptorFile != nil {
		t.Fatalf("file provenance = %v, want cleared", sv.Internal.ProtoDescriptorFile)
	}
	if !proto.Equal(fds, sv.Internal.ProtoDescriptor) {
		t.Fatal("SET did not install the expected graph")
	}
	assertShowGraph(t, sv, fds)
	unpadded := strings.TrimRight(encoded, "=")
	if err := sv.SetFromSimple(protoDescriptorsVarName, unpadded); err != nil {
		t.Fatalf("unpadded standard base64: %v", err)
	}
	assertShowGraph(t, sv, fds)
	if err := sv.SetFromSimple(protoDescriptorsVarName, ""); err != nil {
		t.Fatal(err)
	}
	if sv.Internal.ProtoDescriptor != nil {
		t.Fatal("empty SET did not clear graph")
	}
	assertShowGraph(t, sv, nil)
}

func TestProtoDescriptorsInvalidAtomic(t *testing.T) {
	t.Parallel()
	sv := newSystemVariablesWithDefaultsForTest()
	sv.ensureRegistry()
	if err := sv.SetFromSimple("CLI_PROTO_DESCRIPTOR_FILE", "testdata/protos/order_descriptors.pb"); err != nil {
		t.Fatal(err)
	}
	beforeGraph, beforeFiles := cloneDescriptorState(sv)
	if len(beforeFiles) == 0 || beforeGraph == nil {
		t.Fatal("expected nonempty file provenance before invalid SET")
	}
	broken := &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{{
		Name: proto.String("broken.proto"), Dependency: []string{"missing.proto"},
	}}}
	badGraph, err := proto.Marshal(broken)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"@@@@", base64.StdEncoding.EncodeToString([]byte("not-a-proto")), base64.StdEncoding.EncodeToString(badGraph)} {
		if err := sv.SetFromSimple(protoDescriptorsVarName, bad); err == nil {
			t.Fatalf("invalid SET %q succeeded", bad)
		}
		assertDescriptorState(t, sv, beforeGraph, beforeFiles)
	}
}

func TestProtoDescriptorsNoLocalAndBatchGuard(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	ctx := t.Context()
	sv := session.systemVariables
	if err := sv.SetFromSimple("CLI_PROTO_DESCRIPTOR_FILE", "testdata/protos/order_descriptors.pb"); err != nil {
		t.Fatal(err)
	}
	fileGraph, fileList := cloneDescriptorState(sv)

	if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
		t.Fatal(err)
	}
	_, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: protoDescriptorsVarName, Value: "''"})
	if err == nil || err.Error() != protoDescriptorsVarName+" does not support SET LOCAL" {
		t.Fatalf("SET LOCAL error = %v", err)
	}
	assertDescriptorState(t, sv, fileGraph, fileList)
	if _, err := session.ExecuteStatement(ctx, &RollbackStatement{}); err != nil {
		t.Fatal(err)
	}

	inline := protoDescriptorsGoogleSQL(t, a20DescriptorSet(t))
	for _, mode := range []batchMode{batchModeDDL, batchModeDML} {
		if err := sv.SetFromSimple("CLI_PROTO_DESCRIPTOR_FILE", "testdata/protos/order_descriptors.pb"); err != nil {
			t.Fatal(err)
		}
		beforeGraph, beforeFiles := cloneDescriptorState(sv)
		if err := session.batch.Start(mode); err != nil {
			t.Fatal(err)
		}
		_, err := session.ExecuteStatement(ctx, &SetStatement{VarName: protoDescriptorsVarName, Value: inline})
		if err == nil || err.Error() != "PROTO_DESCRIPTORS cannot be set while a batch is active" {
			t.Fatalf("batch %v SET error = %v", mode, err)
		}
		assertDescriptorState(t, sv, beforeGraph, beforeFiles)
		if _, err := session.ExecuteStatement(ctx, &AbortBatchStatement{}); err != nil {
			t.Fatal(err)
		}
		if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: protoDescriptorsVarName, Value: inline}); err != nil {
			t.Fatalf("SET after aborting %v batch: %v", mode, err)
		}
		if !proto.Equal(a20DescriptorSet(t), sv.Internal.ProtoDescriptor) {
			t.Fatalf("SET after aborting %v batch installed the wrong graph", mode)
		}
		assertShowGraph(t, sv, a20DescriptorSet(t))
		if len(sv.Internal.ProtoDescriptorFile) != 0 {
			t.Fatalf("successful SET left file list %v", sv.Internal.ProtoDescriptorFile)
		}
	}
}

func TestProtoDescriptorsFileCoexistence(t *testing.T) {
	t.Parallel()
	sv := newSystemVariablesWithDefaultsForTest()
	sv.ensureRegistry()
	if err := sv.SetFromSimple("CLI_PROTO_DESCRIPTOR_FILE", "testdata/protos/order_descriptors.pb"); err != nil {
		t.Fatal(err)
	}
	fileGraph, fileList := cloneDescriptorState(sv)
	assertShowGraph(t, sv, fileGraph)
	if len(fileList) == 0 {
		t.Fatal("file SET left empty provenance")
	}

	inline := a20DescriptorSet(t)
	encoded, err := encodeProtoDescriptors(inline)
	if err != nil {
		t.Fatal(err)
	}
	if err := sv.SetFromSimple(protoDescriptorsVarName, encoded); err != nil {
		t.Fatal(err)
	}
	if len(sv.Internal.ProtoDescriptorFile) != 0 {
		t.Fatalf("inline SET left file list %v", sv.Internal.ProtoDescriptorFile)
	}
	assertShowGraph(t, sv, inline)

	orderPath := "testdata/protos/order_descriptors.pb"
	if err := sv.AddFromSimple("CLI_PROTO_DESCRIPTOR_FILE", orderPath); err != nil {
		t.Fatal(err)
	}
	merged := sv.Internal.ProtoDescriptor
	requireUsableDescriptor(t, merged)
	if !containsDescriptorFile(merged, "a20.proto") || !containsDescriptorPackage(merged, "examples.shipping") {
		t.Fatalf("ADD after inline SET did not keep both graphs: %v", merged)
	}
	if !slices.Equal(sv.Internal.ProtoDescriptorFile, []string{orderPath}) {
		t.Fatalf("file list after ADD = %v", sv.Internal.ProtoDescriptorFile)
	}
	assertShowGraph(t, sv, merged)

	badPath := writeDescriptorSet(t, filepath.Join(t.TempDir(), "bad.pb"), &descriptorpb.FileDescriptorProto{
		Name: proto.String("broken.proto"), Dependency: []string{"missing.proto"},
	})
	beforeGraph, beforeFiles := cloneDescriptorState(sv)
	if err := sv.AddFromSimple("CLI_PROTO_DESCRIPTOR_FILE", badPath); err == nil {
		t.Fatal("invalid ADD succeeded")
	}
	assertDescriptorState(t, sv, beforeGraph, beforeFiles)
	assertShowGraph(t, sv, beforeGraph)

	if err := sv.SetFromSimple("CLI_PROTO_DESCRIPTOR_FILE", "testdata/protos/singer.proto"); err != nil {
		t.Fatal(err)
	}
	replaced := sv.Internal.ProtoDescriptor
	requireUsableDescriptor(t, replaced)
	if containsDescriptorFile(replaced, "a20.proto") || containsDescriptorPackage(replaced, "examples.shipping") {
		t.Fatalf("replacing file SET kept previous types: %v", replaced)
	}
	if !containsDescriptorPackage(replaced, "examples.spanner.music") {
		t.Fatalf("replacing file SET missing singer types: %v", replaced)
	}
	if !slices.Equal(sv.Internal.ProtoDescriptorFile, []string{"testdata/protos/singer.proto"}) {
		t.Fatalf("file list after replacing SET = %v", sv.Internal.ProtoDescriptorFile)
	}
	assertShowGraph(t, sv, replaced)
}

func TestDumpProtoDescriptorsPreamble(t *testing.T) {
	t.Parallel()
	fds := a20DescriptorSet(t)
	raw, err := proto.Marshal(fds)
	if err != nil {
		t.Fatal(err)
	}
	stmts := []string{"CREATE PROTO BUNDLE (a20.Root, a20.State)", "CREATE TABLE T (Id INT64 NOT NULL) PRIMARY KEY (Id)"}
	preamble, err := dumpProtoDescriptorsPreamble(raw, stmts)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(preamble), "SET PROTO_DESCRIPTORS = '") {
		t.Fatalf("preamble = %q", preamble)
	}
	if _, err := dumpProtoDescriptorsPreamble(nil, stmts); err == nil {
		t.Fatal("missing descriptors for PROTO BUNDLE succeeded")
	}
	if _, err := dumpProtoDescriptorsPreamble(raw, []string{"CREATE PROTO BUNDLE (a20.Missing)"}); err == nil {
		t.Fatal("missing bundle member succeeded")
	}
	empty, err := dumpProtoDescriptorsPreamble(nil, []string{"CREATE TABLE T (Id INT64 NOT NULL) PRIMARY KEY (Id)"})
	if err != nil || empty != nil {
		t.Fatalf("non-PROTO dump preamble = %q err=%v", empty, err)
	}
}

func decodeMust(t *testing.T, encoded string) *descriptorpb.FileDescriptorSet {
	t.Helper()
	raw, err := decodeProtoDescriptorBytes(encoded)
	if err != nil {
		t.Fatal(err)
	}
	fds, err := parseProtoDescriptorsGraph(raw)
	if err != nil {
		t.Fatal(err)
	}
	return fds
}

func deterministicPaddedEncoding(t *testing.T, fds *descriptorpb.FileDescriptorSet) string {
	t.Helper()
	if fds == nil {
		return ""
	}
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(fds)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func assertShowGraph(t *testing.T, sv *systemVariables, want *descriptorpb.FileDescriptorSet) {
	t.Helper()
	got, err := sv.Registry.Get(protoDescriptorsVarName)
	if err != nil {
		t.Fatal(err)
	}
	if want == nil {
		if got != "" {
			t.Fatalf("SHOW = %q, want empty", got)
		}
		return
	}
	if !proto.Equal(want, decodeMust(t, got)) {
		t.Fatal("SHOW decoded graph does not equal the effective graph")
	}
	if got != deterministicPaddedEncoding(t, want) {
		t.Fatalf("SHOW encoding = %q, want independently computed deterministic padded encoding", got)
	}
}

func cloneDescriptorState(sv *systemVariables) (*descriptorpb.FileDescriptorSet, []string) {
	var graph *descriptorpb.FileDescriptorSet
	if sv.Internal.ProtoDescriptor != nil {
		graph = proto.Clone(sv.Internal.ProtoDescriptor).(*descriptorpb.FileDescriptorSet)
	}
	return graph, slices.Clone(sv.Internal.ProtoDescriptorFile)
}

func assertDescriptorState(t *testing.T, sv *systemVariables, graph *descriptorpb.FileDescriptorSet, files []string) {
	t.Helper()
	if !proto.Equal(graph, sv.Internal.ProtoDescriptor) {
		t.Fatal("descriptor graph changed")
	}
	if !slices.Equal(files, sv.Internal.ProtoDescriptorFile) {
		t.Fatalf("file provenance = %v, want %v", sv.Internal.ProtoDescriptorFile, files)
	}
}

func protoDescriptorsGoogleSQL(t *testing.T, fds *descriptorpb.FileDescriptorSet) string {
	t.Helper()
	encoded, err := encodeProtoDescriptors(fds)
	if err != nil {
		t.Fatal(err)
	}
	return "'" + encoded + "'"
}

func containsDescriptorFile(fds *descriptorpb.FileDescriptorSet, name string) bool {
	if fds == nil {
		return false
	}
	for _, file := range fds.File {
		if file.GetName() == name {
			return true
		}
	}
	return false
}

func containsDescriptorPackage(fds *descriptorpb.FileDescriptorSet, pkg string) bool {
	if fds == nil {
		return false
	}
	for _, file := range fds.File {
		if file.GetPackage() == pkg {
			return true
		}
	}
	return false
}
