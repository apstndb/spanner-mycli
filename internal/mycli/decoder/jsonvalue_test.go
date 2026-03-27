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
	"github.com/apstndb/spanvalue/gcvctor"
	"github.com/google/go-cmp/cmp"
	"github.com/samber/lo"
)

// TestJSONFormatConfig verifies delegation to spanvalue.JSONFormatConfig()
// with representative cases for the JSONL pipeline contract.
// Comprehensive type coverage is in spanvalue's own tests.
func TestJSONFormatConfig(t *testing.T) {
	t.Parallel()

	fc := JSONFormatConfig()

	tests := []struct {
		name     string
		gcv      spanner.GenericColumnValue
		wantJSON string
	}{
		{name: "INT64 as number", gcv: gcvctor.Int64Value(42), wantJSON: "42"},
		{name: "NULL", gcv: gcvctor.SimpleTypedNull(sppb.TypeCode_STRING), wantJSON: "null"},
		{name: "STRING quoted", gcv: gcvctor.StringValue("hello"), wantJSON: `"hello"`},
		{name: "BOOL", gcv: gcvctor.BoolValue(true), wantJSON: "true"},
		{name: "JSON pass-through", gcv: lo.Must(gcvctor.JSONValue(map[string]string{"k": "v"})), wantJSON: `{"k":"v"}`},
		{name: "ARRAY of INT64", gcv: lo.Must(gcvctor.ArrayValue(gcvctor.Int64Value(1), gcvctor.Int64Value(2))), wantJSON: "[1,2]"},
		{name: "STRUCT", gcv: lo.Must(gcvctor.StructValue([]string{"name", "age"}, []spanner.GenericColumnValue{gcvctor.StringValue("Alice"), gcvctor.Int64Value(30)})), wantJSON: `{"name":"Alice","age":30}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := fc.FormatToplevelColumn(tt.gcv)
			if err != nil {
				t.Fatalf("FormatToplevelColumn() error = %v", err)
			}
			if diff := cmp.Diff(tt.wantJSON, got); diff != "" {
				t.Errorf("output mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
