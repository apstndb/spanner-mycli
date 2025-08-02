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

// NewSimpleEnumParser creates an enum parser for custom enum types (e.g., type MyEnum string).
// Use this when you have a typed enum and need custom formatting behavior.
// Example: type ExplainFormat string with values like explainFormatCurrent, explainFormatCompact
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

// FormatTimestamp creates a formatter function that formats a time.Time value
// as RFC3339Nano or "NULL" if zero. This is used for timestamp system variables
// like READ_TIMESTAMP and COMMIT_TIMESTAMP.
//
// Example:
//
//	getter: FormatTimestamp(&sv.ReadTimestamp)
func FormatTimestamp(ptr *time.Time) func() string {
	return func() string {
		if ptr.IsZero() {
			return "NULL"
		}
		return ptr.Format(time.RFC3339Nano)
	}
}

// unimplementedVariableParser represents a system variable that is not yet implemented.
type unimplementedVariableParser struct {
	name        string
	description string
}

// Name returns the variable name.
func (u *unimplementedVariableParser) Name() string {
	return u.name
}

// Description returns the variable description.
func (u *unimplementedVariableParser) Description() string {
	return u.description
}

// ParseAndSetWithMode always returns an error for unimplemented variables.
func (u *unimplementedVariableParser) ParseAndSetWithMode(value string, mode parseMode) error {
	return fmt.Errorf("%s is not implemented", u.name)
}

// GetValue always returns an error for unimplemented variables.
func (u *unimplementedVariableParser) GetValue() (string, error) {
	return "", fmt.Errorf("%s is not implemented", u.name)
}

// IsReadOnly returns false for unimplemented variables.
func (u *unimplementedVariableParser) IsReadOnly() bool {
	return false
}

// NewUnimplementedVariableParser creates a parser for unimplemented variables.
// These variables return an error when accessed, indicating they are not yet implemented.
// The returned parser integrates with the existing error handling system by wrapping
// errors appropriately.
func NewUnimplementedVariableParser(name, description string) VariableParser {
	return &unimplementedVariableParser{
		name:        name,
		description: description,
	}
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

// FormatProtobufEnum creates a formatter for protobuf enums that:
// 1. Returns empty string for zero value (UNSPECIFIED)
// 2. Strips the given prefix from the enum name
// 3. Falls back to the enum's String() method if available
//
// Example:
//
//	FormatProtobufEnum[sppb.RequestOptions_Priority]("PRIORITY_")
//
// Would format:
//   - PRIORITY_UNSPECIFIED (0) -> ""
//   - PRIORITY_HIGH (3) -> "HIGH"
func FormatProtobufEnum[T ~int32](prefix string) func(T) string {
	return func(v T) string {
		// Return empty string for zero value (UNSPECIFIED)
		if v == 0 {
			return ""
		}

		// Use String() method if available
		if stringer, ok := any(v).(fmt.Stringer); ok {
			s := stringer.String()
			// Strip prefix if present
			if prefix != "" {
				return strings.TrimPrefix(s, prefix)
			}
			return s
		}

		// Fallback to numeric representation
		return fmt.Sprintf("%d", v)
	}
}

// FormatEnumFromMap creates a formatter that performs reverse lookup in the enum map.
// This is useful for local enum types that don't implement fmt.Stringer.
// Returns the type name with value for unmapped values.
//
// Example:
//
//	FormatEnumFromMap("DisplayMode", formatValues)
//
// Would look up the value in formatValues map and return the key.
func FormatEnumFromMap[T comparable](typeName string, enumMap map[string]T) func(T) string {
	return func(v T) string {
		// Reverse lookup in the map
		for name, value := range enumMap {
			if value == v {
				return name
			}
		}
		// Fallback: just return the value as string
		// If the type implements fmt.Stringer or has a custom String() method,
		// it will be used. Otherwise, Go's default formatting is used.
		return fmt.Sprint(v)
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

// SetPointerValue returns a setter function that sets a pointer field to point to the given value.
// This is useful for nullable fields where the field itself is a pointer.
//
// Example:
//
//	setter: SetPointerValue(&sv.QueryMode)
//
// Instead of:
//
//	setter: func(v sppb.ExecuteSqlRequest_QueryMode) error { sv.QueryMode = &v; return nil }
func SetPointerValue[T any](ptr **T) func(T) error {
	return func(v T) error {
		*ptr = &v
		return nil
	}
}

// SliceToEnumMap converts a slice of string-based enum values to a map for use with enum parsers.
// This is useful for enums where the constant value is the same as its string representation.
//
// Example:
//
//	type ExplainFormat string
//	const (
//	    ExplainFormatCurrent = "CURRENT"
//	    ExplainFormatCompact = "COMPACT"
//	)
//	enumMap := SliceToEnumMap([]ExplainFormat{ExplainFormatCurrent, ExplainFormatCompact})
//	// Results in: map[string]ExplainFormat{"CURRENT": "CURRENT", "COMPACT": "COMPACT"}
func SliceToEnumMap[T ~string](values []T) map[string]T {
	m := make(map[string]T, len(values))
	for _, v := range values {
		m[string(v)] = v
	}
	return m
}

// BuildEnumMapWithAliases creates an enum map from values and their aliases.
// For string-based enums, the primary value is automatically derived using string(v).
// For other types that implement fmt.Stringer, it uses v.String().
// Additional aliases can be specified in the aliases map.
//
// Example:
//
//	// For string enums:
//	enumMap := BuildEnumMapWithAliases(
//	    []parseMode{parseModeUnspecified, parseModeFallback, parseModeNoMemefish},
//	    map[parseMode][]string{
//	        parseModeUnspecified: {"UNSPECIFIED"},  // "" is auto-added, this adds "UNSPECIFIED" as alias
//	    },
//	)
//
//	// For protobuf enums:
//	enumMap := BuildEnumMapWithAliases(
//	    []sppb.ExecuteSqlRequest_QueryMode{
//	        sppb.ExecuteSqlRequest_NORMAL,
//	        sppb.ExecuteSqlRequest_PLAN,
//	        sppb.ExecuteSqlRequest_PROFILE,
//	    },
//	    map[sppb.ExecuteSqlRequest_QueryMode][]string{
//	        sppb.ExecuteSqlRequest_PROFILE: {"WITH_STATS"},  // "PROFILE" is auto-added
//	    },
//	)
func BuildEnumMapWithAliases[T comparable](values []T, aliases map[T][]string) map[string]T {
	m := make(map[string]T)

	for _, v := range values {
		// Get the primary string representation
		var primaryKey string
		if str, ok := any(v).(string); ok {
			primaryKey = str
		} else if stringer, ok := any(v).(fmt.Stringer); ok {
			primaryKey = stringer.String()
		} else {
			// For types that don't implement Stringer, use fmt.Sprint
			primaryKey = fmt.Sprint(v)
		}

		// Add the primary key
		m[primaryKey] = v

		// Add any aliases
		if aliasKeys, ok := aliases[v]; ok {
			for _, alias := range aliasKeys {
				m[alias] = v
			}
		}
	}

	return m
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

// GetConvertedValue returns a getter function that retrieves a value and converts it to another type.
// This is useful when the storage type differs from the variable type.
//
// Example:
//
//	getter: GetConvertedValue(&sv.Port, func(p int) int64 { return int64(p) })
//
// Or with built-in conversions:
//
//	getter: GetConvertedValue(&sv.Port, int64)
func GetConvertedValue[From, To any](ptr *From, convert func(From) To) func() To {
	return func() To {
		return convert(*ptr)
	}
}

// GetValueOrDefault returns a getter function for nullable fields that returns a default value when nil.
// This is useful for optional configuration with defaults.
//
// Example:
//
//	getter: GetValueOrDefault(&sv.QueryMode, sppb.ExecuteSqlRequest_NORMAL)
func GetValueOrDefault[T any](ptr **T, defaultValue T) func() T {
	return func() T {
		if *ptr == nil {
			return defaultValue
		}
		return **ptr
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

// CreateStringEnumVariableParser creates a parser for simple string-to-string mappings.
// Use this for configuration values where the key and value are identical strings.
// Example: log levels like "DEBUG" -> "DEBUG", "INFO" -> "INFO"
// This differs from NewSimpleEnumParser which is for typed enums (type MyEnum string).
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

// RegisterProtobufEnum is the simplest way to register a protobuf enum variable.
// It automatically handles all the common patterns:
// - Extracts values from the _value map
// - Strips prefixes for user-friendly names
// - Formats output with prefix stripped
//
// Example:
//
//	sysvar.RegisterProtobufEnum(registry,
//	    "RPC_PRIORITY", "Priority for requests",
//	    sppb.RequestOptions_Priority_value, "PRIORITY_",
//	    sysvar.GetValue(&sv.RPCPriority),
//	    sysvar.SetValue(&sv.RPCPriority),
//	)
func RegisterProtobufEnum[T ~int32](
	registry *Registry,
	name, description string,
	enumValueMap map[string]int32,
	prefix string,
	getter func() T,
	setter func(T) error,
) {
	enumMap := BuildProtobufEnumMap[T](enumValueMap, prefix, nil)
	formatter := FormatProtobufEnum[T](prefix)
	parser := NewEnumVariableParser(name, description, enumMap, getter, setter, formatter)

	if err := registry.Register(parser); err != nil {
		panic(fmt.Sprintf("Failed to register %s: %v", name, err))
	}
}

// RegisterProtobufEnumWithAliases registers a protobuf enum variable with custom aliases.
// This is the same as RegisterProtobufEnum but allows specifying additional aliases
// for enum values (like "" or "UNSPECIFIED" for the zero value).
//
// Example:
//
//	sysvar.RegisterProtobufEnumWithAliases(registry,
//	    "DEFAULT_ISOLATION_LEVEL", "Transaction isolation level",
//	    sppb.TransactionOptions_IsolationLevel_value, "ISOLATION_LEVEL_",
//	    sysvar.GetValue(&sv.DefaultIsolationLevel),
//	    sysvar.SetValue(&sv.DefaultIsolationLevel),
//	    map[sppb.TransactionOptions_IsolationLevel][]string{
//	        sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED: {"UNSPECIFIED"},
//	    },
//	)
func RegisterProtobufEnumWithAliases[T ~int32](
	registry *Registry,
	name, description string,
	enumValueMap map[string]int32,
	prefix string,
	getter func() T,
	setter func(T) error,
	aliases map[T][]string,
) {
	enumMap := BuildProtobufEnumMap[T](enumValueMap, prefix, aliases)
	formatter := FormatProtobufEnum[T](prefix)
	parser := NewEnumVariableParser(name, description, enumMap, getter, setter, formatter)

	if err := registry.Register(parser); err != nil {
		panic(fmt.Sprintf("Failed to register %s: %v", name, err))
	}
}

// CreateProtobufEnumParser creates a parser for protobuf enums with custom getter/setter logic.
// Use this when you need special handling like nullable fields or default values.
// For simple cases, use RegisterProtobufEnum instead.
func CreateProtobufEnumParser[T ~int32](
	name, description string,
	enumValueMap map[string]int32,
	prefix string,
	getter func() T,
	setter func(T) error,
) VariableParser {
	enumMap := BuildProtobufEnumMap[T](enumValueMap, prefix, nil)
	formatter := FormatProtobufEnum[T](prefix)
	return NewEnumVariableParser(name, description, enumMap, getter, setter, formatter)
}

// CreateProtobufEnumParserWithAliases creates a parser for protobuf enums with aliases.
// This is the same as CreateProtobufEnumParser but allows specifying additional aliases.
func CreateProtobufEnumParserWithAliases[T ~int32](
	name, description string,
	enumValueMap map[string]int32,
	prefix string,
	getter func() T,
	setter func(T) error,
	aliases map[T][]string,
) VariableParser {
	enumMap := BuildProtobufEnumMap[T](enumValueMap, prefix, aliases)
	formatter := FormatProtobufEnum[T](prefix)
	return NewEnumVariableParser(name, description, enumMap, getter, setter, formatter)
}

// BuildProtobufEnumMap creates an enum map from protobuf enum values with automatic prefix handling.
// This leverages the fact that protobuf enums generated by protoc-gen-go follow consistent patterns:
// - They have a _value map with all string representations
// - They implement fmt.Stringer
// - They often have prefixes that should be stripped for user-facing values
//
// Example:
//
//	enumMap := BuildProtobufEnumMap(
//	    sppb.TransactionOptions_IsolationLevel_value,
//	    "ISOLATION_LEVEL_",
//	    map[sppb.TransactionOptions_IsolationLevel][]string{
//	        sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED: {"UNSPECIFIED"},
//	    },
//	)
func BuildProtobufEnumMap[T ~int32](enumValueMap map[string]int32, prefix string, aliases map[T][]string) map[string]T {
	m := make(map[string]T)

	// Add all values from the protobuf _value map
	for name, value := range enumValueMap {
		typedValue := T(value)
		// Add the full name
		m[name] = typedValue
		// Add the name with prefix stripped
		if prefix != "" {
			stripped := strings.TrimPrefix(name, prefix)
			if stripped != name { // Only add if prefix was actually present
				m[stripped] = typedValue
			}
		}
	}

	// Add any additional aliases
	for value, aliasKeys := range aliases {
		for _, alias := range aliasKeys {
			m[alias] = value
		}
	}

	return m
}
