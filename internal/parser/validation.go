package parser

import (
	"fmt"
	"time"
)

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
func CreateDualModeParserWithValidation[T any](
	googleSQLBaseParser Parser[T],
	simpleBaseParser Parser[T],
	validator func(T) error,
) DualModeParser[T] {
	var googleSQLParser, simpleParser Parser[T]

	if validator != nil {
		googleSQLParser = WithValidation(googleSQLBaseParser, validator)
		simpleParser = WithValidation(simpleBaseParser, validator)
	} else {
		googleSQLParser = googleSQLBaseParser
		simpleParser = simpleBaseParser
	}

	return NewDualModeParser(googleSQLParser, simpleParser)
}
