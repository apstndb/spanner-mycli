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
	"strings"
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestCLIExplainConciseMetadataDefaultSetShowReset(t *testing.T) {
	t.Parallel()
	sv := newSystemVariablesWithDefaultsForTest()
	sv.ensureRegistry()
	if err := sv.CaptureStartupSnapshots(); err != nil {
		t.Fatal(err)
	}
	if got := mustGetVarFromSysVars(t, sv, "CLI_EXPLAIN_CONCISE_METADATA"); got != "FALSE" {
		t.Fatalf("default = %q, want FALSE", got)
	}
	if sv.Display.ExplainConciseMetadata {
		t.Fatal("ExplainConciseMetadata default is true")
	}
	if err := sv.SetFromSimple("CLI_EXPLAIN_CONCISE_METADATA", "TRUE"); err != nil {
		t.Fatal(err)
	}
	if got := mustGetVarFromSysVars(t, sv, "CLI_EXPLAIN_CONCISE_METADATA"); got != "TRUE" {
		t.Fatalf("after SET = %q, want TRUE", got)
	}
	if err := sv.Reset("CLI_EXPLAIN_CONCISE_METADATA"); err != nil {
		t.Fatal(err)
	}
	if got := mustGetVarFromSysVars(t, sv, "CLI_EXPLAIN_CONCISE_METADATA"); got != "FALSE" {
		t.Fatalf("after RESET = %q, want FALSE", got)
	}
}

func TestExplainConciseMetadataOmitsExactDefaultsAndKeepsOthers(t *testing.T) {
	t.Parallel()
	plan := conciseMetadataTestPlan()
	original := proto.Clone(plan).(*sppb.QueryPlan)

	for _, format := range []enums.ExplainFormat{
		enums.ExplainFormatUnspecified,
		enums.ExplainFormatCurrent,
		enums.ExplainFormatTraditional,
		enums.ExplainFormatCompact,
	} {
		t.Run(format.String(), func(t *testing.T) {
			t.Parallel()
			sv := newSystemVariablesWithDefaultsForTest()
			defaultText := explainAndAppendixText(t, sv, plan, format)
			for _, kv := range [][2]string{{"seekable_key_size", "0"}, {"scan_method", "Automatic"}, {"scan_method", "Auto"}} {
				if !containsExactMetadata(defaultText, kv[0], kv[1]) {
					t.Fatalf("default omitted %s: %s:\n%s", kv[0], kv[1], defaultText)
				}
			}

			if err := sv.SetFromSimple("CLI_EXPLAIN_CONCISE_METADATA", "TRUE"); err != nil {
				t.Fatal(err)
			}
			conciseText := explainAndAppendixText(t, sv, plan, format)
			for _, kv := range [][2]string{{"seekable_key_size", "0"}, {"scan_method", "Automatic"}, {"scan_method", "Auto"}} {
				if containsExactMetadata(conciseText, kv[0], kv[1]) {
					t.Fatalf("concise kept %s: %s:\n%s", kv[0], kv[1], conciseText)
				}
			}
			for _, kv := range [][2]string{
				{"seekable_key_size", "1"},
				{"seekable_key_size", "00"},
				{"seekable_key_size", "unknown"},
				{"scan_method", "Row"},
				{"scan_method", "automatic"},
				{"scan_method", "Future"},
				{"unknown", "0"},
				{"unknown", "Auto"},
			} {
				if !containsExactMetadata(conciseText, kv[0], kv[1]) {
					t.Fatalf("concise dropped %s: %s:\n%s", kv[0], kv[1], conciseText)
				}
			}
			if !proto.Equal(plan, original) {
				t.Fatal("rendering mutated the input query plan")
			}
		})
	}
}

func TestExplainConciseMetadataDoesNotChangeRawPlanExports(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	plan := proto.Clone(conciseMetadataTestPlan()).(*sppb.QueryPlan)
	session.systemVariables.LastResult.QueryCache = &LastQueryCache{QueryPlan: plan}
	beforeJSON, beforeYAML, beforeNode := rawPlanExports(t, session)
	if err := session.systemVariables.SetFromSimple("CLI_EXPLAIN_CONCISE_METADATA", "TRUE"); err != nil {
		t.Fatal(err)
	}
	afterJSON, afterYAML, afterNode := rawPlanExports(t, session)
	if beforeJSON != afterJSON {
		t.Fatal("SHOW LAST QUERY PLAN ProtoJSON changed after enabling concise metadata")
	}
	if beforeYAML != afterYAML {
		t.Fatal("SHOW LAST QUERY PLAN YAML changed after enabling concise metadata")
	}
	if beforeNode != afterNode {
		t.Fatal("SHOW PLAN NODE YAML changed after enabling concise metadata")
	}
	if !proto.Equal(plan, conciseMetadataTestPlan()) {
		t.Fatal("cached query plan mutated")
	}
}

func containsExactMetadata(text, key, value string) bool {
	for _, sep := range []string{": ", ":"} {
		needle := key + sep + value
		idx := 0
		for {
			i := strings.Index(text[idx:], needle)
			if i < 0 {
				break
			}
			i += idx
			end := i + len(needle)
			if end == len(text) || strings.ContainsRune("),\n", rune(text[end])) {
				return true
			}
			idx = i + 1
		}
	}
	return false
}

func conciseMetadataTestPlan() *sppb.QueryPlan {
	fields := func(pairs ...string) *structpb.Struct {
		m := make(map[string]*structpb.Value, len(pairs)/2)
		for i := 0; i+1 < len(pairs); i += 2 {
			m[pairs[i]] = structpb.NewStringValue(pairs[i+1])
		}
		return &structpb.Struct{Fields: m}
	}
	return &sppb.QueryPlan{PlanNodes: []*sppb.PlanNode{
		{Index: 0, DisplayName: "Serialize Result", Kind: sppb.PlanNode_RELATIONAL, ChildLinks: []*sppb.PlanNode_ChildLink{
			{ChildIndex: 1}, {ChildIndex: 2}, {ChildIndex: 3}, {ChildIndex: 4}, {ChildIndex: 5}, {ChildIndex: 6},
		}},
		{Index: 1, DisplayName: "Filter Scan", Kind: sppb.PlanNode_RELATIONAL, Metadata: fields("seekable_key_size", "0", "scan_method", "Automatic", "execution_method", "Row")},
		{Index: 2, DisplayName: "Index Scan", Kind: sppb.PlanNode_RELATIONAL, Metadata: fields("scan_method", "Auto")},
		{Index: 3, DisplayName: "Table Scan", Kind: sppb.PlanNode_RELATIONAL, Metadata: fields("seekable_key_size", "1", "scan_method", "Row")},
		{Index: 4, DisplayName: "Scan", Kind: sppb.PlanNode_RELATIONAL, Metadata: fields("seekable_key_size", "00", "scan_method", "automatic")},
		{Index: 5, DisplayName: "Scan", Kind: sppb.PlanNode_RELATIONAL, Metadata: fields("seekable_key_size", "unknown", "scan_method", "Future", "unknown", "0")},
		{Index: 6, DisplayName: "Scan", Kind: sppb.PlanNode_RELATIONAL, Metadata: fields("unknown", "Auto")},
	}}
}

func explainAndAppendixText(t *testing.T, sv *systemVariables, plan *sppb.QueryPlan, format enums.ExplainFormat) string {
	t.Helper()
	explain, err := generateExplainResult(sv, plan, format, 0, nil)
	if err != nil {
		t.Fatalf("generateExplainResult: %v", err)
	}
	analyze, err := buildExplainAnalyzeResult(sv, plan, QueryStats{}, format, 0, nil)
	if err != nil {
		t.Fatalf("buildExplainAnalyzeResult: %v", err)
	}
	prevCache := sv.LastResult.QueryCache
	sv.LastResult.QueryCache = &LastQueryCache{QueryPlan: plan}
	last, err := (&ExplainLastQueryStatement{Format: format}).Execute(t.Context(), &Session{systemVariables: sv}, OperationOutput{})
	sv.LastResult.QueryCache = prevCache
	if err != nil {
		t.Fatalf("ExplainLastQueryStatement: %v", err)
	}
	savedFormat := sv.Display.ExplainFormat
	sv.Display.ExplainFormat = format
	appendices, err := buildQueryPlanAppendix(sv, plan)
	sv.Display.ExplainFormat = savedFormat
	if err != nil {
		t.Fatalf("buildQueryPlanAppendix: %v", err)
	}
	var b strings.Builder
	for _, result := range []*Result{explain, analyze, last} {
		for _, row := range result.presentationRows() {
			for _, cell := range row {
				b.WriteString(cell.RawText())
				b.WriteByte('\n')
			}
		}
	}
	for _, appendix := range appendices {
		b.WriteString(appendix.Title)
		b.WriteByte('\n')
		for _, line := range appendix.Lines {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func TestExplainConciseMetadataDefaultMatchesDisabledOption(t *testing.T) {
	t.Parallel()
	plan := conciseMetadataTestPlan()
	sv := newSystemVariablesWithDefaultsForTest()
	got := explainAndAppendixText(t, sv, plan, enums.ExplainFormatCurrent)
	explicit := newSystemVariablesWithDefaultsForTest()
	if err := explicit.SetFromSimple("CLI_EXPLAIN_CONCISE_METADATA", "FALSE"); err != nil {
		t.Fatal(err)
	}
	want := explainAndAppendixText(t, explicit, plan, enums.ExplainFormatCurrent)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("default vs explicit FALSE mismatch (-want +got):\n%s", diff)
	}
}
