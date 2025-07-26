// Package sysvar migration example demonstrates how to migrate
// existing system variables to use the new parser framework.
package sysvar

import (
	"fmt"
	"time"
	
	"github.com/apstndb/spanner-mycli/internal/parser"
)

// Example system variables structure (simplified version)
type SystemVariables struct {
	// Boolean variables
	ReadOnly            bool
	Verbose             bool
	ReturnCommitStats   bool
	DataBoostEnabled    bool
	AutoBatchDML        bool
	
	// Integer variables
	MaxPartitionedParallelism int64
	TabWidth                  int64
	ExplainWrapWidth          int64
	Port                      int
	
	// Duration variables
	StatementTimeout *time.Duration
	MaxCommitDelay   *time.Duration
	
	// String variables
	OptimizerVersion string
	TransactionTag   string
	RequestTag       string
	
	// Enum variables
	CLIFormat         DisplayMode
	AutocommitDMLMode AutocommitDMLMode
}

// CreateSystemVariableParsers demonstrates how to create parsers for system variables
func CreateSystemVariableParsers(sysVars *SystemVariables) *Registry {
	registry := NewRegistry()
	
	// Boolean variables
	registry.Register(CreateBooleanParser(
		"READONLY",
		"A boolean indicating whether or not the connection is in read-only mode",
		func() bool { return sysVars.ReadOnly },
		func(v bool) error { 
			sysVars.ReadOnly = v
			return nil
		},
	))
	
	registry.Register(CreateBooleanParser(
		"CLI_VERBOSE",
		"Enable verbose output",
		func() bool { return sysVars.Verbose },
		func(v bool) error {
			sysVars.Verbose = v
			return nil
		},
	))
	
	registry.Register(CreateBooleanParser(
		"RETURN_COMMIT_STATS",
		"A property of type BOOL indicating whether statistics should be returned for transactions",
		func() bool { return sysVars.ReturnCommitStats },
		func(v bool) error {
			sysVars.ReturnCommitStats = v
			return nil
		},
	))
	
	// Integer variables with validation
	registry.Register(CreateIntegerParser(
		"MAX_PARTITIONED_PARALLELISM",
		"Number of worker threads for partitioned operations",
		func() int64 { return sysVars.MaxPartitionedParallelism },
		func(v int64) error {
			sysVars.MaxPartitionedParallelism = v
			return nil
		},
		ptr(int64(1)), ptr(int64(1000)), // min: 1, max: 1000
	))
	
	registry.Register(CreateIntegerParser(
		"CLI_TAB_WIDTH",
		"Tab width for expanding tabs",
		func() int64 { return sysVars.TabWidth },
		func(v int64) error {
			sysVars.TabWidth = v
			return nil
		},
		ptr(int64(1)), ptr(int64(100)), // min: 1, max: 100
	))
	
	// Duration variables
	registry.Register(NewVariableParser(
		"STATEMENT_TIMEOUT",
		"Timeout value for statements",
		NewOptionalDurationParser().WithRange(0, time.Hour),
		func(v *time.Duration) error {
			sysVars.StatementTimeout = v
			return nil
		},
		func() (*time.Duration, bool) { 
			return sysVars.StatementTimeout, true 
		},
	))
	
	registry.Register(NewVariableParser(
		"MAX_COMMIT_DELAY",
		"Latency to improve throughput (0-500ms)",
		NewOptionalDurationParser().WithRange(0, 500*time.Millisecond),
		func(v *time.Duration) error {
			sysVars.MaxCommitDelay = v
			return nil
		},
		func() (*time.Duration, bool) {
			return sysVars.MaxCommitDelay, true
		},
	))
	
	// String variables
	registry.Register(CreateStringParser(
		"OPTIMIZER_VERSION",
		"Optimizer version (integer or 'LATEST')",
		func() string { return sysVars.OptimizerVersion },
		func(v string) error {
			// Additional validation could be added here
			if v != "" && v != "LATEST" {
				// Try to parse as integer
				if _, err := parser.NewIntParser().Parse(v); err != nil {
					return fmt.Errorf("must be an integer or 'LATEST'")
				}
			}
			sysVars.OptimizerVersion = v
			return nil
		},
	))
	
	// Enum variables
	registry.Register(CreateEnumParser(
		"CLI_FORMAT",
		"Output format for query results",
		map[string]DisplayMode{
			"TABLE":               DisplayModeTable,
			"TABLE_COMMENT":       DisplayModeTableComment,
			"TABLE_DETAIL_COMMENT": DisplayModeTableDetailComment,
			"VERTICAL":            DisplayModeVertical,
			"TAB":                 DisplayModeTab,
			"HTML":                DisplayModeHTML,
			"XML":                 DisplayModeXML,
			"CSV":                 DisplayModeCSV,
		},
		func() DisplayMode { return sysVars.CLIFormat },
		func(v DisplayMode) error {
			sysVars.CLIFormat = v
			return nil
		},
	))
	
	registry.Register(CreateEnumParser(
		"AUTOCOMMIT_DML_MODE",
		"Autocommit mode for DML statements",
		map[string]AutocommitDMLMode{
			"TRANSACTIONAL":         AutocommitDMLModeTransactional,
			"PARTITIONED_NON_ATOMIC": AutocommitDMLModePartitionedNonAtomic,
		},
		func() AutocommitDMLMode { return sysVars.AutocommitDMLMode },
		func(v AutocommitDMLMode) error {
			sysVars.AutocommitDMLMode = v
			return nil
		},
	))
	
	return registry
}

// Example usage showing how the migration would work
func ExampleMigration() {
	// Create system variables instance
	sysVars := &SystemVariables{
		ReturnCommitStats: true,
		TabWidth:          8,
		CLIFormat:         DisplayModeTable,
	}
	
	// Create parser registry
	registry := CreateSystemVariableParsers(sysVars)
	
	// Example: Setting a boolean variable
	if err := registry.Set("READONLY", "true"); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
	
	// Example: Setting an integer with validation
	if err := registry.Set("CLI_TAB_WIDTH", "4"); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
	
	// Example: Setting an invalid value (will fail validation)
	if err := registry.Set("CLI_TAB_WIDTH", "200"); err != nil {
		fmt.Printf("Expected error: %v\n", err)
	}
	
	// Example: Setting an enum value
	if err := registry.Set("CLI_FORMAT", "VERTICAL"); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
	
	// Example: Getting a value
	if value, err := registry.Get("CLI_FORMAT"); err == nil {
		fmt.Printf("CLI_FORMAT = %s\n", value)
	}
}

// Helper function to create pointer
func ptr[T any](v T) *T {
	return &v
}