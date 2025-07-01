//go:build integration

package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceCommandWithEcho(t *testing.T) {
	ctx := context.Background()

	t.Run("source file with ECHO enabled", func(t *testing.T) {
		// Create a temporary SQL file with various statement types
		tmpDir := t.TempDir()
		sqlFile := filepath.Join(tmpDir, "test_echo.sql")
		sqlContent := `-- Test file for ECHO functionality
SELECT 1 AS n;
UPDATE users SET active = true WHERE id = 1;
CREATE TABLE test_table (id INT64) PRIMARY KEY (id);
EXPLAIN SELECT * FROM users;
DESCRIBE SELECT * FROM users;`
		
		if err := os.WriteFile(sqlFile, []byte(sqlContent), 0644); err != nil {
			t.Fatalf("Failed to create test SQL file: %v", err)
		}

		// Create system variables with ECHO enabled
		sysVars := newSystemVariablesWithDefaults()
		sysVars.Project = "test-project"
		sysVars.Instance = "test-instance"
		sysVars.Database = "test-database"
		sysVars.EchoInput = true  // Enable ECHO

		// Create a mock session
		session := &Session{
			systemVariables: &sysVars,
		}
		sessionHandler := NewSessionHandler(session)

		// Create CLI instance
		input := strings.NewReader("\\. " + sqlFile + "\nexit;\n")
		output := &bytes.Buffer{}
		
		cli := &Cli{
			SessionHandler:  sessionHandler,
			InStream:        io.NopCloser(input),
			OutStream:       output,
			ErrStream:       output,
			SystemVariables: &sysVars,
		}

		// Run in interactive mode
		err := cli.RunInteractive(ctx)
		if err != nil {
			// The `exit;` command causes RunInteractive to return an ExitCodeError, which is expected.
			if _, ok := err.(*ExitCodeError); !ok {
				t.Errorf("RunInteractive() returned an unexpected error = %v", err)
			}
		}

		// Check that statements were echoed
		outputStr := output.String()
		
		// These are the expected echoed statements (reconstructed by String() methods)
		expectedEchoes := []string{
			"SELECT 1 AS n;",
			"UPDATE users SET active = true WHERE id = 1;",
			"CREATE TABLE test_table (id INT64) PRIMARY KEY (id);",
			"EXPLAIN SELECT * FROM users;",
			"DESCRIBE SELECT * FROM users;",
		}

		for _, expected := range expectedEchoes {
			if !strings.Contains(outputStr, expected) {
				t.Errorf("Expected output to contain echoed statement %q, but it was not found.\nOutput:\n%s", expected, outputStr)
			}
		}

		// Verify that the comment was not echoed (comments are not part of Statement.String())
		if strings.Contains(outputStr, "-- Test file for ECHO functionality") {
			t.Errorf("Comments should not be echoed, but found in output")
		}
	})

	t.Run("source file with ECHO disabled", func(t *testing.T) {
		// Create a temporary SQL file
		tmpDir := t.TempDir()
		sqlFile := filepath.Join(tmpDir, "test_no_echo.sql")
		sqlContent := `SELECT 1 AS n;`
		
		if err := os.WriteFile(sqlFile, []byte(sqlContent), 0644); err != nil {
			t.Fatalf("Failed to create test SQL file: %v", err)
		}

		// Create system variables with ECHO disabled
		sysVars := newSystemVariablesWithDefaults()
		sysVars.Project = "test-project"
		sysVars.Instance = "test-instance"
		sysVars.Database = "test-database"
		sysVars.EchoInput = false  // Disable ECHO

		// Create a mock session
		session := &Session{
			systemVariables: &sysVars,
		}
		sessionHandler := NewSessionHandler(session)

		// Create CLI instance
		input := strings.NewReader("\\. " + sqlFile + "\nexit;\n")
		output := &bytes.Buffer{}
		
		cli := &Cli{
			SessionHandler:  sessionHandler,
			InStream:        io.NopCloser(input),
			OutStream:       output,
			ErrStream:       output,
			SystemVariables: &sysVars,
		}

		// Run in interactive mode
		err := cli.RunInteractive(ctx)
		if err != nil {
			// The `exit;` command causes RunInteractive to return an ExitCodeError, which is expected.
			if _, ok := err.(*ExitCodeError); !ok {
				t.Errorf("RunInteractive() returned an unexpected error = %v", err)
			}
		}

		// Check that statements were NOT echoed
		outputStr := output.String()
		if strings.Contains(outputStr, "SELECT 1 AS n;") {
			t.Errorf("Statement should not be echoed when EchoInput is false, but found in output:\n%s", outputStr)
		}
	})
}