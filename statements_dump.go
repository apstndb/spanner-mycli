package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"cloud.google.com/go/spanner"
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

// buildSelectQueryWithColumns creates a SELECT query with explicit column list.
// Column names are quoted with backticks to handle reserved words.
// Returns a SQL query string in the format: SELECT `col1`, `col2` FROM `tableName`
func buildSelectQueryWithColumns(columns []string, tableName string) string {
	quotedColumns := make([]string, len(columns))
	for i, col := range columns {
		quotedColumns[i] = fmt.Sprintf("`%s`", col)
	}
	return fmt.Sprintf("SELECT %s FROM `%s`", strings.Join(quotedColumns, ", "), tableName)
}

// getWritableColumnsWithTxn queries INFORMATION_SCHEMA to get only columns that can accept INSERT values.
// It uses the provided transaction to ensure consistency with other queries.
// It excludes generated columns and other non-writable column types.
// Returns column names in their original form, ordered by ORDINAL_POSITION.
// NOTE: INFORMATION_SCHEMA queries cannot be used in read-write transactions.
func getWritableColumnsWithTxn(ctx context.Context, txn *spanner.ReadOnlyTransaction, tableName string) ([]string, error) {
	// Handle both simple and schema-qualified table names.
	// Cloud Spanner table identifiers can only contain letters, numbers, and underscores.
	// Dots are used exclusively to separate schema from table name in fully qualified names (FQNs).
	// Examples: "Users" (simple) or "myschema.Users" (schema-qualified)
	// Reference: https://cloud.google.com/spanner/docs/reference/standard-sql/data-definition-language#names
	//
	// Note: The current dependency resolver only queries tables from the default schema (empty string),
	// so it will only return simple table names. However, we support schema-qualified names
	// for future extensibility when we might need to export from non-default schemas.
	parts := strings.Split(tableName, ".")
	var tableSchema, tableNameOnly string

	switch len(parts) {
	case 1:
		// Simple table name - use default schema (empty string)
		tableSchema = ""
		tableNameOnly = parts[0]
	case 2:
		// Schema-qualified table name (e.g., "myschema.Users")
		tableSchema = parts[0]
		tableNameOnly = parts[1]
	default:
		// More than one dot is invalid - dots can only separate schema from table
		return nil, fmt.Errorf("invalid table name format: %s (expected 'table' or 'schema.table')", tableName)
	}

	// Build the query to get writable columns
	// IS_GENERATED = 'NEVER' filters out all non-writable columns including:
	// - Generated columns (STORED and virtual)
	// - Any future non-writable column types
	query := `
		SELECT COLUMN_NAME
		FROM INFORMATION_SCHEMA.COLUMNS
		WHERE TABLE_SCHEMA = @schema
		  AND TABLE_NAME = @table
		  AND IS_GENERATED = 'NEVER'
		ORDER BY ORDINAL_POSITION`

	stmt := spanner.Statement{
		SQL: query,
		Params: map[string]interface{}{
			"schema": tableSchema,
			"table":  tableNameOnly,
		},
	}

	var columns []string
	iter := txn.Query(ctx, stmt)
	defer iter.Stop()

	err := iter.Do(func(r *spanner.Row) error {
		var columnName string
		if err := r.Column(0, &columnName); err != nil {
			return err
		}
		columns = append(columns, columnName)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query writable columns for %s: %w", tableName, err)
	}

	// Return empty slice for tables with no writable columns (e.g., all generated columns)
	// Callers should handle this case gracefully by skipping data export
	return columns, nil
}

// executeDumpBuffered performs dump operation with buffering.
// All output is collected in memory before being returned.
func executeDumpBuffered(ctx context.Context, session *Session, mode dumpMode, specificTables []string) (*Result, error) {
	result := &Result{AffectedRows: 0, IsDirectOutput: true}

	// Export DDL first if requested (DDL doesn't need transaction consistency)
	if mode.shouldExportDDL() {
		ddlResult, err := exportDDL(ctx, session)
		if err != nil {
			return nil, fmt.Errorf("export DDL: %w", err)
		}
		result.Rows = append(result.Rows, ddlResult.Rows...)
	}

	// Execute all INFORMATION_SCHEMA queries and data export within a single transaction for consistency
	err := session.withReadOnlyTransactionOrStart(ctx, func(txn *spanner.ReadOnlyTransaction) error {
		// Get tables to export (this queries INFORMATION_SCHEMA)
		if !mode.shouldExportData() {
			return nil
		}
		tables, err := getTableDependencyOrderWithTxn(ctx, txn, specificTables)
		if err != nil {
			return fmt.Errorf("failed to get table dependency order: %w", err)
		}

		for _, table := range tables {
			// Get writable columns using the same transaction
			columns, err := getWritableColumnsWithTxn(ctx, txn, table)
			if err != nil {
				return fmt.Errorf("failed to get writable columns for table %s: %w", table, err)
			}

			// Skip tables with no writable columns (e.g., all generated columns)
			if len(columns) == 0 {
				result.Rows = append(result.Rows, Row{fmt.Sprintf("-- Skipping table %s (no writable columns)", table)})
				continue
			}

			// Build SELECT query with explicit column list
			selectQuery := buildSelectQueryWithColumns(columns, table)

			// Execute query using the transaction variant since we're already within a transaction
			dataResult, err := executeSQLWithFormatAndTxn(ctx, session, txn, selectQuery,
				enums.DisplayModeSQLInsert, enums.StreamingModeFalse, table)
			if err != nil {
				return fmt.Errorf("export table %s: %w", table, err)
			}

			// Format the result for buffered output
			result.Rows = append(result.Rows, Row{fmt.Sprintf("-- Data for table %s", table)})

			if len(dataResult.Rows) > 0 {
				var buf bytes.Buffer
				tempVars := *session.systemVariables
				tempVars.SQLTableName, tempVars.CLIFormat = table, enums.DisplayModeSQLInsert
				if err := formatSQL(enums.DisplayModeSQLInsert)(&buf, dataResult, extractTableColumnNames(dataResult.TableHeader), &tempVars, 0); err != nil {
					return fmt.Errorf("failed to format SQL for table %s: %w", table, err)
				}
				if buf.Len() > 0 {
					for _, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
						result.Rows = append(result.Rows, Row{line})
					}
				}
			}

			if dataResult.AffectedRows > 0 {
				result.Rows = append(result.Rows, Row{""})
			}

			result.AffectedRows += dataResult.AffectedRows
		}
		return nil
	})
	if err != nil {
		return nil, err
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
	// Export DDL first if requested (DDL doesn't need transaction consistency)
	if mode.shouldExportDDL() {
		ddlResult, err := exportDDL(ctx, session)
		if err != nil {
			return nil, fmt.Errorf("failed to export DDL: %w", err)
		}
		if err := writeResultRows(out, ddlResult.Rows); err != nil {
			return nil, fmt.Errorf("failed to write DDL: %w", err)
		}
	}

	// Execute all INFORMATION_SCHEMA queries and data export within a single transaction for consistency
	var totalAffectedRows int
	err := session.withReadOnlyTransactionOrStart(ctx, func(txn *spanner.ReadOnlyTransaction) error {
		// Get tables to export (this queries INFORMATION_SCHEMA)
		if !mode.shouldExportData() {
			return nil
		}
		tables, err := getTableDependencyOrderWithTxn(ctx, txn, specificTables)
		if err != nil {
			return fmt.Errorf("failed to get table dependency order: %w", err)
		}

		for _, table := range tables {
			// Get writable columns using the same transaction
			columns, err := getWritableColumnsWithTxn(ctx, txn, table)
			if err != nil {
				return fmt.Errorf("failed to get writable columns for table %s: %w", table, err)
			}

			// Skip tables with no writable columns (e.g., all generated columns)
			if len(columns) == 0 {
				fmt.Fprintf(out, "-- Skipping table %s (no writable columns)\n", table)
				continue
			}

			// Build SELECT query with explicit column list
			selectQuery := buildSelectQueryWithColumns(columns, table)

			// Write table comment
			fmt.Fprintf(out, "-- Data for table %s\n", table)

			// Execute SELECT with explicit columns - SQL formatter streams INSERT statements directly to output
			// Use the transaction variant since we're already within a transaction
			dataResult, err := executeSQLWithFormatAndTxn(ctx, session, txn, selectQuery,
				enums.DisplayModeSQLInsert, enums.StreamingModeTrue, table)
			if err != nil {
				return fmt.Errorf("failed to export table %s: %w", table, err)
			}

			totalAffectedRows += dataResult.AffectedRows
			if dataResult.AffectedRows > 0 {
				fmt.Fprintln(out, "")
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
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

// getTableDependencyOrderWithTxn returns tables in dependency order using a transaction for consistency.
// It handles both INTERLEAVE IN PARENT relationships and foreign key constraints.
func getTableDependencyOrderWithTxn(ctx context.Context, txn *spanner.ReadOnlyTransaction, specificTables []string) ([]string, error) {
	resolver := NewDependencyResolver()

	// Build the complete dependency graph using the transaction
	if err := resolver.BuildDependencyGraphWithTxn(ctx, txn); err != nil {
		return nil, fmt.Errorf("failed to build dependency graph: %w", err)
	}

	// Get tables in dependency order
	if len(specificTables) > 0 {
		return resolver.GetOrderForTables(specificTables)
	}

	return resolver.GetTableOrder()
}
