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
		
		// Check error type
		if err != nil {
			if _, ok := err.(*ExitCodeError); !ok {
				t.Errorf("Expected error of type *ExitCodeError, but got %T", err)
			}
			// Check that the correct error message was printed to the error stream
			const expectedErrStr = "meta commands are not supported in batch mode"
			if !strings.Contains(output.String(), expectedErrStr) {
				t.Errorf("Expected output to contain %q, but got: %s", expectedErrStr, output.String())
			}
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
		
		// Check error type
		if err != nil {
			if _, ok := err.(*ExitCodeError); !ok {
				t.Errorf("Expected error of type *ExitCodeError, but got %T", err)
			}
			// Check that the correct error message was printed to the error stream
			const expectedErrStr = "meta commands are not supported in batch mode"
			if !strings.Contains(output.String(), expectedErrStr) {
				t.Errorf("Expected output to contain %q, but got: %s", expectedErrStr, output.String())
			}
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
		if !strings.Contains(outputStr, "ERROR: failed to open file") {
			t.Errorf("Expected output to contain 'ERROR: failed to open file', got: %s", outputStr)
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
		
		// Check error type
		if err != nil {
			if _, ok := err.(*ExitCodeError); !ok {
				t.Errorf("Expected error of type *ExitCodeError, but got %T", err)
			}
			// Check that the correct error message was printed to the error stream
			const expectedErrStr = "meta commands are not supported in batch mode"
			if !strings.Contains(output.String(), expectedErrStr) {
				t.Errorf("Expected output to contain %q, but got: %s", expectedErrStr, output.String())
			}
		}
	})

	t.Run("prompt command execution", func(t *testing.T) {
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
		input := strings.NewReader("\\R custom-prompt>\nSHOW VARIABLE CLI_PROMPT;\nexit;\n")
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

		// Verify the prompt was changed (with trailing space added)
		if sysVars.Prompt != "custom-prompt> " {
			t.Errorf("Expected prompt to be 'custom-prompt> ', got %q", sysVars.Prompt)
		}

		// Check that SHOW VARIABLE output contains the expected table format
		// The output should have a single column "CLI_PROMPT" with value "custom-prompt> "
		outputStr := output.String()
		expectedRow := "| custom-prompt> |"  // Note: includes the trailing space
		
		if !strings.Contains(outputStr, "CLI_PROMPT") {
			t.Errorf("Expected output to contain column header 'CLI_PROMPT', got: %s", outputStr)
		}
		if !strings.Contains(outputStr, expectedRow) {
			t.Errorf("Expected output to contain row %q, got: %s", expectedRow, outputStr)
		}
	})

	t.Run("prompt command with percent expansion", func(t *testing.T) {
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
		input := strings.NewReader("\\R [%p/%i/%d]>\nexit;\n")
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

		// Verify the prompt was changed (expansion happens during display, with trailing space added)
		if sysVars.Prompt != "[%p/%i/%d]> " {
			t.Errorf("Expected prompt to be '[%%p/%%i/%%d]> ', got %q", sysVars.Prompt)
		}
		
		// Note: This test verifies that the prompt is stored with percent patterns intact.
		// The actual expansion of %p, %i, %d happens during prompt display in getInterpolatedPrompt.
		// Since this integration test doesn't have a real Spanner connection, we can't verify
		// the expanded output. Unit tests in cli_test.go cover the expansion logic.
	})

	t.Run("prompt command in batch mode", func(t *testing.T) {
		sysVars := newSystemVariablesWithDefaults()
		
		input := strings.NewReader("")
		output := &bytes.Buffer{}
		
		cli, err := NewCli(ctx, nil, io.NopCloser(input), output, output, &sysVars)
		if err != nil {
			t.Fatalf("NewCli() error = %v", err)
		}
		
		// Run in batch mode - should return error since meta commands are not supported
		err = cli.RunBatch(ctx, "\\R new-prompt>")
		if err == nil {
			t.Error("Expected error for meta command in batch mode")
		}
		
		// Check error type
		if err != nil {
			if _, ok := err.(*ExitCodeError); !ok {
				t.Errorf("Expected error of type *ExitCodeError, but got %T", err)
			}
			// Check that the correct error message was printed to the error stream
			const expectedErrStr = "meta commands are not supported in batch mode"
			if !strings.Contains(output.String(), expectedErrStr) {
				t.Errorf("Expected output to contain %q, but got: %s", expectedErrStr, output.String())
			}
		}
	})

	t.Run("use database command in interactive mode", func(t *testing.T) {
		sysVars := newSystemVariablesWithDefaults()
		
		// Create a simulated interactive session with \u command
		input := strings.NewReader("\\u test-db\nexit;\n")
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
		
		// Check output - should show error for non-existent database
		outputStr := output.String()
		if !strings.Contains(outputStr, "ERROR: unknown database \"test-db\"") {
			t.Errorf("Expected output to contain 'ERROR: unknown database', got: %s", outputStr)
		}
	})

	t.Run("use database command with backticks", func(t *testing.T) {
		sysVars := newSystemVariablesWithDefaults()
		
		// Create a simulated interactive session with \u command using backticks
		input := strings.NewReader("\\u `my-database`\nexit;\n")
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
		
		// Check output - should show error for non-existent database
		outputStr := output.String()
		if !strings.Contains(outputStr, "ERROR: unknown database \"my-database\"") {
			t.Errorf("Expected output to contain 'ERROR: unknown database', got: %s", outputStr)
		}
	})

	t.Run("use database command in batch mode", func(t *testing.T) {
		sysVars := newSystemVariablesWithDefaults()
		
		input := strings.NewReader("")
		output := &bytes.Buffer{}
		
		cli, err := NewCli(ctx, nil, io.NopCloser(input), output, output, &sysVars)
		if err != nil {
			t.Fatalf("NewCli() error = %v", err)
		}
		
		// Run in batch mode - should return error since meta commands are not supported
		err = cli.RunBatch(ctx, "\\u test-db")
		if err == nil {
			t.Error("Expected error for meta command in batch mode")
		}
		
		// Check error type
		if err != nil {
			if _, ok := err.(*ExitCodeError); !ok {
				t.Errorf("Expected error of type *ExitCodeError, but got %T", err)
			}
			// Check that the correct error message was printed to the error stream
			const expectedErrStr = "meta commands are not supported in batch mode"
			if !strings.Contains(output.String(), expectedErrStr) {
				t.Errorf("Expected output to contain %q, but got: %s", expectedErrStr, output.String())
			}
		}
	})

	// Helper functions for tee tests
	setupTeeTestCLI := func(commands string) (*Cli, *bytes.Buffer) {
		sysVars := newSystemVariablesWithDefaults()
		sysVars.Project = "test-project"
		sysVars.Instance = "test-instance"
		sysVars.Database = "test-database"
		
		// Create TeeManager
		sysVars.TeeManager = NewTeeManager(os.Stdout, os.Stderr)
		sysVars.CurrentOutStream = sysVars.TeeManager.GetWriter()
		sysVars.CurrentErrStream = os.Stderr

		// Create a mock session handler
		session := &Session{
			systemVariables: &sysVars,
		}
		sessionHandler := NewSessionHandler(session)

		// Create CLI instance
		input := strings.NewReader(commands + "\nexit;\n")
		output := &bytes.Buffer{}
		
		cli := &Cli{
			SessionHandler:  sessionHandler,
			InStream:        io.NopCloser(input),
			OutStream:       output,
			ErrStream:       output,
			SystemVariables: &sysVars,
		}

		return cli, output
	}

	runInteractiveCLI := func(t *testing.T, cli *Cli) {
		err := cli.RunInteractive(ctx)
		if err != nil {
			if _, ok := err.(*ExitCodeError); !ok {
				t.Errorf("RunInteractive() returned an unexpected error = %v", err)
			}
		}
	}

	verifyTeeFileContent := func(t *testing.T, teeFile, expectedContent string) {
		// Check that tee file was created
		if _, err := os.Stat(teeFile); os.IsNotExist(err) {
			t.Errorf("Expected tee file %s to be created", teeFile)
			return
		}

		// Read tee file content
		content, err := os.ReadFile(teeFile)
		if err != nil {
			t.Fatalf("Failed to read tee file: %v", err)
		}

		// Verify content
		if !strings.Contains(string(content), expectedContent) {
			t.Errorf("Expected tee file to contain %q, got: %s", expectedContent, string(content))
		}
	}

	t.Run("tee output command execution", func(t *testing.T) {
		tmpDir := t.TempDir()
		teeFile := filepath.Join(tmpDir, "test_output.log")

		cli, _ := setupTeeTestCLI("\\T " + teeFile + "\n\\! echo 'tee test'\n\\t")
		runInteractiveCLI(t, cli)
		verifyTeeFileContent(t, teeFile, "tee test")
	})

	t.Run("tee command with quoted filename", func(t *testing.T) {
		tmpDir := t.TempDir()
		teeFile := filepath.Join(tmpDir, "file with spaces.log")

		cli, _ := setupTeeTestCLI(`\T "` + teeFile + `"` + "\n\\! echo 'quoted filename test'\n\\t")
		runInteractiveCLI(t, cli)
		verifyTeeFileContent(t, teeFile, "quoted filename test")
	})

	t.Run("tee append to existing file", func(t *testing.T) {
		tmpDir := t.TempDir()
		teeFile := filepath.Join(tmpDir, "existing.log")

		// Create file with initial content
		initialContent := "Initial content\n"
		if err := os.WriteFile(teeFile, []byte(initialContent), 0644); err != nil {
			t.Fatalf("Failed to create initial file: %v", err)
		}

		cli, _ := setupTeeTestCLI("\\T " + teeFile + "\n\\! echo 'appended content'\n\\t")
		runInteractiveCLI(t, cli)
		
		// Read and verify content
		content, err := os.ReadFile(teeFile)
		if err != nil {
			t.Fatalf("Failed to read tee file: %v", err)
		}

		contentStr := string(content)
		if !strings.Contains(contentStr, "Initial content") {
			t.Errorf("Expected tee file to preserve initial content")
		}
		if !strings.Contains(contentStr, "appended content") {
			t.Errorf("Expected tee file to contain appended content")
		}
	})

	t.Run("tee command with directory error", func(t *testing.T) {
		tmpDir := t.TempDir()

		cli, output := setupTeeTestCLI("\\T " + tmpDir)
		runInteractiveCLI(t, cli)

		// Check that error was printed
		outputStr := output.String()
		if !strings.Contains(outputStr, "ERROR: tee output to a non-regular file is not supported") {
			t.Errorf("Expected output to contain error about non-regular file, got: %s", outputStr)
		}
	})

	// Table-driven tests for batch mode errors
	batchModeTests := []struct {
		name     string
		command  string
	}{
		{"tee command in batch mode", "\\T output.log"},
		{"disable tee command in batch mode", "\\t"},
	}

	for _, tt := range batchModeTests {
		t.Run(tt.name, func(t *testing.T) {
			sysVars := newSystemVariablesWithDefaults()
			input := strings.NewReader("")
			output := &bytes.Buffer{}
			
			cli, err := NewCli(ctx, nil, io.NopCloser(input), output, output, &sysVars)
			if err != nil {
				t.Fatalf("NewCli() error = %v", err)
			}
			
			// Run in batch mode - should return error since meta commands are not supported
			err = cli.RunBatch(ctx, tt.command)
			if err == nil {
				t.Error("Expected error for meta command in batch mode")
			}
			
			// Check error type
			if err != nil {
				if _, ok := err.(*ExitCodeError); !ok {
					t.Errorf("Expected error of type *ExitCodeError, but got %T", err)
				}
				// Check that the correct error message was printed to the error stream
				const expectedErrStr = "meta commands are not supported in batch mode"
				if !strings.Contains(output.String(), expectedErrStr) {
					t.Errorf("Expected output to contain %q, but got: %s", expectedErrStr, output.String())
				}
			}
		})
	}
}