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
	"testing"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/enums"
)

func TestAutocommitDefaultTrue(t *testing.T) {
	t.Parallel()
	sv := newSystemVariablesWithDefaultsForTest()
	sv.ensureRegistry()

	if !sv.Transaction.Autocommit {
		t.Fatal("defaults() must initialize Autocommit to true")
	}
	got, err := sv.Get("AUTOCOMMIT")
	if err != nil {
		t.Fatalf("SHOW AUTOCOMMIT: %v", err)
	}
	if got["AUTOCOMMIT"] != "TRUE" {
		t.Fatalf("SHOW AUTOCOMMIT = %v, want TRUE", got)
	}
	if _, ok := sv.Registry.GetVariable("AUTOCOMMIT").(*UnimplementedVar); ok {
		t.Fatal("AUTOCOMMIT must not remain UnimplementedVar")
	}
}

func TestAutocommitResetRestoresStartupSnapshot(t *testing.T) {
	t.Parallel()
	sv := newSystemVariablesWithDefaultsForTest()
	if err := sv.CaptureStartupSnapshots(); err != nil {
		t.Fatal(err)
	}
	if err := sv.SetFromSimple("AUTOCOMMIT", "FALSE"); err != nil {
		t.Fatal(err)
	}
	if sv.Transaction.Autocommit {
		t.Fatal("SET AUTOCOMMIT=FALSE did not take")
	}
	if err := sv.Reset("AUTOCOMMIT"); err != nil {
		t.Fatalf("RESET AUTOCOMMIT: %v", err)
	}
	if !sv.Transaction.Autocommit {
		t.Fatal("RESET AUTOCOMMIT must restore startup true")
	}
}

func TestAutocommitSameValueSetIsIdempotentWithGuards(t *testing.T) {
	t.Parallel()
	sv := newSystemVariablesWithDefaultsForTest()
	sv.ensureRegistry()
	sv.inTransaction = func() bool { return true }
	sv.inManualBatch = func() bool { return true }

	if err := sv.SetFromSimple("AUTOCOMMIT", "TRUE"); err != nil {
		t.Fatalf("same-value SET with owner/batch: %v", err)
	}
	if !sv.Transaction.Autocommit {
		t.Fatal("same-value SET mutated AUTOCOMMIT")
	}
}

func TestAutocommitToggleRejectedWithExactErrors(t *testing.T) {
	t.Parallel()
	t.Run("transaction", func(t *testing.T) {
		t.Parallel()
		sv := newSystemVariablesWithDefaultsForTest()
		sv.ensureRegistry()
		sv.inTransaction = func() bool { return true }
		err := sv.SetFromSimple("AUTOCOMMIT", "FALSE")
		if !errors.Is(err, errSetterInTransaction) {
			t.Fatalf("toggle in transaction: %v, want %v", err, errSetterInTransaction)
		}
		if !sv.Transaction.Autocommit {
			t.Fatal("rejected toggle mutated AUTOCOMMIT")
		}
	})
	t.Run("manual batch", func(t *testing.T) {
		t.Parallel()
		sv := newSystemVariablesWithDefaultsForTest()
		sv.ensureRegistry()
		sv.inManualBatch = func() bool { return true }
		err := sv.SetFromSimple("AUTOCOMMIT", "FALSE")
		if !errors.Is(err, errSetterInManualBatch) {
			t.Fatalf("toggle in batch: %v, want %v", err, errSetterInManualBatch)
		}
		if !sv.Transaction.Autocommit {
			t.Fatal("rejected toggle mutated AUTOCOMMIT")
		}
	})
}

func TestAutocommitResetToggleRejectedBeforeAssign(t *testing.T) {
	t.Parallel()
	sv := newSystemVariablesWithDefaultsForTest()
	if err := sv.CaptureStartupSnapshots(); err != nil {
		t.Fatal(err)
	}
	if err := sv.SetFromSimple("AUTOCOMMIT", "FALSE"); err != nil {
		t.Fatal(err)
	}
	sv.inTransaction = func() bool { return true }
	err := sv.Reset("AUTOCOMMIT")
	if err == nil || !errors.Is(err, errSetterInTransaction) {
		t.Fatalf("RESET toggle in transaction: %v, want %v", err, errSetterInTransaction)
	}
	if sv.Transaction.Autocommit {
		t.Fatal("rejected RESET mutated AUTOCOMMIT")
	}
}

func TestLazyAutocommitEligibilityDispatch(t *testing.T) {
	t.Parallel()

	plan := sppb.ExecuteSqlRequest_PLAN
	profile := sppb.ExecuteSqlRequest_PROFILE
	insert := spanner.NewStatement("INSERT INTO T (id) VALUES (1)")
	createTable := "CREATE TABLE T (id INT64) PRIMARY KEY (id)"

	tests := []struct {
		name              string
		stmt              Statement
		queryMode         *sppb.ExecuteSqlRequest_QueryMode
		tryPartitionQuery bool
		batch             Statement
		want              bool
	}{
		{name: "ordinary SELECT", stmt: &SelectStatement{Query: "SELECT 1"}, want: true},
		{name: "ordinary DML", stmt: &DmlStatement{Dml: "INSERT INTO T (id) VALUES (1)"}, want: true},
		{name: "MUTATE", stmt: &MutateStatement{Table: "T", Operation: "INSERT"}, want: true},
		{name: "EXPLAIN ANALYZE SELECT", stmt: &ExplainAnalyzeStatement{Query: "SELECT 1"}, want: true},
		{name: "EXPLAIN ANALYZE DML", stmt: &ExplainAnalyzeDmlStatement{Dml: "UPDATE T SET x=1 WHERE true"}, want: true},
		{name: "SAVEPOINT", stmt: &SavepointStatement{Name: "keep"}, want: true},
		{name: "nonempty BatchDML", stmt: &BatchDMLStatement{DMLs: []spanner.Statement{insert}}, want: true},
		{name: "empty BatchDML", stmt: &BatchDMLStatement{}, want: false},
		{name: "EXPLAIN PLAN SELECT", stmt: &ExplainStatement{Explain: "SELECT 1"}, want: false},
		{name: "EXPLAIN PLAN DML", stmt: &ExplainStatement{Explain: "UPDATE T SET x=1 WHERE true", IsDML: true}, want: false},
		{name: "explicit PDML", stmt: &PartitionedDmlStatement{Dml: "UPDATE T SET x=1 WHERE true"}, want: false},
		{name: "TRUNCATE", stmt: &TruncateTableStatement{Table: "T"}, want: false},
		{name: "BEGIN", stmt: &BeginStatement{}, want: false},
		{name: "COMMIT", stmt: &CommitStatement{}, want: false},
		{name: "SHOW VARIABLES", stmt: &ShowVariablesStatement{}, want: false},
		{name: "HELP", stmt: &HelpStatement{}, want: false},
		{name: "DDL", stmt: &DdlStatement{Ddl: createTable}, want: false},
		{name: "SYNC PROTO BUNDLE", stmt: &SyncProtoStatement{}, want: false},
		{name: "CLI_QUERY_MODE=PLAN SELECT", stmt: &SelectStatement{Query: "SELECT 1"}, queryMode: &plan, want: false},
		{name: "CLI_QUERY_MODE=PLAN DML", stmt: &DmlStatement{Dml: "UPDATE T SET x=1 WHERE true"}, queryMode: &plan, want: false},
		{name: "TRY PARTITION QUERY SELECT", stmt: &SelectStatement{Query: "SELECT 1"}, tryPartitionQuery: true, want: false},
		{name: "manual-batch DML enqueue", stmt: &DmlStatement{Dml: "INSERT INTO T (id) VALUES (1)"}, batch: &BatchDMLStatement{}, want: false},
		{name: "PROFILE DML in a manual batch", stmt: &DmlStatement{Dml: "UPDATE T SET x=1 WHERE true"}, queryMode: &profile, batch: &BatchDMLStatement{}, want: true},
		{name: "PLAN DML in a manual batch", stmt: &DmlStatement{Dml: "UPDATE T SET x=1 WHERE true"}, queryMode: &plan, batch: &BatchDMLStatement{}, want: false},
		{name: "empty RUN BATCH", stmt: &RunBatchStatement{}, batch: &BatchDMLStatement{}, want: false},
		{name: "nonempty RUN BATCH DML", stmt: &RunBatchStatement{}, batch: &BatchDMLStatement{DMLs: []spanner.Statement{insert}}, want: true},
		{name: "RUN BATCH DDL", stmt: &RunBatchStatement{}, batch: &BulkDdlStatement{Ddls: []string{createTable}}, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			session := &Session{systemVariables: newSystemVariablesWithDefaultsForTest()}
			session.systemVariables.Query.QueryMode = tc.queryMode
			session.systemVariables.Query.TryPartitionQuery = tc.tryPartitionQuery
			if tc.batch != nil {
				session.batch.SetCurrent(tc.batch)
			}
			if got := lazyAutocommitEligible(session, tc.stmt); got != tc.want {
				t.Fatalf("lazyAutocommitEligible() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAdmitLazyAutocommitSavepoint(t *testing.T) {
	t.Parallel()
	session := &Session{systemVariables: newSystemVariablesWithDefaultsForTest()}

	if err := admitLazyAutocommitStatement(session, &SavepointStatement{Name: "keep"}); !errors.Is(err, errSavepointDisabled) {
		t.Fatalf("disabled SAVEPOINT: %v, want %v", err, errSavepointDisabled)
	}
	if err := admitLazyAutocommitStatement(session, &SavepointStatement{}); !errors.Is(err, errSavepointEmptyName) {
		t.Fatalf("empty SAVEPOINT: %v, want %v", err, errSavepointEmptyName)
	}

	session.systemVariables.Transaction.SavepointSupport = enums.SavepointSupportEnabled
	if err := admitLazyAutocommitStatement(session, &SavepointStatement{Name: "keep"}); err != nil {
		t.Fatalf("enabled SAVEPOINT: %v", err)
	}
	if err := admitLazyAutocommitStatement(session, &SelectStatement{Query: "SELECT 1"}); err != nil {
		t.Fatalf("non-SAVEPOINT admit: %v", err)
	}
}
