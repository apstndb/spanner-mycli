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

	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spannerplan/plantree"
	planref "github.com/apstndb/spannerplan/plantree/reference"
	"github.com/google/go-cmp/cmp"
)

func TestBuildPlanAppendices(t *testing.T) {
	t.Parallel()
	rows := []plantree.RowWithPredicates{
		{
			ID:         0,
			Predicates: []string{"Condition: ($SingerId = 1)"},
		},
		{
			ID:          1,
			DisplayName: "Sort",
			ScalarChildLinks: []plantree.ScalarChildLink{
				{Type: "Key", Description: "$LastName (ASC)"},
			},
		},
	}

	predicates, appendices, err := buildPlanAppendices(rows, planref.PrintSections{
		planref.PrintPredicates,
		planref.PrintOrdering,
	})
	if err != nil {
		t.Fatalf("buildPlanAppendices() error = %v", err)
	}
	if len(appendices) != 2 {
		t.Fatalf("len(appendices) = %d, want 2: %+v", len(appendices), appendices)
	}
	if appendices[0].Title != "Predicates(identified by ID):" {
		t.Errorf("appendices[0].Title = %q", appendices[0].Title)
	}
	if appendices[1].Title != "Ordering(identified by ID):" {
		t.Errorf("appendices[1].Title = %q", appendices[1].Title)
	}
	wantPredicateLine := "0: Condition: ($SingerId = 1)"
	wantOrderingLine := "1: Key: $LastName ASC"
	if diff := cmp.Diff([]string{wantPredicateLine}, appendices[0].Lines); diff != "" {
		t.Errorf("predicate appendix lines mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{wantOrderingLine}, appendices[1].Lines); diff != "" {
		t.Errorf("ordering appendix lines mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{wantPredicateLine}, predicates); diff != "" {
		t.Errorf("legacy predicates slice mismatch (-want +got):\n%s", diff)
	}

	predicates, appendices, err = buildPlanAppendices(rows, planref.PrintSections{})
	if err != nil {
		t.Fatalf("empty sections error = %v", err)
	}
	if len(predicates) != 0 || len(appendices) != 0 {
		t.Fatalf("empty sections returned predicates=%q appendices=%+v", predicates, appendices)
	}
}

const invalidPrintSectionsCause = `print section "full" cannot be combined with other sections`

func TestBuildPlanAppendicesPropagatesInvalidSections(t *testing.T) {
	t.Parallel()
	_, _, err := buildPlanAppendices(nil, planref.PrintSections{planref.PrintFull, planref.PrintPredicates})
	if err == nil || !strings.Contains(err.Error(), invalidPrintSectionsCause) {
		t.Fatalf("buildPlanAppendices() error = %v, want %q", err, invalidPrintSectionsCause)
	}
}

func TestBuildQueryPlanAppendixPropagatesAppendixError(t *testing.T) {
	t.Parallel()
	sysVars := newSystemVariablesWithDefaultsForTest()
	sysVars.Display.ParsedExplainPrintSections = planref.PrintSections{planref.PrintFull, planref.PrintPredicates}
	_, err := buildQueryPlanAppendix(sysVars, testQueryPlan(t))
	if err == nil || !strings.Contains(err.Error(), invalidPrintSectionsCause) {
		t.Fatalf("buildQueryPlanAppendix() error = %v, want %q", err, invalidPrintSectionsCause)
	}
}

func TestBuildExplainAnalyzeResultPropagatesAppendixError(t *testing.T) {
	t.Parallel()
	sysVars := newSystemVariablesWithDefaultsForTest()
	invalid := planref.PrintSections{planref.PrintFull, planref.PrintPredicates}
	_, err := buildExplainAnalyzeResult(sysVars, testQueryPlan(t), QueryStats{}, enums.ExplainFormatUnspecified, 0, &invalid)
	if err == nil || !strings.Contains(err.Error(), "failed to process query plan") || !strings.Contains(err.Error(), invalidPrintSectionsCause) {
		t.Fatalf("buildExplainAnalyzeResult() error = %v, want wrapped %q", err, invalidPrintSectionsCause)
	}
}

func TestResolveExplainPrintSectionsEmptySystemVariable(t *testing.T) {
	t.Parallel()
	sysVars := newSystemVariablesWithDefaults()
	if err := sysVars.SetFromSimple("CLI_EXPLAIN_PRINT_SECTIONS", ""); err != nil {
		t.Fatalf("SetFromSimple() error = %v", err)
	}

	got := resolveExplainPrintSections(&sysVars, nil)
	want := planref.PrintSections{}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("resolveExplainPrintSections() mismatch (-want +got):\n%s", diff)
	}
}

func TestParseExplainPrintSectionsPreset(t *testing.T) {
	t.Parallel()
	got, err := parseExplainPrintSections("enhanced")
	if err != nil {
		t.Fatalf("parseExplainPrintSections() error = %v", err)
	}

	want := planref.PrintSections{planref.PrintPredicates, planref.PrintOrdering, planref.PrintAggregate}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("parseExplainPrintSections() mismatch (-want +got):\n%s", diff)
	}
}
