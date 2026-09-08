package mycli

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/ast"
	"github.com/cloudspannerecosystem/memefish/token"
	"github.com/google/go-cmp/cmp"
	"github.com/samber/lo"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// originalParseSyncProtoBundle is a retained snapshot of the Issue 242 baseline
// parser. parsePaths called memefish Parser.ParseExpr, which requires EOF after
// the first list, so mixed clauses failed. Used only as a negative control.
func originalParseSyncProtoBundle(s string) (Statement, error) {
	p := &memefish.Parser{Lexer: &memefish.Lexer{
		File: &token.File{
			Buffer: s,
		},
	}}
	err := p.NextToken()
	if err != nil {
		return nil, err
	}

	var upsertPaths, deletePaths []string
loop:
	for {
		switch {
		case p.Token.Kind == token.TokenEOF:
			break loop
		case p.Token.IsKeywordLike("UPSERT"):
			paths, err := originalParsePaths(p)
			if err != nil {
				return nil, err
			}
			upsertPaths = append(upsertPaths, paths...)
		case p.Token.IsKeywordLike("DELETE"):
			paths, err := originalParsePaths(p)
			if err != nil {
				return nil, err
			}
			deletePaths = append(deletePaths, paths...)
		default:
			return nil, fmt.Errorf("expected UPSERT or DELETE, but: %q", p.Token.AsString)
		}
	}
	return &SyncProtoStatement{UpsertPaths: upsertPaths, DeletePaths: deletePaths}, nil
}

func originalParsePaths(p *memefish.Parser) ([]string, error) {
	expr, err := recoverMemefishParserPanic(p.ParseExpr)
	if err != nil {
		return nil, err
	}

	switch e := expr.(type) {
	case *ast.ParenExpr:
		name, err := exprToFullName(e.Expr)
		if err != nil {
			return nil, err
		}
		return sliceOf(name), nil
	case *ast.TupleStructLiteral:
		return lo.MapErr(e.Values, func(expr ast.Expr, _ int) (string, error) {
			return exprToFullName(expr)
		})
	default:
		return nil, fmt.Errorf("must be paren expr or tuple of path, but: %T", expr)
	}
}

func TestOriginalParseSyncProtoBundleRejectsMixed(t *testing.T) {
	t.Parallel()
	for _, args := range []string{
		"UPSERT (examples.EnumType) DELETE (examples.ProtoType)",
		"DELETE (examples.ProtoType) UPSERT (examples.EnumType)",
	} {
		_, err := originalParseSyncProtoBundle(args)
		if err == nil {
			t.Fatalf("original parser accepted mixed args %q", args)
		}
		if !strings.Contains(err.Error(), "expected token: <eof>") {
			t.Fatalf("original parser error for %q = %v, want leftover EOF", args, err)
		}

		stmt, err := BuildStatement("SYNC PROTO BUNDLE " + args)
		if err != nil {
			t.Fatalf("candidate BuildStatement(%q) error = %v", args, err)
		}
		if _, ok := stmt.(*SyncProtoStatement); !ok {
			t.Fatalf("candidate statement %T, want *SyncProtoStatement", stmt)
		}
	}
}

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
