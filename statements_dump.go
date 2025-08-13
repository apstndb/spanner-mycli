package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"

	"cloud.google.com/go/spanner"
	dbadminpb "cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/apstndb/spanner-mycli/enums"
	"google.golang.org/api/iterator"
)

// DumpDatabaseStatement represents DUMP DATABASE statement
type DumpDatabaseStatement struct{}

func (s *DumpDatabaseStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	return executeDump(ctx, session, dumpModeDatabase, nil)
}

// DumpSchemaStatement represents DUMP SCHEMA statement
type DumpSchemaStatement struct{}

func (s *DumpSchemaStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	return executeDump(ctx, session, dumpModeSchema, nil)
}

// DumpTablesStatement represents DUMP TABLES statement
type DumpTablesStatement struct {
	Tables []string
}

func (s *DumpTablesStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	return executeDump(ctx, session, dumpModeTables, s.Tables)
}

// dumpMode represents the type of dump operation
type dumpMode int

const (
	dumpModeDatabase dumpMode = iota // Export DDL + all tables
	dumpModeSchema                   // Export DDL only
	dumpModeTables                   // Export specific tables only
)

// tableInfo represents a table with its dependencies
type tableInfo struct {
	Name           string
	ParentTable    string // INTERLEAVE IN PARENT table
	OnDeleteAction string
	ForeignKeys    []foreignKeyInfo
	ChildrenTables []string // Tables that interleave in this table
}

// foreignKeyInfo represents a foreign key relationship
type foreignKeyInfo struct {
	ConstraintName     string
	ReferencedTable    string
	ReferencingColumns []string
	ReferencedColumns  []string
}

// executeDump performs the actual dump operation
func executeDump(ctx context.Context, session *Session, mode dumpMode, specificTables []string) (*Result, error) {
	if session.adminClient == nil {
		return nil, fmt.Errorf("admin client is not initialized")
	}

	// Get output stream for potential streaming
	outStream := session.systemVariables.StreamManager.GetOutStream()

	// Check if we should use streaming:
	// - Output stream must be available
	// - Output stream must not be io.Discard (used in tests)
	// - Streaming mode should not be explicitly disabled
	shouldStream := outStream != nil &&
		outStream != io.Discard &&
		session.systemVariables.StreamingMode != enums.StreamingModeFalse

	if shouldStream {
		// Use streaming mode for better memory efficiency with large tables
		return executeDumpStreaming(ctx, session, mode, specificTables, outStream)
	}

	// Fall back to buffered mode for tests or when streaming is disabled
	return executeDumpBuffered(ctx, session, mode, specificTables)
}

// executeDumpBuffered performs dump operation with all results buffered in memory
func executeDumpBuffered(ctx context.Context, session *Session, mode dumpMode, specificTables []string) (*Result, error) {
	result := &Result{
		AffectedRows:   0,
		IsDirectOutput: true,
	}

	// Export DDL if requested
	if mode == dumpModeDatabase || mode == dumpModeSchema {
		ddlResult, err := exportDDL(ctx, session)
		if err != nil {
			return nil, fmt.Errorf("failed to export DDL: %w", err)
		}
		result.Rows = append(result.Rows, ddlResult.Rows...)
	}

	// Export data if requested
	if mode == dumpModeDatabase || mode == dumpModeTables {
		tables, err := getTableDependencyOrder(ctx, session, specificTables)
		if err != nil {
			return nil, fmt.Errorf("failed to get table dependency order: %w", err)
		}

		for _, table := range tables {
			dataResult, err := exportTableDataBuffered(ctx, session, table)
			if err != nil {
				return nil, fmt.Errorf("failed to export table %s: %w", table, err)
			}
			result.Rows = append(result.Rows, dataResult.Rows...)
			result.AffectedRows += dataResult.AffectedRows
		}
	}

	return result, nil
}

// executeDumpStreaming performs dump operation with streaming output
// This avoids buffering all data in memory, making it suitable for large tables
func executeDumpStreaming(ctx context.Context, session *Session, mode dumpMode, specificTables []string, out io.Writer) (*Result, error) {
	var totalAffectedRows int

	// Export DDL if requested
	if mode == dumpModeDatabase || mode == dumpModeSchema {
		ddlResult, err := exportDDL(ctx, session)
		if err != nil {
			return nil, fmt.Errorf("failed to export DDL: %w", err)
		}
		// Write DDL directly to output
		for _, row := range ddlResult.Rows {
			if len(row) > 0 {
				fmt.Fprintln(out, row[0])
			}
		}
	}

	// Export data if requested
	if mode == dumpModeDatabase || mode == dumpModeTables {
		tables, err := getTableDependencyOrder(ctx, session, specificTables)
		if err != nil {
			return nil, fmt.Errorf("failed to get table dependency order: %w", err)
		}

		for _, table := range tables {
			// Write table comment
			fmt.Fprintf(out, "-- Data for table %s\n", table)

			// Execute SELECT * with streaming enabled
			// The SQL formatter will stream INSERT statements directly to output
			query := fmt.Sprintf("SELECT * FROM `%s`", table)
			dataResult, err := executeSQLWithFormat(ctx, session, query,
				enums.DisplayModeSQLInsert,
				enums.StreamingModeTrue,
				table)
			if err != nil {
				return nil, fmt.Errorf("failed to export table %s: %w", table, err)
			}

			totalAffectedRows += dataResult.AffectedRows

			// Add separator after table if there was data
			if dataResult.AffectedRows > 0 {
				fmt.Fprintln(out, "")
			}
		}
	}

	// Return a result indicating streaming was done
	return &Result{
		AffectedRows:   totalAffectedRows,
		Streamed:       true,
		IsDirectOutput: false, // No rows to output, already streamed
	}, nil
}

// exportDDL exports database DDL statements
func exportDDL(ctx context.Context, session *Session) (*Result, error) {
	req := &dbadminpb.GetDatabaseDdlRequest{
		Database: session.DatabasePath(),
	}

	ddl, err := session.adminClient.GetDatabaseDdl(ctx, req)
	if err != nil {
		return nil, err
	}

	result := &Result{
		Rows: make([]Row, 0, len(ddl.Statements)+2),
	}

	// Add header comment
	result.Rows = append(result.Rows, Row{"-- Database DDL exported by spanner-mycli"})
	result.Rows = append(result.Rows, Row{""})

	// Add each DDL statement
	for _, stmt := range ddl.Statements {
		// Ensure statement ends with semicolon
		if !strings.HasSuffix(stmt, ";") {
			stmt += ";"
		}
		result.Rows = append(result.Rows, Row{stmt})
		result.Rows = append(result.Rows, Row{""})
	}

	return result, nil
}

// getTableDependencyOrder returns tables in dependency order (parents before children)
func getTableDependencyOrder(ctx context.Context, session *Session, specificTables []string) ([]string, error) {
	// Query information_schema to get table relationships
	query := `
		SELECT 
			TABLE_NAME,
			PARENT_TABLE_NAME,
			ON_DELETE_ACTION
		FROM information_schema.tables
		WHERE TABLE_SCHEMA = ''
		ORDER BY TABLE_NAME
	`

	iter := session.client.Single().Query(ctx, spanner.Statement{SQL: query})
	defer iter.Stop()

	// Build table dependency map
	tables := make(map[string]*tableInfo)
	var allTableNames []string

	for {
		row, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		var tableName, parentTable, onDeleteAction spanner.NullString
		if err := row.Columns(&tableName, &parentTable, &onDeleteAction); err != nil {
			return nil, err
		}

		if !tableName.Valid {
			continue
		}

		info := &tableInfo{
			Name:           tableName.StringVal,
			ParentTable:    parentTable.StringVal,
			OnDeleteAction: onDeleteAction.StringVal,
		}

		tables[info.Name] = info
		allTableNames = append(allTableNames, info.Name)

		// Track children relationships
		if parentTable.Valid {
			if parent, ok := tables[parentTable.StringVal]; ok {
				parent.ChildrenTables = append(parent.ChildrenTables, info.Name)
			} else {
				// Parent not yet processed, create placeholder
				tables[parentTable.StringVal] = &tableInfo{
					Name:           parentTable.StringVal,
					ChildrenTables: []string{info.Name},
				}
			}
		}
	}

	// Filter tables if specific ones requested
	var tablesToExport []string
	if len(specificTables) > 0 {
		// Validate requested tables exist
		for _, table := range specificTables {
			if _, ok := tables[table]; !ok {
				return nil, fmt.Errorf("table %s not found", table)
			}
		}
		tablesToExport = specificTables
	} else {
		tablesToExport = allTableNames
	}

	// Perform topological sort
	return topologicalSort(tables, tablesToExport)
}

// topologicalSort performs dependency-aware sorting of tables
func topologicalSort(tables map[string]*tableInfo, tablesToExport []string) ([]string, error) {
	var sorted []string
	visited := make(map[string]bool)
	visiting := make(map[string]bool)

	var visit func(string) error
	visit = func(name string) error {
		if visited[name] {
			return nil
		}
		if visiting[name] {
			return fmt.Errorf("circular dependency detected involving table %s", name)
		}

		visiting[name] = true
		info := tables[name]

		// Visit parent first (for INTERLEAVE relationships)
		if info != nil && info.ParentTable != "" {
			if slices.Contains(tablesToExport, info.ParentTable) {
				if err := visit(info.ParentTable); err != nil {
					return err
				}
			}
		}

		// TODO: Add foreign key dependency handling here when FK info is available

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

// exportTableDataBuffered exports data from a single table with buffering
func exportTableDataBuffered(ctx context.Context, session *Session, tableName string) (*Result, error) {
	// Query all data from the table
	query := fmt.Sprintf("SELECT * FROM `%s`", tableName)

	// Execute query with SQL format and buffered mode
	dataResult, err := executeSQLWithFormat(ctx, session, query,
		enums.DisplayModeSQLInsert,
		enums.StreamingModeFalse,
		tableName)
	if err != nil {
		return nil, err
	}

	// Create result with table comment header
	result := &Result{
		Rows: []Row{
			{fmt.Sprintf("-- Data for table %s", tableName)},
		},
		AffectedRows: dataResult.AffectedRows,
	}

	// Format the result as SQL INSERT statements if there's data
	if len(dataResult.Rows) > 0 {
		// Get column names from TableHeader
		columnNames := extractTableColumnNames(dataResult.TableHeader)

		// Create a buffer to capture formatted output
		var buf bytes.Buffer

		// Use the SQL formatter to generate INSERT statements
		// Create a temporary systemVariables with our settings
		tempVars := *session.systemVariables
		tempVars.SQLTableName = tableName
		tempVars.CLIFormat = enums.DisplayModeSQLInsert

		// Format using the SQL formatter
		formatter := formatSQL(enums.DisplayModeSQLInsert)
		if err := formatter(&buf, dataResult, columnNames, &tempVars, 0); err != nil {
			return nil, fmt.Errorf("failed to format SQL for table %s: %w", tableName, err)
		}

		// Split the formatted output into rows and append
		if buf.Len() > 0 {
			lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
			for _, line := range lines {
				result.Rows = append(result.Rows, Row{line})
			}
		}
	}

	// Add separator after table data
	if result.AffectedRows > 0 {
		result.Rows = append(result.Rows, Row{""})
	}

	return result, nil
}
