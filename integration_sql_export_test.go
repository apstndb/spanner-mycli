package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/apstndb/gsqlutils"
	"github.com/apstndb/spanner-mycli/enums"
)

func TestSQLExportIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tests := []struct {
		name       string
		exportMode enums.DisplayMode
		tableName  string
		batchSize  int64
		query      string
		verifySQL  string
		wantRows   []Row
	}{
		{
			name:       "SQL_INSERT export and import",
			exportMode: enums.DisplayModeSQLInsert,
			tableName:  "DestTable",
			batchSize:  0, // Single-row inserts
			query:      "SELECT * FROM SourceTable ORDER BY id",
			// Use CAST to get consistent format - but timestamps still vary by timezone
			verifySQL: "SELECT id, name, CAST(value AS STRING) AS value, active, IFNULL(CAST(created_at AS STRING), 'NULL') AS created_at FROM DestTable ORDER BY id",
			wantRows: []Row{
				{"1", "Alice", "100.5", "true", "2024-01-01T00:00:00Z"},
				{"2", "Bob", "200.75", "false", "2024-01-02T00:00:00Z"},
				{"3", "NULL", "300", "true", "NULL"},
			},
		},
		{
			name:       "SQL_INSERT_OR_UPDATE with batching",
			exportMode: enums.DisplayModeSQLInsertOrUpdate,
			tableName:  "DestTable",
			batchSize:  2, // Batch size of 2
			query:      "SELECT * FROM SourceTable WHERE id <= 2 ORDER BY id",
			verifySQL:  "SELECT id, name FROM DestTable WHERE id <= 2 ORDER BY id",
			wantRows: []Row{
				{"1", "Alice"},
				{"2", "Bob"},
			},
		},
		{
			name:       "SQL export with table rename",
			exportMode: enums.DisplayModeSQLInsertOrIgnore,
			tableName:  "DestTable", // Different from source
			query:      "SELECT id, name, value, active, created_at FROM SourceTable WHERE id = 1",
			verifySQL:  "SELECT id, name FROM DestTable WHERE id = 1",
			wantRows: []Row{
				{"1", "Alice"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
			defer cancel()

			// Create source table with test data
			sourceDDL := `
CREATE TABLE SourceTable (
	id INT64 NOT NULL,
	name STRING(100),
	value FLOAT64,
	active BOOL,
	created_at TIMESTAMP,
) PRIMARY KEY (id)`

			// Create destination table with same schema but different name
			destDDL := `
CREATE TABLE DestTable (
	id INT64 NOT NULL,
	name STRING(100),
	value FLOAT64,
	active BOOL,
	created_at TIMESTAMP,
) PRIMARY KEY (id)`

			// Insert test data into source table
			insertDMLs := []string{
				`INSERT INTO SourceTable (id, name, value, active, created_at) VALUES (1, 'Alice', 100.5, true, TIMESTAMP '2024-01-01T00:00:00Z')`,
				`INSERT INTO SourceTable (id, name, value, active, created_at) VALUES (2, 'Bob', 200.75, false, TIMESTAMP '2024-01-02T00:00:00Z')`,
				`INSERT INTO SourceTable (id, name, value, active, created_at) VALUES (3, NULL, 300.0, true, NULL)`,
			}

			// Each test gets its own database
			_, session, teardown := initialize(t, []string{sourceDDL, destDDL}, insertDMLs)
			defer teardown()

			// Verify source data exists
			countStmt, err := BuildStatement("SELECT COUNT(*) FROM SourceTable")
			if err != nil {
				t.Fatalf("Failed to build count statement: %v", err)
			}
			countResult, err := countStmt.Execute(ctx, session)
			if err != nil {
				t.Fatalf("Failed to count source data: %v", err)
			}
			t.Logf("Source table row count: %v", countResult.Rows)

			// Set up system variables for SQL export
			session.systemVariables.CLIFormat = tt.exportMode
			session.systemVariables.SQLTableName = tt.tableName
			session.systemVariables.SQLBatchSize = tt.batchSize
			// Force buffered mode for testing
			session.systemVariables.StreamingMode = enums.StreamingModeFalse

			// Execute the query - with SQL format set, it should use proper SQL literal formatting
			stmt, err := BuildStatement(tt.query)
			if err != nil {
				t.Fatalf("Failed to build export statement: %v", err)
			}

			result, err := stmt.Execute(ctx, session)
			if err != nil {
				t.Fatalf("Failed to execute export query: %v", err)
			}

			// Debug: Log result details
			t.Logf("Query result: rows=%d, header=%v", len(result.Rows), result.TableHeader)

			// Capture the SQL export output
			var buf bytes.Buffer
			err = printTableData(session.systemVariables, 0, &buf, result)
			if err != nil {
				t.Fatalf("Failed to format SQL export: %v", err)
			}

			sqlOutput := buf.String()
			t.Logf("Generated SQL:\n%s", sqlOutput)

			// Debug: Log if SQL output is empty
			if sqlOutput == "" {
				t.Logf("WARNING: SQL output is empty! Format=%v, TableName=%s, BatchSize=%d",
					tt.exportMode, tt.tableName, tt.batchSize)
			}

			// Execute each generated SQL statement
			// Use proper SQL statement splitter that handles semicolons in string literals
			rawStatements, err := gsqlutils.SeparateInputPreserveCommentsWithStatus("", sqlOutput)
			if err != nil {
				t.Fatalf("Failed to split generated SQL statements: %v", err)
			}

			for _, rawStmt := range rawStatements {
				sqlStmt := strings.TrimSpace(rawStmt.Statement)
				if sqlStmt == "" {
					continue
				}

				// Execute the INSERT statement
				insertStmt, err := BuildStatement(sqlStmt)
				if err != nil {
					t.Fatalf("Failed to build INSERT statement: %v\nSQL: %s", err, sqlStmt)
				}
				_, err = insertStmt.Execute(ctx, session)
				if err != nil {
					t.Fatalf("Failed to execute generated SQL: %v\nSQL: %s", err, sqlStmt)
				}
			}

			// Verify the data was imported correctly
			// Reset format to default for verification to get normal display format
			session.systemVariables.CLIFormat = enums.DisplayModeTable

			verifyStmt, err := BuildStatement(tt.verifySQL)
			if err != nil {
				t.Fatalf("Failed to build verify statement: %v", err)
			}

			verifyResult, err := verifyStmt.Execute(ctx, session)
			if err != nil {
				t.Fatalf("Failed to execute verify query: %v", err)
			}

			// Compare results - for timestamp columns, just verify non-NULL values exist
			if len(verifyResult.Rows) != len(tt.wantRows) {
				t.Errorf("Row count mismatch: got %d, want %d", len(verifyResult.Rows), len(tt.wantRows))
			} else {
				for i, gotRow := range verifyResult.Rows {
					wantRow := tt.wantRows[i]
					if len(gotRow) != len(wantRow) {
						t.Errorf("Row %d column count mismatch: got %d, want %d", i, len(gotRow), len(wantRow))
						continue
					}
					for j := range gotRow {
						// Skip timestamp columns - just verify NULL vs non-NULL
						if strings.Contains(tt.verifySQL, "created_at") && j == len(gotRow)-1 {
							if (gotRow[j] == "NULL") != (wantRow[j] == "NULL") {
								t.Errorf("Row %d col %d NULL mismatch: got %q, want %q", i, j, gotRow[j], wantRow[j])
							}
						} else if gotRow[j] != wantRow[j] {
							t.Errorf("Row %d col %d mismatch: got %q, want %q", i, j, gotRow[j], wantRow[j])
						}
					}
				}
			}
		})
	}
}

func TestSQLExportWithComplexTypes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
	defer cancel()

	// Create table with complex types
	ddl := `
CREATE TABLE ComplexTable (
	id INT64 NOT NULL,
	arr ARRAY<INT64>,
	json_col JSON,
	bytes_col BYTES(100),
	numeric_col NUMERIC,
) PRIMARY KEY (id)`

	// Insert test data with complex types
	insertDML := `INSERT INTO ComplexTable (id, arr, json_col, bytes_col, numeric_col) VALUES 
		(1, [1, 2, 3], JSON '{"key": "value"}', b'hello', NUMERIC '123.456'),
		(2, [], JSON 'null', NULL, NULL)`

	_, session, teardown := initialize(t, []string{ddl}, []string{insertDML})
	defer teardown()

	// Export with SQL_INSERT
	session.systemVariables.CLIFormat = enums.DisplayModeSQLInsert
	session.systemVariables.SQLTableName = "ComplexTable"
	session.systemVariables.SQLBatchSize = 0
	session.systemVariables.StreamingMode = enums.StreamingModeFalse // Force buffered mode

	stmt, err := BuildStatement("SELECT * FROM ComplexTable ORDER BY id")
	if err != nil {
		t.Fatalf("Failed to build statement: %v", err)
	}

	result, err := stmt.Execute(ctx, session)
	if err != nil {
		t.Fatalf("Failed to execute query: %v", err)
	}

	// Generate SQL export
	var buf bytes.Buffer
	err = printTableData(session.systemVariables, 0, &buf, result)
	if err != nil {
		t.Fatalf("Failed to format SQL export: %v", err)
	}

	sqlOutput := buf.String()
	t.Logf("SQL export with complex types:\n%s", sqlOutput)

	// Verify the SQL contains proper type literals
	expectedPatterns := []string{
		"[1, 2, 3]",                // Array literal (Spanner format)
		"JSON",                     // JSON literal
		`b"\x68\x65\x6c\x6c\x6f"`,  // Bytes literal as hex
		"NUMERIC",                  // Numeric literal
		"NULL",                     // NULL values
		"INSERT INTO ComplexTable", // Table name
	}

	for _, pattern := range expectedPatterns {
		if !strings.Contains(sqlOutput, pattern) {
			t.Errorf("Expected SQL output to contain %q, but it didn't.\nOutput: %s", pattern, sqlOutput)
		}
	}
}
