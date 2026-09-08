// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mycli

import (
	"math"
	"testing"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanvalue"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestSQLLiteralFormatConfigFloat32(t *testing.T) {
	t.Parallel()
	negativeZero := float32(math.Copysign(0, -1))
	structValue := spanner.GenericColumnValue{
		Type: &sppb.Type{Code: sppb.TypeCode_STRUCT, StructType: &sppb.StructType{
			Fields: []*sppb.StructType_Field{{Name: "F", Type: &sppb.Type{Code: sppb.TypeCode_FLOAT32}}},
		}},
		Value: structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{structpb.NewNumberValue(math.Copysign(0, -1))}}),
	}
	for _, tc := range []struct {
		name  string
		value any
		want  string // Empty means preserve the upstream preset's existing bytes.
	}{
		{"negative zero", negativeZero, "CAST(-0.0 AS FLOAT32)"},
		{"nullable negative zero", spanner.NullFloat32{Float32: negativeZero, Valid: true}, "CAST(-0.0 AS FLOAT32)"},
		{"positive zero", float32(0), ""},
		{"integer", float32(3), ""},
		{"finite", float32(1.5), ""},
		{"NaN", float32(math.NaN()), ""},
		{"positive infinity", float32(math.Inf(1)), ""},
		{"negative infinity", float32(math.Inf(-1)), ""},
		{"NULL", spanner.NullFloat32{}, ""},
		{"FLOAT64 negative zero", math.Copysign(0, -1), ""},
		{"STRING with cast text", "CAST(-0 AS FLOAT32)", ""},
		{"JSON with cast text", spanner.NullJSON{Value: map[string]any{"text": "CAST(-0 AS FLOAT32)"}, Valid: true}, ""},
		{"array", []spanner.NullFloat32{{Float32: negativeZero, Valid: true}, {Valid: true}, {}}, "[CAST(-0.0 AS FLOAT32), CAST(0 AS FLOAT32), NULL]"},
		{"empty array", []spanner.NullFloat32{}, ""},
		{"NULL array", []spanner.NullFloat32(nil), ""},
		{"struct field", structValue, "STRUCT<F FLOAT32>(CAST(-0.0 AS FLOAT32))"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row, err := spanner.NewRow([]string{"V"}, []any{tc.value})
			if err != nil {
				t.Fatal(err)
			}
			var value spanner.GenericColumnValue
			if err := row.Column(0, &value); err != nil {
				t.Fatal(err)
			}
			want := tc.want
			if want == "" {
				want, err = spanvalue.LiteralFormatConfig().FormatToplevelColumn(value)
				if err != nil {
					t.Fatal(err)
				}
			}
			for _, mode := range []enums.DisplayMode{enums.DisplayModeSQLInsert, enums.DisplayModeSQLInsertOrIgnore, enums.DisplayModeSQLInsertOrUpdate} {
				sv := newSystemVariablesWithDefaults()
				sv.Display.CLIFormat = mode
				sv.Display.SQLTableName = "Values"
				streaming, _, _, err := prepareFormatConfig("SELECT V FROM Values", &sv)
				if err != nil {
					t.Fatal(err)
				}
				buffered, _, err := typedReplayFormatConfig(&sv)
				if err != nil {
					t.Fatal(err)
				}
				for name, config := range map[string]*spanvalue.FormatConfig{"streaming": streaming, "buffered": buffered} {
					got, err := config.FormatToplevelColumn(value)
					if err != nil {
						t.Fatal(err)
					}
					if got != want {
						t.Errorf("%s/%s got %s, want %s", mode, name, got, want)
					}
				}
			}
		})
	}
}

func TestSQLLiteralFormatConfigInvalidFloat32(t *testing.T) {
	t.Parallel()
	value := spanner.GenericColumnValue{
		Type: &sppb.Type{Code: sppb.TypeCode_FLOAT32}, Value: structpb.NewStringValue("not a float"),
	}
	sv := newSystemVariablesWithDefaults()
	sv.Display.CLIFormat = enums.DisplayModeSQLInsert
	config, _, err := typedReplayFormatConfig(&sv)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := config.FormatToplevelColumn(value); err == nil {
		t.Fatal("expected invalid FLOAT32 error")
	}
}
