package sysvar

import (
	"fmt"
	"strings"

	"github.com/apstndb/spanner-mycli/internal/parser"
)

// AppendableVariableParser extends VariableParser to support append operations.
// This is used for variables that can accumulate values rather than just replacing them.
type AppendableVariableParser interface {
	VariableParser
	// AppendWithMode appends a value using the specified parse mode.
	AppendWithMode(value string, mode parser.ParseMode) error
}

// AppendableTypedVariableParser extends TypedVariableParser to support append operations.
type AppendableTypedVariableParser[T any] struct {
	*TypedVariableParser[T]
	appender func(T) error // Function to append a parsed value
}

// NewAppendableStringSliceParser creates a parser for string slice variables that support append.
func NewAppendableStringSliceParser(
	name string,
	description string,
	getter func() []string,
	setter func([]string) error,
	appender func(string) error,
) AppendableVariableParser {
	// Create appendable wrapper
	return &AppendableTypedVariableParser[string]{
		TypedVariableParser: &TypedVariableParser[string]{
			name:        name,
			description: description,
			parser:      parser.DualModeStringParser,
			setter:      nil, // Individual setter not used for append
			getter:      func() string { return strings.Join(getter(), ",") },
			formatter:   func(v string) string { return v },
			readOnly:    false,
		},
		appender: appender,
	}
}

// AppendWithMode implements AppendableVariableParser.
func (avp *AppendableTypedVariableParser[T]) AppendWithMode(value string, mode parser.ParseMode) error {
	parsed, err := avp.parser.ParseAndValidateWithMode(value, mode)
	if err != nil {
		return fmt.Errorf("failed to parse value for append: %w", err)
	}
	return avp.appender(parsed)
}

// Registry extensions for append support.

// AppendFromGoogleSQL appends a value using GoogleSQL syntax.
func (r *Registry) AppendFromGoogleSQL(name, value string) error {
	return r.appendWithMode(name, value, parser.ParseModeGoogleSQL)
}

// AppendFromSimple appends a value using simple syntax.
func (r *Registry) AppendFromSimple(name, value string) error {
	return r.appendWithMode(name, value, parser.ParseModeSimple)
}

func (r *Registry) appendWithMode(name, value string, mode parser.ParseMode) error {
	upperName := strings.ToUpper(name)
	p, exists := r.parsers[upperName]
	if !exists {
		return fmt.Errorf("unknown variable: %s", name)
	}

	// Check if the parser supports append
	appendable, ok := p.(AppendableVariableParser)
	if !ok {
		return fmt.Errorf("variable %s does not support append operations", name)
	}

	return appendable.AppendWithMode(value, mode)
}

// HasAppendSupport checks if a variable supports append operations.
func (r *Registry) HasAppendSupport(name string) bool {
	upperName := strings.ToUpper(name)
	p, exists := r.parsers[upperName]
	if !exists {
		return false
	}
	_, ok := p.(AppendableVariableParser)
	return ok
}
