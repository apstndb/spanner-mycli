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
	"errors"
	"slices"
	"strings"
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
)

func TestExplainDescribeStatementsRPC(t *testing.T) {
	t.Parallel()

	stats := map[string]any{
		"elapsed_time":      "1 msec",
		"rows_returned":     "1",
		"query_text":        "SELECT 1",
		"cpu_time":          "1 msec",
		"rows_scanned":      "0",
		"optimizer_version": "7",
	}

	t.Run("EXPLAIN SELECT", func(t *testing.T) {
		t.Parallel()
		session, _ := newQueryCacheRPCSession(t, testQueryPlan(t), stats, nil)
		got, err := (&ExplainStatement{Explain: "SELECT 1"}).Execute(t.Context(), session, OperationOutput{})
		if err != nil {
			t.Fatalf("EXPLAIN: %v", err)
		}
		if got.AffectedRows == 0 || len(got.presentationRows()) == 0 {
			t.Fatalf("EXPLAIN result = %+v, want plan rows", got)
		}
		joined := rowText(got.presentationRows()[0])
		if !strings.Contains(joined, "Serialize Result") {
			t.Fatalf("EXPLAIN row = %q, want Serialize Result", joined)
		}
	})

	t.Run("EXPLAIN emulator without plan", func(t *testing.T) {
		t.Parallel()
		session, _ := newQueryCacheRPCSession(t, nil, stats, nil)
		_, err := (&ExplainStatement{Explain: "SELECT 1"}).Execute(t.Context(), session, OperationOutput{})
		if err == nil || !strings.Contains(err.Error(), "EXPLAIN statement is not supported for Cloud Spanner Emulator") {
			t.Fatalf("error = %v, want emulator EXPLAIN rejection", err)
		}
	})

	t.Run("EXPLAIN ANALYZE SELECT", func(t *testing.T) {
		t.Parallel()
		session, live := newQueryCacheRPCSession(t, testQueryPlan(t), stats, nil)
		got, err := (&ExplainAnalyzeStatement{Query: "SELECT 1"}).Execute(t.Context(), session, OperationOutput{})
		if err != nil {
			t.Fatalf("EXPLAIN ANALYZE: %v", err)
		}
		if got.AffectedRows != 1 {
			t.Fatalf("AffectedRows = %d, want 1 scanned data row", got.AffectedRows)
		}
		if live.LastResult.QueryCache == nil || live.LastResult.QueryCache.QueryPlan == nil {
			t.Fatal("EXPLAIN ANALYZE did not publish LastQueryCache")
		}
		if len(got.presentationRows()) == 0 || !strings.Contains(rowText(got.presentationRows()[0]), "Serialize Result") {
			t.Fatalf("ANALYZE rows = %v", got.presentationRows())
		}
	})

	t.Run("EXPLAIN ANALYZE emulator without plan", func(t *testing.T) {
		t.Parallel()
		session, _ := newQueryCacheRPCSession(t, nil, stats, nil)
		_, err := (&ExplainAnalyzeStatement{Query: "SELECT 1"}).Execute(t.Context(), session, OperationOutput{})
		if !errors.Is(err, errExplainAnalyzeUnsupportedOnEmulator) {
			t.Fatalf("error = %v, want %v", err, errExplainAnalyzeUnsupportedOnEmulator)
		}
	})

	t.Run("DESCRIBE SELECT", func(t *testing.T) {
		t.Parallel()
		session, _ := newQueryCacheRPCSession(t, testQueryPlan(t), stats, nil)
		got, err := (&DescribeStatement{Statement: "SELECT 1"}).Execute(t.Context(), session, OperationOutput{})
		if err != nil {
			t.Fatalf("DESCRIBE: %v", err)
		}
		if got.AffectedRows != 1 || len(got.presentationRows()) != 1 {
			t.Fatalf("DESCRIBE result = %+v", got)
		}
		if got.presentationRows()[0][0].RawText() != "id" {
			t.Fatalf("Column_Name = %q, want id", got.presentationRows()[0][0].RawText())
		}
		if !strings.Contains(got.presentationRows()[0][1].RawText(), "INT64") {
			t.Fatalf("Column_Type = %q, want INT64", got.presentationRows()[0][1].RawText())
		}
	})
}

// TestReviewPlanRenderBeforeCommit checks that fallible profiled-DML presentation
// is prepared before an implicit commit, while explicit owners stay open.
func TestReviewPlanRenderBeforeCommit(t *testing.T) {
	t.Parallel()

	const (
		dml          = "UPDATE T SET v = v + 1 WHERE id = 1"
		badTemplate  = `SET CLI_ANALYZE_COLUMNS = 'bad:{{index . "missing" 0}}'`
		goodTemplate = `SET CLI_ANALYZE_COLUMNS = 'Mark:{{.Latency}}'`
		affected     = int64(4)
	)
	plan := &sppb.QueryPlan{PlanNodes: hangingIndentPlanNodes()}

	t.Run("template failure emits no commit", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name, sql, mode string
		}{
			{name: "explain_analyze", sql: "EXPLAIN ANALYZE " + dml},
			{name: "profile", sql: dml, mode: "PROFILE"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				ctx := t.Context()
				h := newHeartbeatHarness(t)
				h.server.setQueryPlan(plan)
				h.server.setSQLRowCount(dml, affected)
				session := sessionForTM(t, h.tm)
				mustExec(t, ctx, session, badTemplate)
				if tc.mode != "" {
					mustExec(t, ctx, session, "SET CLI_QUERY_MODE = '"+tc.mode+"'")
				}

				_, err := execSQL(t, ctx, session, tc.sql)
				if err == nil || !strings.Contains(err.Error(), "failed to process query plan") || !strings.Contains(err.Error(), "template") {
					t.Fatalf("expected runtime template failure, got %v", err)
				}
				if n := len(h.server.commitObservations()); n != 0 {
					t.Fatalf("commits = %d, want 0 before the render error", n)
				}
				if n := len(h.server.rollbackIDs()); n != 1 {
					t.Fatalf("rollbacks = %d, want 1 discarded implicit transaction", n)
				}
				if session.txn.InTransaction() {
					t.Fatal("implicit render failure left an owner")
				}
				if session.systemVariables.LastResult.QueryCache != nil {
					t.Fatal("render failure published LastQueryCache")
				}
			})
		}
	})

	t.Run("with_plan_and_stats", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		h := newHeartbeatHarness(t)
		h.server.setQueryPlan(plan)
		h.server.setSQLRowCount(dml, affected)
		session := sessionForTM(t, h.tm)
		mustExec(t, ctx, session, badTemplate)
		mustExec(t, ctx, session, "SET CLI_QUERY_MODE = 'WITH_PLAN_AND_STATS'")

		res, err := execSQL(t, ctx, session, dml)
		if err != nil {
			t.Fatalf("control mode does not use analyze columns: %v", err)
		}
		assertProfiledDMLCommit(t, h, session, res, affected, true)
		if !res.ForceVerbose {
			t.Fatal("WITH_PLAN_AND_STATS did not force verbose stats")
		}
		if !hasAppendixTitle(res, "Query Plan(identified by ID):") {
			t.Fatalf("appendices = %+v, want query plan appendix", res.Appendices)
		}
	})

	t.Run("valid rendering commits once", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name, sql, mode string
		}{
			{name: "explain_analyze", sql: "EXPLAIN ANALYZE " + dml},
			{name: "profile", sql: dml, mode: "PROFILE"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				ctx := t.Context()
				h := newHeartbeatHarness(t)
				h.server.setQueryPlan(plan)
				h.server.setSQLRowCount(dml, affected)
				session := sessionForTM(t, h.tm)
				mustExec(t, ctx, session, goodTemplate)
				if tc.mode != "" {
					mustExec(t, ctx, session, "SET CLI_QUERY_MODE = '"+tc.mode+"'")
				}

				res, err := execSQL(t, ctx, session, tc.sql)
				if err != nil {
					t.Fatalf("valid profiled DML: %v", err)
				}
				assertProfiledDMLCommit(t, h, session, res, affected, true)
				names := extractTableColumnNames(res.TableHeader)
				if !slices.Contains(names, "Mark") {
					t.Fatalf("headers = %v, want rendered Mark column", names)
				}
				if text := joinedPresentation(res); !strings.Contains(text, "Serialize Result") {
					t.Fatalf("plan rows = %q, want Serialize Result", text)
				}
			})
		}
	})

	t.Run("unavailable plan emits no commit", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name, sql, mode string
		}{
			{name: "explain_analyze", sql: "EXPLAIN ANALYZE " + dml},
			{name: "profile", sql: dml, mode: "PROFILE"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				ctx := t.Context()
				h := newHeartbeatHarness(t)
				session := sessionForTM(t, h.tm)
				if tc.mode != "" {
					mustExec(t, ctx, session, "SET CLI_QUERY_MODE = '"+tc.mode+"'")
				}

				_, err := execSQL(t, ctx, session, tc.sql)
				if !errors.Is(err, errExplainAnalyzeUnsupportedOnEmulator) {
					t.Fatalf("error = %v, want %v", err, errExplainAnalyzeUnsupportedOnEmulator)
				}
				if n := len(h.server.commitObservations()); n != 0 {
					t.Fatalf("commits = %d, want 0 before the unavailable-plan error", n)
				}
				if n := len(h.server.rollbackIDs()); n != 1 {
					t.Fatalf("rollbacks = %d, want 1 discarded implicit transaction", n)
				}
				if session.txn.InTransaction() {
					t.Fatal("unavailable plan left an implicit owner")
				}
			})
		}
	})

	t.Run("explicit transaction keeps owner", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name, sql, mode, columns string
			plan                     *sppb.QueryPlan
			wantErr                  error
			wantTemplate             bool
		}{
			{
				name:         "template failure",
				sql:          "EXPLAIN ANALYZE " + dml,
				columns:      badTemplate,
				plan:         plan,
				wantTemplate: true,
			},
			{
				name:    "unavailable plan",
				sql:     dml,
				mode:    "PROFILE",
				wantErr: errExplainAnalyzeUnsupportedOnEmulator,
			},
			{
				name:    "valid rendering",
				sql:     "EXPLAIN ANALYZE " + dml,
				columns: goodTemplate,
				plan:    plan,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				ctx := t.Context()
				h := newHeartbeatHarness(t)
				if tc.plan != nil {
					h.server.setQueryPlan(tc.plan)
				}
				h.server.setSQLRowCount(dml, affected)
				session := sessionForTM(t, h.tm)
				mustExec(t, ctx, session, "SET AUTOCOMMIT = FALSE")
				if tc.columns != "" {
					mustExec(t, ctx, session, tc.columns)
				}
				if tc.mode != "" {
					mustExec(t, ctx, session, "SET CLI_QUERY_MODE = '"+tc.mode+"'")
				}

				res, err := execSQL(t, ctx, session, tc.sql)
				if tc.wantTemplate {
					if err == nil || !strings.Contains(err.Error(), "failed to process query plan") || !strings.Contains(err.Error(), "template") {
						t.Fatalf("expected runtime template failure, got %v", err)
					}
				} else if tc.wantErr != nil {
					if !errors.Is(err, tc.wantErr) {
						t.Fatalf("error = %v, want %v", err, tc.wantErr)
					}
				} else if err != nil {
					t.Fatalf("explicit profiled DML: %v", err)
				}
				if n := len(h.server.commitObservations()); n != 0 {
					t.Fatalf("explicit commits = %d, want 0", n)
				}
				if n := len(h.server.rollbackIDs()); n != 0 {
					t.Fatalf("explicit rollbacks = %d, want 0", n)
				}
				if !session.txn.InTransaction() {
					t.Fatal("explicit profiled DML dropped the owner")
				}
				if err != nil {
					if session.systemVariables.LastResult.QueryCache != nil {
						t.Fatal("explicit presentation failure published LastQueryCache")
					}
					return
				}
				assertProfiledDMLCommit(t, h, session, res, affected, false)
				if text := joinedPresentation(res); !strings.Contains(text, "Serialize Result") {
					t.Fatalf("plan rows = %q, want Serialize Result", text)
				}
			})
		}
	})
}

func assertProfiledDMLCommit(t *testing.T, h *heartbeatHarness, session *Session, res *Result, affected int64, implicit bool) {
	t.Helper()
	if res == nil || !res.IsExecutedDML || res.AffectedRowsType != rowCountTypeExact || res.AffectedRows != int(affected) {
		t.Fatalf("result = %+v, want executed DML with %d affected rows", res, affected)
	}
	cache := session.systemVariables.LastResult.QueryCache
	if cache == nil || cache.QueryPlan == nil || len(cache.QueryPlan.GetPlanNodes()) == 0 {
		t.Fatal("profiled DML did not publish a query-cache plan")
	}
	if !cache.CommitTimestamp.Equal(res.CommitTimestamp) {
		t.Fatalf("query-cache commit timestamp = %v, result = %v", cache.CommitTimestamp, res.CommitTimestamp)
	}
	commits := len(h.server.commitObservations())
	if implicit {
		if commits != 1 {
			t.Fatalf("commits = %d, want 1", commits)
		}
		if res.CommitTimestamp.IsZero() {
			t.Fatal("implicit profiled DML commit timestamp is zero")
		}
		if session.txn.InTransaction() {
			t.Fatal("implicit profiled DML left an owner")
		}
		return
	}
	if commits != 0 {
		t.Fatalf("explicit commits = %d, want 0", commits)
	}
	if !res.CommitTimestamp.IsZero() || !cache.CommitTimestamp.IsZero() {
		t.Fatalf("explicit commit timestamp = result %v cache %v, want zero", res.CommitTimestamp, cache.CommitTimestamp)
	}
}

func hasAppendixTitle(res *Result, title string) bool {
	if res == nil {
		return false
	}
	for _, appendix := range res.Appendices {
		if appendix.Title == title {
			return true
		}
	}
	return false
}

func joinedPresentation(res *Result) string {
	if res == nil {
		return ""
	}
	var b strings.Builder
	for _, row := range res.presentationRows() {
		b.WriteString(rowText(row))
		b.WriteByte('\n')
	}
	return b.String()
}
