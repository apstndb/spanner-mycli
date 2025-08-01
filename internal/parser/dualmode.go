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
//
// Example usage:
//
//	// In REPL (GoogleSQL mode):
//	SET OPTIMIZER_VERSION = 'LATEST';  // String must be quoted
//	SET TAB_WIDTH = 4;                 // Numbers are literals
//
//	// In CLI flags (Simple mode):
//	--set OPTIMIZER_VERSION=LATEST     // No quotes needed
//	--set TAB_WIDTH=4                  // Same as GoogleSQL for numbers
//
// The dual-mode parser ensures correct parsing based on the value source.
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

// NewDelegatingGoogleSQLParser creates a GoogleSQL parser that extracts string literals
// and delegates the actual parsing to another parser. This is useful for string-based
// parsers that need GoogleSQL literal handling but have their own parsing logic.
//
// Note: This approach doesn't work for enum parsers because GoogleSQL enum values can be
// either string literals ('VALUE') or identifiers (VALUE), requiring special handling.
func NewDelegatingGoogleSQLParser[T any](delegateParser Parser[T]) Parser[T] {
	return &BaseParser[T]{
		ParseFunc: func(value string) (T, error) {
			// First extract the string literal using GoogleSQL parser
			str, err := GoogleSQLStringParser.Parse(value)
			if err != nil {
				var zero T
				return zero, err
			}
			// Then delegate to the actual parser
			return delegateParser.Parse(str)
		},
	}
}

// CreateDualModeEnumParser creates an enum parser that works in both modes.
// In GoogleSQL mode, it extracts string literals and delegates to the enum parser.
// In simple mode, it uses the enum parser directly.
// This ensures consistent enum value parsing between both modes.
func CreateDualModeEnumParser[T comparable](values map[string]T) *BaseDualModeParser[T] {
	enumParser := NewEnumParser(values)
	return NewDualModeParser(
		NewDelegatingGoogleSQLParser(enumParser),
		enumParser,
	)
}
