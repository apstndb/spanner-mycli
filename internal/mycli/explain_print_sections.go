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
	"strings"

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

func buildPlanAppendices(rows []plantree.RowWithPredicates, sections planref.PrintSections) ([]string, []ResultAppendix) {
	var predicates []string
	var appendices []ResultAppendix
	for _, section := range sections {
		var appendix ResultAppendix
		switch section {
		case planref.PrintPredicates:
			predicates = appendixLines(rows, func(row plantree.RowWithPredicates) []string {
				return row.Predicates
			})
			appendix = ResultAppendix{Title: "Predicates(identified by ID):", Lines: predicates}
		case planref.PrintOrdering:
			appendix = ResultAppendix{
				Title: "Ordering(identified by ID):",
				Lines: appendixLines(rows, func(row plantree.RowWithPredicates) []string {
					return scalarLinkLines(row, isOrderingScalarLink, func(link plantree.ScalarChildLink) string {
						return normalizeKeyOrderSuffix(link.Description)
					})
				}),
			}
		case planref.PrintAggregate:
			appendix = ResultAppendix{
				Title: "Aggregates(identified by ID):",
				Lines: appendixLines(rows, func(row plantree.RowWithPredicates) []string {
					return scalarLinkLines(row, isAggregateScalarLink, scalarLinkDescription)
				}),
			}
		case planref.PrintTyped, planref.PrintFull:
			appendix = ResultAppendix{
				Title: "Node Parameters(identified by ID):",
				Lines: appendixLines(rows, func(row plantree.RowWithPredicates) []string {
					return scalarLinkLines(row, func(_ plantree.RowWithPredicates, link plantree.ScalarChildLink) bool {
						return section == planref.PrintFull || link.Type != ""
					}, formatRawScalarLink)
				}),
			}
		}
		if len(appendix.Lines) > 0 {
			appendices = append(appendices, appendix)
		}
	}
	return predicates, appendices
}

// buildQueryPlanAppendix renders a query plan as a titled result appendix.
// It is used for CLI_QUERY_MODE='WITH_PLAN_AND_STATS', where the main table is
// occupied by the query result rows, so the plan tree is rendered after the
// table reusing the plan tree processing shared with EXPLAIN [ANALYZE].
func buildQueryPlanAppendix(sysVars *systemVariables, plan *sppb.QueryPlan) (ResultAppendix, error) {
	rows, err := processPlanNodes(plan.GetPlanNodes(), sysVars.Display.ParsedInlineStats,
		sysVars.Display.ExplainFormat, sysVars.Display.ExplainWrapWidth, sysVars.Display.ExplainHangingIndent)
	if err != nil {
		return ResultAppendix{}, err
	}

	var maxIDLength int
	for _, row := range rows {
		maxIDLength = max(maxIDLength, len(fmt.Sprint(row.ID)))
	}

	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, fmt.Sprintf("%*d: %s", maxIDLength, row.ID, row.Text()))
	}
	return ResultAppendix{Title: "Query Plan(identified by ID):", Lines: lines}, nil
}

func formatPlanRowID(row plantree.RowWithPredicates, sections planref.PrintSections) string {
	if slices.Contains(sections, planref.PrintPredicates) {
		return row.FormatID()
	}
	return fmt.Sprint(row.ID)
}

func appendixLines(rows []plantree.RowWithPredicates, items func(plantree.RowWithPredicates) []string) []string {
	var maxIDLength int
	for _, row := range rows {
		if length := len(fmt.Sprint(row.ID)); length > maxIDLength {
			maxIDLength = length
		}
	}

	var lines []string
	for _, row := range rows {
		for i, item := range items(row) {
			var prefix string
			if i == 0 {
				prefix = fmt.Sprintf("%*d:", maxIDLength, row.ID)
			} else {
				prefix = strings.Repeat(" ", maxIDLength+1)
			}
			lines = append(lines, fmt.Sprintf("%s %s", prefix, item))
		}
	}
	return lines
}

type scalarLinkGroup struct {
	typ    string
	values []string
}

func scalarLinkLines(
	row plantree.RowWithPredicates,
	include func(plantree.RowWithPredicates, plantree.ScalarChildLink) bool,
	format func(plantree.ScalarChildLink) string,
) []string {
	groupByType := map[string]int{}
	var groups []scalarLinkGroup

	for _, link := range row.ScalarChildLinks {
		if !include(row, link) {
			continue
		}

		groupIndex, ok := groupByType[link.Type]
		if !ok {
			groupIndex = len(groups)
			groupByType[link.Type] = groupIndex
			groups = append(groups, scalarLinkGroup{typ: link.Type})
		}
		groups[groupIndex].values = append(groups[groupIndex].values, format(link))
	}

	lines := make([]string, 0, len(groups))
	for _, group := range groups {
		joined := strings.Join(group.values, ", ")
		if joined == "" {
			continue
		}

		typePart := ""
		if group.typ != "" {
			typePart = group.typ + ": "
		}
		lines = append(lines, typePart+joined)
	}
	return lines
}

func formatRawScalarLink(link plantree.ScalarChildLink) string {
	if link.Variable != "" {
		return fmt.Sprintf("$%s=%s", link.Variable, link.Description)
	}
	return link.Description
}

func scalarLinkDescription(link plantree.ScalarChildLink) string {
	return link.Description
}

func normalizeKeyOrderSuffix(s string) string {
	s = strings.TrimSpace(s)
	for _, suffix := range []string{"(ASC)", "(DESC)"} {
		if strings.HasSuffix(s, " "+suffix) {
			return strings.TrimSuffix(s, " "+suffix) + " " + strings.Trim(suffix, "()")
		}
	}
	return s
}

func isOrderingScalarLink(row plantree.RowWithPredicates, link plantree.ScalarChildLink) bool {
	switch row.DisplayName {
	case "Sort", "Sort Limit":
		return link.Type == "Key"
	case "Minor Sort", "Minor Sort Limit":
		return link.Type == "MajorKey" || link.Type == "MinorKey"
	default:
		return false
	}
}

func isAggregateScalarLink(row plantree.RowWithPredicates, link plantree.ScalarChildLink) bool {
	return row.DisplayName == "Aggregate" && (link.Type == "Key" || link.Type == "Agg")
}
