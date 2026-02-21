package mycli

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

// Variable interface that all handlers implement
type Variable interface {
	Get() (string, error)
	Set(string) error
	Description() string
	IsReadOnly() bool
}

// formatBool formats a boolean value for display (uppercase string)
func formatBool(b bool) string {
	return strings.ToUpper(strconv.FormatBool(b))
}

// VarHandler handles get/set operations for a variable
//
// TODO: Consider adding native support for session-init-only variables.
// Currently, variables like CLI_ENABLE_ADC_PLUS use custom setters to check
// if CurrentSession != nil, but this could be better supported as a first-class
// feature. Potential implementation:
//   - Add a sessionInitOnly bool field to VarHandler
//   - Move the CurrentSession check logic into the Set method
//   - This would centralize the behavior and make it declarative rather than imperative
type VarHandler[T any] struct {
	ptr         *T
	format      func(T) string
	parse       func(string) (T, error)
	validate    func(T) error
	readOnly    bool
	description string
}

// Get returns the formatted value
func (h *VarHandler[T]) Get() (string, error) {
	if h.ptr == nil {
		return "", fmt.Errorf("variable not initialized")
	}
	return h.format(*h.ptr), nil
}

// Set parses and sets the value
func (h *VarHandler[T]) Set(value string) error {
	if h.readOnly {
		return errSetterReadOnly
	}

	parsed, err := h.parse(value)
	if err != nil {
		return err
	}

	if h.validate != nil {
		if err := h.validate(parsed); err != nil {
			return err
		}
	}

	*h.ptr = parsed
	// Debug logging for bool type
	if boolPtr, ok := any(h.ptr).(*bool); ok {
		slog.Debug("VarHandler.Set bool", "value", value, "parsed", parsed, "ptrValue", *boolPtr,
			"ptr", fmt.Sprintf("%p", boolPtr))
	}
	return nil
}

// Description returns the variable description
func (h *VarHandler[T]) Description() string {
	return h.description
}

// IsReadOnly returns whether the variable is read-only
func (h *VarHandler[T]) IsReadOnly() bool {
	return h.readOnly
}

// AsReadOnly makes the handler read-only
func (h *VarHandler[T]) AsReadOnly() *VarHandler[T] {
	h.readOnly = true
	return h
}

// WithValidator adds a validator function
func (h *VarHandler[T]) WithValidator(validate func(T) error) *VarHandler[T] {
	h.validate = validate
	return h
}

// BoolVar creates a handler for bool variables
func BoolVar(ptr *bool, desc string) *VarHandler[bool] {
	return &VarHandler[bool]{
		ptr:         ptr,
		description: desc,
		format:      formatBool,
		parse:       strconv.ParseBool,
	}
}

// StringVar creates a handler for string variables
func StringVar(ptr *string, desc string) *VarHandler[string] {
	return &VarHandler[string]{
		ptr:         ptr,
		description: desc,
		format:      func(s string) string { return s },
		parse:       func(s string) (string, error) { return s, nil },
	}
}

// IntVar creates a handler for int64 variables
func IntVar(ptr *int64, desc string) *VarHandler[int64] {
	return &VarHandler[int64]{
		ptr:         ptr,
		description: desc,
		format:      func(i int64) string { return strconv.FormatInt(i, 10) },
		parse:       func(s string) (int64, error) { return strconv.ParseInt(s, 10, 64) },
	}
}

// NullableDurationVar creates a handler for nullable duration variables
func NullableDurationVar(ptr **time.Duration, desc string) *VarHandler[*time.Duration] {
	return &VarHandler[*time.Duration]{
		ptr:         ptr,
		description: desc,
		format: func(d *time.Duration) string {
			if d == nil {
				return "NULL"
			}
			return d.String()
		},
		parse: func(s string) (*time.Duration, error) {
			if strings.EqualFold(s, "NULL") {
				return nil, nil
			}
			d, err := time.ParseDuration(s)
			if err != nil {
				return nil, err
			}
			return &d, nil
		},
	}
}

// NullableIntVar creates a handler for nullable int64 variables
func NullableIntVar(ptr **int64, desc string) *VarHandler[*int64] {
	return &VarHandler[*int64]{
		ptr:         ptr,
		description: desc,
		format: func(i *int64) string {
			if i == nil {
				return "NULL"
			}
			return strconv.FormatInt(*i, 10)
		},
		parse: func(s string) (*int64, error) {
			if strings.EqualFold(s, "NULL") {
				return nil, nil
			}
			i, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				return nil, err
			}
			return &i, nil
		},
	}
}

// ReadOnlyVar creates a read-only variable with custom getter
type ReadOnlyVar struct {
	getter      func() string
	description string
}

func (r *ReadOnlyVar) Get() (string, error) {
	return r.getter(), nil
}

func (r *ReadOnlyVar) Set(value string) error {
	return errSetterReadOnly
}

func (r *ReadOnlyVar) Description() string {
	return r.description
}

func (r *ReadOnlyVar) IsReadOnly() bool {
	return true
}

// NewReadOnlyVar creates a new read-only variable
func NewReadOnlyVar(getter func() string, desc string) *ReadOnlyVar {
	return &ReadOnlyVar{
		getter:      getter,
		description: desc,
	}
}

// CustomVar wraps a variable with custom getter/setter
type CustomVar struct {
	base         Variable
	customGetter func() (string, error)
	customSetter func(string) error
}

func (c *CustomVar) Get() (string, error) {
	if c.customGetter != nil {
		return c.customGetter()
	}
	return c.base.Get()
}

func (c *CustomVar) Set(value string) error {
	if c.customSetter != nil {
		return c.customSetter(value)
	}
	return c.base.Set(value)
}

func (c *CustomVar) Description() string {
	return c.base.Description()
}

func (c *CustomVar) IsReadOnly() bool {
	return c.base.IsReadOnly()
}

// Helper function for duration validation
func durationValidator(min, max *time.Duration) func(*time.Duration) error {
	return func(d *time.Duration) error {
		if d == nil {
			return nil
		}
		if min != nil && *d < *min {
			return fmt.Errorf("duration %v is less than minimum %v", *d, *min)
		}
		if max != nil && *d > *max {
			return fmt.Errorf("duration must be at most %v", *max)
		}
		return nil
	}
}

// Helper function to create duration pointers
func durationPtr(d time.Duration) *time.Duration {
	return &d
}
