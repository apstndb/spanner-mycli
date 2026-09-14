package mycli

import (
	"context"
	"fmt"
	"iter"
	"log/slog"
	"slices"
	"strings"

	"github.com/apstndb/spancodec"
	"github.com/bufbuild/protocompile/walk"
	"github.com/cloudspannerecosystem/memefish/ast"
	"github.com/samber/lo"
	loi "github.com/samber/lo/it"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/apstndb/spanner-mycli/internal/proto/zetasql"
)

// descriptorInfo is the row shape for SHOW LOCAL PROTO; SHOW REMOTE PROTO
// uses the same shape with the file column masked out.
type descriptorInfo struct {
	FullName string `spanner:"full_name"`
	Kind     string `spanner:"kind"`
	Package  string `spanner:"package"`
	FileName string `spanner:"file"`
}

var (
	localProtoRowEncoder  = spancodec.MustNewRowEncoder[*descriptorInfo]()
	remoteProtoRowEncoder = spancodec.MustNewRowEncoder[*descriptorInfo](spancodec.WithoutColumns("file"))
)

// syncProtoClause is one parsed UPSERT or DELETE list. recursive is valid
// only on UPSERT and is expanded against the local FileDescriptorSet.
type syncProtoClause struct {
	recursive bool
	delete    bool
	paths     []string
}

type SyncProtoStatement struct {
	UpsertPaths []string
	DeletePaths []string
	// clauses is the ordered per-clause form, including whether each UPSERT
	// is RECURSIVE. Listed UpsertPaths/DeletePaths stay first-occurrence
	// roots for compose and existing constructed statements.
	clauses []syncProtoClause
}

func (SyncProtoStatement) isNonTransactionalMutationStatement() {}

func (s *SyncProtoStatement) Execute(ctx context.Context, session *Session, out OperationOutput) (*Result, error) {
	var local *descriptorpb.FileDescriptorSet
	if session.systemVariables != nil {
		local = session.systemVariables.Internal.ProtoDescriptor
	}
	upsertPaths, deletePaths, err := s.resolvedPaths(local)
	if err != nil {
		return nil, err
	}
	if name, ok := firstSharedFullName(upsertPaths, deletePaths); ok {
		return nil, fmt.Errorf("SYNC PROTO BUNDLE conflict: %q appears in both UPSERT and DELETE", name)
	}

	_, fds, err := session.GetDatabaseSchema(ctx)
	if err != nil {
		return nil, err
	}

	return bufferOrExecuteDdlStatements(ctx, session, composeProtoBundleDDLs(fds, upsertPaths, deletePaths))
}

// resolvedPaths expands RECURSIVE UPSERT clauses against local descriptors.
// Missing, placeholder, and map-entry roots fail here, before Admin reads.
// First-occurrence dedup is per operation; UPSERT/DELETE overlap is left to
// the caller so mixed DELETE conflicts are detected after expansion.
func (s *SyncProtoStatement) resolvedPaths(local *descriptorpb.FileDescriptorSet) ([]string, []string, error) {
	if len(s.clauses) == 0 {
		return s.UpsertPaths, s.DeletePaths, nil
	}

	var upsertPaths, deletePaths []string
	seenUpsert := make(map[string]struct{})
	seenDelete := make(map[string]struct{})
	for _, clause := range s.clauses {
		for _, path := range clause.paths {
			names := []string{path}
			if clause.recursive {
				var err error
				names, err = expandRecursiveProtoNames(local, path)
				if err != nil {
					return nil, nil, err
				}
			}
			target, seen := &upsertPaths, seenUpsert
			if clause.delete {
				target, seen = &deletePaths, seenDelete
			}
			for _, name := range names {
				if _, ok := seen[name]; ok {
					continue
				}
				seen[name] = struct{}{}
				*target = append(*target, name)
			}
		}
	}
	return upsertPaths, deletePaths, nil
}

func expandRecursiveProtoNames(fds *descriptorpb.FileDescriptorSet, root string) ([]string, error) {
	found, ok := lookupLocalDescriptor(fds, root)
	if !ok {
		return nil, fmt.Errorf("SYNC PROTO BUNDLE RECURSIVE UPSERT: unknown type %q", root)
	}
	switch dp := found.(type) {
	case *descriptorpb.EnumDescriptorProto:
		return []string{root}, nil
	case *descriptorpb.DescriptorProto:
		if hasPlaceholderDescriptorProto(dp) {
			return nil, fmt.Errorf("SYNC PROTO BUNDLE RECURSIVE UPSERT: %q is a placeholder descriptor", root)
		}
		if dp.GetOptions().GetMapEntry() {
			return nil, fmt.Errorf("SYNC PROTO BUNDLE RECURSIVE UPSERT: %q is a synthetic map entry", root)
		}
	default:
		return nil, fmt.Errorf("SYNC PROTO BUNDLE RECURSIVE UPSERT: %q is not a message or enum", root)
	}

	var names []string
	prefix := root + "."
	for _, fdp := range fds.GetFile() {
		for name, message := range fdpToSeq(fdp) {
			if name != root && !strings.HasPrefix(name, prefix) {
				continue
			}
			if !selectableProtoDescriptor(message) {
				continue
			}
			names = append(names, name)
		}
	}
	return names, nil
}

func lookupLocalDescriptor(fds *descriptorpb.FileDescriptorSet, fullName string) (proto.Message, bool) {
	for _, fdp := range fds.GetFile() {
		for name, message := range fdpToSeq(fdp) {
			if name == fullName {
				return message, true
			}
		}
	}
	return nil, false
}

func selectableProtoDescriptor(message proto.Message) bool {
	switch m := message.(type) {
	case *descriptorpb.DescriptorProto:
		return !hasPlaceholderDescriptorProto(m) && !m.GetOptions().GetMapEntry()
	case *descriptorpb.EnumDescriptorProto:
		return true
	default:
		return false
	}
}

func firstSharedFullName(upsertPaths, deletePaths []string) (string, bool) {
	inDelete := make(map[string]struct{}, len(deletePaths))
	for _, name := range deletePaths {
		inDelete[name] = struct{}{}
	}
	for _, name := range upsertPaths {
		if _, ok := inDelete[name]; ok {
			return name, true
		}
	}
	return "", false
}

func uniqFullNames(paths []string) []string {
	if len(paths) == 0 {
		return paths
	}
	return lo.Uniq(paths)
}

func composeProtoBundleDDLs(fds *descriptorpb.FileDescriptorSet, upsertPaths, deletePaths []string) []string {
	// Set-wise: first-occurrence order, no duplicate-count DROP/ALTER lists.
	upsertPaths = uniqFullNames(upsertPaths)
	deletePaths = uniqFullNames(deletePaths)

	fullNameSetFds := make(map[string]struct{})
	for info := range fdsToInfoSeq(fds) {
		fullNameSetFds[info.FullName] = struct{}{}
	}

	upsertExists, upsertNotExists := splitExistence(fullNameSetFds, upsertPaths)
	deleteExists, _ := splitExistence(fullNameSetFds, deletePaths)

	ddl := lo.Ternary(len(fds.GetFile()) == 0,
		lo.Ternary[ast.DDL](len(upsertNotExists) > 0,
			&ast.CreateProtoBundle{
				Types: &ast.ProtoBundleTypes{Types: toNamedTypes(upsertNotExists)},
			}, lo.Empty[ast.DDL]()),
		lo.If[ast.DDL](len(upsertNotExists) == 0 && len(upsertExists) == 0 && len(deleteExists) == len(fullNameSetFds),
			&ast.DropProtoBundle{}).
			ElseIf(len(upsertNotExists) > 0 || len(upsertExists) > 0 || len(deleteExists) > 0,
				&ast.AlterProtoBundle{
					Insert: lo.Ternary(len(upsertNotExists) > 0,
						&ast.AlterProtoBundleInsert{Types: &ast.ProtoBundleTypes{Types: toNamedTypes(upsertNotExists)}}, lo.Empty[*ast.AlterProtoBundleInsert]()),
					Update: lo.Ternary(len(upsertExists) > 0,
						&ast.AlterProtoBundleUpdate{Types: &ast.ProtoBundleTypes{Types: toNamedTypes(upsertExists)}}, lo.Empty[*ast.AlterProtoBundleUpdate]()),
					Delete: lo.Ternary(len(deleteExists) > 0,
						&ast.AlterProtoBundleDelete{Types: &ast.ProtoBundleTypes{Types: toNamedTypes(deleteExists)}}, lo.Empty[*ast.AlterProtoBundleDelete]()),
				}).
			Else(nil),
	)

	if ddl == nil {
		return nil
	}

	return sliceOf(ddl.SQL())
}

type ShowLocalProtoStatement struct{}

func (s *ShowLocalProtoStatement) Execute(ctx context.Context, session *Session, out OperationOutput) (*Result, error) {
	fds := session.systemVariables.Internal.ProtoDescriptor

	result, err := executeStructRows(localProtoRowEncoder, slices.Collect(fdsToInfoSeq(fds)), session, out)
	if err != nil {
		return nil, err
	}
	result.KeepVariables = true
	return result, nil
}

type ShowRemoteProtoStatement struct{}

func (s *ShowRemoteProtoStatement) Execute(ctx context.Context, session *Session, out OperationOutput) (*Result, error) {
	resp, err := session.GetDatabaseDdlCached(ctx)
	if err != nil {
		return nil, err
	}

	var fds descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(resp.GetProtoDescriptors(), &fds); err != nil {
		return nil, err
	}

	result, err := executeStructRows(remoteProtoRowEncoder, slices.Collect(fdsToInfoSeq(&fds)), session, out)
	if err != nil {
		return nil, err
	}
	result.KeepVariables = true
	return result, nil
}

// Helper functions

func fdsToInfoSeq(fds *descriptorpb.FileDescriptorSet) iter.Seq[*descriptorInfo] {
	return loi.FlatMap(slices.Values(fds.GetFile()), fdpToInfo)
}

func splitExistence(fullNameSet map[string]struct{}, paths []string) ([]string, []string) {
	grouped := lo.GroupBy(paths, hasKey(fullNameSet))
	return grouped[true], grouped[false]
}

func hasKey[K comparable, V any, M map[K]V](m M) func(key K) bool {
	return func(key K) bool {
		_, ok := m[key]
		return ok
	}
}

func fdpToSeq(fdp *descriptorpb.FileDescriptorProto) iter.Seq2[string, proto.Message] {
	return func(yield func(string, proto.Message) bool) {
		var stopped bool
		err := walk.DescriptorProtosWithPath(fdp, func(name protoreflect.FullName, path protoreflect.SourcePath, message proto.Message) error {
			if stopped {
				return nil
			}

			if !yield(string(name), message) {
				stopped = true
			}

			return nil
		})
		if err != nil {
			slog.Warn("error ignored", slog.Any("err", err))
		}
	}
}

func fdpToInfo(fdp *descriptorpb.FileDescriptorProto) iter.Seq[*descriptorInfo] {
	return func(yield func(*descriptorInfo) bool) {
		for name, message := range fdpToSeq(fdp) {
			if !isValidDescriptorProto(message) {
				continue
			}
			if !yield(&descriptorInfo{FullName: name, Kind: toKind(message), Package: fdp.GetPackage(), FileName: fdp.GetName()}) {
				return
			}
		}
	}
}

func toKind(message proto.Message) string {
	var kind string
	switch message.(type) {
	case *descriptorpb.DescriptorProto:
		kind = "PROTO"
	case *descriptorpb.EnumDescriptorProto:
		kind = "ENUM"
	default:
		kind = "INVALID"
	}
	return kind
}

func hasPlaceholderDescriptorProto(descriptor *descriptorpb.DescriptorProto) bool {
	p, ok := proto.GetExtension(descriptor.GetOptions(),
		zetasql.E_PlaceholderDescriptorProto_PlaceholderDescriptor).(*zetasql.PlaceholderDescriptorProto)
	if !ok {
		return false
	}
	return p.GetIsPlaceholder()
}

func isValidDescriptorProto(message proto.Message) bool {
	switch message := message.(type) {
	case *descriptorpb.DescriptorProto:
		return !hasPlaceholderDescriptorProto(message)
	case *descriptorpb.EnumDescriptorProto:
		return true
	default:
		return false
	}
}

func toNamedType(fullName string) *ast.NamedType {
	return &ast.NamedType{
		Path: slices.Collect(loi.Map(
			slices.Values(strings.Split(fullName, ".")),
			func(s string) *ast.Ident {
				return &ast.Ident{Name: s}
			},
		)),
	}
}

func toNamedTypes(fullNames []string) []*ast.NamedType {
	return slices.Collect(loi.Map(slices.Values(fullNames), toNamedType))
}
