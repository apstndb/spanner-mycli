//go:build !short

package main

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/apstndb/spanner-mycli/enums"
)

func TestDumpStatements(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()
	ctx := context.Background()

	_, session, teardown := initialize(t, nil, nil)
	defer teardown()

	// Create test tables with INTERLEAVE relationship
	setupDDL := []string{
		`CREATE TABLE Singers (
			SingerId INT64 NOT NULL,
			FirstName STRING(1024),
			LastName STRING(1024),
		) PRIMARY KEY (SingerId)`,
		`CREATE TABLE Albums (
			SingerId INT64 NOT NULL,
			AlbumId INT64 NOT NULL,
			AlbumTitle STRING(MAX),
		) PRIMARY KEY (SingerId, AlbumId),
		  INTERLEAVE IN PARENT Singers ON DELETE CASCADE`,
		`CREATE TABLE Songs (
			SingerId INT64 NOT NULL,
			AlbumId INT64 NOT NULL,
			SongId INT64 NOT NULL,
			SongTitle STRING(MAX),
		) PRIMARY KEY (SingerId, AlbumId, SongId),
		  INTERLEAVE IN PARENT Albums ON DELETE CASCADE`,
	}

	for _, ddl := range setupDDL {
		stmt, err := BuildStatement(ddl)
		if err != nil { t.Fatalf("Failed to build DDL statement: %v", err) }
		if _, err := stmt.Execute(ctx, session); err != nil { t.Fatalf("Failed to create test table: %v", err) }
	}

	// Insert test data
	insertStmts := []string{
		`INSERT INTO Singers (SingerId, FirstName, LastName) VALUES (1, 'Marc', 'Richards')`,
		`INSERT INTO Singers (SingerId, FirstName, LastName) VALUES (2, 'Catalina', 'Smith')`,
		`INSERT INTO Albums (SingerId, AlbumId, AlbumTitle) VALUES (1, 1, 'Total Junk')`,
		`INSERT INTO Albums (SingerId, AlbumId, AlbumTitle) VALUES (1, 2, 'Go Go Go')`,
		`INSERT INTO Albums (SingerId, AlbumId, AlbumTitle) VALUES (2, 1, 'Green')`,
		`INSERT INTO Songs (SingerId, AlbumId, SongId, SongTitle) VALUES (1, 1, 1, 'Track 1')`,
		`INSERT INTO Songs (SingerId, AlbumId, SongId, SongTitle) VALUES (1, 1, 2, 'Track 2')`,
	}

	for _, sql := range insertStmts {
		stmt, err := BuildStatement(sql)
		if err != nil { t.Fatalf("Failed to build DML statement: %v", err) }
		if _, err := stmt.Execute(ctx, session); err != nil { t.Fatalf("Failed to insert test data: %v", err) }
	}

	tests := []struct {
		name               string
		stmt               Statement
		expectDDL          bool
		expectTables       []string // Expected tables in order
		expectInsertCount  int      // Minimum number of INSERT statements expected
		expectNoResultLine bool     // Should suppress result lines
	}{
		{
			name:               "DUMP DATABASE",
			stmt:               &DumpDatabaseStatement{},
			expectDDL:          true,
			expectTables:       []string{"Singers", "Albums", "Songs"}, // Parent before children
			expectInsertCount:  7,                                      // 2 singers + 3 albums + 2 songs
			expectNoResultLine: true,
		},
		{
			name:               "DUMP SCHEMA",
			stmt:               &DumpSchemaStatement{},
			expectDDL:          true,
			expectTables:       []string{}, // No data expected
			expectInsertCount:  0,
			expectNoResultLine: true,
		},
		{
			name:               "DUMP TABLES specific",
			stmt:               &DumpTablesStatement{Tables: []string{"Albums", "Singers"}},
			expectDDL:          false,
			expectTables:       []string{"Singers", "Albums"}, // Should be reordered by dependency
			expectInsertCount:  5,                             // 2 singers + 3 albums
			expectNoResultLine: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.stmt.Execute(ctx, session)
			if err != nil { t.Fatalf("Execute failed: %v", err) }
			if !result.IsDirectOutput { t.Errorf("Expected IsDirectOutput to be true") }

			// Convert rows to string for analysis
			var output strings.Builder
			for _, row := range result.Rows {
				if len(row) > 0 {
					output.WriteString(row[0])
					output.WriteString("\n")
				}
			}
			outputStr := output.String()

			// Check for DDL presence
			if tt.expectDDL {
				if !strings.Contains(outputStr, "CREATE TABLE") {
					t.Errorf("Expected DDL statements in output")
				}
				if !strings.Contains(outputStr, "-- Database DDL exported by spanner-mycli") {
					t.Errorf("Expected DDL header comment")
				}
			} else {
				if strings.Contains(outputStr, "CREATE TABLE") {
					t.Errorf("Unexpected DDL statements in output")
				}
			}

			// Check table order in data export
			if len(tt.expectTables) > 0 {
				var lastIndex int
				for _, table := range tt.expectTables {
					comment := "-- Data for table " + table
					index := strings.Index(outputStr, comment)
					if index == -1 {
						t.Errorf("Expected table %s in output", table)
					} else if index < lastIndex {
						t.Errorf("Table %s appears out of order (dependency violation)", table)
					}
					lastIndex = index
				}
			}

			// Count INSERT statements
			insertCount := strings.Count(outputStr, "INSERT INTO")
			if insertCount < tt.expectInsertCount {
				t.Errorf("Expected at least %d INSERT statements, got %d\nOutput:\n%s", tt.expectInsertCount, insertCount, outputStr)
			}

			// Verify settings were restored
			if session.systemVariables.CLIFormat == enums.DisplayModeSQLInsert {
				t.Errorf("CLIFormat should be restored after DUMP")
			}
			if session.systemVariables.SuppressResultLines {
				t.Errorf("SuppressResultLines should be restored after DUMP")
			}
		})
	}
}

func TestDumpTablesWithInvalidTable(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()
	ctx := context.Background()

	_, session, teardown := initialize(t, nil, nil)
	defer teardown()

	stmt := &DumpTablesStatement{Tables: []string{"NonExistentTable"}}
	_, err := stmt.Execute(ctx, session)
	if err == nil { t.Fatalf("Expected error for non-existent table") }
	if !strings.Contains(err.Error(), "NonExistentTable") { t.Errorf("Error should mention the non-existent table: %v", err) }
}

func TestDumpEmptyDatabase(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()
	ctx := context.Background()

	_, session, teardown := initialize(t, nil, nil)
	defer teardown()

	stmt := &DumpDatabaseStatement{}
	result, err := stmt.Execute(ctx, session)
	if err != nil { t.Fatalf("Execute failed: %v", err) }
	if len(result.Rows) == 0 { t.Errorf("Expected at least header comment in output") }

	hasHeader := false
	for _, row := range result.Rows {
		if len(row) > 0 && strings.Contains(row[0], "-- Database DDL exported by spanner-mycli") {
			hasHeader = true
			break
		}
	}
	if !hasHeader { t.Errorf("Expected DDL header comment") }
}

func TestDumpWithStreaming(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()
	ctx := context.Background()

	_, session, teardown := initialize(t, nil, nil)
	defer teardown()

	// Create test table with data
	ddl := `CREATE TABLE StreamTest (
		id INT64 NOT NULL,
		value STRING(100),
	) PRIMARY KEY (id)`

	stmt, err := BuildStatement(ddl)
	if err != nil { t.Fatalf("Failed to build DDL statement: %v", err) }
	if _, err := stmt.Execute(ctx, session); err != nil { t.Fatalf("Failed to create test table: %v", err) }

	// Insert test data
	for i := 1; i <= 5; i++ {
		stmt, err := BuildStatement(fmt.Sprintf("INSERT INTO StreamTest (id, value) VALUES (%d, 'value%d')", i, i))
		if err != nil { t.Fatalf("Failed to build DML statement: %v", err) }
		if _, err := stmt.Execute(ctx, session); err != nil { t.Fatalf("Failed to insert test data: %v", err) }
	}

	// Create a buffer to capture streaming output
	var buf strings.Builder

	// Replace the session's output stream with our buffer
	// This simulates streaming mode with captured output
	originalStream := session.systemVariables.StreamManager
	session.systemVariables.StreamManager = NewStreamManager(
		originalStream.GetInStream(),
		&buf, // Use our buffer as output
		originalStream.GetErrStream(),
	)

	dumpStmt := &DumpTablesStatement{Tables: []string{"StreamTest"}}
	result, err := dumpStmt.Execute(ctx, session)
	if err != nil { t.Fatalf("Execute failed: %v", err) }
	if !result.Streamed { t.Errorf("Expected Streamed to be true") }

	// Check the captured output
	output := buf.String()
	if !strings.Contains(output, "-- Data for table StreamTest") {
		t.Errorf("Expected table comment in output")
	}

	// Count INSERT statements
	insertCount := strings.Count(output, "INSERT INTO StreamTest")
	if insertCount < 5 {
		t.Errorf("Expected at least 5 INSERT statements, got %d\nOutput:\n%s", insertCount, output)
	}
}
