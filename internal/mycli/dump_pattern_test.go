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
	"strings"
	"testing"

	"cloud.google.com/go/spanner"
	"github.com/apstndb/spanner-mycli/internal/mycli/streamio"
	"github.com/google/go-cmp/cmp"
)

func dumpPatternCatalogDDL() []string {
	return []string{
		"CREATE SCHEMA Alpha",
		"CREATE SCHEMA Beta",
		"CREATE TABLE Users (Id INT64 NOT NULL, Label STRING(16)) PRIMARY KEY(Id)",
		"CREATE TABLE Alpha.Users (Id INT64 NOT NULL, Label STRING(16)) PRIMARY KEY(Id)",
		"CREATE TABLE Beta.Users (Id INT64 NOT NULL, Label STRING(16)) PRIMARY KEY(Id)",
		"CREATE TABLE UserTest (Id INT64 NOT NULL) PRIMARY KEY(Id)",
		"CREATE TABLE Tmp (Id INT64 NOT NULL) PRIMARY KEY(Id)",
		"CREATE TABLE Alpha.Tmp (Id INT64 NOT NULL) PRIMARY KEY(Id)",
		"CREATE TABLE a_b (Id INT64 NOT NULL) PRIMARY KEY(Id)",
		"CREATE TABLE axb (Id INT64 NOT NULL) PRIMARY KEY(Id)",
		"CREATE TABLE Parent (Id INT64 NOT NULL) PRIMARY KEY(Id)",
		"CREATE TABLE Child (Id INT64 NOT NULL, ChildId INT64 NOT NULL) PRIMARY KEY(Id, ChildId), INTERLEAVE IN PARENT Parent ON DELETE CASCADE",
		"CREATE TABLE Venues (Id INT64 NOT NULL) PRIMARY KEY(Id)",
		"CREATE TABLE Concerts (Id INT64 NOT NULL, VenueId INT64 NOT NULL, CONSTRAINT ConcertsVenue FOREIGN KEY (VenueId) REFERENCES Venues(Id)) PRIMARY KEY(Id)",
		"CREATE VIEW V SQL SECURITY INVOKER AS SELECT Users.Id FROM Users",
	}
}

func dumpPatternCatalogDML() []string {
	return []string{
		"INSERT INTO Users (Id, Label) VALUES (1, 'default')",
		"INSERT INTO Alpha.Users (Id, Label) VALUES (1, 'alpha')",
		"INSERT INTO Beta.Users (Id, Label) VALUES (1, 'beta')",
		"INSERT INTO UserTest (Id) VALUES (2)",
		"INSERT INTO Tmp (Id) VALUES (3)",
		"INSERT INTO Alpha.Tmp (Id) VALUES (4)",
		"INSERT INTO a_b (Id) VALUES (5)",
		"INSERT INTO axb (Id) VALUES (6)",
		"INSERT INTO Parent (Id) VALUES (1)",
		"INSERT INTO Child (Id, ChildId) VALUES (1, 10)",
		"INSERT INTO Venues (Id) VALUES (7)",
		"INSERT INTO Concerts (Id, VenueId) VALUES (8, 7)",
	}
}

func dumpInsertTargets(output string) []string {
	var targets []string
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		const prefix = "INSERT INTO "
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		rest := strings.TrimPrefix(line, prefix)
		before, _, ok := strings.Cut(rest, " (")
		if !ok {
			continue
		}
		targets = append(targets, before)
	}
	return targets
}

func TestDumpTablesLikeExceptEmulator(t *testing.T) {
	skipIfShortIntegration(t)
	_, session := initializeWithRandomDB(t, dumpPatternCatalogDDL(), dumpPatternCatalogDML())

	dumpLike := func(t *testing.T, sql string) string {
		t.Helper()
		stmt, err := BuildStatement(sql)
		if err != nil {
			t.Fatal(err)
		}
		return dumpSQL(t, session, stmt)
	}

	t.Run("LIKE User% selects default User* only", func(t *testing.T) {
		out := dumpLike(t, "DUMP TABLES LIKE 'User%'")
		got := dumpInsertTargets(out)
		want := []string{"`UserTest`", "`Users`"}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("targets mismatch (-want +got): %s\n%s", diff, out)
		}
		if strings.Contains(out, "`Alpha`.`Users`") || strings.Contains(out, "alpha") {
			t.Fatalf("named schema leaked:\n%s", out)
		}
		if !strings.Contains(out, `"default"`) {
			t.Fatalf("missing default Users row:\n%s", out)
		}
	})

	t.Run("LIKE %.Users selects named schemas", func(t *testing.T) {
		out := dumpLike(t, "DUMP TABLES LIKE '%.Users'")
		got := dumpInsertTargets(out)
		want := []string{"`Alpha`.`Users`", "`Beta`.`Users`"}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("targets mismatch (-want +got): %s\n%s", diff, out)
		}
		if strings.Contains(out, "INSERT INTO `Users`") || strings.Contains(out, "default") {
			t.Fatalf("default Users leaked:\n%s", out)
		}
	})

	t.Run("LIKE plus EXCEPT", func(t *testing.T) {
		out := dumpLike(t, "DUMP TABLES LIKE 'User%' EXCEPT 'UserTest%'")
		if diff := cmp.Diff([]string{"`Users`"}, dumpInsertTargets(out)); diff != "" {
			t.Fatal(diff)
		}
	})

	t.Run("EXCEPT Tmp leaves other tables", func(t *testing.T) {
		out := dumpLike(t, "DUMP TABLES EXCEPT '%Tmp'")
		targets := dumpInsertTargets(out)
		joined := strings.Join(targets, ",")
		if strings.Contains(joined, "Tmp") {
			t.Fatalf("Tmp not excluded: %v", targets)
		}
		if !strings.Contains(joined, "`Users`") || !strings.Contains(joined, "`Alpha`.`Users`") {
			t.Fatalf("EXCEPT dropped too much: %v", targets)
		}
	})

	t.Run("native underscore escape", func(t *testing.T) {
		out := dumpLike(t, `DUMP TABLES LIKE r'a\_b'`)
		if diff := cmp.Diff([]string{"`a_b`"}, dumpInsertTargets(out)); diff != "" {
			t.Fatalf("%s\n%s", diff, out)
		}
		if strings.Contains(out, "INSERT INTO `axb`") {
			t.Fatalf("underscore wildcard leaked axb:\n%s", out)
		}
	})

	t.Run("child LIKE does not add parent", func(t *testing.T) {
		out := dumpLike(t, "DUMP TABLES LIKE 'Child'")
		if diff := cmp.Diff([]string{"`Child`"}, dumpInsertTargets(out)); diff != "" {
			t.Fatal(diff)
		}
		if strings.Contains(out, "INSERT INTO `Parent`") {
			t.Fatalf("parent added:\n%s", out)
		}
	})

	t.Run("concerts LIKE does not add venues", func(t *testing.T) {
		out := dumpLike(t, "DUMP TABLES LIKE 'Concerts'")
		if diff := cmp.Diff([]string{"`Concerts`"}, dumpInsertTargets(out)); diff != "" {
			t.Fatal(diff)
		}
		if strings.Contains(out, "INSERT INTO `Venues`") {
			t.Fatalf("venues added:\n%s", out)
		}
	})

	t.Run("exact list still qualified", func(t *testing.T) {
		out := dumpSQL(t, session, &DumpTablesStatement{Tables: []tableID{tid("Users")}})
		if diff := cmp.Diff([]string{"`Users`"}, dumpInsertTargets(out)); diff != "" {
			t.Fatal(diff)
		}
	})

	t.Run("DUMP DATABASE still includes all schemas", func(t *testing.T) {
		out := dumpSQL(t, session, &DumpDatabaseStatement{})
		targets := strings.Join(dumpInsertTargets(out), ",")
		for _, want := range []string{"`Users`", "`Alpha`.`Users`", "`Beta`.`Users`", "`Child`", "`Venues`"} {
			if !strings.Contains(targets, want) {
				t.Fatalf("DUMP DATABASE missing %s: %s", want, targets)
			}
		}
	})

	t.Run("LIKE percent does not include views", func(t *testing.T) {
		out := dumpLike(t, "DUMP TABLES LIKE '%'")
		if strings.Contains(out, "-- Data for table V\n") || strings.Contains(out, "INSERT INTO `V`") {
			t.Fatalf("view leaked:\n%s", out)
		}
	})

	t.Run("zero match is pre-output error", func(t *testing.T) {
		stmt, err := BuildStatement("DUMP TABLES LIKE 'NoSuch%'")
		if err != nil {
			t.Fatal(err)
		}
		var buf strings.Builder
		original := session.systemVariables.StreamManager
		session.systemVariables.StreamManager = streamio.NewStreamManager(original.GetInStream(), &buf, original.GetErrStream())
		defer func() { session.systemVariables.StreamManager = original }()
		result, execErr := stmt.Execute(t.Context(), session, OperationOutput{w: &buf})
		if !errors.Is(execErr, errDumpTablesNoMatch) {
			t.Fatalf("err=%v", execErr)
		}
		if result != nil || buf.Len() != 0 {
			t.Fatalf("zero-match leaked output result=%+v buf=%q", result, buf.String())
		}
	})

	t.Run("nil selection guard", func(t *testing.T) {
		var buf strings.Builder
		result, err := executeDump(t.Context(), session, dumpModeTables, nil, nil, OperationOutput{w: &buf})
		if !errors.Is(err, errDumpTablesMissingSelection) {
			t.Fatalf("err=%v", err)
		}
		if result != nil || buf.Len() != 0 {
			t.Fatalf("nil selection leaked output result=%+v buf=%q", result, buf.String())
		}
	})

	t.Run("selector catalog and output share snapshot", func(t *testing.T) {
		var selectorTxn, catalogTxn, outputTxn *spanner.ReadOnlyTransaction
		session.dumpReadTxnProbe = func(phase string, txn *spanner.ReadOnlyTransaction) {
			switch phase {
			case "selector":
				selectorTxn = txn
			case "catalog":
				catalogTxn = txn
			case "output":
				outputTxn = txn
			}
		}
		defer func() { session.dumpReadTxnProbe = nil }()
		stmt, err := BuildStatement("DUMP TABLES LIKE 'Users'")
		if err != nil {
			t.Fatal(err)
		}
		_ = dumpSQL(t, session, stmt)
		if selectorTxn == nil || catalogTxn == nil || outputTxn == nil {
			t.Fatalf("missing probes selector=%p catalog=%p output=%p", selectorTxn, catalogTxn, outputTxn)
		}
		if selectorTxn != catalogTxn || catalogTxn != outputTxn {
			t.Fatalf("txn mismatch selector=%p catalog=%p output=%p", selectorTxn, catalogTxn, outputTxn)
		}
	})

	t.Run("cancelled context", func(t *testing.T) {
		stmt, err := BuildStatement("DUMP TABLES LIKE '%'")
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		var buf strings.Builder
		result, execErr := stmt.Execute(ctx, session, OperationOutput{w: &buf})
		if execErr == nil {
			t.Fatal("cancelled context succeeded")
		}
		if result != nil {
			t.Fatalf("cancelled result=%+v", result)
		}
	})
}
