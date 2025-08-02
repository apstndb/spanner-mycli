// sysvar.go - System variable parser interface and registry
//
// This file contains the system variable framework including the
// VariableParser interface, TypedVariableParser implementation,
// registry, and helper functions for system variable management.

package sysvar

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/ast"
)

// ============================================================================
// System Variable Parser Interface
// ============================================================================

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
	ParseAndSetWithMode(value string, mode parseMode) error
	GetValue() (string, error)
	IsReadOnly() bool
}

// typedVariableParser provides a generic implementation of VariableParser for any type T.
// This struct encapsulates type-specific parsing and formatting logic.
type typedVariableParser[T any] struct {
	name        string
	description string
	parser      DualModeParser[T]
	setter      func(T) error
	getter      func() T
	formatter   func(T) string
	readOnly    bool
}

// Name returns the variable name.
func (vp *typedVariableParser[T]) Name() string {
	return vp.name
}

// Description returns the variable description.
func (vp *typedVariableParser[T]) Description() string {
	return vp.description
}

// ParseAndSetWithMode parses a value and sets the variable using the specified mode.
func (vp *typedVariableParser[T]) ParseAndSetWithMode(value string, mode parseMode) error {
	if vp.readOnly || vp.setter == nil {
		return &ErrVariableReadOnly{Name: vp.name}
	}

	parsed, err := vp.parser.ParseAndValidateWithMode(value, mode)
	if err != nil {
		return fmt.Errorf("%s: %w", vp.name, err)
	}

	if err := vp.setter(parsed); err != nil {
		return fmt.Errorf("%s: %w", vp.name, err)
	}

	return nil
}

// GetValue returns the formatted value of the variable.
func (vp *typedVariableParser[T]) GetValue() (string, error) {
	if vp.getter == nil {
		return "", fmt.Errorf("%s: no getter configured", vp.name)
	}

	value := vp.getter()
	if vp.formatter != nil {
		return vp.formatter(value), nil
	}

	// Default formatting
	return fmt.Sprintf("%v", value), nil
}

// IsReadOnly returns true if the variable is read-only.
func (vp *typedVariableParser[T]) IsReadOnly() bool {
	return vp.readOnly
}

// createTypedParser is a helper function to create TypedVariableParser instances.
func createTypedParser[T any](
	name string,
	description string,
	parser DualModeParser[T],
	getter func() T,
	setter func(T) error,
	formatter func(T) string,
) *typedVariableParser[T] {
	return &typedVariableParser[T]{
		name:        name,
		description: description,
		parser:      parser,
		getter:      getter,
		setter:      setter,
		formatter:   formatter,
		readOnly:    setter == nil,
	}
}

// ============================================================================
// Convenience Constructors for Common Variable Types
// ============================================================================

// NewBooleanParser creates a boolean variable parser.
func NewBooleanParser(
	name string,
	description string,
	getter func() bool,
	setter func(bool) error,
) VariableParser {
	return createTypedParser(name, description, DualModeBoolParser, getter, setter, FormatBool)
}

// NewStringVariableParser creates a string variable parser.
func NewStringVariableParser(
	name string,
	description string,
	getter func() string,
	setter func(string) error,
) VariableParser {
	return createTypedParser(name, description, DualModeStringParser, getter, setter, FormatString)
}

// NewIntegerVariableParser creates an integer variable parser with optional range validation.
func NewIntegerVariableParser(
	name string,
	description string,
	getter func() int64,
	setter func(int64) error,
	min, max *int64,
) VariableParser {
	var opts *rangeParserOptions[int64]
	if min != nil || max != nil {
		opts = &rangeParserOptions[int64]{Min: min, Max: max}
	}
	parser := createIntRangeParser(opts)
	return createTypedParser(name, description, parser, getter, setter, FormatInt)
}

// NewEnumVariableParser creates an enum variable parser with a custom formatter.
func NewEnumVariableParser[T comparable](
	name string,
	description string,
	values map[string]T,
	getter func() T,
	setter func(T) error,
	formatter func(T) string,
) VariableParser {
	parser := createDualModeEnumParser(values)
	return createTypedParser(name, description, parser, getter, setter, formatter)
}

// newDurationVariableParser creates a duration variable parser with optional range constraints.
func newDurationVariableParser(
	name string,
	description string,
	getter func() time.Duration,
	setter func(time.Duration) error,
	min, max *time.Duration,
) VariableParser {
	var opts *rangeParserOptions[time.Duration]
	if min != nil || max != nil {
		opts = &rangeParserOptions[time.Duration]{Min: min, Max: max}
	}
	parser := createDurationRangeParser(opts)
	return createTypedParser(name, description, parser, getter, setter, formatDuration)
}

// NewNullableDurationVariableParser creates a nullable duration variable parser.
func NewNullableDurationVariableParser(
	name string,
	description string,
	getter func() *time.Duration,
	setter func(*time.Duration) error,
	min, max *time.Duration,
) VariableParser {
	var baseParser DualModeParser[time.Duration]
	if min != nil || max != nil {
		opts := &rangeParserOptions[time.Duration]{Min: min, Max: max}
		baseParser = createDurationRangeParser(opts)
	} else {
		baseParser = dualModeDurationParser
	}
	nullableParser := newNullableParser(baseParser)
	return NewTypedVariableParser(name, description, nullableParser, getter, setter, formatNullable(formatDuration))
}

// NewTypedVariableParser creates a variable parser with a custom dual-mode parser.
// This is the most flexible constructor that allows full control over parsing behavior.
func NewTypedVariableParser[T any](
	name string,
	description string,
	parser DualModeParser[T],
	getter func() T,
	setter func(T) error,
	formatter func(T) string,
) VariableParser {
	return createTypedParser(name, description, parser, getter, setter, formatter)
}

// NewSimpleEnumParser creates an enum parser that works directly with string values.
// This is useful for simple string enums where the parsed value is the same as the formatted value.
func NewSimpleEnumParser[T ~string](
	name string,
	description string,
	values map[string]T,
	getter func() T,
	setter func(T) error,
) VariableParser {
	// For simple string enums, the formatter just converts to string
	formatter := func(v T) string { return string(v) }
	return NewEnumVariableParser(name, description, values, getter, setter, formatter)
}

// ============================================================================
// Registry for System Variables
// ============================================================================

// Registry manages a collection of variable parsers.
type Registry struct {
	parsers map[string]VariableParser
	mu      sync.RWMutex
}

// NewRegistry creates a new variable parser registry.
func NewRegistry() *Registry {
	return &Registry{
		parsers: make(map[string]VariableParser),
	}
}

// Register adds a variable parser to the registry.
func (r *Registry) Register(parser VariableParser) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := strings.ToUpper(parser.Name())
	if _, exists := r.parsers[name]; exists {
		return fmt.Errorf("variable %s already registered", name)
	}

	r.parsers[name] = parser
	return nil
}

// GetParser retrieves a variable parser by name (case-insensitive).
func (r *Registry) GetParser(name string) (VariableParser, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	parser, ok := r.parsers[strings.ToUpper(name)]
	return parser, ok
}

// Has checks if a variable is registered (case-insensitive).
func (r *Registry) Has(name string) bool {
	_, ok := r.GetParser(name)
	return ok
}

// GetAll returns all registered variable parsers.
func (r *Registry) GetAll() map[string]VariableParser {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Create a copy to avoid concurrent modification
	result := make(map[string]VariableParser, len(r.parsers))
	for k, v := range r.parsers {
		result[k] = v
	}
	return result
}

// SetFromGoogleSQL sets a variable value using GoogleSQL parsing mode.
func (r *Registry) SetFromGoogleSQL(name, value string) error {
	parser, ok := r.GetParser(name)
	if !ok {
		return fmt.Errorf("unknown variable: %s", name)
	}
	return parser.ParseAndSetWithMode(value, ParseModeGoogleSQL)
}

// SetFromSimple sets a variable value using simple parsing mode.
func (r *Registry) SetFromSimple(name, value string) error {
	parser, ok := r.GetParser(name)
	if !ok {
		return fmt.Errorf("unknown variable: %s", name)
	}
	return parser.ParseAndSetWithMode(value, ParseModeSimple)
}

// Get retrieves the current value of a variable as a formatted string.
func (r *Registry) Get(name string) (string, error) {
	parser, ok := r.GetParser(name)
	if !ok {
		return "", fmt.Errorf("unknown variable: %s", name)
	}
	return parser.GetValue()
}

// ============================================================================
// Read-Only Variable Support
// ============================================================================

// NewReadOnlyStringParser creates a read-only string variable parser.
func NewReadOnlyStringParser(
	name string,
	description string,
	getter func() string,
) VariableParser {
	return createTypedParser(name, description, DualModeStringParser, getter, nil, FormatString)
}

// NewReadOnlyBooleanParser creates a read-only boolean variable parser.
func NewReadOnlyBooleanParser(
	name string,
	description string,
	getter func() bool,
) VariableParser {
	return createTypedParser(name, description, DualModeBoolParser, getter, nil, FormatBool)
}

// NewNullableIntVariableParser creates a nullable integer variable parser.
func NewNullableIntVariableParser(
	name string,
	description string,
	getter func() *int64,
	setter func(*int64) error,
	min, max *int64,
) VariableParser {
	var baseParser DualModeParser[int64]
	if min != nil || max != nil {
		opts := &rangeParserOptions[int64]{Min: min, Max: max}
		baseParser = createIntRangeParser(opts)
	} else {
		baseParser = DualModeIntParser
	}
	nullableParser := newNullableParser(baseParser)
	return NewTypedVariableParser(name, description, nullableParser, getter, setter, formatNullable(FormatInt))
}

// ============================================================================
// Nullable Parser Support
// ============================================================================

// nullableParser wraps a parser to handle NULL values.
type nullableParser[T any] struct {
	baseParser[*T]
	innerParser DualModeParser[T]
}

// newNullableParser creates a parser that accepts NULL values.
func newNullableParser[T any](innerParser DualModeParser[T]) *nullableParser[T] {
	return &nullableParser[T]{
		innerParser: innerParser,
	}
}

// ParseAndValidateWithMode parses a value that can be NULL.
func (p *nullableParser[T]) ParseAndValidateWithMode(s string, mode parseMode) (*T, error) {
	// In GoogleSQL mode, check if it's a NULL literal
	if mode == ParseModeGoogleSQL {
		// Try to parse as an expression to check for NULL literal
		expr, err := memefish.ParseExpr("", s)
		if err == nil {
			if _, ok := expr.(*ast.NullLiteral); ok {
				return nil, nil
			}
		}
		// If not a NULL literal, fall through to parse as regular value
	} else {
		// In simple mode, we also accept "NULL" as a special case for compatibility
		// This allows CLI flags and config files to use NULL to unset nullable variables
		trimmed := strings.TrimSpace(s)
		if strings.ToUpper(trimmed) == "NULL" {
			return nil, nil
		}
	}

	// Parse as regular value
	value, err := p.innerParser.ParseAndValidateWithMode(s, mode)
	if err != nil {
		return nil, err
	}

	return &value, nil
}

// ParseWithMode implements the DualModeParser interface.
func (p *nullableParser[T]) ParseWithMode(s string, mode parseMode) (*T, error) {
	// Just parse without validation
	return p.ParseAndValidateWithMode(s, mode)
}

// Parse implements the parser interface (defaults to GoogleSQL mode).
func (p *nullableParser[T]) Parse(s string) (*T, error) {
	return p.ParseWithMode(s, ParseModeGoogleSQL)
}

// Validate implements the parser interface.
func (p *nullableParser[T]) Validate(v *T) error {
	return nil // NULL is always valid
}

// ParseAndValidate implements the parser interface.
func (p *nullableParser[T]) ParseAndValidate(s string) (*T, error) {
	return p.ParseAndValidateWithMode(s, ParseModeSimple)
}

// newNullableDuration creates a nullable duration parser.
// This is a convenience function for newNullableParser[time.Duration].
func newNullableDuration(innerParser DualModeParser[time.Duration]) *nullableParser[time.Duration] {
	return newNullableParser(innerParser)
}

// newNullableInt creates a nullable integer parser.
// This is a convenience function for newNullableParser[int64].
func newNullableInt(innerParser DualModeParser[int64]) *nullableParser[int64] {
	return newNullableParser(innerParser)
}

// ============================================================================
// Formatting Functions
// ============================================================================

// FormatBool formats a boolean value as uppercase TRUE/FALSE.
func FormatBool(v bool) string {
	return strings.ToUpper(strconv.FormatBool(v))
}

// FormatInt formats an integer value.
func FormatInt(v int64) string {
	return strconv.FormatInt(v, 10)
}

// FormatString formats a string value (returns as-is).
func FormatString(v string) string {
	return v
}

// formatDuration formats a duration value.
func formatDuration(v time.Duration) string {
	return v.String()
}

// formatNullable creates a formatter for nullable values.
func formatNullable[T any](innerFormatter func(T) string) func(*T) string {
	return func(v *T) string {
		if v == nil {
			return "NULL"
		}
		return innerFormatter(*v)
	}
}

// ============================================================================
// Helper Functions
// ============================================================================

// GetValue returns a getter function that retrieves the value from the given pointer.
// This is useful for simplifying getter functions in system variable parsers.
//
// Example:
//
//	getter: GetValue(&sv.RPCPriority)
//
// Instead of:
//
//	getter: func() sppb.RequestOptions_Priority { return sv.RPCPriority }
func GetValue[T any](ptr *T) func() T {
	return func() T {
		return *ptr
	}
}

// SetValue returns a setter function that sets the value to the given pointer.
// This is useful for simplifying setter functions in system variable parsers.
//
// Example:
//
//	setter: SetValue(&sv.RPCPriority)
//
// Instead of:
//
//	setter: func(v sppb.RequestOptions_Priority) error { sv.RPCPriority = v; return nil }
func SetValue[T any](ptr *T) func(T) error {
	return func(v T) error {
		*ptr = v
		return nil
	}
}

// SetSessionInitOnly returns a setter that only allows changes before a session is created.
// This is used for variables that affect session initialization and cannot be changed afterwards.
func SetSessionInitOnly[T any, S any](ptr *T, varName string, sessionPtr **S) func(T) error {
	return func(v T) error {
		if *sessionPtr != nil {
			return fmt.Errorf("%s cannot be changed after session creation", varName)
		}
		*ptr = v
		return nil
	}
}

// ============================================================================
// Builder Pattern Support
// ============================================================================

// rangeParserOptions holds options for creating range-validated parsers.
type rangeParserOptions[T any] struct {
	Min *T
	Max *T
}

// hasRange returns true if either Min or Max is set.
func (opts *rangeParserOptions[T]) hasRange() bool {
	return opts != nil && (opts.Min != nil || opts.Max != nil)
}

// createIntRangeParser creates a dual-mode integer parser with range validation.
func createIntRangeParser(opts *rangeParserOptions[int64]) DualModeParser[int64] {
	if opts == nil || (opts.Min == nil && opts.Max == nil) {
		return DualModeIntParser
	}

	validator := createRangeValidator(opts.Min, opts.Max)
	return createDualModeParserWithValidation(
		GoogleSQLIntParser,
		newIntParser(),
		validator,
	)
}

// createDurationRangeParser creates a dual-mode duration parser with range validation.
func createDurationRangeParser(opts *rangeParserOptions[time.Duration]) DualModeParser[time.Duration] {
	if opts == nil || (opts.Min == nil && opts.Max == nil) {
		return dualModeDurationParser
	}

	validator := createDurationRangeValidator(opts.Min, opts.Max)
	return createDualModeParserWithValidation(
		googleSQLDurationParser,
		newDurationParser(),
		validator,
	)
}

// CreateStringEnumVariableParser creates a variable parser for enums that use string values directly.
// This is simpler than NewEnumVariableParser when the enum values are just strings.
func CreateStringEnumVariableParser[T ~string](
	name string,
	description string,
	values map[string]string,
	getter func() string,
	setter func(string) error,
) VariableParser {
	parser := createDualModeEnumParser(values)
	return createTypedParser(name, description, parser, getter, setter, FormatString)
}

// CreateProtobufEnumVariableParserWithAutoFormatter creates a parser for protobuf enums
// with automatic formatting based on the enum descriptor.
func CreateProtobufEnumVariableParserWithAutoFormatter[T ~int32](
	name string,
	description string,
	enumValues map[string]int32,
	prefix string,
	getter func() T,
	setter func(T) error,
) VariableParser {
	// Convert generic enum values to the specific type
	typedValues := make(map[string]T)
	for k, v := range enumValues {
		// Include both the original key and the trimmed version
		typedValues[k] = T(v)
		typedValues[strings.TrimPrefix(k, prefix)] = T(v)
	}

	// Create a formatter that uses the enum descriptor
	formatter := func(v T) string {
		// First, try to find the value in the original enumValues map
		// and return the trimmed version
		for fullName, value := range enumValues {
			if T(value) == v {
				return strings.TrimPrefix(fullName, prefix)
			}
		}
		// Fallback to generic formatting
		return fmt.Sprintf("%v", v)
	}

	return NewEnumVariableParser(name, description, typedValues, getter, setter, formatter)
}
