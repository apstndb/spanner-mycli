package parser

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// BoolParser parses boolean values.
// It uses strconv.ParseBool which accepts:
// "1", "t", "T", "true", "TRUE", "True",
// "0", "f", "F", "false", "FALSE", "False".
type BoolParser struct {
	BaseParser[bool]
}

// NewBoolParser creates a new boolean parser.
func NewBoolParser() *BoolParser {
	return &BoolParser{
		BaseParser: BaseParser[bool]{
			ParseFunc: func(value string) (bool, error) {
				return strconv.ParseBool(strings.TrimSpace(value))
			},
		},
	}
}

// IntParser parses integer values with optional range validation.
type IntParser struct {
	BaseParser[int64]
	min *int64
	max *int64
}

// NewIntParser creates a new integer parser.
func NewIntParser() *IntParser {
	return &IntParser{
		BaseParser: BaseParser[int64]{
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
	BaseParser[time.Duration]
	min *time.Duration
	max *time.Duration
}

// NewDurationParser creates a new duration parser.
func NewDurationParser() *DurationParser {
	return &DurationParser{
		BaseParser: BaseParser[time.Duration]{
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
	BaseParser[string]
	minLen *int
	maxLen *int
}

// NewStringParser creates a new string parser.
// By default, it returns the value as-is without any processing.
// This is suitable for CLI flags and config files where values should be preserved exactly.
func NewStringParser() *StringParser {
	return &StringParser{
		BaseParser: BaseParser[string]{
			ParseFunc: func(value string) (string, error) {
				// Return value as-is, no processing
				return value, nil
			},
		},
	}
}

// NewQuotedStringParser creates a string parser that removes quotes.
// This is useful when quotes are part of the input syntax but not the value.
func NewQuotedStringParser() *StringParser {
	return &StringParser{
		BaseParser: BaseParser[string]{
			ParseFunc: func(value string) (string, error) {
				// Remove surrounding quotes if present
				trimmed := strings.TrimSpace(value)
				if len(trimmed) >= 2 {
					if (trimmed[0] == '"' && trimmed[len(trimmed)-1] == '"') ||
						(trimmed[0] == '\'' && trimmed[len(trimmed)-1] == '\'') {
						return trimmed[1 : len(trimmed)-1], nil
					}
				}
				return trimmed, nil
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
