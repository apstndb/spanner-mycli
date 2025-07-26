// Package parser provides a generic framework for parsing and validating values
// from string representations. It is designed to unify parsing logic across
// system variables, CLI arguments, and configuration files.
package parser

import (
	"fmt"
)

// Parser is the core interface for parsing and validating values of type T.
// It provides a unified way to convert strings to typed values with validation.
type Parser[T any] interface {
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

// BaseParser provides a foundation for implementing parsers.
// It handles the common ParseAndValidate logic.
type BaseParser[T any] struct {
	ParseFunc    func(string) (T, error)
	ValidateFunc func(T) error
}

// Parse implements the Parser interface.
func (p *BaseParser[T]) Parse(value string) (T, error) {
	if p.ParseFunc == nil {
		var zero T
		return zero, fmt.Errorf("parse function not implemented")
	}
	return p.ParseFunc(value)
}

// Validate implements the Parser interface.
func (p *BaseParser[T]) Validate(value T) error {
	if p.ValidateFunc == nil {
		return nil // No validation if not specified
	}
	return p.ValidateFunc(value)
}

// ParseAndValidate implements the Parser interface.
func (p *BaseParser[T]) ParseAndValidate(value string) (T, error) {
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

// Validator is a function type for value validation.
type Validator[T any] func(value T) error

// ChainValidators combines multiple validators into a single validator.
// All validators must pass for the value to be considered valid.
func ChainValidators[T any](validators ...Validator[T]) Validator[T] {
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
func WithValidation[T any](parser Parser[T], validators ...Validator[T]) Parser[T] {
	return &BaseParser[T]{
		ParseFunc: parser.Parse,
		ValidateFunc: func(value T) error {
			// First run the original validation
			if err := parser.Validate(value); err != nil {
				return err
			}
			// Then run additional validators
			return ChainValidators(validators...)(value)
		},
	}
}