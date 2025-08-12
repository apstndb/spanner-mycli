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
	"cmp"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"

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
	"google.golang.org/protobuf/types/known/structpb"
)

func DecodeRow(row *spanner.Row) ([]string, error) {
	return spanvalue.FormatRowSpannerCLICompatible(row)
}

func formatConfigWithProto(fds *descriptorpb.FileDescriptorSet, multiline bool) (*spanvalue.FormatConfig, error) {
	types, err := dynamicTypesByFDS(fds)
	if err != nil {
		return nil, err
	}

	return &spanvalue.FormatConfig{
		NullString:  "NULL",
		FormatArray: spanvalue.FormatUntypedArray,
		FormatStruct: spanvalue.FormatStruct{
			FormatStructField: spanvalue.FormatSimpleStructField,
			FormatStructParen: spanvalue.FormatBracketStruct,
		},
		FormatComplexPlugins: []spanvalue.FormatComplexFunc{
			formatInterval(), // workaround
			formatUUID(),     // workaround
			formatProto(types, multiline),
			formatEnum(types),
		},
		FormatNullable: spanvalue.FormatNullableSpannerCLICompatible,
	}, nil
}

// formatConfigForSQL creates a FormatConfig that formats values as SQL literals
// instead of display format. This is used for SQL export formats.
// For now, we just use LiteralFormatConfig directly and see if it handles
// INTERVAL, UUID, PROTO, and ENUM correctly.
func formatConfigForSQL(fds *descriptorpb.FileDescriptorSet) (*spanvalue.FormatConfig, error) {
	// First, let's try using LiteralFormatConfig as-is
	// TODO: Verify if INTERVAL, UUID, PROTO, ENUM need special handling
	// If they do, it might be better to fix this in spanvalue itself
	return spanvalue.LiteralFormatConfig, nil
}

func dynamicTypesByFDS(fds *descriptorpb.FileDescriptorSet) (*dynamicpb.Types, error) {
	if fds == nil {
		return dynamicpb.NewTypes(nil), nil
	}

	files, err := protodesc.NewFiles(fds)
	if err != nil {
		return nil, err
	}

	return dynamicpb.NewTypes(files), nil
}

type protoEnumResolver interface {
	protoregistry.MessageTypeResolver
	FindEnumByName(protoreflect.FullName) (protoreflect.EnumType, error)
}

var (
	_ protoEnumResolver = (*dynamicpb.Types)(nil)
	_ protoEnumResolver = (*protoregistry.Types)(nil)
)

func formatProto(types protoEnumResolver, multiline bool) func(formatter spanvalue.Formatter, value spanner.GenericColumnValue, toplevel bool) (string, error) {
	return func(formatter spanvalue.Formatter, value spanner.GenericColumnValue, toplevel bool) (string, error) {
		if value.Type.GetCode() != sppb.TypeCode_PROTO {
			return "", spanvalue.ErrFallthrough
		}

		messageType, err := types.FindMessageByName(protoreflect.FullName(value.Type.GetProtoTypeFqn()))
		if errors.Is(err, protoregistry.NotFound) {
			return "", spanvalue.ErrFallthrough
		} else if err != nil {
			return "", err
		}

		b, err := base64.StdEncoding.DecodeString(value.Value.GetStringValue())
		if err != nil {
			return "", err
		}

		m := messageType.New()
		if err = proto.Unmarshal(b, m.Interface()); err != nil {
			return "", err
		}
		return strings.TrimSpace(prototext.MarshalOptions{Multiline: multiline}.Format(m.Interface())), nil
	}
}

func formatEnum(types protoEnumResolver) func(formatter spanvalue.Formatter, value spanner.GenericColumnValue, toplevel bool) (string, error) {
	return func(formatter spanvalue.Formatter, value spanner.GenericColumnValue, toplevel bool) (string, error) {
		if value.Type.GetCode() != sppb.TypeCode_ENUM {
			return "", spanvalue.ErrFallthrough
		}

		enumType, err := types.FindEnumByName(protoreflect.FullName(value.Type.GetProtoTypeFqn()))
		if errors.Is(err, protoregistry.NotFound) {
			return "", spanvalue.ErrFallthrough
		} else if err != nil {
			return "", err
		}

		n, err := strconv.ParseInt(value.Value.GetStringValue(), 10, 64)
		if err != nil {
			return "", err
		}

		return cmp.Or(
			string(enumType.Descriptor().Values().ByNumber(protoreflect.EnumNumber(n)).Name()),
			value.Value.GetStringValue()), nil
	}
}

// formatUUID is workaround because google-cloud-go/spanner doesn't yet support UUID type.
func formatUUID() spanvalue.FormatComplexFunc {
	return func(formatter spanvalue.Formatter, value spanner.GenericColumnValue, toplevel bool) (string, error) {
		if value.Type.GetCode() != sppb.TypeCode_UUID {
			return "", spanvalue.ErrFallthrough
		}

		if _, ok := value.Value.Kind.(*structpb.Value_NullValue); ok {
			return "NULL", nil
		}
		return value.Value.GetStringValue(), nil
	}
}

// formatInterval is workaround because google-cloud-go/spanner doesn't yet support INTERVAL type.
func formatInterval() spanvalue.FormatComplexFunc {
	return func(formatter spanvalue.Formatter, value spanner.GenericColumnValue, toplevel bool) (string, error) {
		if value.Type.GetCode() != sppb.TypeCode_INTERVAL {
			return "", spanvalue.ErrFallthrough
		}

		if _, ok := value.Value.Kind.(*structpb.Value_NullValue); ok {
			return "NULL", nil
		}
		return value.Value.GetStringValue(), nil
	}
}

// formatTypeSimple is format type for headers.
func formatTypeSimple(typ *sppb.Type) string {
	return spantype.FormatType(typ, spantype.FormatOption{
		Struct: spantype.StructModeBase,
		Proto:  spantype.ProtoEnumModeLeafWithKind,
		Enum:   spantype.ProtoEnumModeLeafWithKind,
		Array:  spantype.ArrayModeRecursive,
	})
}

// formatTypeVerbose is format type for DESCRIBE.
func formatTypeVerbose(typ *sppb.Type) string {
	return spantype.FormatTypeMoreVerbose(typ)
}
