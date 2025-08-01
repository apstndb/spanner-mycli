package sysvar

import (
	"fmt"
	"strings"
	"time"

	"github.com/apstndb/spanner-mycli/internal/parser"
)

// ErrVariableReadOnly is returned when attempting to set a read-only variable.
type ErrVariableReadOnly struct {
	Name string
}

func (e *ErrVariableReadOnly) Error() string {
	return fmt.Sprintf("variable %s is read-only", e.Name)
}

// VariableParser is the interface that all typed variable parsers must implement.
// This allows the registry to handle different types uniformly without reflection.
type VariableParser interface {
	Name() string
	Description() string
	ParseAndSetWithMode(value string, mode parser.ParseMode) error
	GetValue() (string, error)
	IsReadOnly() bool
}

// TypedVariableParser provides a generic implementation of VariableParser for any type T.
// This struct encapsulates type-specific parsing and formatting logic.
type TypedVariableParser[T any] struct {
	name        string
	description string
	parser      parser.DualModeParser[T]
	setter      func(T) error
	getter      func() T
	formatter   func(T) string
	readOnly    bool
}

// Name returns the variable name.
func (vp *TypedVariableParser[T]) Name() string {
	return vp.name
}

// Description returns the variable description.
func (vp *TypedVariableParser[T]) Description() string {
	return vp.description
}

// IsReadOnly returns whether the variable is read-only.
func (vp *TypedVariableParser[T]) IsReadOnly() bool {
	return vp.readOnly
}

// ParseAndSetWithMode parses a string value using the specified mode and sets it.
func (vp *TypedVariableParser[T]) ParseAndSetWithMode(value string, mode parser.ParseMode) error {
	if vp.readOnly {
		return &ErrVariableReadOnly{Name: vp.name}
	}

	parsed, err := vp.parser.ParseAndValidateWithMode(value, mode)
	if err != nil {
		return fmt.Errorf("invalid value for %s: %w", vp.name, err)
	}

	return vp.setter(parsed)
}

// GetValue retrieves the current value formatted as a string.
func (vp *TypedVariableParser[T]) GetValue() (string, error) {
	value := vp.getter()
	return vp.formatter(value), nil
}

// Helper functions for creating typed parsers

// createTypedParser is a generic helper to create TypedVariableParser instances
func createTypedParser[T any](
	name, description string,
	p parser.DualModeParser[T],
	getter func() T,
	setter func(T) error,
	formatter func(T) string,
) VariableParser {
	return &TypedVariableParser[T]{
		name:        name,
		description: description,
		parser:      p,
		setter:      setter,
		getter:      getter,
		formatter:   formatter,
		readOnly:    setter == nil,
	}
}

// NewBooleanParser creates a boolean variable parser.
func NewBooleanParser(
	name string,
	description string,
	getter func() bool,
	setter func(bool) error,
) VariableParser {
	return createTypedParser(name, description, parser.DualModeBoolParser, getter, setter, FormatBool)
}

// NewStringParser creates a string variable parser.
func NewStringParser(
	name string,
	description string,
	getter func() string,
	setter func(string) error,
) VariableParser {
	return createTypedParser(name, description, parser.DualModeStringParser, getter, setter, FormatString)
}

// NewIntegerParser creates an integer variable parser with optional range validation.
func NewIntegerParser(
	name string,
	description string,
	getter func() int64,
	setter func(int64) error,
	min, max *int64,
) VariableParser {
	var opts *RangeParserOptions[int64]
	if min != nil || max != nil {
		opts = &RangeParserOptions[int64]{Min: min, Max: max}
	}

	return createTypedParser(name, description, CreateIntRangeParser(opts), getter, setter, FormatInt)
}

// NewEnumParser creates an enum variable parser for any comparable type.
func NewEnumParser[T comparable](
	name string,
	description string,
	values map[string]T,
	getter func() T,
	setter func(T) error,
	formatter func(T) string,
) VariableParser {
	return createTypedParser(name, description, parser.CreateDualModeEnumParser(values), getter, setter, formatter)
}

// NewDurationParser creates a duration variable parser.
func NewDurationParser(
	name string,
	description string,
	getter func() time.Duration,
	setter func(time.Duration) error,
	min, max *time.Duration,
) VariableParser {
	var opts *RangeParserOptions[time.Duration]
	if min != nil || max != nil {
		opts = &RangeParserOptions[time.Duration]{Min: min, Max: max}
	}

	return createTypedParser(name, description, CreateDurationRangeParser(opts), getter, setter, FormatDuration)
}

// NewNullableDurationParser creates a nullable duration variable parser.
func NewNullableDurationParser(
	name string,
	description string,
	getter func() *time.Duration,
	setter func(*time.Duration) error,
	min, max *time.Duration,
) VariableParser {
	var opts *RangeParserOptions[time.Duration]
	if min != nil || max != nil {
		opts = &RangeParserOptions[time.Duration]{Min: min, Max: max}
	}

	innerParser := CreateDurationRangeParser(opts)
	p := parser.NewNullableDurationParser(innerParser)

	return createTypedParser(name, description, p, getter, setter, FormatNullable(FormatDuration))
}

// NewTypedVariableParser creates a new TypedVariableParser for custom types.
// This is useful for creating parsers for application-specific enums and types.
func NewTypedVariableParser[T any](
	name string,
	description string,
	parser parser.DualModeParser[T],
	getter func() T,
	setter func(T) error,
	formatter func(T) string,
) VariableParser {
	return createTypedParser(name, description, parser, getter, setter, formatter)
}

// NewSimpleEnumParser creates an enum variable parser with standard behavior.
// It automatically creates dual-mode parsing for both GoogleSQL and Simple modes.
// The formatter uses fmt.Sprint to convert the enum value to string.
func NewSimpleEnumParser[T comparable](
	name string,
	description string,
	enumValues map[string]T,
	getter func() T,
	setter func(T) error,
) VariableParser {
	return createTypedParser(name, description, parser.CreateDualModeEnumParser(enumValues), getter, setter, func(v T) string { return fmt.Sprint(v) })
}

// Registry manages system variable parsers.
//
// Note: The current implementation only supports Set and Get operations.
// Add/Append operations are not yet supported. Variables that require
// append functionality (like CLI_PROTO_DESCRIPTOR_FILE) must remain in
// the legacy system until this feature is implemented.
type Registry struct {
	parsers map[string]VariableParser
}

// NewRegistry creates a new variable registry.
func NewRegistry() *Registry {
	return &Registry{
		parsers: make(map[string]VariableParser),
	}
}

// Register adds a variable parser to the registry.
func (r *Registry) Register(parser VariableParser) error {
	upperName := strings.ToUpper(parser.Name())
	if _, exists := r.parsers[upperName]; exists {
		return fmt.Errorf("variable %s already registered", upperName)
	}
	r.parsers[upperName] = parser
	return nil
}

// Has checks if a variable is registered in the registry.
func (r *Registry) Has(name string) bool {
	upperName := strings.ToUpper(name)
	_, exists := r.parsers[upperName]
	return exists
}

// SetFromGoogleSQL parses and sets a value using GoogleSQL syntax.
func (r *Registry) SetFromGoogleSQL(name, value string) error {
	return r.setWithMode(name, value, parser.ParseModeGoogleSQL)
}

// SetFromSimple parses and sets a value using simple syntax.
func (r *Registry) SetFromSimple(name, value string) error {
	return r.setWithMode(name, value, parser.ParseModeSimple)
}

func (r *Registry) setWithMode(name, value string, mode parser.ParseMode) error {
	upperName := strings.ToUpper(name)
	p, exists := r.parsers[upperName]
	if !exists {
		return fmt.Errorf("unknown variable: %s", name)
	}

	return p.ParseAndSetWithMode(value, mode)
}

// Get retrieves the current value of a variable as a formatted string.
func (r *Registry) Get(name string) (string, error) {
	upperName := strings.ToUpper(name)
	p, exists := r.parsers[upperName]
	if !exists {
		return "", fmt.Errorf("unknown variable: %s", name)
	}

	return p.GetValue()
}

// NewReadOnlyStringParser creates a read-only string variable parser.
func NewReadOnlyStringParser(
	name string,
	description string,
	getter func() string,
) VariableParser {
	return createTypedParser(name, description, parser.DualModeStringParser, getter, nil, FormatString)
}

// NewReadOnlyBooleanParser creates a read-only boolean variable parser.
func NewReadOnlyBooleanParser(
	name string,
	description string,
	getter func() bool,
) VariableParser {
	return createTypedParser(name, description, parser.DualModeBoolParser, getter, nil, FormatBool)
}

// NewNullableIntParser creates a nullable integer variable parser.
func NewNullableIntParser(
	name string,
	description string,
	getter func() *int64,
	setter func(*int64) error,
	min, max *int64,
) VariableParser {
	var opts *RangeParserOptions[int64]
	if min != nil || max != nil {
		opts = &RangeParserOptions[int64]{Min: min, Max: max}
	}

	innerParser := CreateIntRangeParser(opts)
	p := parser.NewNullableIntParser(innerParser)

	return createTypedParser(name, description, p, getter, setter, FormatNullable(FormatInt))
}
