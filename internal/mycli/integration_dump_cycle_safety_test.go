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
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	dbadminpb "cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanner-mycli/internal/mycli/streamio"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
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

func enableDumpCyclicMutations(t *testing.T, session *Session) {
	t.Helper()
	// initializeSession deliberately uses mostly zero-value settings instead
	// of the CLI defaults. Set the new positive cap through the registry.
	defaults := newSystemVariablesWithDefaults()
	if err := session.systemVariables.SetFromSimple("CLI_DUMP_CYCLIC_MAX_BYTES", strconv.FormatInt(defaults.Display.DumpCyclicMaxBytes, 10)); err != nil {
		t.Fatal(err)
	}
	if err := session.systemVariables.SetFromSimple("CLI_DUMP_CYCLIC_MODE", "MUTATE"); err != nil {
		t.Fatal(err)
	}
}

func TestDumpCyclicMutateReplay(t *testing.T) {
	t.Parallel()
	skipIfShortIntegration(t)
	for _, tc := range []struct {
		name   string
		ddl    []string
		seed   string
		tables []tableID
		groups int
	}{
		{
			name: "mutual_not_null",
			ddl: []string{
				"CREATE TABLE A (Id INT64 NOT NULL, BId INT64 NOT NULL) PRIMARY KEY(Id)",
				"CREATE TABLE B (Id INT64 NOT NULL, AId INT64 NOT NULL) PRIMARY KEY(Id)",
				"ALTER TABLE A ADD CONSTRAINT fk_a_b FOREIGN KEY(BId) REFERENCES B(Id)",
				"ALTER TABLE B ADD CONSTRAINT fk_b_a FOREIGN KEY(AId) REFERENCES A(Id)",
			},
			seed:   "BEGIN RW; MUTATE A INSERT STRUCT<Id INT64, BId INT64>(1,1); MUTATE B INSERT STRUCT<Id INT64, AId INT64>(1,1); COMMIT;",
			tables: []tableID{tid("A"), tid("B")}, groups: 1,
		},
		{
			name: "self_not_null",
			ddl: []string{
				"CREATE TABLE A (Id INT64 NOT NULL, Ref INT64 NOT NULL, CONSTRAINT fk_self FOREIGN KEY(Ref) REFERENCES A(Id)) PRIMARY KEY(Id)",
			},
			seed:   "BEGIN RW; MUTATE A INSERT STRUCT<Id INT64, Ref INT64>(2,1); MUTATE A INSERT STRUCT<Id INT64, Ref INT64>(1,2); COMMIT;",
			tables: []tableID{tid("A")}, groups: 1,
		},
		{
			name: "ancestor_reverse",
			ddl: []string{
				"CREATE TABLE AParent (Id INT64 NOT NULL, ChildId INT64 NOT NULL) PRIMARY KEY(Id)",
				"CREATE TABLE ZChild (Id INT64 NOT NULL, ChildId INT64 NOT NULL) PRIMARY KEY(Id,ChildId), INTERLEAVE IN PARENT AParent ON DELETE CASCADE",
				"ALTER TABLE AParent ADD CONSTRAINT fk_reverse FOREIGN KEY(Id,ChildId) REFERENCES ZChild(Id,ChildId)",
			},
			seed:   "BEGIN RW; MUTATE AParent INSERT STRUCT<Id INT64, ChildId INT64>(1,2); MUTATE ZChild INSERT STRUCT<Id INT64, ChildId INT64>(1,2); COMMIT;",
			tables: []tableID{tid("AParent"), tid("ZChild")}, groups: 1,
		},
		{
			name: "acyclic_prerequisite_and_dependent",
			ddl: []string{
				"CREATE TABLE ZParent (Id INT64 NOT NULL) PRIMARY KEY(Id)",
				"CREATE TABLE M1 (Id INT64 NOT NULL, Ref INT64 NOT NULL, ParentId INT64 NOT NULL, CONSTRAINT fk_parent FOREIGN KEY(ParentId) REFERENCES ZParent(Id)) PRIMARY KEY(Id)",
				"CREATE TABLE M2 (Id INT64 NOT NULL, Ref INT64 NOT NULL, CONSTRAINT fk_m2_m1 FOREIGN KEY(Ref) REFERENCES M1(Id)) PRIMARY KEY(Id)",
				"ALTER TABLE M1 ADD CONSTRAINT fk_m1_m2 FOREIGN KEY(Ref) REFERENCES M2(Id)",
				"CREATE TABLE AChild (Id INT64 NOT NULL, Ref INT64 NOT NULL, CONSTRAINT fk_child FOREIGN KEY(Ref) REFERENCES M2(Id)) PRIMARY KEY(Id)",
			},
			seed: "INSERT INTO ZParent(Id) VALUES(1); BEGIN RW; " +
				"MUTATE M1 INSERT STRUCT<Id INT64, Ref INT64, ParentId INT64>(1,1,1); " +
				"MUTATE M2 INSERT STRUCT<Id INT64, Ref INT64>(1,1); COMMIT; INSERT INTO AChild(Id,Ref) VALUES(1,1);",
			tables: []tableID{tid("AChild"), tid("M1"), tid("M2"), tid("ZParent")}, groups: 1,
		},
		{
			name: "named_same_basename_and_independent_cycle",
			ddl: []string{
				"CREATE SCHEMA Alpha", "CREATE SCHEMA Beta",
				"CREATE TABLE Alpha.T (Id INT64 NOT NULL, Ref INT64 NOT NULL) PRIMARY KEY(Id)",
				"CREATE TABLE Beta.T (Id INT64 NOT NULL, Ref INT64 NOT NULL) PRIMARY KEY(Id)",
				"ALTER TABLE Alpha.T ADD CONSTRAINT fk_alpha FOREIGN KEY(Ref) REFERENCES Beta.T(Id)",
				"ALTER TABLE Beta.T ADD CONSTRAINT fk_beta FOREIGN KEY(Ref) REFERENCES Alpha.T(Id)",
				"CREATE TABLE ZSelf (Id INT64 NOT NULL, Ref INT64 NOT NULL, CONSTRAINT fk_self FOREIGN KEY(Ref) REFERENCES ZSelf(Id)) PRIMARY KEY(Id)",
			},
			seed:   "BEGIN RW; MUTATE Alpha.T INSERT STRUCT<Id INT64, Ref INT64>(1,1); MUTATE Beta.T INSERT STRUCT<Id INT64, Ref INT64>(1,1); MUTATE ZSelf INSERT STRUCT<Id INT64, Ref INT64>(1,1); COMMIT;",
			tables: []tableID{tidn("Alpha", "T"), tidn("Beta", "T"), tid("ZSelf")}, groups: 2,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, source := initializeWithRandomDB(t, tc.ddl, nil)
			replayDumpSQL(t, source, tc.seed)
			enableDumpCyclicMutations(t, source)
			for _, mode := range []string{"TABLES", "DATABASE"} {
				t.Run(mode, func(t *testing.T) {
					var stmt Statement = &DumpTablesStatement{Tables: tc.tables}
					if mode == "DATABASE" {
						stmt = &DumpDatabaseStatement{}
					}
					buffered := dumpSQL(t, source, stmt)
					streamed := dumpStreamingSQL(t, source, stmt)
					if buffered != streamed {
						t.Fatal("buffered and streaming output differ")
					}
					if strings.Count(buffered, "\nBEGIN RW;\n") != tc.groups || strings.Count(buffered, "\nCOMMIT;\n") != tc.groups {
						t.Fatalf("wrong transaction boundaries:\n%s", buffered)
					}
					if !strings.Contains(buffered, dumpCyclicWarning) {
						t.Fatal("partial-restore warning absent")
					}
					var ddl []string
					if mode == "TABLES" {
						ddl = tc.ddl
					}
					_, target := initializeWithRandomDB(t, ddl, nil)
					replayDumpSQL(t, target, buffered)
					enableDumpCyclicMutations(t, target)
					// Re-encode target rows through the same production path to
					// compare every typed field and deterministic row order.
					tableStmt := &DumpTablesStatement{Tables: tc.tables}
					want := dumpSQL(t, source, tableStmt)
					if got := dumpSQL(t, target, tableStmt); got != want {
						t.Fatalf("restored rows differ:\nwant %s\ngot %s", want, got)
					}
					for _, table := range tc.tables {
						assertDumpTableValuesEqual(t, source, target, quoteTableID(source.systemVariables.Feature.DatabaseDialect, table))
					}
				})
			}
		})
	}
}

func TestDumpCyclicMutateEmptyOutputUnchanged(t *testing.T) {
	t.Parallel()
	skipIfShortIntegration(t)
	_, source := initializeWithRandomDB(t, parentChildOtherDDL(), []string{"INSERT INTO Parent (Id) VALUES (1)"})
	for _, stmt := range []Statement{&DumpDatabaseStatement{}, &DumpTablesStatement{Tables: []tableID{tid("Child"), tid("Other"), tid("Parent")}}} {
		source.systemVariables.Display.DumpCyclicMode = enums.DumpCyclicModeReject
		want := dumpSQL(t, source, stmt)
		enableDumpCyclicMutations(t, source)
		if got := dumpSQL(t, source, stmt); got != want {
			t.Fatalf("empty cycles changed output:\nwant %s\ngot %s", want, got)
		}
		if got := dumpStreamingSQL(t, source, stmt); got != want {
			t.Fatal("empty streaming cycles changed output")
		}
	}
}

func TestDumpCyclicMutateSubsetKeepsTargetConstraints(t *testing.T) {
	t.Parallel()
	skipIfShortIntegration(t)
	ddl := []string{
		"CREATE TABLE A (Id INT64 NOT NULL, Ref INT64) PRIMARY KEY(Id)",
		"CREATE TABLE B (Id INT64 NOT NULL, Ref INT64, CONSTRAINT fk_b FOREIGN KEY(Ref) REFERENCES A(Id)) PRIMARY KEY(Id)",
		"ALTER TABLE A ADD CONSTRAINT fk_a FOREIGN KEY(Ref) REFERENCES B(Id)",
	}
	_, source := initializeWithRandomDB(t, ddl, nil)
	replayDumpSQL(t, source, "BEGIN RW; MUTATE A INSERT STRUCT<Id INT64, Ref INT64>(1,1); MUTATE B INSERT STRUCT<Id INT64, Ref INT64>(1,1); COMMIT;")
	enableDumpCyclicMutations(t, source)
	stmt := &DumpTablesStatement{Tables: []tableID{tid("A")}}
	output := dumpSQL(t, source, stmt)
	if output != dumpStreamingSQL(t, source, stmt) {
		t.Fatal("subset buffered and streamed output differ")
	}
	if !strings.Contains(output, "INSERT INTO `A`") || strings.Contains(output, "MUTATE ") || strings.Contains(output, "BEGIN RW") || strings.Contains(output, "ALTER TABLE") {
		t.Fatalf("a selected singleton must retain ordinary INSERT and never change target constraints:\n%s", output)
	}
	for _, prerequisites := range []bool{false, true} {
		t.Run(fmt.Sprintf("prerequisites_%v", prerequisites), func(t *testing.T) {
			var seed []string
			if prerequisites {
				// The omitted table is a caller-provided target prerequisite.
				// Its rows need not match the omitted source rows.
				seed = []string{"INSERT INTO B(Id,Ref) VALUES(1,NULL)"}
			}
			_, target := initializeWithRandomDB(t, ddl, seed)
			if prerequisites {
				replayDumpSQL(t, target, output)
				assertDumpTableValuesEqual(t, source, target, "A")
			} else {
				parts, err := separateInput(output)
				if err != nil {
					t.Fatal(err)
				}
				var replayErr error
				for _, part := range parts {
					if strings.TrimSpace(part.statementWithoutComments) == "" {
						continue
					}
					stmt, err := BuildStatement(part.statementWithoutComments)
					if err != nil {
						t.Fatal(err)
					}
					if _, replayErr = stmt.Execute(t.Context(), target); replayErr != nil {
						break
					}
				}
				if status.Code(replayErr) != codes.FailedPrecondition || !strings.Contains(replayErr.Error(), "fk_a") {
					t.Fatalf("missing prerequisite did not fail the existing FK: %v", replayErr)
				}
			}
			before, err := source.adminClient.GetDatabaseDdl(t.Context(), &dbadminpb.GetDatabaseDdlRequest{Database: source.systemVariables.DatabasePath()})
			if err != nil {
				t.Fatal(err)
			}
			after, err := target.adminClient.GetDatabaseDdl(t.Context(), &dbadminpb.GetDatabaseDdlRequest{Database: target.systemVariables.DatabasePath()})
			if err != nil {
				t.Fatal(err)
			}
			if !proto.Equal(before, after) {
				t.Fatalf("subset restore changed target schema: %v != %v", before, after)
			}
		})
	}
}

func twoSelfCyclesDDL() []string {
	return []string{
		"CREATE TABLE A (Id INT64 NOT NULL, Ref INT64 NOT NULL, V INT64, CONSTRAINT fk_a FOREIGN KEY(Ref) REFERENCES A(Id)) PRIMARY KEY(Id)",
		"CREATE TABLE Z (Id INT64 NOT NULL, Ref INT64 NOT NULL, V INT64, CONSTRAINT fk_z FOREIGN KEY(Ref) REFERENCES Z(Id)) PRIMARY KEY(Id)",
	}
}

func TestDumpCyclicMutateOutputDoesNotRescan(t *testing.T) {
	t.Parallel()
	skipIfShortIntegration(t)
	for _, populated := range []bool{false, true} {
		for _, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("populated_%v/streaming_%v", populated, streaming), func(t *testing.T) {
				_, source := initializeWithRandomDB(t, twoSelfCyclesDDL(), nil)
				if populated {
					replayDumpSQL(t, source, twoSelfCyclesSeed)
				}
				enableDumpCyclicMutations(t, source)
				stmt := &DumpTablesStatement{Tables: []tableID{tid("A"), tid("Z")}}
				want := dumpSQL(t, source, stmt)
				var catalog *spanner.ReadOnlyTransaction
				preflightCount, outputCount := 0, 0
				source.dumpReadTxnProbe = func(phase string, txn *spanner.ReadOnlyTransaction) {
					switch phase {
					case "catalog":
						catalog = txn
					case "preflight":
						preflightCount++
						if txn != catalog || catalog == nil {
							t.Fatal("cyclic preflight did not use the catalog transaction")
						}
					case "output":
						outputCount++
						if txn != catalog || preflightCount != 2 {
							t.Fatalf("output before both cyclic scans: count=%d", preflightCount)
						}
						// All cyclic data, including empty-group decisions, must
						// already be retained. Any read on this SDK transaction
						// now fails; a fresh-transaction rescan sees changed rows.
						txn.Close()
						ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
						defer cancel()
						_, err := source.client.Apply(ctx, []*spanner.Mutation{
							spanner.InsertOrUpdate("A", []string{"Id", "Ref", "V"}, []any{int64(1), int64(1), int64(999)}),
							spanner.InsertOrUpdate("Z", []string{"Id", "Ref", "V"}, []any{int64(1), int64(1), int64(999)}),
						})
						if err != nil {
							t.Fatalf("change source after preflight: %v", err)
						}
					}
				}
				defer func() { source.dumpReadTxnProbe = nil }()
				var got string
				if streaming {
					got = dumpStreamingSQL(t, source, stmt)
				} else {
					got = dumpSQL(t, source, stmt)
				}
				if got != want || outputCount != 1 {
					t.Fatalf("output rescanned source: outputCount=%d\nwant %s\ngot %s", outputCount, want, got)
				}
				source.dumpReadTxnProbe = nil
				if fresh := dumpSQL(t, source, stmt); fresh == want {
					t.Fatal("negative assertion: subsequent dump did not see the committed source change")
				}
			})
		}
	}
}

func TestDumpCyclicMutateNoWritableColumns(t *testing.T) {
	t.Parallel()
	skipIfShortIntegration(t)
	_, session := initializeWithRandomDB(t, []string{"CREATE TABLE T (Id INT64 NOT NULL) PRIMARY KEY(Id)"}, nil)
	// Exercise the planner's no-writable-column contract directly. This is
	// not evidence that an all-generated-column cyclic schema is service-valid.
	check := func(wantPopulated bool) {
		t.Helper()
		err := session.txn.withReadOnlyTransactionOrStart(t.Context(), func(txn *spanner.ReadOnlyTransaction) error {
			data, err := prepareDumpCyclicData(t.Context(), session, txn, []dumpTablePlan{{ID: tid("T")}}, &dumpCyclicBudget{limit: 64 << 20}, nil)
			if wantPopulated {
				if err == nil || !strings.Contains(err.Error(), "populated cyclic DUMP table T has no writable columns") {
					t.Fatalf("got %v, want populated-table rejection", err)
				}
			} else if err != nil || len(data.Statements) != 0 {
				t.Fatalf("empty no-writable table: %v, %v", data, err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	check(false)
	replayDumpSQL(t, session, "INSERT INTO T (Id) VALUES (1);")
	check(true)
}

const twoSelfCyclesSeed = "BEGIN RW; " +
	"MUTATE A INSERT STRUCT<Id INT64, Ref INT64, V INT64>(1,1,10); " +
	"MUTATE Z INSERT STRUCT<Id INT64, Ref INT64, V INT64>(1,2,10); " +
	"MUTATE Z INSERT STRUCT<Id INT64, Ref INT64, V INT64>(2,1,20); COMMIT;"

func TestDumpCyclicMutatePlanningFailures(t *testing.T) {
	t.Parallel()
	skipIfShortIntegration(t)
	_, source := initializeWithRandomDB(t, twoSelfCyclesDDL(), nil)
	replayDumpSQL(t, source, twoSelfCyclesSeed)
	enableDumpCyclicMutations(t, source)
	firstGroup := dumpSQL(t, source, &DumpTablesStatement{Tables: []tableID{tid("A")}})
	for _, failTable := range []tableID{tid("A"), tid("Z")} {
		for _, fault := range []string{"probe", "cancel", "closed_iterator", "cap"} {
			for _, streaming := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/streaming_%v", failTable.Name, fault, streaming), func(t *testing.T) {
					enableDumpCyclicMutations(t, source)
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					injected := errors.New("injected preflight failure")
					var catalog *spanner.ReadOnlyTransaction
					var visited []tableID
					outputSeen := false
					source.dumpReadTxnProbe = func(phase string, txn *spanner.ReadOnlyTransaction) {
						if phase == "catalog" {
							catalog = txn
						}
						if phase == "output" {
							outputSeen = true
						}
						if phase == "preflight" && (catalog == nil || txn != catalog) {
							t.Fatal("cyclic scan uses another snapshot")
						}
					}
					source.dumpCyclePreflightProbe = func(id tableID, txn *spanner.ReadOnlyTransaction) error {
						visited = append(visited, id)
						if id != failTable {
							return nil
						}
						switch fault {
						case "probe":
							return injected
						case "cancel":
							cancel()
						case "closed_iterator":
							txn.Close() // Actual SDK iterator failure, not a returned sentinel.
						}
						return nil
					}
					defer func() { source.dumpReadTxnProbe = nil; source.dumpCyclePreflightProbe = nil }()
					if fault == "cap" {
						limit := int64(1)
						if failTable.Name == "Z" {
							limit = int64(len(firstGroup))
						}
						if err := source.systemVariables.SetFromSimple("CLI_DUMP_CYCLIC_MAX_BYTES", strconv.FormatInt(limit, 10)); err != nil {
							t.Fatal(err)
						}
					}
					var out strings.Builder
					var result *Result
					var err error
					run := func() error { result, err = (&DumpDatabaseStatement{}).Execute(ctx, source); return err }
					if streaming {
						err = source.withOutput(outputContext{w: &out}, run)
					} else {
						err = run()
					}
					if err == nil {
						t.Fatal("expected pre-output failure")
					}
					if fault == "probe" && !errors.Is(err, injected) {
						t.Fatalf("wrong error: %v", err)
					}
					if fault == "cancel" && !errors.Is(err, context.Canceled) && status.Code(err) != codes.Canceled {
						t.Fatalf("wrong cancel error: %v", err)
					}
					if fault == "cap" && (!strings.Contains(err.Error(), "CLI_DUMP_CYCLIC_MAX_BYTES") || !strings.Contains(err.Error(), "table "+failTable.Name)) {
						t.Fatalf("wrong cap failure: %v", err)
					}
					if result != nil || outputSeen || out.Len() != 0 {
						t.Fatalf("partial output: result=%v probe=%v text=%s", result, outputSeen, out.String())
					}
					if len(visited) == 0 || visited[len(visited)-1] != failTable {
						t.Fatalf("wrong failing group: %v", visited)
					}
				})
			}
		}
	}
}

func TestDumpCyclicMutateSecondCommitFailure(t *testing.T) {
	t.Parallel()
	skipIfShortIntegration(t)
	_, source := initializeWithRandomDB(t, twoSelfCyclesDDL(), nil)
	replayDumpSQL(t, source, twoSelfCyclesSeed)
	enableDumpCyclicMutations(t, source)
	output := dumpSQL(t, source, &DumpTablesStatement{Tables: []tableID{tid("A"), tid("Z")}})
	ddl := append(twoSelfCyclesDDL(), "ALTER TABLE Z ADD CONSTRAINT reject_twenty CHECK(V != 20)")
	_, target := initializeWithRandomDB(t, ddl, nil)
	parts, err := separateInput(output)
	if err != nil {
		t.Fatal(err)
	}
	commits := 0
	var replayError error
	for _, part := range parts {
		if strings.TrimSpace(part.statementWithoutComments) == "" {
			continue
		}
		stmt, err := BuildStatementWithComments(part.statementWithoutComments, part.statement)
		if err != nil {
			t.Fatal(err)
		}
		_, isCommit := stmt.(*CommitStatement)
		if isCommit {
			commits++
		}
		if _, err := stmt.Execute(t.Context(), target); err != nil {
			if !isCommit || commits != 2 {
				t.Fatalf("failure before second COMMIT: %s: %v", part.statement, err)
			}
			replayError = err
			break
		}
	}
	if replayError == nil {
		t.Fatal("second cyclic transaction unexpectedly committed")
	}
	for _, tc := range []struct {
		table string
		want  int64
	}{{"A", 1}, {"Z", 0}} {
		iter := target.client.Single().Query(t.Context(), spanner.Statement{SQL: "SELECT COUNT(*) FROM " + tc.table})
		row, err := iter.Next()
		iter.Stop()
		if err != nil {
			t.Fatal(err)
		}
		var got int64
		if err := row.Column(0, &got); err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Fatalf("%s count=%d, want %d after second SCC failure", tc.table, got, tc.want)
		}
	}
	if !strings.Contains(output, dumpCyclicWarning) {
		t.Fatal("partial-restore warning absent")
	}
}

// assertDumpWireValueEqual checks the original service values, independently
// of the encoder. proto.Equal alone considers positive and negative zero equal.
func assertDumpWireValueEqual(t *testing.T, want, got *structpb.Value) {
	t.Helper()
	if !proto.Equal(want, got) {
		t.Fatalf("wire value changed: %v -> %v", want, got)
	}
	if _, ok := want.GetKind().(*structpb.Value_NumberValue); ok {
		if math.Signbit(want.GetNumberValue()) != math.Signbit(got.GetNumberValue()) {
			t.Fatalf("numeric sign changed: %v -> %v", want, got)
		}
	}
	for i, v := range want.GetListValue().GetValues() {
		assertDumpWireValueEqual(t, v, got.GetListValue().Values[i])
	}
}

func assertDumpTableValuesEqual(t *testing.T, source, target *Session, table string) {
	t.Helper()
	query := spanner.Statement{SQL: "SELECT * FROM " + table + " ORDER BY Id"}
	readRows := func(session *Session) []*spanner.Row {
		t.Helper()
		var rows []*spanner.Row
		err := session.client.Single().Query(t.Context(), query).Do(func(row *spanner.Row) error {
			rows = append(rows, row)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	sourceRows, targetRows := readRows(source), readRows(target)
	if len(sourceRows) != len(targetRows) {
		t.Fatalf("row counts: %d != %d", len(sourceRows), len(targetRows))
	}
	for i, want := range sourceRows {
		got := targetRows[i]
		if want.Size() != got.Size() {
			t.Fatal("column count changed")
		}
		for col := range want.Size() {
			if want.ColumnName(col) != got.ColumnName(col) || !proto.Equal(want.ColumnType(col), got.ColumnType(col)) {
				t.Fatalf("metadata mismatch at %s", want.ColumnName(col))
			}
			assertDumpWireValueEqual(t, want.ColumnValue(col), got.ColumnValue(col))
		}
	}
}

func TestDumpCyclicMutateTypeMatrix(t *testing.T) {
	t.Parallel()
	skipIfShortIntegration(t)
	for _, tc := range []struct {
		name, typ   string
		expressions []string
	}{
		{"bool", "BOOL", []string{"TRUE", "FALSE"}},
		{"int", "INT64", []string{"9223372036854775807", "-9223372036854775808"}},
		{"float32", "FLOAT32", []string{"CAST('-0' AS FLOAT32)", "CAST(0 AS FLOAT32)", "CAST(1.5 AS FLOAT32)", "CAST('nan' AS FLOAT32)", "CAST('inf' AS FLOAT32)", "CAST('-inf' AS FLOAT32)"}},
		{"float64", "FLOAT64", []string{"CAST('-0' AS FLOAT64)", "0.0", "1.5", "CAST('nan' AS FLOAT64)", "CAST('inf' AS FLOAT64)", "CAST('-inf' AS FLOAT64)"}},
		{"string", "STRING(MAX)", []string{`'CAST(-0 AS FLOAT32)'`, `'quote" slash\\\r\ntext'`}},
		{"bytes", "BYTES(MAX)", []string{`b'\x00\xff\r\n'`}},
		{"numeric", "NUMERIC", []string{"NUMERIC '12345678901234567890.123456789'"}},
		{"json", "JSON", []string{`JSON '{"x":[1,null,"CAST(-0 AS FLOAT32)"]}'`}},
		{"date", "DATE", []string{"DATE '2026-09-08'"}},
		{"timestamp", "TIMESTAMP", []string{"TIMESTAMP '2026-09-07T22:00:00.123456Z'"}},
		{"uuid", "UUID", []string{"CAST('01234567-89ab-cdef-0123-456789abcdef' AS UUID)"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ddl := fmt.Sprintf("CREATE TABLE T (Id INT64 NOT NULL, Ref INT64 NOT NULL, `select` %s, Items ARRAY<%s>, EmptyItems ARRAY<%s>, NullItems ARRAY<%s>, NullValue %s, DefaultV INT64 DEFAULT(7), GeneratedV INT64 AS (DefaultV+1) STORED, CONSTRAINT fk_self FOREIGN KEY(Ref) REFERENCES T(Id)) PRIMARY KEY(Id)", tc.typ, tc.typ, tc.typ, tc.typ, tc.typ)
			var inserts []string
			for i, expr := range tc.expressions {
				inserts = append(inserts, fmt.Sprintf("INSERT INTO T(Id,Ref,`select`,Items,EmptyItems,NullItems,NullValue) VALUES(%d,%d,%s,[%s,NULL],[],NULL,NULL)", i, i, expr, expr))
			}
			_, source := initializeWithRandomDB(t, []string{ddl}, inserts)
			enableDumpCyclicMutations(t, source)
			for _, streaming := range []bool{false, true} {
				t.Run(fmt.Sprintf("streaming_%v", streaming), func(t *testing.T) {
					// A changed target default must not replace the observed source value.
					_, target := initializeWithRandomDB(t, []string{strings.Replace(ddl, "DEFAULT(7)", "DEFAULT(99)", 1)}, nil)
					stmt := &DumpTablesStatement{Tables: []tableID{tid("T")}}
					var output string
					if streaming {
						output = dumpStreamingSQL(t, source, stmt)
					} else {
						output = dumpSQL(t, source, stmt)
					}
					if strings.Contains(output, "GeneratedV") || !strings.Contains(output, "DefaultV") {
						t.Fatal("writable-column projection is wrong")
					}
					replayDumpSQL(t, target, output)
					assertDumpTableValuesEqual(t, source, target, "T")
				})
			}
		})
	}
}

func TestDumpCyclicMutateProtoEnum(t *testing.T) {
	t.Parallel()
	skipIfShortIntegration(t)
	_, source := initializeWithRandomDB(t, nil, nil)
	setup := `SET CLI_PROTO_DESCRIPTOR_FILE = "testdata/protos/singer.proto";
		CREATE PROTO BUNDLE (` + "`examples.spanner.music.SingerInfo`, `examples.spanner.music.Genre`" + `);
		CREATE TABLE T (Id INT64 NOT NULL, Ref INT64 NOT NULL,
		P examples.spanner.music.SingerInfo, E examples.spanner.music.Genre,
		AP ARRAY<examples.spanner.music.SingerInfo>, AE ARRAY<examples.spanner.music.Genre>,
		CONSTRAINT fk_self FOREIGN KEY(Ref) REFERENCES T(Id)) PRIMARY KEY(Id);`
	replayDumpSQL(t, source, setup)
	replayDumpSQL(t, source, `INSERT INTO T(Id,Ref,P,E,AP,AE) VALUES
		(1,1,CAST(b'\x08\x07' AS examples.spanner.music.SingerInfo),CAST(3 AS examples.spanner.music.Genre),[CAST(b'\x08\x07' AS examples.spanner.music.SingerInfo),NULL],[CAST(3 AS examples.spanner.music.Genre),NULL]),
		(2,2,NULL,NULL,[],[]),(3,3,NULL,NULL,NULL,NULL);`)
	enableDumpCyclicMutations(t, source)
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprintf("streaming_%v", streaming), func(t *testing.T) {
			_, target := initializeWithRandomDB(t, nil, nil)
			replayDumpSQL(t, target, setup)
			stmt := &DumpTablesStatement{Tables: []tableID{tid("T")}}
			var output string
			if streaming {
				output = dumpStreamingSQL(t, source, stmt)
			} else {
				output = dumpSQL(t, source, stmt)
			}
			if !strings.Contains(output, "`P` BYTES") || !strings.Contains(output, "`E` INT64") || !strings.Contains(output, "`AP` ARRAY<BYTES>") || !strings.Contains(output, "`AE` ARRAY<INT64>") {
				t.Fatalf("missing wire surrogates:\n%s", output)
			}
			replayDumpSQL(t, target, output)
			assertDumpTableValuesEqual(t, source, target, "T")
		})
	}
}
