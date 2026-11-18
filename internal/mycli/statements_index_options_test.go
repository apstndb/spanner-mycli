// Copyright 2026 apstndb
// SPDX-License-Identifier: Apache-2.0

package mycli

import (
	"strings"
	"testing"

	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
)

func TestShowIndexOptions(t *testing.T) {
	for _, tc := range []struct{ input, schema, table string }{
		{"SHOW INDEX OPTIONS FROM Products", "", "Products"},
		{"show index options from shop.`books-2026`", "shop", "books-2026"},
	} {
		stmt, err := BuildStatement(tc.input)
		if err != nil {
			t.Fatal(err)
		}
		s, ok := stmt.(*ShowIndexOptionsStatement)
		if !ok {
			t.Fatalf("%s parsed as %T", tc.input, stmt)
		}
		g := s.query(databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL)
		if g.Params["schema"] != tc.schema || g.Params["table"] != tc.table {
			t.Fatal(g)
		}
		if _, err := parseMemefishStatement("", g.SQL); err != nil {
			t.Fatal(err)
		}
		pg := s.query(databasepb.DatabaseDialect_POSTGRESQL)
		pgSchema := tc.schema
		if pgSchema == "" {
			pgSchema = "public"
		}
		if pg.Params["p1"] != pgSchema || pg.Params["p2"] != tc.table || !strings.Contains(pg.SQL, "table_name = $2") {
			t.Fatal(pg)
		}
	}
}
