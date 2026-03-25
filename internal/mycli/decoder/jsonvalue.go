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
	"math"
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
func formatJSONSimpleValue(_ spanvalue.Formatter, value spanner.GenericColumnValue, _ bool) (string, error) {
	val := value.Value
	typ := value.Type

	// Handle NULL
	if _, isNull := val.GetKind().(*structpb.Value_NullValue); isNull {
		return "null", nil
	}

	switch typ.GetCode() {
	case sppb.TypeCode_BOOL:
		if val.GetBoolValue() {
			return "true", nil
		}
		return "false", nil

	case sppb.TypeCode_INT64:
		// Spanner encodes INT64 as string in proto; the string IS a valid JSON number
		return val.GetStringValue(), nil

	case sppb.TypeCode_FLOAT32, sppb.TypeCode_FLOAT64:
		switch v := val.GetKind().(type) {
		case *structpb.Value_NumberValue:
			f := v.NumberValue
			if math.IsNaN(f) || math.IsInf(f, 0) {
				// JSON doesn't support NaN/Inf; quote as string
				b, _ := json.Marshal(strconv.FormatFloat(f, 'g', -1, 64))
				return string(b), nil
			}
			return strconv.FormatFloat(f, 'f', -1, 64), nil
		case *structpb.Value_StringValue:
			// "NaN", "Infinity", "-Infinity" - quote as JSON string
			b, _ := json.Marshal(v.StringValue)
			return string(b), nil
		default:
			b, _ := json.Marshal(val.GetStringValue())
			return string(b), nil
		}

	case sppb.TypeCode_JSON:
		// JSON column value is already valid JSON - pass through as-is
		return val.GetStringValue(), nil

	default:
		// STRING, BYTES, TIMESTAMP, DATE, NUMERIC, ENUM, PROTO, INTERVAL, UUID
		// All are string values in the proto; quote as JSON strings
		b, _ := json.Marshal(val.GetStringValue())
		return string(b), nil
	}
}
