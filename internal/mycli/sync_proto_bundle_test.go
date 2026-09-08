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
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
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
			got := composeProtoBundleDDLs(tt.fds, syncStmt.UpsertPaths, syncStmt.DeletePaths)
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
