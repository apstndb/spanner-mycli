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

package main

import (
	"encoding/base64"
	"errors"
	"fmt"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spantype"
	"github.com/apstndb/spanvalue"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

func DecodeRow(row *spanner.Row) ([]string, error) {
	return spanvalue.FormatRowSpannerCLICompatible(row)
}

func DecodeColumn(column spanner.GenericColumnValue) (string, error) {
	return spanvalue.FormatColumnSpannerCLICompatible(column)
}

func formatConfig(fds *descriptorpb.FileDescriptorSet) (*spanvalue.FormatConfig, error) {
	var files *protoregistry.Files
	if fds != nil {
		var err error
		files, err = protodesc.NewFiles(fds)
		if err != nil {
			return nil, err
		}
	}

	return &spanvalue.FormatConfig{
		NullString:  "NULL",
		FormatArray: spanvalue.FormatUntypedArray,
		FormatStruct: spanvalue.FormatStruct{
			FormatStructField: spanvalue.FormatSimpleStructField,
			FormatStructParen: spanvalue.FormatBracketStruct,
		},
		FormatComplexPlugins: []spanvalue.FormatComplexFunc{
			func(formatter spanvalue.Formatter, value spanner.GenericColumnValue, toplevel bool) (string, error) {
				if value.Type.GetCode() == sppb.TypeCode_PROTO {
					desc, err := files.FindDescriptorByName(protoreflect.FullName(value.Type.GetProtoTypeFqn()))
					switch {
					case errors.Is(err, protoregistry.NotFound):
						return "", spanvalue.ErrFallthrough
					case err != nil:
						return "", err
					default:
						md, ok := desc.(protoreflect.MessageDescriptor)
						if !ok {
							return "", fmt.Errorf("protoFqn %v corresponds not a message descriptor: %T", value.Type.GetProtoTypeFqn(), desc)
						}

						message := dynamicpb.NewMessage(md)
						b, err := base64.StdEncoding.DecodeString(value.Value.GetStringValue())
						if err != nil {
							return "", err
						}

						if err = proto.Unmarshal(b, message); err != nil {
							return "", err
						}
						return prototext.MarshalOptions{Multiline: false}.Format(message), nil
					}
				}
				return "", spanvalue.ErrFallthrough
			},
		},
		FormatNullable: spanvalue.FormatNullableSpannerCLICompatible,
	}, nil
}

func DecodeRowExperimental(row *spanner.Row, fds *descriptorpb.FileDescriptorSet) ([]string, error) {
	config, err := formatConfig(fds)
	if err != nil {
		return nil, err
	}
	return config.FormatRow(row)
}

func DecodeColumnExperimental(column spanner.GenericColumnValue, fds *descriptorpb.FileDescriptorSet) (string, error) {
	config, err := formatConfig(fds)
	if err != nil {
		return "", err
	}
	return config.FormatToplevelColumn(column)
}

// formatTypeSimple is format type for headers.
func formatTypeSimple(typ *sppb.Type) string {
	return spantype.FormatType(typ, spantype.FormatOption{
		Struct: spantype.StructModeBase,
		Proto:  spantype.ProtoEnumModeBase,
		Enum:   spantype.ProtoEnumModeBase,
		Array:  spantype.ArrayModeRecursive,
	})
}

// formatTypeVerbose is format type for DESCRIBE.
func formatTypeVerbose(typ *sppb.Type) string {
	return spantype.FormatTypeMoreVerbose(typ)
}
