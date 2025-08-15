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
// It exports both DDL and data for all tables in the database
type DumpDatabaseStatement struct{}

func (s *DumpDatabaseStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	return executeDump(ctx, session, dumpModeDatabase, nil)
}

// DumpSchemaStatement represents DUMP SCHEMA statement
// It exports only DDL statements without any data
type DumpSchemaStatement struct{}

func (s *DumpSchemaStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	return executeDump(ctx, session, dumpModeSchema, nil)
}

// DumpTablesStatement represents DUMP TABLES statement
// It exports data only for specified tables (no DDL)
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

func (m dumpMode) shouldExportDDL() bool  { return m == dumpModeDatabase || m == dumpModeSchema }
func (m dumpMode) shouldExportData() bool { return m == dumpModeDatabase || m == dumpModeTables }

// tableInfo represents a table with its dependencies
type tableInfo struct {
	Name           string
	ParentTable    string // INTERLEAVE IN PARENT table
	OnDeleteAction string
	ChildrenTables []string // Tables that interleave in this table
	// ForeignKeys will be added for Issue #426
}

// executeDump is the main entry point for all dump operations.
// It decides between streaming and buffered mode based on the output stream and settings.
func executeDump(ctx context.Context, session *Session, mode dumpMode, specificTables []string) (*Result, error) {
	if session.adminClient == nil {
		return nil, fmt.Errorf("admin client is not initialized")
	}
	// TODO: Add proper PostgreSQL support. Currently the SQL export format depends on spanvalue.LiteralFormatConfig
	// which generates Google SQL literals, not PostgreSQL-compatible ones.
	if session.systemVariables.DatabaseDialect == dbadminpb.DatabaseDialect_POSTGRESQL {
		return nil, fmt.Errorf("DUMP statements are not yet supported for PostgreSQL dialect databases")
	}
	outStream := session.systemVariables.StreamManager.GetWriter()
	// Use streaming unless: output is nil/io.Discard (tests) or streaming explicitly disabled
	if outStream != nil && outStream != io.Discard && session.systemVariables.StreamingMode != enums.StreamingModeFalse {
		return executeDumpStreaming(ctx, session, mode, specificTables, outStream)
	}
	return executeDumpBuffered(ctx, session, mode, specificTables)
}

// getTablesForExport returns the list of tables to export based on the dump mode.
// For data export modes, it returns tables in dependency order (parents before children).
func getTablesForExport(ctx context.Context, session *Session, mode dumpMode, specificTables []string) ([]string, error) {
	if !mode.shouldExportData() {
		return nil, nil
	}
	return getTableDependencyOrder(ctx, session, specificTables)
}

// executeDumpBuffered performs dump operation with buffering.
// All output is collected in memory before being returned.
func executeDumpBuffered(ctx context.Context, session *Session, mode dumpMode, specificTables []string) (*Result, error) {
	result := &Result{AffectedRows: 0, IsDirectOutput: true}
	if mode.shouldExportDDL() {
		ddlResult, err := exportDDL(ctx, session)
		if err != nil {
			return nil, fmt.Errorf("export DDL: %w", err)
		}
		result.Rows = append(result.Rows, ddlResult.Rows...)
	}
	tables, err := getTablesForExport(ctx, session, mode, specificTables)
	if err != nil {
		return nil, err
	}
	for _, table := range tables {
		dataResult, err := exportTableDataBuffered(ctx, session, table)
		if err != nil {
			return nil, fmt.Errorf("export table %s: %w", table, err)
		}
		result.Rows = append(result.Rows, dataResult.Rows...)
		result.AffectedRows += dataResult.AffectedRows
	}
	return result, nil
}

// writeResultRows writes Result rows to an io.Writer
func writeResultRows(out io.Writer, rows []Row) error {
	for _, row := range rows {
		if len(row) > 0 {
			if _, err := fmt.Fprintln(out, row[0]); err != nil {
				return err
			}
		}
	}
	return nil
}

// executeDumpStreaming performs dump operation with streaming output.
// Data is written directly to the output stream as it's processed,
// avoiding memory buildup for large tables.
func executeDumpStreaming(ctx context.Context, session *Session, mode dumpMode, specificTables []string, out io.Writer) (*Result, error) {
	var totalAffectedRows int

	// Export DDL if requested
	if mode.shouldExportDDL() {
		ddlResult, err := exportDDL(ctx, session)
		if err != nil {
			return nil, fmt.Errorf("failed to export DDL: %w", err)
		}
		if err := writeResultRows(out, ddlResult.Rows); err != nil {
			return nil, fmt.Errorf("failed to write DDL: %w", err)
		}
	}

	tables, err := getTablesForExport(ctx, session, mode, specificTables)
	if err != nil {
		return nil, fmt.Errorf("failed to get table dependency order: %w", err)
	}

	for _, table := range tables {
		// Write table comment
		fmt.Fprintf(out, "-- Data for table %s\n", table)

		// Execute SELECT * with streaming enabled - SQL formatter streams INSERT statements directly to output
		dataResult, err := executeSQLWithFormat(ctx, session, fmt.Sprintf("SELECT * FROM `%s`", table),
			enums.DisplayModeSQLInsert, enums.StreamingModeTrue, table)
		if err != nil {
			return nil, fmt.Errorf("failed to export table %s: %w", table, err)
		}

		totalAffectedRows += dataResult.AffectedRows
		if dataResult.AffectedRows > 0 {
			fmt.Fprintln(out, "")
		}
	}

	return &Result{AffectedRows: totalAffectedRows, Streamed: true, IsDirectOutput: false}, nil
}

// exportDDL exports database DDL statements
func exportDDL(ctx context.Context, session *Session) (*Result, error) {
	ddl, err := session.adminClient.GetDatabaseDdl(ctx, &dbadminpb.GetDatabaseDdlRequest{
		Database: session.DatabasePath(),
	})
	if err != nil {
		return nil, err
	}

	result := &Result{Rows: make([]Row, 0, len(ddl.Statements)+2)}
	result.Rows = append(result.Rows, Row{"-- Database DDL exported by spanner-mycli"}, Row{""})

	for _, stmt := range ddl.Statements {
		if !strings.HasSuffix(stmt, ";") {
			stmt += ";"
		}
		result.Rows = append(result.Rows, Row{stmt}, Row{""})
	}

	return result, nil
}

// getTableDependencyOrder returns tables in dependency order (parents before children).
// It handles INTERLEAVE IN PARENT relationships and will support foreign keys in Issue #426.
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

		name := tableName.StringVal

		// Get or create table info for the current table
		info, ok := tables[name]
		if !ok {
			info = &tableInfo{Name: name}
			tables[name] = info
		}
		// Update its details from the query result
		info.ParentTable = parentTable.StringVal
		info.OnDeleteAction = onDeleteAction.StringVal
		allTableNames = append(allTableNames, name)

		if parentTable.Valid {
			parentName := parentTable.StringVal
			// Get or create parent table info
			parentInfo, ok := tables[parentName]
			if !ok {
				parentInfo = &tableInfo{Name: parentName}
				tables[parentName] = parentInfo
			}
			// Add current table as a child of the parent
			parentInfo.ChildrenTables = append(parentInfo.ChildrenTables, name)
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

// topologicalSort performs dependency-aware sorting of tables.
// It ensures parent tables are processed before their children and detects circular dependencies.
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
		if info != nil && info.ParentTable != "" && slices.Contains(tablesToExport, info.ParentTable) {
			if err := visit(info.ParentTable); err != nil {
				return err
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
	dataResult, err := executeSQLWithFormat(ctx, session, fmt.Sprintf("SELECT * FROM `%s`", tableName),
		enums.DisplayModeSQLInsert, enums.StreamingModeFalse, tableName)
	if err != nil {
		return nil, err
	}

	result := &Result{
		Rows:         []Row{{fmt.Sprintf("-- Data for table %s", tableName)}},
		AffectedRows: dataResult.AffectedRows,
	}

	if len(dataResult.Rows) > 0 {
		var buf bytes.Buffer
		tempVars := *session.systemVariables
		tempVars.SQLTableName, tempVars.CLIFormat = tableName, enums.DisplayModeSQLInsert
		if err := formatSQL(enums.DisplayModeSQLInsert)(&buf, dataResult, extractTableColumnNames(dataResult.TableHeader), &tempVars, 0); err != nil {
			return nil, fmt.Errorf("failed to format SQL for table %s: %w", tableName, err)
		}
		if buf.Len() > 0 {
			for _, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
				result.Rows = append(result.Rows, Row{line})
			}
		}
	}

	if result.AffectedRows > 0 {
		result.Rows = append(result.Rows, Row{""})
	}

	return result, nil
}
