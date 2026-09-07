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
	// FKParents are selected-table FK prerequisites used only for physical
	// INSERT order. Self-edges are omitted because a table cannot precede
	// itself. Known NOT ENFORCED FKs are omitted from this graph as well as
	// from SafetyFKParents. Ancestor-to-descendant FKs stay here but are
	// skipped at order time so INTERLEAVE parent-first is preserved.
	FKParents []tableID
	// SafetyFKParents are enforced (or unknown-enforcement) FK prerequisites
	// including self-edges. Known NOT ENFORCED constraints are omitted.
	// This graph is not used for INSERT order.
	SafetyFKParents []tableID
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
	ConstraintSchema       string  `spanner:"CONSTRAINT_SCHEMA"`
	ConstraintName         string  `spanner:"CONSTRAINT_NAME"`
	ChildSchema            string  `spanner:"CHILD_SCHEMA"`
	ChildTable             string  `spanner:"CHILD_TABLE"`
	UniqueConstraintSchema string  `spanner:"UNIQUE_CONSTRAINT_SCHEMA"`
	UniqueConstraintName   string  `spanner:"UNIQUE_CONSTRAINT_NAME"`
	ParentSchema           string  `spanner:"PARENT_SCHEMA"`
	ParentTable            string  `spanner:"PARENT_TABLE"`
	Enforced               *string `spanner:"ENFORCED"`
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
	// Join TABLE_CONSTRAINTS on qualified constraint identity so known
	// NOT ENFORCED FKs can be omitted from the safety graph. A query error
	// is returned; missing ENFORCED values are treated as enforced.
	query := `
		SELECT DISTINCT
			rc.CONSTRAINT_SCHEMA,
			rc.CONSTRAINT_NAME,
			kcu_child.TABLE_SCHEMA AS CHILD_SCHEMA,
			kcu_child.TABLE_NAME AS CHILD_TABLE,
			rc.UNIQUE_CONSTRAINT_SCHEMA,
			rc.UNIQUE_CONSTRAINT_NAME,
			kcu_parent.TABLE_SCHEMA AS PARENT_SCHEMA,
			kcu_parent.TABLE_NAME AS PARENT_TABLE,
			tc.ENFORCED
		FROM INFORMATION_SCHEMA.REFERENTIAL_CONSTRAINTS rc
		JOIN INFORMATION_SCHEMA.KEY_COLUMN_USAGE kcu_child
			ON rc.CONSTRAINT_SCHEMA = kcu_child.CONSTRAINT_SCHEMA
			AND rc.CONSTRAINT_NAME = kcu_child.CONSTRAINT_NAME
		JOIN INFORMATION_SCHEMA.KEY_COLUMN_USAGE kcu_parent
			ON rc.UNIQUE_CONSTRAINT_SCHEMA = kcu_parent.CONSTRAINT_SCHEMA
			AND rc.UNIQUE_CONSTRAINT_NAME = kcu_parent.CONSTRAINT_NAME
		LEFT JOIN INFORMATION_SCHEMA.TABLE_CONSTRAINTS tc
			ON rc.CONSTRAINT_SCHEMA = tc.CONSTRAINT_SCHEMA
			AND rc.CONSTRAINT_NAME = tc.CONSTRAINT_NAME
			AND tc.CONSTRAINT_TYPE = 'FOREIGN KEY'
		ORDER BY rc.CONSTRAINT_SCHEMA, rc.CONSTRAINT_NAME`

	var rows []informationSchemaFKRow
	if err := spanner.SelectAll(txn.Query(ctx, spanner.Statement{SQL: query}), &rows); err != nil {
		return fmt.Errorf("failed to query foreign keys: %w", err)
	}
	orderSeen := make(map[[2]tableID]bool)
	safetySeen := make(map[[2]tableID]bool)
	for _, row := range rows {
		child := tableID{Schema: row.ChildSchema, Name: row.ChildTable}
		parent := tableID{Schema: row.ParentSchema, Name: row.ParentTable}
		childDep, ok := dr.tables[child]
		if !ok {
			continue
		}
		if _, ok := dr.tables[parent]; !ok {
			continue
		}
		if isKnownNotEnforced(row.Enforced) {
			continue
		}
		if parent != child && !orderSeen[[2]tableID{child, parent}] {
			orderSeen[[2]tableID{child, parent}] = true
			if !slices.Contains(childDep.FKParents, parent) {
				childDep.FKParents = append(childDep.FKParents, parent)
			}
		}
		if safetySeen[[2]tableID{child, parent}] {
			continue
		}
		safetySeen[[2]tableID{child, parent}] = true
		if !slices.Contains(childDep.SafetyFKParents, parent) {
			childDep.SafetyFKParents = append(childDep.SafetyFKParents, parent)
		}
	}
	return nil
}

func isKnownNotEnforced(enforced *string) bool {
	return enforced != nil && strings.EqualFold(*enforced, "NO")
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

	indegree := make(map[tableID]int, len(tablesToExport))
	children := make(map[tableID][]tableID, len(tablesToExport))
	addOrderEdge := func(parent, child tableID) {
		if parent == child {
			return
		}
		if _, ok := selected[parent]; !ok {
			return
		}
		if _, ok := selected[child]; !ok {
			return
		}
		if slices.Contains(children[parent], child) {
			return
		}
		children[parent] = append(children[parent], child)
		indegree[child]++
	}

	for _, id := range tablesToExport {
		dep := dr.tables[id]
		if dep == nil {
			return nil, fmt.Errorf("table %s not found", id.FQN())
		}
		if dep.InterleaveParent != nil {
			addOrderEdge(*dep.InterleaveParent, id)
		}
		for _, parent := range dep.FKParents {
			// Skip FK edges that point from an interleave ancestor to a
			// selected descendant. Following them would emit the child
			// before its INTERLEAVE parent. Safety classification uses
			// SafetyFKParents instead; this skip is order-only.
			if dr.hasInterleavePathBetween(id, parent) {
				continue
			}
			addOrderEdge(parent, id)
		}
	}

	remaining := make(map[tableID]struct{}, len(tablesToExport))
	for _, id := range tablesToExport {
		remaining[id] = struct{}{}
	}
	sorted := make([]tableID, 0, len(tablesToExport))
	for len(remaining) > 0 {
		var ready []tableID
		for id := range remaining {
			if indegree[id] == 0 {
				ready = append(ready, id)
			}
		}
		var pick tableID
		if len(ready) > 0 {
			slices.SortFunc(ready, func(a, b tableID) int { return a.compare(b) })
			pick = ready[0]
		} else {
			// Remaining order-graph cycle: emit interleave-valid then
			// (Schema,Name). Populated cyclic safety SCCs are rejected
			// before the writer; this path orders empty components.
			var cands []tableID
			for id := range remaining {
				dep := dr.tables[id]
				if dep != nil && dep.InterleaveParent != nil {
					if _, parentLeft := remaining[*dep.InterleaveParent]; parentLeft {
						continue
					}
				}
				cands = append(cands, id)
			}
			if len(cands) == 0 {
				for id := range remaining {
					cands = append(cands, id)
				}
			}
			slices.SortFunc(cands, func(a, b tableID) int { return a.compare(b) })
			pick = cands[0]
		}
		sorted = append(sorted, pick)
		delete(remaining, pick)
		for _, child := range children[pick] {
			if _, ok := remaining[child]; ok {
				indegree[child]--
			}
		}
	}
	return sorted, nil
}

// cyclicSafetySCCs returns selected-table strongly connected components that
// are cyclic under interleave prerequisites and enforced/unknown FK edges.
// A self-FK is a cyclic singleton. Known NOT ENFORCED FKs are not edges.
func (dr *DependencyResolver) cyclicSafetySCCs(selected []tableID) [][]tableID {
	selectedSet := make(map[tableID]struct{}, len(selected))
	graph := make(map[tableID][]tableID, len(selected))
	for _, id := range selected {
		selectedSet[id] = struct{}{}
		graph[id] = nil
	}
	selfLoop := make(map[tableID]bool)
	add := func(from, to tableID) {
		if _, ok := selectedSet[to]; !ok {
			return
		}
		if from == to {
			selfLoop[from] = true
			return
		}
		if !slices.Contains(graph[from], to) {
			graph[from] = append(graph[from], to)
		}
	}
	for _, id := range selected {
		dep := dr.tables[id]
		if dep == nil {
			continue
		}
		if dep.InterleaveParent != nil {
			add(id, *dep.InterleaveParent)
		}
		for _, parent := range dep.SafetyFKParents {
			add(id, parent)
		}
	}
	for id := range graph {
		slices.SortFunc(graph[id], func(a, b tableID) int { return a.compare(b) })
	}
	var cyclic [][]tableID
	for _, scc := range stronglyConnectedTableIDs(graph, selected) {
		if len(scc) > 1 || (len(scc) == 1 && selfLoop[scc[0]]) {
			cyclic = append(cyclic, scc)
		}
	}
	return cyclic
}

func stronglyConnectedTableIDs(graph map[tableID][]tableID, nodes []tableID) [][]tableID {
	ordered := append([]tableID(nil), nodes...)
	slices.SortFunc(ordered, func(a, b tableID) int { return a.compare(b) })
	visited := make(map[tableID]bool, len(ordered))
	var stack []tableID
	var dfs1 func(tableID)
	dfs1 = func(u tableID) {
		if visited[u] {
			return
		}
		visited[u] = true
		for _, v := range graph[u] {
			dfs1(v)
		}
		stack = append(stack, u)
	}
	for _, n := range ordered {
		dfs1(n)
	}
	rev := make(map[tableID][]tableID, len(graph))
	for u, vs := range graph {
		for _, v := range vs {
			rev[v] = append(rev[v], u)
		}
	}
	visited = make(map[tableID]bool, len(ordered))
	var sccs [][]tableID
	var dfs2 func(tableID, *[]tableID)
	dfs2 = func(u tableID, comp *[]tableID) {
		if visited[u] {
			return
		}
		visited[u] = true
		*comp = append(*comp, u)
		for _, v := range rev[u] {
			dfs2(v, comp)
		}
	}
	for i := len(stack) - 1; i >= 0; i-- {
		u := stack[i]
		if visited[u] {
			continue
		}
		var comp []tableID
		dfs2(u, &comp)
		slices.SortFunc(comp, func(a, b tableID) int { return a.compare(b) })
		sccs = append(sccs, comp)
	}
	return sccs
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
