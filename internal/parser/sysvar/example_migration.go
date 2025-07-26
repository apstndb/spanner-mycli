// Package sysvar example demonstrates how to migrate an existing
// system variable to use the new generics-based parser framework.
package sysvar

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	
	"github.com/apstndb/spanner-mycli/internal/parser"
)

// BEFORE: Old style system variable handling
// This is how STATEMENT_TIMEOUT was implemented in the original code

func oldStatementTimeoutSetter(sysVars interface{}, name, value string) error {
	// Manual parsing
	timeout, err := time.ParseDuration(unquoteString(value))
	if err != nil {
		return fmt.Errorf("invalid timeout format: %v", err)
	}
	if timeout < 0 {
		return fmt.Errorf("timeout cannot be negative")
	}
	// Set the value
	// sysVars.StatementTimeout = &timeout
	return nil
}

func oldStatementTimeoutGetter(sysVars interface{}, name string) (map[string]string, error) {
	// Manual formatting
	// if sysVars.StatementTimeout == nil {
	// 	return singletonMap(name, "10m"), nil
	// }
	// return singletonMap(name, sysVars.StatementTimeout.String()), nil
	return nil, nil
}

func unquoteString(s string) string {
	return strings.Trim(s, `"'`)
}

// AFTER: New style using the parser framework
// This shows how to migrate to the new framework

// Example system variables struct
type ExampleSystemVariables struct {
	StatementTimeout *time.Duration
	Verbose          bool
	TabWidth         int64
	CLIFormat        DisplayMode
	OptimizerVersion string
}

// CreateExampleRegistry shows how to set up system variables with the new framework
func CreateExampleRegistry(sysVars *ExampleSystemVariables) *DualModeRegistry {
	registry := NewDualModeRegistry()
	
	// Example 1: Optional duration with validation
	// STATEMENT_TIMEOUT can be NULL or a duration between 0 and 1 hour
	registry.Register(NewDualModeVariableParser(
		"STATEMENT_TIMEOUT",
		"Timeout value for statements (e.g., '10s', '5m', '1h'). Default is '10m'.",
		// Use dual-mode parser for GoogleSQL and simple modes
		parser.NewDualModeParser(
			// GoogleSQL mode: Use optional duration parser with validation
			parser.WithValidation(
				&parser.BaseParser[*time.Duration]{
					ParseFunc: func(value string) (*time.Duration, error) {
						// Handle NULL values
						upper := strings.ToUpper(strings.TrimSpace(value))
						if upper == "NULL" || upper == "'NULL'" {
							return nil, nil
						}
						// Use GoogleSQL string parser to extract the value
						strVal, err := parser.GoogleSQLStringParser.Parse(value)
						if err != nil {
							// Try as unquoted value
							strVal = strings.TrimSpace(value)
						}
						d, err := time.ParseDuration(strVal)
						if err != nil {
							return nil, err
						}
						return &d, nil
					},
				},
				func(v *time.Duration) error {
					if v != nil && *v < 0 {
						return errors.New("timeout cannot be negative")
					}
					if v != nil && *v > time.Hour {
						return errors.New("timeout cannot exceed 1 hour")
					}
					return nil
				},
			),
			// Simple mode: Direct duration parsing
			NewOptionalDurationParser().WithRange(0, time.Hour),
		),
		// Setter
		func(v *time.Duration) error {
			sysVars.StatementTimeout = v
			return nil
		},
		// Getter
		func() (*time.Duration, bool) {
			return sysVars.StatementTimeout, true
		},
	))
	
	// Example 2: Boolean variable
	// CLI_VERBOSE is a simple boolean
	registry.Register(CreateDualModeBooleanParser(
		"CLI_VERBOSE",
		"Enable verbose output",
		func() bool { return sysVars.Verbose },
		func(v bool) error {
			sysVars.Verbose = v
			return nil
		},
	))
	
	// Example 3: Integer with range validation
	// CLI_TAB_WIDTH must be between 1 and 100
	registry.Register(CreateDualModeIntegerParser(
		"CLI_TAB_WIDTH",
		"Tab width for expanding tabs",
		func() int64 { return sysVars.TabWidth },
		func(v int64) error {
			sysVars.TabWidth = v
			return nil
		},
		ptr(int64(1)), ptr(int64(100)),
	))
	
	// Example 4: Enum variable
	// CLI_FORMAT has predefined valid values
	registry.Register(CreateDualModeEnumParser(
		"CLI_FORMAT",
		"Output format for query results",
		map[string]DisplayMode{
			"TABLE":               DisplayModeTable,
			"VERTICAL":            DisplayModeVertical,
			"TAB":                 DisplayModeTab,
			"CSV":                 DisplayModeCSV,
		},
		func() DisplayMode { return sysVars.CLIFormat },
		func(v DisplayMode) error {
			sysVars.CLIFormat = v
			return nil
		},
	))
	
	// Example 5: String with custom validation
	// OPTIMIZER_VERSION can be an integer or 'LATEST'
	registry.Register(NewDualModeVariableParser(
		"OPTIMIZER_VERSION",
		"Optimizer version (integer or 'LATEST')",
		parser.NewDualModeParser(
			// GoogleSQL mode
			parser.GoogleSQLStringParser,
			// Simple mode with custom validation
			parser.WithValidation(
				parser.NewStringParser(),
				func(v string) error {
					if v == "" || strings.ToUpper(v) == "LATEST" {
						return nil
					}
					// Must be a valid integer
					if _, err := strconv.Atoi(v); err != nil {
						return fmt.Errorf("must be an integer or 'LATEST'")
					}
					return nil
				},
			),
		),
		func(v string) error {
			sysVars.OptimizerVersion = v
			return nil
		},
		func() (string, bool) {
			return sysVars.OptimizerVersion, true
		},
	))
	
	return registry
}

// Usage examples showing the benefits of the new framework
func ExampleUsage() {
	sysVars := &ExampleSystemVariables{
		TabWidth:  8,
		CLIFormat: DisplayModeTable,
	}
	
	registry := CreateExampleRegistry(sysVars)
	
	// Example 1: Setting from REPL (GoogleSQL mode)
	// The value '10s' is parsed as a GoogleSQL string literal
	err := registry.SetFromREPL("STATEMENT_TIMEOUT", "'10s'")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	}
	
	// Example 2: Setting from CLI (Simple mode)
	// No quotes needed for CLI flags
	err = registry.SetFromCLI("STATEMENT_TIMEOUT", "10s")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	}
	
	// Example 3: Validation works automatically
	err = registry.SetFromCLI("CLI_TAB_WIDTH", "200")
	if err != nil {
		fmt.Printf("Expected error: %v\n", err)
		// Output: Expected error: invalid value for CLI_TAB_WIDTH: value 200 is greater than maximum 100
	}
	
	// Example 4: Enum validation with helpful error messages
	err = registry.SetFromCLI("CLI_FORMAT", "INVALID")
	if err != nil {
		fmt.Printf("Expected error: %v\n", err)
		// Output: Expected error: invalid value for CLI_FORMAT: invalid value "INVALID", must be one of: TABLE, VERTICAL, TAB, CSV
	}
	
	// Example 5: Type safety prevents mistakes
	// The parser ensures that only valid DisplayMode values can be set
	// This prevents runtime errors from invalid string values
}

// Benefits of the new framework:
// 1. Consistent error messages across all variables
// 2. Automatic validation based on type constraints
// 3. Dual-mode support for different input sources
// 4. Type safety at compile time
// 5. Reusable parser logic
// 6. Easy to test and maintain
// 7. Clear separation of concerns