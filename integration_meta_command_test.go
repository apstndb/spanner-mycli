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
}