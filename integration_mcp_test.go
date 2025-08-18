package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/cloudspannerecosystem/memefish/ast"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/samber/lo"
)

// setupMCPClientServer creates a complete MCP client-server setup for testing
func setupMCPClientServer(t *testing.T, ctx context.Context, session *Session) (*mcp.ClientSession, *mcp.Server, error) {
	t.Helper()
	// Create CLI instance
	var outputBuf strings.Builder

	// Update the session's StatementTimeout for integration tests
	session.systemVariables.StatementTimeout = lo.ToPtr(1 * time.Hour)
	session.systemVariables.Verbose = true // Set Verbose to true to ensure result line is printed

	// Update the session's StreamManager to use the output buffer
	session.systemVariables.StreamManager = NewStreamManager(io.NopCloser(strings.NewReader("")), &outputBuf, &outputBuf)

	cli := &Cli{
		SessionHandler:  NewSessionHandler(session),
		SystemVariables: session.systemVariables, // Use the same systemVariables as the session
	}

	// Create MCP server using extracted function
	mcpServer := createMCPServer(cli)

	// Create in-memory transport for testing
	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	// Start server in a goroutine
	serverDone := make(chan error, 1)
	var serverSession *mcp.ServerSession
	go func() {
		var err error
		serverSession, err = mcpServer.Connect(ctx, serverTransport)
		if err != nil {
			serverDone <- err
			return
		}
		// Wait for the server session to complete
		serverDone <- serverSession.Wait()
	}()

	// Create and connect client
	mcpClient := mcp.NewClient(&mcp.Implementation{
		Name:    "spanner-mycli-test",
		Version: version,
	}, nil)

	// Connect client and get session
	clientSession, err := mcpClient.Connect(ctx, clientTransport)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect client: %w", err)
	}

	// Wait a moment for server to initialize
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case err := <-serverDone:
		return nil, nil, fmt.Errorf("server exited early: %w", err)
	case <-time.After(100 * time.Millisecond):
		// Give the server a moment to initialize
	}

	// Add cleanup function to ensure server goroutine exits
	t.Cleanup(func() {
		// Close the client session first (if not already closed)
		_ = clientSession.Close()
		// This should cause the server Wait() to return
		// Give it a moment to clean up
		select {
		case <-serverDone:
			// Server exited cleanly
		case <-time.After(1 * time.Second):
			t.Log("Warning: MCP server did not exit cleanly")
		}
	})

	return clientSession, mcpServer, nil
}

// testExecuteStatementTool tests the execute_statement tool functionality
func testExecuteStatementTool(t *testing.T, ctx context.Context, session *Session, statement string, wantOutput string, wantError bool) {
	t.Helper()
	// Skip tests that are not compatible with the emulator
	upperStatement := strings.ToUpper(statement)
	if strings.HasPrefix(upperStatement, "EXPLAIN") {
		t.Skip("EXPLAIN statement is not supported for Cloud Spanner Emulator")
	}
	// We no longer need to skip these tests since we're checking the result line
	// which should be consistent across different environments
	// if strings.HasPrefix(upperStatement, "CREATE TABLE") {
	// 	t.Skip("CREATE TABLE output format differs in emulator")
	// }
	// if strings.HasPrefix(upperStatement, "INSERT INTO") {
	// 	t.Skip("INSERT INTO output format differs in emulator")
	// }
	// if strings.HasPrefix(upperStatement, "SET CLI_") {
	// 	t.Skip("SET VARIABLE output format differs in emulator")
	// }

	// Setup MCP client and server
	mcpClient, _, err := setupMCPClientServer(t, ctx, session)
	if err != nil {
		t.Fatalf("Failed to setup MCP client-server: %v", err)
	}

	// Call the execute_statement tool using the MCP client
	t.Logf("Executing statement via MCP client: %q", statement)
	params := &mcp.CallToolParams{
		Name:      "execute_statement",
		Arguments: ExecuteStatementArgs{Statement: statement},
	}
	result, err := mcpClient.CallTool(ctx, params)
	// Handle errors
	if err != nil {
		if wantError {
			t.Logf("Got expected error: %v", err)
			return // Expected error
		}
		t.Fatalf("Failed to call execute_statement tool: %v", err)
	}

	// Extract the text content from the result
	gotOutput := ""
	if result != nil && len(result.Content) > 0 {
		for _, content := range result.Content {
			if textContent, ok := content.(*mcp.TextContent); ok {
				gotOutput = textContent.Text
				break
			}
		}
	}

	// For error cases, check if we got an error message in the output
	if wantError {
		t.Logf("Testing error case, got output: %q", gotOutput)
		// Check if the output contains error indicators or is empty
		// Empty output means the statement failed before producing any results
		if strings.Contains(gotOutput, "ERROR:") ||
			strings.Contains(gotOutput, "error:") ||
			strings.Contains(gotOutput, "unknown statement") ||
			strings.Contains(gotOutput, "syntax error") ||
			strings.Contains(gotOutput, "invalid") ||
			strings.Contains(gotOutput, "Invalid") ||
			len(gotOutput) == 0 {
			t.Logf("Got expected error in output")
			return
		}
		t.Errorf("Expected error but got successful output: %s", gotOutput)
		return
	}

	// Extract the first line of the result message (after the table output)
	// This is typically a line that starts with "Empty set", "N rows in set", or "Query OK"
	lines := strings.Split(gotOutput, "\n")
	var resultLine string
	for _, line := range lines {
		if strings.Contains(line, "Empty set") ||
			strings.Contains(line, "rows in set") ||
			strings.Contains(line, "Query OK") {
			resultLine = line
			break
		}
	}

	t.Logf("Result line: %q", resultLine)

	// If we found a result line, check if it contains the expected output
	if resultLine != "" && strings.Contains(resultLine, wantOutput) {
		return
	}

	// If we didn't find a result line or it doesn't contain the expected output,
	// fall back to checking the entire output
	if !strings.Contains(gotOutput, wantOutput) {
		// Print the full output for debugging
		t.Logf("Full output: %q", gotOutput)
		t.Errorf("Output should contain %q, got: %s", wantOutput, gotOutput)
	}

	// Verify output is not empty for valid statements
	compareResult(t, len(gotOutput) > 0, true)
}

// testDatabaseExistence tests the database existence check functionality
func testDatabaseExistence(t *testing.T, session *Session, shouldExist bool) {
	t.Helper()
	exists, err := session.DatabaseExists()
	if err != nil {
		t.Fatalf("DatabaseExists check failed: %v", err)
	}

	compareResult(t, exists, shouldExist)
}

// testRunMCPWithNonExistentDatabase tests RunMCP with a non-existent database
func testRunMCPWithNonExistentDatabase(t *testing.T) {
	t.Helper()
	ctx := t.Context()

	// Create system variables with non-existent database
	host, port, err := parseEndpoint(emulator.URI())
	if err != nil {
		t.Fatalf("Failed to parse emulator URI: %v", err)
	}
	sysVarsNonExistent := systemVariables{
		Project:               "test-project",
		Instance:              "test-instance",
		Database:              "non-existent-database",
		Params:                make(map[string]ast.Node),
		RPCPriority:           sppb.RequestOptions_PRIORITY_UNSPECIFIED,
		StatementTimeout:      lo.ToPtr(1 * time.Hour), // Long timeout for integration tests
		Host:                  host,
		Port:                  port,
		WithoutAuthentication: true,
	}

	sessionNonExistent, err := NewSession(ctx, &sysVarsNonExistent, defaultClientOptions(emulator)...)
	if err != nil {
		t.Fatalf("Failed to create session for non-existent database test: %v", err)
	}
	defer sessionNonExistent.Close()

	// Test database existence check
	testDatabaseExistence(t, sessionNonExistent, false)

	// Test that RunMCP returns error for non-existent database
	var outputBuf strings.Builder
	pipeReader, pipeWriter := io.Pipe()
	defer func() { _ = pipeReader.Close() }()
	defer func() { _ = pipeWriter.Close() }()

	// Create StreamManager with the pipe for input
	sysVarsNonExistent.StreamManager = NewStreamManager(pipeReader, &outputBuf, &outputBuf)

	cli, err := NewCli(ctx, nil, &sysVarsNonExistent)
	if err != nil {
		t.Fatalf("Failed to create CLI with non-existent database: %v", err)
	}
	defer cli.SessionHandler.Close()

	err = cli.RunMCP(ctx)
	if err == nil {
		t.Errorf("RunMCP should return error for non-existent database")
		return
	}

	// Check that an error was returned
	compareResult(t, err != nil, true)
}

// testMCPClientServerSetup tests the MCP client-server setup
func testMCPClientServerSetup(t *testing.T, ctx context.Context, session *Session) (*mcp.ClientSession, *mcp.Server) {
	t.Helper()
	mcpClient, mcpServer, err := setupMCPClientServer(t, ctx, session)
	if err != nil {
		t.Fatalf("Failed to setup MCP client-server: %v", err)
	}

	// Verify client and server are not nil
	if mcpClient == nil {
		t.Fatalf("MCP client is nil")
	}
	if mcpServer == nil {
		t.Fatalf("MCP server is nil")
	}

	return mcpClient, mcpServer
}

func TestRunMCP(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	tests := []struct {
		desc       string
		ddls, dmls []string
		statement  string
		wantOutput string
		wantError  bool
	}{
		// Basic functionality tests
		{
			desc:       "MCP execute_statement with HELP",
			statement:  "HELP",
			wantOutput: "Usage", // HELP output should contain usage information
		},
		{
			desc:       "MCP execute_statement with SHOW TABLES",
			ddls:       testTableDDLs,
			statement:  "SHOW TABLES",
			wantOutput: "tbl", // Should show the test table
		},
		{
			desc:       "MCP execute_statement with SELECT",
			ddls:       testTableDDLs,
			dmls:       sliceOf("INSERT INTO tbl (id, active) VALUES (1, true), (2, false)"),
			statement:  "SELECT id, active FROM tbl ORDER BY id",
			wantOutput: "1", // Should contain data from the table
		},
		{
			desc:       "MCP execute_statement with SELECT empty result",
			ddls:       testTableDDLs,
			statement:  "SELECT id, active FROM tbl WHERE id > 1000",
			wantOutput: "Empty set", // Should show Empty set for no results
		},
		{
			desc:       "MCP execute_statement with SHOW VARIABLES",
			statement:  "SHOW VARIABLES",
			wantOutput: "name", // Should show variable names
		},

		// Additional statement types
		{
			desc:       "MCP execute_statement with DML",
			ddls:       testTableDDLs,
			statement:  "INSERT INTO tbl (id, active) VALUES (3, true)",
			wantOutput: "1 row", // Should show rows affected
		},
		{
			desc:       "MCP execute_statement with DDL",
			statement:  "CREATE TABLE test_table (id INT64) PRIMARY KEY (id)",
			wantOutput: "OK", // Should show success
		},
		{
			desc:       "MCP execute_statement with EXPLAIN",
			ddls:       testTableDDLs,
			statement:  "EXPLAIN SELECT * FROM tbl",
			wantOutput: "Query Plan", // Should contain query plan
		},
		{
			desc:       "MCP execute_statement with SET VARIABLE",
			statement:  "SET CLI_AUTOWRAP = TRUE",
			wantOutput: "Query OK", // Output for SET VARIABLE statements (fixed in issue #414)
		},

		// Error cases
		{
			desc:      "MCP execute_statement with invalid SQL",
			statement: "INVALID SQL STATEMENT",
			wantError: true,
		},
		{
			desc:      "MCP execute_statement with syntax error",
			statement: "SELECT * FROM",
			wantError: true,
		},
		{
			desc:      "MCP execute_statement with non-existent table",
			statement: "SELECT * FROM non_existent_table",
			wantError: true,
		},
		{
			desc:      "MCP execute_statement with invalid column",
			ddls:      testTableDDLs,
			statement: "SELECT non_existent_column FROM tbl",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
			defer cancel()

			// Initialize database for each test case
			_, session, teardown := initializeWithRandomDB(t, tt.ddls, tt.dmls)
			defer teardown()

			// Test the execute_statement tool functionality
			testExecuteStatementTool(t, ctx, session, tt.statement, tt.wantOutput, tt.wantError)
		})
	}

	// Test database validation (the first step of RunMCP)
	t.Run("database exists validation", func(t *testing.T) {
		// Test with existing database
		_, session, teardown := initializeWithRandomDB(t, testTableDDLs, nil)
		defer teardown()

		testDatabaseExistence(t, session, true)
	})

	// Test MCP client-server setup
	t.Run("mcp client-server setup", func(t *testing.T) {
		_, session, teardown := initializeWithRandomDB(t, testTableDDLs, nil)
		defer teardown()

		ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
		defer cancel()

		client, server := testMCPClientServerSetup(t, ctx, session)
		// Just verify they're created successfully, no need to use them
		_ = client
		_ = server
	})

	// Test server creation with different CLI configurations
	t.Run("server creation with different CLI configurations", func(t *testing.T) {
		_, session, teardown := initializeWithRandomDB(t, testTableDDLs, nil)
		defer teardown()

		// Create CLI with different system variables (but make sure session has timeout too)
		session.systemVariables.StatementTimeout = lo.ToPtr(1 * time.Hour)

		var outputBuf strings.Builder
		// Create a new system variables with modified values
		modifiedSysVars := &systemVariables{
			Project:               session.systemVariables.Project,
			Instance:              session.systemVariables.Instance,
			Database:              session.systemVariables.Database,
			Params:                make(map[string]ast.Node),
			RPCPriority:           sppb.RequestOptions_PRIORITY_UNSPECIFIED,
			Host:                  session.systemVariables.Host,
			Port:                  session.systemVariables.Port,
			WithoutAuthentication: session.systemVariables.WithoutAuthentication,
			StatementTimeout:      lo.ToPtr(1 * time.Hour), // Long timeout for integration tests
			AutoWrap:              true,                    // Set a different value
			EnableHighlight:       true,                    // Set a different value
			StreamManager:         NewStreamManager(io.NopCloser(strings.NewReader("")), &outputBuf, &outputBuf),
		}
		cli := &Cli{
			SessionHandler:  NewSessionHandler(session),
			SystemVariables: modifiedSysVars,
		}

		// Create server with the modified CLI
		server := createMCPServer(cli)
		if server == nil {
			t.Fatalf("Failed to create MCP server with modified CLI")
		}
	})

	// Test non-existent database error case
	t.Run("database does not exist", func(t *testing.T) {
		testRunMCPWithNonExistentDatabase(t)
	})
}
