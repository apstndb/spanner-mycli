package main

import (
	"testing"

	"github.com/jessevdk/go-flags"
)

func TestInitializeWithSet(t *testing.T) {
	// Simulate the test case that's failing
	var gopts globalOptions
	parser := flags.NewParser(&gopts, flags.Default)

	args := []string{
		"--project", "p",
		"--instance", "i",
		"--database", "d",
		"--execute", "SELECT 1",
		"--set", "CLI_FORMAT=VERTICAL",
	}

	_, err := parser.ParseArgs(args)
	if err != nil {
		t.Fatalf("Failed to parse args: %v", err)
	}

	// Check parsed values
	t.Logf("opts.Set = %+v", gopts.Spanner.Set)

	// Initialize system variables
	sysVars, err := initializeSystemVariables(&gopts.Spanner)
	if err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	// Check the values
	t.Logf("sysVars.CLIFormat = %v (type: %T)", sysVars.CLIFormat, sysVars.CLIFormat)
	t.Logf("DisplayModeVertical = %v", DisplayModeVertical)

	if sysVars.CLIFormat != DisplayModeVertical {
		t.Errorf("CLIFormat = %v, want %v", sysVars.CLIFormat, DisplayModeVertical)
	}
}
