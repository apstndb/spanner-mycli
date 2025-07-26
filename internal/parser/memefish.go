package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/ast"
)

// MemefishExprParser parses values using memefish to ensure GoogleSQL compatibility.
// It can parse expressions and extract literal values.
type MemefishExprParser[T any] struct {
	BaseParser[T]
	extractFunc func(ast.Expr) (T, error)
}

// NewMemefishLiteralParser creates a parser that uses memefish to parse
// GoogleSQL-compatible literals and converts them to Go types.
func NewMemefishLiteralParser[T any](extractFunc func(ast.Expr) (T, error)) *MemefishExprParser[T] {
	parser := &MemefishExprParser[T]{
		extractFunc: extractFunc,
	}

	parser.BaseParser = BaseParser[T]{
		ParseFunc: parser.parseWithMemefish,
	}

	return parser
}

func (p *MemefishExprParser[T]) parseWithMemefish(value string) (T, error) {
	// Parse as a GoogleSQL expression
	expr, err := memefish.ParseExpr("", value)
	if err != nil {
		var zero T
		return zero, fmt.Errorf("invalid GoogleSQL expression: %w", err)
	}

	return p.extractFunc(expr)
}

// GoogleSQLStringParser parses GoogleSQL string literals.
var GoogleSQLStringParser = NewMemefishLiteralParser(func(expr ast.Expr) (string, error) {
	switch lit := expr.(type) {
	case *ast.StringLiteral:
		return lit.Value, nil
	case *ast.BytesLiteral:
		// Convert bytes literal to string
		return string(lit.Value), nil
	default:
		return "", fmt.Errorf("expected string literal, got %T", expr)
	}
})

// GoogleSQLBoolParser parses GoogleSQL boolean literals.
var GoogleSQLBoolParser = NewMemefishLiteralParser(func(expr ast.Expr) (bool, error) {
	switch lit := expr.(type) {
	case *ast.BoolLiteral:
		return lit.Value, nil
	case *ast.Ident:
		// Handle TRUE/FALSE identifiers
		switch strings.ToUpper(lit.Name) {
		case "TRUE":
			return true, nil
		case "FALSE":
			return false, nil
		default:
			return false, fmt.Errorf("expected boolean literal or TRUE/FALSE, got %q", lit.Name)
		}
	default:
		return false, fmt.Errorf("expected boolean literal, got %T", expr)
	}
})

// GoogleSQLIntParser parses GoogleSQL integer literals.
var GoogleSQLIntParser = NewMemefishLiteralParser(func(expr ast.Expr) (int64, error) {
	intLit, ok := expr.(*ast.IntLiteral)
	if !ok {
		return 0, fmt.Errorf("expected integer literal, got %T", expr)
	}

	// When base is not 0, strconv.ParseInt doesn't handle the 0x prefix
	// So we need to strip it for base 16
	value := intLit.Value
	if intLit.Base == 16 && len(value) > 2 && (value[:2] == "0x" || value[:2] == "0X") {
		value = value[2:]
	}
	if intLit.Base == 16 && len(value) > 3 && value[0] == '-' && (value[1:3] == "0x" || value[1:3] == "0X") {
		value = "-" + value[3:]
	}

	return strconv.ParseInt(value, intLit.Base, 64)
})

// GoogleSQLFloatParser parses GoogleSQL float literals.
var GoogleSQLFloatParser = NewMemefishLiteralParser(func(expr ast.Expr) (float64, error) {
	switch lit := expr.(type) {
	case *ast.FloatLiteral:
		return strconv.ParseFloat(lit.Value, 64)
	case *ast.IntLiteral:
		// Allow integers as floats - reuse the int parser logic
		intVal, err := GoogleSQLIntParser.Parse(expr.SQL())
		if err != nil {
			return 0, err
		}
		return float64(intVal), nil
	default:
		return 0, fmt.Errorf("expected numeric literal, got %T", expr)
	}
})

// GoogleSQLEnumParser parses enum values using GoogleSQL string/identifier syntax.
func NewGoogleSQLEnumParser[T comparable](values map[string]T) *MemefishExprParser[T] {
	return NewMemefishLiteralParser(func(expr ast.Expr) (T, error) {
		var strValue string

		switch lit := expr.(type) {
		case *ast.StringLiteral:
			strValue = lit.Value
		case *ast.Ident:
			strValue = lit.Name
		default:
			var zero T
			return zero, fmt.Errorf("expected string literal or identifier for enum value, got %T", expr)
		}

		// Try case-insensitive match first
		upperValue := strings.ToUpper(strValue)
		for k, v := range values {
			if strings.ToUpper(k) == upperValue {
				return v, nil
			}
		}

		// Build error message
		var validValues []string
		for k := range values {
			validValues = append(validValues, k)
		}

		var zero T
		return zero, fmt.Errorf("invalid enum value %q, must be one of: %s", strValue, strings.Join(validValues, ", "))
	})
}

// StringLiteralParser parses GoogleSQL string literals efficiently using the lexer.
type StringLiteralParser struct {
	BaseParser[string]
}

// NewStringLiteralParser creates a parser that uses memefish lexer for string literals.
func NewStringLiteralParser() *StringLiteralParser {
	parser := &StringLiteralParser{}

	parser.BaseParser = BaseParser[string]{
		ParseFunc: ParseGoogleSQLStringLiteral,
	}

	return parser
}

// GoogleSQLStringLiteralParser is an optimized parser using the lexer directly.
var GoogleSQLStringLiteralParser = NewStringLiteralParser()

// ParseGoogleSQLStringLiteral parses a GoogleSQL string literal using memefish.
// It returns an error if the input is not a valid string literal.
func ParseGoogleSQLStringLiteral(s string) (result string, err error) {
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
