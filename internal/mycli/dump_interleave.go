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
	"fmt"
	"strings"

	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/ast"
)

func parseMemefishDDL(filepath, input string) (ast.DDL, error) {
	return recoverMemefishParserPanic(func() (ast.DDL, error) {
		return memefish.ParseDDL(filepath, input)
	})
}

// extractInterleaveParent parses the unique CREATE TABLE DDL for child and
// returns the INTERLEAVE IN [PARENT] target. catalogParentName is the
// TABLES.PARENT_TABLE_NAME basename used only as a consistency check.
func extractInterleaveParent(statements []string, child tableID, catalogParentName string) (tableID, error) {
	var matches []string
	for _, stmt := range statements {
		if isCreateDDL(stmt, "TABLE", child.Schema, child.Name) {
			matches = append(matches, stmt)
		}
	}
	if len(matches) == 0 {
		return tableID{}, fmt.Errorf("dump catalog: no CREATE TABLE DDL for %s", child.FQN())
	}
	if len(matches) > 1 {
		return tableID{}, fmt.Errorf("dump catalog: duplicate CREATE TABLE DDL for %s", child.FQN())
	}

	ddl, err := parseMemefishDDL("dump-interleave", matches[0])
	if err != nil {
		return tableID{}, fmt.Errorf("dump catalog: parse CREATE TABLE for %s: %w", child.FQN(), err)
	}
	ct, ok := ddl.(*ast.CreateTable)
	if !ok {
		return tableID{}, fmt.Errorf("dump catalog: DDL for %s is not CREATE TABLE", child.FQN())
	}
	parsedChild, err := tableIDFromPath(ct.Name)
	if err != nil {
		return tableID{}, fmt.Errorf("dump catalog: CREATE TABLE name for %s: %w", child.FQN(), err)
	}
	if parsedChild != child {
		return tableID{}, fmt.Errorf("dump catalog: CREATE TABLE name %s does not match catalog %s", parsedChild.FQN(), child.FQN())
	}
	if ct.Cluster == nil || ct.Cluster.TableName == nil {
		return tableID{}, fmt.Errorf("dump catalog: CREATE TABLE %s has no INTERLEAVE clause", child.FQN())
	}
	parent, err := tableIDFromPath(ct.Cluster.TableName)
	if err != nil {
		return tableID{}, fmt.Errorf("dump catalog: INTERLEAVE parent for %s: %w", child.FQN(), err)
	}
	if !strings.EqualFold(parent.Name, catalogParentName) {
		return tableID{}, fmt.Errorf("dump catalog: INTERLEAVE parent basename %q for %s does not match catalog PARENT_TABLE_NAME %q", parent.Name, child.FQN(), catalogParentName)
	}
	return parent, nil
}
