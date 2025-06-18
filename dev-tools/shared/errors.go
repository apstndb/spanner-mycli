package shared

import "fmt"

// Common error message patterns to eliminate duplication across dev-tools.
// These constants provide consistent error formatting throughout the codebase.

const (
	// Error message templates
	ErrFailedToFetch    = "failed to fetch %s: %w"
	ErrFailedToParse    = "failed to parse %s: %w"
	ErrFailedToCreate   = "failed to create %s: %w"
	ErrFailedToLoad     = "failed to load %s: %w"
	ErrFailedToSave     = "failed to save %s: %w"
	ErrInvalidFormat    = "invalid %s format: %w"
	ErrInvalidArgument  = "invalid %s argument: %w"
	ErrNotFound         = "%s not found: %w"
	ErrUnauthorized     = "unauthorized access to %s: %w"
	ErrRateLimited      = "rate limited while accessing %s: %w"
)

// Error formatting helpers

// WrapError creates a formatted error with operation context
func WrapError(template, operation string, err error) error {
	return fmt.Errorf(template, operation, err)
}

// FetchError creates a "failed to fetch" error
func FetchError(resource string, err error) error {
	return fmt.Errorf(ErrFailedToFetch, resource, err)
}

// ParseError creates a "failed to parse" error
func ParseError(resource string, err error) error {
	return fmt.Errorf(ErrFailedToParse, resource, err)
}

// CreateError creates a "failed to create" error
func CreateError(resource string, err error) error {
	return fmt.Errorf(ErrFailedToCreate, resource, err)
}

// LoadError creates a "failed to load" error
func LoadError(resource string, err error) error {
	return fmt.Errorf(ErrFailedToLoad, resource, err)
}

// SaveError creates a "failed to save" error
func SaveError(resource string, err error) error {
	return fmt.Errorf(ErrFailedToSave, resource, err)
}

// FormatError creates a "invalid format" error
func FormatError(resource string, err error) error {
	return fmt.Errorf(ErrInvalidFormat, resource, err)
}

// ArgumentError creates a "invalid argument" error
func ArgumentError(argument string, err error) error {
	return fmt.Errorf(ErrInvalidArgument, argument, err)
}

// NotFoundError creates a "not found" error
func NotFoundError(resource string, err error) error {
	return fmt.Errorf(ErrNotFound, resource, err)
}