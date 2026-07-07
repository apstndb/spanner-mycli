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

package bigquery

import (
	"testing"

	"github.com/apstndb/spanner-mycli/internal/mycli"
)

// TestFeatureDefDispatch proves the BIGQUERY def, when merged into the
// statement table, dispatches "BIGQUERY <sql>" to a *BigQueryStatement carrying
// the SQL remainder.
func TestFeatureDefDispatch(t *testing.T) {
	t.Parallel()

	defs := mycli.MergedStatementDefs(Feature())
	stmt, err := mycli.BuildStatementWithDefs(defs, "BIGQUERY SELECT 1")
	if err != nil {
		t.Fatalf("BuildStatementWithDefs() error = %v", err)
	}
	bq, ok := stmt.(*BigQueryStatement)
	if !ok {
		t.Fatalf("dispatch returned %T, want *BigQueryStatement", stmt)
	}
	if bq.SQL != "SELECT 1" {
		t.Fatalf("SQL = %q, want %q", bq.SQL, "SELECT 1")
	}
}

// TestFeatureVarsGetSet verifies the three feature variables' Get/Set semantics,
// including the nullable-int NULL display for CLI_BIGQUERY_MAX_BYTES_BILLED,
// which must match the original NullableIntVar behavior exactly.
func TestFeatureVarsGetSet(t *testing.T) {
	t.Parallel()

	feat := Feature()
	vars := make(map[string]mycli.Variable, len(feat.Vars))
	for _, fv := range feat.Vars {
		vars[fv.Name] = fv.Var
	}

	t.Run("project string", func(t *testing.T) {
		t.Parallel()
		v := vars["CLI_BIGQUERY_PROJECT"]
		if got, _ := v.Get(); got != "" {
			t.Fatalf("initial Get() = %q, want empty", got)
		}
		if err := v.Set("bq-project"); err != nil {
			t.Fatalf("Set() error = %v", err)
		}
		if got, _ := v.Get(); got != "bq-project" {
			t.Fatalf("Get() = %q, want bq-project", got)
		}
	})

	t.Run("location string", func(t *testing.T) {
		t.Parallel()
		v := vars["CLI_BIGQUERY_LOCATION"]
		if err := v.Set("US"); err != nil {
			t.Fatalf("Set() error = %v", err)
		}
		if got, _ := v.Get(); got != "US" {
			t.Fatalf("Get() = %q, want US", got)
		}
	})

	t.Run("max bytes billed nullable int", func(t *testing.T) {
		t.Parallel()
		v := vars["CLI_BIGQUERY_MAX_BYTES_BILLED"]
		// Unset renders as NULL (must match NullableIntVar semantics exactly).
		if got, _ := v.Get(); got != "NULL" {
			t.Fatalf("initial Get() = %q, want NULL", got)
		}
		if err := v.Set("1000000"); err != nil {
			t.Fatalf("Set() error = %v", err)
		}
		if got, _ := v.Get(); got != "1000000" {
			t.Fatalf("Get() = %q, want 1000000", got)
		}
		// Setting NULL clears it back to the NULL display.
		if err := v.Set("NULL"); err != nil {
			t.Fatalf("Set(NULL) error = %v", err)
		}
		if got, _ := v.Get(); got != "NULL" {
			t.Fatalf("Get() after Set(NULL) = %q, want NULL", got)
		}
	})
}

// TestFeatureReturnsFreshInstances proves the session-isolation commitment
// (#778 §4.4): each Feature() call owns an independent config, so a SET through
// one Feature's variables does not leak into another Feature's variables or
// statements.
func TestFeatureReturnsFreshInstances(t *testing.T) {
	t.Parallel()

	feat1 := Feature()
	feat2 := Feature()

	var1 := featureVar(t, feat1, "CLI_BIGQUERY_PROJECT")
	var2 := featureVar(t, feat2, "CLI_BIGQUERY_PROJECT")

	if err := var1.Set("project-one"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if got, _ := var2.Get(); got != "" {
		t.Fatalf("second Feature's CLI_BIGQUERY_PROJECT = %q, want empty (state leaked between Feature instances)", got)
	}

	// The statement built from feat1's def must see feat1's config, not feat2's.
	stmt1, err := feat1.StatementDefs[0].HandleGroups(map[string]string{"sql": "SELECT 1"})
	if err != nil {
		t.Fatalf("HandleGroups() error = %v", err)
	}
	bq1, ok := stmt1.(*BigQueryStatement)
	if !ok {
		t.Fatalf("HandleGroups returned %T, want *BigQueryStatement", stmt1)
	}
	if bq1.cfg.Project != "project-one" {
		t.Fatalf("statement config Project = %q, want project-one", bq1.cfg.Project)
	}
}

func featureVar(t *testing.T, feat mycli.Feature, name string) mycli.Variable {
	t.Helper()
	for _, fv := range feat.Vars {
		if fv.Name == name {
			return fv.Var
		}
	}
	t.Fatalf("feature has no variable %q", name)
	return nil
}

// TestEffectiveProject preserves the CLI_BIGQUERY_PROJECT -> CLI_PROJECT
// fallback coverage from the pre-extraction bigQueryProject test.
func TestEffectiveProject(t *testing.T) {
	t.Parallel()

	if got := effectiveProject("bq-project", "spanner-project"); got != "bq-project" {
		t.Fatalf("effectiveProject(explicit) = %q, want bq-project", got)
	}
	if got := effectiveProject("", "spanner-project"); got != "spanner-project" {
		t.Fatalf("effectiveProject(fallback) = %q, want spanner-project", got)
	}
	if got := effectiveProject("", ""); got != "" {
		t.Fatalf("effectiveProject(none) = %q, want empty", got)
	}
}
