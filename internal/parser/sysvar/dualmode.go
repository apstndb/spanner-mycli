package sysvar

import (
	"fmt"
	"strings"

	"github.com/apstndb/spanner-mycli/internal/parser"
)

// VariableParser wraps a parser for system variables.
// It automatically selects the appropriate parsing mode based on the context.
type VariableParser struct {
	Name        string
	Description string
	parser      interface{} // Will be DualModeParser[T] for some T
	setter      interface{} // Function to set the value
	getter      interface{} // Function to get the value
	readOnly    bool
}

// NewVariableParser creates a system variable parser with dual-mode support.
func NewVariableParser[T any](
	name string,
	description string,
	parser parser.DualModeParser[T],
	setter func(value T) error,
	getter func() (T, bool),
) *VariableParser {
	return &VariableParser{
		Name:        name,
		Description: description,
		parser:      parser,
		setter:      setter,
		getter:      getter,
		readOnly:    setter == nil,
	}
}

// ParseAndSet parses a string value using the specified mode and sets it.
func (vp *VariableParser) ParseAndSetWithMode(value string, mode parser.ParseMode) error {
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

// Registry manages system variable parsers with context awareness.
type Registry struct {
	parsers map[string]*VariableParser
}

// NewRegistry creates a new variable registry.
func NewRegistry() *Registry {
	return &Registry{
		parsers: make(map[string]*VariableParser),
	}
}

// Register adds a variable parser to the registry.
func (r *Registry) Register(parser *VariableParser) error {
	upperName := strings.ToUpper(parser.Name)
	if _, exists := r.parsers[upperName]; exists {
		return fmt.Errorf("variable %s already registered", upperName)
	}
	r.parsers[upperName] = parser
	return nil
}

// SetFromGoogleSQL parses and sets a value using GoogleSQL syntax.
// This is used for SET statements in REPL and SQL scripts.
func (r *Registry) SetFromGoogleSQL(name, value string) error {
	return r.setWithMode(name, value, parser.ParseModeGoogleSQL)
}

// SetFromSimple parses and sets a value using simple syntax.
// This is used for CLI flags and config files.
func (r *Registry) SetFromSimple(name, value string) error {
	return r.setWithMode(name, value, parser.ParseModeSimple)
}

func (r *Registry) setWithMode(name, value string, mode parser.ParseMode) error {
	upperName := strings.ToUpper(name)
	p, exists := r.parsers[upperName]
	if !exists {
		return fmt.Errorf("unknown variable: %s", name)
	}

	return p.ParseAndSetWithMode(value, mode)
}

// Example helper functions for creating dual-mode parsers

// CreateBooleanParser creates a boolean parser with dual-mode support.
func CreateBooleanParser(name, description string, getter func() bool, setter func(bool) error) *VariableParser {
	return NewVariableParser(
		name,
		description,
		parser.DualModeBoolParser,
		setter,
		func() (bool, bool) { return getter(), true },
	)
}

// CreateStringParser creates a string parser with dual-mode support.
// In GoogleSQL mode, it properly handles GoogleSQL string literals.
// In Simple mode, it preserves the value as-is.
func CreateStringParser(name, description string, getter func() string, setter func(string) error) *VariableParser {
	return NewVariableParser(
		name,
		description,
		parser.DualModeStringParser,
		setter,
		func() (string, bool) { return getter(), true },
	)
}

// CreateIntegerParser creates an integer parser with dual-mode support.
func CreateIntegerParser(name, description string, getter func() int64, setter func(int64) error, min, max *int64) *VariableParser {
	// Create parser with validation
	var p parser.DualModeParser[int64]

	if min != nil || max != nil {
		// Create simple parser with validation
		simpleParser := parser.NewIntParser()

		if min != nil && max != nil {
			simpleParser = simpleParser.WithRange(*min, *max)
		} else if min != nil {
			simpleParser = simpleParser.WithMin(*min)
		} else if max != nil {
			simpleParser = simpleParser.WithMax(*max)
		}

		// Create GoogleSQL parser with same validation
		var googleSQLParser parser.Parser[int64]
		if min != nil && max != nil {
			googleSQLParser = parser.WithValidation(parser.GoogleSQLIntParser, func(v int64) error {
				if v < *min {
					return fmt.Errorf("value %d is less than minimum %d", v, *min)
				}
				if v > *max {
					return fmt.Errorf("value %d is greater than maximum %d", v, *max)
				}
				return nil
			})
		} else if min != nil {
			googleSQLParser = parser.WithValidation(parser.GoogleSQLIntParser, func(v int64) error {
				if v < *min {
					return fmt.Errorf("value %d is less than minimum %d", v, *min)
				}
				return nil
			})
		} else if max != nil {
			googleSQLParser = parser.WithValidation(parser.GoogleSQLIntParser, func(v int64) error {
				if v > *max {
					return fmt.Errorf("value %d is greater than maximum %d", v, *max)
				}
				return nil
			})
		} else {
			googleSQLParser = parser.GoogleSQLIntParser
		}

		p = parser.NewDualModeParser(googleSQLParser, simpleParser)
	} else {
		p = parser.DualModeIntParser
	}

	return NewVariableParser(
		name,
		description,
		p,
		setter,
		func() (int64, bool) { return getter(), true },
	)
}

// CreateEnumParser creates an enum parser with dual-mode support.
func CreateEnumParser[T comparable](name, description string, values map[string]T, getter func() T, setter func(T) error) *VariableParser {
	return NewVariableParser(
		name,
		description,
		parser.CreateDualModeEnumParser(values),
		setter,
		func() (T, bool) { return getter(), true },
	)
}
