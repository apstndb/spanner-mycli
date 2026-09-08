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
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/apstndb/spanner-mycli/internal/mycli/decoder"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
)

func writeDescriptorSet(t *testing.T, path string, files ...*descriptorpb.FileDescriptorProto) string {
	t.Helper()
	b, err := proto.Marshal(&descriptorpb.FileDescriptorSet{File: files})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func descriptorFile(name, pkg string) *descriptorpb.FileDescriptorProto {
	return &descriptorpb.FileDescriptorProto{
		Name: proto.String(name), Package: proto.String(pkg), Syntax: proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{{Name: proto.String("Root")}},
	}
}

func requireUsableDescriptor(t *testing.T, fds *descriptorpb.FileDescriptorSet) *protoregistry.Files {
	t.Helper()
	files, err := protodesc.NewFiles(fds)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decoder.FormatConfigWithProto(fds, false); err != nil {
		t.Fatalf("actual decoder rejected descriptors: %v", err)
	}
	return files
}

func writeProtoSource(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return filepath.ToSlash(path)
}

func TestProtoDescriptorDependencyClosure(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dep := writeProtoSource(t, filepath.Join(dir, "dep.proto"), `syntax="proto3"; package graph; message Dep { string value=1; }`)
	left := writeProtoSource(t, filepath.Join(dir, "left.proto"), fmt.Sprintf(`syntax="proto3"; package graph; import %q; message Left { Dep child=1; }`, dep))
	right := writeProtoSource(t, filepath.Join(dir, "right.proto"), fmt.Sprintf(`syntax="proto3"; package graph; import %q; message Right { Dep child=1; }`, dep))
	root := writeProtoSource(t, filepath.Join(dir, "root.proto"), fmt.Sprintf(`syntax="proto3"; package graph; import %q; import %q; message Root { Left left=1; Right right=2; }`, left, right))
	wantNames := []string{dep, left, right, root}

	fds, err := readFileDescriptorProtoFromFile(root)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, file := range fds.File {
		names = append(names, file.GetName())
	}
	if diff := cmp.Diff(wantNames, names); diff != "" {
		t.Fatalf("dependency-first diamond closure (-want +got):\n%s", diff)
	}
	files := requireUsableDescriptor(t, fds)
	if _, err := files.FindDescriptorByName("graph.Dep"); err != nil {
		t.Fatal(err)
	}
	again, err := readFileDescriptorProtoFromFile(root)
	if err != nil || !proto.Equal(fds, again) {
		t.Fatalf("repeated compilation changed graph: err=%v", err)
	}

	var sv systemVariables
	if err := sv.SetFromSimple("CLI_PROTO_DESCRIPTOR_FILE", left+","+root); err != nil {
		t.Fatal(err)
	}
	if len(sv.Internal.ProtoDescriptor.File) != 4 {
		t.Fatalf("multiple roots duplicated shared dependencies: %v", sv.Internal.ProtoDescriptor)
	}
	requireUsableDescriptor(t, sv.Internal.ProtoDescriptor)
}

func TestProtoDescriptorStandardImport(t *testing.T) {
	t.Parallel()
	root := writeProtoSource(t, filepath.Join(t.TempDir(), "root.proto"), `syntax="proto3"; package standard; import "google/protobuf/timestamp.proto"; message Root { google.protobuf.Timestamp value=1; }`)
	fds, err := readFileDescriptorProtoFromFile(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(fds.File) != 2 || fds.File[0].GetName() != "google/protobuf/timestamp.proto" {
		t.Fatalf("missing standard import: %v", fds)
	}
	requireUsableDescriptor(t, fds)
}

func TestProtoDescriptorInvalidGraphPreservesState(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"SET", "ADD"} {
		for _, failure := range []string{"missing import", "duplicate symbol", "replaced dependency"} {
			t.Run(operation+"/"+failure, func(t *testing.T) {
				t.Parallel()
				dir := t.TempDir()
				good := descriptorFile("base.proto", "base")
				goodFiles := []*descriptorpb.FileDescriptorProto{good}
				bad := descriptorFile("broken.proto", "broken")
				bad.Dependency = []string{"missing.proto"}
				badFiles := []*descriptorpb.FileDescriptorProto{bad}
				if failure == "duplicate symbol" {
					bad = descriptorFile("duplicate.proto", "base")
					badFiles = []*descriptorpb.FileDescriptorProto{good, bad}
				}
				if failure == "replaced dependency" {
					consumer := descriptorFile("consumer.proto", "consumer")
					consumer.Dependency = []string{"base.proto"}
					consumer.MessageType[0].Field = []*descriptorpb.FieldDescriptorProto{{
						Name: proto.String("child"), Number: proto.Int32(1),
						Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Type:  descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(), TypeName: proto.String(".base.Root"),
					}}
					goodFiles = append(goodFiles, consumer)
					bad = descriptorFile("base.proto", "renamed")
					// This replacement is valid alone but invalidates an existing
					// consumer; only whole-candidate validation detects that.
					requireUsableDescriptor(t, &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{bad}})
					badFiles = []*descriptorpb.FileDescriptorProto{bad, consumer}
				}
				goodPath := writeDescriptorSet(t, filepath.Join(dir, "good.pb"), goodFiles...)
				badPath := writeDescriptorSet(t, filepath.Join(dir, "bad.pb"), badFiles...)
				var sv systemVariables
				if err := sv.SetFromSimple("CLI_PROTO_DESCRIPTOR_FILE", goodPath); err != nil {
					t.Fatal(err)
				}
				before := proto.Clone(sv.Internal.ProtoDescriptor)
				beforePaths := slices.Clone(sv.Internal.ProtoDescriptorFile)
				var err error
				if operation == "SET" {
					err = sv.SetFromSimple("CLI_PROTO_DESCRIPTOR_FILE", badPath)
				} else {
					err = sv.AddFromSimple("CLI_PROTO_DESCRIPTOR_FILE", badPath)
				}
				if err == nil || !strings.Contains(err.Error(), "invalid proto descriptor set") {
					t.Errorf("invalid graph error = %v", err)
				}
				if !proto.Equal(before, sv.Internal.ProtoDescriptor) || !slices.Equal(beforePaths, sv.Internal.ProtoDescriptorFile) {
					t.Error("failed load changed descriptor or file list")
				}
				requireUsableDescriptor(t, sv.Internal.ProtoDescriptor)
				if err := sv.AddFromSimple("CLI_PROTO_DESCRIPTOR_FILE", goodPath); err != nil || len(sv.Internal.ProtoDescriptorFile) != 1 {
					t.Fatalf("duplicate ADD no longer a no-op: err=%v", err)
				}
				if err := sv.SetFromSimple("CLI_PROTO_DESCRIPTOR_FILE", ""); err != nil || sv.Internal.ProtoDescriptor != nil || len(sv.Internal.ProtoDescriptorFile) != 0 {
					t.Fatalf("empty reset failed: %v", err)
				}
			})
		}
	}
}

func TestProtoDescriptorFileIdentityReplacement(t *testing.T) {
	t.Parallel()
	for _, pkg := range []string{"old", "new"} {
		t.Run(pkg, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			first := writeDescriptorSet(t, filepath.Join(dir, "first.pb"), descriptorFile("shared.proto", "old"))
			replacement := descriptorFile("shared.proto", pkg)
			replacement.MessageType = append(replacement.MessageType, &descriptorpb.DescriptorProto{Name: proto.String("Added")})
			second := writeDescriptorSet(t, filepath.Join(dir, "second.pb"), replacement)
			var sv systemVariables
			if err := sv.SetFromSimple("CLI_PROTO_DESCRIPTOR_FILE", first+","+second); err != nil {
				t.Fatal(err)
			}
			if len(sv.Internal.ProtoDescriptor.File) != 1 {
				t.Fatalf("file identity duplicated: %v", sv.Internal.ProtoDescriptor)
			}
			files := requireUsableDescriptor(t, sv.Internal.ProtoDescriptor)
			if _, err := files.FindDescriptorByName(protoreflect.FullName(pkg + ".Added")); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestApplyProtoDescriptorsCombinedInputs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dep := descriptorFile("dep.proto", "dep")
	root := descriptorFile("root.proto", "root")
	root.Dependency = []string{"dep.proto"}
	root.MessageType[0].Field = []*descriptorpb.FieldDescriptorProto{{
		Name: proto.String("child"), Number: proto.Int32(1),
		Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
		Type:  descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(), TypeName: proto.String(".dep.Root"),
	}}
	rootPath := writeDescriptorSet(t, filepath.Join(dir, "root.pb"), root)
	depPath := writeDescriptorSet(t, filepath.Join(dir, "dep.pb"), dep)
	for _, path := range []string{rootPath + "," + depPath, depPath + "," + rootPath} {
		var sv systemVariables
		if err := applyProtoDescriptors(&sv, &spannerOptions{ProtoDescriptorFile: path}); err != nil {
			t.Fatal(err)
		}
		requireUsableDescriptor(t, sv.Internal.ProtoDescriptor)
		if len(sv.Internal.ProtoDescriptorFile) != 2 {
			t.Fatalf("startup file list = %v", sv.Internal.ProtoDescriptorFile)
		}
	}
}
