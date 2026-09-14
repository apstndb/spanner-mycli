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
	session := &Session{systemVariables: newSystemVariablesWithDefaultsForTest()}

	if !lazyAutocommitEligible(session, &SelectStatement{Query: "SELECT 1"}) {
		t.Fatal("ordinary SELECT must be eligible")
	}
	if !lazyAutocommitEligible(session, &DmlStatement{Dml: "INSERT INTO T (id) VALUES (1)"}) {
		t.Fatal("ordinary DML must be eligible")
	}
	if !lazyAutocommitEligible(session, &MutateStatement{Table: "T", Operation: "INSERT"}) {
		t.Fatal("MUTATE must be eligible")
	}
	if !lazyAutocommitEligible(session, &ExplainAnalyzeStatement{Query: "SELECT 1"}) {
		t.Fatal("EXPLAIN ANALYZE SELECT must be eligible")
	}
	if !lazyAutocommitEligible(session, &ExplainAnalyzeDmlStatement{Dml: "UPDATE T SET x=1 WHERE true"}) {
		t.Fatal("EXPLAIN ANALYZE DML must be eligible")
	}
	if !lazyAutocommitEligible(session, &SavepointStatement{Name: "keep"}) {
		t.Fatal("SAVEPOINT must be eligible")
	}
	if !lazyAutocommitEligible(session, &BatchDMLStatement{DMLs: []spanner.Statement{spanner.NewStatement("INSERT INTO T (id) VALUES (1)")}}) {
		t.Fatal("nonempty BatchDML must be eligible")
	}
	if lazyAutocommitEligible(session, &BatchDMLStatement{}) {
		t.Fatal("empty BatchDML must not be eligible")
	}
	if lazyAutocommitEligible(session, &ExplainStatement{Explain: "SELECT 1"}) {
		t.Fatal("EXPLAIN PLAN SELECT must not be eligible")
	}
	if lazyAutocommitEligible(session, &ExplainStatement{Explain: "UPDATE T SET x=1 WHERE true", IsDML: true}) {
		t.Fatal("EXPLAIN PLAN DML must not be eligible")
	}
	if lazyAutocommitEligible(session, &PartitionedDmlStatement{Dml: "UPDATE T SET x=1 WHERE true"}) {
		t.Fatal("explicit PDML must not be eligible")
	}
	if lazyAutocommitEligible(session, &TruncateTableStatement{Table: "T"}) {
		t.Fatal("TRUNCATE must not be eligible")
	}
	if lazyAutocommitEligible(session, &BeginStatement{}) {
		t.Fatal("BEGIN must not be eligible")
	}
	if lazyAutocommitEligible(session, &CommitStatement{}) {
		t.Fatal("COMMIT must not be eligible")
	}
	if lazyAutocommitEligible(session, &ShowVariablesStatement{}) {
		t.Fatal("SHOW VARIABLES must not be eligible")
	}
	if lazyAutocommitEligible(session, &HelpStatement{}) {
		t.Fatal("HELP must not be eligible")
	}
	if lazyAutocommitEligible(session, &DdlStatement{Ddl: "CREATE TABLE T (id INT64) PRIMARY KEY (id)"}) {
		t.Fatal("DDL must not be eligible")
	}

	plan := sppb.ExecuteSqlRequest_PLAN
	session.systemVariables.Query.QueryMode = &plan
	if lazyAutocommitEligible(session, &SelectStatement{Query: "SELECT 1"}) {
		t.Fatal("CLI_QUERY_MODE=PLAN SELECT must not be eligible")
	}
	if lazyAutocommitEligible(session, &DmlStatement{Dml: "UPDATE T SET x=1 WHERE true"}) {
		t.Fatal("CLI_QUERY_MODE=PLAN DML must not be eligible")
	}

	session.systemVariables.Query.QueryMode = nil
	session.systemVariables.Query.TryPartitionQuery = true
	if lazyAutocommitEligible(session, &SelectStatement{Query: "SELECT 1"}) {
		t.Fatal("TRY PARTITION QUERY SELECT must not be eligible")
	}

	session.systemVariables.Query.TryPartitionQuery = false
	session.batch.SetCurrent(&BatchDMLStatement{})
	if lazyAutocommitEligible(session, &DmlStatement{Dml: "INSERT INTO T (id) VALUES (1)"}) {
		t.Fatal("manual-batch DML enqueue must not acquire an owner")
	}
	if lazyAutocommitEligible(session, &RunBatchStatement{}) {
		t.Fatal("empty RUN BATCH must not be eligible")
	}
	session.batch.SetCurrent(&BatchDMLStatement{DMLs: []spanner.Statement{spanner.NewStatement("INSERT INTO T (id) VALUES (1)")}})
	if !lazyAutocommitEligible(session, &RunBatchStatement{}) {
		t.Fatal("nonempty RUN BATCH DML must be eligible")
	}
	session.batch.SetCurrent(&BulkDdlStatement{Ddls: []string{"CREATE TABLE T (id INT64) PRIMARY KEY (id)"}})
	if lazyAutocommitEligible(session, &RunBatchStatement{}) {
		t.Fatal("RUN BATCH DDL must not be eligible")
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
