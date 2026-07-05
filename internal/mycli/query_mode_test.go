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
	"github.com/apstndb/spanner-mycli/internal/mycli/metrics"
)

func TestEffectiveQueryMode(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		desc     string
		userMode *sppb.ExecuteSqlRequest_QueryMode
		want     sppb.ExecuteSqlRequest_QueryMode
	}{
		{
			desc:     "unset defaults to PROFILE",
			userMode: nil,
			want:     sppb.ExecuteSqlRequest_PROFILE,
		},
		{
			desc:     "NORMAL keeps internal PROFILE default",
			userMode: sppb.ExecuteSqlRequest_NORMAL.Enum(),
			want:     sppb.ExecuteSqlRequest_PROFILE,
		},
		{
			desc:     "PLAN is handled by EXPLAIN dispatch, PROFILE here",
			userMode: sppb.ExecuteSqlRequest_PLAN.Enum(),
			want:     sppb.ExecuteSqlRequest_PROFILE,
		},
		{
			desc:     "PROFILE stays PROFILE",
			userMode: sppb.ExecuteSqlRequest_PROFILE.Enum(),
			want:     sppb.ExecuteSqlRequest_PROFILE,
		},
		{
			desc:     "WITH_STATS is respected",
			userMode: sppb.ExecuteSqlRequest_WITH_STATS.Enum(),
			want:     sppb.ExecuteSqlRequest_WITH_STATS,
		},
		{
			desc:     "WITH_PLAN_AND_STATS is respected",
			userMode: sppb.ExecuteSqlRequest_WITH_PLAN_AND_STATS.Enum(),
			want:     sppb.ExecuteSqlRequest_WITH_PLAN_AND_STATS,
		},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			t.Parallel()
			if got := effectiveQueryMode(tt.userMode); got != tt.want {
				t.Errorf("effectiveQueryMode(%v) = %v, want %v", tt.userMode, got, tt.want)
			}
		})
	}
}

func TestSetCLIQueryModeStatsValues(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		value   string
		want    sppb.ExecuteSqlRequest_QueryMode
		wantErr bool
	}{
		{value: "WITH_STATS", want: sppb.ExecuteSqlRequest_WITH_STATS},
		{value: "with_stats", want: sppb.ExecuteSqlRequest_WITH_STATS},
		{value: "WITH_PLAN_AND_STATS", want: sppb.ExecuteSqlRequest_WITH_PLAN_AND_STATS},
		{value: "WITH_BOGUS", wantErr: true},
	} {
		t.Run(tt.value, func(t *testing.T) {
			t.Parallel()
			sysVars := newSystemVariablesWithDefaultsForTest()
			err := sysVars.SetFromSimple("CLI_QUERY_MODE", tt.value)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("SetFromSimple(CLI_QUERY_MODE, %q) error = nil, want error", tt.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("SetFromSimple(CLI_QUERY_MODE, %q) error = %v, want nil", tt.value, err)
			}
			if sysVars.Query.QueryMode == nil || *sysVars.Query.QueryMode != tt.want {
				t.Errorf("Query.QueryMode = %v, want %v", sysVars.Query.QueryMode, tt.want)
			}

			got, err := sysVars.Get("CLI_QUERY_MODE")
			if err != nil {
				t.Fatalf("Get(CLI_QUERY_MODE) error = %v, want nil", err)
			}
			if got["CLI_QUERY_MODE"] != tt.want.String() {
				t.Errorf("SHOW CLI_QUERY_MODE = %q, want %q", got["CLI_QUERY_MODE"], tt.want.String())
			}
		})
	}
}

// testQueryPlan returns a minimal two-node relational plan for rendering tests.
func testQueryPlan(t *testing.T) *sppb.QueryPlan {
	t.Helper()
	return &sppb.QueryPlan{
		PlanNodes: []*sppb.PlanNode{
			{
				Index:       0,
				DisplayName: "Serialize Result",
				Kind:        sppb.PlanNode_RELATIONAL,
				ChildLinks:  []*sppb.PlanNode_ChildLink{{ChildIndex: 1}},
			},
			{
				Index:       1,
				DisplayName: "Scan",
				Kind:        sppb.PlanNode_RELATIONAL,
				Metadata:    mustNewStruct(map[string]interface{}{"scan_type": "TableScan", "scan_target": "Songs"}),
			},
		},
	}
}

func TestBuildQueryPlanAppendix(t *testing.T) {
	t.Parallel()

	sysVars := newSystemVariablesWithDefaultsForTest()
	appendix, err := buildQueryPlanAppendix(sysVars, testQueryPlan(t))
	if err != nil {
		t.Fatalf("buildQueryPlanAppendix() error = %v, want nil", err)
	}

	if want := "Query Plan(identified by ID):"; appendix.Title != want {
		t.Errorf("appendix.Title = %q, want %q", appendix.Title, want)
	}
	if len(appendix.Lines) != 2 {
		t.Fatalf("len(appendix.Lines) = %d, want 2; lines: %q", len(appendix.Lines), appendix.Lines)
	}
	if !strings.HasPrefix(appendix.Lines[0], "0: ") || !strings.Contains(appendix.Lines[0], "Serialize Result") {
		t.Errorf("appendix.Lines[0] = %q, want prefix \"0: \" and operator \"Serialize Result\"", appendix.Lines[0])
	}
	if !strings.HasPrefix(appendix.Lines[1], "1: ") || !strings.Contains(appendix.Lines[1], "Scan") {
		t.Errorf("appendix.Lines[1] = %q, want prefix \"1: \" and operator \"Scan\"", appendix.Lines[1])
	}
}

func TestFinalizeQueryResultQueryModeRendering(t *testing.T) {
	t.Parallel()

	stats := map[string]any{
		"elapsed_time": "1 msec",
		"cpu_time":     "0.5 msecs",
	}

	for _, tt := range []struct {
		desc             string
		userMode         *sppb.ExecuteSqlRequest_QueryMode
		plan             *sppb.QueryPlan
		wantForceVerbose bool
		wantPlanAppendix bool
	}{
		{
			desc:     "default mode keeps normal rendering",
			userMode: nil,
			plan:     nil,
		},
		{
			desc:     "NORMAL keeps normal rendering",
			userMode: sppb.ExecuteSqlRequest_NORMAL.Enum(),
			// The internal PROFILE request returns a plan, but NORMAL must not render it.
			plan: nil,
		},
		{
			desc:             "WITH_STATS renders stats without plan",
			userMode:         sppb.ExecuteSqlRequest_WITH_STATS.Enum(),
			plan:             nil,
			wantForceVerbose: true,
		},
		{
			desc:             "WITH_PLAN_AND_STATS renders stats and plan appendix",
			userMode:         sppb.ExecuteSqlRequest_WITH_PLAN_AND_STATS.Enum(),
			plan:             nil, // set below
			wantForceVerbose: true,
			wantPlanAppendix: true,
		},
		{
			desc:             "WITH_PLAN_AND_STATS tolerates missing plan (emulator)",
			userMode:         sppb.ExecuteSqlRequest_WITH_PLAN_AND_STATS.Enum(),
			plan:             nil,
			wantForceVerbose: true,
		},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			t.Parallel()

			plan := tt.plan
			if tt.wantPlanAppendix {
				plan = testQueryPlan(t)
			}

			sysVars := newSystemVariablesWithDefaultsForTest()
			sysVars.Query.QueryMode = tt.userMode

			result := &Result{}
			if err := finalizeQueryResult(result, stats, nil, plan, sysVars, &metrics.ExecutionMetrics{}); err != nil {
				t.Fatalf("finalizeQueryResult() error = %v, want nil", err)
			}

			if result.ForceVerbose != tt.wantForceVerbose {
				t.Errorf("result.ForceVerbose = %v, want %v", result.ForceVerbose, tt.wantForceVerbose)
			}

			if result.Stats.ElapsedTime != "1 msec" {
				t.Errorf("result.Stats.ElapsedTime = %q, want %q", result.Stats.ElapsedTime, "1 msec")
			}

			var gotPlanAppendix bool
			for _, appendix := range result.Appendices {
				if appendix.Title == "Query Plan(identified by ID):" {
					gotPlanAppendix = true
				}
			}
			if gotPlanAppendix != tt.wantPlanAppendix {
				t.Errorf("plan appendix rendered = %v, want %v; appendices: %+v", gotPlanAppendix, tt.wantPlanAppendix, result.Appendices)
			}

			if sysVars.LastResult.QueryCache == nil {
				t.Error("LastResult.QueryCache = nil, want non-nil")
			}
		})
	}
}
