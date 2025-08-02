package main

import (
	"errors"
	"fmt"
)

// Common errors for variable operations
var (
	errSetterReadOnly      = errors.New("variable is read-only")
	errSetterInTransaction = errors.New("can't change variable when there is an active transaction")
)

// Error types for proper error handling with errors.Is/As
type (
	// ErrUnknownVariable is returned when a variable name is not recognized
	ErrUnknownVariable struct {
		Name string
	}

	// ErrAddNotSupported is returned when ADD operation is not supported for a variable
	ErrAddNotSupported struct {
		Name string
	}
)

func (e *ErrUnknownVariable) Error() string {
	return fmt.Sprintf("unknown variable: %s", e.Name)
}

func (e *ErrAddNotSupported) Error() string {
	return fmt.Sprintf("ADD not supported for %s", e.Name)
}
