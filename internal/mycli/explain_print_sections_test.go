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
	"testing"

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
				{Type: "Key", Description: "$FirstName (DESC)"},
			},
		},
		{
			ID:          2,
			DisplayName: "Aggregate",
			ScalarChildLinks: []plantree.ScalarChildLink{
				{Type: "Key", Description: "$SingerId"},
				{Type: "Agg", Variable: "count", Description: "COUNT(*)"},
			},
		},
	}

	predicates, appendices := buildPlanAppendices(rows, planref.PrintSections{
		planref.PrintPredicates,
		planref.PrintOrdering,
		planref.PrintAggregate,
	})

	wantPredicates := []string{"0: Condition: ($SingerId = 1)"}
	if diff := cmp.Diff(wantPredicates, predicates); diff != "" {
		t.Errorf("predicates mismatch (-want +got):\n%s", diff)
	}

	wantAppendices := []ResultAppendix{
		{
			Title: "Predicates(identified by ID):",
			Lines: []string{"0: Condition: ($SingerId = 1)"},
		},
		{
			Title: "Ordering(identified by ID):",
			Lines: []string{"1: Key: $LastName ASC, $FirstName DESC"},
		},
		{
			Title: "Aggregates(identified by ID):",
			Lines: []string{
				"2: Key: $SingerId",
				"   Agg: COUNT(*)",
			},
		},
	}
	if diff := cmp.Diff(wantAppendices, appendices); diff != "" {
		t.Errorf("appendices mismatch (-want +got):\n%s", diff)
	}
}

func TestBuildPlanAppendicesTypedAndFull(t *testing.T) {
	t.Parallel()
	rows := []plantree.RowWithPredicates{
		{
			ID: 0,
			ScalarChildLinks: []plantree.ScalarChildLink{
				{Type: "Condition", Description: "($SingerId = 1)"},
				{Variable: "SingerId", Description: "SingerId"},
			},
		},
	}

	_, typed := buildPlanAppendices(rows, planref.PrintSections{planref.PrintTyped})
	wantTyped := []ResultAppendix{{
		Title: "Node Parameters(identified by ID):",
		Lines: []string{"0: Condition: ($SingerId = 1)"},
	}}
	if diff := cmp.Diff(wantTyped, typed); diff != "" {
		t.Errorf("typed appendices mismatch (-want +got):\n%s", diff)
	}

	_, full := buildPlanAppendices(rows, planref.PrintSections{planref.PrintFull})
	wantFull := []ResultAppendix{{
		Title: "Node Parameters(identified by ID):",
		Lines: []string{"0: Condition: ($SingerId = 1)", "   $SingerId=SingerId"},
	}}
	if diff := cmp.Diff(wantFull, full); diff != "" {
		t.Errorf("full appendices mismatch (-want +got):\n%s", diff)
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
