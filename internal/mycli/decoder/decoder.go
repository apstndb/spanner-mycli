//
// Copyright 2020 Google LLC
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
//

package decoder

import (
	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spantype"
	"github.com/apstndb/spanvalue"
	"github.com/apstndb/spanvalue/protofmt"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/types/descriptorpb"
)

func DecodeRow(row *spanner.Row) ([]string, error) {
	return spanvalue.FormatRowSpannerCLICompatible(row)
}

// FormatConfigWithProto creates a format configuration for decoding Spanner values,
// with special handling for PROTO and ENUM types based on the provided protobuf file descriptor set.
// If fds is nil, it returns a config without custom proto/enum support.
// The multiline parameter controls the formatting of protobuf messages.
func FormatConfigWithProto(fds *descriptorpb.FileDescriptorSet, multiline bool) (*spanvalue.FormatConfig, error) {
	resolver, err := protofmt.ProtoEnumResolverFromFileDescriptorSet(fds)
	if err != nil {
		return nil, err
	}

	fc := spanvalue.SpannerCLICompatibleFormatConfig().Clone()
	fc.FormatComplexPlugins = append(
		[]spanvalue.FormatComplexFunc{
			protofmt.FormatProtoTextValue(protofmt.ProtoTextValueOptions{
				Resolver: resolver,
				Marshal:  prototext.MarshalOptions{Multiline: multiline},
			}),
			protofmt.FormatEnumNameValue(protofmt.EnumNameValueOptions{Resolver: resolver}),
		},
		fc.FormatComplexPlugins...,
	)
	return fc, nil
}

// FormatTypeSimple is format type for headers.
func FormatTypeSimple(typ *sppb.Type) string {
	return spantype.FormatType(typ, spantype.FormatOption{
		Struct: spantype.StructModeBase,
		Proto:  spantype.ProtoEnumModeLeafWithKind,
		Enum:   spantype.ProtoEnumModeLeafWithKind,
		Array:  spantype.ArrayModeRecursive,
	})
}

// FormatTypeVerbose is format type for DESCRIBE.
func FormatTypeVerbose(typ *sppb.Type) string {
	return spantype.FormatTypeMoreVerbose(typ)
}
