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
	"errors"
	"math/big"
	"testing"
	"time"

	bq "cloud.google.com/go/bigquery"
	"cloud.google.com/go/civil"
)

func TestFormatBigQueryValue(t *testing.T) {
	t.Parallel()

	ts := time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)
	tsNano := time.Date(2024, 3, 15, 10, 30, 0, 123456789, time.UTC)
	date := civil.Date{Year: 2024, Month: 3, Day: 15}
	civTime := civil.Time{Hour: 13, Minute: 4, Second: 5}
	civTimeNano := civil.Time{Hour: 13, Minute: 4, Second: 5, Nanosecond: 123}
	dateTime := civil.DateTime{Date: date, Time: civil.Time{Hour: 10, Minute: 30, Second: 0}}

	for _, tt := range []struct {
		name      string
		in        bq.Value
		fieldType bq.FieldType
		want      string
	}{
		{name: "nil", in: nil, want: "NULL"},
		{name: "string", in: "hello", want: "hello"},
		{name: "empty string", in: "", want: ""},
		{name: "bool true", in: true, want: "true"},
		{name: "bool false", in: false, want: "false"},
		{name: "int", in: int(7), want: "7"},
		{name: "int64", in: int64(42), want: "42"},
		{name: "int64 zero", in: int64(0), want: "0"},
		{name: "float64", in: float64(3.14), want: "3.14"},
		{name: "float64 zero", in: float64(0), want: "0"},
		{name: "float32", in: float32(1.5), want: "1.5"},
		{name: "bytes", in: []byte{0xde, 0xad}, want: "3q0="},
		{name: "empty bytes", in: []byte{}, want: ""},
		{name: "timestamp", in: ts, want: "2024-03-15T10:30:00Z"},
		{name: "timestamp nanos", in: tsNano, want: "2024-03-15T10:30:00.123456789Z"},
		{name: "zero timestamp", in: time.Time{}, want: "0001-01-01T00:00:00Z"},
		{name: "date", in: date, want: "2024-03-15"},
		{name: "zero date", in: civil.Date{}, want: "0000-00-00"},
		{name: "civil time", in: civTime, want: "13:04:05"},
		{name: "civil time nanos", in: civTimeNano, want: "13:04:05.000000123"},
		{name: "civil datetime", in: dateTime, want: "2024-03-15T10:30:00"},
		{name: "numeric", in: big.NewRat(1, 2), fieldType: bq.NumericFieldType, want: "0.500000000"},
		{name: "rat default scale", in: big.NewRat(1, 2), want: "0.500000000"},
		{name: "numeric one third", in: big.NewRat(1, 3), fieldType: bq.NumericFieldType, want: "0.333333333"},
		{name: "bignumeric", in: big.NewRat(1, 3), fieldType: bq.BigNumericFieldType, want: "0.33333333333333333333333333333333333333"},
		{name: "numeric zero", in: big.NewRat(0, 1), fieldType: bq.NumericFieldType, want: "0.000000000"},
		{name: "bignumeric zero", in: big.NewRat(0, 1), fieldType: bq.BigNumericFieldType, want: "0.00000000000000000000000000000000000000"},
		{name: "nil rat", in: (*big.Rat)(nil), want: "NULL"},
		{name: "array", in: []bq.Value{"a", int64(1)}, want: `["a",1]`},
		{name: "array with null", in: []bq.Value{"a", nil}, want: `["a",null]`},
		{name: "empty array", in: []bq.Value{}, want: `[]`},
		{name: "record", in: map[string]bq.Value{"k": "v"}, want: `{"k":"v"}`},
		{name: "record with null", in: map[string]bq.Value{"k": nil}, want: `{"k":null}`},
		{name: "empty record", in: map[string]bq.Value{}, want: `{}`},
		{name: "json encode error fallback", in: jsonEncodeError{}, want: "{}"},

		{name: "null string invalid", in: bq.NullString{}, want: "NULL"},
		{name: "null string valid", in: bq.NullString{StringVal: "x", Valid: true}, want: "x"},
		{name: "null string empty valid", in: bq.NullString{StringVal: "", Valid: true}, want: ""},
		{name: "null int64 invalid", in: bq.NullInt64{}, want: "NULL"},
		{name: "null int64 zero valid", in: bq.NullInt64{Int64: 0, Valid: true}, want: "0"},
		{name: "null int64 valid", in: bq.NullInt64{Int64: 42, Valid: true}, want: "42"},
		{name: "null float64 invalid", in: bq.NullFloat64{}, want: "NULL"},
		{name: "null float64 zero valid", in: bq.NullFloat64{Float64: 0, Valid: true}, want: "0"},
		{name: "null float64 valid", in: bq.NullFloat64{Float64: 3.14, Valid: true}, want: "3.14"},
		{name: "null bool invalid", in: bq.NullBool{}, want: "NULL"},
		{name: "null bool false valid", in: bq.NullBool{Bool: false, Valid: true}, want: "false"},
		{name: "null bool true valid", in: bq.NullBool{Bool: true, Valid: true}, want: "true"},
		{name: "null timestamp invalid", in: bq.NullTimestamp{}, want: "NULL"},
		{name: "null timestamp valid", in: bq.NullTimestamp{Timestamp: tsNano, Valid: true}, want: "2024-03-15T10:30:00.123456789Z"},
		{name: "null date invalid", in: bq.NullDate{}, want: "NULL"},
		{name: "null date valid", in: bq.NullDate{Date: date, Valid: true}, want: "2024-03-15"},
		{name: "null time invalid", in: bq.NullTime{}, want: "NULL"},
		{name: "null time valid", in: bq.NullTime{Time: civTime, Valid: true}, want: "13:04:05"},
		{name: "null datetime invalid", in: bq.NullDateTime{}, want: "NULL"},
		{name: "null datetime valid", in: bq.NullDateTime{DateTime: dateTime, Valid: true}, want: "2024-03-15T10:30:00"},
		{name: "null json invalid", in: bq.NullJSON{}, want: "NULL"},
		{name: "null json valid", in: bq.NullJSON{JSONVal: `{"a":1}`, Valid: true}, want: `{"a":1}`},
		{name: "null json empty valid", in: bq.NullJSON{JSONVal: "", Valid: true}, want: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := formatBigQueryValue(tt.in, tt.fieldType); got != tt.want {
				t.Fatalf("formatBigQueryValue() = %q, want %q", got, tt.want)
			}
		})
	}
}

// jsonEncodeError is a value json.Marshal cannot encode, so formatBigQueryValue
// falls back to fmt.Sprint. The display string of the zero value is "{}".
type jsonEncodeError struct{}

func (jsonEncodeError) MarshalJSON() ([]byte, error) {
	return nil, errors.New("forced json encode error")
}
