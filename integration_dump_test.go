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
		if err != nil {
			t.Fatalf("Failed to build DDL statement: %v", err)
		}
		if _, err := stmt.Execute(ctx, session); err != nil {
			t.Fatalf("Failed to create test table: %v", err)
		}
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
		if err != nil {
			t.Fatalf("Failed to build DML statement: %v", err)
		}
		if _, err := stmt.Execute(ctx, session); err != nil {
			t.Fatalf("Failed to insert test data: %v", err)
		}
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
			if err != nil {
				t.Fatalf("Execute failed: %v", err)
			}
			if !result.IsDirectOutput {
				t.Errorf("Expected IsDirectOutput to be true")
			}

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
	if err == nil {
		t.Fatalf("Expected error for non-existent table")
	}
	if !strings.Contains(err.Error(), "NonExistentTable") {
		t.Errorf("Error should mention the non-existent table: %v", err)
	}
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
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(result.Rows) == 0 {
		t.Errorf("Expected at least header comment in output")
	}

	hasHeader := false
	for _, row := range result.Rows {
		if len(row) > 0 && strings.Contains(row[0], "-- Database DDL exported by spanner-mycli") {
			hasHeader = true
			break
		}
	}
	if !hasHeader {
		t.Errorf("Expected DDL header comment")
	}
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
	if err != nil {
		t.Fatalf("Failed to build DDL statement: %v", err)
	}
	if _, err := stmt.Execute(ctx, session); err != nil {
		t.Fatalf("Failed to create test table: %v", err)
	}

	// Insert test data
	for i := 1; i <= 5; i++ {
		stmt, err := BuildStatement(fmt.Sprintf("INSERT INTO StreamTest (id, value) VALUES (%d, 'value%d')", i, i))
		if err != nil {
			t.Fatalf("Failed to build DML statement: %v", err)
		}
		if _, err := stmt.Execute(ctx, session); err != nil {
			t.Fatalf("Failed to insert test data: %v", err)
		}
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
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.Streamed {
		t.Errorf("Expected Streamed to be true")
	}

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

func TestDumpWithForeignKeys(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()
	ctx := context.Background()

	_, session, teardown := initialize(t, nil, nil)
	defer teardown()

	// Create tables with foreign key relationships
	setupDDL := []string{
		`CREATE TABLE Venues (
			VenueId INT64 NOT NULL,
			VenueName STRING(100),
		) PRIMARY KEY (VenueId)`,
		`CREATE TABLE Artists (
			ArtistId INT64 NOT NULL,
			ArtistName STRING(100),
		) PRIMARY KEY (ArtistId)`,
		`CREATE TABLE Concerts (
			ConcertId INT64 NOT NULL,
			VenueId INT64 NOT NULL,
			ArtistId INT64 NOT NULL,
			ConcertDate DATE,
			CONSTRAINT FK_Venue FOREIGN KEY (VenueId) REFERENCES Venues (VenueId),
			CONSTRAINT FK_Artist FOREIGN KEY (ArtistId) REFERENCES Artists (ArtistId),
		) PRIMARY KEY (ConcertId)`,
	}

	for _, ddl := range setupDDL {
		stmt, err := BuildStatement(ddl)
		if err != nil {
			t.Fatalf("Failed to build DDL statement: %v", err)
		}
		if _, err := stmt.Execute(ctx, session); err != nil {
			t.Fatalf("Failed to create test table: %v", err)
		}
	}

	// Insert test data
	insertStmts := []string{
		`INSERT INTO Venues (VenueId, VenueName) VALUES (1, 'Madison Square Garden')`,
		`INSERT INTO Venues (VenueId, VenueName) VALUES (2, 'Hollywood Bowl')`,
		`INSERT INTO Artists (ArtistId, ArtistName) VALUES (1, 'The Beatles')`,
		`INSERT INTO Artists (ArtistId, ArtistName) VALUES (2, 'Rolling Stones')`,
		`INSERT INTO Concerts (ConcertId, VenueId, ArtistId, ConcertDate) VALUES (1, 1, 1, '2024-01-15')`,
		`INSERT INTO Concerts (ConcertId, VenueId, ArtistId, ConcertDate) VALUES (2, 2, 2, '2024-02-20')`,
	}

	for _, sql := range insertStmts {
		stmt, err := BuildStatement(sql)
		if err != nil {
			t.Fatalf("Failed to build DML statement: %v", err)
		}
		if _, err := stmt.Execute(ctx, session); err != nil {
			t.Fatalf("Failed to insert test data: %v", err)
		}
	}

	// Test DUMP TABLES with FK dependencies
	dumpStmt := &DumpTablesStatement{Tables: []string{"Concerts", "Venues", "Artists"}}
	result, err := dumpStmt.Execute(ctx, session)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Convert rows to string for analysis
	var output strings.Builder
	for _, row := range result.Rows {
		if len(row) > 0 {
			output.WriteString(row[0])
			output.WriteString("\n")
		}
	}
	outputStr := output.String()

	// Check table order - FK referenced tables should come before referencing tables
	venueIndex := strings.Index(outputStr, "-- Data for table Venues")
	artistIndex := strings.Index(outputStr, "-- Data for table Artists")
	concertIndex := strings.Index(outputStr, "-- Data for table Concerts")

	if venueIndex == -1 || artistIndex == -1 || concertIndex == -1 {
		t.Errorf("Expected all three tables in output")
	}

	// Concerts should come after both Venues and Artists due to FK constraints
	if concertIndex < venueIndex {
		t.Errorf("Concerts should appear after Venues (FK dependency)")
	}
	if concertIndex < artistIndex {
		t.Errorf("Concerts should appear after Artists (FK dependency)")
	}

	// Check INSERT statements
	if strings.Count(outputStr, "INSERT INTO Venues") < 2 {
		t.Errorf("Expected at least 2 INSERT statements for Venues")
	}
	if strings.Count(outputStr, "INSERT INTO Artists") < 2 {
		t.Errorf("Expected at least 2 INSERT statements for Artists")
	}
	if strings.Count(outputStr, "INSERT INTO Concerts") < 2 {
		t.Errorf("Expected at least 2 INSERT statements for Concerts")
	}
}

func TestDumpWithMixedDependencies(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()
	ctx := context.Background()

	_, session, teardown := initialize(t, nil, nil)
	defer teardown()

	// Create tables with both INTERLEAVE and FK relationships
	setupDDL := []string{
		`CREATE TABLE Categories (
			CategoryId INT64 NOT NULL,
			CategoryName STRING(100),
		) PRIMARY KEY (CategoryId)`,
		`CREATE TABLE Products (
			ProductId INT64 NOT NULL,
			ProductName STRING(100),
			CategoryId INT64,
			CONSTRAINT FK_Category FOREIGN KEY (CategoryId) REFERENCES Categories (CategoryId),
		) PRIMARY KEY (ProductId)`,
		`CREATE TABLE Customers (
			CustomerId INT64 NOT NULL,
			CustomerName STRING(100),
		) PRIMARY KEY (CustomerId)`,
		`CREATE TABLE Orders (
			CustomerId INT64 NOT NULL,
			OrderId INT64 NOT NULL,
			OrderDate DATE,
		) PRIMARY KEY (CustomerId, OrderId),
		  INTERLEAVE IN PARENT Customers ON DELETE CASCADE`,
		`CREATE TABLE OrderItems (
			CustomerId INT64 NOT NULL,
			OrderId INT64 NOT NULL,
			ItemId INT64 NOT NULL,
			ProductId INT64 NOT NULL,
			Quantity INT64,
			CONSTRAINT FK_Product FOREIGN KEY (ProductId) REFERENCES Products (ProductId),
		) PRIMARY KEY (CustomerId, OrderId, ItemId),
		  INTERLEAVE IN PARENT Orders ON DELETE CASCADE`,
	}

	for _, ddl := range setupDDL {
		stmt, err := BuildStatement(ddl)
		if err != nil {
			t.Fatalf("Failed to build DDL statement: %v", err)
		}
		if _, err := stmt.Execute(ctx, session); err != nil {
			t.Fatalf("Failed to create test table: %v", err)
		}
	}

	// Insert test data
	insertStmts := []string{
		`INSERT INTO Categories (CategoryId, CategoryName) VALUES (1, 'Electronics')`,
		`INSERT INTO Products (ProductId, ProductName, CategoryId) VALUES (1, 'Laptop', 1)`,
		`INSERT INTO Customers (CustomerId, CustomerName) VALUES (1, 'Alice')`,
		`INSERT INTO Orders (CustomerId, OrderId, OrderDate) VALUES (1, 1, '2024-01-01')`,
		`INSERT INTO OrderItems (CustomerId, OrderId, ItemId, ProductId, Quantity) VALUES (1, 1, 1, 1, 2)`,
	}

	for _, sql := range insertStmts {
		stmt, err := BuildStatement(sql)
		if err != nil {
			t.Fatalf("Failed to build DML statement: %v", err)
		}
		if _, err := stmt.Execute(ctx, session); err != nil {
			t.Fatalf("Failed to insert test data: %v", err)
		}
	}

	// Test DUMP DATABASE with mixed dependencies
	dumpStmt := &DumpDatabaseStatement{}
	result, err := dumpStmt.Execute(ctx, session)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Convert rows to string for analysis
	var output strings.Builder
	for _, row := range result.Rows {
		if len(row) > 0 {
			output.WriteString(row[0])
			output.WriteString("\n")
		}
	}
	outputStr := output.String()

	// Check table order
	categoryIndex := strings.Index(outputStr, "-- Data for table Categories")
	productIndex := strings.Index(outputStr, "-- Data for table Products")
	customerIndex := strings.Index(outputStr, "-- Data for table Customers")
	orderIndex := strings.Index(outputStr, "-- Data for table Orders")
	orderItemIndex := strings.Index(outputStr, "-- Data for table OrderItems")

	// FK dependencies: Categories before Products
	if categoryIndex > productIndex && productIndex != -1 {
		t.Errorf("Categories should appear before Products (FK dependency)")
	}

	// INTERLEAVE dependencies: Customers before Orders before OrderItems
	if customerIndex > orderIndex && orderIndex != -1 {
		t.Errorf("Customers should appear before Orders (INTERLEAVE dependency)")
	}
	if orderIndex > orderItemIndex && orderItemIndex != -1 {
		t.Errorf("Orders should appear before OrderItems (INTERLEAVE dependency)")
	}

	// Mixed dependency: Products before OrderItems (FK from OrderItems to Products)
	if productIndex > orderItemIndex && orderItemIndex != -1 {
		t.Errorf("Products should appear before OrderItems (FK dependency)")
	}
}
