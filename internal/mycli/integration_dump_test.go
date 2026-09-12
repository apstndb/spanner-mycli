package mycli

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"

	"cloud.google.com/go/spanner"
	"github.com/apstndb/gsqlutils"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanner-mycli/internal/mycli/streamio"
	"github.com/google/go-cmp/cmp"
)

func dumpRenderedOutputForTest(t *testing.T, result *Result) string {
	t.Helper()

	output, ok := result.Body.PreparedBytes()
	if !ok {
		t.Fatalf("expected DUMP fallback to return a prepared body")
	}
	if _, ok := result.Body.PresentationRows(); ok {
		t.Fatalf("expected DUMP fallback to return no presentation table")
	}
	return string(output)
}

func TestDumpStatements(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	ctx := context.Background()

	_, session := initializeWithRandomDB(t, nil, nil)

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
			stmt:               &DumpTablesStatement{Tables: []tableID{tid("Albums"), tid("Singers")}},
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
			outputStr := dumpRenderedOutputForTest(t, result)

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
			if session.systemVariables.Display.CLIFormat == enums.DisplayModeSQLInsert {
				t.Errorf("CLIFormat should be restored after DUMP")
			}
			if session.systemVariables.Display.SuppressResultLines {
				t.Errorf("SuppressResultLines should be restored after DUMP")
			}
		})
	}
}

func TestDumpTablesWithInvalidTable(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	ctx := context.Background()

	_, session := initializeWithRandomDB(t, nil, nil)

	stmt := &DumpTablesStatement{Tables: []tableID{tid("NonExistentTable")}}
	_, err := stmt.Execute(ctx, session)
	if err == nil {
		t.Fatalf("Expected error for non-existent table")
	}
	if !strings.Contains(err.Error(), "NonExistentTable") {
		t.Errorf("Error should mention the non-existent table: %v", err)
	}
}

func TestDumpEmptyDatabase(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	ctx := context.Background()

	_, session := initializeWithRandomDB(t, nil, nil)

	stmt := &DumpDatabaseStatement{}
	result, err := stmt.Execute(ctx, session)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	outputStr := dumpRenderedOutputForTest(t, result)
	if outputStr == "" {
		t.Errorf("Expected at least header comment in output")
	}

	if !strings.Contains(outputStr, "-- Database DDL exported by spanner-mycli") {
		t.Errorf("Expected DDL header comment")
	}
}

func TestDumpWithStreaming(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	ctx := context.Background()

	_, session := initializeWithRandomDB(t, nil, nil)

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
	session.systemVariables.StreamManager = streamio.NewStreamManager(
		originalStream.GetInStream(),
		&buf, // Use our buffer as output
		originalStream.GetErrStream(),
	)

	dumpStmt := &DumpTablesStatement{Tables: []tableID{tid("StreamTest")}}
	result, err := dumpStmt.Execute(ctx, session)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.alreadyDelivered() {
		t.Errorf("Expected Streamed to be true")
	}

	// Check the captured output
	output := buf.String()
	if !strings.Contains(output, "-- Data for table StreamTest") {
		t.Errorf("Expected table comment in output")
	}

	// Count INSERT statements
	insertCount := strings.Count(output, "INSERT INTO `StreamTest`")
	if insertCount < 5 {
		t.Errorf("Expected at least 5 INSERT statements, got %d\nOutput:\n%s", insertCount, output)
	}
}

func TestDumpWithForeignKeys(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	ctx := context.Background()

	_, session := initializeWithRandomDB(t, nil, nil)

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
	dumpStmt := &DumpTablesStatement{Tables: []tableID{tid("Concerts"), tid("Venues"), tid("Artists")}}
	result, err := dumpStmt.Execute(ctx, session)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	outputStr := dumpRenderedOutputForTest(t, result)

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
	if strings.Count(outputStr, "INSERT INTO `Venues`") < 2 {
		t.Errorf("Expected at least 2 INSERT statements for Venues")
	}
	if strings.Count(outputStr, "INSERT INTO `Artists`") < 2 {
		t.Errorf("Expected at least 2 INSERT statements for Artists")
	}
	if strings.Count(outputStr, "INSERT INTO `Concerts`") < 2 {
		t.Errorf("Expected at least 2 INSERT statements for Concerts")
	}
}

func TestDumpWithMixedDependencies(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	ctx := context.Background()

	_, session := initializeWithRandomDB(t, nil, nil)

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

	outputStr := dumpRenderedOutputForTest(t, result)

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

func TestDumpWithGeneratedColumns(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	ctx := context.Background()

	_, session := initializeWithRandomDB(t, nil, nil)

	// Create test table with generated columns and reserved word column names
	ddl := `CREATE TABLE Users (
		UserId INT64 NOT NULL,
		FirstName STRING(100),
		LastName STRING(100),
		FullName STRING(201) AS (CONCAT(FirstName, ' ', LastName)) STORED,
		` + "`Order`" + ` INT64,
		SearchTokens TOKENLIST AS (TOKENIZE_FULLTEXT(FullName)) HIDDEN,
		ComputedValue INT64 AS (UserId * 2),
		CreatedAt TIMESTAMP NOT NULL
	) PRIMARY KEY (UserId)`

	stmt, err := BuildStatement(ddl)
	if err != nil {
		t.Fatalf("Failed to build DDL statement: %v", err)
	}
	if _, err := stmt.Execute(ctx, session); err != nil {
		t.Fatalf("Failed to create test table: %v", err)
	}

	// Insert test data with fixed timestamp (only writable columns)
	insertSQL := "INSERT INTO Users (UserId, FirstName, LastName, `Order`, CreatedAt) VALUES (1, 'John', 'Doe', 10, TIMESTAMP '2024-01-01T12:00:00Z')"
	insertStmt, err := BuildStatement(insertSQL)
	if err != nil {
		t.Fatalf("Failed to build INSERT statement: %v", err)
	}
	if _, err := insertStmt.Execute(ctx, session); err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	// Execute DUMP TABLES
	dumpStmt := &DumpTablesStatement{Tables: []tableID{tid("Users")}}
	result, err := dumpStmt.Execute(ctx, session)
	if err != nil {
		t.Fatalf("DUMP TABLES failed: %v", err)
	}

	// Expected output with only writable columns
	// The generated INSERT should include: UserId, FirstName, LastName, Order, CreatedAt
	// It should NOT include: FullName, SearchTokens, ComputedValue (all generated columns)
	expectedOutput := strings.Join([]string{
		"-- Data for table Users",
		"INSERT INTO `Users` (`UserId`, `FirstName`, `LastName`, `Order`, `CreatedAt`) VALUES (1, \"John\", \"Doe\", 10, TIMESTAMP \"2024-01-01T12:00:00Z\");",
		"",
	}, "\n") + "\n"

	if diff := cmp.Diff(expectedOutput, dumpRenderedOutputForTest(t, result)); diff != "" {
		t.Errorf("DUMP output mismatch (-want +got):\n%s", diff)
	}

	streamedResult, streamedOutput, err := executeSQLExportForTest(t, ctx, session, "DUMP TABLES Users")
	if err != nil {
		t.Fatalf("streamed DUMP TABLES failed: %v", err)
	}
	if !streamedResult.alreadyDelivered() {
		t.Fatal("expected streamed DUMP")
	}
	if diff := cmp.Diff(expectedOutput, streamedOutput); diff != "" {
		t.Errorf("streamed DUMP output mismatch (-want +got):\n%s", diff)
	}
}

func TestDumpFloat32NegativeZeroReplay(t *testing.T) {
	t.Parallel()
	skipIfShortIntegration(t)
	const castText = "CAST(-0 AS FLOAT32)"
	const ddl = `CREATE TABLE FloatValues (
		Id INT64 NOT NULL, V FLOAT32, A ARRAY<FLOAT32>, EmptyValues ARRAY<FLOAT32>,
		NullValues ARRAY<FLOAT32>, NullValue FLOAT32, D FLOAT64,
		TextValue STRING(MAX), JsonValue JSON, BytesValue BYTES(MAX)
	) PRIMARY KEY(Id)`
	execute := func(t *testing.T, session *Session, sql string) *Result {
		t.Helper()
		stmt, err := BuildStatement(sql)
		if err != nil {
			t.Fatal(err)
		}
		result, err := stmt.Execute(t.Context(), session)
		if err != nil {
			t.Fatalf("execute %s: %v", sql, err)
		}
		return result
	}
	check := func(t *testing.T, session *Session) {
		t.Helper()
		iter := session.client.Single().Query(t.Context(), spanner.Statement{
			SQL: "SELECT V,A,EmptyValues,NullValues,NullValue,D,TextValue,JsonValue,BytesValue FROM FloatValues WHERE Id=1",
		})
		defer iter.Stop()
		row, err := iter.Next()
		if err != nil {
			t.Fatal(err)
		}
		var scalar, nullScalar spanner.NullFloat32
		var values []spanner.NullFloat32
		var emptyArray, nullArray spanner.GenericColumnValue
		var double spanner.NullFloat64
		var text string
		var json spanner.NullJSON
		var bytes []byte
		if err := row.Columns(&scalar, &values, &emptyArray, &nullArray, &nullScalar, &double, &text, &json, &bytes); err != nil {
			t.Fatal(err)
		}
		if !scalar.Valid || math.Float32bits(scalar.Float32) != math.Float32bits(float32(math.Copysign(0, -1))) {
			t.Errorf("FLOAT32 scalar negative zero changed: %v", scalar)
		}
		if len(values) != 7 {
			t.Fatalf("array length = %d, want 7", len(values))
		}
		wantFloats := []float32{float32(math.Copysign(0, -1)), 0, 1.5, float32(math.NaN()), float32(math.Inf(1)), float32(math.Inf(-1))}
		for i, want := range wantFloats {
			got := values[i]
			if !got.Valid || !(math.IsNaN(float64(want)) && math.IsNaN(float64(got.Float32))) && math.Float32bits(got.Float32) != math.Float32bits(want) {
				t.Errorf("FLOAT32 array[%d] = %v, want %v with sign bit preserved", i, got, want)
			}
		}
		if values[6].Valid || nullScalar.Valid || emptyArray.Value.GetListValue() == nil || len(emptyArray.Value.GetListValue().Values) != 0 || nullArray.Value.GetListValue() != nil {
			t.Error("NULL or empty-array shape changed")
		}
		if !double.Valid || math.Float64bits(double.Float64) != math.Float64bits(math.Copysign(0, -1)) {
			t.Errorf("FLOAT64 negative-zero control changed: %v", double)
		}
		if text != castText || string(bytes) != castText || !json.Valid {
			t.Errorf("literal-text controls changed: string=%q bytes=%q JSON=%v", text, bytes, json)
		}
		if diff := cmp.Diff(any(map[string]any{"text": castText}), json.Value); diff != "" {
			t.Errorf("JSON control changed (-want +got):\n%s", diff)
		}
	}
	for _, mode := range []string{"buffered", "streamed"} {
		t.Run(mode, func(t *testing.T) {
			_, source := initializeWithRandomDB(t, nil, nil)
			_, target := initializeWithRandomDB(t, nil, nil)
			execute(t, source, ddl)
			execute(t, target, ddl)
			execute(t, source, `INSERT INTO FloatValues
				(Id,V,A,EmptyValues,NullValues,NullValue,D,TextValue,JsonValue,BytesValue) VALUES
				(1, CAST('-0' AS FLOAT32),
				[CAST('-0' AS FLOAT32),CAST(0 AS FLOAT32),CAST(1.5 AS FLOAT32),CAST('nan' AS FLOAT32),CAST('inf' AS FLOAT32),CAST('-inf' AS FLOAT32),NULL],
				[],NULL,NULL,CAST('-0' AS FLOAT64),'CAST(-0 AS FLOAT32)',JSON '{"text":"CAST(-0 AS FLOAT32)"}',b'CAST(-0 AS FLOAT32)')`)
			check(t, source)
			if t.Failed() {
				t.Fatal("invalid source fixture")
			}
			var output string
			if mode == "buffered" {
				output = dumpRenderedOutputForTest(t, execute(t, source, "DUMP TABLES FloatValues"))
			} else {
				result, text, err := executeSQLExportForTest(t, t.Context(), source, "DUMP TABLES FloatValues")
				if err != nil {
					t.Fatal(err)
				}
				if !result.alreadyDelivered() {
					t.Fatal("expected streamed DUMP")
				}
				output = text
			}
			statements, err := gsqlutils.SeparateInputPreserveCommentsWithStatus("", output)
			if err != nil {
				t.Fatal(err)
			}
			for _, raw := range statements {
				if sql := strings.TrimSpace(raw.Statement); sql != "" {
					execute(t, target, sql)
				}
			}
			check(t, target)
		})
	}
}
