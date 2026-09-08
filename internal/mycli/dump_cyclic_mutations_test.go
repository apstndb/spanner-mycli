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
	"errors"
	"math"
	"strings"
	"testing"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestEncodeDumpMutationRow(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		value any
	}{
		{"int", int64(42)},
		{"bool", true},
		{"string", "CAST(-0 AS FLOAT32)"},
		{"bytes", []byte{0, 1, 255}},
		{"float32 negative zero", float32(math.Copysign(0, -1))},
		{"float64 negative zero", math.Copysign(0, -1)},
		{"float32 array", []spanner.NullFloat32{{Float32: float32(math.Copysign(0, -1)), Valid: true}, {Valid: true}, {Float32: 1.5, Valid: true}, {}}},
		{"NULL scalar", spanner.NullInt64{}},
		{"empty array", []spanner.NullInt64{}},
		{"NULL array", []spanner.NullInt64(nil)},
		{"proto surrogate", spanner.GenericColumnValue{Type: &sppb.Type{Code: sppb.TypeCode_PROTO, ProtoTypeFqn: "example.Message"}, Value: structpb.NewStringValue("CAc=")}},
		{"enum surrogate", spanner.GenericColumnValue{Type: &sppb.Type{Code: sppb.TypeCode_ENUM, ProtoTypeFqn: "example.Kind"}, Value: structpb.NewStringValue("3")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row, err := spanner.NewRow([]string{"Order"}, []any{tc.value})
			if err != nil {
				t.Fatal(err)
			}
			originalType := proto.Clone(row.ColumnType(0))
			originalValue := proto.Clone(row.ColumnValue(0))
			text, err := encodeDumpMutationRow(tidn("Select", "Table"), row, dumpMutationFormatConfig())
			if err != nil {
				t.Fatal(err)
			}
			stmt, err := BuildStatement(strings.TrimSuffix(text, ";\n"))
			if err != nil {
				t.Fatalf("parse %s: %v", text, err)
			}
			mutation, ok := stmt.(*MutateStatement)
			if !ok {
				t.Fatalf("got %T, want MUTATE", stmt)
			}
			columns, values, err := parseLiteralString(mutation.Body)
			if err != nil {
				t.Fatalf("parse body %s: %v", mutation.Body, err)
			}
			if len(columns) != 1 || columns[0] != "Order" || len(values) != 1 || len(values[0]) != 1 {
				t.Fatalf("unexpected parsed row: %v %v", columns, values)
			}
			if !proto.Equal(originalValue, values[0][0].Value) {
				t.Errorf("wire value changed: %v -> %v", originalValue, values[0][0].Value)
			}
			if !proto.Equal(originalType, row.ColumnType(0)) || !proto.Equal(originalValue, row.ColumnValue(0)) {
				t.Fatal("encoder mutated source row")
			}
			if strings.Contains(tc.name, "negative zero") && !math.Signbit(values[0][0].Value.GetNumberValue()) {
				t.Fatal("negative sign lost")
			}
			if tc.name == "float32 array" && !math.Signbit(values[0][0].Value.GetListValue().Values[0].GetNumberValue()) {
				t.Fatal("array negative sign lost")
			}
		})
	}
}

func TestEncodeDumpMutationRowMalformed(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		typ   *sppb.Type
		value *structpb.Value
	}{
		{"unknown NULL", &sppb.Type{Code: sppb.TypeCode(999)}, structpb.NewNullValue()},
		{"STRUCT NULL", &sppb.Type{Code: sppb.TypeCode_STRUCT}, structpb.NewNullValue()},
		{"ARRAY without element NULL", &sppb.Type{Code: sppb.TypeCode_ARRAY}, structpb.NewNullValue()},
		{"nil type", nil, structpb.NewNullValue()},
		{"nil value", &sppb.Type{Code: sppb.TypeCode_INT64}, nil},
		{"int", &sppb.Type{Code: sppb.TypeCode_INT64}, structpb.NewStringValue("x")},
		{"float", &sppb.Type{Code: sppb.TypeCode_FLOAT32}, structpb.NewStringValue("x")},
		{"numeric", &sppb.Type{Code: sppb.TypeCode_NUMERIC}, structpb.NewStringValue("x")},
		{"date", &sppb.Type{Code: sppb.TypeCode_DATE}, structpb.NewStringValue("not a date")},
		{"timestamp", &sppb.Type{Code: sppb.TypeCode_TIMESTAMP}, structpb.NewStringValue("not a timestamp")},
		{"JSON", &sppb.Type{Code: sppb.TypeCode_JSON}, structpb.NewStringValue("{")},
		{"UUID", &sppb.Type{Code: sppb.TypeCode_UUID}, structpb.NewStringValue("bad uuid")},
		{"PROTO base64", &sppb.Type{Code: sppb.TypeCode_PROTO, ProtoTypeFqn: "p.M"}, structpb.NewStringValue("*")},
		{"PROTO descriptor", &sppb.Type{Code: sppb.TypeCode_PROTO}, structpb.NewStringValue("CAc=")},
		{"ENUM number", &sppb.Type{Code: sppb.TypeCode_ENUM, ProtoTypeFqn: "p.E"}, structpb.NewStringValue("x")},
		{"ARRAY shape", &sppb.Type{Code: sppb.TypeCode_ARRAY, ArrayElementType: &sppb.Type{Code: sppb.TypeCode_INT64}}, structpb.NewStringValue("[]")},
		{"annotated", &sppb.Type{Code: sppb.TypeCode_NUMERIC, TypeAnnotation: sppb.TypeAnnotationCode_PG_NUMERIC}, structpb.NewNullValue()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row, err := spanner.NewRow([]string{"V"}, []any{spanner.GenericColumnValue{Type: tc.typ, Value: tc.value}})
			if err != nil {
				t.Fatal(err)
			}
			if text, err := encodeDumpMutationRow(tid("T"), row, dumpMutationFormatConfig()); err == nil || text != "" {
				t.Fatalf("got %q, %v; want error and no statement", text, err)
			}
		})
	}
}

func TestDumpCyclicBudget(t *testing.T) {
	t.Parallel()
	for _, limit := range []int64{1, 64 << 20, math.MaxInt64} {
		budget := &dumpCyclicBudget{limit: limit, used: limit - 1}
		if err := budget.retain(1); err != nil {
			t.Fatal(err)
		}
		if err := budget.retain(1); err == nil {
			t.Fatal("over-limit retention succeeded")
		}
		if budget.used != limit {
			t.Fatal("failed reservation changed usage")
		}
	}
	for _, limit := range []int64{0, -1} {
		if err := (&dumpCyclicBudget{limit: limit}).retain(0); err == nil {
			t.Fatal("nonpositive cap accepted")
		}
	}
}

func TestDumpCyclicVariables(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, defaultValue, value string
		invalid                   []string
	}{
		{"CLI_DUMP_CYCLIC_MODE", "REJECT", "MUTATE", []string{"other", "0"}},
		{"CLI_DUMP_CYCLIC_MAX_BYTES", "67108864", "1", []string{"0", "-1", "9223372036854775808"}},
	} {
		for _, end := range []struct {
			name      string
			statement Statement
		}{{"commit", &CommitStatement{}}, {"rollback", &RollbackStatement{}}} {
			t.Run(tc.name+"/"+end.name, func(t *testing.T) {
				session := newSessionForLocalVarTest(t)
				if got := mustGetVar(t, session, tc.name); got != tc.defaultValue {
					t.Fatalf("default = %s", got)
				}
				for _, bad := range tc.invalid {
					if err := session.systemVariables.SetFromSimple(tc.name, bad); err == nil {
						t.Errorf("accepted %s", bad)
					}
				}
				if got := mustGetVar(t, session, tc.name); got != tc.defaultValue {
					t.Fatal("invalid SET changed value")
				}
				if _, err := session.ExecuteStatement(t.Context(), &BeginStatement{}); err != nil {
					t.Fatal(err)
				}
				if _, err := session.ExecuteStatement(t.Context(), &SetLocalStatement{VarName: tc.name, Value: tc.value}); err != nil {
					t.Fatal(err)
				}
				if got := mustGetVar(t, session, tc.name); got != tc.value {
					t.Fatalf("SET LOCAL got %s", got)
				}
				if _, err := session.ExecuteStatement(t.Context(), end.statement); err != nil {
					t.Fatal(err)
				}
				if got := mustGetVar(t, session, tc.name); got != tc.defaultValue {
					t.Fatalf("restore got %s", got)
				}
			})
		}
	}
}

func TestDumpCyclicSafetyHelp(t *testing.T) {
	t.Parallel()
	for _, want := range []string{"never automatically split", "No prediction", "80,000", "100 MiB", "earlier groups, DDL and ordinary INSERTs may remain committed"} {
		if !strings.Contains(dumpCyclicWarning, want) {
			t.Errorf("cyclic output warning omits %q", want)
		}
	}
	found := 0
	for _, def := range MergedStatementDefs() {
		for _, description := range def.Descriptions {
			if description.Syntax != "DUMP DATABASE" && description.Syntax != "DUMP TABLES <table1> [, <table2>, ...]" {
				continue
			}
			found++
			for _, want := range []string{"CLI_DUMP_CYCLIC_MODE defaults to REJECT", "opt-in MUTATE", "unsplit transaction", "No service-quota prediction", "may remain committed"} {
				if !strings.Contains(strings.ToLower(description.Note), strings.ToLower(want)) {
					t.Errorf("%s help omits %q", description.Syntax, want)
				}
			}
		}
	}
	if found != 2 {
		t.Fatalf("found %d DUMP data help entries, want 2", found)
	}
}

type dumpFailWriter struct{ err error }

func (w dumpFailWriter) Write([]byte) (int, error) { return 0, w.err }

func TestDumpCyclicDataWriteError(t *testing.T) {
	t.Parallel()
	want := errors.New("output failed")
	if err := (&dumpCyclicData{Statements: []string{"MUTATE T INSERT STRUCT<Id INT64>(1);\n"}}).writeTo(dumpFailWriter{want}); !errors.Is(err, want) {
		t.Fatalf("got %v", err)
	}
}
