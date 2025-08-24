package main

import (
	"errors"
	"fmt"
)

// ExitCodeError represents an error that only carries an exit code without a message
type ExitCodeError struct {
	exitCode int
}

// Error implements the error interface
func (e *ExitCodeError) Error() string {
	return fmt.Sprintf("exit code: %d", e.exitCode)
}

// NewExitCodeError creates a new ExitCodeError
func NewExitCodeError(exitCode int) error {
	if exitCode == exitCodeSuccess {
		return nil
	}

	return &ExitCodeError{
		exitCode: exitCode,
	}
}

// GetExitCode returns the appropriate exit code based on the error type.
// It checks for ExitCodeError first and returns its exit code if found.
// Otherwise, it returns exitCodeSuccess for nil errors and exitCodeError for all other errors.
//
// In the future, this function could be further enhanced to return different
// exit codes based on other error types, such as:
//
//	var validationErr *ValidationError
//	if errors.As(err, &validationErr) {
//	    return exitCodeValidationError // e.g., 2
//	}
//
// This would require defining additional exit code constants.
func GetExitCode(err error) int {
	if err == nil {
		return exitCodeSuccess
	}

	// Check for ExitCodeError
	var exitCodeErr *ExitCodeError
	if errors.As(err, &exitCodeErr) {
		return exitCodeErr.exitCode
	}

	// Default to generic error
	return exitCodeError
}
