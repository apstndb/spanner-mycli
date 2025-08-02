package sysvar

import (
	"fmt"
	"strings"
)

// appendableVariableParser extends VariableParser to support append operations.
// This is used for variables that can accumulate values rather than just replacing them.
type appendableVariableParser interface {
	VariableParser
	// AppendWithMode appends a value using the specified parse mode.
	AppendWithMode(value string, mode parseMode) error
}

// Registry extensions for append support.

// AppendFromGoogleSQL appends a value using GoogleSQL syntax.
func (r *Registry) AppendFromGoogleSQL(name, value string) error {
	return r.appendWithMode(name, value, ParseModeGoogleSQL)
}

// AppendFromSimple appends a value using simple syntax.
func (r *Registry) AppendFromSimple(name, value string) error {
	return r.appendWithMode(name, value, ParseModeSimple)
}

func (r *Registry) appendWithMode(name, value string, mode parseMode) error {
	upperName := strings.ToUpper(name)
	p, exists := r.parsers[upperName]
	if !exists {
		return fmt.Errorf("unknown variable: %s", name)
	}

	// Check if the parser supports append
	appendable, ok := p.(appendableVariableParser)
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
	_, ok := p.(appendableVariableParser)
	return ok
}
