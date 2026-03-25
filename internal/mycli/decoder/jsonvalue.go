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
	"encoding/json"
	"strconv"
	"strings"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanvalue"
	"google.golang.org/protobuf/types/known/structpb"
)

// JSONFormatConfig returns a spanvalue.FormatConfig that produces valid JSON value strings.
// Each cell text produced by this config is a standalone JSON value:
//   - NULL → null
//   - BOOL → true / false
//   - INT64 → 42 (unquoted number)
//   - FLOAT32/FLOAT64 → 3.14 (NaN/Inf as quoted strings)
//   - STRING, BYTES, TIMESTAMP, DATE, etc. → "quoted string"
//   - JSON column → raw JSON value (passed through)
//   - ARRAY → [elem1,elem2,...]
//   - STRUCT → {"field1":val1,"field2":val2,...}
func JSONFormatConfig() *spanvalue.FormatConfig {
	return &spanvalue.FormatConfig{
		NullString:  "null",
		FormatArray: formatJSONArray,
		FormatStruct: spanvalue.FormatStruct{
			FormatStructField: spanvalue.FormatSimpleStructField,
			FormatStructParen: formatJSONStructParen,
		},
		FormatComplexPlugins: []spanvalue.FormatComplexFunc{
			formatJSONSimpleValue,
		},
		FormatNullable: spanvalue.FormatNullableSpannerCLICompatible,
	}
}

// formatJSONArray formats array elements as a JSON array.
// elemStrings are already JSON-formatted by recursive FormatColumn calls.
func formatJSONArray(_ *sppb.Type, _ bool, elemStrings []string) string {
	return "[" + strings.Join(elemStrings, ",") + "]"
}

// formatJSONStructParen formats struct fields as a JSON object with field names as keys.
// fieldStrings are already JSON-formatted values from FormatSimpleStructField.
func formatJSONStructParen(typ *sppb.Type, _ bool, fieldStrings []string) string {
	fields := typ.GetStructType().GetFields()
	parts := make([]string, len(fieldStrings))
	for i, valStr := range fieldStrings {
		name := fields[i].GetName()
		if name == "" {
			name = "f" + strconv.Itoa(i)
		}
		keyJSON, _ := json.Marshal(name)
		parts[i] = string(keyJSON) + ":" + valStr
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// formatJSONSimpleValue handles all non-ARRAY, non-STRUCT types for JSON output.
// It never returns ErrFallthrough, so it handles every simple type.
//
// For most types, structpb.Value.MarshalJSON() produces the correct JSON representation
// (BOOL→true/false, FLOAT→number, STRING→"quoted", NULL→null, NaN/Inf→"NaN"/"Infinity").
// Only INT64 and JSON columns need special handling:
//   - INT64: Spanner encodes as StringValue("42"), MarshalJSON() would produce "42" (quoted),
//     but we want 42 (unquoted number).
//   - JSON: Spanner encodes as StringValue('{"key":"value"}'), MarshalJSON() would produce
//     escaped quoted string, but we want the raw JSON value passed through.
func formatJSONSimpleValue(_ spanvalue.Formatter, value spanner.GenericColumnValue, _ bool) (string, error) {
	val := value.Value

	if _, isNull := val.GetKind().(*structpb.Value_NullValue); isNull {
		return "null", nil
	}

	switch value.Type.GetCode() {
	case sppb.TypeCode_INT64, sppb.TypeCode_JSON:
		// INT64: StringValue is already a valid JSON number
		// JSON column: StringValue is already valid JSON
		return val.GetStringValue(), nil

	default:
		// For all other types, structpb.Value's JSON marshaling matches our needs
		b, err := val.MarshalJSON()
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
}
