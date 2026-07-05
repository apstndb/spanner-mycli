package mycli

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

// Variable is the minimal get/set surface every handler implements.
// Metadata (description, read-only/scope) lives in the varDef table
// (see var_defs.go); the registry enforces policy from the def, not the handler.
type Variable interface {
	Get() (string, error)
	Set(string) error
}

// MultiValueVar is an optional capability for a Variable whose SHOW VARIABLE
// result has multiple columns (e.g. COMMIT_RESPONSE, which surfaces
// COMMIT_TIMESTAMP and MUTATION_COUNT). Such a variable's plain Get returns an
// error (the value cannot be rendered as a single string); SHOW VARIABLE and
// SHOW VARIABLES consult GetMulti instead. Returning errIgnored signals that
// the value is currently unavailable and should be omitted.
type MultiValueVar interface {
	GetMulti() (map[string]string, error)
}

// ValidValuesEnumerator is implemented by variables that have a constrained set of valid values.
// Used by fuzzy completion to offer value candidates for SET <name> = <Ctrl+T>.
// Values must be returned as valid GoogleSQL literals (e.g., 'TABLE' for strings, TRUE for booleans).
type ValidValuesEnumerator interface {
	ValidValues() []string
}

// formatBool formats a boolean value for display (uppercase string)
func formatBool(b bool) string {
	return strings.ToUpper(strconv.FormatBool(b))
}

// VarHandler handles get/set operations for a variable.
//
// Read-only enforcement no longer lives here: the varDef's scope/readOnly
// metadata drives it in VarRegistry.Set, so a handler only needs to know how
// to format, parse, and validate its value.
type VarHandler[T any] struct {
	ptr        *T
	format     func(T) string
	parse      func(string) (T, error)
	validate   func(T) error
	enumValues []string // optional: constrained valid values for fuzzy completion
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

// ValidValues returns the constrained valid values, if any.
// Implements ValidValuesEnumerator for VarHandler instances with enumValues set.
func (h *VarHandler[T]) ValidValues() []string {
	return h.enumValues
}

// WithValidator adds a validator function
func (h *VarHandler[T]) WithValidator(validate func(T) error) *VarHandler[T] {
	h.validate = validate
	return h
}

// BoolVar creates a handler for bool variables
func BoolVar(ptr *bool) *VarHandler[bool] {
	return &VarHandler[bool]{
		ptr:        ptr,
		format:     formatBool,
		parse:      strconv.ParseBool,
		enumValues: []string{"TRUE", "FALSE"},
	}
}

// StringVar creates a handler for string variables
func StringVar(ptr *string) *VarHandler[string] {
	return &VarHandler[string]{
		ptr:    ptr,
		format: func(s string) string { return s },
		parse:  func(s string) (string, error) { return s, nil },
	}
}

// IntVar creates a handler for int64 variables
func IntVar(ptr *int64) *VarHandler[int64] {
	return &VarHandler[int64]{
		ptr:    ptr,
		format: func(i int64) string { return strconv.FormatInt(i, 10) },
		parse:  func(s string) (int64, error) { return strconv.ParseInt(s, 10, 64) },
	}
}

// NullableDurationVar creates a handler for nullable duration variables
func NullableDurationVar(ptr **time.Duration) *VarHandler[*time.Duration] {
	return &VarHandler[*time.Duration]{
		ptr: ptr,
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
func NullableIntVar(ptr **int64) *VarHandler[*int64] {
	return &VarHandler[*int64]{
		ptr: ptr,
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
	getter func() string
}

func (r *ReadOnlyVar) Get() (string, error) {
	return r.getter(), nil
}

func (r *ReadOnlyVar) Set(value string) error {
	return errSetterReadOnly
}

// NewReadOnlyVar creates a new read-only variable
func NewReadOnlyVar(getter func() string) *ReadOnlyVar {
	return &ReadOnlyVar{
		getter: getter,
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
