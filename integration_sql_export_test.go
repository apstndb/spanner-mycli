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
