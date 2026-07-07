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

package bigquery

import (
	"math/big"
	"testing"
	"time"

	bq "cloud.google.com/go/bigquery"
	"cloud.google.com/go/civil"
)

func TestFormatBigQueryValue(t *testing.T) {
	t.Parallel()

	ts := time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)

	for _, tt := range []struct {
		name      string
		in        bq.Value
		fieldType bq.FieldType
		want      string
	}{
		{name: "nil", in: nil, want: "NULL"},
		{name: "string", in: "hello", want: "hello"},
		{name: "bool true", in: true, want: "true"},
		{name: "bool false", in: false, want: "false"},
		{name: "int64", in: int64(42), want: "42"},
		{name: "float64", in: float64(3.14), want: "3.14"},
		{name: "bytes", in: []byte{0xde, 0xad}, want: "3q0="},
		{name: "timestamp", in: ts, want: ts.Format(time.RFC3339Nano)},
		{name: "date", in: civil.Date{Year: 2024, Month: 3, Day: 15}, want: "2024-03-15"},
		{name: "numeric", in: big.NewRat(1, 2), fieldType: bq.NumericFieldType, want: "0.500000000"},
		{name: "rat default scale", in: big.NewRat(1, 2), want: "0.500000000"},
		{name: "bignumeric", in: big.NewRat(1, 3), fieldType: bq.BigNumericFieldType, want: "0.33333333333333333333333333333333333333"},
		{name: "array", in: []bq.Value{"a", int64(1)}, want: `["a",1]`},
		{name: "record", in: map[string]bq.Value{"k": "v"}, want: `{"k":"v"}`},
		{name: "null string", in: bq.NullString{}, want: "NULL"},
		{name: "valid null string", in: bq.NullString{StringVal: "x", Valid: true}, want: "x"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := formatBigQueryValue(tt.in, tt.fieldType); got != tt.want {
				t.Fatalf("formatBigQueryValue() = %q, want %q", got, tt.want)
			}
		})
	}
}
