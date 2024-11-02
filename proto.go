package main

import (
	"iter"
	"slices"

	"github.com/apstndb/lox"
	"github.com/apstndb/spanner-mycli/internal/proto/zetasql"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"spheric.cloud/xiter"
)

type hasName interface {
	GetName() string
}

func ToAny[T any](v T) any {
	return v
}

func ToAnySeq[T any](seq iter.Seq[T]) iter.Seq[any] {
	return xiter.Map(seq, ToAny)
}

func ToMaybeInterface[T, I any](v T) iter.Seq[I] {
	if i, ok := any(v).(I); ok {
		return xiter.Of(i)
	}
	return xiter.Empty[I]()
}

func ToInterfaceSeq[T, I any](seq iter.Seq[T]) iter.Seq[I] {
	return xiter.Flatmap(seq, ToMaybeInterface[T, I])
}

func enumerateNamesInFDP(fdp *descriptorpb.FileDescriptorProto) iter.Seq[hasName] {
	return xiter.Concat(
		xiter.Flatmap(
			xiter.Filter(
				xiter.Flatmap(
					xiter.Map(
						slices.Values(fdp.GetMessageType()),
						dpToDPWithPath("")),
					flattenWithNestedType),
				lox.Not(hasPlaceholderDescriptorProto)),
			flattenWithEnumTypes),
		ToInterfaceSeq[*descriptorpb.EnumDescriptorProto, hasName](slices.Values(fdp.GetEnumType())))
}

func dpToDPWithPath(parentPath string) func(*descriptorpb.DescriptorProto) *descriptorProtoWithPath {
	return func(dp *descriptorpb.DescriptorProto) *descriptorProtoWithPath {
		return &descriptorProtoWithPath{dp, parentPath}
	}
}

func fdpToRows(fdp *descriptorpb.FileDescriptorProto) iter.Seq[Row] {
	pkg := fdp.GetPackage()
	return xiter.Map(enumerateNamesInFDP(fdp), func(v hasName) Row {
		return Row{Columns: sliceOf(lox.IfOrEmpty(pkg != "", pkg+".")+v.GetName(), pkg, fdp.GetName())}
	})
}

type descriptorProtoWithPath struct {
	DescriptorProto *descriptorpb.DescriptorProto
	Parent          string
}

func (dp *descriptorProtoWithPath) GetName() string {
	return lox.IfOrEmpty(dp.Parent != "", dp.Parent+".") + dp.DescriptorProto.GetName()
}

type hasNameWithParent struct {
	Value  hasName
	Parent string
}

func (h *hasNameWithParent) GetName() string {
	return lox.IfOrEmpty(h.Parent != "", h.Parent+".") + h.Value.GetName()
}

func flattenWithEnumTypes(parent *descriptorProtoWithPath) iter.Seq[hasName] {
	return xiter.Concat(
		xiter.Of(hasName(parent)),
		xiter.Map(
			slices.Values(parent.DescriptorProto.GetEnumType()),
			func(dp *descriptorpb.EnumDescriptorProto) hasName {
				return &hasNameWithParent{Value: hasName(dp), Parent: parent.GetName()}
			}))
}

func flattenWithNestedType(parent *descriptorProtoWithPath) iter.Seq[*descriptorProtoWithPath] {
	return xiter.Concat(
		xiter.Of(parent),
		xiter.Flatmap(
			slices.Values(parent.DescriptorProto.GetNestedType()),
			func(dp *descriptorpb.DescriptorProto) iter.Seq[*descriptorProtoWithPath] {
				return flattenWithNestedType(&descriptorProtoWithPath{dp, parent.GetName()})
			}))
}

func hasPlaceholderDescriptorProto(parent *descriptorProtoWithPath) bool {
	op := parent.DescriptorProto.GetOptions()
	return proto.GetExtension(op, zetasql.E_PlaceholderDescriptorProto_PlaceholderDescriptor).(*zetasql.PlaceholderDescriptorProto).
		GetIsPlaceholder()
}
