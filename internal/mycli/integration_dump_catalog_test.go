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
	"time"

	"cloud.google.com/go/spanner"
	adminpb "cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/internal/mycli/streamio"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func skipIfShortIntegration(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
}

func fourParentCatalogDDL() []string {
	return []string{
		"CREATE SCHEMA Alpha",
		"CREATE SCHEMA Beta",
		"CREATE TABLE Parent (Id INT64 NOT NULL) PRIMARY KEY(Id)",
		"CREATE TABLE Alpha.Parent (Id INT64 NOT NULL) PRIMARY KEY(Id)",
		"CREATE TABLE Beta.Parent (Id INT64 NOT NULL) PRIMARY KEY(Id)",
		"CREATE TABLE Alpha.SameChild (Id INT64 NOT NULL, ChildId INT64 NOT NULL) PRIMARY KEY(Id, ChildId), INTERLEAVE IN PARENT Alpha.Parent ON DELETE CASCADE",
		"CREATE TABLE Beta.CrossChild (Id INT64 NOT NULL, ChildId INT64 NOT NULL) PRIMARY KEY(Id, ChildId), INTERLEAVE IN PARENT Alpha.Parent ON DELETE CASCADE",
		"CREATE TABLE Alpha.DefaultChild (Id INT64 NOT NULL, ChildId INT64 NOT NULL) PRIMARY KEY(Id, ChildId), INTERLEAVE IN PARENT Parent ON DELETE CASCADE",
		"CREATE TABLE NamedChild (Id INT64 NOT NULL, ChildId INT64 NOT NULL) PRIMARY KEY(Id, ChildId), INTERLEAVE IN PARENT Alpha.Parent ON DELETE CASCADE",
	}
}

func dumpSQL(t *testing.T, session *Session, stmt Statement) string {
	t.Helper()
	result, err := stmt.Execute(t.Context(), session)
	if err != nil {
		t.Fatal(err)
	}
	return dumpRenderedOutputForTest(t, result)
}

func dumpStreamingSQL(t *testing.T, session *Session, stmt Statement) string {
	t.Helper()
	var buf strings.Builder
	original := session.systemVariables.StreamManager
	session.systemVariables.StreamManager = streamio.NewStreamManager(
		original.GetInStream(),
		&buf,
		original.GetErrStream(),
	)
	defer func() { session.systemVariables.StreamManager = original }()
	result, err := stmt.Execute(t.Context(), session)
	if err != nil {
		t.Fatal(err)
	}
	if !result.alreadyDelivered() {
		t.Fatal("expected streamed dump")
	}
	return buf.String()
}

func catalogInterleaveParent(t *testing.T, session *Session, selected []tableID, child tableID) *tableID {
	t.Helper()
	dr := NewDependencyResolver()
	err := session.txn.withReadOnlyTransactionOrStart(t.Context(), func(txn *spanner.ReadOnlyTransaction) error {
		return dr.BuildDependencyGraphWithTxn(t.Context(), txn, nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	ddl, err := session.GetDatabaseDdlFresh(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := dr.applyInterleaveParents(ddl.GetStatements(), selected); err != nil {
		t.Fatal(err)
	}
	dep := dr.tables[child]
	if dep == nil {
		t.Fatalf("missing child %s", child.FQN())
	}
	return dep.InterleaveParent
}

func dataCommentIndex(output, fqn string) int {
	return strings.Index(output, "-- Data for table "+fqn)
}

func TestDumpCatalogFourParentEdges(t *testing.T) {
	t.Parallel()
	skipIfShortIntegration(t)
	_, session := initializeWithRandomDB(t, fourParentCatalogDDL(), nil)

	all := []tableID{
		tid("Parent"), tidn("Alpha", "Parent"), tidn("Beta", "Parent"),
		tidn("Alpha", "SameChild"), tidn("Beta", "CrossChild"),
		tidn("Alpha", "DefaultChild"), tid("NamedChild"),
	}
	assertParent := func(child, want tableID) {
		t.Helper()
		got := catalogInterleaveParent(t, session, all, child)
		if got == nil || *got != want {
			t.Fatalf("%s parent = %v, want %s", child.FQN(), got, want.FQN())
		}
	}
	assertParent(tidn("Alpha", "SameChild"), tidn("Alpha", "Parent"))
	assertParent(tidn("Beta", "CrossChild"), tidn("Alpha", "Parent"))
	assertParent(tidn("Alpha", "DefaultChild"), tid("Parent"))
	assertParent(tid("NamedChild"), tidn("Alpha", "Parent"))

	if got := catalogInterleaveParent(t, session, []tableID{tidn("Beta", "CrossChild")}, tidn("Beta", "CrossChild")); got != nil {
		t.Fatalf("child-only edge %v", got)
	}
	if got := catalogInterleaveParent(t, session, []tableID{tidn("Beta", "CrossChild"), tidn("Beta", "Parent")}, tidn("Beta", "CrossChild")); got != nil {
		t.Fatalf("same-schema fallback bound %s", got.FQN())
	}

	out := dumpSQL(t, session, &DumpTablesStatement{Tables: []tableID{tidn("Beta", "CrossChild"), tidn("Beta", "Parent")}})
	cross := dataCommentIndex(out, "Beta.CrossChild")
	betaParent := dataCommentIndex(out, "Beta.Parent")
	if cross == -1 || betaParent == -1 {
		t.Fatalf("missing comments:\n%s", out)
	}
	if betaParent < cross {
		t.Fatalf("wrong same-schema resolution ordered Beta.Parent first:\n%s", out)
	}

	trueOut := dumpSQL(t, session, &DumpTablesStatement{Tables: []tableID{tidn("Beta", "CrossChild"), tidn("Alpha", "Parent")}})
	if dataCommentIndex(trueOut, "Alpha.Parent") > dataCommentIndex(trueOut, "Beta.CrossChild") {
		t.Fatalf("true parent must precede child:\n%s", trueOut)
	}
}

func TestDumpCatalogNamedUsersBufferedStreamingReplay(t *testing.T) {
	t.Parallel()
	skipIfShortIntegration(t)
	ddls := []string{
		"CREATE SCHEMA Alpha",
		"CREATE SCHEMA Beta",
		"CREATE TABLE Users (Id INT64 NOT NULL, Label STRING(16)) PRIMARY KEY(Id)",
		"CREATE TABLE Alpha.Users (Id INT64 NOT NULL, Label STRING(16)) PRIMARY KEY(Id)",
		"CREATE TABLE Beta.Users (Id INT64 NOT NULL, Label STRING(16)) PRIMARY KEY(Id)",
	}
	dmls := []string{
		"INSERT INTO Users (Id, Label) VALUES (1, 'default')",
		"INSERT INTO Alpha.Users (Id, Label) VALUES (1, 'alpha')",
		"INSERT INTO Beta.Users (Id, Label) VALUES (1, 'beta')",
	}
	_, session := initializeWithRandomDB(t, ddls, dmls)

	defaultOnly := dumpSQL(t, session, &DumpTablesStatement{Tables: []tableID{tid("Users")}})
	if !strings.Contains(defaultOnly, "INSERT INTO `Users`") || strings.Contains(defaultOnly, "`Alpha`.`Users`") || strings.Contains(defaultOnly, "alpha") {
		t.Fatalf("name-only identity leaked named-schema rows:\n%s", defaultOnly)
	}
	alphaOnly := dumpSQL(t, session, &DumpTablesStatement{Tables: []tableID{tidn("Alpha", "Users")}})
	if !strings.Contains(alphaOnly, "INSERT INTO `Alpha`.`Users`") || strings.Contains(alphaOnly, "default") || strings.Contains(alphaOnly, "beta") {
		t.Fatalf("Alpha.Users dump mismatch:\n%s", alphaOnly)
	}
	betaOnly := dumpSQL(t, session, &DumpTablesStatement{Tables: []tableID{tidn("Beta", "Users")}})
	if !strings.Contains(betaOnly, "INSERT INTO `Beta`.`Users`") || strings.Contains(betaOnly, "default") || strings.Contains(betaOnly, "alpha") {
		t.Fatalf("Beta.Users dump mismatch:\n%s", betaOnly)
	}
	allThree := &DumpTablesStatement{Tables: []tableID{tid("Users"), tidn("Alpha", "Users"), tidn("Beta", "Users")}}
	bufferedTables := dumpSQL(t, session, allThree)
	streamedTables := dumpStreamingSQL(t, session, allThree)
	for _, output := range []string{bufferedTables, streamedTables} {
		if !strings.Contains(output, "INSERT INTO `Users`") ||
			!strings.Contains(output, "INSERT INTO `Alpha`.`Users`") ||
			!strings.Contains(output, "INSERT INTO `Beta`.`Users`") ||
			!strings.Contains(output, "default") ||
			!strings.Contains(output, "alpha") ||
			!strings.Contains(output, "beta") {
			t.Fatalf("explicit three-table dump missing identities:\n%s", output)
		}
	}

	buffered := dumpSQL(t, session, &DumpDatabaseStatement{})
	streamed := dumpStreamingSQL(t, session, &DumpDatabaseStatement{})
	for _, output := range []string{buffered, streamed} {
		if !strings.Contains(output, "INSERT INTO `Users`") ||
			!strings.Contains(output, "INSERT INTO `Alpha`.`Users`") ||
			!strings.Contains(output, "INSERT INTO `Beta`.`Users`") {
			t.Fatalf("DUMP DATABASE missing qualified INSERT targets:\n%s", output)
		}
		if dataCommentIndex(output, "Users") == -1 ||
			dataCommentIndex(output, "Alpha.Users") == -1 ||
			dataCommentIndex(output, "Beta.Users") == -1 {
			t.Fatalf("DUMP DATABASE missing table comments:\n%s", output)
		}
	}
	if diff := cmp.Diff(buffered, streamed); diff != "" {
		t.Fatalf("buffered vs streaming dump differ: %s", diff)
	}
	second := dumpSQL(t, session, &DumpDatabaseStatement{})
	if diff := cmp.Diff(buffered, second); diff != "" {
		t.Fatalf("dump is not deterministic: %s", diff)
	}

	_, dest := initializeWithRandomDB(t, nil, nil)
	replayDumpSQL(t, dest, buffered)
	assertLabel := func(sql, want string) {
		t.Helper()
		stmt, err := BuildStatement(sql)
		if err != nil {
			t.Fatal(err)
		}
		result, err := stmt.Execute(t.Context(), dest)
		if err != nil {
			t.Fatal(err)
		}
		result = normalizeResultForCompare(t, result)
		if len(result.presentationRows()) != 1 {
			t.Fatalf("%s: rows=%d affected=%d", sql, len(result.presentationRows()), result.AffectedRows)
		}
		got := result.presentationRows()[0][0].RawText()
		if !strings.Contains(got, want) {
			t.Fatalf("%s got %q want substring %q", sql, got, want)
		}
	}
	assertLabel("SELECT Label FROM Users", "default")
	assertLabel("SELECT Label FROM Alpha.Users", "alpha")
	assertLabel("SELECT Label FROM Beta.Users", "beta")
}

func replayDumpSQL(t *testing.T, dest *Session, sql string) {
	t.Helper()
	parts, err := separateInput(sql)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range parts {
		stripped := strings.TrimSpace(p.statementWithoutComments)
		if stripped == "" {
			continue
		}
		stmt, err := BuildStatementWithComments(p.statementWithoutComments, p.statement)
		if err != nil {
			t.Fatalf("replay parse %q: %v", stripped, err)
		}
		if _, err := stmt.Execute(t.Context(), dest); err != nil {
			t.Fatalf("replay exec %q: %v", stripped, err)
		}
	}
}

func TestDumpCatalogFreshDDLOnce(t *testing.T) {
	t.Parallel()
	skipIfShortIntegration(t)
	_, session := initializeWithRandomDB(t, fourParentCatalogDDL(), nil)

	real, err := session.adminClient.GetDatabaseDdl(t.Context(), &adminpb.GetDatabaseDdlRequest{Database: session.DatabasePath()})
	if err != nil {
		t.Fatal(err)
	}
	stale := append([]string(nil), real.GetStatements()...)
	replaced := false
	for i, stmt := range stale {
		if strings.Contains(stmt, "CREATE TABLE Beta.CrossChild") && strings.Contains(stmt, "INTERLEAVE IN PARENT Alpha.Parent") {
			stale[i] = strings.Replace(stmt, "INTERLEAVE IN PARENT Alpha.Parent", "INTERLEAVE IN PARENT Beta.Parent", 1)
			replaced = true
		}
	}
	if !replaced {
		t.Fatalf("could not mutate CrossChild DDL: %q", real.GetStatements())
	}
	session.ddlCache.response = &adminpb.GetDatabaseDdlResponse{Statements: stale}
	session.ddlCache.fetchedAt = time.Now()
	session.ddlCache.schemaGeneration = session.SchemaGeneration()

	calls := 0
	session.dumpDDLOverride = func(context.Context) (*adminpb.GetDatabaseDdlResponse, error) {
		calls++
		return real, nil
	}
	out := dumpSQL(t, session, &DumpTablesStatement{Tables: []tableID{tidn("Beta", "CrossChild"), tidn("Beta", "Parent")}})
	if calls != 1 {
		t.Fatalf("fresh GetDdl calls = %d, want 1", calls)
	}
	if dataCommentIndex(out, "Beta.Parent") < dataCommentIndex(out, "Beta.CrossChild") {
		t.Fatalf("stale cache parent was used:\n%s", out)
	}
}

func TestDumpCatalogConditionalGetDdlAndPermission(t *testing.T) {
	t.Parallel()
	skipIfShortIntegration(t)
	_, session := initializeWithRandomDB(t, append(fourParentCatalogDDL(),
		"CREATE TABLE Plain (Id INT64 NOT NULL, Value STRING(16)) PRIMARY KEY(Id)",
	), []string{"INSERT INTO Plain (Id, Value) VALUES (1, 'x')"})

	session.dumpDDLOverride = func(context.Context) (*adminpb.GetDatabaseDdlResponse, error) {
		t.Fatal("GetDatabaseDdlFresh should not be called for TABLES-only dumps")
		return nil, errors.New("fail-if-called")
	}
	_ = dumpSQL(t, session, &DumpTablesStatement{Tables: []tableID{tidn("Beta", "CrossChild")}})
	_ = dumpSQL(t, session, &DumpTablesStatement{Tables: []tableID{tid("Plain")}})

	schemaCalls := 0
	session.dumpDDLOverride = func(ctx context.Context) (*adminpb.GetDatabaseDdlResponse, error) {
		schemaCalls++
		return session.adminClient.GetDatabaseDdl(ctx, &adminpb.GetDatabaseDdlRequest{Database: session.DatabasePath()})
	}
	_ = dumpSQL(t, session, &DumpSchemaStatement{})
	if schemaCalls != 1 {
		t.Fatalf("SCHEMA dump fresh GetDdl calls = %d, want 1", schemaCalls)
	}

	session.dumpDDLOverride = func(context.Context) (*adminpb.GetDatabaseDdlResponse, error) {
		return nil, status.Error(codes.PermissionDenied, "denied")
	}
	var buf strings.Builder
	original := session.systemVariables.StreamManager
	session.systemVariables.StreamManager = streamio.NewStreamManager(original.GetInStream(), &buf, original.GetErrStream())
	_, err := (&DumpTablesStatement{Tables: []tableID{tidn("Beta", "CrossChild"), tidn("Alpha", "Parent")}}).Execute(t.Context(), session)
	if err == nil || !strings.Contains(err.Error(), "spanner.databases.getDdl") {
		t.Fatalf("PermissionDenied = %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected zero output, got %q", buf.String())
	}
}

func TestDumpCatalogCrossSchemaFKAndView(t *testing.T) {
	t.Parallel()
	skipIfShortIntegration(t)
	ddls := []string{
		"CREATE SCHEMA Alpha",
		"CREATE SCHEMA Beta",
		"CREATE TABLE Alpha.Parent (Id INT64 NOT NULL) PRIMARY KEY(Id)",
		"CREATE TABLE Beta.Parent (Id INT64 NOT NULL) PRIMARY KEY(Id)",
		"CREATE TABLE Alpha.Child (Id INT64 NOT NULL, Pid INT64, CONSTRAINT fk_alpha FOREIGN KEY(Pid) REFERENCES Beta.Parent(Id)) PRIMARY KEY(Id)",
		"CREATE TABLE Beta.Child (Id INT64 NOT NULL, Pid INT64, CONSTRAINT fk_beta FOREIGN KEY(Pid) REFERENCES Alpha.Parent(Id)) PRIMARY KEY(Id)",
		"CREATE TABLE Base (Id INT64 NOT NULL, Label STRING(8)) PRIMARY KEY(Id)",
		"CREATE VIEW V SQL SECURITY INVOKER AS SELECT Base.Id, Base.Label FROM Base",
	}
	dmls := []string{
		"INSERT INTO Alpha.Parent (Id) VALUES (1)",
		"INSERT INTO Beta.Parent (Id) VALUES (1)",
		"INSERT INTO Alpha.Child (Id, Pid) VALUES (1, 1)",
		"INSERT INTO Beta.Child (Id, Pid) VALUES (1, 1)",
		"INSERT INTO Base (Id, Label) VALUES (1, 'base')",
	}
	_, session := initializeWithRandomDB(t, ddls, dmls)

	err := session.txn.withReadOnlyTransactionOrStart(t.Context(), func(txn *spanner.ReadOnlyTransaction) error {
		dr := NewDependencyResolver()
		if err := dr.BuildDependencyGraphWithTxn(t.Context(), txn, nil); err != nil {
			return err
		}
		alpha := dr.tables[tidn("Alpha", "Child")]
		beta := dr.tables[tidn("Beta", "Child")]
		if alpha == nil || beta == nil {
			t.Fatalf("missing children in catalog")
		}
		if diff := cmp.Diff([]tableID{tidn("Beta", "Parent")}, alpha.FKParents); diff != "" {
			t.Fatalf("Alpha.Child FKParents: %s", diff)
		}
		if diff := cmp.Diff([]tableID{tidn("Alpha", "Parent")}, beta.FKParents); diff != "" {
			t.Fatalf("Beta.Child FKParents: %s", diff)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	out := dumpSQL(t, session, &DumpDatabaseStatement{})
	if dataCommentIndex(out, "Beta.Parent") > dataCommentIndex(out, "Alpha.Child") {
		t.Fatalf("Beta.Parent must precede Alpha.Child:\n%s", out)
	}
	if dataCommentIndex(out, "Alpha.Parent") > dataCommentIndex(out, "Beta.Child") {
		t.Fatalf("Alpha.Parent must precede Beta.Child:\n%s", out)
	}
	if strings.Contains(out, "-- Data for table V") {
		t.Fatalf("view leaked into data:\n%s", out)
	}

	childOnly := dumpSQL(t, session, &DumpTablesStatement{Tables: []tableID{tidn("Alpha", "Child")}})
	if strings.Contains(childOnly, "Beta.Parent") {
		t.Fatalf("absent FK parent was added:\n%s", childOnly)
	}

	_, err = (&DumpTablesStatement{Tables: []tableID{tid("V")}}).Execute(t.Context(), session)
	if err == nil || !strings.Contains(err.Error(), "not a base table") {
		t.Fatalf("view dump error = %v", err)
	}
	_, err = (&DumpTablesStatement{Tables: []tableID{tid("Missing")}}).Execute(t.Context(), session)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing dump error = %v", err)
	}
}

func TestDumpPlanSkipsNoWritableColumns(t *testing.T) {
	t.Parallel()
	skipIfShortIntegration(t)
	_, session := initializeWithRandomDB(t, nil, nil)
	plan := &dumpPlan{Data: []dumpDataPlan{{Table: dumpTablePlan{ID: tid("Ghost"), Columns: nil}}}}
	var result *Result
	err := session.txn.withReadOnlyTransactionOrStart(t.Context(), func(txn *spanner.ReadOnlyTransaction) error {
		var err error
		result, err = executeDumpBufferedWithTxn(t.Context(), session, dumpModeTables, plan, txn, nil)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	got := dumpRenderedOutputForTest(t, result)
	if !strings.Contains(got, "-- Skipping table Ghost (no writable columns)") {
		t.Fatalf("skip comment missing:\n%s", got)
	}
}

func TestDumpCatalogEmulatorGetDdlCorpus(t *testing.T) {
	t.Parallel()
	skipIfShortIntegration(t)
	ddls := append(fourParentCatalogDDL(),
		`CREATE TABLE Complex (
			Id INT64 NOT NULL,
			Name STRING(MAX),
			NameUpper STRING(MAX) AS (UPPER(Name)) STORED,
			CreatedAt TIMESTAMP OPTIONS (allow_commit_timestamp = true),
			CONSTRAINT FK_Parent FOREIGN KEY (Id) REFERENCES Parent (Id),
			CHECK (Id > 0)
		) PRIMARY KEY(Id)`,
	)
	_, session := initializeWithRandomDB(t, ddls, nil)
	ddl, err := session.GetDatabaseDdlFresh(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	children := []struct {
		id     tableID
		parent string
	}{
		{tidn("Alpha", "SameChild"), "Parent"},
		{tidn("Beta", "CrossChild"), "Parent"},
		{tidn("Alpha", "DefaultChild"), "Parent"},
		{tid("NamedChild"), "Parent"},
	}
	for _, child := range children {
		if _, err := extractInterleaveParent(ddl.GetStatements(), child.id, child.parent); err != nil {
			t.Fatalf("emulator GetDdl corpus %s: %v\n%s", child.id.FQN(), err, strings.Join(ddl.GetStatements(), "\n---\n"))
		}
	}
}

func TestDumpSameReadTransaction(t *testing.T) {
	t.Parallel()
	skipIfShortIntegration(t)
	ddls := []string{
		"CREATE TABLE Parent (Id INT64 NOT NULL) PRIMARY KEY(Id)",
		"CREATE TABLE Child (Id INT64 NOT NULL, ChildId INT64 NOT NULL) PRIMARY KEY(Id, ChildId), INTERLEAVE IN PARENT Parent ON DELETE CASCADE",
	}
	dmls := []string{
		"INSERT INTO Parent (Id) VALUES (1)",
		"INSERT INTO Child (Id, ChildId) VALUES (1, 1)",
	}
	for _, explicit := range []bool{false, true} {
		for _, mode := range []string{"database", "tables"} {
			for _, streaming := range []bool{false, true} {
				name := mode + "_automatic"
				if explicit {
					name = mode + "_explicit"
				}
				if streaming {
					name += "_streaming"
				} else {
					name += "_buffered"
				}
				t.Run(name, func(t *testing.T) {
					_, session := initializeWithRandomDB(t, ddls, dmls)
					if explicit {
						if _, err := session.txn.BeginReadOnlyTransaction(t.Context(), timestampBoundUnspecified, 0, time.Time{}, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
							t.Fatal(err)
						}
					}
					var catalogTxn, outputTxn *spanner.ReadOnlyTransaction
					session.dumpReadTxnProbe = func(phase string, txn *spanner.ReadOnlyTransaction) {
						switch phase {
						case "catalog":
							catalogTxn = txn
						case "output":
							outputTxn = txn
						}
					}
					if streaming {
						original := session.systemVariables.StreamManager
						session.systemVariables.StreamManager = streamio.NewStreamManager(original.GetInStream(), &strings.Builder{}, original.GetErrStream())
						defer func() { session.systemVariables.StreamManager = original }()
					}
					var stmt Statement = &DumpDatabaseStatement{}
					if mode == "tables" {
						stmt = &DumpTablesStatement{Tables: []tableID{tid("Parent"), tid("Child")}}
					}
					result, err := stmt.Execute(t.Context(), session)
					if err != nil {
						t.Fatal(err)
					}
					if catalogTxn == nil || outputTxn == nil {
						t.Fatalf("missing txn observations catalog=%p output=%p result=%+v", catalogTxn, outputTxn, result)
					}
					if catalogTxn != outputTxn {
						t.Fatalf("catalog txn %p != output txn %p", catalogTxn, outputTxn)
					}
					if explicit {
						if !session.txn.InReadOnlyTransaction() {
							t.Fatal("explicit RO transaction was closed")
						}
					} else if session.txn.InReadOnlyTransaction() {
						t.Fatal("automatic RO transaction was left open")
					}
				})
			}
		}
	}
}

func TestDumpInvalidSameBasenameCandidateDoesNotGetDdl(t *testing.T) {
	t.Parallel()
	skipIfShortIntegration(t)
	ddls := append(fourParentCatalogDDL(),
		"CREATE SCHEMA Gamma",
		"CREATE VIEW Gamma.Parent SQL SECURITY INVOKER AS SELECT Parent.Id FROM Parent",
	)
	_, session := initializeWithRandomDB(t, ddls, nil)
	session.dumpDDLOverride = func(context.Context) (*adminpb.GetDatabaseDdlResponse, error) {
		t.Fatal("GetDatabaseDdlFresh should not be called")
		return nil, errors.New("fail-if-called")
	}

	cases := []struct {
		name   string
		tables []tableID
		errSub string
	}{
		{name: "missing", tables: []tableID{tidn("Beta", "CrossChild"), tidn("Missing", "Parent")}, errSub: "not found"},
		{name: "view", tables: []tableID{tidn("Beta", "CrossChild"), tidn("Gamma", "Parent")}, errSub: "not a base table"},
	}
	for _, tt := range cases {
		t.Run(tt.name+"_buffered", func(t *testing.T) {
			_, err := (&DumpTablesStatement{Tables: tt.tables}).Execute(t.Context(), session)
			if err == nil || !strings.Contains(err.Error(), tt.errSub) {
				t.Fatalf("error = %v, want %q", err, tt.errSub)
			}
		})
		t.Run(tt.name+"_streaming", func(t *testing.T) {
			var buf strings.Builder
			original := session.systemVariables.StreamManager
			session.systemVariables.StreamManager = streamio.NewStreamManager(original.GetInStream(), &buf, original.GetErrStream())
			defer func() { session.systemVariables.StreamManager = original }()
			_, err := (&DumpTablesStatement{Tables: tt.tables}).Execute(t.Context(), session)
			if err == nil || !strings.Contains(err.Error(), tt.errSub) {
				t.Fatalf("error = %v, want %q", err, tt.errSub)
			}
			if buf.Len() != 0 {
				t.Fatalf("expected zero output, got %q", buf.String())
			}
		})
	}
}

func TestDumpAncestorFKInterleaveAccepted(t *testing.T) {
	t.Parallel()
	skipIfShortIntegration(t)
	ddls := []string{
		"CREATE TABLE AParent (Id INT64 NOT NULL, ChildId INT64) PRIMARY KEY (Id)",
		"CREATE TABLE ZChild (Id INT64 NOT NULL, ChildId INT64 NOT NULL) PRIMARY KEY (Id, ChildId), INTERLEAVE IN PARENT AParent ON DELETE CASCADE",
		"ALTER TABLE AParent ADD CONSTRAINT FK_Child FOREIGN KEY (Id, ChildId) REFERENCES ZChild (Id, ChildId)",
	}
	_, session := initializeWithRandomDB(t, ddls, nil)
	out := dumpSQL(t, session, &DumpDatabaseStatement{})
	if !strings.Contains(out, "CREATE TABLE") {
		t.Fatalf("expected DDL:\n%s", out)
	}
	if strings.Contains(out, "circular foreign key") {
		t.Fatalf("rejected valid ancestor FK schema:\n%s", out)
	}
}

func TestDumpDDLRewriteFailureBeforeOutput(t *testing.T) {
	t.Parallel()
	skipIfShortIntegration(t)
	_, session := initializeWithRandomDB(t, []string{"CREATE TABLE A (Id INT64 NOT NULL) PRIMARY KEY(Id)"}, nil)
	for _, invalid := range []string{
		"CREATE TABLE A (Id INT64 NOT NULL, FOREIGN KEY(Id) REFERENCES Missing(Id)) PRIMARY KEY(Id)",
		"CREATE TABLE A (Id INT64 NOT NULL, FOREIGN KEY(Id) REFERENCES B()",
	} {
		response := &adminpb.GetDatabaseDdlResponse{Statements: []string{"CREATE SCHEMA Earlier", invalid}}
		session.ddlCache.response = response
		session.ddlCache.fetchedAt = time.Now()
		session.ddlCache.schemaGeneration = session.SchemaGeneration()
		session.dumpDDLOverride = func(context.Context) (*adminpb.GetDatabaseDdlResponse, error) { return response, nil }
		for _, statement := range []Statement{&DumpSchemaStatement{}, &DumpDatabaseStatement{}} {
			for _, streaming := range []bool{false, true} {
				var buf strings.Builder
				original := session.systemVariables.StreamManager
				if streaming {
					session.systemVariables.StreamManager = streamio.NewStreamManager(original.GetInStream(), &buf, original.GetErrStream())
				}
				result, err := statement.Execute(t.Context(), session)
				session.systemVariables.StreamManager = original
				if err == nil || result != nil || buf.Len() != 0 {
					t.Fatalf("%T streaming=%v: result=%+v error=%v output=%q; want error before any output", statement, streaming, result, err, buf.String())
				}
			}
		}
	}
}

func TestDumpDDLForwardForeignKeyReplay(t *testing.T) {
	t.Parallel()
	skipIfShortIntegration(t)
	for _, tc := range []struct {
		name string
		ddl  []string
	}{
		{"mutual", []string{
			"CREATE TABLE A (Id INT64 NOT NULL, Ref INT64) PRIMARY KEY(Id)",
			"CREATE TABLE B (Id INT64 NOT NULL, Ref INT64) PRIMARY KEY(Id)",
			"ALTER TABLE A ADD CONSTRAINT fk_a FOREIGN KEY(Ref) REFERENCES B(Id)",
			"ALTER TABLE B ADD CONSTRAINT fk_b FOREIGN KEY(Ref) REFERENCES A(Id)",
		}},
		{"ancestor", []string{
			"CREATE TABLE A (Id INT64 NOT NULL, ChildId INT64) PRIMARY KEY(Id)",
			"CREATE TABLE B (Id INT64 NOT NULL, ChildId INT64 NOT NULL) PRIMARY KEY(Id,ChildId), INTERLEAVE IN PARENT A ON DELETE CASCADE",
			"ALTER TABLE A ADD CONSTRAINT fk_child FOREIGN KEY(Id,ChildId) REFERENCES B(Id,ChildId)",
		}},
		{"self_synonym", []string{
			"CREATE TABLE A (Id INT64 NOT NULL, Ref INT64, SYNONYM (AliasA)) PRIMARY KEY(Id)",
			"ALTER TABLE A ADD CONSTRAINT fk_self FOREIGN KEY(Ref) REFERENCES AliasA(Id)",
		}},
		{"unnamed_mutual", []string{
			"CREATE TABLE A (Id INT64 NOT NULL, Ref INT64) PRIMARY KEY(Id)",
			"CREATE TABLE B (Id INT64 NOT NULL, Ref INT64) PRIMARY KEY(Id)",
			"ALTER TABLE A ADD FOREIGN KEY(Ref) REFERENCES B(Id)",
			"ALTER TABLE B ADD FOREIGN KEY(Ref) REFERENCES A(Id)",
		}},
		{"informational_named", []string{
			"CREATE SCHEMA Alpha", "CREATE SCHEMA Beta",
			"CREATE TABLE Alpha.T (Id INT64 NOT NULL, Ref INT64) PRIMARY KEY(Id)",
			"CREATE TABLE Beta.T (Id INT64 NOT NULL, Ref INT64) PRIMARY KEY(Id)",
			"ALTER TABLE Alpha.T ADD CONSTRAINT fk_alpha FOREIGN KEY(Ref) REFERENCES Beta.T(Id) NOT ENFORCED",
			"ALTER TABLE Beta.T ADD CONSTRAINT fk_beta FOREIGN KEY(Ref) REFERENCES Alpha.T(Id) NOT ENFORCED",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, source := initializeWithRandomDB(t, tc.ddl, nil)
			want, err := source.GetDatabaseDdlFresh(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("source canonical DDL: %q", want.Statements)
			for _, mode := range []string{"SCHEMA", "DATABASE"} {
				t.Run(mode, func(t *testing.T) {
					var statement Statement = &DumpSchemaStatement{}
					if mode == "DATABASE" {
						statement = &DumpDatabaseStatement{}
					}
					buffered := dumpSQL(t, source, statement)
					if streamed := dumpStreamingSQL(t, source, statement); buffered != streamed {
						t.Fatal("buffered/streaming DDL differs")
					}
					_, target := initializeWithRandomDB(t, nil, nil)
					replayDumpSQL(t, target, buffered)
					got, err := target.GetDatabaseDdlFresh(t.Context())
					if err != nil {
						t.Fatal(err)
					}
					if diff := cmp.Diff(want.Statements, got.Statements); diff != "" {
						t.Fatalf("restored schema changed (-want +got):\n%s", diff)
					}
				})
			}
		})
	}
}
