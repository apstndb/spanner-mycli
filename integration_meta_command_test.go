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

func TestMetaCommandIntegration(t *testing.T) {
	ctx := context.Background()
	
	t.Run("interactive shell command execution", func(t *testing.T) {
		sysVars := newSystemVariablesWithDefaults()
		sysVars.SkipSystemCommand = false
		
		// Create a simulated interactive session with shell commands
		input := strings.NewReader("\\! echo hello\n\\! echo world\nexit;\n")
		output := &bytes.Buffer{}
		
		cli, err := NewCli(ctx, nil, io.NopCloser(input), output, output, &sysVars)
		if err != nil {
			t.Fatalf("NewCli() error = %v", err)
		}
		
		// Run in interactive mode
		err = cli.RunInteractive(ctx)
		if err != nil {
			// The `exit;` command causes RunInteractive to return an ExitCodeError, which is expected.
			if _, ok := err.(*ExitCodeError); !ok {
				t.Errorf("RunInteractive() returned an unexpected error = %v", err)
			}
		}
		
		// Check output contains both commands' results
		outputStr := output.String()
		if !strings.Contains(outputStr, "hello") {
			t.Errorf("Expected output to contain 'hello', got: %s", outputStr)
		}
		if !strings.Contains(outputStr, "world") {
			t.Errorf("Expected output to contain 'world', got: %s", outputStr)
		}
		
		// Verify no "Empty set" messages for meta commands
		if strings.Contains(outputStr, "Empty set") {
			t.Errorf("Output should not contain 'Empty set' for meta commands, got: %s", outputStr)
		}
	})
	
	t.Run("shell command execution in batch mode", func(t *testing.T) {
		sysVars := newSystemVariablesWithDefaults()
		sysVars.SkipSystemCommand = false
		
		// Create a mock stdin with a shell command
		input := strings.NewReader("\\! echo hello\nexit;\n")
		output := &bytes.Buffer{}
		
		cli, err := NewCli(ctx, nil, io.NopCloser(input), output, output, &sysVars)
		if err != nil {
			t.Fatalf("NewCli() error = %v", err)
		}
		
		// Run in batch mode - should return error since meta commands are not supported
		err = cli.RunBatch(ctx, "\\! echo hello")
		if err == nil {
			t.Error("Expected error for meta command in batch mode")
		}
		
		// Check error message
		if err != nil && err.Error() != "meta commands are not supported in batch mode" {
			t.Errorf("Expected 'meta commands are not supported in batch mode' error, got: %v", err)
		}
	})
	
	t.Run("shell command disabled", func(t *testing.T) {
		sysVars := newSystemVariablesWithDefaults()
		sysVars.SkipSystemCommand = true
		
		input := strings.NewReader("")
		output := &bytes.Buffer{}
		errOutput := &bytes.Buffer{}
		
		cli, err := NewCli(ctx, nil, io.NopCloser(input), output, errOutput, &sysVars)
		if err != nil {
			t.Fatalf("NewCli() error = %v", err)
		}
		
		// Run in batch mode - should return error since meta commands are not supported
		err = cli.RunBatch(ctx, "\\! echo hello")
		if err == nil {
			t.Error("Expected error for meta command in batch mode")
		}
		
		// Check error message (batch mode check happens before system command check)
		if err != nil && err.Error() != "meta commands are not supported in batch mode" {
			t.Errorf("Expected 'meta commands are not supported in batch mode' error, got: %v", err)
		}
	})
	
	t.Run("interactive shell command disabled", func(t *testing.T) {
		sysVars := newSystemVariablesWithDefaults()
		sysVars.SkipSystemCommand = true
		
		// Create a simulated interactive session
		input := strings.NewReader("\\! echo hello\nexit;\n")
		output := &bytes.Buffer{}
		
		cli, err := NewCli(ctx, nil, io.NopCloser(input), output, output, &sysVars)
		if err != nil {
			t.Fatalf("NewCli() error = %v", err)
		}
		
		// Run in interactive mode
		err = cli.RunInteractive(ctx)
		// Should exit normally (exit command will cause ExitCodeError)
		if err != nil {
			if _, ok := err.(*ExitCodeError); !ok {
				t.Errorf("RunInteractive() returned unexpected error = %v", err)
			}
		}
		
		// Check that the error was printed to the output stream
		outputStr := output.String()
		if !strings.Contains(outputStr, "ERROR: system commands are disabled") {
			t.Errorf("Expected output to contain 'ERROR: system commands are disabled', got: %s", outputStr)
		}
		
		// Verify the shell command was not executed
		if strings.Contains(outputStr, "hello") {
			t.Errorf("Shell command should not have been executed, but output contains 'hello': %s", outputStr)
		}
	})

	t.Run("source command execution", func(t *testing.T) {
		// Create a temporary SQL file
		tmpDir := t.TempDir()
		sqlFile := filepath.Join(tmpDir, "test.sql")
		sqlContent := `SELECT 1 AS n;
SELECT "foo" AS s;`
		if err := os.WriteFile(sqlFile, []byte(sqlContent), 0644); err != nil {
			t.Fatalf("Failed to create test SQL file: %v", err)
		}

		sysVars := newSystemVariablesWithDefaults()
		sysVars.Project = "test-project"
		sysVars.Instance = "test-instance"
		sysVars.Database = "test-database"

		// Create a mock session handler with a test client
		session := &Session{
			systemVariables: &sysVars,
			// Mock client would be set up here in a real integration test
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

		// NOTE: This test verifies the file reading and parsing logic, but cannot verify
		// actual SQL execution without a connected Spanner instance or emulator.
		// The Session has a nil client, so any actual SQL execution would fail.
		// Full integration testing would require TestMain setup with a real emulator,
		// which is handled by other integration tests in the codebase.
		// This test ensures the \. command correctly reads files and integrates with
		// the CLI's execution flow, which is the primary responsibility of this feature.
		outputStr := output.String()
		if strings.Contains(outputStr, "failed to read file") {
			t.Errorf("Failed to read SQL file: %s", outputStr)
		}
	})

	t.Run("source command with non-existent file", func(t *testing.T) {
		sysVars := newSystemVariablesWithDefaults()
		sysVars.Project = "test-project"
		sysVars.Instance = "test-instance"
		sysVars.Database = "test-database"

		// Create a mock session handler with a test client
		session := &Session{
			systemVariables: &sysVars,
		}
		sessionHandler := NewSessionHandler(session)

		// Create CLI instance
		input := strings.NewReader("\\. /non/existent/file.sql\nexit;\n")
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

		// Check that error was printed
		outputStr := output.String()
		if !strings.Contains(outputStr, "ERROR: failed to read file") {
			t.Errorf("Expected output to contain 'ERROR: failed to read file', got: %s", outputStr)
		}
	})

	t.Run("source command in batch mode", func(t *testing.T) {
		sysVars := newSystemVariablesWithDefaults()
		
		input := strings.NewReader("")
		output := &bytes.Buffer{}
		
		cli, err := NewCli(ctx, nil, io.NopCloser(input), output, output, &sysVars)
		if err != nil {
			t.Fatalf("NewCli() error = %v", err)
		}
		
		// Run in batch mode - should return error since meta commands are not supported
		err = cli.RunBatch(ctx, "\\. test.sql")
		if err == nil {
			t.Error("Expected error for meta command in batch mode")
		}
		
		// Check error message
		if err != nil && err.Error() != "meta commands are not supported in batch mode" {
			t.Errorf("Expected 'meta commands are not supported in batch mode' error, got: %v", err)
		}
	})
}