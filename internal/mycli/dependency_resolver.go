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
	"fmt"
	"slices"
	"strings"

	"cloud.google.com/go/spanner"
)

// TableDependency is a BASE TABLE member of the dump data catalog.
type TableDependency struct {
	ID               tableID
	ParentBasename   string   // TABLES.PARENT_TABLE_NAME; never a schema
	InterleaveParent *tableID // set only after authoritative DDL, and only if selected
	FKParents        []tableID
}

// catalogObject is a user-visible INFORMATION_SCHEMA.TABLES row.
type catalogObject struct {
	ID         tableID
	Type       string
	ParentName string
}

// DependencyResolver orders dump tables by interleave and FK edges.
type DependencyResolver struct {
	tables  map[tableID]*TableDependency
	objects map[tableID]catalogObject
}

func NewDependencyResolver() *DependencyResolver {
	return &DependencyResolver{
		tables:  make(map[tableID]*TableDependency),
		objects: make(map[tableID]catalogObject),
	}
}

type informationSchemaTableRow struct {
	TableSchema     string  `spanner:"TABLE_SCHEMA"`
	TableName       string  `spanner:"TABLE_NAME"`
	TableType       string  `spanner:"TABLE_TYPE"`
	ParentTableName *string `spanner:"PARENT_TABLE_NAME"`
}

type informationSchemaFKRow struct {
	ConstraintSchema       string `spanner:"CONSTRAINT_SCHEMA"`
	ConstraintName         string `spanner:"CONSTRAINT_NAME"`
	ChildSchema            string `spanner:"CHILD_SCHEMA"`
	ChildTable             string `spanner:"CHILD_TABLE"`
	UniqueConstraintSchema string `spanner:"UNIQUE_CONSTRAINT_SCHEMA"`
	UniqueConstraintName   string `spanner:"UNIQUE_CONSTRAINT_NAME"`
	ParentSchema           string `spanner:"PARENT_SCHEMA"`
	ParentTable            string `spanner:"PARENT_TABLE"`
}

func (dr *DependencyResolver) BuildDependencyGraphWithTxn(ctx context.Context, txn *spanner.ReadOnlyTransaction) error {
	if err := dr.queryCatalogWithTxn(ctx, txn); err != nil {
		return err
	}
	if err := dr.queryForeignKeysWithTxn(ctx, txn); err != nil {
		return err
	}
	return nil
}

func (dr *DependencyResolver) queryCatalogWithTxn(ctx context.Context, txn *spanner.ReadOnlyTransaction) error {
	query := `
		SELECT
			TABLE_SCHEMA,
			TABLE_NAME,
			TABLE_TYPE,
			PARENT_TABLE_NAME
		FROM INFORMATION_SCHEMA.TABLES
		WHERE TABLE_SCHEMA NOT IN ('INFORMATION_SCHEMA', 'information_schema', 'SPANNER_SYS')
		ORDER BY TABLE_SCHEMA, TABLE_NAME`

	var rows []informationSchemaTableRow
	if err := spanner.SelectAll(txn.Query(ctx, spanner.Statement{SQL: query}), &rows); err != nil {
		return fmt.Errorf("failed to query dump catalog: %w", err)
	}
	for _, row := range rows {
		if userSchemaExcluded(row.TableSchema) {
			continue
		}
		id := tableID{Schema: row.TableSchema, Name: row.TableName}
		obj := catalogObject{ID: id, Type: row.TableType}
		if row.ParentTableName != nil {
			obj.ParentName = *row.ParentTableName
		}
		dr.objects[id] = obj
		if row.TableType != "BASE TABLE" {
			continue
		}
		dep := &TableDependency{ID: id, ParentBasename: obj.ParentName}
		dr.tables[id] = dep
	}
	return nil
}

func (dr *DependencyResolver) queryForeignKeysWithTxn(ctx context.Context, txn *spanner.ReadOnlyTransaction) error {
	query := `
		SELECT DISTINCT
			rc.CONSTRAINT_SCHEMA,
			rc.CONSTRAINT_NAME,
			kcu_child.TABLE_SCHEMA AS CHILD_SCHEMA,
			kcu_child.TABLE_NAME AS CHILD_TABLE,
			rc.UNIQUE_CONSTRAINT_SCHEMA,
			rc.UNIQUE_CONSTRAINT_NAME,
			kcu_parent.TABLE_SCHEMA AS PARENT_SCHEMA,
			kcu_parent.TABLE_NAME AS PARENT_TABLE
		FROM INFORMATION_SCHEMA.REFERENTIAL_CONSTRAINTS rc
		JOIN INFORMATION_SCHEMA.KEY_COLUMN_USAGE kcu_child
			ON rc.CONSTRAINT_SCHEMA = kcu_child.CONSTRAINT_SCHEMA
			AND rc.CONSTRAINT_NAME = kcu_child.CONSTRAINT_NAME
		JOIN INFORMATION_SCHEMA.KEY_COLUMN_USAGE kcu_parent
			ON rc.UNIQUE_CONSTRAINT_SCHEMA = kcu_parent.CONSTRAINT_SCHEMA
			AND rc.UNIQUE_CONSTRAINT_NAME = kcu_parent.CONSTRAINT_NAME
		ORDER BY rc.CONSTRAINT_SCHEMA, rc.CONSTRAINT_NAME`

	var rows []informationSchemaFKRow
	if err := spanner.SelectAll(txn.Query(ctx, spanner.Statement{SQL: query}), &rows); err != nil {
		return fmt.Errorf("failed to query foreign keys: %w", err)
	}
	seen := make(map[[2]tableID]bool)
	for _, row := range rows {
		child := tableID{Schema: row.ChildSchema, Name: row.ChildTable}
		parent := tableID{Schema: row.ParentSchema, Name: row.ParentTable}
		if seen[[2]tableID{child, parent}] {
			continue
		}
		seen[[2]tableID{child, parent}] = true
		childDep, ok := dr.tables[child]
		if !ok {
			continue
		}
		if _, ok := dr.tables[parent]; !ok {
			continue
		}
		if parent == child {
			continue
		}
		if !slices.Contains(childDep.FKParents, parent) {
			childDep.FKParents = append(childDep.FKParents, parent)
		}
	}
	return nil
}

func (dr *DependencyResolver) lookupExplicit(id tableID) error {
	// Unit tests inject members through tables without catalog rows.
	// An empty objects map skips TABLE_TYPE checks and requires tables membership.
	if len(dr.objects) == 0 {
		if _, ok := dr.tables[id]; !ok {
			return fmt.Errorf("table %s not found", id.FQN())
		}
		return nil
	}
	obj, ok := dr.objects[id]
	if !ok {
		return fmt.Errorf("table %s not found", id.FQN())
	}
	if obj.Type != "BASE TABLE" {
		return fmt.Errorf("%s is not a base table (TABLE_TYPE=%s)", id.FQN(), obj.Type)
	}
	return nil
}

func (dr *DependencyResolver) selectedNeedsInterleaveDDL(selected []tableID) bool {
	set := make(map[tableID]struct{}, len(selected))
	for _, id := range selected {
		set[id] = struct{}{}
	}
	for _, id := range selected {
		dep := dr.tables[id]
		if dep == nil || dep.ParentBasename == "" {
			continue
		}
		for other := range set {
			if other == id {
				continue
			}
			if other.Name == dep.ParentBasename {
				return true
			}
		}
	}
	return false
}

func (dr *DependencyResolver) applyInterleaveParents(statements []string, selected []tableID) error {
	set := make(map[tableID]struct{}, len(selected))
	for _, id := range selected {
		set[id] = struct{}{}
	}
	for _, id := range selected {
		dep := dr.tables[id]
		if dep == nil || dep.ParentBasename == "" {
			continue
		}
		hasCandidate := false
		for other := range set {
			if other != id && other.Name == dep.ParentBasename {
				hasCandidate = true
				break
			}
		}
		if !hasCandidate {
			continue
		}
		parent, err := extractInterleaveParent(statements, id, dep.ParentBasename)
		if err != nil {
			return err
		}
		if _, selectedParent := set[parent]; !selectedParent {
			continue
		}
		if _, visible := dr.tables[parent]; !visible {
			return fmt.Errorf("dump catalog: INTERLEAVE parent %s of %s is not a visible BASE TABLE", parent.FQN(), id.FQN())
		}
		p := parent
		dep.InterleaveParent = &p
	}
	return nil
}

func (dr *DependencyResolver) GetTableOrder() ([]tableID, error) {
	all := make([]tableID, 0, len(dr.tables))
	for id := range dr.tables {
		all = append(all, id)
	}
	return dr.GetOrderForTables(all)
}

func (dr *DependencyResolver) GetOrderForTables(tablesToExport []tableID) ([]tableID, error) {
	for _, id := range tablesToExport {
		if err := dr.lookupExplicit(id); err != nil {
			return nil, err
		}
	}
	return dr.topologicalSort(tablesToExport)
}

func (dr *DependencyResolver) topologicalSort(tablesToExport []tableID) ([]tableID, error) {
	if len(tablesToExport) == 0 {
		return []tableID{}, nil
	}
	selected := make(map[tableID]struct{}, len(tablesToExport))
	for _, id := range tablesToExport {
		selected[id] = struct{}{}
	}

	var sorted []tableID
	visited := make(map[tableID]bool)
	visiting := make(map[tableID]bool)
	visitPath := []tableID{}

	var visit func(tableID) error
	visit = func(id tableID) error {
		if visited[id] {
			return nil
		}
		if visiting[id] {
			cycle := append(append([]tableID{}, visitPath...), id)
			names := make([]string, len(cycle))
			for i, c := range cycle {
				names[i] = c.FQN()
			}
			return fmt.Errorf("circular foreign key dependency detected: %s", strings.Join(names, " -> "))
		}
		visiting[id] = true
		visitPath = append(visitPath, id)
		defer func() {
			visitPath = visitPath[:len(visitPath)-1]
		}()

		dep := dr.tables[id]
		if dep == nil {
			return fmt.Errorf("table %s not found", id.FQN())
		}
		if dep.InterleaveParent != nil {
			if _, ok := selected[*dep.InterleaveParent]; ok {
				if err := visit(*dep.InterleaveParent); err != nil {
					return err
				}
			}
		}
		fkParents := append([]tableID(nil), dep.FKParents...)
		slices.SortFunc(fkParents, func(a, b tableID) int { return a.compare(b) })
		for _, parent := range fkParents {
			if _, ok := selected[parent]; !ok {
				continue
			}
			if parent == id {
				continue
			}
			// Skip FK edges that point from an interleave ancestor to a
			// selected descendant. Following them would visit the child
			// before its INTERLEAVE parent. This is not A11 cycle restoration.
			if dr.hasInterleavePathBetween(id, parent) {
				continue
			}
			if visiting[parent] {
				parentDep := dr.tables[parent]
				if dep.InterleaveParent != nil || (parentDep != nil && parentDep.InterleaveParent != nil) {
					continue
				}
				if dr.hasInterleavePathBetween(parent, id) || dr.hasInterleavePathBetween(id, parent) {
					continue
				}
				cycle := append(append([]tableID{}, visitPath...), parent)
				names := make([]string, len(cycle))
				for i, c := range cycle {
					names[i] = c.FQN()
				}
				return fmt.Errorf("circular foreign key dependency detected: %s", strings.Join(names, " -> "))
			}
			if !visited[parent] {
				if err := visit(parent); err != nil {
					return err
				}
			}
		}

		visiting[id] = false
		visited[id] = true
		sorted = append(sorted, id)
		return nil
	}

	export := append([]tableID(nil), tablesToExport...)
	slices.SortFunc(export, func(a, b tableID) int { return a.compare(b) })
	for _, id := range export {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	return sorted, nil
}

// hasInterleavePathBetween reports whether descendant is in the selected
// INTERLEAVE lineage of ancestor (ancestor is a parent/grandparent of descendant).
func (dr *DependencyResolver) hasInterleavePathBetween(ancestor, descendant tableID) bool {
	current := dr.tables[descendant]
	seen := make(map[tableID]bool)
	for current != nil && current.InterleaveParent != nil {
		if seen[current.ID] {
			break
		}
		seen[current.ID] = true
		if *current.InterleaveParent == ancestor {
			return true
		}
		current = dr.tables[*current.InterleaveParent]
	}
	return false
}
