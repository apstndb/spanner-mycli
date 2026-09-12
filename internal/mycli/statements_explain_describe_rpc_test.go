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
	"strings"
	"testing"
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
