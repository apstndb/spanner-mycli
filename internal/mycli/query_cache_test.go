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
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanner-mycli/internal/mycli/metrics"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"
)

const sqlExportSelectUsers = "SELECT * FROM Users"

func TestQueryCachePublicationSQLExportNames(t *testing.T) {
	t.Parallel()

	statsB := map[string]any{"elapsed_time": "2 msec", "query": "B"}
	planB := testQueryPlan(t)
	readTS := time.Date(2026, 9, 12, 8, 0, 0, 0, time.UTC)

	for _, tt := range []struct {
		name           string
		sqlTableName   string
		wantCopiedVars bool
	}{
		{
			name:           "auto-detected SQL table name",
			wantCopiedVars: true,
		},
		{
			name:           "explicit SQL table name",
			sqlTableName:   "Users",
			wantCopiedVars: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			live := newSQLExportQueryVars(t, tt.sqlTableName)
			seed := seedQueryCacheA()
			live.LastResult.QueryCache = seed

			formatted, err := publishCompletedQuery(t, live, sqlExportSelectUsers, statsB, planB, readTS, &live.LastResult.QueryCache)
			if err != nil {
				t.Fatalf("finalizeQueryResult: %v", err)
			}
			if (formatted != live) != tt.wantCopiedVars {
				t.Fatalf("prepareFormatConfig copy = %v, want copy=%v", formatted != live, tt.wantCopiedVars)
			}
			if live.Display.SQLTableName != tt.sqlTableName {
				t.Errorf("live SQLTableName = %q, want %q (auto-detect must not persist)", live.Display.SQLTableName, tt.sqlTableName)
			}
			if tt.wantCopiedVars && formatted.LastResult.QueryCache != seed {
				t.Fatal("auto-detect copy received the publication; live cache would stay stale")
			}

			got := live.LastResult.QueryCache
			if got == nil || got == seed {
				t.Fatal("live QueryCache was not replaced")
			}
			if diff := cmp.Diff(planB, got.QueryPlan, protocmp.Transform()); diff != "" {
				t.Errorf("QueryPlan mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(statsB, got.QueryStats); diff != "" {
				t.Errorf("QueryStats mismatch (-want +got):\n%s", diff)
			}
			if !got.ReadTimestamp.Equal(readTS) {
				t.Errorf("ReadTimestamp = %v, want %v", got.ReadTimestamp, readTS)
			}
		})
	}

	// Subtests run in parallel, so compare after they complete via t.Cleanup
	// is racy. Re-run the two modes sequentially here for equality.
	t.Run("detected and explicit publish equal plan/stats/timestamp", func(t *testing.T) {
		t.Parallel()
		var published [2]*LastQueryCache
		for i, sqlTableName := range []string{"", "Users"} {
			live := newSQLExportQueryVars(t, sqlTableName)
			live.LastResult.QueryCache = seedQueryCacheA()
			if _, err := publishCompletedQuery(t, live, sqlExportSelectUsers, statsB, planB, readTS, &live.LastResult.QueryCache); err != nil {
				t.Fatalf("sqlTableName=%q: %v", sqlTableName, err)
			}
			published[i] = live.LastResult.QueryCache
		}
		if diff := cmp.Diff(published[0], published[1], protocmp.Transform()); diff != "" {
			t.Errorf("detected vs explicit cache mismatch (-detected +explicit):\n%s", diff)
		}
	})
}

func TestQueryCachePublicationBufferedAndStreamingEmitters(t *testing.T) {
	t.Parallel()

	// Buffered executeWithBuffering and both streaming emitters
	// (spanvalue writer and spanvalue processor) share queryExecution.finalizeQueryResult.
	statsB := map[string]any{"elapsed_time": "3 msec"}
	planB := testQueryPlan(t)
	readTS := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)

	for _, route := range []string{"buffered", "spanvalue writer", "spanvalue processor"} {
		t.Run(route, func(t *testing.T) {
			t.Parallel()
			live := newSQLExportQueryVars(t, "")
			seed := seedQueryCacheA()
			live.LastResult.QueryCache = seed
			if _, err := publishCompletedQuery(t, live, sqlExportSelectUsers, statsB, planB, readTS, &live.LastResult.QueryCache); err != nil {
				t.Fatalf("%s: %v", route, err)
			}
			got := live.LastResult.QueryCache
			if got == nil || got == seed {
				t.Fatalf("%s did not replace the live cache", route)
			}
			if diff := cmp.Diff(planB, got.QueryPlan, protocmp.Transform()); diff != "" {
				t.Errorf("%s QueryPlan mismatch (-want +got):\n%s", route, diff)
			}
			if diff := cmp.Diff(statsB, got.QueryStats); diff != "" {
				t.Errorf("%s QueryStats mismatch (-want +got):\n%s", route, diff)
			}
			if !got.ReadTimestamp.Equal(readTS) {
				t.Errorf("%s ReadTimestamp = %v, want %v", route, got.ReadTimestamp, readTS)
			}
		})
	}
}

func TestQueryCachePublicationNilDestinationLeavesLiveCache(t *testing.T) {
	t.Parallel()

	live := newSQLExportQueryVars(t, "")
	seed := seedQueryCacheA()
	live.LastResult.QueryCache = seed

	// DUMP's executeSQLWithFormatAndTxn path passes a nil destination even
	// when SQL export auto-detects a table name and copies settings.
	formatted, err := publishCompletedQuery(t, live, sqlExportSelectUsers, map[string]any{"query": "DUMP"}, testQueryPlan(t), time.Time{}, nil)
	if err != nil {
		t.Fatalf("finalizeQueryResult: %v", err)
	}
	if formatted == live {
		t.Fatal("expected auto-detect copy so this covers DUMP-like isolated settings")
	}
	if live.LastResult.QueryCache != seed {
		t.Fatal("nil destination replaced the user's last-query cache")
	}
	if formatted.LastResult.QueryCache != seed {
		t.Fatal("nil destination must not publish onto the settings copy either")
	}
}

func TestQueryCachePublicationDoesNotInferFromPointerEquality(t *testing.T) {
	t.Parallel()

	live := newSQLExportQueryVars(t, "Users")
	seed := seedQueryCacheA()
	live.LastResult.QueryCache = seed

	var isolated *LastQueryCache
	formatted, err := publishCompletedQuery(t, live, sqlExportSelectUsers, map[string]any{"query": "B"}, testQueryPlan(t), time.Time{}, &isolated)
	if err != nil {
		t.Fatalf("finalizeQueryResult: %v", err)
	}
	if formatted != live {
		t.Fatal("explicit table name should keep the caller's settings pointer")
	}
	if live.LastResult.QueryCache != seed {
		t.Fatal("publication inferred the sysVars pointer as the destination")
	}
	if isolated == nil || isolated.QueryStats["query"] != "B" {
		t.Fatal("explicit destination was not published")
	}
}

func TestQueryCachePublicationParseFailureLeavesOldCache(t *testing.T) {
	t.Parallel()

	live := newSQLExportQueryVars(t, "Users")
	seed := seedQueryCacheA()
	live.LastResult.QueryCache = seed

	// Iterator and parse failures return before finalizeQueryResult. A
	// destination is captured, but publication must not run.
	dest := &live.LastResult.QueryCache
	if _, _, _, err := prepareFormatConfig(sqlExportSelectUsers, live); err != nil {
		t.Fatal(err)
	}
	if dest == nil || *dest != seed {
		t.Fatal("capturing the slot must not replace the cache")
	}
	if live.LastResult.QueryCache != seed {
		t.Fatal("skipped finalization replaced the live cache")
	}
}

func TestQueryCachePublicationAppendixFailureKeepsPublishedCache(t *testing.T) {
	t.Parallel()

	live := newSQLExportQueryVars(t, "Users")
	seed := seedQueryCacheA()
	live.LastResult.QueryCache = seed

	// Index 5 at slice position 0 is rejected by spannerplan.New during appendix
	// rendering, after the cache has already been replaced.
	brokenPlan := &sppb.QueryPlan{
		PlanNodes: []*sppb.PlanNode{{Index: 5, DisplayName: "Scan", Kind: sppb.PlanNode_RELATIONAL}},
	}
	statsB := map[string]any{"elapsed_time": "9 msec", "query": "B"}
	live.Query.QueryMode = sppb.ExecuteSqlRequest_WITH_PLAN_AND_STATS.Enum()

	_, err := publishCompletedQuery(t, live, sqlExportSelectUsers, statsB, brokenPlan, time.Time{}, &live.LastResult.QueryCache)
	if err == nil {
		t.Fatal("finalizeQueryResult error = nil, want appendix rendering failure")
	}
	got := live.LastResult.QueryCache
	if got == nil || got == seed {
		t.Fatal("appendix failure cleared or skipped the already published cache")
	}
	if diff := cmp.Diff(brokenPlan, got.QueryPlan, protocmp.Transform()); diff != "" {
		t.Errorf("published QueryPlan mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(statsB, got.QueryStats); diff != "" {
		t.Errorf("published QueryStats mismatch (-want +got):\n%s", diff)
	}
}

func TestQueryCachePublicationExplainLastQueryAndPlanNodes(t *testing.T) {
	t.Parallel()

	live := newSQLExportQueryVars(t, "")
	seed := &LastQueryCache{
		QueryPlan: &sppb.QueryPlan{PlanNodes: []*sppb.PlanNode{
			{Index: 0, Kind: sppb.PlanNode_RELATIONAL, DisplayName: "OldScan"},
		}},
		QueryStats: map[string]any{"query": "A"},
	}
	live.LastResult.QueryCache = seed

	planB := testQueryPlan(t)
	statsB := map[string]any{"elapsed_time": "2 msec", "query": "B"}
	result := &Result{Rows: []Row{toRow("should not persist")}}
	if _, err := publishCompletedQueryResult(t, live, sqlExportSelectUsers, result, statsB, planB, time.Time{}, &live.LastResult.QueryCache); err != nil {
		t.Fatalf("finalizeQueryResult: %v", err)
	}

	session := &Session{systemVariables: live}
	explain, err := (&ExplainLastQueryStatement{}).Execute(t.Context(), session)
	if err != nil {
		t.Fatalf("EXPLAIN LAST QUERY: %v", err)
	}
	if explain == nil || len(explain.Rows) == 0 {
		t.Fatal("EXPLAIN LAST QUERY returned no plan rows")
	}
	var sawNewPlan, sawOldPlan, sawDataRow bool
	for _, row := range explain.Rows {
		joined := rowText(row)
		if strings.Contains(joined, "Serialize Result") {
			sawNewPlan = true
		}
		if strings.Contains(joined, "OldScan") {
			sawOldPlan = true
		}
		if strings.Contains(joined, "should not persist") {
			sawDataRow = true
		}
	}
	if !sawNewPlan {
		t.Errorf("EXPLAIN LAST QUERY did not see the newly published plan; rows=%v", explain.Rows)
	}
	if sawOldPlan {
		t.Errorf("EXPLAIN LAST QUERY still showed the seed plan; rows=%v", explain.Rows)
	}
	if sawDataRow {
		t.Fatal("query result rows were retained in the published cache")
	}

	show, err := (&ShowPlanNodeStatement{NodeID: 1}).Execute(t.Context(), session)
	if err != nil {
		t.Fatalf("SHOW PLAN NODE 1: %v", err)
	}
	if show == nil || len(show.Rows) == 0 || !strings.Contains(show.Rows[0][0].RawText(), "Scan") {
		t.Errorf("SHOW PLAN NODE 1 = %v, want cached Scan node from the new plan", show)
	}

	f := &fuzzyFinderCommand{cli: &Cli{SystemVariables: live}}
	got, err := f.resolveCandidates(t.Context(), fuzzyCompletePlanNode, "")
	if err != nil {
		t.Fatalf("plan-node completion: %v", err)
	}
	if len(got) != 2 || got[0].Value != "0" || got[1].Value != "1" {
		t.Fatalf("plan-node completion = %v, want the newly published two-node plan", got)
	}
}

func newSQLExportQueryVars(t *testing.T, sqlTableName string) *systemVariables {
	t.Helper()
	sv := newSystemVariablesWithDefaultsForTest()
	sv.Display.CLIFormat = enums.DisplayModeSQLInsert
	sv.Display.SQLTableName = sqlTableName
	return sv
}

func seedQueryCacheA() *LastQueryCache {
	return &LastQueryCache{
		QueryPlan: &sppb.QueryPlan{PlanNodes: []*sppb.PlanNode{
			{Index: 0, Kind: sppb.PlanNode_RELATIONAL, DisplayName: "OldScan"},
		}},
		QueryStats:    map[string]any{"query": "A", "elapsed_time": "1 msec"},
		ReadTimestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func publishCompletedQuery(t *testing.T, live *systemVariables, sql string, stats map[string]any, plan *sppb.QueryPlan, readTS time.Time, dest **LastQueryCache) (*systemVariables, error) {
	t.Helper()
	return publishCompletedQueryResult(t, live, sql, &Result{}, stats, plan, readTS, dest)
}

func publishCompletedQueryResult(t *testing.T, live *systemVariables, sql string, result *Result, stats map[string]any, plan *sppb.QueryPlan, readTS time.Time, dest **LastQueryCache) (*systemVariables, error) {
	t.Helper()
	_, _, formatted, err := prepareFormatConfig(sql, live)
	if err != nil {
		return nil, err
	}
	result.ReadTimestamp = readTS
	qe := &queryExecution{
		SysVars:        formatted,
		Metrics:        &metrics.ExecutionMetrics{},
		QueryCacheDest: dest,
	}
	return formatted, qe.finalizeQueryResult(result, stats, plan)
}

func rowText(row Row) string {
	parts := make([]string, len(row))
	for i, cell := range row {
		parts[i] = cell.RawText()
	}
	return strings.Join(parts, " ")
}
