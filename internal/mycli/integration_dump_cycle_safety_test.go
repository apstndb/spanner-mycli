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
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"cloud.google.com/go/spanner"
	"github.com/apstndb/spanner-mycli/internal/mycli/streamio"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func parentChildOtherDDL() []string {
	return []string{
		"CREATE TABLE Parent (Id INT64 NOT NULL) PRIMARY KEY(Id)",
		"CREATE TABLE Child (Id INT64 NOT NULL, OtherId INT64) PRIMARY KEY(Id), INTERLEAVE IN PARENT Parent ON DELETE CASCADE",
		"CREATE TABLE Other (Id INT64 NOT NULL, ChildId INT64) PRIMARY KEY(Id)",
		"ALTER TABLE Child ADD CONSTRAINT fk_child_other FOREIGN KEY(OtherId) REFERENCES Other(Id)",
		"ALTER TABLE Other ADD CONSTRAINT fk_other_child FOREIGN KEY(ChildId) REFERENCES Child(Id)",
	}
}

func dumpExpectCycleReject(t *testing.T, session *Session, stmt Statement) {
	t.Helper()
	_, err := stmt.Execute(t.Context(), session)
	if err == nil || !strings.Contains(err.Error(), dumpCyclicInsertUnsupported) {
		t.Fatalf("error = %v, want %q", err, dumpCyclicInsertUnsupported)
	}
}

func dumpExpectCycleRejectStreaming(t *testing.T, session *Session, stmt Statement) {
	t.Helper()
	var buf strings.Builder
	original := session.systemVariables.StreamManager
	session.systemVariables.StreamManager = streamio.NewStreamManager(original.GetInStream(), &buf, original.GetErrStream())
	defer func() { session.systemVariables.StreamManager = original }()
	_, err := stmt.Execute(t.Context(), session)
	if err == nil || !strings.Contains(err.Error(), dumpCyclicInsertUnsupported) {
		t.Fatalf("error = %v, want %q", err, dumpCyclicInsertUnsupported)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected zero output, got %q", buf.String())
	}
}

func TestDumpPopulatedMutualFKCycleRejected(t *testing.T) {
	t.Parallel()
	skipIfShortIntegration(t)
	_, session := initializeWithRandomDB(t, parentChildOtherDDL(), []string{
		"INSERT INTO Parent (Id) VALUES (1)",
		"INSERT INTO Child (Id, OtherId) VALUES (1, NULL)",
		"INSERT INTO Other (Id, ChildId) VALUES (1, 1)",
		"UPDATE Child SET OtherId=1 WHERE Id=1",
	})
	tables := &DumpTablesStatement{Tables: []tableID{tid("Parent"), tid("Child"), tid("Other")}}
	dumpExpectCycleReject(t, session, &DumpDatabaseStatement{})
	dumpExpectCycleRejectStreaming(t, session, &DumpDatabaseStatement{})
	dumpExpectCycleReject(t, session, tables)
	dumpExpectCycleRejectStreaming(t, session, tables)
}

func TestDumpEmptyTwoTableFKCycleSucceeds(t *testing.T) {
	t.Parallel()
	skipIfShortIntegration(t)
	ddls := []string{
		"CREATE TABLE A (Id INT64 NOT NULL, BId INT64) PRIMARY KEY(Id)",
		"CREATE TABLE B (Id INT64 NOT NULL, AId INT64) PRIMARY KEY(Id)",
		"ALTER TABLE A ADD CONSTRAINT fk_a_b FOREIGN KEY(BId) REFERENCES B(Id)",
		"ALTER TABLE B ADD CONSTRAINT fk_b_a FOREIGN KEY(AId) REFERENCES A(Id)",
	}
	_, session := initializeWithRandomDB(t, ddls, nil)
	out := dumpSQL(t, session, &DumpDatabaseStatement{})
	if !strings.Contains(out, "CREATE TABLE") {
		t.Fatalf("expected DDL:\n%s", out)
	}
	if strings.Contains(out, "INSERT") {
		t.Fatalf("empty dump emitted INSERT:\n%s", out)
	}
}

func TestDumpEmptyMutualFKCycleSucceeds(t *testing.T) {
	t.Parallel()
	skipIfShortIntegration(t)
	_, session := initializeWithRandomDB(t, parentChildOtherDDL(), nil)
	out := dumpSQL(t, session, &DumpDatabaseStatement{})
	if !strings.Contains(out, "CREATE TABLE") {
		t.Fatalf("expected DDL:\n%s", out)
	}
	if strings.Contains(out, dumpCyclicInsertUnsupported) {
		t.Fatalf("empty cycle rejected:\n%s", out)
	}
	if strings.Contains(out, "INSERT") {
		t.Fatalf("empty dump emitted INSERT:\n%s", out)
	}
	_ = dumpSQL(t, session, &DumpTablesStatement{Tables: []tableID{tid("Parent"), tid("Child"), tid("Other")}})
}

func TestDumpPopulatedSelfFKRejected(t *testing.T) {
	t.Parallel()
	skipIfShortIntegration(t)
	ddls := []string{
		"CREATE TABLE Emp (Id INT64 NOT NULL, ManagerId INT64) PRIMARY KEY(Id)",
		"ALTER TABLE Emp ADD CONSTRAINT fk_emp_mgr FOREIGN KEY(ManagerId) REFERENCES Emp(Id)",
	}
	_, session := initializeWithRandomDB(t, ddls, []string{
		"INSERT INTO Emp (Id, ManagerId) VALUES (1, NULL)",
		"INSERT INTO Emp (Id, ManagerId) VALUES (2, 1)",
		"UPDATE Emp SET ManagerId=2 WHERE Id=1",
	})
	dumpExpectCycleReject(t, session, &DumpDatabaseStatement{})
	dumpExpectCycleRejectStreaming(t, session, &DumpTablesStatement{Tables: []tableID{tid("Emp")}})
}

func TestDumpEmptySelfFKSucceeds(t *testing.T) {
	t.Parallel()
	skipIfShortIntegration(t)
	ddls := []string{
		"CREATE TABLE Emp (Id INT64 NOT NULL, ManagerId INT64) PRIMARY KEY(Id)",
		"ALTER TABLE Emp ADD CONSTRAINT fk_emp_mgr FOREIGN KEY(ManagerId) REFERENCES Emp(Id)",
	}
	_, session := initializeWithRandomDB(t, ddls, nil)
	out := dumpSQL(t, session, &DumpDatabaseStatement{})
	if !strings.Contains(out, "CREATE TABLE") {
		t.Fatalf("expected DDL:\n%s", out)
	}
}

func TestDumpPopulatedAncestorReverseFKRejected(t *testing.T) {
	t.Parallel()
	skipIfShortIntegration(t)
	ddls := []string{
		"CREATE TABLE AParent (Id INT64 NOT NULL, ChildId INT64) PRIMARY KEY (Id)",
		"CREATE TABLE ZChild (Id INT64 NOT NULL, ChildId INT64 NOT NULL) PRIMARY KEY (Id, ChildId), INTERLEAVE IN PARENT AParent ON DELETE CASCADE",
		"ALTER TABLE AParent ADD CONSTRAINT FK_Child FOREIGN KEY (Id, ChildId) REFERENCES ZChild (Id, ChildId)",
	}
	_, session := initializeWithRandomDB(t, ddls, []string{
		"INSERT INTO AParent (Id, ChildId) VALUES (1, NULL)",
		"INSERT INTO ZChild (Id, ChildId) VALUES (1, 1)",
		"UPDATE AParent SET ChildId=1 WHERE Id=1",
	})
	dumpExpectCycleReject(t, session, &DumpDatabaseStatement{})
}

func TestDumpAllNullAndRowAcyclicCyclesConservativelyRejected(t *testing.T) {
	t.Parallel()
	skipIfShortIntegration(t)
	ddls := []string{
		"CREATE TABLE A (Id INT64 NOT NULL, BId INT64) PRIMARY KEY(Id)",
		"CREATE TABLE B (Id INT64 NOT NULL, AId INT64) PRIMARY KEY(Id)",
		"ALTER TABLE A ADD CONSTRAINT fk_a_b FOREIGN KEY(BId) REFERENCES B(Id)",
		"ALTER TABLE B ADD CONSTRAINT fk_b_a FOREIGN KEY(AId) REFERENCES A(Id)",
	}
	t.Run("all_null", func(t *testing.T) {
		_, session := initializeWithRandomDB(t, ddls, []string{
			"INSERT INTO A (Id, BId) VALUES (1, NULL)",
			"INSERT INTO B (Id, AId) VALUES (1, NULL)",
		})
		dumpExpectCycleReject(t, session, &DumpDatabaseStatement{})
	})
	t.Run("row_acyclic", func(t *testing.T) {
		_, session := initializeWithRandomDB(t, ddls, []string{
			"INSERT INTO A (Id, BId) VALUES (1, NULL)",
			"INSERT INTO B (Id, AId) VALUES (1, 1)",
		})
		dumpExpectCycleReject(t, session, &DumpDatabaseStatement{})
	})
}

func TestDumpNotEnforcedCycleDoesNotReject(t *testing.T) {
	t.Parallel()
	skipIfShortIntegration(t)
	base := []string{
		"CREATE TABLE A (Id INT64 NOT NULL, BId INT64) PRIMARY KEY(Id)",
		"CREATE TABLE B (Id INT64 NOT NULL, AId INT64) PRIMARY KEY(Id)",
	}
	_, session := initializeWithRandomDB(t, base, nil)
	mustExec := func(sql string) {
		t.Helper()
		stmt, err := BuildStatement(sql)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := stmt.Execute(t.Context(), session); err != nil {
			t.Skipf("emulator did not accept NOT ENFORCED foreign keys: %v", err)
		}
	}
	mustExec("ALTER TABLE A ADD CONSTRAINT fk_a_b FOREIGN KEY(BId) REFERENCES B(Id) NOT ENFORCED")
	mustExec("ALTER TABLE B ADD CONSTRAINT fk_b_a FOREIGN KEY(AId) REFERENCES A(Id) NOT ENFORCED")
	mustExec("INSERT INTO A (Id, BId) VALUES (1, 1)")
	mustExec("INSERT INTO B (Id, AId) VALUES (1, 1)")
	out := dumpSQL(t, session, &DumpDatabaseStatement{})
	if strings.Contains(out, dumpCyclicInsertUnsupported) {
		t.Fatalf("informational cycle rejected:\n%s", out)
	}
	if !strings.Contains(out, "INSERT") {
		t.Fatalf("expected INSERT for informational cycle:\n%s", out)
	}
}

func TestDumpCyclePreflightSameTransaction(t *testing.T) {
	t.Parallel()
	skipIfShortIntegration(t)
	_, session := initializeWithRandomDB(t, parentChildOtherDDL(), nil)
	var catalog, preflight, output *spanner.ReadOnlyTransaction
	session.dumpReadTxnProbe = func(phase string, txn *spanner.ReadOnlyTransaction) {
		switch phase {
		case "catalog":
			catalog = txn
		case "preflight":
			preflight = txn
		case "output":
			output = txn
		}
	}
	_ = dumpSQL(t, session, &DumpDatabaseStatement{})
	if catalog == nil || preflight == nil || output == nil {
		t.Fatalf("missing txn observations catalog=%p preflight=%p output=%p", catalog, preflight, output)
	}
	if catalog != preflight || catalog != output {
		t.Fatalf("txn mismatch catalog=%p preflight=%p output=%p", catalog, preflight, output)
	}
}

func TestDumpCyclePreflightOrchestrationErrorIsZeroOutput(t *testing.T) {
	t.Parallel()
	skipIfShortIntegration(t)
	_, session := initializeWithRandomDB(t, parentChildOtherDDL(), nil)
	injected := errors.New("injected cycle preflight failure")
	session.dumpCyclePreflightProbe = func(_ tableID, txn *spanner.ReadOnlyTransaction) error {
		if txn == nil {
			t.Fatal("preflight txn is nil")
		}
		return injected
	}
	var buf strings.Builder
	original := session.systemVariables.StreamManager
	session.systemVariables.StreamManager = streamio.NewStreamManager(original.GetInStream(), &buf, original.GetErrStream())
	defer func() { session.systemVariables.StreamManager = original }()
	_, err := (&DumpDatabaseStatement{}).Execute(t.Context(), session)
	if !errors.Is(err, injected) {
		t.Fatalf("error = %v, want injected", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected zero output, got %q", buf.String())
	}
}

func TestDumpCycleRowQueryCanceled(t *testing.T) {
	t.Parallel()
	skipIfShortIntegration(t)
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprintf("streaming_%v", streaming), func(t *testing.T) {
			_, session := initializeWithRandomDB(t, parentChildOtherDDL(), nil)
			ctx, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)
			session.dumpCyclePreflightProbe = func(_ tableID, txn *spanner.ReadOnlyTransaction) error {
				if txn == nil {
					t.Fatal("preflight txn is nil")
				}
				cancel()
				return nil
			}
			var buf strings.Builder
			if streaming {
				original := session.systemVariables.StreamManager
				session.systemVariables.StreamManager = streamio.NewStreamManager(original.GetInStream(), &buf, original.GetErrStream())
				defer func() { session.systemVariables.StreamManager = original }()
			}
			_, err := (&DumpDatabaseStatement{}).Execute(ctx, session)
			t.Logf("row-query cancel error chain: %v", err)
			for e := err; e != nil; e = errors.Unwrap(e) {
				t.Logf("unwrap %T: %v grpc=%v", e, e, status.Code(e))
			}
			if err == nil {
				t.Fatal("expected cancellation from the row-presence query")
			}
			// The Spanner client wraps the canceled context as grpc Canceled
			// ("context canceled"). errors.Is(context.Canceled) is false on
			// that chain; status.Code is the observed contract.
			t.Logf("errors.Is(context.Canceled)=%v grpc=%v", errors.Is(err, context.Canceled), status.Code(err))
			if status.Code(err) != codes.Canceled {
				t.Fatalf("error = %v, want grpc Canceled from the row-presence query", err)
			}
			if streaming && buf.Len() != 0 {
				t.Fatalf("expected zero output, got %q", buf.String())
			}
		})
	}
}

func TestDumpMixedEnforcementOrdersAndReplays(t *testing.T) {
	t.Parallel()
	skipIfShortIntegration(t)
	ddl := []string{
		"CREATE TABLE A (Id INT64 NOT NULL, BId INT64) PRIMARY KEY(Id)",
		"CREATE TABLE B (Id INT64 NOT NULL, AId INT64) PRIMARY KEY(Id)",
		"ALTER TABLE A ADD CONSTRAINT fk_a_b_enforced FOREIGN KEY(BId) REFERENCES B(Id)",
		"ALTER TABLE B ADD CONSTRAINT fk_b_a_informational FOREIGN KEY(AId) REFERENCES A(Id) NOT ENFORCED",
	}
	_, source := initializeWithRandomDB(t, ddl, []string{
		"INSERT INTO B (Id, AId) VALUES (1, 1)",
		"INSERT INTO A (Id, BId) VALUES (1, 1)",
	})
	sql := dumpSQL(t, source, &DumpTablesStatement{Tables: []tableID{tid("A"), tid("B")}})
	aPos, bPos := strings.Index(sql, "INSERT INTO `A`"), strings.Index(sql, "INSERT INTO `B`")
	if aPos < 0 || bPos < 0 || bPos > aPos {
		t.Fatalf("want INSERT B before INSERT A, got %s", sql)
	}
	_, target := initializeWithRandomDB(t, ddl, nil)
	parts, err := separateInput(sql)
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range parts {
		text := strings.TrimSpace(part.statementWithoutComments)
		if text == "" {
			continue
		}
		stmt, err := BuildStatement(text)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := stmt.Execute(t.Context(), target); err != nil {
			t.Fatalf("%s: %v", text, err)
		}
	}
	got := dumpSQL(t, target, &DumpTablesStatement{Tables: []tableID{tid("A"), tid("B")}})
	if !strings.Contains(got, "INSERT INTO `A`") || !strings.Contains(got, "INSERT INTO `B`") {
		t.Fatalf("replay missing rows:\n%s", got)
	}
}
