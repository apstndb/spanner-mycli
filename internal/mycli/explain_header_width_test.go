// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mycli

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/olekukonko/tablewriter/tw"
)

func TestExplainResultHeaderWidth(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name                string
		width, defaultWidth int64
		want                string
	}{
		{"unlimited", 0, 0, operatorColumnName},
		{"narrow", 20, 0, "Operator"},
		{"below_boundary", operatorColumnNameLength - 1, 0, "Operator"},
		{"at_boundary", operatorColumnNameLength, 0, operatorColumnName},
		{"wide", 80, 0, operatorColumnName},
		{"narrow_default", 0, 20, "Operator"},
		{"wide_default", 0, 80, operatorColumnName},
		{"narrow_override", 20, 80, "Operator"},
		{"wide_override", 80, 20, operatorColumnName},
	} {
		t.Run(tt.name, func(t *testing.T) {
			vars := newSystemVariablesWithDefaultsForTest()
			vars.Display.ExplainWrapWidth = tt.defaultWidth
			plan := &sppb.QueryPlan{PlanNodes: hangingIndentPlanNodes()}
			plain, err := generateExplainResult(vars, plan, enums.ExplainFormatCurrent, tt.width, nil)
			if err != nil {
				t.Fatal(err)
			}
			analyze, err := buildExplainAnalyzeResult(vars, plan, QueryStats{}, enums.ExplainFormatCurrent, tt.width, nil)
			if err != nil {
				t.Fatal(err)
			}
			for name, result := range map[string]*Result{"plain": plain, "analyze": analyze} {
				got := result.TableHeader.Render(false)
				if len(got) < 2 || !slices.Equal(got[:2], []string{"ID", tt.want}) {
					t.Errorf("%s header = %v, want base [ID %s]", name, got, tt.want)
				}
				if len(result.Rows) != 3 {
					t.Errorf("%s rows = %d, want 3", name, len(result.Rows))
				}
				if !slices.Equal(result.ColumnAlign[:2], explainColumnAlign) {
					t.Errorf("%s alignment changed", name)
				}
			}
		})
	}
}

func TestExplainAnalyzeHeaderPreservesCustomColumns(t *testing.T) {
	t.Parallel()
	def := []columnRenderDef{{Name: "Elapsed", Alignment: tw.AlignCenter}}
	for _, width := range []int64{0, 20, operatorColumnNameLength} {
		names, aligns := explainAnalyzeHeader(def, width)
		if len(names) != 3 || names[2] != "Elapsed" || !slices.Equal(aligns, []tw.Align{tw.AlignRight, tw.AlignLeft, tw.AlignCenter}) {
			t.Errorf("width %d: names=%v aligns=%v", width, names, aligns)
		}
	}
}

func TestExplainNarrowHeaderRenderedTable(t *testing.T) {
	t.Parallel()
	vars := newSystemVariablesWithDefaultsForTest()
	plan := &sppb.QueryPlan{PlanNodes: hangingIndentPlanNodes()}
	result, err := generateExplainResult(vars, plan, enums.ExplainFormatCurrent, 20, nil)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := printTableData(vars, 0, &out, result); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), operatorColumnName) || !strings.Contains(out.String(), "Operator") || !strings.Contains(out.String(), "Cross Apply") {
		t.Fatalf("unexpected narrow plan table:\n%s", out.String())
	}
	// WIDTH limits the operator content, not ID, borders or cell padding.
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if len(line) > 30 {
			t.Errorf("long header still widens WIDTH20 table: %q", line)
		}
	}
}
