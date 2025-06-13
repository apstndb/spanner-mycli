//go:build !skip_slow_test

package main

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/cloudspannerecosystem/memefish/ast"
)

func TestRunMCP(t *testing.T) {
	tests := []struct {
		desc       string
		ddls, dmls []string
		statement  string
		wantOutput string
		wantError  bool
	}{
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
			desc:       "MCP execute_statement with SHOW VARIABLES",
			statement:  "SHOW VARIABLES",
			wantOutput: "name", // Should show variable names
		},
		{
			desc:      "MCP execute_statement with invalid SQL",
			statement: "INVALID SQL STATEMENT",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
			defer cancel()

			// Initialize database for each test case
			_, session, teardown := initialize(t, tt.ddls, tt.dmls)
			defer teardown()

			// Test the MCP execute_statement tool functionality directly
			// This tests the core logic inside the tool handler in RunMCP
			var outputBuf strings.Builder
			cli := &Cli{
				Session:   session,
				OutStream: &outputBuf,
				ErrStream: &outputBuf,
				SystemVariables: &systemVariables{
					Params: make(map[string]ast.Node),
				},
			}

			// Parse the statement (same logic as in MCP tool handler)
			statement := strings.TrimSuffix(tt.statement, ";")
			stmt, err := cli.parseStatement(&inputStatement{
				statement:                statement,
				statementWithoutComments: statement,
				delim:                    ";",
			})
			if err != nil {
				if tt.wantError {
					return // Expected error during parsing
				}
				t.Fatalf("Failed to parse statement: %v", err)
			}

			// Execute the statement (same logic as in MCP tool handler)
			_, err = cli.executeStatement(ctx, stmt, false, statement)
			if err != nil {
				if tt.wantError {
					return // Expected error during execution
				}
				t.Fatalf("Failed to execute statement: %v", err)
			}

			if tt.wantError {
				t.Errorf("Expected error but statement executed successfully")
				return
			}

			// Check output contains expected content
			gotOutput := outputBuf.String()
			if !strings.Contains(gotOutput, tt.wantOutput) {
				t.Errorf("Output should contain %q, got: %s", tt.wantOutput, gotOutput)
			}

			// Verify output is not empty for valid statements
			compareResult(t, len(gotOutput) > 0, true)
		})
	}

	// Test database validation (the first step of RunMCP)
	t.Run("database exists validation", func(t *testing.T) {

		// Test with existing database
		_, session, teardown := initialize(t, testTableDDLs, nil)
		defer teardown()

		exists, err := session.DatabaseExists()
		if err != nil {
			t.Fatalf("DatabaseExists check failed: %v", err)
		}

		compareResult(t, exists, true)
	})

	// Test non-existent database error case
	t.Run("database does not exist", func(t *testing.T) {

		// Create system variables with non-existent database
		sysVarsNonExistent := systemVariables{
			Project:               "test-project",
			Instance:              "test-instance",
			Database:              "non-existent-database",
			Params:                make(map[string]ast.Node),
			RPCPriority:           sppb.RequestOptions_PRIORITY_UNSPECIFIED,
			Endpoint:              emulator.URI(),
			WithoutAuthentication: true,
		}

		sessionNonExistent, err := NewSession(context.Background(), &sysVarsNonExistent, defaultClientOptions(emulator)...)
		if err != nil {
			t.Fatalf("Failed to create session for non-existent database test: %v", err)
		}
		defer sessionNonExistent.Close()

		exists, err := sessionNonExistent.DatabaseExists()
		if err != nil {
			t.Fatalf("DatabaseExists check failed for non-existent database: %v", err)
		}

		// Should return false for non-existent database
		compareResult(t, exists, false)

		// Test that RunMCP returns error for non-existent database
		var outputBuf strings.Builder
		pipeReader, pipeWriter := io.Pipe()
		defer func() { _ = pipeReader.Close() }()
		defer func() { _ = pipeWriter.Close() }()

		cli, err := NewCli(context.Background(), nil, pipeReader, &outputBuf, &outputBuf, &sysVarsNonExistent)
		if err != nil {
			t.Fatalf("Failed to create CLI with non-existent database: %v", err)
		}
		defer cli.Session.Close()

		err = cli.RunMCP(context.Background())
		if err == nil {
			t.Errorf("RunMCP should return error for non-existent database")
			return
		}

		// Check that an error was returned (the actual error type may vary)
		// In practice, RunMCP fails due to database connection issues for non-existent databases
		compareResult(t, err != nil, true)
	})
}
