//go:build integration

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTeeOptionIntegration(t *testing.T) {
	if os.Getenv("SPANNER_EMULATOR_HOST") == "" {
		t.Skip("Test environment is not configured")
	}

	project := "test"
	instance := "test"
	database := "test"

	t.Run("batch mode with --tee", func(t *testing.T) {
		// Create temp directory for tee file
		tmpDir := t.TempDir()
		teeFile := filepath.Join(tmpDir, "output.log")

		// Execute query with --tee option
		args := []string{
			"--project", project,
			"--instance", instance,
			"--database", database,
			"--execute", "SELECT 'Hello from tee test' AS message",
			"--tee", teeFile,
		}

		// TODO: Implement RunCLI or use existing test helper
		t.Skip("RunCLI not implemented yet")
		return
		if result.ExitCode != 0 {
			t.Fatalf("Expected exit code 0, got %d. stderr: %s", result.ExitCode, result.Stderr)
		}

		// Check that output contains the result
		if !strings.Contains(result.Stdout, "Hello from tee test") {
			t.Errorf("Expected stdout to contain 'Hello from tee test', got: %s", result.Stdout)
		}

		// Check that tee file exists and contains the same output
		teeContent, err := os.ReadFile(teeFile)
		if err != nil {
			t.Fatalf("Failed to read tee file: %v", err)
		}

		if !strings.Contains(string(teeContent), "Hello from tee test") {
			t.Errorf("Expected tee file to contain 'Hello from tee test', got: %s", string(teeContent))
		}

		// Both outputs should be identical
		if result.Stdout != string(teeContent) {
			t.Errorf("Stdout and tee file content should be identical.\nStdout:\n%s\nTee file:\n%s", 
				result.Stdout, string(teeContent))
		}
	})

	t.Run("multiple statements with --tee append mode", func(t *testing.T) {
		// Create temp directory for tee file
		tmpDir := t.TempDir()
		teeFile := filepath.Join(tmpDir, "append.log")

		// First execution
		args1 := []string{
			"--project", project,
			"--instance", instance,
			"--database", database,
			"--execute", "SELECT 'First query' AS msg",
			"--tee", teeFile,
		}

		result1 := RunCLI(t, args1, "")
		if result1.ExitCode != 0 {
			t.Fatalf("First execution failed with exit code %d", result1.ExitCode)
		}

		// Second execution (should append)
		args2 := []string{
			"--project", project,
			"--instance", instance,
			"--database", database,
			"--execute", "SELECT 'Second query' AS msg",
			"--tee", teeFile,
		}

		result2 := RunCLI(t, args2, "")
		if result2.ExitCode != 0 {
			t.Fatalf("Second execution failed with exit code %d", result2.ExitCode)
		}

		// Check that tee file contains both outputs
		teeContent, err := os.ReadFile(teeFile)
		if err != nil {
			t.Fatalf("Failed to read tee file: %v", err)
		}

		if !strings.Contains(string(teeContent), "First query") {
			t.Errorf("Expected tee file to contain 'First query'")
		}
		if !strings.Contains(string(teeContent), "Second query") {
			t.Errorf("Expected tee file to contain 'Second query'")
		}

		// Verify append mode (both outputs should be present)
		expectedContent := result1.Stdout + result2.Stdout
		if string(teeContent) != expectedContent {
			t.Errorf("Tee file should contain concatenated outputs.\nExpected:\n%s\nGot:\n%s",
				expectedContent, string(teeContent))
		}
	})

	t.Run("--tee with non-existent directory", func(t *testing.T) {
		// Try to use tee with a non-existent directory
		teeFile := "/non/existent/directory/output.log"

		args := []string{
			"--project", project,
			"--instance", instance,
			"--database", database,
			"--execute", "SELECT 1",
			"--tee", teeFile,
		}

		// TODO: Implement RunCLI or use existing test helper
		t.Skip("RunCLI not implemented yet")
		return
		if result.ExitCode == 0 {
			t.Errorf("Expected non-zero exit code for non-existent directory")
		}

		if !strings.Contains(result.Stderr, "failed to open tee file") {
			t.Errorf("Expected error about failed to open tee file, got: %s", result.Stderr)
		}
	})
}