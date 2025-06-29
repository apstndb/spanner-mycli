//go:build integration

package main

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
)

func TestMetaCommandIntegration(t *testing.T) {
	ctx := context.Background()
	
	t.Run("shell command execution", func(t *testing.T) {
		sysVars := newSystemVariablesWithDefaults()
		sysVars.SkipSystemCommand = false
		
		// Create a mock stdin with a shell command
		input := strings.NewReader("\\! echo hello\nexit;\n")
		output := &bytes.Buffer{}
		
		cli, err := NewCli(ctx, nil, io.NopCloser(input), output, output, &sysVars)
		if err != nil {
			t.Fatalf("NewCli() error = %v", err)
		}
		
		// Run in batch mode to avoid interactive readline
		err = cli.RunBatch(ctx, "\\! echo hello")
		if err != nil {
			t.Errorf("RunBatch() error = %v", err)
		}
		
		// Check output contains "hello"
		if !strings.Contains(output.String(), "hello") {
			t.Errorf("Expected output to contain 'hello', got: %s", output.String())
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
		
		// Run in batch mode
		err = cli.RunBatch(ctx, "\\! echo hello")
		if err == nil {
			t.Error("Expected error when system commands are disabled")
		}
		
		// Check error output
		errorStr := errOutput.String()
		if !strings.Contains(errorStr, "system commands are disabled") {
			t.Errorf("Expected error message about system commands being disabled, got: %s", errorStr)
		}
	})
}