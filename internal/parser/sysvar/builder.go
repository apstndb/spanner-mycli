package sysvar

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/apstndb/spanner-mycli/internal/parser"
)

// VariableBuilder provides a fluent interface for building system variable parsers.
// This reduces boilerplate and makes variable registration more consistent.
type VariableBuilder[T any] struct {
	name        string
	description string
	parser      parser.DualModeParser[T]
	getter      func() T
	setter      func(T) error
	formatter   func(T) string
}

// NewVariable creates a new variable builder.
func NewVariable[T any](name, description string) *VariableBuilder[T] {
	return &VariableBuilder[T]{
		name:        name,
		description: description,
	}
}

// WithParser sets the dual-mode parser.
func (b *VariableBuilder[T]) WithParser(p parser.DualModeParser[T]) *VariableBuilder[T] {
	b.parser = p
	return b
}

// WithGetter sets the getter function.
func (b *VariableBuilder[T]) WithGetter(getter func() T) *VariableBuilder[T] {
	b.getter = getter
	return b
}

// WithSetter sets the setter function.
func (b *VariableBuilder[T]) WithSetter(setter func(T) error) *VariableBuilder[T] {
	b.setter = setter
	return b
}

// WithFormatter sets the formatter function.
func (b *VariableBuilder[T]) WithFormatter(formatter func(T) string) *VariableBuilder[T] {
	b.formatter = formatter
	return b
}

// ReadOnly marks the variable as read-only (no setter).
func (b *VariableBuilder[T]) ReadOnly() *VariableBuilder[T] {
	b.setter = nil
	return b
}

// Build creates the final VariableParser.
func (b *VariableBuilder[T]) Build() VariableParser {
	if b.parser == nil {
		panic(fmt.Sprintf("parser not set for variable %s", b.name))
	}
	if b.getter == nil {
		panic(fmt.Sprintf("getter not set for variable %s", b.name))
	}
	if b.formatter == nil {
		panic(fmt.Sprintf("formatter not set for variable %s", b.name))
	}

	return &TypedVariableParser[T]{
		name:        b.name,
		description: b.description,
		parser:      b.parser,
		getter:      b.getter,
		setter:      b.setter,
		formatter:   b.formatter,
		readOnly:    b.setter == nil,
	}
}

// Common formatter functions

// FormatBool formats a boolean value as uppercase TRUE/FALSE.
func FormatBool(v bool) string {
	return strings.ToUpper(strconv.FormatBool(v))
}

// FormatInt formats an integer value.
func FormatInt(v int64) string {
	return strconv.FormatInt(v, 10)
}

// FormatDuration formats a duration value.
func FormatDuration(v time.Duration) string {
	return v.String()
}

// FormatString formats a string value (identity function).
func FormatString(v string) string {
	return v
}

// FormatStringer formats any type that implements fmt.Stringer.
func FormatStringer[T fmt.Stringer](v T) string {
	return v.String()
}

// FormatNullable creates a formatter for nullable types.
func FormatNullable[T any](innerFormatter func(T) string) func(*T) string {
	return func(v *T) string {
		if v == nil {
			return "NULL"
		}
		return innerFormatter(*v)
	}
}

// RangeParserOptions holds range validation options for parsers.
type RangeParserOptions[T any] struct {
	Min *T
	Max *T
}

// HasRange returns true if any range constraint is set.
func (o *RangeParserOptions[T]) HasRange() bool {
	return o != nil && (o.Min != nil || o.Max != nil)
}

// CreateIntRangeParser creates an integer parser with range validation.
func CreateIntRangeParser(opts *RangeParserOptions[int64]) parser.DualModeParser[int64] {
	if opts == nil || !opts.HasRange() {
		return parser.DualModeIntParser
	}

	// Create simple parser with built-in range validation
	simpleParser := parser.NewIntParser()
	if opts.Min != nil && opts.Max != nil {
		simpleParser = simpleParser.WithRange(*opts.Min, *opts.Max)
	} else if opts.Min != nil {
		simpleParser = simpleParser.WithMin(*opts.Min)
	} else if opts.Max != nil {
		simpleParser = simpleParser.WithMax(*opts.Max)
	}

	// Create dual-mode parser with the same validation
	return parser.CreateDualModeParserWithValidation(
		parser.GoogleSQLIntParser,
		simpleParser,
		parser.CreateRangeValidator(opts.Min, opts.Max),
	)
}

// CreateDurationRangeParser creates a duration parser with range validation.
func CreateDurationRangeParser(opts *RangeParserOptions[time.Duration]) parser.DualModeParser[time.Duration] {
	if opts == nil || !opts.HasRange() {
		return parser.DualModeDurationParser
	}

	// Create simple parser with built-in range validation
	simpleParser := parser.NewDurationParser()
	if opts.Min != nil && opts.Max != nil {
		simpleParser = simpleParser.WithRange(*opts.Min, *opts.Max)
	} else if opts.Min != nil {
		simpleParser = simpleParser.WithMin(*opts.Min)
	} else if opts.Max != nil {
		simpleParser = simpleParser.WithMax(*opts.Max)
	}

	// Create dual-mode parser with the same validation
	return parser.CreateDualModeParserWithValidation(
		parser.GoogleSQLDurationParser,
		simpleParser,
		parser.CreateDurationRangeValidator(opts.Min, opts.Max),
	)
}

// Common validation helpers for system variables

// CreateEnumVariableParser creates a variable parser for enum types with string formatting.
func CreateEnumVariableParser[T comparable](
	name, description string,
	values map[string]T,
	getter func() T,
	setter func(T) error,
	formatter func(T) string,
) VariableParser {
	return NewTypedVariableParser(
		name,
		description,
		parser.CreateDualModeEnumParser(values),
		getter,
		setter,
		formatter,
	)
}

// CreateStringEnumVariableParser creates a variable parser for string-based enums.
func CreateStringEnumVariableParser[T ~string](
	name, description string,
	values map[string]T,
	getter func() T,
	setter func(T) error,
) VariableParser {
	return CreateEnumVariableParser(
		name,
		description,
		values,
		getter,
		setter,
		func(v T) string { return string(v) },
	)
}

// CreateProtobufEnumVariableParser creates a variable parser for protobuf enums with optional prefix stripping.
func CreateProtobufEnumVariableParser[T ~int32](
	name, description string,
	enumMap map[string]int32,
	prefix string,
	getter func() T,
	setter func(T) error,
	formatter func(T) string,
) VariableParser {
	// Build values map with both full and short names
	values := make(map[string]T)
	for enumName, enumValue := range enumMap {
		values[enumName] = T(enumValue)
		if prefix != "" && strings.HasPrefix(enumName, prefix) {
			shortName := strings.TrimPrefix(enumName, prefix)
			values[shortName] = T(enumValue)
		}
	}

	return CreateEnumVariableParser(
		name,
		description,
		values,
		getter,
		setter,
		formatter,
	)
}

// CreateProtobufEnumVariableParserWithAutoFormatter creates a variable parser for protobuf enums
// that automatically strips the prefix in the formatter.
func CreateProtobufEnumVariableParserWithAutoFormatter[T interface {
	~int32
	fmt.Stringer
}](
	name, description string,
	enumMap map[string]int32,
	prefix string,
	getter func() T,
	setter func(T) error,
) VariableParser {
	formatter := func(v T) string {
		fullName := v.String()
		if prefix != "" && strings.HasPrefix(fullName, prefix) {
			return strings.TrimPrefix(fullName, prefix)
		}
		return fullName
	}

	return CreateProtobufEnumVariableParser(
		name,
		description,
		enumMap,
		prefix,
		getter,
		setter,
		formatter,
	)
}
