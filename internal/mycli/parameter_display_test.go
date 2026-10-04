// Copyright 2026 apstndb
// SPDX-License-Identifier: Apache-2.0

package mycli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/apstndb/memebridge"
	"github.com/apstndb/spanner-mycli/enums"
	"google.golang.org/protobuf/proto"
)

func TestParameterDisplayValue(t *testing.T) {
	for _, tc := range []struct{ name, sql, want string }{
		{"unnamed", "STRUCT<INT64>(1)", "(1)"},
		{"named", "STRUCT<Id INT64>(1)", "(1)"},
		{"nested", "STRUCT<child STRUCT<value INT64>>(STRUCT<value INT64>(1))", "((1))"},
		{"null", "CAST(NULL AS INT64)", "NULL"},
		{"null struct", "CAST(NULL AS STRUCT<Id INT64>)", "NULL"},
		{"empty array", "ARRAY<INT64>[]", "[]"},
		{"null array", "CAST(NULL AS ARRAY<INT64>)", "NULL"},
		{"precision", "9007199254740993", "9007199254740993"},
		{"string", "'hello'", "'hello'"},
		{"bytes", "B'hello'", "b'hello'"},
		{"named pair", "STRUCT<Id INT64, Name STRING>(1, 'Alice')", "(1, 'Alice')"},
		{"string null", "'NULL'", "'NULL'"},
		{"null string", "CAST(NULL AS STRING)", "NULL"},
		{"nested array", "ARRAY<STRUCT<Id INT64, Name STRING>>[(1, 'Alice'), (2, NULL)]", "[(1, 'Alice'), (2, NULL)]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value, err := memebridge.ParseExprToGCV(tc.sql)
			if err != nil {
				t.Fatal(err)
			}
			beforeType := proto.Clone(value.Type)
			beforeValue := proto.Clone(value.Value)
			got, err := parameterDisplayValue(value)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
			if !proto.Equal(beforeType, value.Type) || !proto.Equal(beforeValue, value.Value) {
				t.Fatal("display mutated stored parameter")
			}
		})
	}
}

// Inspection runs detached and must not expose values in the listing.
// Exercise the actual output path, including stream modes.
func TestShowParamInspection(t *testing.T) {
	for _, mode := range []enums.DisplayMode{enums.DisplayModeTable, enums.DisplayModeCSV, enums.DisplayModeJSONL} {
		t.Run(mode.String(), func(t *testing.T) {
			var out bytes.Buffer
			cli := newDetachedEchoCli(t, &out)
			sv := cli.SystemVariables
			sv.Display.CLIFormat = mode
			sv.Feature.EchoInput = false
			for name, sql := range map[string]string{"Saved": "STRUCT<Id INT64, Name STRING>(1, 'retained-secret')", "Secret": "'explicit-secret'", "NullValue": "CAST(NULL AS STRING)", "Unsupported": "ABS('unresolved-secret')"} {
				expr, err := parseMemefishExpr("", sql)
				if err != nil {
					t.Fatal(err)
				}
				sv.Params[name] = expr
			}
			typ, err := parseMemefishType("", "STRING")
			if err != nil {
				t.Fatal(err)
			}
			sv.Params["Unset"] = typ
			run := func(stmt Statement) string {
				t.Helper()
				out.Reset()
				if _, err := cli.executeStatement(t.Context(), stmt, false, "", &out); err != nil {
					t.Fatal(err)
				}
				return out.String()
			}
			listing := run(&ShowParamsStatement{})
			for _, secret := range []string{"retained-secret", "explicit-secret", "unresolved-secret"} {
				if strings.Contains(listing, secret) {
					t.Fatalf("listing leaked value: %s", listing)
				}
			}
			if !strings.Contains(listing, "UNKNOWN") {
				t.Fatalf("missing unresolved signature: %s", listing)
			}
			for _, tc := range []struct{ name, want string }{
				{"SAVED", "(1, 'retained-secret')"},
				{"secret", "'explicit-secret'"},
				{"nullvalue", "NULL"},
				{"unset", "<unset>"},
			} {
				got := run(&ShowParamStatement{Name: tc.name})
				if !strings.Contains(got, tc.want) {
					t.Fatalf("%s: missing %q: %s", tc.name, tc.want, got)
				}
			}
			sv.Params["SECRET"] = intParam("2")
			for _, name := range []string{"missing", "secret", "unsupported"} {
				if _, err := cli.executeStatement(t.Context(), &ShowParamStatement{Name: name}, false, "", &out); err == nil {
					t.Fatalf("accepted %s", name)
				}
			}
		})
	}
}
