// Package sysvar provides integration between the generic parser framework
// and system variables, enabling type-safe parsing and validation.
package sysvar

import (
	"fmt"
	"strings"
	
	"github.com/apstndb/spanner-mycli/internal/parser"
)

// VariableParser wraps a generic parser to work with system variables.
// It handles the conversion between string values and typed system variable fields.
type VariableParser struct {
	Name        string
	Description string
	parser      interface{} // Will be Parser[T] for some T
	setter      interface{} // Function to set the value
	getter      interface{} // Function to get the value
	readOnly    bool
}

// NewVariableParser creates a new system variable parser.
func NewVariableParser[T any](
	name string,
	description string,
	parser parser.Parser[T],
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

// ParseAndSet parses a string value and sets it in the system variable.
func (vp *VariableParser) ParseAndSet(value string) error {
	if vp.readOnly {
		return fmt.Errorf("variable %s is read-only", vp.Name)
	}
	
	// Type assertion to get the concrete parser type
	switch p := vp.parser.(type) {
	case parser.Parser[bool]:
		if setter, ok := vp.setter.(func(bool) error); ok {
			parsed, err := p.ParseAndValidate(value)
			if err != nil {
				return fmt.Errorf("invalid value for %s: %w", vp.Name, err)
			}
			return setter(parsed)
		}
	case parser.Parser[int64]:
		if setter, ok := vp.setter.(func(int64) error); ok {
			parsed, err := p.ParseAndValidate(value)
			if err != nil {
				return fmt.Errorf("invalid value for %s: %w", vp.Name, err)
			}
			return setter(parsed)
		}
	case parser.Parser[string]:
		if setter, ok := vp.setter.(func(string) error); ok {
			parsed, err := p.ParseAndValidate(value)
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

// GetValue returns the current value of the system variable as a string.
func (vp *VariableParser) GetValue() (string, bool) {
	// Type assertion to get the concrete getter type
	switch g := vp.getter.(type) {
	case func() (bool, bool):
		if value, exists := g(); exists {
			return strings.ToUpper(fmt.Sprintf("%v", value)), true
		}
	case func() (int64, bool):
		if value, exists := g(); exists {
			return fmt.Sprintf("%d", value), true
		}
	case func() (string, bool):
		if value, exists := g(); exists {
			return value, true
		}
	default:
		// For other types, use reflection or add more cases as needed
		return "", false
	}
	
	return "", false
}

// Registry manages all system variable parsers.
type Registry struct {
	parsers map[string]*VariableParser
}

// NewRegistry creates a new system variable parser registry.
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

// Set parses and sets a system variable value.
func (r *Registry) Set(name, value string) error {
	upperName := strings.ToUpper(name)
	parser, exists := r.parsers[upperName]
	if !exists {
		return fmt.Errorf("unknown variable: %s", name)
	}
	
	return parser.ParseAndSet(value)
}

// Get returns the current value of a system variable.
func (r *Registry) Get(name string) (string, error) {
	upperName := strings.ToUpper(name)
	parser, exists := r.parsers[upperName]
	if !exists {
		return "", fmt.Errorf("unknown variable: %s", name)
	}
	
	value, ok := parser.GetValue()
	if !ok {
		return "", fmt.Errorf("variable %s has no value", name)
	}
	
	return value, nil
}

// List returns all registered variable names and descriptions.
func (r *Registry) List() map[string]string {
	result := make(map[string]string)
	for name, parser := range r.parsers {
		result[name] = parser.Description
	}
	return result
}

// CreateBooleanParser is a helper to create a parser for boolean system variables.
func CreateBooleanParser(name, description string, getter func() bool, setter func(bool) error) *VariableParser {
	return NewVariableParser(
		name,
		description,
		parser.NewBoolParser(),
		setter,
		func() (bool, bool) { return getter(), true },
	)
}

// CreateIntegerParser is a helper to create a parser for integer system variables.
func CreateIntegerParser(name, description string, getter func() int64, setter func(int64) error, min, max *int64) *VariableParser {
	p := parser.NewIntParser()
	if min != nil && max != nil {
		p.WithRange(*min, *max)
	} else if min != nil {
		p.WithMin(*min)
	} else if max != nil {
		p.WithMax(*max)
	}
	
	return NewVariableParser(
		name,
		description,
		p,
		setter,
		func() (int64, bool) { return getter(), true },
	)
}

// CreateStringParser is a helper to create a parser for string system variables.
func CreateStringParser(name, description string, getter func() string, setter func(string) error) *VariableParser {
	return NewVariableParser(
		name,
		description,
		parser.NewStringParser(),
		setter,
		func() (string, bool) { return getter(), true },
	)
}

// CreateEnumParser is a helper to create a parser for enum system variables.
func CreateEnumParser[T comparable](name, description string, values map[string]T, getter func() T, setter func(T) error) *VariableParser {
	return NewVariableParser(
		name,
		description,
		parser.NewEnumParser(values),
		setter,
		func() (T, bool) { return getter(), true },
	)
}