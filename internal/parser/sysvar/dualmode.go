package sysvar

import (
	"fmt"
	"strings"
	
	"github.com/apstndb/spanner-mycli/internal/parser"
)

// DualModeVariableParser wraps a dual-mode parser for system variables.
// It automatically selects the appropriate parsing mode based on the context.
type DualModeVariableParser struct {
	Name        string
	Description string
	parser      interface{} // Will be DualModeParser[T] for some T
	setter      interface{} // Function to set the value
	getter      interface{} // Function to get the value
	readOnly    bool
}

// NewDualModeVariableParser creates a system variable parser with dual-mode support.
func NewDualModeVariableParser[T any](
	name string,
	description string,
	parser parser.DualModeParser[T],
	setter func(value T) error,
	getter func() (T, bool),
) *DualModeVariableParser {
	return &DualModeVariableParser{
		Name:        name,
		Description: description,
		parser:      parser,
		setter:      setter,
		getter:      getter,
		readOnly:    setter == nil,
	}
}

// ParseAndSet parses a string value using the specified mode and sets it.
func (vp *DualModeVariableParser) ParseAndSetWithMode(value string, mode parser.ParseMode) error {
	if vp.readOnly {
		return fmt.Errorf("variable %s is read-only", vp.Name)
	}
	
	// Type assertion to get the concrete parser type
	switch p := vp.parser.(type) {
	case parser.DualModeParser[bool]:
		if setter, ok := vp.setter.(func(bool) error); ok {
			parsed, err := p.ParseAndValidateWithMode(value, mode)
			if err != nil {
				return fmt.Errorf("invalid value for %s: %w", vp.Name, err)
			}
			return setter(parsed)
		}
	case parser.DualModeParser[int64]:
		if setter, ok := vp.setter.(func(int64) error); ok {
			parsed, err := p.ParseAndValidateWithMode(value, mode)
			if err != nil {
				return fmt.Errorf("invalid value for %s: %w", vp.Name, err)
			}
			return setter(parsed)
		}
	case parser.DualModeParser[string]:
		if setter, ok := vp.setter.(func(string) error); ok {
			parsed, err := p.ParseAndValidateWithMode(value, mode)
			if err != nil {
				return fmt.Errorf("invalid value for %s: %w", vp.Name, err)
			}
			return setter(parsed)
		}
	default:
		// For other types, use reflection or add more cases as needed
		return fmt.Errorf("unsupported parser type for variable %s", vp.Name)
	}
	
	return fmt.Errorf("type mismatch in setter for variable %s", vp.Name)
}

// DualModeRegistry manages system variable parsers with context awareness.
type DualModeRegistry struct {
	parsers map[string]*DualModeVariableParser
}

// NewDualModeRegistry creates a new dual-mode registry.
func NewDualModeRegistry() *DualModeRegistry {
	return &DualModeRegistry{
		parsers: make(map[string]*DualModeVariableParser),
	}
}

// Register adds a dual-mode variable parser to the registry.
func (r *DualModeRegistry) Register(parser *DualModeVariableParser) error {
	upperName := strings.ToUpper(parser.Name)
	if _, exists := r.parsers[upperName]; exists {
		return fmt.Errorf("variable %s already registered", upperName)
	}
	r.parsers[upperName] = parser
	return nil
}

// SetFromREPL parses and sets a value from a SET statement (GoogleSQL mode).
func (r *DualModeRegistry) SetFromREPL(name, value string) error {
	return r.setWithMode(name, value, parser.ParseModeGoogleSQL)
}

// SetFromCLI parses and sets a value from CLI flags or config (simple mode).
func (r *DualModeRegistry) SetFromCLI(name, value string) error {
	return r.setWithMode(name, value, parser.ParseModeSimple)
}

func (r *DualModeRegistry) setWithMode(name, value string, mode parser.ParseMode) error {
	upperName := strings.ToUpper(name)
	p, exists := r.parsers[upperName]
	if !exists {
		return fmt.Errorf("unknown variable: %s", name)
	}
	
	return p.ParseAndSetWithMode(value, mode)
}

// Example helper functions for creating dual-mode parsers

// CreateDualModeBooleanParser creates a boolean parser with dual-mode support.
func CreateDualModeBooleanParser(name, description string, getter func() bool, setter func(bool) error) *DualModeVariableParser {
	return NewDualModeVariableParser(
		name,
		description,
		parser.DualModeBoolParser,
		setter,
		func() (bool, bool) { return getter(), true },
	)
}

// CreateDualModeStringParser creates a string parser with dual-mode support.
// In REPL mode, it properly handles GoogleSQL string literals.
// In CLI mode, it uses simple quote removal.
func CreateDualModeStringParser(name, description string, getter func() string, setter func(string) error) *DualModeVariableParser {
	return NewDualModeVariableParser(
		name,
		description,
		parser.DualModeStringParser,
		setter,
		func() (string, bool) { return getter(), true },
	)
}

// CreateDualModeIntegerParser creates an integer parser with dual-mode support.
func CreateDualModeIntegerParser(name, description string, getter func() int64, setter func(int64) error, min, max *int64) *DualModeVariableParser {
	// Create parser with validation
	var p parser.DualModeParser[int64]
	
	if min != nil || max != nil {
		// Create validators for both parsers
		googleSQLParser := parser.GoogleSQLIntParser
		simpleParser := parser.NewIntParser()
		
		if min != nil && max != nil {
			simpleParser = simpleParser.WithRange(*min, *max)
			// Add validation to GoogleSQL parser
			validateRange := func(v int64) error {
				if v < *min {
					return fmt.Errorf("value %d is less than minimum %d", v, *min)
				}
				if v > *max {
					return fmt.Errorf("value %d is greater than maximum %d", v, *max)
				}
				return nil
			}
			googleSQLParser = parser.WithValidation(googleSQLParser, validateRange)
		}
		
		p = parser.NewDualModeParser(googleSQLParser, simpleParser)
	} else {
		p = parser.DualModeIntParser
	}
	
	return NewDualModeVariableParser(
		name,
		description,
		p,
		setter,
		func() (int64, bool) { return getter(), true },
	)
}

// CreateDualModeEnumParser creates an enum parser with dual-mode support.
func CreateDualModeEnumParser[T comparable](name, description string, values map[string]T, getter func() T, setter func(T) error) *DualModeVariableParser {
	return NewDualModeVariableParser(
		name,
		description,
		parser.CreateDualModeEnumParser(values),
		setter,
		func() (T, bool) { return getter(), true },
	)
}