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
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/cloudspannerecosystem/memefish/ast"
)

func TestShowChangeStreamsGoogleSQLParses(t *testing.T) {
	t.Parallel()

	stmt, err := parseMemefishStatement("show-change-streams.sql", showChangeStreamsGoogleSQL)
	if err != nil {
		t.Fatalf("parse GoogleSQL SHOW CHANGE STREAMS: %v", err)
	}
	query, ok := stmt.(*ast.QueryStatement)
	if !ok {
		t.Fatalf("parsed %T, want *ast.QueryStatement", stmt)
	}
	sql := query.SQL()
	if !strings.Contains(showChangeStreamsGoogleSQL, "`ALL`") {
		t.Fatalf("GoogleSQL literal missing quoted ALL: %s", showChangeStreamsGoogleSQL)
	}
	if strings.Contains(strings.ToUpper(showChangeStreamsGoogleSQL), "CATALOG") {
		t.Fatalf("GoogleSQL listing must not filter catalog: %s", showChangeStreamsGoogleSQL)
	}
	if sql == "" {
		t.Fatal("unparsed query SQL is empty")
	}
}

func TestShowChangeStreamsPostgreSQLHasNoCatalogPredicate(t *testing.T) {
	t.Parallel()

	if strings.Contains(strings.ToUpper(showChangeStreamsPostgreSQL), "CATALOG") {
		t.Fatalf("PostgreSQL listing must not filter catalog: %s", showChangeStreamsPostgreSQL)
	}
	if !strings.Contains(showChangeStreamsPostgreSQL, `("all" = 'YES')`) {
		t.Fatalf("PostgreSQL listing must map YES/NO to BOOL: %s", showChangeStreamsPostgreSQL)
	}
	if showChangeStreamsSQL(databasepb.DatabaseDialect_POSTGRESQL) != showChangeStreamsPostgreSQL {
		t.Fatal("POSTGRESQL dialect did not select the PostgreSQL literal")
	}
	if showChangeStreamsSQL(databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL) != showChangeStreamsGoogleSQL {
		t.Fatal("GOOGLE_STANDARD_SQL dialect did not select the GoogleSQL literal")
	}
	if showChangeStreamsSQL(databasepb.DatabaseDialect_DATABASE_DIALECT_UNSPECIFIED) != showChangeStreamsGoogleSQL {
		t.Fatal("unspecified dialect must follow GoogleSQL quoting")
	}
}

func TestShowCreateChangeStreamUsesCachedDDL(t *testing.T) {
	t.Parallel()

	const ddl = "CREATE CHANGE STREAM NamesAndAlbums FOR Singers(FirstName, LastName), Albums OPTIONS ( retention_period = '36h', value_capture_type = 'NEW_VALUES' )"
	session := newSessionForLocalVarTest(t)
	session.ddlCache.response = &databasepb.GetDatabaseDdlResponse{Statements: []string{
		"CREATE TABLE Singers (FirstName STRING(MAX), LastName STRING(MAX)) PRIMARY KEY(FirstName, LastName)",
		ddl,
	}}
	session.ddlCache.fetchedAt = time.Now()
	session.ddlCache.schemaGeneration = session.SchemaGeneration()

	got, err := (&ShowCreateStatement{ObjectType: "CHANGE STREAM", Name: "NamesAndAlbums"}).Execute(t.Context(), session, OperationOutput{})
	if err != nil {
		t.Fatalf("SHOW CREATE CHANGE STREAM: %v", err)
	}
	if got.AffectedRows != 1 || len(got.presentationRows()) != 1 {
		t.Fatalf("result = %+v, want 1 row", got)
	}
	row := got.presentationRows()[0]
	if row[0].RawText() != "NamesAndAlbums" {
		t.Errorf("Name = %q, want NamesAndAlbums", row[0].RawText())
	}
	if row[1].RawText() != ddl {
		t.Errorf("DDL = %q, want representative CREATE CHANGE STREAM", row[1].RawText())
	}
}

func TestShowChangeStreamsClassification(t *testing.T) {
	t.Parallel()

	var stmt Statement = &ShowChangeStreamsStatement{}
	if _, ok := stmt.(MutationStatement); ok {
		t.Fatal("SHOW CHANGE STREAMS must not be a MutationStatement")
	}
	if _, ok := stmt.(nonTransactionalMutationStatement); ok {
		t.Fatal("SHOW CHANGE STREAMS must not be a nonTransactionalMutationStatement")
	}
	if _, ok := stmt.(DetachedCompatible); ok {
		t.Fatal("SHOW CHANGE STREAMS must not be DetachedCompatible")
	}

	session := newSessionForLocalVarTest(t)
	session.mode = Detached
	if err := session.ValidateStatementExecution(stmt); err == nil {
		t.Fatal("detached session accepted SHOW CHANGE STREAMS")
	}
}
