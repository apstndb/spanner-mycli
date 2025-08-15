package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	dbadminpb "cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/apstndb/spanner-mycli/enums"
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
// It handles both INTERLEAVE IN PARENT relationships and foreign key constraints.
func getTableDependencyOrder(ctx context.Context, session *Session, specificTables []string) ([]string, error) {
	resolver := NewDependencyResolver()

	// Build the complete dependency graph
	if err := resolver.BuildDependencyGraph(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to build dependency graph: %w", err)
	}

	// Get tables in dependency order
	if len(specificTables) > 0 {
		return resolver.GetOrderForTables(specificTables)
	}

	return resolver.GetTableOrder(ctx, session)
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
