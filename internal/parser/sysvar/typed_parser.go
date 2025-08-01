package sysvar

import (
	"fmt"
	"strconv"
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

// NewBooleanParser creates a boolean variable parser.
func NewBooleanParser(
	name string,
	description string,
	getter func() bool,
	setter func(bool) error,
) VariableParser {
	return &TypedVariableParser[bool]{
		name:        name,
		description: description,
		parser:      parser.DualModeBoolParser,
		setter:      setter,
		getter:      getter,
		formatter:   func(v bool) string { return strings.ToUpper(strconv.FormatBool(v)) },
		readOnly:    setter == nil,
	}
}

// NewSimpleBooleanParser creates a boolean variable parser from a pointer.
// This is a convenience function for simple cases where the variable is just a field.
func NewSimpleBooleanParser(
	name string,
	description string,
	field *bool,
) VariableParser {
	return NewBooleanParser(
		name,
		description,
		func() bool { return *field },
		func(v bool) error {
			*field = v
			return nil
		},
	)
}

// NewStringParser creates a string variable parser.
func NewStringParser(
	name string,
	description string,
	getter func() string,
	setter func(string) error,
) VariableParser {
	return &TypedVariableParser[string]{
		name:        name,
		description: description,
		parser:      parser.DualModeStringParser,
		setter:      setter,
		getter:      getter,
		formatter:   func(v string) string { return v },
		readOnly:    setter == nil,
	}
}

// NewSimpleStringParser creates a string variable parser from a pointer.
// This is a convenience function for simple cases where the variable is just a field.
func NewSimpleStringParser(
	name string,
	description string,
	field *string,
) VariableParser {
	return NewStringParser(
		name,
		description,
		func() string { return *field },
		func(v string) error {
			*field = v
			return nil
		},
	)
}

// NewSimpleIntegerParser creates an integer variable parser from a pointer.
// This is a convenience function for simple cases where the variable is just a field.
func NewSimpleIntegerParser(
	name string,
	description string,
	field *int64,
) VariableParser {
	return NewIntegerParser(
		name,
		description,
		func() int64 { return *field },
		func(v int64) error {
			*field = v
			return nil
		},
		nil, nil, // No constraints
	)
}

// NewIntegerParser creates an integer variable parser with optional range validation.
func NewIntegerParser(
	name string,
	description string,
	getter func() int64,
	setter func(int64) error,
	min, max *int64,
) VariableParser {
	var p parser.DualModeParser[int64]

	if min != nil || max != nil {
		// Create simple parser with validation
		simpleParser := parser.NewIntParser()

		if min != nil && max != nil {
			simpleParser = simpleParser.WithRange(*min, *max)
		} else if min != nil {
			simpleParser = simpleParser.WithMin(*min)
		} else if max != nil {
			simpleParser = simpleParser.WithMax(*max)
		}

		// Create GoogleSQL parser with same validation
		var googleSQLParser parser.Parser[int64]
		if min != nil && max != nil {
			googleSQLParser = parser.WithValidation(parser.GoogleSQLIntParser, func(v int64) error {
				if v < *min {
					return fmt.Errorf("value %d is less than minimum %d", v, *min)
				}
				if v > *max {
					return fmt.Errorf("value %d is greater than maximum %d", v, *max)
				}
				return nil
			})
		} else if min != nil {
			googleSQLParser = parser.WithValidation(parser.GoogleSQLIntParser, func(v int64) error {
				if v < *min {
					return fmt.Errorf("value %d is less than minimum %d", v, *min)
				}
				return nil
			})
		} else if max != nil {
			googleSQLParser = parser.WithValidation(parser.GoogleSQLIntParser, func(v int64) error {
				if v > *max {
					return fmt.Errorf("value %d is greater than maximum %d", v, *max)
				}
				return nil
			})
		} else {
			googleSQLParser = parser.GoogleSQLIntParser
		}

		p = parser.NewDualModeParser(googleSQLParser, simpleParser)
	} else {
		p = parser.DualModeIntParser
	}

	return &TypedVariableParser[int64]{
		name:        name,
		description: description,
		parser:      p,
		setter:      setter,
		getter:      getter,
		formatter:   func(v int64) string { return strconv.FormatInt(v, 10) },
		readOnly:    setter == nil,
	}
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
	return &TypedVariableParser[T]{
		name:        name,
		description: description,
		parser:      parser.CreateDualModeEnumParser(values),
		setter:      setter,
		getter:      getter,
		formatter:   formatter,
		readOnly:    setter == nil,
	}
}

// NewEnumParserWithStringer creates an enum parser using the type's String() method.
// This is useful for types that implement fmt.Stringer.
func NewEnumParserWithStringer[T interface {
	comparable
	fmt.Stringer
}](
	name string,
	description string,
	values map[string]T,
	getter func() T,
	setter func(T) error,
) VariableParser {
	return NewEnumParser(name, description, values, getter, setter, func(v T) string {
		return v.String()
	})
}

// NewStringEnumParser creates an enum parser for string-based enum types.
// The formatter simply returns the string value as-is.
func NewStringEnumParser[T ~string](
	name string,
	description string,
	values map[string]T,
	getter func() T,
	setter func(T) error,
) VariableParser {
	return NewEnumParser(name, description, values, getter, setter, func(v T) string {
		return string(v)
	})
}

// NewDurationParser creates a duration variable parser.
func NewDurationParser(
	name string,
	description string,
	getter func() time.Duration,
	setter func(time.Duration) error,
	min, max *time.Duration,
) VariableParser {
	var p parser.DualModeParser[time.Duration]

	if min != nil || max != nil {
		// Create parser with constraints
		simpleParser := parser.NewDurationParser()
		if min != nil && max != nil {
			simpleParser = simpleParser.WithRange(*min, *max)
		} else if min != nil {
			simpleParser = simpleParser.WithMin(*min)
		} else if max != nil {
			simpleParser = simpleParser.WithMax(*max)
		}

		// For GoogleSQL mode, wrap with validation
		var googleSQLParser parser.Parser[time.Duration]
		if min != nil || max != nil {
			googleSQLParser = parser.WithValidation(parser.GoogleSQLDurationParser, func(v time.Duration) error {
				if min != nil && v < *min {
					return fmt.Errorf("duration %s is less than minimum %s", v, *min)
				}
				if max != nil && v > *max {
					return fmt.Errorf("duration %s is greater than maximum %s", v, *max)
				}
				return nil
			})
		} else {
			googleSQLParser = parser.GoogleSQLDurationParser
		}

		p = parser.NewDualModeParser(googleSQLParser, simpleParser)
	} else {
		p = parser.DualModeDurationParser
	}

	return &TypedVariableParser[time.Duration]{
		name:        name,
		description: description,
		parser:      p,
		setter:      setter,
		getter:      getter,
		formatter:   func(v time.Duration) string { return v.String() },
		readOnly:    setter == nil,
	}
}

// NewNullableDurationParser creates a nullable duration variable parser.
func NewNullableDurationParser(
	name string,
	description string,
	getter func() *time.Duration,
	setter func(*time.Duration) error,
	min, max *time.Duration,
) VariableParser {
	var innerParser parser.DualModeParser[time.Duration]

	if min != nil || max != nil {
		// Create parser with constraints
		simpleParser := parser.NewDurationParser()
		if min != nil && max != nil {
			simpleParser = simpleParser.WithRange(*min, *max)
		} else if min != nil {
			simpleParser = simpleParser.WithMin(*min)
		} else if max != nil {
			simpleParser = simpleParser.WithMax(*max)
		}

		// For GoogleSQL mode, wrap with validation
		var googleSQLParser parser.Parser[time.Duration]
		if min != nil || max != nil {
			googleSQLParser = parser.WithValidation(parser.GoogleSQLDurationParser, func(v time.Duration) error {
				if min != nil && v < *min {
					return fmt.Errorf("duration %s is less than minimum %s", v, *min)
				}
				if max != nil && v > *max {
					return fmt.Errorf("duration %s is greater than maximum %s", v, *max)
				}
				return nil
			})
		} else {
			googleSQLParser = parser.GoogleSQLDurationParser
		}

		innerParser = parser.NewDualModeParser(googleSQLParser, simpleParser)
	} else {
		innerParser = parser.DualModeDurationParser
	}

	p := parser.NewNullableDurationParser(innerParser)

	return &TypedVariableParser[*time.Duration]{
		name:        name,
		description: description,
		parser:      p,
		setter:      setter,
		getter:      getter,
		formatter: func(v *time.Duration) string {
			if v == nil {
				return "NULL"
			}
			return v.String()
		},
		readOnly: setter == nil,
	}
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
	return &TypedVariableParser[T]{
		name:        name,
		description: description,
		parser:      parser,
		setter:      setter,
		getter:      getter,
		formatter:   formatter,
		readOnly:    setter == nil,
	}
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
	return &TypedVariableParser[T]{
		name:        name,
		description: description,
		parser: parser.NewDualModeParser(
			parser.NewGoogleSQLEnumParser(enumValues),
			parser.NewEnumParser(enumValues),
		),
		setter:    setter,
		getter:    getter,
		formatter: func(v T) string { return fmt.Sprint(v) },
		readOnly:  setter == nil,
	}
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
	return &TypedVariableParser[string]{
		name:        name,
		description: description,
		parser:      parser.DualModeStringParser,
		setter:      nil, // Read-only
		getter:      getter,
		formatter:   func(v string) string { return v },
		readOnly:    true,
	}
}

// NewReadOnlyBooleanParser creates a read-only boolean variable parser.
func NewReadOnlyBooleanParser(
	name string,
	description string,
	getter func() bool,
) VariableParser {
	return &TypedVariableParser[bool]{
		name:        name,
		description: description,
		parser:      parser.DualModeBoolParser,
		setter:      nil, // Read-only
		getter:      getter,
		formatter:   func(v bool) string { return strings.ToUpper(strconv.FormatBool(v)) },
		readOnly:    true,
	}
}

// NewNullableIntParser creates a nullable integer variable parser.
func NewNullableIntParser(
	name string,
	description string,
	getter func() *int64,
	setter func(*int64) error,
	min, max *int64,
) VariableParser {
	var innerParser parser.DualModeParser[int64]

	if min != nil || max != nil {
		// Create simple parser with validation
		simpleParser := parser.NewIntParser()

		if min != nil && max != nil {
			simpleParser = simpleParser.WithRange(*min, *max)
		} else if min != nil {
			simpleParser = simpleParser.WithMin(*min)
		} else if max != nil {
			simpleParser = simpleParser.WithMax(*max)
		}

		// Create GoogleSQL parser with same validation
		var googleSQLParser parser.Parser[int64]
		if min != nil && max != nil {
			googleSQLParser = parser.WithValidation(parser.GoogleSQLIntParser, func(v int64) error {
				if v < *min {
					return fmt.Errorf("value %d is less than minimum %d", v, *min)
				}
				if v > *max {
					return fmt.Errorf("value %d is greater than maximum %d", v, *max)
				}
				return nil
			})
		} else if min != nil {
			googleSQLParser = parser.WithValidation(parser.GoogleSQLIntParser, func(v int64) error {
				if v < *min {
					return fmt.Errorf("value %d is less than minimum %d", v, *min)
				}
				return nil
			})
		} else if max != nil {
			googleSQLParser = parser.WithValidation(parser.GoogleSQLIntParser, func(v int64) error {
				if v > *max {
					return fmt.Errorf("value %d is greater than maximum %d", v, *max)
				}
				return nil
			})
		} else {
			googleSQLParser = parser.GoogleSQLIntParser
		}

		innerParser = parser.NewDualModeParser(googleSQLParser, simpleParser)
	} else {
		innerParser = parser.DualModeIntParser
	}

	p := parser.NewNullableIntParser(innerParser)

	return &TypedVariableParser[*int64]{
		name:        name,
		description: description,
		parser:      p,
		setter:      setter,
		getter:      getter,
		formatter: func(v *int64) string {
			if v == nil {
				return "NULL"
			}
			return strconv.FormatInt(*v, 10)
		},
		readOnly: setter == nil,
	}
}
