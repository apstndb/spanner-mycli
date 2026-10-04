// Copyright 2026 apstndb
// SPDX-License-Identifier: Apache-2.0

package mycli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/memebridge"
	"github.com/cloudspannerecosystem/memefish/ast"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestParameterReferences(t *testing.T) {
	session := &Session{systemVariables: &systemVariables{Params: map[string]ast.Node{}}}
	set := func(name, value string) error {
		_, err := (&SetParamValueStatement{Name: name, Value: value}).Execute(t.Context(), session, OperationOutput{})
		return err
	}
	if err := set("source", "STRUCT<id INT64, nested STRUCT<name STRING>, absent ARRAY<INT64>>(42, STRUCT<name STRING>('Alice'), NULL)"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, expr string
		typ        sppb.TypeCode
	}{{"copy", "@source", sppb.TypeCode_STRUCT}, {"id", "@SOURCE.id", sppb.TypeCode_INT64}, {"name", "@source.nested.name", sppb.TypeCode_STRING}, {"absent", "@source.absent", sppb.TypeCode_ARRAY}} {
		if err := set(tc.name, tc.expr); err != nil {
			t.Fatal(err)
		}
		stmt, err := newStatement("SELECT @"+tc.name, session.systemVariables.Params, false)
		if err != nil {
			t.Fatal(err)
		}
		if got := stmt.Params[tc.name].(spanner.GenericColumnValue).Type.Code; got != tc.typ {
			t.Fatalf("type=%v want %v", got, tc.typ)
		}
	}
	frozen, err := newStatement("SELECT @copy", session.systemVariables.Params, false)
	if err != nil {
		t.Fatal(err)
	}
	old := frozen.Params["copy"].(spanner.GenericColumnValue)
	if err := set("source", "NULL"); err != nil {
		t.Fatal(err)
	}
	now, err := newStatement("SELECT @copy", session.systemVariables.Params, false)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(old.Value, now.Params["copy"].(spanner.GenericColumnValue).Value) {
		t.Fatal("copy followed source mutation")
	}
	for _, expr := range []string{"@missing", "@copy.missing", "@id.nope", "@id + 1", "ABS(@id)"} {
		if err := set("copy", expr); err == nil {
			t.Fatalf("accepted %s", expr)
		}
	}
	after, _ := newStatement("SELECT @copy", session.systemVariables.Params, false)
	if !proto.Equal(old.Value, after.Params["copy"].(spanner.GenericColumnValue).Value) {
		t.Fatal("failed assignment changed destination")
	}
	if err := set("null_struct", "CAST(NULL AS STRUCT<a INT64>)"); err != nil {
		t.Fatal(err)
	}
	if err := set("null_field", "@null_struct.a"); err != nil {
		t.Fatal(err)
	}
	null, _ := newStatement("SELECT @null_field", session.systemVariables.Params, false)
	v := null.Params["null_field"].(spanner.GenericColumnValue)
	_, isNull := v.Value.GetKind().(*structpb.Value_NullValue)
	if v.Type.Code != sppb.TypeCode_INT64 || !isNull {
		t.Fatal("lost typed NULL")
	}
}

func TestParameterReferenceWireCopies(t *testing.T) {
	for _, sql := range []string{
		"9007199254740993", "b'hello'", "ARRAY<INT64>[]",
		"CAST(NULL AS ARRAY<STRING>)", "STRUCT<Id INT64, Name STRING>(1, 'Alice')",
	} {
		t.Run(sql, func(t *testing.T) {
			var out bytes.Buffer
			cli := newDetachedEchoCli(t, &out)
			params := cli.SystemVariables.Params
			value, err := memebridge.ParseExprToGCV(sql)
			if err != nil {
				t.Fatal(err)
			}
			params["source"] = &boundParameter{value: cloneParameterValue(value)}
			set := &SetParamValueStatement{Name: "copy", Value: "@SOURCE"}
			if _, err := set.Execute(t.Context(), &Session{systemVariables: cli.SystemVariables}, OperationOutput{}); err != nil {
				t.Fatal(err)
			}
			// Mutating the source's backing messages must not change the copy.
			params["source"].(*boundParameter).value.Type.Code = sppb.TypeCode_BOOL
			params["source"].(*boundParameter).value.Value.Kind = structpb.NewBoolValue(true).Kind
			delete(params, "source")
			stmt, err := newStatement("SELECT @COPY", params, false)
			if err != nil {
				t.Fatal(err)
			}
			got := stmt.Params["COPY"].(spanner.GenericColumnValue)
			if !proto.Equal(value.Type, got.Type) || !proto.Equal(value.Value, got.Value) {
				t.Fatal("snapshot lost the original wire type/value")
			}
			// A generated request must not share mutable messages with storage.
			got.Type.Code = sppb.TypeCode_BOOL
			got.Value.Kind = structpb.NewBoolValue(false).Kind
			stored := params["copy"].(*boundParameter).value
			if !proto.Equal(value.Type, stored.Type) || !proto.Equal(value.Value, stored.Value) {
				t.Fatal("request mutation changed stored snapshot")
			}
			if _, err := cli.executeStatement(t.Context(), &ShowParamStatement{Name: "copy"}, false, "", &out); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(out.String(), "<bound") || strings.Contains(out.String(), "UNKNOWN") || !strings.Contains(out.String(), "VALUE") {
				t.Fatalf("snapshot inspection: %s", out.String())
			}
		})
	}
}

func TestParameterReferenceRejectsValuelessAndAmbiguousSources(t *testing.T) {
	typ, err := parseMemefishType("", "INT64")
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := parseMemefishExpr("", "STRUCT<Id INT64, id INT64>(1, 2)")
	if err != nil {
		t.Fatal(err)
	}
	params := map[string]ast.Node{"type_only": typ, "duplicate": duplicate, "dest": intParam("7"), "Alias": intParam("1"), "alias": intParam("2")}
	session := &Session{systemVariables: &systemVariables{Params: params}}
	before := params["dest"]
	for _, expr := range []string{"@type_only", "@duplicate.ID", "@ALIAS", "[@dest]", "STRUCT(@dest AS Id)"} {
		if _, err := (&SetParamValueStatement{Name: "dest", Value: expr}).Execute(t.Context(), session, OperationOutput{}); err == nil {
			t.Fatalf("accepted %s", expr)
		}
		if params["dest"] != before {
			t.Fatalf("failed assignment %s changed destination", expr)
		}
	}
}

func TestBoundParameterAliases(t *testing.T) {
	value, err := memebridge.ParseExprToGCV("STRUCT<Id INT64>(1)")
	if err != nil {
		t.Fatal(err)
	}
	params := map[string]ast.Node{
		"Saved": &boundParameter{value: value},
		"saved": &boundParameter{value: cloneParameterValue(value)},
	}
	if _, ok, err := lookupParam(params, "SAVED"); err != nil || !ok {
		t.Fatalf("identical snapshots were not accepted: %v", err)
	}
	params["saved"].(*boundParameter).value.Type.StructType.Fields[0].Name = "Other"
	if _, _, err := lookupParam(params, "SAVED"); !errors.Is(err, errAmbiguousQueryParameter) {
		t.Fatalf("different wire types were not rejected: %v", err)
	}
	params["saved"] = intParam("1")
	if _, _, err := lookupParam(params, "SAVED"); !errors.Is(err, errAmbiguousQueryParameter) {
		t.Fatalf("expression/snapshot aliases were not rejected: %v", err)
	}
}
