package parser

import (
	"strings"
	"time"
)

// NullableParser wraps a parser to handle NULL values.
type NullableParser[T any] struct {
	BaseParser[*T]
	innerParser DualModeParser[T]
}

// NewNullableParser creates a parser that accepts NULL values.
func NewNullableParser[T any](innerParser DualModeParser[T]) *NullableParser[T] {
	return &NullableParser[T]{
		innerParser: innerParser,
	}
}

// ParseAndValidateWithMode parses a value that can be NULL.
func (p *NullableParser[T]) ParseAndValidateWithMode(s string, mode ParseMode) (*T, error) {
	// Check for NULL (case-insensitive)
	trimmed := strings.TrimSpace(s)
	if strings.ToUpper(trimmed) == "NULL" {
		return nil, nil
	}
	
	// Parse as regular value
	value, err := p.innerParser.ParseAndValidateWithMode(s, mode)
	if err != nil {
		return nil, err
	}
	
	return &value, nil
}

// ParseWithMode implements DualModeParser interface.
func (p *NullableParser[T]) ParseWithMode(s string, mode ParseMode) (*T, error) {
	return p.ParseAndValidateWithMode(s, mode)
}

// ParseAndValidate implements Parser interface for simple mode.
func (p *NullableParser[T]) ParseAndValidate(s string) (*T, error) {
	return p.ParseAndValidateWithMode(s, ParseModeSimple)
}

// NewNullableDurationParser creates a nullable duration parser.
func NewNullableDurationParser(innerParser DualModeParser[time.Duration]) *NullableParser[time.Duration] {
	return NewNullableParser(innerParser)
}