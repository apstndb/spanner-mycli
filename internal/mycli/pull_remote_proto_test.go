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
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanner-mycli/internal/mycli/decoder"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestParsePullRemoteProto(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		sql  string
		want *PullRemoteProtoStatement
	}{
		{sql: "PULL REMOTE PROTO ALL", want: &PullRemoteProtoStatement{All: true}},
		{sql: "pull remote proto all", want: &PullRemoteProtoStatement{All: true}},
		{sql: "PULL REMOTE PROTO examples.shipping.Order", want: &PullRemoteProtoStatement{Names: sliceOf("examples.shipping.Order")}},
		{sql: "PULL REMOTE PROTO examples.shipping.Order.Item", want: &PullRemoteProtoStatement{Names: sliceOf("examples.shipping.Order.Item")}},
		{sql: "PULL REMOTE PROTO `examples.shipping.Order`", want: &PullRemoteProtoStatement{Names: sliceOf("examples.shipping.Order")}},
		{
			sql:  "PULL REMOTE PROTO (examples.shipping.Order, examples.shipping.Customer, examples.shipping.Order)",
			want: &PullRemoteProtoStatement{Names: sliceOf("examples.shipping.Order", "examples.shipping.Customer")},
		},
	} {
		t.Run(tt.sql, func(t *testing.T) {
			t.Parallel()
			got, err := BuildStatement(tt.sql)
			if err != nil {
				t.Fatalf("BuildStatement(%q) error = %v", tt.sql, err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("BuildStatement(%q) mismatch (-want +got):\n%s", tt.sql, diff)
			}
		})
	}
}

func TestParsePullRemoteProtoInvalid(t *testing.T) {
	t.Parallel()
	for _, sql := range []string{
		"PULL REMOTE PROTO",
		"PULL REMOTE PROTO ()",
		"PULL REMOTE PROTO ALL leftover",
		"PULL REMOTE PROTO ALL, examples.shipping.Order",
		"PULL REMOTE PROTO (examples.A) leftover",
		"PULL REMOTE PROTO examples.A leftover",
		"PULL REMOTE PROTO (",
	} {
		t.Run(sql, func(t *testing.T) {
			t.Parallel()
			got, err := BuildStatement(sql)
			if err == nil || got != nil {
				t.Fatalf("BuildStatement(%q) = %#v, %v; want error", sql, got, err)
			}
		})
	}
}

func TestPlanPullRemoteProtoSelectionAndErrors(t *testing.T) {
	t.Parallel()
	remote := lexicalSelectionFDS()
	money := &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{remote.File[0]}}
	order := &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{remote.File[1]}}

	t.Run("ALL empty remote is no-op", func(t *testing.T) {
		t.Parallel()
		planned, err := planPullRemoteProto(nil, &descriptorpb.FileDescriptorSet{}, true, nil)
		if err != nil {
			t.Fatal(err)
		}
		if planned.changed || len(planned.selected) != 0 || planned.candidate != nil {
			t.Fatalf("empty ALL = %+v", planned)
		}
	})

	t.Run("listed missing placeholder and map-entry fail before mutation", func(t *testing.T) {
		t.Parallel()
		local := proto.Clone(money).(*descriptorpb.FileDescriptorSet)
		for _, tt := range []struct {
			name    string
			wantErr string
		}{
			{name: "examples.missing.Type", wantErr: `unknown type "examples.missing.Type"`},
			{name: "examples.shipping.Order.Ghost", wantErr: `is a placeholder descriptor`},
			{name: "examples.shipping.Order.AttrEntry", wantErr: `is a synthetic map entry`},
		} {
			planned, err := planPullRemoteProto(local, remote, false, []string{tt.name})
			if err == nil || planned != nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("%s: err=%v planned=%v", tt.name, err, planned)
			}
			if !proto.Equal(local, money) {
				t.Fatal("planner mutated the local graph")
			}
		}
	})

	t.Run("ALL skips placeholder and map-entry", func(t *testing.T) {
		t.Parallel()
		// order.proto does not import money.proto, so ALL cannot install.
		_, err := planPullRemoteProto(nil, remote, true, nil)
		if err == nil || !strings.Contains(err.Error(), "invalid proto descriptor set") {
			t.Fatalf("ALL without import closure: %v", err)
		}
		_, err = planPullRemoteProto(nil, money, true, nil)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("do not invent missing imports", func(t *testing.T) {
		t.Parallel()
		_, err := planPullRemoteProto(nil, order, false, []string{"examples.shipping.Order"})
		if err == nil || !strings.Contains(err.Error(), "invalid proto descriptor set") {
			t.Fatalf("Order without money import: %v", err)
		}
	})
}

func TestPlanPullRemoteProtoImportClosureAndLocalReuse(t *testing.T) {
	t.Parallel()
	compiled := compiledRootDepFDS(t)
	rootOnly := rootOnlyRemote(compiled)

	t.Run("complete remote closure into empty store", func(t *testing.T) {
		t.Parallel()
		planned, err := planPullRemoteProto(nil, compiled, false, []string{"graph.Root"})
		if err != nil {
			t.Fatal(err)
		}
		if !planned.changed || len(planned.selected) != 1 || planned.selected[0].FullName != "graph.Root" {
			t.Fatalf("selected=%v changed=%v", infos(planned.selected), planned.changed)
		}
		requireUsableDescriptor(t, planned.candidate)
		if _, err := requireUsableDescriptor(t, planned.candidate).FindDescriptorByName("graph.Dep"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("missing remote import supplied by local file", func(t *testing.T) {
		t.Parallel()
		local := &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{fileByName(t, compiled, compiled.File[0].GetName())}}
		planned, err := planPullRemoteProto(local, rootOnly, false, []string{"graph.Root"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := requireUsableDescriptor(t, planned.candidate).FindDescriptorByName("graph.Dep"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("genuinely missing import preserves caller state", func(t *testing.T) {
		t.Parallel()
		_, err := planPullRemoteProto(nil, rootOnly, false, []string{"graph.Root"})
		if err == nil || !strings.Contains(err.Error(), "missing import") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("unrelated broken remote file does not block a complete component", func(t *testing.T) {
		t.Parallel()
		broken := proto.Clone(compiled).(*descriptorpb.FileDescriptorSet)
		broken.File = append(broken.File, &descriptorpb.FileDescriptorProto{
			Name:       proto.String("broken.proto"),
			Package:    proto.String("broken"),
			Syntax:     proto.String("proto3"),
			Dependency: []string{"missing.proto"},
			MessageType: []*descriptorpb.DescriptorProto{
				{Name: proto.String("Broken")},
			},
		})
		planned, err := planPullRemoteProto(nil, broken, false, []string{"graph.Root"})
		if err != nil {
			t.Fatal(err)
		}
		requireUsableDescriptor(t, planned.candidate)
		if fileByNameOK(planned.candidate, "broken.proto") {
			t.Fatal("unrelated broken file was installed")
		}
	})
}

func TestPlanPullRemoteProtoMergeConflictsAndNoop(t *testing.T) {
	t.Parallel()

	t.Run("same-file complete update", func(t *testing.T) {
		t.Parallel()
		local := simpleMessageFDS("shared.proto", "pkg", "Root", "old")
		remote := simpleMessageFDS("shared.proto", "pkg", "Root", "new")
		remote.File[0].MessageType[0].Field = append(remote.File[0].MessageType[0].Field, &descriptorpb.FieldDescriptorProto{
			Name:   proto.String("extra"),
			Number: proto.Int32(2),
			Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
		})
		planned, err := planPullRemoteProto(local, remote, false, []string{"pkg.Root"})
		if err != nil || !planned.changed {
			t.Fatalf("err=%v changed=%v", err, planned != nil && planned.changed)
		}
		requireUsableDescriptor(t, planned.candidate)
		if len(planned.candidate.File[0].MessageType[0].Field) != 2 {
			t.Fatal("updated definition was not installed")
		}
	})

	t.Run("unchanged pull is a no-op", func(t *testing.T) {
		t.Parallel()
		fds := simpleMessageFDS("shared.proto", "pkg", "Root", "value")
		planned, err := planPullRemoteProto(fds, proto.Clone(fds).(*descriptorpb.FileDescriptorSet), false, []string{"pkg.Root"})
		if err != nil || planned.changed {
			t.Fatalf("err=%v changed=%v", err, planned != nil && planned.changed)
		}
	})

	t.Run("untouched local files survive", func(t *testing.T) {
		t.Parallel()
		local := mergeFDS(
			simpleMessageFDS("keep.proto", "keep", "Kept", "value"),
			simpleMessageFDS("shared.proto", "pkg", "Root", "old"),
		)
		remote := simpleMessageFDS("shared.proto", "pkg", "Root", "new")
		planned, err := planPullRemoteProto(local, remote, false, []string{"pkg.Root"})
		if err != nil {
			t.Fatal(err)
		}
		if !fileByNameOK(planned.candidate, "keep.proto") {
			t.Fatal("untouched local file was dropped")
		}
		requireUsableDescriptor(t, planned.candidate)
	})

	t.Run("removal of a complete local sibling is rejected", func(t *testing.T) {
		t.Parallel()
		local := simpleMessageFDS("shared.proto", "pkg", "Root", "value")
		local.File[0].MessageType = append(local.File[0].MessageType, &descriptorpb.DescriptorProto{
			Name: proto.String("Sibling"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name: proto.String("value"), Number: proto.Int32(1),
				Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
			}},
		})
		remote := simpleMessageFDS("shared.proto", "pkg", "Root", "value")
		_, err := planPullRemoteProto(local, remote, false, []string{"pkg.Root"})
		if err == nil || !strings.Contains(err.Error(), `would remove complete type "pkg.Sibling"`) {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("placeholder downgrade is rejected", func(t *testing.T) {
		t.Parallel()
		local := simpleMessageFDS("shared.proto", "pkg", "Root", "value")
		local.File[0].MessageType = append(local.File[0].MessageType, &descriptorpb.DescriptorProto{
			Name: proto.String("Sibling"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name: proto.String("value"), Number: proto.Int32(1),
				Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
			}},
		})
		remote := simpleMessageFDS("shared.proto", "pkg", "Root", "value")
		remote.File[0].MessageType = append(remote.File[0].MessageType, placeholderDescriptor("Sibling"))
		_, err := planPullRemoteProto(local, remote, false, []string{"pkg.Root"})
		if err == nil || !strings.Contains(err.Error(), `would downgrade complete type "pkg.Sibling"`) {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("ambiguous remote type identity is rejected", func(t *testing.T) {
		t.Parallel()
		remote := mergeFDS(
			simpleMessageFDS("a.proto", "pkg", "Root", "value"),
			simpleMessageFDS("b.proto", "pkg", "Root", "value"),
		)
		_, err := planPullRemoteProto(nil, remote, false, []string{"pkg.Root"})
		if err == nil || !strings.Contains(err.Error(), `type "pkg.Root" is declared in multiple files`) {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("duplicate remote file identity is rejected", func(t *testing.T) {
		t.Parallel()
		first := simpleMessageFDS("shared.proto", "pkg", "Root", "old")
		second := simpleMessageFDS("shared.proto", "pkg", "Other", "new")
		remote := &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{first.File[0], second.File[0]}}
		_, err := planPullRemoteProto(nil, remote, false, []string{"pkg.Root"})
		if err == nil || !strings.Contains(err.Error(), `file "shared.proto" is declared more than once`) {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("same full name in different files is a conflict", func(t *testing.T) {
		t.Parallel()
		local := simpleMessageFDS("local.proto", "pkg", "Root", "value")
		remote := simpleMessageFDS("remote.proto", "pkg", "Root", "value")
		_, err := planPullRemoteProto(local, remote, false, []string{"pkg.Root"})
		if err == nil || !strings.Contains(err.Error(), `local file "local.proto"`) || !strings.Contains(err.Error(), `remote file "remote.proto"`) {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("nested selection still loads the containing file", func(t *testing.T) {
		t.Parallel()
		compiled := compiledNestedFDS(t)
		planned, err := planPullRemoteProto(nil, compiled, false, []string{"graph.Root.Payload"})
		if err != nil {
			t.Fatal(err)
		}
		if len(planned.selected) != 1 || planned.selected[0].FullName != "graph.Root.Payload" {
			t.Fatalf("selected=%v", infos(planned.selected))
		}
		requireUsableDescriptor(t, planned.candidate)
		if _, err := requireUsableDescriptor(t, planned.candidate).FindDescriptorByName("graph.Root"); err != nil {
			t.Fatal(err)
		}
	})
}

func TestPullRemoteProtoExecuteFreshCacheBatchAndDetached(t *testing.T) {
	t.Parallel()

	t.Run("installs selected rows and clears file paths on change", func(t *testing.T) {
		t.Parallel()
		compiled := compiledRootDepFDS(t)
		session, server := newProtoBundleAdminSession(t, &databasepb.GetDatabaseDdlResponse{ProtoDescriptors: mustMarshalFDS(t, compiled)})
		if err := session.systemVariables.SetFromSimple("PROTO_DESCRIPTORS_FILE_PATH", "testdata/protos/order_descriptors.pb"); err != nil {
			t.Fatal(err)
		}
		beforeGraph, beforeFiles := cloneDescriptorState(session.systemVariables)
		if len(beforeFiles) == 0 {
			t.Fatal("expected file provenance")
		}
		result, err := executePull(t, session, "PULL REMOTE PROTO graph.Root")
		if err != nil {
			t.Fatal(err)
		}
		if !result.KeepVariables || result.AffectedRows != 1 {
			t.Fatalf("result=%+v", result)
		}
		if got := pullDisplayRows(t, session, result); len(got) != 1 || got[0][0] != "graph.Root" || got[0][1] != "PROTO" {
			t.Fatalf("rows=%v", got)
		}
		if len(session.systemVariables.Internal.ProtoDescriptorFile) != 0 {
			t.Fatalf("file paths survived a changing pull: %v", session.systemVariables.Internal.ProtoDescriptorFile)
		}
		requireUsableDescriptor(t, session.systemVariables.Internal.ProtoDescriptor)
		if proto.Equal(beforeGraph, session.systemVariables.Internal.ProtoDescriptor) {
			t.Fatal("local graph did not change")
		}
		if server.getDDL != 1 || len(server.reqs) != 0 {
			t.Fatalf("admin getDDL=%d updates=%d", server.getDDL, len(server.reqs))
		}
	})

	t.Run("no-op preserves file paths", func(t *testing.T) {
		t.Parallel()
		fds := simpleMessageFDS("shared.proto", "pkg", "Root", "value")
		session, _ := newProtoBundleAdminSession(t, &databasepb.GetDatabaseDdlResponse{ProtoDescriptors: mustMarshalFDS(t, fds)})
		session.systemVariables.Internal.ProtoDescriptor = proto.Clone(fds).(*descriptorpb.FileDescriptorSet)
		session.systemVariables.Internal.ProtoDescriptorFile = []string{"keep.pb"}
		if _, err := executePull(t, session, "PULL REMOTE PROTO pkg.Root"); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(session.systemVariables.Internal.ProtoDescriptorFile, []string{"keep.pb"}) {
			t.Fatalf("paths=%v", session.systemVariables.Internal.ProtoDescriptorFile)
		}
	})

	t.Run("ALL empty remote is a no-op", func(t *testing.T) {
		t.Parallel()
		session, _ := newProtoBundleAdminSession(t, &databasepb.GetDatabaseDdlResponse{})
		if err := session.systemVariables.SetFromSimple("PROTO_DESCRIPTORS_FILE_PATH", "testdata/protos/order_descriptors.pb"); err != nil {
			t.Fatal(err)
		}
		beforeGraph, beforeFiles := cloneDescriptorState(session.systemVariables)
		result, err := executePull(t, session, "PULL REMOTE PROTO ALL")
		if err != nil {
			t.Fatal(err)
		}
		if result.AffectedRows != 0 {
			t.Fatalf("affected=%d", result.AffectedRows)
		}
		assertDescriptorState(t, session.systemVariables, beforeGraph, beforeFiles)
	})

	t.Run("fresh Admin read ignores the 30s cache", func(t *testing.T) {
		t.Parallel()
		first := simpleMessageFDS("shared.proto", "pkg", "Root", "old")
		second := simpleMessageFDS("shared.proto", "pkg", "Root", "new")
		second.File[0].MessageType[0].Field = append(second.File[0].MessageType[0].Field, &descriptorpb.FieldDescriptorProto{
			Name: proto.String("extra"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
		})
		session, server := newProtoBundleAdminSession(t, &databasepb.GetDatabaseDdlResponse{ProtoDescriptors: mustMarshalFDS(t, first)})
		if _, err := session.GetDatabaseDdlCached(t.Context()); err != nil {
			t.Fatal(err)
		}
		server.mu.Lock()
		server.schema = &databasepb.GetDatabaseDdlResponse{ProtoDescriptors: mustMarshalFDS(t, second)}
		server.mu.Unlock()
		if _, err := executePull(t, session, "PULL REMOTE PROTO pkg.Root"); err != nil {
			t.Fatal(err)
		}
		got := session.systemVariables.Internal.ProtoDescriptor
		if len(got.GetFile()) != 1 || len(got.File[0].MessageType[0].Field) != 2 {
			t.Fatalf("PULL used the cached first response: %v", got)
		}
	})

	t.Run("manual batch rejects without mutation or Admin write", func(t *testing.T) {
		t.Parallel()
		fds := simpleMessageFDS("shared.proto", "pkg", "Root", "value")
		session, server := newProtoBundleAdminSession(t, &databasepb.GetDatabaseDdlResponse{ProtoDescriptors: mustMarshalFDS(t, fds)})
		if err := session.systemVariables.SetFromSimple("PROTO_DESCRIPTORS_FILE_PATH", "testdata/protos/order_descriptors.pb"); err != nil {
			t.Fatal(err)
		}
		beforeGraph, beforeFiles := cloneDescriptorState(session.systemVariables)
		if err := session.batch.Start(batchModeDDL); err != nil {
			t.Fatal(err)
		}
		_, err := executePull(t, session, "PULL REMOTE PROTO pkg.Root")
		if !errors.Is(err, errPullRemoteProtoBatchActive) {
			t.Fatalf("err=%v", err)
		}
		assertDescriptorState(t, session.systemVariables, beforeGraph, beforeFiles)
		if server.getDDL != 0 || len(server.reqs) != 0 {
			t.Fatalf("admin leaked getDDL=%d updates=%d", server.getDDL, len(server.reqs))
		}
	})

	t.Run("detached rejects without mutation", func(t *testing.T) {
		t.Parallel()
		session := newSessionForLocalVarTest(t)
		if err := session.systemVariables.SetFromSimple("PROTO_DESCRIPTORS_FILE_PATH", "testdata/protos/order_descriptors.pb"); err != nil {
			t.Fatal(err)
		}
		beforeGraph, beforeFiles := cloneDescriptorState(session.systemVariables)
		session.mode = Detached
		stmt, err := BuildStatement("PULL REMOTE PROTO ALL")
		if err != nil {
			t.Fatal(err)
		}
		_, err = session.ExecuteStatement(t.Context(), stmt)
		if err == nil || !strings.Contains(err.Error(), "no database selected; use USE <database>;") {
			t.Fatalf("err=%v", err)
		}
		assertDescriptorState(t, session.systemVariables, beforeGraph, beforeFiles)
	})

	t.Run("failed listed pull does not submit DDL", func(t *testing.T) {
		t.Parallel()
		session, server := newProtoBundleAdminSession(t, &databasepb.GetDatabaseDdlResponse{})
		if err := session.systemVariables.SetFromSimple("PROTO_DESCRIPTORS_FILE_PATH", "testdata/protos/order_descriptors.pb"); err != nil {
			t.Fatal(err)
		}
		beforeGraph, beforeFiles := cloneDescriptorState(session.systemVariables)
		_, err := executePull(t, session, "PULL REMOTE PROTO examples.shipping.Order")
		if err == nil || !strings.Contains(err.Error(), "unknown type") {
			t.Fatalf("err=%v", err)
		}
		assertDescriptorState(t, session.systemVariables, beforeGraph, beforeFiles)
		if len(server.reqs) != 0 {
			t.Fatalf("DDL submitted: %v", server.reqs)
		}
	})
}

func TestPullRemoteProtoDoesNotDetermineTransaction(t *testing.T) {
	t.Parallel()
	stmt, err := BuildStatement("PULL REMOTE PROTO ALL")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := stmt.(MutationStatement); ok {
		t.Fatal("PULL must not determine a pending Spanner transaction")
	}
	if _, ok := stmt.(nonTransactionalMutationStatement); ok {
		t.Fatal("PULL is a local graph write, not a database mutation")
	}
	if _, ok := stmt.(DetachedCompatible); ok {
		t.Fatal("PULL must not run in detached mode")
	}
}

func TestPullRemoteProtoOutputFailurePreservesDescriptors(t *testing.T) {
	t.Parallel()
	for _, mode := range []enums.DisplayMode{enums.DisplayModeCSV, enums.DisplayModeJSONL} {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()
			fds := simpleMessageFDS("shared.proto", "pkg", "Root", "value")
			session, server := newProtoBundleAdminSession(t, &databasepb.GetDatabaseDdlResponse{ProtoDescriptors: mustMarshalFDS(t, fds)})
			if err := session.systemVariables.SetFromSimple("PROTO_DESCRIPTORS_FILE_PATH", "testdata/protos/order_descriptors.pb"); err != nil {
				t.Fatal(err)
			}
			session.systemVariables.Display.CLIFormat = mode
			beforeGraph, beforeFiles := cloneDescriptorState(session.systemVariables)
			if beforeGraph == nil || len(beforeFiles) == 0 {
				t.Fatal("expected local graph and file provenance")
			}

			stmt, err := BuildStatement("PULL REMOTE PROTO ALL")
			if err != nil {
				t.Fatal(err)
			}
			writer := &errCauseWriter{err: errPullRemoteProtoOutput}
			_, err = session.ExecuteStatementWithOutput(t.Context(), stmt, OperationOutput{w: writer})
			if !errors.Is(err, errPullRemoteProtoOutput) {
				t.Fatalf("err=%v, want cause %v", err, errPullRemoteProtoOutput)
			}
			assertDescriptorState(t, session.systemVariables, beforeGraph, beforeFiles)
			if len(server.reqs) != 0 {
				t.Fatalf("DDL submitted: %v", server.reqs)
			}
		})
	}

	t.Run("CSV success still installs", func(t *testing.T) {
		t.Parallel()
		fds := simpleMessageFDS("shared.proto", "pkg", "Root", "value")
		session, server := newProtoBundleAdminSession(t, &databasepb.GetDatabaseDdlResponse{ProtoDescriptors: mustMarshalFDS(t, fds)})
		if err := session.systemVariables.SetFromSimple("PROTO_DESCRIPTORS_FILE_PATH", "testdata/protos/order_descriptors.pb"); err != nil {
			t.Fatal(err)
		}
		session.systemVariables.Display.CLIFormat = enums.DisplayModeCSV
		beforeGraph, _ := cloneDescriptorState(session.systemVariables)
		stmt, err := BuildStatement("PULL REMOTE PROTO ALL")
		if err != nil {
			t.Fatal(err)
		}
		result, err := session.ExecuteStatementWithOutput(t.Context(), stmt, OperationOutput{w: io.Discard})
		if err != nil {
			t.Fatal(err)
		}
		if !result.KeepVariables || result.AffectedRows != 1 {
			t.Fatalf("result=%+v", result)
		}
		if len(session.systemVariables.Internal.ProtoDescriptorFile) != 0 {
			t.Fatalf("file paths survived a successful stream: %v", session.systemVariables.Internal.ProtoDescriptorFile)
		}
		if proto.Equal(beforeGraph, session.systemVariables.Internal.ProtoDescriptor) {
			t.Fatal("successful CSV pull did not change the local graph")
		}
		if _, err := requireUsableDescriptor(t, session.systemVariables.Internal.ProtoDescriptor).FindDescriptorByName("pkg.Root"); err != nil {
			t.Fatal(err)
		}
		if len(server.reqs) != 0 {
			t.Fatalf("DDL submitted: %v", server.reqs)
		}
	})
}

var errPullRemoteProtoOutput = errors.New("pull remote proto output failure")

type errCauseWriter struct {
	err error
}

func (w *errCauseWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func executePull(t *testing.T, session *Session, sql string) (*Result, error) {
	t.Helper()
	stmt, err := BuildStatement(sql)
	if err != nil {
		t.Fatal(err)
	}
	return session.ExecuteStatement(t.Context(), stmt)
}

func pullDisplayRows(t *testing.T, session *Session, result *Result) [][]string {
	t.Helper()
	rows, err := deriveDisplayRows(session.systemVariables, result.typedPayload())
	if err != nil {
		t.Fatal(err)
	}
	out := make([][]string, len(rows))
	for i, row := range rows {
		out[i] = make([]string, len(row))
		for j, cell := range row {
			out[i][j] = cell.RawText()
		}
	}
	return out
}

func mustMarshalFDS(t *testing.T, fds *descriptorpb.FileDescriptorSet) []byte {
	t.Helper()
	raw, err := proto.Marshal(fds)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func simpleMessageFDS(file, pkg, message, field string) *descriptorpb.FileDescriptorSet {
	return &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{{
		Name:    proto.String(file),
		Package: proto.String(pkg),
		Syntax:  proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: proto.String(message),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name: proto.String(field), Number: proto.Int32(1),
				Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
			}},
		}},
	}}}
}

func compiledRootDepFDS(t *testing.T) *descriptorpb.FileDescriptorSet {
	t.Helper()
	dir := t.TempDir()
	dep := writeProtoSource(t, filepath.Join(dir, "dep.proto"), `syntax="proto3"; package graph; message Dep { string value=1; }`)
	root := writeProtoSource(t, filepath.Join(dir, "root.proto"), fmt.Sprintf(`syntax="proto3"; package graph; import %q; message Root { Dep child=1; }`, dep))
	fds, err := readFileDescriptorProtoFromFile(root)
	if err != nil {
		t.Fatal(err)
	}
	return fds
}

func compiledNestedFDS(t *testing.T) *descriptorpb.FileDescriptorSet {
	t.Helper()
	dir := t.TempDir()
	root := writeProtoSource(t, filepath.Join(dir, "root.proto"), `syntax="proto3"; package graph; message Root { message Payload { string note=1; } Payload payload=1; }`)
	fds, err := readFileDescriptorProtoFromFile(root)
	if err != nil {
		t.Fatal(err)
	}
	return fds
}

func rootOnlyRemote(compiled *descriptorpb.FileDescriptorSet) *descriptorpb.FileDescriptorSet {
	for _, file := range compiled.GetFile() {
		if strings.HasSuffix(file.GetName(), "root.proto") || file.GetName() == "root.proto" {
			return &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{proto.Clone(file).(*descriptorpb.FileDescriptorProto)}}
		}
	}
	return &descriptorpb.FileDescriptorSet{}
}

func fileByName(t *testing.T, fds *descriptorpb.FileDescriptorSet, name string) *descriptorpb.FileDescriptorProto {
	t.Helper()
	for _, file := range fds.GetFile() {
		if file.GetName() == name {
			return proto.Clone(file).(*descriptorpb.FileDescriptorProto)
		}
	}
	t.Fatalf("file %q not found", name)
	return nil
}

func fileByNameOK(fds *descriptorpb.FileDescriptorSet, name string) bool {
	for _, file := range fds.GetFile() {
		if file.GetName() == name {
			return true
		}
	}
	return false
}

func infos(rows []*descriptorInfo) []string {
	out := make([]string, len(rows))
	for i, row := range rows {
		out[i] = row.FullName
	}
	return out
}

func TestPlanPullRemoteProtoUsableGraph(t *testing.T) {
	t.Parallel()
	compiled := compiledRootDepFDS(t)
	planned, err := planPullRemoteProto(nil, compiled, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decoder.FormatConfigWithProto(planned.candidate, false); err != nil {
		t.Fatalf("decoder rejected pulled graph: %v", err)
	}
	if _, err := protodesc.NewFiles(planned.candidate); err != nil {
		t.Fatal(err)
	}
}
