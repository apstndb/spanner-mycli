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

package mycli

import (
	"context"
	"errors"
	"maps"
	"slices"
	"testing"

	"github.com/cloudspannerecosystem/memefish/ast"
	"github.com/google/go-cmp/cmp"
)

func TestSetParamStatementMalformedInput(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		stmt Statement
	}{
		{name: "type", stmt: &SetParamTypeStatement{Name: "p", Type: "'"}},
		{name: "value", stmt: &SetParamValueStatement{Name: "p", Value: "'"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			session := &Session{systemVariables: &systemVariables{Params: make(map[string]ast.Node)}}
			if _, err := tt.stmt.Execute(context.Background(), session, OperationOutput{}); err == nil {
				t.Fatal("Execute() error = nil, want malformed-input error")
			}
		})
	}
}

func intParam(value string) ast.Node {
	return &ast.IntLiteral{Base: 10, Value: value}
}

func TestSetParamKeepsFirstSpelling(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	session := &Session{systemVariables: &systemVariables{Params: make(map[string]ast.Node)}}

	if _, err := (&SetParamValueStatement{Name: "MixedCase", Value: "42"}).Execute(ctx, session, OperationOutput{}); err != nil {
		t.Fatal(err)
	}
	if _, err := (&SetParamValueStatement{Name: "mixedcase", Value: "99"}).Execute(ctx, session, OperationOutput{}); err != nil {
		t.Fatal(err)
	}

	got := session.systemVariables.Params
	if _, ok := got["mixedcase"]; ok {
		t.Fatalf("updated map kept later spelling: %v", slices.Sorted(maps.Keys(got)))
	}
	node, ok := got["MixedCase"]
	if !ok {
		t.Fatalf("first spelling missing: %v", slices.Sorted(maps.Keys(got)))
	}
	if node.SQL() != "99" {
		t.Fatalf("stored SQL() = %q, want 99", node.SQL())
	}
}

func TestSetParamTypeUpdatesLogicalName(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	session := &Session{systemVariables: &systemVariables{Params: make(map[string]ast.Node)}}

	if _, err := (&SetParamValueStatement{Name: "MixedCase", Value: "42"}).Execute(ctx, session, OperationOutput{}); err != nil {
		t.Fatal(err)
	}
	if _, err := (&SetParamTypeStatement{Name: "mixedcase", Type: "INT64"}).Execute(ctx, session, OperationOutput{}); err != nil {
		t.Fatal(err)
	}

	node, ok := session.systemVariables.Params["MixedCase"]
	if !ok {
		t.Fatalf("type SET lost first spelling: %v", session.systemVariables.Params)
	}
	if paramKind(node) != "TYPE" || node.SQL() != "INT64" {
		t.Fatalf("stored %s %s, want TYPE INT64", paramKind(node), node.SQL())
	}
}

func TestUnsetParamCaseInsensitive(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	session := &Session{systemVariables: &systemVariables{Params: map[string]ast.Node{
		"MixedCase": intParam("1"),
	}}}

	if _, err := (&UnsetParamStatement{Name: "mixedcase"}).Execute(ctx, session, OperationOutput{}); err != nil {
		t.Fatal(err)
	}
	if len(session.systemVariables.Params) != 0 {
		t.Fatalf("UNSET left params: %v", session.systemVariables.Params)
	}
}

func TestUnsetParamUnknown(t *testing.T) {
	t.Parallel()
	session := &Session{systemVariables: &systemVariables{Params: make(map[string]ast.Node)}}
	_, err := (&UnsetParamStatement{Name: "missing"}).Execute(t.Context(), session, OperationOutput{})
	if err == nil || err.Error() != "unknown parameter: missing" {
		t.Fatalf("error = %v, want unknown parameter: missing", err)
	}
}

func TestLookupParamIdenticalAliases(t *testing.T) {
	t.Parallel()
	params := map[string]ast.Node{
		"MixedCase": intParam("1"),
		"mixedcase": intParam("1"),
	}
	node, ok, err := lookupParam(params, "MIXEDCASE")
	if err != nil || !ok {
		t.Fatalf("lookupParam() ok=%v err=%v", ok, err)
	}
	if node.SQL() != "1" {
		t.Fatalf("SQL() = %q, want 1", node.SQL())
	}
	if err := checkParamMapAmbiguity(params); err != nil {
		t.Fatalf("identical aliases rejected: %v", err)
	}
}

func TestLookupParamConflictingAliases(t *testing.T) {
	t.Parallel()
	params := map[string]ast.Node{
		"MixedCase": intParam("1"),
		"mixedcase": intParam("2"),
	}
	_, _, err := lookupParam(params, "mixedcase")
	if !errors.Is(err, errAmbiguousQueryParameter) {
		t.Fatalf("error = %v, want errAmbiguousQueryParameter", err)
	}
	var amb *ambiguousQueryParameterError
	if !errors.As(err, &amb) {
		t.Fatalf("error = %v, want ambiguousQueryParameterError", err)
	}
	if diff := cmp.Diff([]string{"MixedCase", "mixedcase"}, amb.Aliases); diff != "" {
		t.Fatalf("Aliases mismatch (-want +got):\n%s", diff)
	}
}

func TestLookupParamRejectsInventedSemanticEquality(t *testing.T) {
	t.Parallel()
	castExpr, err := parseMemefishExpr("", "CAST(1 AS INT64)")
	if err != nil {
		t.Fatal(err)
	}
	params := map[string]ast.Node{
		"MixedCase": intParam("1"),
		"mixedcase": castExpr,
	}
	if paramNodeIdentity(params["MixedCase"]) == paramNodeIdentity(params["mixedcase"]) {
		t.Fatal("setup: INT64 1 and CAST(1 AS INT64) collapsed to the same identity")
	}
	_, _, err = lookupParam(params, "mixedcase")
	if !errors.Is(err, errAmbiguousQueryParameter) {
		t.Fatalf("error = %v, want errAmbiguousQueryParameter", err)
	}
}

func TestSetParamRejectsConflictingPreexistingAliases(t *testing.T) {
	t.Parallel()
	params := map[string]ast.Node{
		"MixedCase": intParam("1"),
		"mixedcase": intParam("2"),
	}
	err := setParam(params, "MIXEDCASE", intParam("3"))
	if !errors.Is(err, errAmbiguousQueryParameter) {
		t.Fatalf("error = %v, want errAmbiguousQueryParameter", err)
	}
	if params["MixedCase"].SQL() != "1" || params["mixedcase"].SQL() != "2" {
		t.Fatalf("conflicting SET mutated map: %v", params)
	}
}

func TestSetParamCollapsesIdenticalAliasesDeterministically(t *testing.T) {
	t.Parallel()
	params := map[string]ast.Node{
		"mixedcase": intParam("1"),
		"MixedCase": intParam("1"),
	}
	if err := setParam(params, "MIXEDCASE", intParam("3")); err != nil {
		t.Fatal(err)
	}
	if len(params) != 1 {
		t.Fatalf("identical aliases not collapsed: %v", params)
	}
	node, ok := params["MixedCase"]
	if !ok {
		t.Fatalf("kept spelling = %v, want lexicographically first MixedCase", slices.Sorted(maps.Keys(params)))
	}
	if node.SQL() != "3" {
		t.Fatalf("stored SQL() = %q, want 3", node.SQL())
	}
}

func TestUnsetParamIdenticalAliases(t *testing.T) {
	t.Parallel()
	params := map[string]ast.Node{
		"MixedCase": intParam("1"),
		"mixedcase": intParam("1"),
	}
	if err := unsetParam(params, "MIXEDCASE"); err != nil {
		t.Fatal(err)
	}
	if len(params) != 0 {
		t.Fatalf("UNSET left identical aliases: %v", params)
	}
}

func TestCheckParamMapAmbiguityDeterministic(t *testing.T) {
	t.Parallel()
	params := map[string]ast.Node{
		"zzz": intParam("1"),
		"ZZZ": intParam("2"),
		"aaa": intParam("3"),
		"AAA": intParam("4"),
	}
	err := checkParamMapAmbiguity(params)
	if !errors.Is(err, errAmbiguousQueryParameter) {
		t.Fatalf("error = %v, want errAmbiguousQueryParameter", err)
	}
	var amb *ambiguousQueryParameterError
	if !errors.As(err, &amb) {
		t.Fatalf("error = %v, want ambiguousQueryParameterError", err)
	}
	if amb.Name != "AAA" {
		t.Fatalf("Name = %q, want first sorted group AAA", amb.Name)
	}
	if diff := cmp.Diff([]string{"AAA", "aaa"}, amb.Aliases); diff != "" {
		t.Fatalf("Aliases mismatch (-want +got):\n%s", diff)
	}
}
