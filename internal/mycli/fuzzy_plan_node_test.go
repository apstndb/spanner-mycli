// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
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
	"reflect"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	readline "github.com/nyaosorg/go-readline-ny"
)

func TestPlanNodeCompletionContext(t *testing.T) {
	for _, tt := range []struct{ input, prefix string }{
		{"SHOW PLAN NODE ", ""},
		{"show plan node 12", "12"},
		{"  SHOW\tPLAN\tNODE\t3", "3"},
		{"\nSHOW PLAN NODE 0", "0"},
	} {
		t.Run(tt.input, func(t *testing.T) {
			got := detectFuzzyContext(tt.input)
			if got.completionType != fuzzyCompletePlanNode || got.argPrefix != tt.prefix || got.suffix != "" || got.argStartPos != utf8.RuneCountInString(strings.TrimSuffix(tt.input, tt.prefix)) {
				t.Fatalf("context = %+v", got)
			}
		})
	}
	for _, input := range []string{"SHOW PLAN NODE -1", "SHOW PLAN NODE abc", "SHOW PLAN NODE 1;", "SELECT 'SHOW PLAN NODE 1'"} {
		if got := detectFuzzyContext(input); got.completionType == fuzzyCompletePlanNode {
			t.Fatalf("unexpected plan completion for %q", input)
		}
	}
	if requiresNetwork(fuzzyCompletePlanNode) || fuzzyCompletePlanNode.String() != "plan_node" || completionHeader(fuzzyCompletePlanNode) != "Cached Plan Nodes" {
		t.Fatal("plan completion registration mismatch")
	}
}

func TestPlanNodeCompletionEmptyOrCancelledEditor(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		sv := newSystemVariablesWithDefaults()
		f := &fuzzyFinderCommand{cli: &Cli{SystemVariables: &sv}}
		ctx, cancel := context.WithCancel(t.Context())
		if cancelled {
			cancel()
			sv.LastResult.QueryCache = &LastQueryCache{QueryPlan: &sppb.QueryPlan{PlanNodes: []*sppb.PlanNode{{}}}}
		}
		b := &readline.Buffer{Editor: &readline.Editor{}}
		input := "SHOW PLAN NODE "
		b.Cursor = b.InsertString(0, input)
		result := f.Call(ctx, b)
		cancel()
		if result != readline.CONTINUE || b.String() != input || b.Cursor != len(input) {
			t.Fatalf("cancelled=%v: result=%v buffer=%q cursor=%d", cancelled, result, b.String(), b.Cursor)
		}
	}
}

func TestPlanNodeCompletionCandidates(t *testing.T) {
	sv := newSystemVariablesWithDefaults()
	// No SessionHandler, client or editor: local resolution cannot use their
	// network/loading paths. Re-read the current cache on every invocation.
	f := &fuzzyFinderCommand{cli: &Cli{SystemVariables: &sv}}
	for _, cache := range []*LastQueryCache{nil, {}, {QueryPlan: &sppb.QueryPlan{}}} {
		sv.LastResult.QueryCache = cache
		got, err := f.resolveCandidates(t.Context(), fuzzyCompletePlanNode, "")
		if err != nil || len(got) != 0 {
			t.Fatalf("empty cache: %v, %v", got, err)
		}
	}
	nodes := []*sppb.PlanNode{
		{Index: 0, Kind: sppb.PlanNode_RELATIONAL, DisplayName: "Scan"},
		nil,
		{Index: 99, Kind: sppb.PlanNode_SCALAR, DisplayName: "Scan"},
		{Index: 3, DisplayName: "field\n\t\x01\x1b" + strings.Repeat("界", 200)},
	}
	sv.LastResult.QueryCache = &LastQueryCache{QueryPlan: &sppb.QueryPlan{PlanNodes: nodes}}
	got, err := f.resolveCandidates(t.Context(), fuzzyCompletePlanNode, "")
	if err != nil || len(got) != 3 {
		t.Fatalf("candidates: %v, %v", got, err)
	}
	if got[0].Value != "0" || got[1].Value != "2" || got[2].Value != "3" || !strings.Contains(got[1].Label, "SCALAR Scan") {
		t.Fatalf("positions/scalar label: %v", got)
	}
	for _, item := range got {
		if !utf8.ValidString(item.Label) || utf8.RuneCountInString(item.Label) > 160 || strings.ContainsFunc(item.Label, unicode.IsControl) {
			t.Fatalf("unsafe/unbounded label: %q", item.Label)
		}
	}
	// Actual fzf filters labels but returns only IDs, including duplicate names
	// and scalar nodes. SHOW uses the selected slice position, not Index=99.
	selected := runFzfFilter(got, "Scan", "Cached Plan Nodes", "--no-sort")
	if !reflect.DeepEqual(selected, []string{"0", "2"}) {
		t.Fatalf("selected = %v", selected)
	}
	session := &Session{systemVariables: &sv}
	for _, id := range selected {
		stmt, err := BuildStatement("SHOW PLAN NODE " + id)
		if err != nil {
			t.Fatal(err)
		}
		result, err := stmt.Execute(t.Context(), session, OperationOutput{})
		if err != nil || result == nil {
			t.Fatalf("SHOW %s: result=%v err=%v", id, result, err)
		}
	}
	sv.LastResult.QueryCache = &LastQueryCache{QueryPlan: &sppb.QueryPlan{PlanNodes: nodes[:1]}}
	got, err = f.resolveCandidates(t.Context(), fuzzyCompletePlanNode, "")
	if err != nil || len(got) != 1 {
		t.Fatalf("replacement cache: %v %v", got, err)
	}
	sv.LastResult.QueryCache = &LastQueryCache{}
	got, err = f.resolveCandidates(t.Context(), fuzzyCompletePlanNode, "")
	if err != nil || len(got) != 0 {
		t.Fatalf("planless replacement: %v %v", got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := f.resolveCandidates(ctx, fuzzyCompletePlanNode, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled completion: %v", err)
	}
}
