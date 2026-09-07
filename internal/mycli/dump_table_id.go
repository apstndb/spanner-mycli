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
	"cmp"
	"fmt"
	"strings"

	dbadminpb "cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/apstndb/spanvalue"
	"github.com/cloudspannerecosystem/memefish/ast"
)

// tableID is a GoogleSQL catalog identity. Empty Schema is the default schema.
type tableID struct {
	Schema string
	Name   string
}

func (id tableID) FQN() string {
	if id.Schema == "" {
		return id.Name
	}
	return id.Schema + "." + id.Name
}

func (id tableID) compare(other tableID) int {
	if c := cmp.Compare(id.Schema, other.Schema); c != 0 {
		return c
	}
	return cmp.Compare(id.Name, other.Name)
}

func tableIDFromPath(path *ast.Path) (tableID, error) {
	if path == nil || len(path.Idents) == 0 {
		return tableID{}, fmt.Errorf("empty table path")
	}
	if len(path.Idents) > 2 {
		return tableID{}, fmt.Errorf("table path %q has %d components, want 1 or 2", path.SQL(), len(path.Idents))
	}
	if len(path.Idents) == 1 {
		return tableID{Name: path.Idents[0].Name}, nil
	}
	return tableID{Schema: path.Idents[0].Name, Name: path.Idents[1].Name}, nil
}

func parseDumpTableID(input string) (tableID, error) {
	idents, err := parseIdentifierPath(input)
	if err != nil {
		return tableID{}, err
	}
	switch len(idents) {
	case 1:
		return tableID{Name: idents[0]}, nil
	case 2:
		return tableID{Schema: idents[0], Name: idents[1]}, nil
	default:
		return tableID{}, fmt.Errorf("expected [<schema>.]<table>, but %q has %d components", input, len(idents))
	}
}

func quoteTableID(dialect dbadminpb.DatabaseDialect, id tableID) string {
	if id.Schema == "" {
		return spanvalue.QuoteIdentifier(dialect, id.Name)
	}
	return spanvalue.QuoteIdentifier(dialect, id.Schema) + "." + spanvalue.QuoteIdentifier(dialect, id.Name)
}

func userSchemaExcluded(schema string) bool {
	switch strings.ToUpper(schema) {
	case "INFORMATION_SCHEMA", "SPANNER_SYS":
		return true
	default:
		return false
	}
}
