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
	"context"
	"errors"
	"net"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/longrunning/autogen/longrunningpb"
	adminapi "cloud.google.com/go/spanner/admin/database/apiv1"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/option"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/apstndb/spanner-mycli/internal/proto/zetasql"
)

func TestSyncProtoBundleParserToComposer(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		sql  string
		fds  *descriptorpb.FileDescriptorSet
		want []string
	}{
		{
			sql:  "SYNC PROTO BUNDLE UPSERT (pkg.New, pkg.Old) DELETE (pkg.Keep)",
			fds:  twoTypeFds,
			want: sliceOf("ALTER PROTO BUNDLE INSERT (pkg.`New`) UPDATE (pkg.Old) DELETE (pkg.Keep)"),
		},
		{
			sql:  "SYNC PROTO BUNDLE DELETE (pkg.Keep) UPSERT (pkg.New, pkg.Old)",
			fds:  twoTypeFds,
			want: sliceOf("ALTER PROTO BUNDLE INSERT (pkg.`New`) UPDATE (pkg.Old) DELETE (pkg.Keep)"),
		},
		{
			sql:  "SYNC PROTO BUNDLE UPSERT (pkg.New)",
			fds:  &descriptorpb.FileDescriptorSet{},
			want: sliceOf("CREATE PROTO BUNDLE (pkg.`New`)"),
		},
		{
			sql:  "SYNC PROTO BUNDLE DELETE (pkg.Unknown)",
			fds:  twoTypeFds,
			want: nil,
		},
		{
			sql:  "SYNC PROTO BUNDLE DELETE (pkg.Old, pkg.Old)",
			fds:  twoTypeFds,
			want: sliceOf("ALTER PROTO BUNDLE DELETE (pkg.Old)"),
		},
		{
			sql:  "SYNC PROTO BUNDLE DELETE (pkg.Old, pkg.Old)",
			fds:  oneTypeFds,
			want: sliceOf("DROP PROTO BUNDLE"),
		},
	} {
		t.Run(tt.sql, func(t *testing.T) {
			t.Parallel()
			stmt, err := BuildStatement(tt.sql)
			if err != nil {
				t.Fatalf("BuildStatement(%q) error = %v", tt.sql, err)
			}
			syncStmt, ok := stmt.(*SyncProtoStatement)
			if !ok {
				t.Fatalf("BuildStatement(%q) = %T", tt.sql, stmt)
			}
			upsert, del, err := syncStmt.resolvedPaths(nil)
			if err != nil {
				t.Fatalf("resolvedPaths: %v", err)
			}
			got := composeProtoBundleDDLs(tt.fds, upsert, del)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("compose mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSyncProtoBundleLexicalAndIdentity(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		sql  string
		want *SyncProtoStatement
	}{
		{
			name: "comment with parentheses and DELETE keyword",
			sql:  "SYNC PROTO BUNDLE UPSERT /* DELETE (examples.Hidden) */ (examples.A) DELETE (examples.B)",
			want: &SyncProtoStatement{UpsertPaths: sliceOf("examples.A"), DeletePaths: sliceOf("examples.B")},
		},
		{
			name: "quoted ident containing parentheses and DELETE",
			sql:  "SYNC PROTO BUNDLE UPSERT (`foo) DELETE (`)",
			want: &SyncProtoStatement{UpsertPaths: sliceOf("foo) DELETE (")},
		},
		{
			name: "repeated DELETE then interleaved UPSERT keeps first-occurrence order",
			sql:  "SYNC PROTO BUNDLE DELETE (examples.A) UPSERT (examples.B) DELETE (examples.A, examples.C)",
			want: &SyncProtoStatement{UpsertPaths: sliceOf("examples.B"), DeletePaths: sliceOf("examples.A", "examples.C")},
		},
		{
			name: "repeated same-kind DELETE clauses unique in first-occurrence order",
			sql:  "SYNC PROTO BUNDLE DELETE (examples.A, examples.B) DELETE (examples.B, examples.C)",
			want: &SyncProtoStatement{DeletePaths: sliceOf("examples.A", "examples.B", "examples.C")},
		},
		{
			name: "RECURSIVE UPSERT listed paths stay the requested roots",
			sql:  "SYNC PROTO BUNDLE RECURSIVE UPSERT (examples.shipping.Order)",
			want: &SyncProtoStatement{UpsertPaths: sliceOf("examples.shipping.Order")},
		},
		{
			name: "RECURSIVE modifies only the following UPSERT",
			sql:  "SYNC PROTO BUNDLE RECURSIVE UPSERT (examples.shipping.Order) UPSERT (examples.shipping.OrderHistory) DELETE (examples.Gone)",
			want: &SyncProtoStatement{
				UpsertPaths: sliceOf("examples.shipping.Order", "examples.shipping.OrderHistory"),
				DeletePaths: sliceOf("examples.Gone"),
			},
		},
		{
			name: "plain UPSERT then RECURSIVE UPSERT then DELETE",
			sql:  "SYNC PROTO BUNDLE UPSERT (examples.shipping.Order.Item) RECURSIVE UPSERT (examples.shipping.Order) DELETE (examples.Gone)",
			want: &SyncProtoStatement{
				UpsertPaths: sliceOf("examples.shipping.Order.Item", "examples.shipping.Order"),
				DeletePaths: sliceOf("examples.Gone"),
			},
		},
		{
			name: "comment between RECURSIVE and UPSERT",
			sql:  "SYNC PROTO BUNDLE RECURSIVE /* DELETE (examples.Hidden) */ UPSERT (examples.A)",
			want: &SyncProtoStatement{UpsertPaths: sliceOf("examples.A")},
		},
		{
			name: "quoted ident containing parentheses after RECURSIVE UPSERT",
			sql:  "SYNC PROTO BUNDLE RECURSIVE UPSERT (`foo) DELETE (`)",
			want: &SyncProtoStatement{UpsertPaths: sliceOf("foo) DELETE (")},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := BuildStatement(tt.sql)
			if err != nil {
				t.Fatalf("BuildStatement(%q) error = %v", tt.sql, err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("BuildStatement(%q) mismatch (-want +got):\n%s", tt.sql, diff)
			}
		})
	}
}

func TestSyncProtoBundleDecodedOverlapAndMalformed(t *testing.T) {
	t.Parallel()
	t.Run("equivalent decoded spellings conflict", func(t *testing.T) {
		t.Parallel()
		sql := "SYNC PROTO BUNDLE UPSERT (examples.`Type`) DELETE (`examples.Type`)"
		got, err := BuildStatement(sql)
		if err == nil || got != nil {
			t.Fatalf("BuildStatement(%q) = %#v, %v; want error and nil statement", sql, got, err)
		}
		if !strings.Contains(err.Error(), "appears in both UPSERT and DELETE") {
			t.Fatalf("error = %v, want overlap conflict", err)
		}
	})
	t.Run("malformed later clause returns nil statement", func(t *testing.T) {
		t.Parallel()
		sql := "SYNC PROTO BUNDLE UPSERT (examples.A) DELETE ("
		got, err := BuildStatement(sql)
		if err == nil || got != nil {
			t.Fatalf("BuildStatement(%q) = %#v, %v; want error and nil statement", sql, got, err)
		}
	})
}

func TestSyncProtoBundleOverlapExecuteRejects(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	_, err := session.ExecuteStatement(t.Context(), &SyncProtoStatement{
		UpsertPaths: sliceOf("pkg.Old"),
		DeletePaths: sliceOf("pkg.Old"),
	})
	if err == nil || !strings.Contains(err.Error(), "appears in both UPSERT and DELETE") {
		t.Fatalf("overlap Execute error = %v", err)
	}
}

func TestSyncProtoBundleManualDDLBatch(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	desc, err := proto.Marshal(twoTypeFds)
	if err != nil {
		t.Fatal(err)
	}
	session.ddlCache.response = &databasepb.GetDatabaseDdlResponse{ProtoDescriptors: desc}
	session.ddlCache.fetchedAt = time.Now()
	session.ddlCache.schemaGeneration = session.SchemaGeneration()

	if err := session.batch.Start(batchModeDDL); err != nil {
		t.Fatalf("batch.Start: %v", err)
	}
	stmt, err := BuildStatement("SYNC PROTO BUNDLE UPSERT (pkg.New) DELETE (pkg.Old)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(t.Context(), stmt); err != nil {
		t.Fatalf("ExecuteStatement: %v", err)
	}
	bulk, ok := session.batch.Current().(*BulkDdlStatement)
	if !ok {
		t.Fatalf("batch.Current() = %T", session.batch.Current())
	}
	want := sliceOf("ALTER PROTO BUNDLE INSERT (pkg.`New`) DELETE (pkg.Old)")
	if diff := cmp.Diff(want, bulk.Ddls); diff != "" {
		t.Errorf("buffered DDLs mismatch (-want +got):\n%s", diff)
	}
}

func TestSyncProtoBundleRecursiveParserClauses(t *testing.T) {
	t.Parallel()
	stmt, err := BuildStatement("SYNC PROTO BUNDLE RECURSIVE UPSERT (examples.A) UPSERT (examples.B) DELETE (examples.C) RECURSIVE UPSERT (examples.D)")
	if err != nil {
		t.Fatal(err)
	}
	got := stmt.(*SyncProtoStatement)
	want := []syncProtoClause{
		{recursive: true, paths: sliceOf("examples.A")},
		{paths: sliceOf("examples.B")},
		{delete: true, paths: sliceOf("examples.C")},
		{recursive: true, paths: sliceOf("examples.D")},
	}
	if diff := cmp.Diff(want, got.clauses, cmp.AllowUnexported(syncProtoClause{})); diff != "" {
		t.Errorf("clauses mismatch (-want +got):\n%s", diff)
	}
}

func TestSyncProtoBundleRecursiveParserRejects(t *testing.T) {
	t.Parallel()
	for _, sql := range []string{
		"SYNC PROTO BUNDLE RECURSIVE DELETE (examples.A)",
		"SYNC PROTO BUNDLE RECURSIVE",
		"SYNC PROTO BUNDLE UPSERT (examples.A) RECURSIVE",
		"SYNC PROTO BUNDLE RECURSIVE RECURSIVE UPSERT (examples.A)",
		"SYNC PROTO BUNDLE RECURSIVE leftover",
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

func lexicalSelectionFDS() *descriptorpb.FileDescriptorSet {
	return &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{
			{
				Name:    proto.String("money.proto"),
				Package: proto.String("other"),
				Syntax:  proto.String("proto3"),
				MessageType: []*descriptorpb.DescriptorProto{
					{Name: proto.String("Money")},
				},
			},
			{
				Name:    proto.String("order.proto"),
				Package: proto.String("examples.shipping"),
				Syntax:  proto.String("proto3"),
				MessageType: []*descriptorpb.DescriptorProto{
					{
						Name: proto.String("Order"),
						Field: []*descriptorpb.FieldDescriptorProto{{
							Name:     proto.String("fee"),
							Number:   proto.Int32(1),
							Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
							TypeName: proto.String(".other.Money"),
						}},
						NestedType: []*descriptorpb.DescriptorProto{
							{Name: proto.String("Address")},
							{Name: proto.String("Item")},
							{
								Name: proto.String("Unused"),
							},
							{
								Name:    proto.String("AttrEntry"),
								Options: &descriptorpb.MessageOptions{MapEntry: proto.Bool(true)},
							},
							placeholderDescriptor("Ghost"),
						},
						EnumType: []*descriptorpb.EnumDescriptorProto{
							{Name: proto.String("Priority")},
						},
					},
					{Name: proto.String("OrderHistory")},
				},
				EnumType: []*descriptorpb.EnumDescriptorProto{
					{Name: proto.String("ShipStatus")},
				},
			},
		},
	}
}

func placeholderDescriptor(name string) *descriptorpb.DescriptorProto {
	opts := &descriptorpb.MessageOptions{}
	proto.SetExtension(opts, zetasql.E_PlaceholderDescriptorProto_PlaceholderDescriptor, &zetasql.PlaceholderDescriptorProto{
		IsPlaceholder: proto.Bool(true),
	})
	return &descriptorpb.DescriptorProto{Name: proto.String(name), Options: opts}
}

func TestExpandRecursiveProtoNames(t *testing.T) {
	t.Parallel()
	fds := lexicalSelectionFDS()
	for _, tt := range []struct {
		name    string
		root    string
		want    []string
		wantErr string
	}{
		{
			name: "nested messages enums skip map-entry and placeholder",
			root: "examples.shipping.Order",
			want: sliceOf(
				"examples.shipping.Order",
				"examples.shipping.Order.Address",
				"examples.shipping.Order.Item",
				"examples.shipping.Order.Unused",
				"examples.shipping.Order.Priority",
			),
		},
		{
			name: "enum root is itself",
			root: "examples.shipping.ShipStatus",
			want: sliceOf("examples.shipping.ShipStatus"),
		},
		{
			name: "nested enum root is itself",
			root: "examples.shipping.Order.Priority",
			want: sliceOf("examples.shipping.Order.Priority"),
		},
		{
			name:    "missing root",
			root:    "examples.shipping.Missing",
			wantErr: `unknown type "examples.shipping.Missing"`,
		},
		{
			name:    "placeholder root",
			root:    "examples.shipping.Order.Ghost",
			wantErr: "placeholder descriptor",
		},
		{
			name:    "map-entry root",
			root:    "examples.shipping.Order.AttrEntry",
			wantErr: "synthetic map entry",
		},
		{
			name:    "referenced sibling is not a root error; field is not selectable",
			root:    "examples.shipping.Order.fee",
			wantErr: "not a message or enum",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := expandRecursiveProtoNames(fds, tt.root)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("expand mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestResolvedPathsFirstOccurrenceAndConflicts(t *testing.T) {
	t.Parallel()
	fds := lexicalSelectionFDS()
	mustParse := func(sql string) *SyncProtoStatement {
		t.Helper()
		stmt, err := BuildStatement(sql)
		if err != nil {
			t.Fatal(err)
		}
		return stmt.(*SyncProtoStatement)
	}

	t.Run("plain UPSERT is not recursive", func(t *testing.T) {
		t.Parallel()
		upsert, del, err := mustParse("SYNC PROTO BUNDLE UPSERT (examples.shipping.Order)").resolvedPaths(fds)
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(sliceOf("examples.shipping.Order"), upsert); diff != "" {
			t.Errorf("upsert (-want +got):\n%s", diff)
		}
		if len(del) != 0 {
			t.Fatalf("delete = %v", del)
		}
	})
	t.Run("similarly prefixed sibling stays out", func(t *testing.T) {
		t.Parallel()
		upsert, _, err := mustParse("SYNC PROTO BUNDLE RECURSIVE UPSERT (examples.shipping.Order)").resolvedPaths(fds)
		if err != nil {
			t.Fatal(err)
		}
		if slices.Contains(upsert, "examples.shipping.OrderHistory") || slices.Contains(upsert, "other.Money") {
			t.Fatalf("closure leaked: %v", upsert)
		}
	})
	t.Run("later recursive appends unseen descendants", func(t *testing.T) {
		t.Parallel()
		upsert, _, err := mustParse("SYNC PROTO BUNDLE UPSERT (examples.shipping.Order) RECURSIVE UPSERT (examples.shipping.Order)").resolvedPaths(fds)
		if err != nil {
			t.Fatal(err)
		}
		want := sliceOf(
			"examples.shipping.Order",
			"examples.shipping.Order.Address",
			"examples.shipping.Order.Item",
			"examples.shipping.Order.Unused",
			"examples.shipping.Order.Priority",
		)
		if diff := cmp.Diff(want, upsert); diff != "" {
			t.Errorf("order (-want +got):\n%s", diff)
		}
	})
	t.Run("overlapping roots first-occurrence", func(t *testing.T) {
		t.Parallel()
		upsert, _, err := mustParse("SYNC PROTO BUNDLE RECURSIVE UPSERT (examples.shipping.Order.Item, examples.shipping.Order)").resolvedPaths(fds)
		if err != nil {
			t.Fatal(err)
		}
		want := sliceOf(
			"examples.shipping.Order.Item",
			"examples.shipping.Order",
			"examples.shipping.Order.Address",
			"examples.shipping.Order.Unused",
			"examples.shipping.Order.Priority",
		)
		if diff := cmp.Diff(want, upsert); diff != "" {
			t.Errorf("order (-want +got):\n%s", diff)
		}
	})
	t.Run("cross-file reference is not selected", func(t *testing.T) {
		t.Parallel()
		upsert, _, err := mustParse("SYNC PROTO BUNDLE RECURSIVE UPSERT (examples.shipping.Order) UPSERT (other.Money)").resolvedPaths(fds)
		if err != nil {
			t.Fatal(err)
		}
		if upsert[len(upsert)-1] != "other.Money" {
			t.Fatalf("plain UPSERT other.Money should stay last: %v", upsert)
		}
		if !slices.Contains(upsert, "examples.shipping.Order.Unused") {
			t.Fatalf("unreferenced nested child missing: %v", upsert)
		}
	})
}

func TestSyncProtoBundleRecursiveManualDDLBatch(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	session.systemVariables.Internal.ProtoDescriptor = orderFds
	desc, err := proto.Marshal(&descriptorpb.FileDescriptorSet{})
	if err != nil {
		t.Fatal(err)
	}
	session.ddlCache.response = &databasepb.GetDatabaseDdlResponse{ProtoDescriptors: desc}
	session.ddlCache.fetchedAt = time.Now()
	session.ddlCache.schemaGeneration = session.SchemaGeneration()

	if err := session.batch.Start(batchModeDDL); err != nil {
		t.Fatalf("batch.Start: %v", err)
	}
	stmt, err := BuildStatement("SYNC PROTO BUNDLE RECURSIVE UPSERT (examples.shipping.Order)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(t.Context(), stmt); err != nil {
		t.Fatalf("ExecuteStatement: %v", err)
	}
	bulk, ok := session.batch.Current().(*BulkDdlStatement)
	if !ok {
		t.Fatalf("batch.Current() = %T", session.batch.Current())
	}
	want := sliceOf("CREATE PROTO BUNDLE (examples.shipping.`Order`, examples.shipping.`Order`.Address, examples.shipping.`Order`.Item)")
	if diff := cmp.Diff(want, bulk.Ddls); diff != "" {
		t.Errorf("buffered DDLs mismatch (-want +got):\n%s", diff)
	}
}

func TestSyncProtoBundleRecursiveFakeAdmin(t *testing.T) {
	t.Parallel()
	local := orderFds
	emptyRemote, err := proto.Marshal(&descriptorpb.FileDescriptorSet{})
	if err != nil {
		t.Fatal(err)
	}
	orderOnlyRemote, err := proto.Marshal(&descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{{
			Name:    proto.String("remote.proto"),
			Package: proto.String("examples.shipping"),
			Syntax:  proto.String("proto3"),
			MessageType: []*descriptorpb.DescriptorProto{
				{Name: proto.String("Order")},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("CREATE from empty remote", func(t *testing.T) {
		t.Parallel()
		session, server := newProtoBundleAdminSession(t, &databasepb.GetDatabaseDdlResponse{ProtoDescriptors: emptyRemote})
		session.systemVariables.Internal.ProtoDescriptor = local
		stmt, err := BuildStatement("SYNC PROTO BUNDLE RECURSIVE UPSERT (examples.shipping.Order)")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := session.ExecuteStatement(t.Context(), stmt); err != nil {
			t.Fatal(err)
		}
		if server.getDDL != 1 || len(server.reqs) != 1 {
			t.Fatalf("getDDL=%d updates=%d", server.getDDL, len(server.reqs))
		}
		want := []string{"CREATE PROTO BUNDLE (examples.shipping.`Order`, examples.shipping.`Order`.Address, examples.shipping.`Order`.Item)"}
		if diff := cmp.Diff(want, server.reqs[0].GetStatements()); diff != "" {
			t.Errorf("DDL (-want +got):\n%s", diff)
		}
	})
	t.Run("ALTER insert nested update root", func(t *testing.T) {
		t.Parallel()
		session, server := newProtoBundleAdminSession(t, &databasepb.GetDatabaseDdlResponse{ProtoDescriptors: orderOnlyRemote})
		session.systemVariables.Internal.ProtoDescriptor = local
		stmt, err := BuildStatement("SYNC PROTO BUNDLE RECURSIVE UPSERT (examples.shipping.Order)")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := session.ExecuteStatement(t.Context(), stmt); err != nil {
			t.Fatal(err)
		}
		want := []string{"ALTER PROTO BUNDLE INSERT (examples.shipping.`Order`.Address, examples.shipping.`Order`.Item) UPDATE (examples.shipping.`Order`)"}
		if diff := cmp.Diff(want, server.reqs[0].GetStatements()); diff != "" {
			t.Errorf("DDL (-want +got):\n%s", diff)
		}
	})
	t.Run("ordinary UPSERT is not expanded", func(t *testing.T) {
		t.Parallel()
		session, server := newProtoBundleAdminSession(t, &databasepb.GetDatabaseDdlResponse{ProtoDescriptors: emptyRemote})
		session.systemVariables.Internal.ProtoDescriptor = local
		stmt, err := BuildStatement("SYNC PROTO BUNDLE UPSERT (examples.shipping.Order)")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := session.ExecuteStatement(t.Context(), stmt); err != nil {
			t.Fatal(err)
		}
		want := []string{"CREATE PROTO BUNDLE (examples.shipping.`Order`)"}
		if diff := cmp.Diff(want, server.reqs[0].GetStatements()); diff != "" {
			t.Errorf("DDL (-want +got):\n%s", diff)
		}
	})
	t.Run("missing root is local and skips Admin", func(t *testing.T) {
		t.Parallel()
		session, server := newProtoBundleAdminSession(t, &databasepb.GetDatabaseDdlResponse{ProtoDescriptors: emptyRemote})
		session.systemVariables.Internal.ProtoDescriptor = local
		stmt, err := BuildStatement("SYNC PROTO BUNDLE RECURSIVE UPSERT (examples.shipping.Missing)")
		if err != nil {
			t.Fatal(err)
		}
		_, err = session.ExecuteStatement(t.Context(), stmt)
		if err == nil || !strings.Contains(err.Error(), `unknown type "examples.shipping.Missing"`) {
			t.Fatalf("err=%v", err)
		}
		if server.getDDL != 0 || len(server.reqs) != 0 {
			t.Fatalf("Admin leaked getDDL=%d updates=%d", server.getDDL, len(server.reqs))
		}
	})
	t.Run("#402 FAIL pending owner skips Update", func(t *testing.T) {
		t.Parallel()
		session, server := newProtoBundleAdminSession(t, &databasepb.GetDatabaseDdlResponse{ProtoDescriptors: emptyRemote})
		session.systemVariables.Internal.ProtoDescriptor = local
		if _, err := session.ExecuteStatement(t.Context(), &BeginStatement{}); err != nil {
			t.Fatal(err)
		}
		stmt, err := BuildStatement("SYNC PROTO BUNDLE RECURSIVE UPSERT (examples.shipping.Order)")
		if err != nil {
			t.Fatal(err)
		}
		_, err = session.ExecuteStatement(t.Context(), stmt)
		if !errors.Is(err, errDDLInTransaction) {
			t.Fatalf("err=%v, want errDDLInTransaction", err)
		}
		if len(server.reqs) != 0 {
			t.Fatalf("UpdateDatabaseDdl leaked: %d", len(server.reqs))
		}
		if !session.txn.InPendingTransaction() {
			t.Fatal("FAIL must leave the pending owner")
		}
	})
	t.Run("expanded DELETE conflict skips Admin", func(t *testing.T) {
		t.Parallel()
		session, server := newProtoBundleAdminSession(t, &databasepb.GetDatabaseDdlResponse{ProtoDescriptors: emptyRemote})
		session.systemVariables.Internal.ProtoDescriptor = local
		stmt, err := BuildStatement("SYNC PROTO BUNDLE RECURSIVE UPSERT (examples.shipping.Order) DELETE (examples.shipping.Order.Item)")
		if err != nil {
			t.Fatal(err)
		}
		_, err = session.ExecuteStatement(t.Context(), stmt)
		if err == nil || !strings.Contains(err.Error(), "appears in both UPSERT and DELETE") {
			t.Fatalf("err=%v", err)
		}
		if server.getDDL != 0 || len(server.reqs) != 0 {
			t.Fatalf("Admin leaked getDDL=%d updates=%d", server.getDDL, len(server.reqs))
		}
	})
}

type protoBundleAdminServer struct {
	databasepb.UnimplementedDatabaseAdminServer
	longrunningpb.UnimplementedOperationsServer

	mu     sync.Mutex
	getDDL int
	reqs   []*databasepb.UpdateDatabaseDdlRequest
	schema *databasepb.GetDatabaseDdlResponse
	ops    map[string]*longrunningpb.Operation
}

func (s *protoBundleAdminServer) GetDatabaseDdl(context.Context, *databasepb.GetDatabaseDdlRequest) (*databasepb.GetDatabaseDdlResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getDDL++
	if s.schema == nil {
		return &databasepb.GetDatabaseDdlResponse{}, nil
	}
	return proto.Clone(s.schema).(*databasepb.GetDatabaseDdlResponse), nil
}

func (s *protoBundleAdminServer) UpdateDatabaseDdl(_ context.Context, req *databasepb.UpdateDatabaseDdlRequest) (*longrunningpb.Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cloned := proto.Clone(req).(*databasepb.UpdateDatabaseDdlRequest)
	s.reqs = append(s.reqs, cloned)
	name := "ops/proto-bundle-" + strings.ReplaceAll(strings.Join(req.GetStatements(), "/"), " ", "_")
	md := &databasepb.UpdateDatabaseDdlMetadata{
		Database:         req.GetDatabase(),
		Statements:       append([]string(nil), req.GetStatements()...),
		CommitTimestamps: []*timestamppb.Timestamp{timestamppb.Now()},
	}
	anyMD, err := anypb.New(md)
	if err != nil {
		return nil, err
	}
	op := &longrunningpb.Operation{
		Name:     name,
		Done:     true,
		Metadata: anyMD,
		Result:   &longrunningpb.Operation_Response{Response: mustEmptyAny()},
	}
	if s.ops == nil {
		s.ops = map[string]*longrunningpb.Operation{}
	}
	s.ops[name] = proto.Clone(op).(*longrunningpb.Operation)
	return op, nil
}

func (s *protoBundleAdminServer) GetOperation(_ context.Context, req *longrunningpb.GetOperationRequest) (*longrunningpb.Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if op := s.ops[req.Name]; op != nil {
		return proto.Clone(op).(*longrunningpb.Operation), nil
	}
	return &longrunningpb.Operation{
		Name:   req.Name,
		Done:   true,
		Result: &longrunningpb.Operation_Error{Error: &statuspb.Status{Message: "unknown op"}},
	}, nil
}

func newProtoBundleAdminSession(t *testing.T, schema *databasepb.GetDatabaseDdlResponse) (*Session, *protoBundleAdminServer) {
	t.Helper()
	server := &protoBundleAdminServer{schema: schema}
	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer()
	databasepb.RegisterDatabaseAdminServer(gs, server)
	longrunningpb.RegisterOperationsServer(gs, server)
	go func() {
		if err := gs.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Errorf("serve: %v", err)
		}
	}()
	t.Cleanup(func() {
		gs.Stop()
		_ = lis.Close()
	})
	conn, err := grpc.NewClient("passthrough:///proto-bundle",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	admin, err := adminapi.NewDatabaseAdminClient(t.Context(), option.WithGRPCConn(conn))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	session := newSessionForLocalVarTest(t)
	session.adminClient = admin
	return session, server
}
