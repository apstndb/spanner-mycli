package main

import (
	"iter"
	"log/slog"

	"github.com/bufbuild/protocompile/walk"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	scxiter "spheric.cloud/xiter"

	"github.com/apstndb/spanner-mycli/internal/proto/zetasql"
)

func ToAny[T any](v T) any {
	return v
}

func ToAnySeq[T any](seq iter.Seq[T]) iter.Seq[any] {
	return scxiter.Map(seq, ToAny)
}

func ToMaybeInterface[T, I any](v T) iter.Seq[I] {
	if i, ok := any(v).(I); ok {
		return scxiter.Of(i)
	}
	return scxiter.Empty[I]()
}

func ToInterfaceSeq[T, I any](seq iter.Seq[T]) iter.Seq[I] {
	return scxiter.Flatmap(seq, ToMaybeInterface[T, I])
}

type descriptorInfo struct {
	FullName string
	Kind     string
	Package  string
	FileName string
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
	return scxiter.MapLower(
		scxiter.FilterValue(
			fdpToSeq(fdp),
			isValidDescriptorProto,
		),
		func(name string, message proto.Message) *descriptorInfo {
			return &descriptorInfo{FullName: name, Kind: toKind(message), Package: fdp.GetPackage(), FileName: fdp.GetName()}
		},
	)
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
