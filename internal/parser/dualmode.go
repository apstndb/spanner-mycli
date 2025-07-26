// Package parser provides dual-mode parsing support for system variables
// that can accept values from both GoogleSQL SET statements and CLI/config sources.
package parser

import (
	"fmt"
)

// ParseMode indicates the source of the value being parsed.
type ParseMode int

const (
	// ParseModeGoogleSQL indicates the value comes from a SET statement in REPL
	// and should follow GoogleSQL lexical structure.
	ParseModeGoogleSQL ParseMode = iota

	// ParseModeSimple indicates the value comes from CLI flags or config files
	// and uses simple string parsing without GoogleSQL rules.
	ParseModeSimple
)

// DualModeParser can parse values in both GoogleSQL and simple modes.
// This is essential for system variables that need to accept values from
// different sources with different parsing rules.
type DualModeParser[T any] interface {
	Parser[T]

	// ParseWithMode parses a value using the specified mode.
	ParseWithMode(value string, mode ParseMode) (T, error)

	// ParseAndValidateWithMode combines parsing and validation for a specific mode.
	ParseAndValidateWithMode(value string, mode ParseMode) (T, error)
}

// BaseDualModeParser provides a foundation for dual-mode parsers.
type BaseDualModeParser[T any] struct {
	BaseParser[T]
	googleSQLParser Parser[T]
	simpleParser    Parser[T]
}

// NewDualModeParser creates a parser that supports both parsing modes.
func NewDualModeParser[T any](googleSQLParser, simpleParser Parser[T]) *BaseDualModeParser[T] {
	p := &BaseDualModeParser[T]{
		googleSQLParser: googleSQLParser,
		simpleParser:    simpleParser,
	}

	// Default Parse uses GoogleSQL mode for backward compatibility
	p.BaseParser = BaseParser[T]{
		ParseFunc: func(value string) (T, error) {
			return p.ParseWithMode(value, ParseModeGoogleSQL)
		},
		ValidateFunc: p.validate,
	}

	return p
}

// ParseWithMode implements DualModeParser interface.
func (p *BaseDualModeParser[T]) ParseWithMode(value string, mode ParseMode) (T, error) {
	switch mode {
	case ParseModeGoogleSQL:
		if p.googleSQLParser == nil {
			var zero T
			return zero, fmt.Errorf("GoogleSQL parser not configured")
		}
		return p.googleSQLParser.Parse(value)

	case ParseModeSimple:
		if p.simpleParser == nil {
			var zero T
			return zero, fmt.Errorf("simple parser not configured")
		}
		return p.simpleParser.Parse(value)

	default:
		var zero T
		return zero, fmt.Errorf("unknown parse mode: %v", mode)
	}
}

// ParseAndValidateWithMode implements DualModeParser interface.
func (p *BaseDualModeParser[T]) ParseAndValidateWithMode(value string, mode ParseMode) (T, error) {
	parsed, err := p.ParseWithMode(value, mode)
	if err != nil {
		var zero T
		return zero, err
	}

	if err := p.validate(parsed); err != nil {
		var zero T
		return zero, err
	}

	return parsed, nil
}

// validate runs validation using the appropriate parser's validator.
func (p *BaseDualModeParser[T]) validate(value T) error {
	// Try GoogleSQL parser validation first
	if p.googleSQLParser != nil {
		if err := p.googleSQLParser.Validate(value); err != nil {
			return err
		}
	}

	// Also run simple parser validation if different
	if p.simpleParser != nil && p.simpleParser != p.googleSQLParser {
		if err := p.simpleParser.Validate(value); err != nil {
			return err
		}
	}

	return nil
}

// DualModeBoolParser parses boolean values in both modes.
var DualModeBoolParser = NewDualModeParser(
	GoogleSQLBoolParser,
	NewBoolParser(),
)

// DualModeIntParser parses integer values in both modes.
var DualModeIntParser = NewDualModeParser(
	GoogleSQLIntParser,
	NewIntParser(),
)

// DualModeStringParser parses string values in both modes.
// In GoogleSQL mode, it properly handles SQL string literals.
// In simple mode, it preserves the value as-is.
var DualModeStringParser = NewDualModeParser(
	GoogleSQLStringLiteralParser,
	NewStringParser(),
)

// CreateDualModeEnumParser creates an enum parser that works in both modes.
func CreateDualModeEnumParser[T comparable](values map[string]T) *BaseDualModeParser[T] {
	return NewDualModeParser(
		NewGoogleSQLEnumParser(values),
		NewEnumParser(values),
	)
}
