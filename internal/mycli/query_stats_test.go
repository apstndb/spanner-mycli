// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mycli

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseQueryStatsPreservesUnknown(t *testing.T) {
	t.Parallel()

	got, err := parseQueryStats(map[string]any{
		"elapsed_time":  "1.23 msecs",
		"rows_returned": 42,
		"future_key":    "future value",
	})
	if err != nil {
		t.Fatal(err)
	}

	want := QueryStats{
		ElapsedTime: "1.23 msecs",
		Unknown: map[string]any{
			"rows_returned": 42,
			"future_key":    "future value",
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("parseQueryStats mismatch (-want +got):\n%s", diff)
	}
}

func TestParseQueryProfilePreservesQueryStatsUnknown(t *testing.T) {
	t.Parallel()

	got, err := parseQueryProfile(`{
		"queryPlan": {"planNodes": []},
		"queryStats": {
			"elapsed_time": "1.23 msecs",
			"rows_returned": 42,
			"future_key": "future value"
		},
		"fprint": "123"
	}`)
	if err != nil {
		t.Fatal(err)
	}

	want := QueryStats{
		ElapsedTime: "1.23 msecs",
		Unknown: map[string]any{
			"rows_returned": float64(42),
			"future_key":    "future value",
		},
	}
	if diff := cmp.Diff(want, got.QueryStats); diff != "" {
		t.Errorf("parseQueryProfile QueryStats mismatch (-want +got):\n%s", diff)
	}
	if got.Fprint != "123" {
		t.Errorf("Fprint = %q, want %q", got.Fprint, "123")
	}
}
