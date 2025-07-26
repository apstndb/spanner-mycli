package parser

import (
	"fmt"
	"strings"
	"time"
)

// OptionalParser wraps a parser to handle NULL values.
// It converts NULL (case-insensitive) to nil pointer.
type OptionalParser[T any] struct {
	base Parser[T]
}

// NewOptionalParser creates a parser that can handle NULL values.
func NewOptionalParser[T any](base Parser[T]) *OptionalParser[T] {
	return &OptionalParser[T]{base: base}
}

// Parse parses the value, returning nil for NULL.
func (p *OptionalParser[T]) Parse(value string) (*T, error) {
	// Check for NULL (case-insensitive, trimmed)
	if strings.ToUpper(strings.TrimSpace(value)) == "NULL" {
		return nil, nil
	}

	// Parse non-NULL value
	v, err := p.base.Parse(value)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// Validate validates a non-nil value using the base parser's validation.
func (p *OptionalParser[T]) Validate(value *T) error {
	if value == nil {
		return nil // NULL is always valid
	}
	return p.base.Validate(*value)
}

// ParseAndValidate combines parsing and validation.
func (p *OptionalParser[T]) ParseAndValidate(value string) (*T, error) {
	parsed, err := p.Parse(value)
	if err != nil {
		return nil, err
	}
	if err := p.Validate(parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

// OptionalGoogleSQLParser handles NULL in GoogleSQL mode.
// It first tries to parse as a literal value, then falls back to checking for NULL identifier.
type OptionalGoogleSQLParser[T any] struct {
	base Parser[T]
}

// NewOptionalGoogleSQLParser creates a parser for GoogleSQL mode that handles NULL.
func NewOptionalGoogleSQLParser[T any](base Parser[T]) *OptionalGoogleSQLParser[T] {
	return &OptionalGoogleSQLParser[T]{base: base}
}

// Parse tries to parse as a literal first, then checks for NULL identifier.
func (p *OptionalGoogleSQLParser[T]) Parse(value string) (*T, error) {
	// First try to parse as the base type
	v, err := p.base.Parse(value)
	if err == nil {
		return &v, nil
	}

	// If parsing failed, check if it's a NULL identifier
	if strings.ToUpper(strings.TrimSpace(value)) == "NULL" {
		return nil, nil
	}

	// Return the original parsing error
	return nil, err
}

// Validate validates a non-nil value.
func (p *OptionalGoogleSQLParser[T]) Validate(value *T) error {
	if value == nil {
		return nil
	}
	return p.base.Validate(*value)
}

// ParseAndValidate combines parsing and validation.
func (p *OptionalGoogleSQLParser[T]) ParseAndValidate(value string) (*T, error) {
	parsed, err := p.Parse(value)
	if err != nil {
		return nil, err
	}
	if err := p.Validate(parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

// CreateOptionalDualModeParser creates a dual-mode parser that handles NULL values.
func CreateOptionalDualModeParser[T any](googleSQLBase, simpleBase Parser[T]) DualModeParser[*T] {
	return NewDualModeParser[*T](
		NewOptionalGoogleSQLParser(googleSQLBase),
		NewOptionalParser(simpleBase),
	)
}

// WithRangeValidation adds range validation to an optional parser.
// This is a generic helper that works with any ordered type.
func WithRangeValidation[T interface {
	int64 | float64 | time.Duration
}](
	parser Parser[*T],
	min, max T,
) Parser[*T] {
	return WithValidation(parser, func(value *T) error {
		if value == nil {
			return nil // NULL is always valid
		}
		if *value < min {
			return fmt.Errorf("value %v is less than minimum %v", *value, min)
		}
		if *value > max {
			return fmt.Errorf("value %v is greater than maximum %v", *value, max)
		}
		return nil
	})
}

// WithMinValidation adds minimum validation to an optional parser.
func WithMinValidation[T interface {
	int64 | float64 | time.Duration
}](
	parser Parser[*T],
	min T,
) Parser[*T] {
	return WithValidation(parser, func(value *T) error {
		if value == nil {
			return nil // NULL is always valid
		}
		if *value < min {
			return fmt.Errorf("value %v is less than minimum %v", *value, min)
		}
		return nil
	})
}

// WithMaxValidation adds maximum validation to an optional parser.
func WithMaxValidation[T interface {
	int64 | float64 | time.Duration
}](
	parser Parser[*T],
	max T,
) Parser[*T] {
	return WithValidation(parser, func(value *T) error {
		if value == nil {
			return nil // NULL is always valid
		}
		if *value > max {
			return fmt.Errorf("value %v is greater than maximum %v", *value, max)
		}
		return nil
	})
}
