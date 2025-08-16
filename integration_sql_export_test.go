package main

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/gsqlutils"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spantype/typector"
	"github.com/apstndb/spanvalue/gcvctor"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/structpb"
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
		wantRows   [][]spanner.GenericColumnValue
	}{
		{
			name:       "SQL_INSERT export and import",
			exportMode: enums.DisplayModeSQLInsert,
			tableName:  "DestTable",
			batchSize:  0, // Single-row inserts
			query:      "SELECT * FROM SourceTable ORDER BY id",
			verifySQL:  "SELECT id, name, value, active, created_at FROM DestTable ORDER BY id",
			wantRows: [][]spanner.GenericColumnValue{
				{
					gcvctor.Int64Value(1),
					gcvctor.StringValue("Alice"),
					gcvctor.Float64Value(100.5),
					gcvctor.BoolValue(true),
					gcvctor.TimestampValue(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
				},
				{
					gcvctor.Int64Value(2),
					gcvctor.StringValue("Bob"),
					gcvctor.Float64Value(200.75),
					gcvctor.BoolValue(false),
					gcvctor.TimestampValue(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)),
				},
				{
					gcvctor.Int64Value(3),
					gcvctor.SimpleTypedNull(sppb.TypeCode_STRING),
					gcvctor.Float64Value(300),
					gcvctor.BoolValue(true),
					gcvctor.SimpleTypedNull(sppb.TypeCode_TIMESTAMP),
				},
			},
		},
		{
			name:       "SQL_INSERT_OR_UPDATE with batching",
			exportMode: enums.DisplayModeSQLInsertOrUpdate,
			tableName:  "DestTable",
			batchSize:  2, // Batch size of 2
			query:      "SELECT * FROM SourceTable WHERE id <= 2 ORDER BY id",
			verifySQL:  "SELECT id, name FROM DestTable WHERE id <= 2 ORDER BY id",
			wantRows: [][]spanner.GenericColumnValue{
				{gcvctor.Int64Value(1), gcvctor.StringValue("Alice")},
				{gcvctor.Int64Value(2), gcvctor.StringValue("Bob")},
			},
		},
		{
			name:       "SQL export with table rename",
			exportMode: enums.DisplayModeSQLInsertOrIgnore,
			tableName:  "DestTable", // Different from source
			query:      "SELECT id, name, value, active, created_at FROM SourceTable WHERE id = 1",
			verifySQL:  "SELECT id, name FROM DestTable WHERE id = 1",
			wantRows: [][]spanner.GenericColumnValue{
				{gcvctor.Int64Value(1), gcvctor.StringValue("Alice")},
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

			// Verify the data was imported correctly using Spanner client API directly
			// This avoids circular dependency on spanner-mycli's formatting
			iter := session.client.Single().Query(ctx, spanner.Statement{SQL: tt.verifySQL})
			defer iter.Stop()

			var gotRows [][]spanner.GenericColumnValue
			err = iter.Do(func(row *spanner.Row) error {
				// Convert row to GenericColumnValue slice
				var gcvs []spanner.GenericColumnValue
				for i := 0; i < row.Size(); i++ {
					var gcv spanner.GenericColumnValue
					if err := row.Column(i, &gcv); err != nil {
						return fmt.Errorf("failed to read column %d: %w", i, err)
					}
					gcvs = append(gcvs, gcv)
				}
				gotRows = append(gotRows, gcvs)
				return nil
			})
			if err != nil {
				t.Fatalf("Failed to query verification data: %v", err)
			}

			// Compare results using go-cmp with protocmp for protobuf types
			if diff := cmp.Diff(tt.wantRows, gotRows, protocmp.Transform()); diff != "" {
				t.Errorf("Verification data mismatch (-want +got):\n%s", diff)
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

	// Create source and destination tables with complex types
	sourceDDL := `
CREATE TABLE ComplexSource (
	id INT64 NOT NULL,
	arr ARRAY<INT64>,
	json_col JSON,
	bytes_col BYTES(100),
	numeric_col NUMERIC,
) PRIMARY KEY (id)`

	destDDL := `
CREATE TABLE ComplexDest (
	id INT64 NOT NULL,
	arr ARRAY<INT64>,
	json_col JSON,
	bytes_col BYTES(100),
	numeric_col NUMERIC,
) PRIMARY KEY (id)`

	// Insert test data with complex types
	insertDML := `INSERT INTO ComplexSource (id, arr, json_col, bytes_col, numeric_col) VALUES 
		(1, [1, 2, 3], JSON '{"key": "value"}', b'hello', NUMERIC '123.456'),
		(2, [], JSON 'null', NULL, NULL)`

	_, session, teardown := initialize(t, []string{sourceDDL, destDDL}, []string{insertDML})
	defer teardown()

	// Export with SQL_INSERT
	session.systemVariables.CLIFormat = enums.DisplayModeSQLInsert
	session.systemVariables.SQLTableName = "ComplexDest"
	session.systemVariables.SQLBatchSize = 0
	session.systemVariables.StreamingMode = enums.StreamingModeFalse // Force buffered mode

	stmt, err := BuildStatement("SELECT * FROM ComplexSource ORDER BY id")
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

	// Execute the generated SQL to import data
	rawStatements, err := gsqlutils.SeparateInputPreserveCommentsWithStatus("", sqlOutput)
	if err != nil {
		t.Fatalf("Failed to split generated SQL statements: %v", err)
	}

	for _, rawStmt := range rawStatements {
		sqlStmt := strings.TrimSpace(rawStmt.Statement)
		if sqlStmt == "" {
			continue
		}

		insertStmt, err := BuildStatement(sqlStmt)
		if err != nil {
			t.Fatalf("Failed to build INSERT statement: %v\nSQL: %s", err, sqlStmt)
		}
		_, err = insertStmt.Execute(ctx, session)
		if err != nil {
			t.Fatalf("Failed to execute generated SQL: %v\nSQL: %s", err, sqlStmt)
		}
	}

	// Verify the data was imported correctly using Spanner client API
	verifySQL := "SELECT id, arr, json_col, bytes_col, numeric_col FROM ComplexDest ORDER BY id"
	iter := session.client.Single().Query(ctx, spanner.Statement{SQL: verifySQL})
	defer iter.Stop()

	// Define expected values using GenericColumnValue
	expectedRows := [][]spanner.GenericColumnValue{
		{
			gcvctor.Int64Value(1),
			// Array of INT64
			func() spanner.GenericColumnValue {
				v, _ := gcvctor.ArrayValue(
					gcvctor.Int64Value(1),
					gcvctor.Int64Value(2),
					gcvctor.Int64Value(3),
				)
				return v
			}(),
			// JSON
			func() spanner.GenericColumnValue {
				v, _ := gcvctor.JSONValue(map[string]interface{}{"key": "value"})
				return v
			}(),
			// BYTES
			gcvctor.BytesValue([]byte("hello")),
			// NUMERIC - Spanner normalizes and returns "123.456" even though we insert with trailing zeros
			gcvctor.StringBasedValue(sppb.TypeCode_NUMERIC, "123.456"),
		},
		{
			gcvctor.Int64Value(2),
			// Empty array of INT64 - gcvctor doesn't have a function for empty arrays, so create manually
			spanner.GenericColumnValue{
				Type: typector.ElemCodeToArrayType(sppb.TypeCode_INT64),
				Value: &structpb.Value{
					Kind: &structpb.Value_ListValue{
						ListValue: &structpb.ListValue{
							Values: []*structpb.Value{},
						},
					},
				},
			},
			// JSON null
			func() spanner.GenericColumnValue {
				v, _ := gcvctor.JSONValue(nil)
				return v
			}(),
			// NULL BYTES
			gcvctor.SimpleTypedNull(sppb.TypeCode_BYTES),
			// NULL NUMERIC
			gcvctor.SimpleTypedNull(sppb.TypeCode_NUMERIC),
		},
	}

	var gotRows [][]spanner.GenericColumnValue
	err = iter.Do(func(row *spanner.Row) error {
		var gcvs []spanner.GenericColumnValue
		for i := 0; i < row.Size(); i++ {
			var gcv spanner.GenericColumnValue
			if err := row.Column(i, &gcv); err != nil {
				return fmt.Errorf("failed to read column %d: %w", i, err)
			}
			gcvs = append(gcvs, gcv)
		}
		gotRows = append(gotRows, gcvs)
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to query verification data: %v", err)
	}

	// Compare results using go-cmp with protocmp for protobuf types
	if diff := cmp.Diff(expectedRows, gotRows, protocmp.Transform()); diff != "" {
		t.Errorf("Complex type data mismatch (-want +got):\n%s", diff)
	}
}

func TestSQLExportStreamingMode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tests := []struct {
		name       string
		exportMode enums.DisplayMode
		tableName  string
		batchSize  int64
		query      string
	}{
		{
			name:       "SQL_INSERT streaming",
			exportMode: enums.DisplayModeSQLInsert,
			tableName:  "TestTable",
			batchSize:  0,
			query:      "SELECT * FROM TestTable ORDER BY id",
		},
		{
			name:       "SQL_INSERT_OR_UPDATE streaming with batching",
			exportMode: enums.DisplayModeSQLInsertOrUpdate,
			tableName:  "TestTable",
			batchSize:  2,
			query:      "SELECT * FROM TestTable ORDER BY id",
		},
		{
			name:       "SQL_INSERT_OR_IGNORE streaming",
			exportMode: enums.DisplayModeSQLInsertOrIgnore,
			tableName:  "TestTable",
			batchSize:  0,
			query:      "SELECT * FROM TestTable WHERE id = 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
			defer cancel()

			// Create test table
			ddl := `
CREATE TABLE TestTable (
	id INT64 NOT NULL,
	name STRING(100),
	value FLOAT64,
) PRIMARY KEY (id)`

			// Insert test data
			insertDMLs := []string{
				`INSERT INTO TestTable (id, name, value) VALUES (1, 'Alice', 100.5)`,
				`INSERT INTO TestTable (id, name, value) VALUES (2, 'Bob', 200.75)`,
				`INSERT INTO TestTable (id, name, value) VALUES (3, 'Charlie', 300.0)`,
			}

			_, session, teardown := initialize(t, []string{ddl}, insertDMLs)
			defer teardown()

			// Set up system variables for SQL export with STREAMING mode
			session.systemVariables.CLIFormat = tt.exportMode
			session.systemVariables.SQLTableName = tt.tableName
			session.systemVariables.SQLBatchSize = tt.batchSize
			session.systemVariables.StreamingMode = enums.StreamingModeTrue // Force streaming mode

			// Set up a buffer to capture streaming output
			var buf bytes.Buffer

			// Create StreamManager and assign it to system variables
			// The streaming execution will use GetOutputStream() to get the writer
			session.systemVariables.StreamManager = NewStreamManager(nil, &buf, &buf)

			// Execute the query - with streaming mode, output goes directly to buffer
			stmt, err := BuildStatement(tt.query)
			if err != nil {
				t.Fatalf("Failed to build statement: %v", err)
			}

			// Execute will stream directly to the buffer via StreamManager
			result, err := stmt.Execute(ctx, session)
			if err != nil {
				t.Fatalf("Failed to execute query: %v", err)
			}

			// Get the streamed SQL output
			sqlOutput := buf.String()
			t.Logf("Streamed SQL output:\n%s", sqlOutput)

			// In streaming mode, result should be minimal (no buffered rows)
			if result != nil && len(result.Rows) > 0 {
				t.Logf("Warning: Got %d buffered rows in streaming mode", len(result.Rows))
			}

			// Verify the output contains expected SQL statements
			if sqlOutput == "" {
				t.Errorf("Expected SQL output but got empty string")
			}

			// Check for expected patterns based on export mode
			switch tt.exportMode {
			case enums.DisplayModeSQLInsert:
				if !strings.Contains(sqlOutput, "INSERT INTO TestTable") {
					t.Errorf("Expected INSERT INTO statement, got: %s", sqlOutput)
				}
			case enums.DisplayModeSQLInsertOrUpdate:
				if !strings.Contains(sqlOutput, "INSERT OR UPDATE INTO TestTable") {
					t.Errorf("Expected INSERT OR UPDATE statement, got: %s", sqlOutput)
				}
			case enums.DisplayModeSQLInsertOrIgnore:
				if !strings.Contains(sqlOutput, "INSERT OR IGNORE INTO TestTable") {
					t.Errorf("Expected INSERT OR IGNORE statement, got: %s", sqlOutput)
				}
			}

			// Verify the data is correctly formatted
			expectedValues := []string{
				`"Alice"`,
				`"Bob"`,
				`100.5`,
				`200.75`,
			}

			for _, value := range expectedValues {
				if tt.name != "SQL_INSERT_OR_IGNORE streaming" || (value != `"Bob"` && value != `200.75`) {
					// INSERT OR IGNORE only exports id=1
					if !strings.Contains(sqlOutput, value) {
						t.Errorf("Expected value %s in output, got: %s", value, sqlOutput)
					}
				}
			}

			// For batched mode, verify batch formatting
			if tt.batchSize > 0 && tt.exportMode == enums.DisplayModeSQLInsertOrUpdate {
				if !strings.Contains(sqlOutput, "VALUES\n") {
					t.Errorf("Expected multi-line VALUES clause for batched mode")
				}
			}
		})
	}
}

func TestSQLExportWithUnnamedColumns(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// Create a test table
	ddl := `CREATE TABLE TestTable (id INT64 NOT NULL, name STRING(100)) PRIMARY KEY (id)`
	insertDML := `INSERT INTO TestTable (id, name) VALUES (1, 'Alice'), (2, 'Bob')`

	_, session, teardown := initialize(t, []string{ddl}, []string{insertDML})
	defer teardown()

	// Test cases with unnamed columns
	testCases := []struct {
		name      string
		query     string
		expectErr bool
		desc      string
	}{
		{
			name:      "Expressions without aliases",
			query:     "SELECT id + 10, UPPER(name), id * 2 FROM TestTable WHERE id = 1",
			expectErr: true, // SQL export should fail with unnamed columns
			desc:      "Mathematical and string expressions",
		},
		{
			name:      "Literal values",
			query:     "SELECT 1, 'literal', true, NULL FROM TestTable WHERE id = 1",
			expectErr: true,
			desc:      "Literal values without aliases",
		},
		{
			name:      "Aggregate function",
			query:     "SELECT COUNT(*), MAX(id), MIN(name) FROM TestTable",
			expectErr: true,
			desc:      "Aggregate functions without aliases",
		},
		{
			name:      "Mixed named and unnamed",
			query:     "SELECT id, id + 10, name AS customer_name, UPPER(name) FROM TestTable WHERE id = 1",
			expectErr: true, // Even one unnamed column should cause failure
			desc:      "Mix of named and unnamed columns",
		},
		{
			name:      "All columns named with aliases",
			query:     "SELECT id AS record_id, id + 10 AS computed_id, name AS customer_name FROM TestTable WHERE id = 1",
			expectErr: false, // Should work fine with all columns named
			desc:      "All columns have explicit aliases",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up for SQL export
			session.systemVariables.CLIFormat = enums.DisplayModeSQLInsert
			session.systemVariables.SQLTableName = "TargetTable"
			session.systemVariables.SQLBatchSize = 0
			session.systemVariables.StreamingMode = enums.StreamingModeFalse

			stmt, err := BuildStatement(tc.query)
			if err != nil {
				t.Fatalf("Failed to build statement: %v", err)
			}

			result, err := stmt.Execute(ctx, session)
			if err != nil {
				t.Fatalf("Failed to execute query: %v", err)
			}

			// Extract column names using the same method as printTableData
			columnNames := extractTableColumnNames(result.TableHeader)

			t.Logf("Test: %s", tc.desc)
			t.Logf("Query: %s", tc.query)
			t.Logf("Column names: %v", columnNames)
			t.Logf("Row count: %d", len(result.Rows))

			// Check for unnamed columns (empty strings)
			unnamedCount := 0
			for i, name := range columnNames {
				if name == "" {
					t.Logf("  Column %d is unnamed", i)
					unnamedCount++
				}
			}

			// Try to format as SQL
			var buf bytes.Buffer
			err = printTableData(session.systemVariables, 0, &buf, result)

			if tc.expectErr {
				if err == nil {
					t.Errorf("Expected error for unnamed columns but got none")
				} else {
					t.Logf("Got expected error: %v", err)
					// Check if error message is helpful
					if !strings.Contains(err.Error(), "no name") && !strings.Contains(err.Error(), "aliases") {
						t.Logf("Warning: Error message could be more helpful: %v", err)
					}
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error formatting SQL: %v", err)
				} else {
					sqlOutput := buf.String()
					if sqlOutput == "" {
						t.Errorf("Expected SQL output but got empty string")
					} else {
						t.Logf("SQL output generated successfully:")
						// Show first few lines to avoid too much output
						lines := strings.Split(sqlOutput, "\n")
						for i, line := range lines {
							if i < 3 || len(lines) <= 5 {
								t.Logf("  %s", line)
							} else if i == 3 {
								t.Logf("  ... (%d more lines)", len(lines)-3)
								break
							}
						}
					}
				}
			}

			t.Logf("---")
		})
	}

	// Also test streaming mode
	t.Run("Streaming mode with unnamed columns", func(t *testing.T) {
		session.systemVariables.CLIFormat = enums.DisplayModeSQLInsert
		session.systemVariables.SQLTableName = "StreamTarget"
		session.systemVariables.SQLBatchSize = 0
		session.systemVariables.StreamingMode = enums.StreamingModeTrue

		// Set up StreamManager for streaming
		var buf bytes.Buffer
		session.systemVariables.StreamManager = NewStreamManager(nil, &buf, &buf)

		query := "SELECT id + 100, CONCAT('Name: ', name) FROM TestTable"
		stmt, err := BuildStatement(query)
		if err != nil {
			t.Fatalf("Failed to build statement: %v", err)
		}

		result, err := stmt.Execute(ctx, session)

		t.Logf("Streaming mode test:")
		t.Logf("Query: %s", query)

		// Should get an error about unnamed columns in streaming mode too
		if err != nil {
			t.Logf("Got expected error: %v", err)
			if !strings.Contains(err.Error(), "no name") && !strings.Contains(err.Error(), "aliases") {
				t.Logf("Note: Error might be from streaming setup, not column validation")
			}
		} else {
			t.Errorf("Expected error for unnamed columns in streaming mode but got none")
			if result != nil {
				t.Logf("Result returned (rows: %d)", len(result.Rows))
			}
		}

		streamOutput := buf.String()
		if streamOutput != "" {
			t.Errorf("Unexpected stream output with unnamed columns: %s", streamOutput)
		} else {
			t.Logf("No stream output (as expected with unnamed columns)")
		}
	})

	// Test streaming mode with properly named columns
	t.Run("Streaming mode with named columns", func(t *testing.T) {
		session.systemVariables.CLIFormat = enums.DisplayModeSQLInsert
		session.systemVariables.SQLTableName = "StreamTarget"
		session.systemVariables.SQLBatchSize = 0
		session.systemVariables.StreamingMode = enums.StreamingModeTrue

		// Set up StreamManager for streaming
		var buf bytes.Buffer
		session.systemVariables.StreamManager = NewStreamManager(nil, &buf, &buf)

		query := "SELECT id AS record_id, name AS customer_name FROM TestTable"
		stmt, err := BuildStatement(query)
		if err != nil {
			t.Fatalf("Failed to build statement: %v", err)
		}

		result, err := stmt.Execute(ctx, session)

		t.Logf("Streaming mode test with named columns:")
		t.Logf("Query: %s", query)

		if err != nil {
			t.Errorf("Unexpected error with named columns: %v", err)
		} else {
			t.Logf("Execution successful")
			if result != nil {
				t.Logf("Result returned (rows: %d)", len(result.Rows))
			}
		}

		streamOutput := buf.String()
		if streamOutput != "" {
			lines := strings.Split(streamOutput, "\n")
			t.Logf("Stream output (%d lines):", len(lines))
			for i, line := range lines {
				if i < 3 {
					t.Logf("  %s", line)
				}
			}
			// Verify the output contains expected SQL
			if !strings.Contains(streamOutput, "INSERT INTO StreamTarget") {
				t.Errorf("Expected INSERT statement in output")
			}
		} else {
			t.Errorf("Expected stream output with named columns but got empty string")
		}
	})
}

func TestSQLExportAutoTableNameDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tests := []struct {
		name             string
		query            string
		shouldAutoDetect bool
		expectedTable    string
		exportMode       enums.DisplayMode
		description      string
	}{
		{
			name:             "Simple SELECT * auto-detection",
			query:            "SELECT * FROM TestTable",
			shouldAutoDetect: true,
			expectedTable:    "TestTable",
			exportMode:       enums.DisplayModeSQLInsert,
			description:      "Basic SELECT * should auto-detect table name",
		},
		{
			name:             "SELECT * with WHERE clause",
			query:            "SELECT * FROM TestTable WHERE id > 1",
			shouldAutoDetect: true,
			expectedTable:    "TestTable",
			exportMode:       enums.DisplayModeSQLInsert,
			description:      "Auto-detect with WHERE clause",
		},
		{
			name:             "SELECT * with ORDER BY",
			query:            "SELECT * FROM TestTable ORDER BY name DESC",
			shouldAutoDetect: true,
			expectedTable:    "TestTable",
			exportMode:       enums.DisplayModeSQLInsertOrUpdate,
			description:      "Auto-detect with ORDER BY",
		},
		{
			name:             "SELECT * with LIMIT",
			query:            "SELECT * FROM TestTable LIMIT 1",
			shouldAutoDetect: true,
			expectedTable:    "TestTable",
			exportMode:       enums.DisplayModeSQLInsertOrIgnore,
			description:      "Auto-detect with LIMIT",
		},
		{
			name:             "SELECT * with WHERE, ORDER BY, and LIMIT",
			query:            "SELECT * FROM TestTable WHERE id > 0 ORDER BY id LIMIT 2",
			shouldAutoDetect: true,
			expectedTable:    "TestTable",
			exportMode:       enums.DisplayModeSQLInsert,
			description:      "Auto-detect with multiple clauses",
		},
		{
			name:             "Table name with backticks",
			query:            "SELECT * FROM `TestTable`",
			shouldAutoDetect: true,
			expectedTable:    "TestTable", // Backticks not needed in output since TestTable is not a reserved word
			exportMode:       enums.DisplayModeSQLInsert,
			description:      "Auto-detect with quoted table name",
		},
		{
			name:             "SELECT with column list - no auto-detection",
			query:            "SELECT id, name FROM TestTable",
			shouldAutoDetect: false,
			expectedTable:    "",
			exportMode:       enums.DisplayModeSQLInsert,
			description:      "Column list should not auto-detect",
		},
		{
			name:             "SELECT with JOIN - no auto-detection",
			query:            "SELECT * FROM TestTable t1 JOIN TestTable t2 ON t1.id = t2.id",
			shouldAutoDetect: false,
			expectedTable:    "",
			exportMode:       enums.DisplayModeSQLInsert,
			description:      "JOIN should not auto-detect",
		},
		{
			name:             "SELECT with subquery - no auto-detection",
			query:            "SELECT * FROM (SELECT * FROM TestTable)",
			shouldAutoDetect: false,
			expectedTable:    "",
			exportMode:       enums.DisplayModeSQLInsert,
			description:      "Subquery should not auto-detect",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
			defer cancel()

			// Create test table
			ddl := `
CREATE TABLE TestTable (
	id INT64 NOT NULL,
	name STRING(100),
	value FLOAT64,
) PRIMARY KEY (id)`

			// Insert test data
			insertDMLs := []string{
				`INSERT INTO TestTable (id, name, value) VALUES (1, 'Alice', 100.5)`,
				`INSERT INTO TestTable (id, name, value) VALUES (2, 'Bob', 200.75)`,
				`INSERT INTO TestTable (id, name, value) VALUES (3, 'Charlie', 300.0)`,
			}

			_, session, teardown := initialize(t, []string{ddl}, insertDMLs)
			defer teardown()

			// Set up system variables for SQL export
			// DO NOT set SQLTableName - we want to test auto-detection
			session.systemVariables.CLIFormat = tt.exportMode
			session.systemVariables.SQLTableName = "" // Explicitly empty for auto-detection
			session.systemVariables.SQLBatchSize = 0
			session.systemVariables.StreamingMode = enums.StreamingModeFalse

			t.Logf("Testing: %s", tt.description)
			t.Logf("Query: %s", tt.query)

			// Execute the query
			stmt, err := BuildStatement(tt.query)
			if err != nil {
				t.Fatalf("Failed to build statement: %v", err)
			}

			result, err := stmt.Execute(ctx, session)

			if tt.shouldAutoDetect {
				// Auto-detection happens during execution, so the result should already
				// have been formatted with the auto-detected table name.
				// We can verify this by checking if the result contains properly formatted data.
				if err != nil {
					t.Fatalf("Expected successful execution with auto-detection, got error: %v", err)
				}

				// The result should have been streamed or buffered with SQL format
				// Check if we got valid result with rows
				if result == nil || (len(result.Rows) == 0 && !result.Streamed) {
					t.Fatalf("Expected result with data but got empty result")
				}

				// For buffered mode (which the test uses), we can verify by formatting
				// However, since auto-detection only works during execution,
				// we need to re-execute with streaming to see the actual SQL output

				// Re-execute the same query with streaming to capture SQL output
				var buf bytes.Buffer
				session.systemVariables.StreamManager = NewStreamManager(nil, &buf, &buf)
				session.systemVariables.StreamingMode = enums.StreamingModeTrue

				// Execute again with streaming to capture the SQL output
				_, err = stmt.Execute(ctx, session)
				if err != nil {
					t.Fatalf("Failed to execute with streaming: %v", err)
				}

				sqlOutput := buf.String()
				t.Logf("Generated SQL with auto-detected table:\n%s", sqlOutput)

				// Verify the output contains the expected table name
				expectedPattern := fmt.Sprintf("INTO %s", tt.expectedTable)
				if !strings.Contains(sqlOutput, expectedPattern) {
					t.Errorf("Expected table name %s in output, but got:\n%s", tt.expectedTable, sqlOutput)
				}

				// Verify it's valid SQL by parsing the generated statements
				rawStatements, err := gsqlutils.SeparateInputPreserveCommentsWithStatus("", sqlOutput)
				if err != nil {
					t.Errorf("Generated SQL is not valid: %v", err)
				}

				// Count statements
				validStmts := 0
				for _, rawStmt := range rawStatements {
					if strings.TrimSpace(rawStmt.Statement) != "" {
						validStmts++
					}
				}
				t.Logf("Generated %d valid SQL statements", validStmts)

			} else {
				// Should fail without explicit table name
				if err != nil {
					// Error during execution is expected
					t.Logf("Got expected error during execution: %v", err)
				} else {
					// If execution succeeded, formatting should fail
					var buf bytes.Buffer
					err = printTableData(session.systemVariables, 0, &buf, result)
					if err == nil {
						t.Errorf("Expected error without table name, but formatting succeeded")
						t.Logf("Unexpected output: %s", buf.String())
					} else {
						t.Logf("Got expected error during formatting: %v", err)
						// Check if error message is helpful
						errStr := err.Error()
						if !strings.Contains(errStr, "SQL export requires a table name") &&
							!strings.Contains(errStr, "Auto-detection failed") &&
							!strings.Contains(errStr, "CLI_SQL_TABLE_NAME") {
							t.Logf("Warning: Error message could be more helpful: %v", err)
						}
					}
				}
			}
		})
	}
}

func TestSQLExportAutoDetectionWithComplexQueries(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
	defer cancel()

	// Create test tables
	ddls := []string{
		`CREATE TABLE Users (
			id INT64 NOT NULL,
			name STRING(100),
			age INT64,
		) PRIMARY KEY (id)`,
		`CREATE TABLE Orders (
			id INT64 NOT NULL,
			user_id INT64,
			amount FLOAT64,
		) PRIMARY KEY (id)`,
	}

	// Insert test data
	insertDMLs := []string{
		`INSERT INTO Users (id, name, age) VALUES (1, 'Alice', 30), (2, 'Bob', 25)`,
		`INSERT INTO Orders (id, user_id, amount) VALUES (1, 1, 100.0), (2, 1, 200.0), (3, 2, 150.0)`,
	}

	_, session, teardown := initialize(t, ddls, insertDMLs)
	defer teardown()

	// Test cases that should NOT auto-detect
	complexQueries := []struct {
		name  string
		query string
		desc  string
	}{
		{
			name:  "UNION query",
			query: "SELECT * FROM Users WHERE age > 25 UNION ALL SELECT * FROM Users WHERE age <= 25",
			desc:  "UNION queries are too complex for auto-detection",
		},
		{
			name:  "CTE with SELECT *",
			query: "WITH young_users AS (SELECT * FROM Users WHERE age < 30) SELECT * FROM young_users",
			desc:  "CTE queries are not supported",
		},
		{
			name:  "JOIN with SELECT *",
			query: "SELECT * FROM Users u JOIN Orders o ON u.id = o.user_id",
			desc:  "JOIN queries are too complex",
		},
		{
			name:  "Subquery in FROM",
			query: "SELECT * FROM (SELECT * FROM Users WHERE age > 25) AS filtered_users",
			desc:  "Subqueries are not supported",
		},
		{
			name:  "SELECT with expressions",
			query: "SELECT id, name, age * 2 AS double_age FROM Users",
			desc:  "Queries with expressions need explicit table names",
		},
	}

	for _, tc := range complexQueries {
		t.Run(tc.name, func(t *testing.T) {
			// Set up for SQL export without table name
			session.systemVariables.CLIFormat = enums.DisplayModeSQLInsert
			session.systemVariables.SQLTableName = "" // Empty for auto-detection attempt
			session.systemVariables.SQLBatchSize = 0
			session.systemVariables.StreamingMode = enums.StreamingModeFalse

			t.Logf("Testing: %s", tc.desc)
			t.Logf("Query: %s", tc.query)

			stmt, err := BuildStatement(tc.query)
			if err != nil {
				t.Fatalf("Failed to build statement: %v", err)
			}

			result, err := stmt.Execute(ctx, session)
			if err != nil {
				// Error during execution is acceptable
				t.Logf("Error during execution (expected): %v", err)
			} else {
				// If execution succeeded, formatting should fail
				var buf bytes.Buffer
				err = printTableData(session.systemVariables, 0, &buf, result)
				if err == nil {
					t.Errorf("Expected error for complex query without table name, but succeeded")
					t.Logf("Unexpected output: %s", buf.String())
				} else {
					t.Logf("Got expected error: %v", err)
					// Verify error message mentions the need for explicit table name
					if !strings.Contains(err.Error(), "CLI_SQL_TABLE_NAME") {
						t.Logf("Error message should mention CLI_SQL_TABLE_NAME")
					}
				}
			}

			// Now test with explicit table name - should work
			session.systemVariables.SQLTableName = "ExportTable"

			// Re-execute with explicit table name
			result, err = stmt.Execute(ctx, session)
			if err != nil {
				t.Logf("Note: Query failed even with explicit table name: %v", err)
				// Some queries like JOINs may still fail for other reasons
			} else {
				var buf bytes.Buffer
				err = printTableData(session.systemVariables, 0, &buf, result)
				if err != nil {
					t.Logf("Note: Formatting failed even with explicit table name: %v", err)
				} else {
					sqlOutput := buf.String()
					if strings.Contains(sqlOutput, "INSERT INTO ExportTable") {
						t.Logf("Success with explicit table name: Generated %d bytes of SQL", len(sqlOutput))
					} else {
						t.Errorf("Expected INSERT INTO ExportTable in output")
					}
				}
			}
		})
	}
}
