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
	"fmt"
	"slices"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spannerplan/plantree"
	planref "github.com/apstndb/spannerplan/plantree/reference"
)

// DefaultExplainPrintSections is the default value of CLI_EXPLAIN_PRINT_SECTIONS.
const DefaultExplainPrintSections = string(planref.PrintPresetBasic)

// DefaultParsedExplainPrintSections is the parsed form of DefaultExplainPrintSections.
var DefaultParsedExplainPrintSections = mustParseExplainPrintSections(DefaultExplainPrintSections)

// ResultAppendix is a titled set of post-table diagnostic lines.
type ResultAppendix struct {
	Title string
	Lines []string
}

func mustParseExplainPrintSections(value string) planref.PrintSections {
	sections, err := parseExplainPrintSections(value)
	if err != nil {
		panic(err)
	}
	return sections
}

func parseExplainPrintSections(value string) (planref.PrintSections, error) {
	sections, err := planref.ParsePrintSections(value)
	if err != nil {
		return nil, err
	}
	if sections == nil {
		return planref.PrintSections{}, nil
	}
	return sections, nil
}

func resolveExplainPrintSections(sysVars *systemVariables, override *planref.PrintSections) planref.PrintSections {
	if override != nil {
		return append(planref.PrintSections{}, (*override)...)
	}
	if sysVars == nil {
		return append(planref.PrintSections{}, DefaultParsedExplainPrintSections...)
	}
	if sysVars.Display.ExplainPrintSections == "" && sysVars.Display.ParsedExplainPrintSections == nil {
		return append(planref.PrintSections{}, DefaultParsedExplainPrintSections...)
	}
	return append(planref.PrintSections{}, sysVars.Display.ParsedExplainPrintSections...)
}

func buildPlanAppendices(rows []plantree.RowWithPredicates, sections planref.PrintSections) ([]string, []ResultAppendix, error) {
	// Always pass WithPrintSections so an empty CLI/override list stays empty.
	// Omitting the option would select the library default (predicates only).
	// Do not enable scalar-variable resolution; keep the historical raw descriptions.
	built, err := planref.BuildAppendices(rows, planref.WithPrintSections(sections...))
	if err != nil {
		return nil, nil, err
	}

	var predicates []string
	var appendices []ResultAppendix
	for _, appendix := range built {
		mapped := ResultAppendix{Title: appendix.Title, Lines: appendix.Lines}
		appendices = append(appendices, mapped)
		if appendix.Section == planref.PrintPredicates {
			predicates = appendix.Lines
		}
	}
	return predicates, appendices, nil
}

// buildQueryPlanAppendix renders a query plan as one or more titled result
// appendices. It is used for CLI_QUERY_MODE='WITH_PLAN_AND_STATS', where the
// main table is occupied by the query result rows, so the plan tree (and any
// requested CLI_EXPLAIN_PRINT_SECTIONS sections) are rendered after the table
// instead of as table columns.
//
// It reuses the same section-resolution and appendix-building machinery as
// EXPLAIN [ANALYZE] (see buildExplainAnalyzeResult): resolveExplainPrintSections
// resolves CLI_EXPLAIN_PRINT_SECTIONS, and buildPlanAppendices turns the
// resolved sections into additional appendices (e.g. Predicates, Ordering)
// following the plan-tree appendix.
func buildQueryPlanAppendix(sysVars *systemVariables, plan *sppb.QueryPlan) ([]ResultAppendix, error) {
	sections := resolveExplainPrintSections(sysVars, nil)

	rows, err := processPlanNodes(plan.GetPlanNodes(), sysVars.Display.ParsedInlineStats,
		sysVars.Display.ExplainFormat, sysVars.Display.ExplainWrapWidth, sysVars.Display.ExplainHangingIndent)
	if err != nil {
		return nil, err
	}

	var maxIDLength int
	for _, row := range rows {
		maxIDLength = max(maxIDLength, len(formatPlanRowID(row, sections)))
	}

	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, fmt.Sprintf("%*s: %s", maxIDLength, formatPlanRowID(row, sections), row.Text()))
	}
	planAppendix := ResultAppendix{Title: "Query Plan(identified by ID):", Lines: lines}

	_, sectionAppendices, err := buildPlanAppendices(rows, sections)
	if err != nil {
		return nil, err
	}

	return append([]ResultAppendix{planAppendix}, sectionAppendices...), nil
}

func formatPlanRowID(row plantree.RowWithPredicates, sections planref.PrintSections) string {
	if slices.Contains(sections, planref.PrintPredicates) {
		return row.FormatID()
	}
	return fmt.Sprint(row.ID)
}
