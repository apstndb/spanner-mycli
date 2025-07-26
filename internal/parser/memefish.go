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
	case *ast.Ident:
		// Allow unquoted identifiers as strings
		return lit.Name, nil
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
	switch lit := expr.(type) {
	case *ast.IntLiteral:
		// Parse the string value based on the base
		if lit.Base == 16 {
			return strconv.ParseInt(lit.Value, 16, 64)
		}
		return strconv.ParseInt(lit.Value, 10, 64)
	case *ast.UnaryExpr:
		// Handle negative numbers
		if lit.Op == ast.OpMinus {
			if intLit, ok := lit.Expr.(*ast.IntLiteral); ok {
				var val int64
				var err error
				if intLit.Base == 16 {
					val, err = strconv.ParseInt(intLit.Value, 16, 64)
				} else {
					val, err = strconv.ParseInt(intLit.Value, 10, 64)
				}
				if err != nil {
					return 0, err
				}
				return -val, nil
			}
		}
		return 0, fmt.Errorf("expected integer literal, got unary expression %s", lit.SQL())
	default:
		return 0, fmt.Errorf("expected integer literal, got %T", expr)
	}
})

// GoogleSQLFloatParser parses GoogleSQL float literals.
var GoogleSQLFloatParser = NewMemefishLiteralParser(func(expr ast.Expr) (float64, error) {
	switch lit := expr.(type) {
	case *ast.FloatLiteral:
		return strconv.ParseFloat(lit.Value, 64)
	case *ast.IntLiteral:
		// Allow integers as floats
		var intVal int64
		var err error
		if lit.Base == 16 {
			intVal, err = strconv.ParseInt(lit.Value, 16, 64)
		} else {
			intVal, err = strconv.ParseInt(lit.Value, 10, 64)
		}
		if err != nil {
			return 0, err
		}
		return float64(intVal), nil
	case *ast.UnaryExpr:
		// Handle negative numbers
		if lit.Op == ast.OpMinus {
			switch innerLit := lit.Expr.(type) {
			case *ast.FloatLiteral:
				val, err := strconv.ParseFloat(innerLit.Value, 64)
				if err != nil {
					return 0, err
				}
				return -val, nil
			case *ast.IntLiteral:
				var intVal int64
				var err error
				if innerLit.Base == 16 {
					intVal, err = strconv.ParseInt(innerLit.Value, 16, 64)
				} else {
					intVal, err = strconv.ParseInt(innerLit.Value, 10, 64)
				}
				if err != nil {
					return 0, err
				}
				return -float64(intVal), nil
			}
		}
		return 0, fmt.Errorf("expected numeric literal, got unary expression %s", lit.SQL())
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

// CompatibleStringParser provides a parser that accepts both simple strings
// and GoogleSQL string literals.
type CompatibleStringParser struct {
	BaseParser[string]
	googleSQLParser *MemefishExprParser[string]
	simpleParser    *StringParser
}

// NewCompatibleStringParser creates a parser that tries simple parsing first,
// then falls back to GoogleSQL parsing.
func NewCompatibleStringParser() *CompatibleStringParser {
	parser := &CompatibleStringParser{
		googleSQLParser: GoogleSQLStringParser,
		simpleParser:    NewStringParser(),
	}
	
	parser.BaseParser = BaseParser[string]{
		ParseFunc: parser.parse,
	}
	
	return parser
}

func (p *CompatibleStringParser) parse(value string) (string, error) {
	// First try simple parsing for common cases
	trimmed := strings.TrimSpace(value)
	
	// If it doesn't look like a GoogleSQL literal, use simple parser
	if !strings.HasPrefix(trimmed, "'") && !strings.HasPrefix(trimmed, "\"") &&
		!strings.HasPrefix(trimmed, "r'") && !strings.HasPrefix(trimmed, "r\"") &&
		!strings.HasPrefix(trimmed, "b'") && !strings.HasPrefix(trimmed, "b\"") {
		return p.simpleParser.Parse(value)
	}
	
	// Otherwise use GoogleSQL parser
	return p.googleSQLParser.Parse(value)
}

// UnquoteGoogleSQLString removes GoogleSQL string literal quotes and handles escape sequences.
// This is useful when you need to process values that might be GoogleSQL string literals.
func UnquoteGoogleSQLString(s string) string {
	trimmed := strings.TrimSpace(s)
	
	// Try parsing as GoogleSQL expression
	if expr, err := memefish.ParseExpr("", trimmed); err == nil {
		if strLit, ok := expr.(*ast.StringLiteral); ok {
			return strLit.Value
		}
		if ident, ok := expr.(*ast.Ident); ok {
			return ident.Name
		}
	}
	
	// Fallback to simple unquoting
	if len(trimmed) >= 2 {
		if (trimmed[0] == '"' && trimmed[len(trimmed)-1] == '"') ||
			(trimmed[0] == '\'' && trimmed[len(trimmed)-1] == '\'') {
			// Handle basic escape sequences
			unquoted := trimmed[1 : len(trimmed)-1]
			unquoted = strings.ReplaceAll(unquoted, `\'`, `'`)
			unquoted = strings.ReplaceAll(unquoted, `\"`, `"`)
			unquoted = strings.ReplaceAll(unquoted, `\\`, `\`)
			unquoted = strings.ReplaceAll(unquoted, `\n`, "\n")
			unquoted = strings.ReplaceAll(unquoted, `\r`, "\r")
			unquoted = strings.ReplaceAll(unquoted, `\t`, "\t")
			
			// Handle Unicode escape sequences \uXXXX and \UXXXXXXXX
			if strings.Contains(unquoted, `\u`) || strings.Contains(unquoted, `\U`) {
				if unquotedStr, err := strconv.Unquote(`"` + unquoted + `"`); err == nil {
					return unquotedStr
				}
			}
			
			return unquoted
		}
	}
	
	return trimmed
}