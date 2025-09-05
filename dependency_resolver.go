package main

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"cloud.google.com/go/spanner"
)

// FKReference represents a foreign key relationship between tables
type FKReference struct {
	ConstraintName string `spanner:"CONSTRAINT_NAME"`
	ChildTable     string `spanner:"CHILD_TABLE"`
	ParentTable    string `spanner:"PARENT_TABLE"`
}

// TableDependency represents a table with all its dependencies
type TableDependency struct {
	TableName      string
	ParentTable    string        // INTERLEAVE parent
	OnDeleteAction string        // CASCADE, NO ACTION for INTERLEAVE
	ChildrenTables []string      // Tables that interleave in this table
	ForeignKeys    []FKReference // Foreign key references FROM this table
	ReferencedBy   []FKReference // Foreign key references TO this table
	Level          int           // INTERLEAVE depth (0-7)
}

// DependencyResolver manages table dependency resolution
// Note: This implementation works at the table level. For row-level dependency
// resolution that can handle circular FK relationships, see issue #438.
type DependencyResolver struct {
	tables map[string]*TableDependency
}

// NewDependencyResolver creates a new dependency resolver
func NewDependencyResolver() *DependencyResolver {
	return &DependencyResolver{
		tables: make(map[string]*TableDependency),
	}
}

// interleaveRow represents a row from INFORMATION_SCHEMA.TABLES for interleave relationships
type interleaveRow struct {
	TableName      string  `spanner:"TABLE_NAME"`
	ParentTable    *string `spanner:"PARENT_TABLE_NAME"`
	OnDeleteAction *string `spanner:"ON_DELETE_ACTION"`
}

// BuildDependencyGraphWithTxn queries the database and builds the complete dependency graph using a transaction
func (dr *DependencyResolver) BuildDependencyGraphWithTxn(ctx context.Context, txn *spanner.ReadOnlyTransaction) error {
	// First, query INTERLEAVE relationships
	if err := dr.queryInterleaveRelationshipsWithTxn(ctx, txn); err != nil {
		return fmt.Errorf("failed to query interleave relationships: %w", err)
	}

	// Then, query foreign key relationships
	if err := dr.queryForeignKeyRelationshipsWithTxn(ctx, txn); err != nil {
		return fmt.Errorf("failed to query foreign key relationships: %w", err)
	}

	// Calculate INTERLEAVE levels
	dr.calculateInterleaveLevels()

	return nil
}

// processInterleaveRows processes all interleave relationship rows and updates the dependency graph.
func (dr *DependencyResolver) processInterleaveRows(rows []interleaveRow) {
	for _, data := range rows {
		// Get or create table dependency
		table := dr.getOrCreateTable(data.TableName)

		// Set INTERLEAVE parent information
		if data.ParentTable != nil {
			table.ParentTable = *data.ParentTable
			if data.OnDeleteAction != nil {
				table.OnDeleteAction = *data.OnDeleteAction
			}

			// Update parent's children list
			parent := dr.getOrCreateTable(*data.ParentTable)
			if !slices.Contains(parent.ChildrenTables, data.TableName) {
				parent.ChildrenTables = append(parent.ChildrenTables, data.TableName)
			}
		}
	}
}

// queryInterleaveRelationshipsWithTxn queries and builds INTERLEAVE parent-child relationships using a transaction
func (dr *DependencyResolver) queryInterleaveRelationshipsWithTxn(ctx context.Context, txn *spanner.ReadOnlyTransaction) error {
	query := `
		SELECT 
			TABLE_NAME,
			PARENT_TABLE_NAME,
			ON_DELETE_ACTION
		FROM information_schema.tables
		WHERE TABLE_SCHEMA = ''
		ORDER BY TABLE_NAME
	`

	var rows []interleaveRow
	if err := spanner.SelectAll(txn.Query(ctx, spanner.Statement{SQL: query}), &rows); err != nil {
		return fmt.Errorf("failed to query interleave relationships: %w", err)
	}

	dr.processInterleaveRows(rows)
	return nil
}

// processForeignKeyRows processes all foreign key rows and updates the dependency graph.
func (dr *DependencyResolver) processForeignKeyRows(rows []FKReference) {
	// No need for deduplication since the query now returns unique constraints only
	for _, fkRef := range rows {
		// Update child table's foreign keys
		childDep := dr.getOrCreateTable(fkRef.ChildTable)
		childDep.ForeignKeys = append(childDep.ForeignKeys, fkRef)

		// Update parent table's referenced by list
		parentDep := dr.getOrCreateTable(fkRef.ParentTable)
		parentDep.ReferencedBy = append(parentDep.ReferencedBy, fkRef)
	}
}

// queryForeignKeyRelationshipsWithTxn queries and builds foreign key relationships using a transaction
func (dr *DependencyResolver) queryForeignKeyRelationshipsWithTxn(ctx context.Context, txn *spanner.ReadOnlyTransaction) error {
	// Query foreign key relationships at the table level
	// KEY_COLUMN_USAGE is needed because REFERENTIAL_CONSTRAINTS doesn't contain table names
	query := `
		SELECT DISTINCT
			rc.CONSTRAINT_NAME,
			kcu_child.TABLE_NAME AS CHILD_TABLE,
			kcu_parent.TABLE_NAME AS PARENT_TABLE
		FROM information_schema.referential_constraints rc
		JOIN information_schema.key_column_usage kcu_child
			USING (CONSTRAINT_SCHEMA, CONSTRAINT_NAME)
		JOIN information_schema.key_column_usage kcu_parent
			ON rc.UNIQUE_CONSTRAINT_SCHEMA = kcu_parent.CONSTRAINT_SCHEMA
			AND rc.UNIQUE_CONSTRAINT_NAME = kcu_parent.CONSTRAINT_NAME
		WHERE rc.CONSTRAINT_SCHEMA = ''
		ORDER BY rc.CONSTRAINT_NAME
	`

	var rows []FKReference
	if err := spanner.SelectAll(txn.Query(ctx, spanner.Statement{SQL: query}), &rows); err != nil {
		return fmt.Errorf("failed to query foreign key relationships: %w", err)
	}

	dr.processForeignKeyRows(rows)
	return nil
}

// calculateInterleaveLevels calculates the INTERLEAVE depth for each table
func (dr *DependencyResolver) calculateInterleaveLevels() {
	for _, table := range dr.tables {
		level := 0
		current := table //nolint:copyloopvar // current is modified in the loop
		visited := make(map[string]bool)

		// Follow parent chain to calculate depth
		for current.ParentTable != "" {
			if visited[current.TableName] {
				// Circular dependency detected (shouldn't happen with INTERLEAVE)
				break
			}
			visited[current.TableName] = true

			parent, exists := dr.tables[current.ParentTable]
			if !exists {
				break
			}
			current = parent
			level++

			// Spanner supports up to 7 levels of interleaving
			if level > 7 {
				level = 7
				break
			}
		}

		table.Level = level
	}
}

// getOrCreateTable gets an existing table or creates a new one
func (dr *DependencyResolver) getOrCreateTable(name string) *TableDependency {
	if table, exists := dr.tables[name]; exists {
		return table
	}

	table := &TableDependency{
		TableName:      name,
		ChildrenTables: []string{},
		ForeignKeys:    []FKReference{},
		ReferencedBy:   []FKReference{},
	}
	dr.tables[name] = table
	return table
}

// GetTableOrder returns all tables in safe execution order.
// It assumes that BuildDependencyGraph has already been called.
func (dr *DependencyResolver) GetTableOrder() ([]string, error) {
	allTables := make([]string, 0, len(dr.tables))
	for tableName := range dr.tables {
		allTables = append(allTables, tableName)
	}

	return dr.GetOrderForTables(allTables)
}

// GetOrderForTables returns specific tables in safe execution order
func (dr *DependencyResolver) GetOrderForTables(tablesToExport []string) ([]string, error) {
	// Validate that all requested tables exist
	for _, table := range tablesToExport {
		if _, exists := dr.tables[table]; !exists {
			return nil, fmt.Errorf("table %s not found", table)
		}
	}

	// Perform topological sort with priority rules
	return dr.topologicalSort(tablesToExport)
}

// topologicalSort performs dependency-aware sorting with priority rules:
// 1. INTERLEAVE relationships have highest priority
// 2. Foreign key relationships come second
// 3. Alphabetical ordering for independent tables
func (dr *DependencyResolver) topologicalSort(tablesToExport []string) ([]string, error) {
	// Handle empty input
	if len(tablesToExport) == 0 {
		return []string{}, nil
	}

	var sorted []string
	visited := make(map[string]bool)
	visiting := make(map[string]bool)
	visitPath := []string{} // Track the current visit path for better error messages

	var visit func(string) error
	visit = func(name string) error {
		if visited[name] {
			return nil
		}
		if visiting[name] {
			// Find where the cycle starts in the visit path
			cycleStart := -1
			for i, table := range visitPath {
				if table == name {
					cycleStart = i
					break
				}
			}

			// Build cycle description
			cyclePath := visitPath[cycleStart:]
			cyclePath = append(cyclePath, name) // Complete the cycle

			// Check if this is due to FK cycle (INTERLEAVE cycles are impossible)
			table := dr.tables[name]
			if table.ParentTable == "" || !slices.Contains(tablesToExport, table.ParentTable) {
				// This is a FK cycle
				return fmt.Errorf("circular foreign key dependency detected: %s",
					strings.Join(cyclePath, " -> "))
			}
			// INTERLEAVE cycle shouldn't happen, but handle gracefully
			return fmt.Errorf("circular dependency detected: %s",
				strings.Join(cyclePath, " -> "))
		}

		visiting[name] = true
		visitPath = append(visitPath, name) // Add to visit path
		defer func() {
			visitPath = visitPath[:len(visitPath)-1] // Remove from visit path when done
		}()

		table := dr.tables[name]

		// Priority 1: Visit INTERLEAVE parent first
		if table.ParentTable != "" && slices.Contains(tablesToExport, table.ParentTable) {
			if err := visit(table.ParentTable); err != nil {
				return err
			}
		}

		// Priority 2: Visit FK referenced tables
		// Deduplicate FK parent tables
		fkParents := make(map[string]bool)
		for _, fk := range table.ForeignKeys {
			if slices.Contains(tablesToExport, fk.ParentTable) && fk.ParentTable != name {
				fkParents[fk.ParentTable] = true
			}
		}

		// Sort FK parents for consistent ordering
		var fkParentList []string
		for parent := range fkParents {
			fkParentList = append(fkParentList, parent)
		}
		sort.Strings(fkParentList)

		for _, parent := range fkParentList {
			// Check if visiting this FK parent would conflict with INTERLEAVE hierarchy
			// Example: If Child is interleaved in Parent, and Child has FK to Other,
			// but Other has FK back to Child, we skip the Other->Child FK
			if dr.wouldCreateInterleaveConflict(name, parent) {
				continue
			}

			// Try to visit the FK parent
			if !visited[parent] && !visiting[parent] {
				// Parent hasn't been processed yet, visit it first
				if err := visit(parent); err != nil {
					return err
				}
			} else if visiting[parent] {
				// We're already visiting this parent, which means we have a cycle
				// However, some cycles are acceptable if they involve INTERLEAVE relationships

				currentTable := dr.tables[name]
				parentTableInfo := dr.tables[parent]

				// Case 1: If either table in the cycle has an INTERLEAVE parent,
				// the INTERLEAVE relationship takes precedence and we can ignore this FK
				if currentTable.ParentTable != "" || parentTableInfo.ParentTable != "" {
					continue
				}

				// Case 2: Check if there's an INTERLEAVE path between the tables
				// This handles complex scenarios where the cycle involves tables
				// that are in the same INTERLEAVE hierarchy
				if dr.hasInterleavePathBetween(parent, name) || dr.hasInterleavePathBetween(name, parent) {
					continue
				}

				// No INTERLEAVE relationship to break the cycle - this is an error
				// Build the cycle path for a clearer error message
				cyclePath := append(visitPath, parent)
				return fmt.Errorf("circular foreign key dependency detected: %s",
					strings.Join(cyclePath, " -> "))
			}
		}

		visiting[name] = false
		visited[name] = true
		sorted = append(sorted, name)
		return nil
	}

	// Sort tables alphabetically first for consistent ordering
	sort.Strings(tablesToExport)

	// Visit all tables
	for _, table := range tablesToExport {
		if err := visit(table); err != nil {
			return nil, err
		}
	}

	return sorted, nil
}

// wouldCreateInterleaveConflict checks if processing FK parent would conflict with INTERLEAVE
func (dr *DependencyResolver) wouldCreateInterleaveConflict(child, fkParent string) bool {
	// Check if fkParent is an INTERLEAVE child of the current table
	return dr.hasInterleavePathBetween(child, fkParent)
}

// hasInterleavePathBetween checks if there's an INTERLEAVE path from ancestor to descendant
func (dr *DependencyResolver) hasInterleavePathBetween(ancestor, descendant string) bool {
	current := dr.tables[descendant]
	visited := make(map[string]bool)

	for current != nil && current.ParentTable != "" {
		if visited[current.TableName] {
			break // Cycle detection
		}
		visited[current.TableName] = true

		if current.ParentTable == ancestor {
			return true
		}
		current = dr.tables[current.ParentTable]
	}

	return false
}

// GetDependencyInfo returns detailed dependency information for a table
func (dr *DependencyResolver) GetDependencyInfo(tableName string) (*TableDependency, error) {
	table, exists := dr.tables[tableName]
	if !exists {
		return nil, fmt.Errorf("table %s not found", tableName)
	}
	return table, nil
}
