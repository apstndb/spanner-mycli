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

package decoder

import (
	"testing"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestJSONFormatConfig(t *testing.T) {
	t.Parallel()

	fc := JSONFormatConfig()

	tests := []struct {
		name     string
		gcv      spanner.GenericColumnValue
		wantJSON string
	}{
		{
			name: "NULL",
			gcv: spanner.GenericColumnValue{
				Type:  &sppb.Type{Code: sppb.TypeCode_STRING},
				Value: structpb.NewNullValue(),
			},
			wantJSON: "null",
		},
		{
			name: "BOOL true",
			gcv: spanner.GenericColumnValue{
				Type:  &sppb.Type{Code: sppb.TypeCode_BOOL},
				Value: structpb.NewBoolValue(true),
			},
			wantJSON: "true",
		},
		{
			name: "BOOL false",
			gcv: spanner.GenericColumnValue{
				Type:  &sppb.Type{Code: sppb.TypeCode_BOOL},
				Value: structpb.NewBoolValue(false),
			},
			wantJSON: "false",
		},
		{
			name: "INT64",
			gcv: spanner.GenericColumnValue{
				Type:  &sppb.Type{Code: sppb.TypeCode_INT64},
				Value: structpb.NewStringValue("42"),
			},
			wantJSON: "42",
		},
		{
			name: "INT64 large",
			gcv: spanner.GenericColumnValue{
				Type:  &sppb.Type{Code: sppb.TypeCode_INT64},
				Value: structpb.NewStringValue("9223372036854775807"),
			},
			wantJSON: "9223372036854775807",
		},
		{
			name: "FLOAT64",
			gcv: spanner.GenericColumnValue{
				Type:  &sppb.Type{Code: sppb.TypeCode_FLOAT64},
				Value: structpb.NewNumberValue(3.14),
			},
			wantJSON: "3.14",
		},
		{
			name: "FLOAT64 NaN as string",
			gcv: spanner.GenericColumnValue{
				Type:  &sppb.Type{Code: sppb.TypeCode_FLOAT64},
				Value: structpb.NewStringValue("NaN"),
			},
			wantJSON: `"NaN"`,
		},
		{
			name: "FLOAT64 Infinity as string",
			gcv: spanner.GenericColumnValue{
				Type:  &sppb.Type{Code: sppb.TypeCode_FLOAT64},
				Value: structpb.NewStringValue("Infinity"),
			},
			wantJSON: `"Infinity"`,
		},
		{
			name: "STRING",
			gcv: spanner.GenericColumnValue{
				Type:  &sppb.Type{Code: sppb.TypeCode_STRING},
				Value: structpb.NewStringValue("hello"),
			},
			wantJSON: `"hello"`,
		},
		{
			name: "STRING with special chars",
			gcv: spanner.GenericColumnValue{
				Type:  &sppb.Type{Code: sppb.TypeCode_STRING},
				Value: structpb.NewStringValue("line1\nline2"),
			},
			wantJSON: `"line1\nline2"`,
		},
		{
			name: "TIMESTAMP",
			gcv: spanner.GenericColumnValue{
				Type:  &sppb.Type{Code: sppb.TypeCode_TIMESTAMP},
				Value: structpb.NewStringValue("2024-01-15T12:00:00Z"),
			},
			wantJSON: `"2024-01-15T12:00:00Z"`,
		},
		{
			name: "DATE",
			gcv: spanner.GenericColumnValue{
				Type:  &sppb.Type{Code: sppb.TypeCode_DATE},
				Value: structpb.NewStringValue("2024-01-15"),
			},
			wantJSON: `"2024-01-15"`,
		},
		{
			name: "NUMERIC",
			gcv: spanner.GenericColumnValue{
				Type:  &sppb.Type{Code: sppb.TypeCode_NUMERIC},
				Value: structpb.NewStringValue("123.456"),
			},
			wantJSON: `"123.456"`,
		},
		{
			name: "JSON column",
			gcv: spanner.GenericColumnValue{
				Type:  &sppb.Type{Code: sppb.TypeCode_JSON},
				Value: structpb.NewStringValue(`{"key":"value"}`),
			},
			wantJSON: `{"key":"value"}`,
		},
		{
			name: "BYTES",
			gcv: spanner.GenericColumnValue{
				Type:  &sppb.Type{Code: sppb.TypeCode_BYTES},
				Value: structpb.NewStringValue("SGVsbG8="),
			},
			wantJSON: `"SGVsbG8="`,
		},
		{
			name: "ARRAY of INT64",
			gcv: spanner.GenericColumnValue{
				Type: &sppb.Type{
					Code:             sppb.TypeCode_ARRAY,
					ArrayElementType: &sppb.Type{Code: sppb.TypeCode_INT64},
				},
				Value: structpb.NewListValue(&structpb.ListValue{
					Values: []*structpb.Value{
						structpb.NewStringValue("1"),
						structpb.NewStringValue("2"),
						structpb.NewStringValue("3"),
					},
				}),
			},
			wantJSON: `[1,2,3]`,
		},
		{
			name: "ARRAY of STRING",
			gcv: spanner.GenericColumnValue{
				Type: &sppb.Type{
					Code:             sppb.TypeCode_ARRAY,
					ArrayElementType: &sppb.Type{Code: sppb.TypeCode_STRING},
				},
				Value: structpb.NewListValue(&structpb.ListValue{
					Values: []*structpb.Value{
						structpb.NewStringValue("a"),
						structpb.NewStringValue("b"),
					},
				}),
			},
			wantJSON: `["a","b"]`,
		},
		{
			name: "ARRAY with NULL element",
			gcv: spanner.GenericColumnValue{
				Type: &sppb.Type{
					Code:             sppb.TypeCode_ARRAY,
					ArrayElementType: &sppb.Type{Code: sppb.TypeCode_INT64},
				},
				Value: structpb.NewListValue(&structpb.ListValue{
					Values: []*structpb.Value{
						structpb.NewStringValue("1"),
						structpb.NewNullValue(),
						structpb.NewStringValue("3"),
					},
				}),
			},
			wantJSON: `[1,null,3]`,
		},
		{
			name: "STRUCT",
			gcv: spanner.GenericColumnValue{
				Type: &sppb.Type{
					Code: sppb.TypeCode_STRUCT,
					StructType: &sppb.StructType{
						Fields: []*sppb.StructType_Field{
							{Name: "name", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
							{Name: "age", Type: &sppb.Type{Code: sppb.TypeCode_INT64}},
						},
					},
				},
				Value: structpb.NewListValue(&structpb.ListValue{
					Values: []*structpb.Value{
						structpb.NewStringValue("Alice"),
						structpb.NewStringValue("30"),
					},
				}),
			},
			wantJSON: `{"name":"Alice","age":30}`,
		},
		{
			name: "ARRAY of STRUCT",
			gcv: spanner.GenericColumnValue{
				Type: &sppb.Type{
					Code: sppb.TypeCode_ARRAY,
					ArrayElementType: &sppb.Type{
						Code: sppb.TypeCode_STRUCT,
						StructType: &sppb.StructType{
							Fields: []*sppb.StructType_Field{
								{Name: "COUNT", Type: &sppb.Type{Code: sppb.TypeCode_INT64}},
								{Name: "MEAN", Type: &sppb.Type{Code: sppb.TypeCode_FLOAT64}},
							},
						},
					},
				},
				Value: structpb.NewListValue(&structpb.ListValue{
					Values: []*structpb.Value{
						structpb.NewListValue(&structpb.ListValue{
							Values: []*structpb.Value{
								structpb.NewStringValue("1"),
								structpb.NewNumberValue(0.057294),
							},
						}),
					},
				}),
			},
			wantJSON: `[{"COUNT":1,"MEAN":0.057294}]`,
		},
		{
			name: "STRUCT with unnamed fields",
			gcv: spanner.GenericColumnValue{
				Type: &sppb.Type{
					Code: sppb.TypeCode_STRUCT,
					StructType: &sppb.StructType{
						Fields: []*sppb.StructType_Field{
							{Name: "", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
							{Name: "", Type: &sppb.Type{Code: sppb.TypeCode_INT64}},
						},
					},
				},
				Value: structpb.NewListValue(&structpb.ListValue{
					Values: []*structpb.Value{
						structpb.NewStringValue("value"),
						structpb.NewStringValue("42"),
					},
				}),
			},
			wantJSON: `{"f0":"value","f1":42}`,
		},
		{
			name: "NULL ARRAY",
			gcv: spanner.GenericColumnValue{
				Type: &sppb.Type{
					Code:             sppb.TypeCode_ARRAY,
					ArrayElementType: &sppb.Type{Code: sppb.TypeCode_INT64},
				},
				Value: structpb.NewNullValue(),
			},
			wantJSON: "null",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := fc.FormatToplevelColumn(tt.gcv)
			if err != nil {
				t.Fatalf("FormatToplevelColumn() error = %v", err)
			}

			if diff := cmp.Diff(tt.wantJSON, got); diff != "" {
				t.Errorf("JSON output mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
