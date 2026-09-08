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
	got, err := sv.Registry.Get(protoDescriptorsVarName)
	if err != nil {
		t.Fatal(err)
	}
	if got != encoded && decodeMust(t, got) == nil {
		t.Fatal("SHOW did not round-trip a valid graph")
	}
	unpadded := strings.TrimRight(encoded, "=")
	if err := sv.SetFromSimple(protoDescriptorsVarName, unpadded); err != nil {
		t.Fatalf("unpadded standard base64: %v", err)
	}
	if err := sv.SetFromSimple(protoDescriptorsVarName, ""); err != nil {
		t.Fatal(err)
	}
	if sv.Internal.ProtoDescriptor != nil {
		t.Fatal("empty SET did not clear graph")
	}
}

func TestProtoDescriptorsInvalidAtomic(t *testing.T) {
	t.Parallel()
	sv := newSystemVariablesWithDefaultsForTest()
	sv.ensureRegistry()
	good, err := encodeProtoDescriptors(a20DescriptorSet(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := sv.SetFromSimple(protoDescriptorsVarName, good); err != nil {
		t.Fatal(err)
	}
	before := proto.Clone(sv.Internal.ProtoDescriptor)
	for _, bad := range []string{"@@@@", base64.StdEncoding.EncodeToString([]byte("not-a-proto"))} {
		if err := sv.SetFromSimple(protoDescriptorsVarName, bad); err == nil {
			t.Fatalf("invalid SET %q succeeded", bad)
		}
		if !proto.Equal(before, sv.Internal.ProtoDescriptor) {
			t.Fatal("invalid SET mutated graph")
		}
	}
	broken := &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{{
		Name: proto.String("broken.proto"), Dependency: []string{"missing.proto"},
	}}}
	badGraph, err := proto.Marshal(broken)
	if err != nil {
		t.Fatal(err)
	}
	if err := sv.SetFromSimple(protoDescriptorsVarName, base64.StdEncoding.EncodeToString(badGraph)); err == nil {
		t.Fatal("invalid graph SET succeeded")
	}
	if !proto.Equal(before, sv.Internal.ProtoDescriptor) {
		t.Fatal("invalid graph SET mutated graph")
	}
}

func TestProtoDescriptorsNoLocalAndBatchGuard(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	ctx := t.Context()
	if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: protoDescriptorsVarName, Value: "''"}); err == nil {
		t.Fatal("SET LOCAL PROTO_DESCRIPTORS succeeded")
	}
	if _, err := session.ExecuteStatement(ctx, &RollbackStatement{}); err != nil {
		t.Fatal(err)
	}
	if err := session.batch.Start(batchModeDDL); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: protoDescriptorsVarName, Value: "''"}); err == nil || !strings.Contains(err.Error(), "batch is active") {
		t.Fatalf("batch SET error = %v", err)
	}
}

func TestProtoDescriptorsFileCoexistence(t *testing.T) {
	t.Parallel()
	sv := newSystemVariablesWithDefaultsForTest()
	sv.ensureRegistry()
	if err := sv.SetFromSimple("CLI_PROTO_DESCRIPTOR_FILE", "testdata/protos/order_descriptors.pb"); err != nil {
		t.Fatal(err)
	}
	encoded, err := sv.Registry.Get(protoDescriptorsVarName)
	if err != nil || encoded == "" {
		t.Fatalf("SHOW after file SET: %q %v", encoded, err)
	}
	if err := sv.SetFromSimple(protoDescriptorsVarName, encoded); err != nil {
		t.Fatal(err)
	}
	if len(sv.Internal.ProtoDescriptorFile) != 0 {
		t.Fatalf("inline SET left file list %v", sv.Internal.ProtoDescriptorFile)
	}
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
