// Copyright 2026 apstndb
// SPDX-License-Identifier: Apache-2.0

package mycli

import (
	"fmt"
	"strings"
	"testing"

	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/apstndb/spanemuboost"
)

func TestShowIndexOptions(t *testing.T) {
	for _, tc := range []struct{ input, schema, table string }{
		{"SHOW INDEX FROM Products", "", "Products"},
		{"show indexes from shop.`books-2026`", "shop", "books-2026"},
	} {
		stmt, err := BuildStatement(tc.input)
		if err != nil {
			t.Fatal(err)
		}
		s, ok := stmt.(*ShowIndexStatement)
		if !ok {
			t.Fatalf("%s parsed as %T", tc.input, stmt)
		}
		g := s.query(databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL, true)
		if g.Params["schema"] != tc.schema || g.Params["table"] != tc.table {
			t.Fatal(g)
		}
		if _, err := parseMemefishStatement("", g.SQL); err != nil {
			t.Fatal(err)
		}
		pg := s.query(databasepb.DatabaseDialect_POSTGRESQL, true)
		pgSchema := tc.schema
		if pgSchema == "" {
			pgSchema = "public"
		}
		if pg.Params["p1"] != pgSchema || pg.Params["p2"] != tc.table || !strings.Contains(pg.SQL, "table_name = $2") {
			t.Fatal(pg)
		}
	}
}

func TestShowIndexOptionsAggregation(t *testing.T) {
	if testing.Short() {
		t.Skip("requires the emulator")
	}
	for _, tc := range []struct {
		dialect databasepb.DatabaseDialect
		ddl     string
		schema  string
	}{
		{databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL, "CREATE TABLE items (id INT64 NOT NULL, code INT64) PRIMARY KEY(id)", ""},
		{databasepb.DatabaseDialect_POSTGRESQL, "CREATE TABLE items (id bigint PRIMARY KEY, code bigint)", "public"},
	} {
		t.Run(tc.dialect.String(), func(t *testing.T) {
			_, session := initializeWithDB(t, "", []string{tc.ddl, "CREATE INDEX by_code ON items(code)"}, nil, spanemuboost.WithDatabaseDialect(tc.dialect))
			session.systemVariables.Feature.DatabaseDialect = tc.dialect
			show := ShowIndexStatement{Table: "items"}
			result, err := show.Execute(t.Context(), session, OperationOutput{})
			if err != nil || result.AffectedRows != 2 {
				t.Fatalf("SHOW INDEX returned %v, error %v", result, err)
			}

			// The emulator lacks INDEX_OPTIONS. Substitute just that catalog
			// relation and run the production aggregation against real indexes.
			stmt := show.query(tc.dialect, true)
			const options = `WITH fixture AS (
  SELECT '%[1]s' AS table_schema, 'items' AS table_name, 'by_code' AS index_name,
         'INDEX' AS index_type, 'z' AS option_name, '3' AS option_value
  UNION ALL SELECT '%[1]s', 'items', 'by_code', 'INDEX', 'a', 'x,y'
  UNION ALL SELECT '%[1]s', 'other', 'by_code', 'INDEX', 'wrong', '0'
)
`
			stmt.SQL = fmt.Sprintf(options, tc.schema) + strings.Replace(stmt.SQL, "information_schema.index_options", "fixture", 1)
			result, err = executeInformationSchemaBasedStatement(t.Context(), session, "SHOW INDEX", stmt, nil)
			if err != nil {
				t.Fatal(err)
			}
			rows, _ := result.Body.PresentationRows()
			if len(rows) != 2 {
				t.Fatalf("expected two indexes, got %v", rows)
			}
			for _, row := range rows {
				if row[2] == p("by_code") {
					compareResult(t, row[7:], Row{p("a=x,y, z=3")})
				} else {
					compareResult(t, row[7:], Row{n("NULL")})
				}
			}
		})
	}
}
