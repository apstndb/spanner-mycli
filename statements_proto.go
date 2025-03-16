package main

import (
	"context"
	"fmt"
	"iter"
	"maps"
	"slices"

	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/apstndb/lox"
	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/ast"
	"github.com/cloudspannerecosystem/memefish/token"
	"github.com/samber/lo"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"spheric.cloud/xiter"
)

type SyncProtoStatement struct {
	UpsertPaths []string
	DeletePaths []string
}

func (s *SyncProtoStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	_, fds, err := session.GetDatabaseSchema(ctx)
	if err != nil {
		return nil, err

	}

	return bufferOrExecuteDdlStatements(ctx, session, composeProtoBundleDDLs(fds, s.UpsertPaths, s.DeletePaths))
}

func composeProtoBundleDDLs(fds *descriptorpb.FileDescriptorSet, upsertPaths, deletePaths []string) []string {
	fullNameSetFds := maps.Collect(
		xiter.MapLift(fdsToInfoSeq(fds), func(info *descriptorInfo) (string, struct{}) {
			return info.FullName, struct{}{}
		}),
	)

	upsertExists, upsertNotExists := splitExistence(fullNameSetFds, upsertPaths)
	deleteExists, _ := splitExistence(fullNameSetFds, deletePaths)

	ddl := lo.Ternary(len(fds.GetFile()) == 0,
		lox.IfOrEmpty[ast.DDL](len(upsertNotExists) > 0,
			&ast.CreateProtoBundle{
				Types: &ast.ProtoBundleTypes{Types: toNamedTypes(upsertNotExists)},
			}),
		lo.If[ast.DDL](len(upsertNotExists) == 0 && len(upsertExists) == 0 && len(deleteExists) == len(fullNameSetFds),
			&ast.DropProtoBundle{}).
			ElseIf(len(upsertNotExists) > 0 || len(upsertExists) > 0 || len(deleteExists) > 0,
				&ast.AlterProtoBundle{
					Insert: lox.IfOrEmpty(len(upsertNotExists) > 0,
						&ast.AlterProtoBundleInsert{Types: &ast.ProtoBundleTypes{Types: toNamedTypes(upsertNotExists)}}),
					Update: lox.IfOrEmpty(len(upsertExists) > 0,
						&ast.AlterProtoBundleUpdate{Types: &ast.ProtoBundleTypes{Types: toNamedTypes(upsertExists)}}),
					Delete: lox.IfOrEmpty(len(deleteExists) > 0,
						&ast.AlterProtoBundleDelete{Types: &ast.ProtoBundleTypes{Types: toNamedTypes(deleteExists)}}),
				}).
			Else(nil),
	)

	if ddl == nil {
		return nil
	}

	return sliceOf(ddl.SQL())
}

func parseSyncProtoBundle(s string) (Statement, error) {
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
			paths, err := parsePaths(p)
			if err != nil {
				return nil, fmt.Errorf("failed to parsePaths: %w", err)
			}
			upsertPaths = append(upsertPaths, paths...)
		case p.Token.IsKeywordLike("DELETE"):
			paths, err := parsePaths(p)
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

type ShowLocalProtoStatement struct{}

func (s *ShowLocalProtoStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	fds := session.systemVariables.ProtoDescriptor

	rows := slices.Collect(
		xiter.Map(
			xiter.Flatmap(slices.Values(fds.GetFile()), fdpToInfo),
			func(info *descriptorInfo) Row {
				return toRow(info.FullName, info.Kind, info.Package, info.FileName)
			},
		),
	)

	return &Result{
		ColumnNames:   []string{"full_name", "kind", "package", "file"},
		Rows:          rows,
		AffectedRows:  len(rows),
		KeepVariables: true,
	}, nil
}

type ShowRemoteProtoStatement struct{}

func (s *ShowRemoteProtoStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	resp, err := session.adminClient.GetDatabaseDdl(ctx, &databasepb.GetDatabaseDdlRequest{
		Database: session.DatabasePath(),
	})
	if err != nil {
		return nil, err
	}

	var fds descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(resp.GetProtoDescriptors(), &fds); err != nil {
		return nil, err
	}

	rows := slices.Collect(
		xiter.Map(
			xiter.Flatmap(slices.Values(fds.GetFile()), fdpToInfo),
			func(info *descriptorInfo) Row {
				return toRow(info.FullName, info.Kind, info.Package)
			},
		),
	)

	return &Result{
		ColumnNames:   []string{"full_name", "kind", "package"},
		Rows:          rows,
		AffectedRows:  len(rows),
		KeepVariables: true,
	}, nil
}

// Helper functions

func fdsToInfoSeq(fds *descriptorpb.FileDescriptorSet) iter.Seq[*descriptorInfo] {
	return xiter.Flatmap(slices.Values(fds.GetFile()), fdpToInfo)
}

func splitExistence(fullNameSet map[string]struct{}, paths []string) ([]string, []string) {
	grouped := lo.GroupBy(paths, hasKey(fullNameSet))
	return grouped[true], grouped[false]
}

func parsePaths(p *memefish.Parser) ([]string, error) {
	expr, err := p.ParseExpr()
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
		names, err := xiter.TryCollect(xiter.MapErr(
			slices.Values(e.Values),
			exprToFullName))
		if err != nil {
			return nil, err
		}

		return names, err
	default:
		return nil, fmt.Errorf("must be paren expr or tuple of path, but: %T", expr)
	}
}

func hasKey[K comparable, V any, M map[K]V](m M) func(key K) bool {
	return func(key K) bool {
		_, ok := m[key]
		return ok
	}
}
