package parser

import (
	"fmt"
	"strings"
)

// EnumParser parses string values into enum types.
// It supports case-insensitive matching and custom value mappings.
type EnumParser[T comparable] struct {
	BaseParser[T]
	originalValues map[string]T // Store original values for case-sensitive mode
	values         map[string]T
	caseMatters    bool
}

// NewEnumParser creates a new enum parser with the given valid values.
// By default, it performs case-insensitive matching.
func NewEnumParser[T comparable](values map[string]T) *EnumParser[T] {
	// Create case-insensitive map by default
	normalizedValues := make(map[string]T)
	for k, v := range values {
		normalizedValues[strings.ToUpper(k)] = v
	}

	parser := &EnumParser[T]{
		originalValues: values, // Store the original map
		values:         normalizedValues,
		caseMatters:    false,
	}

	parser.BaseParser = BaseParser[T]{
		ParseFunc: parser.parseEnum,
	}

	return parser
}

// CaseSensitive makes the enum parser case-sensitive.
func (p *EnumParser[T]) CaseSensitive() *EnumParser[T] {
	if !p.caseMatters {
		p.values = p.originalValues // Use the original, non-normalized map
		p.caseMatters = true
	}
	return p
}

func (p *EnumParser[T]) parseEnum(value string) (T, error) {
	trimmed := strings.TrimSpace(value)

	lookupKey := trimmed
	if !p.caseMatters {
		lookupKey = strings.ToUpper(lookupKey)
	}

	if result, ok := p.values[lookupKey]; ok {
		return result, nil
	}

	// Build error message with valid values
	var validValues []string
	for k := range p.values {
		validValues = append(validValues, k)
	}

	var zero T
	return zero, fmt.Errorf("invalid value %q, must be one of: %s", value, strings.Join(validValues, ", "))
}

// EnumStringParser is a convenience type for string enums.
type EnumStringParser = EnumParser[string]

// NewEnumStringParser creates a parser for string enum values.
func NewEnumStringParser(values ...string) *EnumStringParser {
	valueMap := make(map[string]string)
	for _, v := range values {
		valueMap[v] = v
	}
	return NewEnumParser(valueMap)
}

// EnumIntParser is a convenience type for int enums.
type EnumIntParser = EnumParser[int]

// NewEnumIntParser creates a parser for int enum values from a map.
func NewEnumIntParser(values map[string]int) *EnumIntParser {
	return NewEnumParser(values)
}
