package mycli

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

	// errSetterInitOnly is returned when an init-only variable (initOnly) is set
	// after the session has been created. It replaces the ad-hoc error the
	// CLI_ENABLE_ADC_PLUS custom setter used to return.
	errSetterInitOnly struct {
		Name string
	}
)

func (e *ErrUnknownVariable) Error() string {
	return fmt.Sprintf("unknown variable: %s", e.Name)
}

func (e *errSetterInitOnly) Error() string {
	return fmt.Sprintf("%s cannot be changed after session creation", e.Name)
}

func (e *ErrAddNotSupported) Error() string {
	return fmt.Sprintf("ADD not supported for %s", e.Name)
}
