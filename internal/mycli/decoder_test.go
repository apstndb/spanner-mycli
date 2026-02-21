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

package mycli

import (
	"math/big"
	"testing"
	"time"

	"cloud.google.com/go/spanner/testdata/protos"
	"github.com/apstndb/spantype/typector"
	"github.com/apstndb/spanvalue/gcvctor"
	"github.com/google/go-cmp/cmp"
	"github.com/samber/lo"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/typepb"

	"cloud.google.com/go/civil"
	"cloud.google.com/go/spanner"
)

func createRow(t *testing.T, values []interface{}) *spanner.Row {
	t.Helper()

	// column names are not important in this test, so use dummy name
	names := make([]string, len(values))
	for i := 0; i < len(names); i++ {
		names[i] = "dummy"
	}

	row, err := spanner.NewRow(names, values)
	if err != nil {
		t.Fatalf("Creating spanner row failed unexpectedly: %v", err)
	}
	return row
}

func createColumnValue(t *testing.T, value interface{}) spanner.GenericColumnValue {
	t.Helper()

	row := createRow(t, []interface{}{value})
	var cv spanner.GenericColumnValue
	if err := row.Column(0, &cv); err != nil {
		t.Fatalf("Creating spanner column value failed unexpectedly: %v", err)
	}

	return cv
}

func equalStringSlice(a []string, b []string) bool {
	if a == nil || b == nil {
		return false
	}
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

type jsonMessage struct {
	Msg string `json:"msg"`
}

func TestDecodeColumn(t *testing.T) {
	t.Parallel()
	tests := []struct {
		desc        string
		value       interface{}
		fds         *descriptorpb.FileDescriptorSet
		multiline   bool
		want        string
		wantMessage proto.Message
	}{
		// non-nullable
		{
			desc:  "bool",
			value: true,
			want:  "true",
		},
		{
			desc:  "bytes",
			value: []byte{'a', 'b', 'c', 'd'},
			want:  "YWJjZA==", // base64 encoded 'abc'
		},
		{
			desc:  "float32",
			value: float32(1.23),
			want:  "1.230000",
		},
		{
			desc:  "float64",
			value: 1.23,
			want:  "1.230000",
		},
		{
			desc:  "int64",
			value: 123,
			want:  "123",
		},
		{
			desc:  "numeric",
			value: big.NewRat(123, 100),
			want:  "1.23",
		},
		{
			desc:  "string",
			value: "foo",
			want:  "foo",
		},
		{
			desc:  "timestamp",
			value: time.Unix(1516676400, 0),
			want:  "2018-01-23T03:00:00Z",
		},
		{
			desc:  "date",
			value: civil.DateOf(time.Unix(1516676400, 0)),
			want:  "2018-01-23",
		},
		{
			desc:  "json",
			value: spanner.NullJSON{Value: jsonMessage{Msg: "foo"}, Valid: true},
			want:  `{"msg":"foo"}`,
		},
		{
			desc:  "json null is not NULL",
			value: spanner.NullJSON{Value: nil, Valid: true},
			want:  `null`,
		},

		// nullable
		{
			desc:  "null bool",
			value: spanner.NullBool{Bool: false, Valid: false},
			want:  "NULL",
		},
		{
			desc:  "null bytes",
			value: []byte(nil),
			want:  "NULL",
		},
		{
			desc:  "null float32",
			value: spanner.NullFloat32{Float32: 0, Valid: false},
			want:  "NULL",
		},
		{
			desc:  "null float64",
			value: spanner.NullFloat64{Float64: 0, Valid: false},
			want:  "NULL",
		},
		{
			desc:  "null int64",
			value: spanner.NullInt64{Int64: 0, Valid: false},
			want:  "NULL",
		},
		{
			desc:  "null numeric",
			value: spanner.NullNumeric{Numeric: big.Rat{}, Valid: false},
			want:  "NULL",
		},
		{
			desc:  "null string",
			value: spanner.NullString{StringVal: "", Valid: false},
			want:  "NULL",
		},
		{
			desc:  "null time",
			value: spanner.NullTime{Time: time.Unix(0, 0), Valid: false},
			want:  "NULL",
		},
		{
			desc:  "null date",
			value: spanner.NullDate{Date: civil.DateOf(time.Unix(0, 0)), Valid: false},
			want:  "NULL",
		},
		{
			desc:  "null json",
			value: spanner.NullJSON{Value: nil, Valid: false},
			want:  "NULL",
		},

		// array non-nullable
		{
			desc:  "empty array",
			value: []bool{},
			want:  "[]",
		},
		{
			desc:  "array bool",
			value: []bool{true, false},
			want:  "[true, false]",
		},
		{
			desc:  "array bytes",
			value: [][]byte{{'a', 'b', 'c', 'd'}, {'e', 'f', 'g', 'h'}},
			want:  "[YWJjZA==, ZWZnaA==]",
		},
		{
			desc:  "array float32",
			value: []float32{1.23, 2.45},
			want:  "[1.230000, 2.450000]",
		},
		{
			desc:  "array float64",
			value: []float64{1.23, 2.45},
			want:  "[1.230000, 2.450000]",
		},
		{
			desc:  "array int64",
			value: []int64{123, 456},
			want:  "[123, 456]",
		},
		{
			desc:  "array numeric",
			value: []*big.Rat{big.NewRat(123, 100), big.NewRat(456, 1)},
			want:  "[1.23, 456]",
		},
		{
			desc:  "array string",
			value: []string{"foo", "bar"},
			want:  "[foo, bar]",
		},
		{
			desc:  "array timestamp",
			value: []time.Time{time.Unix(1516676400, 0), time.Unix(1516680000, 0)},
			want:  "[2018-01-23T03:00:00Z, 2018-01-23T04:00:00Z]",
		},
		{
			desc:  "array date",
			value: []civil.Date{civil.DateOf(time.Unix(1516676400, 0)), civil.DateOf(time.Unix(1516762800, 0))},
			want:  "[2018-01-23, 2018-01-24]",
		},
		{
			desc: "array struct",
			value: []struct {
				X int64
				Y spanner.NullString
			}{
				{
					X: 10,
					Y: spanner.NullString{StringVal: "Hello", Valid: true},
				},
				{
					X: 20,
					Y: spanner.NullString{StringVal: "", Valid: false},
				},
			},
			want: "[[10, Hello], [20, NULL]]",
		},
		{
			desc: "array json",
			value: []spanner.NullJSON{
				{Value: jsonMessage{Msg: "foo"}, Valid: true},
				{Value: jsonMessage{Msg: "bar"}, Valid: true},
			},
			want: `[{"msg":"foo"}, {"msg":"bar"}]`,
		},

		// array nullable
		{
			desc:  "null array bool",
			value: []bool(nil),
			want:  "NULL",
		},
		{
			desc:  "null array bytes",
			value: [][]byte(nil),
			want:  "NULL",
		},
		{
			desc:  "null array float32",
			value: []float32(nil),
			want:  "NULL",
		},
		{
			desc:  "null array float64",
			value: []float64(nil),
			want:  "NULL",
		},
		{
			desc:  "null array int64",
			value: []int64(nil),
			want:  "NULL",
		},
		{
			desc:  "null array numeric",
			value: []*big.Rat(nil),
			want:  "NULL",
		},
		{
			desc:  "null array string",
			value: []string(nil),
			want:  "NULL",
		},
		{
			desc:  "null array timestamp",
			value: []time.Time(nil),
			want:  "NULL",
		},
		{
			desc:  "null array date",
			value: []civil.Date(nil),
			want:  "NULL",
		},
		{
			desc:  "null array json",
			value: []spanner.NullJSON(nil),
			want:  "NULL",
		},

		// UUID
		// TODO: Use NullUUID when available. See https://github.com/googleapis/google-cloud-go/pull/11345.
		{
			desc:  "UUID",
			value: gcvctor.StringBasedValue(sppb.TypeCode_UUID, "bd667006-c4a7-49de-814f-e2a2ec65abca"),
			want:  "bd667006-c4a7-49de-814f-e2a2ec65abca",
		},

		// PROTO
		// This table tests uses spanner.GenericColumnValue because of non-stability
		{
			desc:  "proto",
			value: gcvctor.ProtoValue("examples.spanner.music.SingerInfo", []byte("abcd")),
			want:  "YWJjZA==",
		},
		{
			desc:  "null proto",
			value: gcvctor.TypedNull(typector.FQNToProtoType("examples.spanner.music.SingerInfo")),
			want:  "NULL",
		},
		{
			desc: "array proto",
			value: spanner.GenericColumnValue{
				Type: typector.ElemTypeToArrayType(typector.FQNToProtoType("examples.spanner.music.SingerInfo")),
				Value: structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{
					structpb.NewStringValue("YWJjZA=="),
					structpb.NewStringValue("ZWZnaA=="),
				}}),
			},
			want: "[YWJjZA==, ZWZnaA==]",
		},
		{
			desc:  "null array proto",
			value: gcvctor.TypedNull(typector.ElemTypeToArrayType(typector.FQNToProtoType("examples.spanner.music.SingerInfo"))),
			want:  "NULL",
		},

		// ENUM
		{
			desc:  "enum",
			value: typepb.Syntax_SYNTAX_PROTO3,
			want:  "1",
		},
		{
			desc:  "null enum",
			value: (*typepb.Syntax)(nil),
			want:  "NULL",
		},
		{
			desc:  "array enum",
			value: []*typepb.Syntax{typepb.Syntax_SYNTAX_PROTO2.Enum(), typepb.Syntax_SYNTAX_PROTO3.Enum()},
			want:  "[0, 1]",
		},
		{
			desc:  "null array enum",
			value: []*typepb.Syntax(nil),
			want:  "NULL",
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			got, err := lo.Must(formatConfigWithProto(test.fds, test.multiline)).FormatToplevelColumn(createColumnValue(t, test.value))
			if err != nil {
				t.Error(err)
			}
			if test.wantMessage != nil {
				nm := dynamicpb.NewMessageType(test.wantMessage.ProtoReflect().Descriptor()).New()
				err := prototext.Unmarshal([]byte(got), nm.Interface())
				if err != nil {
					t.Error(err)
				}
				if diff := cmp.Diff(nm.Interface(), test.wantMessage, protocmp.Transform()); diff != "" {
					t.Errorf("formatConfigWithProto(%v) mismatch (-got +want):\n%s", test.value, diff)
				}
			} else {
				if got != test.want {
					t.Errorf("DecodeColumn(%v) = %v, want = %v", test.value, got, test.want)
				}
			}
		})
	}
}

func TestDecodeColumnRoundTripEnum(t *testing.T) {
	t.Parallel()
	tests := []struct {
		desc  string
		value interface{}
		fds   *descriptorpb.FileDescriptorSet
		want  interface {
			Type() protoreflect.EnumType
			Number() protoreflect.EnumNumber
		}
	}{
		{
			desc:  "proto with FileDescriptorProto",
			value: gcvctor.EnumValue("examples.spanner.music.Genre", int64(protos.Genre_POP)),
			fds:   &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{protodesc.ToFileDescriptorProto(protos.File_singer_proto)}},
			want:  protos.Genre_POP.Enum(),
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			got, err := lo.Must(formatConfigWithProto(test.fds, false)).FormatToplevelColumn(createColumnValue(t, test.value))
			if err != nil {
				t.Error(err)
			}

			gotEnumValue := test.want.Type().Descriptor().Values().ByName(protoreflect.Name(got))
			gotNumber := gotEnumValue.Number()
			if gotNumber != test.want.Number() {
				t.Errorf("formatConfigWithProto(%v): %v(%v), want: %v(%v)", test.value, gotEnumValue.Name(), gotNumber, test.want, test.want.Number())
			}
		})
	}
}

func TestDecodeColumnRoundTripProto(t *testing.T) {
	t.Parallel()
	tests := []struct {
		desc      string
		value     interface{}
		fds       *descriptorpb.FileDescriptorSet
		multiline bool
		want      proto.Message
	}{
		{
			desc:  "proto with FileDescriptorProto",
			value: gcvctor.ProtoValue("examples.spanner.music.SingerInfo", lo.Must(proto.Marshal(&protos.SingerInfo{SingerId: proto.Int64(1), Genre: protos.Genre_POP.Enum()}))),
			fds:   &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{protodesc.ToFileDescriptorProto(protos.File_singer_proto)}},
			want:  &protos.SingerInfo{SingerId: proto.Int64(1), Genre: protos.Genre_POP.Enum()},
		},
		{
			desc:      "proto with FileDescriptorProto",
			value:     gcvctor.ProtoValue("examples.spanner.music.SingerInfo", lo.Must(proto.Marshal(&protos.SingerInfo{SingerId: proto.Int64(1), Genre: protos.Genre_POP.Enum()}))),
			fds:       &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{protodesc.ToFileDescriptorProto(protos.File_singer_proto)}},
			multiline: true,
			want:      &protos.SingerInfo{SingerId: proto.Int64(1), Genre: protos.Genre_POP.Enum()},
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			got, err := lo.Must(formatConfigWithProto(test.fds, test.multiline)).FormatToplevelColumn(createColumnValue(t, test.value))
			if err != nil {
				t.Error(err)
			}

			nm := dynamicpb.NewMessageType(test.want.ProtoReflect().Descriptor()).New()
			if err := prototext.Unmarshal([]byte(got), nm.Interface()); err != nil {
				t.Error(err)
			}
			if diff := cmp.Diff(nm.Interface(), test.want, protocmp.Transform()); diff != "" {
				t.Errorf("formatConfigWithProto(%v) mismatch (-got +want):\n%s", test.value, diff)
			}
		})
	}
}

func TestDecodeRow(t *testing.T) {
	t.Parallel()
	tests := []struct {
		desc   string
		values []interface{}
		want   []string
	}{
		{
			desc:   "non-null columns",
			values: []interface{}{"foo", 123},
			want:   []string{"foo", "123"},
		},
		{
			desc:   "non-null column and null column",
			values: []interface{}{"foo", spanner.NullString{StringVal: "", Valid: false}},
			want:   []string{"foo", "NULL"},
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			got, err := DecodeRow(createRow(t, test.values))
			if err != nil {
				t.Error(err)
			}
			if !equalStringSlice(got, test.want) {
				t.Errorf("DecodeRow(%v) = %v, want = %v", test.values, got, test.want)
			}
		})
	}
}

func TestFormatTypeVerbose(t *testing.T) {
	t.Parallel()
	tests := []struct {
		desc     string
		sppbType *sppb.Type
		want     string
	}{
		{
			desc:     "PROTO",
			sppbType: typector.FQNToProtoType("example.ProtoType"),
			want:     "PROTO<example.ProtoType>",
		},
		{
			desc:     "ENUM",
			sppbType: typector.FQNToEnumType("example.EnumType"),
			want:     "ENUM<example.EnumType>",
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			got := formatTypeVerbose(test.sppbType)
			if diff := cmp.Diff(got, test.want); diff != "" {
				t.Errorf("formatTypeVerbose(%v) mismatch (-got +want):\n%s", test.sppbType, diff)
			}
		})
	}
}
