// parser.go - Core parser interfaces and base implementations
//
// This file defines the fundamental Parser[T] interface that forms the basis
// of the generic parsing framework. It is the lowest level of the parser hierarchy
// and has no dependencies on other files in this package.
//
// The Parser[T] interface is used by:
// - types.go: Implements concrete parsers for basic types
// - enum.go: Implements enum parsing
// - nullable.go: Adds nullable support
// - memefish.go: Implements GoogleSQL parsing
// - dualmode.go: Combines parsers for different modes
// - typed_parser.go: Wraps parsers for system variable usage
package sysvar

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// parser is the core interface for parsing and validating values of type T.
// It provides a unified way to convert strings to typed values with validation.
type parser[T any] interface {
	// Parse converts a string value to type T.
	// It returns an error if the string cannot be parsed.
	Parse(value string) (T, error)

	// Validate checks if a parsed value meets additional constraints.
	// It returns an error if validation fails.
	Validate(value T) error

	// ParseAndValidate combines parsing and validation in a single step.
	// This is a convenience method that calls Parse followed by Validate.
	ParseAndValidate(value string) (T, error)
}

// baseParser provides a foundation for implementing parsers.
// It handles the common ParseAndValidate logic.
type baseParser[T any] struct {
	ParseFunc    func(string) (T, error)
	ValidateFunc func(T) error
}

// Parse implements the Parser interface.
func (p *baseParser[T]) Parse(value string) (T, error) {
	if p.ParseFunc == nil {
		var zero T
		return zero, fmt.Errorf("parse function not implemented")
	}
	return p.ParseFunc(value)
}

// Validate implements the Parser interface.
func (p *baseParser[T]) Validate(value T) error {
	if p.ValidateFunc == nil {
		return nil // No validation if not specified
	}
	return p.ValidateFunc(value)
}

// ParseAndValidate implements the Parser interface.
func (p *baseParser[T]) ParseAndValidate(value string) (T, error) {
	parsed, err := p.Parse(value)
	if err != nil {
		var zero T
		return zero, err
	}

	if err := p.Validate(parsed); err != nil {
		var zero T
		return zero, err
	}

	return parsed, nil
}

// validator is a function type for value validation.
type validator[T any] func(value T) error

// chainValidators combines multiple validators into a single validator.
// All validators must pass for the value to be considered valid.
func chainValidators[T any](validators ...validator[T]) validator[T] {
	return func(value T) error {
		for _, validator := range validators {
			if err := validator(value); err != nil {
				return err
			}
		}
		return nil
	}
}

// WithValidation wraps an existing parser with additional validation.
func WithValidation[T any](p parser[T], validators ...validator[T]) parser[T] {
	return &baseParser[T]{
		ParseFunc: p.Parse,
		ValidateFunc: func(value T) error {
			// First run the original validation
			if err := p.Validate(value); err != nil {
				return err
			}
			// Then run additional validators
			return chainValidators(validators...)(value)
		},
	}
}

// ============================================================================
// Basic Type Parsers
// ============================================================================

// BoolParser parses boolean values.
// It uses strconv.ParseBool which accepts:
// "1", "t", "T", "true", "TRUE", "True",
// "0", "f", "F", "false", "FALSE", "False".
type BoolParser struct {
	baseParser[bool]
}

// NewBoolParser creates a new boolean parser.
func NewBoolParser() *BoolParser {
	return &BoolParser{
		baseParser: baseParser[bool]{
			ParseFunc: func(value string) (bool, error) {
				return strconv.ParseBool(strings.TrimSpace(value))
			},
		},
	}
}

// IntParser parses integer values with optional range validation.
type IntParser struct {
	baseParser[int64]
	min *int64
	max *int64
}

// NewIntParser creates a new integer parser.
func NewIntParser() *IntParser {
	return &IntParser{
		baseParser: baseParser[int64]{
			ParseFunc: func(value string) (int64, error) {
				return strconv.ParseInt(strings.TrimSpace(value), 10, 64)
			},
		},
	}
}

// WithRange adds range validation to the integer parser.
func (p *IntParser) WithRange(min, max int64) *IntParser {
	p.min = &min
	p.max = &max
	p.ValidateFunc = p.validateRange
	return p
}

// WithMin adds minimum value validation.
func (p *IntParser) WithMin(min int64) *IntParser {
	p.min = &min
	p.ValidateFunc = p.validateRange
	return p
}

// WithMax adds maximum value validation.
func (p *IntParser) WithMax(max int64) *IntParser {
	p.max = &max
	p.ValidateFunc = p.validateRange
	return p
}

func (p *IntParser) validateRange(value int64) error {
	if p.min != nil && value < *p.min {
		return fmt.Errorf("value %d is less than minimum %d", value, *p.min)
	}
	if p.max != nil && value > *p.max {
		return fmt.Errorf("value %d is greater than maximum %d", value, *p.max)
	}
	return nil
}

// DurationParser parses duration values with optional range validation.
type DurationParser struct {
	baseParser[time.Duration]
	min *time.Duration
	max *time.Duration
}

// NewDurationParser creates a new duration parser.
func NewDurationParser() *DurationParser {
	return &DurationParser{
		baseParser: baseParser[time.Duration]{
			ParseFunc: func(value string) (time.Duration, error) {
				return time.ParseDuration(strings.TrimSpace(value))
			},
		},
	}
}

// WithRange adds range validation to the duration parser.
func (p *DurationParser) WithRange(min, max time.Duration) *DurationParser {
	p.min = &min
	p.max = &max
	p.ValidateFunc = p.validateRange
	return p
}

// WithMin adds minimum duration validation.
func (p *DurationParser) WithMin(min time.Duration) *DurationParser {
	p.min = &min
	p.ValidateFunc = p.validateRange
	return p
}

// WithMax adds maximum duration validation.
func (p *DurationParser) WithMax(max time.Duration) *DurationParser {
	p.max = &max
	p.ValidateFunc = p.validateRange
	return p
}

func (p *DurationParser) validateRange(value time.Duration) error {
	if p.min != nil && value < *p.min {
		return fmt.Errorf("duration %v is less than minimum %v", value, *p.min)
	}
	if p.max != nil && value > *p.max {
		return fmt.Errorf("duration %v is greater than maximum %v", value, *p.max)
	}
	return nil
}

// StringParser parses string values with optional validation.
type StringParser struct {
	baseParser[string]
	minLen *int
	maxLen *int
}

// NewStringParser creates a new string parser.
// By default, it returns the value as-is without any processing.
// This is suitable for CLI flags and config files where values should be preserved exactly.
func NewStringParser() *StringParser {
	return &StringParser{
		baseParser: baseParser[string]{
			ParseFunc: func(value string) (string, error) {
				// Return value as-is, no processing
				return value, nil
			},
		},
	}
}

// WithLengthRange adds length validation.
func (p *StringParser) WithLengthRange(min, max int) *StringParser {
	p.minLen = &min
	p.maxLen = &max
	p.ValidateFunc = p.validateString
	return p
}

func (p *StringParser) validateString(value string) error {
	if p.minLen != nil && len(value) < *p.minLen {
		return fmt.Errorf("string length %d is less than minimum %d", len(value), *p.minLen)
	}
	if p.maxLen != nil && len(value) > *p.maxLen {
		return fmt.Errorf("string length %d is greater than maximum %d", len(value), *p.maxLen)
	}
	return nil
}

// ============================================================================
// Enum Parser
// ============================================================================

// EnumParser parses string values into enum types.
// It supports case-insensitive matching and custom value mappings.
type EnumParser[T comparable] struct {
	baseParser[T]
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

	parser.baseParser = baseParser[T]{
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

// ============================================================================
// Validation Helpers
// ============================================================================

// CreateRangeValidator creates a validation function for numeric types with min/max constraints.
func CreateRangeValidator[T interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64
}](min, max *T) func(T) error {
	return func(v T) error {
		if min != nil && v < *min {
			return fmt.Errorf("value %v is less than minimum %v", v, *min)
		}
		if max != nil && v > *max {
			return fmt.Errorf("value %v is greater than maximum %v", v, *max)
		}
		return nil
	}
}

// CreateDurationRangeValidator creates a validation function for duration values with min/max constraints.
func CreateDurationRangeValidator(min, max *time.Duration) func(time.Duration) error {
	return func(v time.Duration) error {
		if min != nil && v < *min {
			return fmt.Errorf("duration %s is less than minimum %s", v, *min)
		}
		if max != nil && v > *max {
			return fmt.Errorf("duration %s is greater than maximum %s", v, *max)
		}
		return nil
	}
}

// CreateDualModeParserWithValidation creates a dual-mode parser with the same validation applied to both modes.
// CreateDualModeParserWithValidation creates a dual-mode parser with the same validation applied to both modes.
func CreateDualModeParserWithValidation[T any](
	googleSQLBaseParser parser[T],
	simpleBaseParser parser[T],
	validator func(T) error,
) DualModeParser[T] {
	var googleSQLParser, simpleParser parser[T]

	if validator != nil {
		googleSQLParser = WithValidation(googleSQLBaseParser, validator)
		simpleParser = WithValidation(simpleBaseParser, validator)
	} else {
		googleSQLParser = googleSQLBaseParser
		simpleParser = simpleBaseParser
	}

	return NewDualModeParser(googleSQLParser, simpleParser)
}
