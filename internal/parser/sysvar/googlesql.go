// googlesql.go - GoogleSQL parsing and dual-mode support
//
// This file contains all GoogleSQL-related parsing functionality,
// including memefish integration and dual-mode parsers that switch
// between GoogleSQL and simple parsing modes.

package sysvar

import (
	"fmt"
	"strconv"
	"time"

	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/ast"
)

// ============================================================================
// Memefish-based GoogleSQL Parsers
// ============================================================================

// memefishExprParser parses values using memefish to ensure GoogleSQL compatibility.
// It parses expressions according to GoogleSQL syntax rules and extracts typed values.
//
// This parser is used for SET statements in the REPL where values must follow
// GoogleSQL lexical structure:
//   - Strings: 'single quotes', "double quotes", r'raw strings'
//   - Numbers: 123, -456, 3.14, 1e10
//   - Booleans: TRUE, FALSE (parsed as BoolLiteral, not identifiers)
//   - NULL: NULL (parsed as NullLiteral)
type memefishExprParser[T any] struct {
	baseParser[T]
	extractFunc func(ast.Expr) (T, error)
}

// newMemefishLiteralParser creates a parser that uses memefish to parse
// GoogleSQL-compatible literals and converts them to Go types.
func newMemefishLiteralParser[T any](extractFunc func(ast.Expr) (T, error)) *memefishExprParser[T] {
	parser := &memefishExprParser[T]{
		extractFunc: extractFunc,
	}

	parser.baseParser = baseParser[T]{
		ParseFunc: parser.parseWithMemefish,
	}

	return parser
}

func (p *memefishExprParser[T]) parseWithMemefish(value string) (T, error) {
	// Parse as a GoogleSQL expression
	expr, err := memefish.ParseExpr("", value)
	if err != nil {
		var zero T
		return zero, fmt.Errorf("invalid GoogleSQL expression: %w", err)
	}

	return p.extractFunc(expr)
}

// googleSQLStringParser is an alias for googleSQLStringLiteralParser for backward compatibility.
var googleSQLStringParser = googleSQLStringLiteralParser

// GoogleSQLBoolParser parses GoogleSQL boolean literals.
// TRUE/FALSE are parsed as BoolLiteral by memefish, not as Ident.
var GoogleSQLBoolParser = newMemefishLiteralParser(func(expr ast.Expr) (bool, error) {
	if lit, ok := expr.(*ast.BoolLiteral); ok {
		return lit.Value, nil
	}
	return false, fmt.Errorf("expected boolean literal, got %T", expr)
})

// GoogleSQLIntParser parses GoogleSQL integer literals.
var GoogleSQLIntParser = newMemefishLiteralParser(func(expr ast.Expr) (int64, error) {
	intLit, ok := expr.(*ast.IntLiteral)
	if !ok {
		return 0, fmt.Errorf("expected integer literal, got %T", expr)
	}

	// When base is not 0, strconv.ParseInt expects the number without prefix.
	// memefish includes the 0x/0X prefix in the Value field for base 16.
	value := intLit.Value
	if intLit.Base == 16 && len(value) > 2 && (value[:2] == "0x" || value[:2] == "0X") {
		value = value[2:]
	}
	if intLit.Base == 16 && len(value) > 3 && value[0] == '-' && (value[1:3] == "0x" || value[1:3] == "0X") {
		value = "-" + value[3:]
	}

	return strconv.ParseInt(value, intLit.Base, 64)
})

// stringLiteralParser parses GoogleSQL string literals efficiently using the lexer.
type stringLiteralParser struct {
	baseParser[string]
}

// newStringLiteralParser creates a parser that uses memefish lexer for string literals.
func newStringLiteralParser() *stringLiteralParser {
	parser := &stringLiteralParser{}

	parser.baseParser = baseParser[string]{
		ParseFunc: parseGoogleSQLStringLiteral,
	}

	return parser
}

// googleSQLStringLiteralParser is an optimized parser using the lexer directly.
var googleSQLStringLiteralParser = newStringLiteralParser()

// parseGoogleSQLStringLiteral parses a GoogleSQL string literal using memefish.
// It returns an error if the input is not a valid string literal.
func parseGoogleSQLStringLiteral(s string) (result string, err error) {
	// Recover from panics that memefish might throw on invalid syntax
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("invalid string literal: %v", r)
		}
	}()

	// Parse as expression
	expr, err := memefish.ParseExpr("", s)
	if err != nil {
		return "", fmt.Errorf("invalid string literal: %w", err)
	}

	// Expect a string literal
	switch lit := expr.(type) {
	case *ast.StringLiteral:
		return lit.Value, nil
	case *ast.BytesLiteral:
		// Convert bytes literal to string
		return string(lit.Value), nil
	default:
		return "", fmt.Errorf("expected string literal, got %T", expr)
	}
}

// ============================================================================
// Dual-Mode Parser Support
// ============================================================================

// parseMode represents the parsing mode for system variables.
type parseMode int

const (
	// ParseModeGoogleSQL represents GoogleSQL parsing mode (for REPL SET statements).
	ParseModeGoogleSQL parseMode = iota
	// ParseModeSimple represents simple parsing mode (for CLI flags and config files).
	ParseModeSimple
)

// DualModeParser is the interface for parsers that support both GoogleSQL and simple modes.
// This is the primary parser type used throughout the system variable framework.
type DualModeParser[T any] interface {
	parser[T]
	// ParseWithMode parses a value using the specified mode.
	ParseWithMode(value string, mode parseMode) (T, error)
	// ParseAndValidateWithMode parses and validates a value using the specified mode.
	ParseAndValidateWithMode(value string, mode parseMode) (T, error)
}

// baseDualModeParser provides a foundation for dual-mode parsers.
type baseDualModeParser[T any] struct {
	baseParser[T]
	googleSQLParser parser[T]
	simpleParser    parser[T]
}

// newDualModeParser creates a parser that supports both parsing modes.
func newDualModeParser[T any](googleSQLParser, simpleParser parser[T]) *baseDualModeParser[T] {
	p := &baseDualModeParser[T]{
		googleSQLParser: googleSQLParser,
		simpleParser:    simpleParser,
	}

	// Default Parse uses GoogleSQL mode for backward compatibility
	p.baseParser = baseParser[T]{
		ParseFunc: func(value string) (T, error) {
			return p.ParseWithMode(value, ParseModeGoogleSQL)
		},
		ValidateFunc: p.validate,
	}

	return p
}

// ParseWithMode implements DualModeParser interface.
func (p *baseDualModeParser[T]) ParseWithMode(value string, mode parseMode) (T, error) {
	switch mode {
	case ParseModeGoogleSQL:
		// GoogleSQL mode may have specific error handling
		result, err := p.googleSQLParser.Parse(value)
		if err != nil {
			var zero T
			// Wrap error to indicate it's from GoogleSQL parsing
			return zero, fmt.Errorf("GoogleSQL parse error: %w", err)
		}
		return result, nil
	case ParseModeSimple:
		return p.simpleParser.Parse(value)
	default:
		var zero T
		return zero, fmt.Errorf("unknown parse mode: %v", mode)
	}
}

// ParseAndValidateWithMode implements DualModeParser interface.
func (p *baseDualModeParser[T]) ParseAndValidateWithMode(value string, mode parseMode) (T, error) {
	// Parse using the specified mode
	parsed, err := p.ParseWithMode(value, mode)
	if err != nil {
		var zero T
		return zero, err
	}

	// Validate the parsed value
	if err := p.validate(parsed); err != nil {
		var zero T
		return zero, err
	}

	return parsed, nil
}

// validate runs validation for both parsers if they have validation.
func (p *baseDualModeParser[T]) validate(value T) error {
	// Run GoogleSQL parser's validation if it exists
	if err := p.googleSQLParser.Validate(value); err != nil {
		return err
	}
	// Also run simple parser's validation
	// This ensures consistent validation across both modes
	return p.simpleParser.Validate(value)
}

// ============================================================================
// Pre-defined Dual-Mode Parsers
// ============================================================================

// DualModeBoolParser parses boolean values in both modes.
// In GoogleSQL mode, it accepts only TRUE/FALSE literals.
// In simple mode, it uses strconv.ParseBool for flexibility.
var DualModeBoolParser = newDualModeParser(
	GoogleSQLBoolParser,
	newBoolParser(),
)

// DualModeIntParser parses integer values in both modes.
// Both modes support the same integer parsing logic.
var DualModeIntParser = newDualModeParser(
	GoogleSQLIntParser,
	newIntParser(),
)

// DualModeStringParser parses string values in both modes.
// In GoogleSQL mode, it properly handles SQL string literals.
// In simple mode, it preserves the value as-is.
var DualModeStringParser = newDualModeParser(
	googleSQLStringLiteralParser,
	newStringParser(),
)

// newDelegatingGoogleSQLParser creates a GoogleSQL parser that extracts string literals
// and delegates the actual parsing to another parser. This is useful for string-based
// parsers that need GoogleSQL literal handling but have their own parsing logic.
//
// Note: This approach doesn't work for enum parsers because GoogleSQL enum values can be
// either string literals ('VALUE') or identifiers (VALUE), requiring special handling.
func newDelegatingGoogleSQLParser[T any](delegateParser parser[T]) parser[T] {
	return &baseParser[T]{
		ParseFunc: func(value string) (T, error) {
			// First extract the string literal using GoogleSQL parser
			str, err := googleSQLStringParser.Parse(value)
			if err != nil {
				var zero T
				return zero, err
			}
			// Then delegate to the actual parser
			return delegateParser.Parse(str)
		},
	}
}

// createDualModeEnumParser creates an enum parser that works in both modes.
// In GoogleSQL mode, it extracts string literals and delegates to the enum parser.
// In simple mode, it uses the enum parser directly.
// This ensures consistent enum value parsing between both modes.
func createDualModeEnumParser[T comparable](values map[string]T) *baseDualModeParser[T] {
	enumParser := newEnumParser(values)
	return newDualModeParser(
		newDelegatingGoogleSQLParser(enumParser),
		enumParser,
	)
}

// googleSQLDurationParser parses duration from GoogleSQL string literals.
// It extracts the string literal and delegates to the standard duration parser.
var googleSQLDurationParser = newDelegatingGoogleSQLParser(newDurationParser())

// dualModeDurationParser provides dual-mode parsing for duration values.
var dualModeDurationParser = newDualModeParser[time.Duration](
	googleSQLDurationParser,
	newDurationParser(),
)
