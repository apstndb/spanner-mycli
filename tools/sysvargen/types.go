// Package sysvargen provides types and utilities for generating system variable registration code.
package sysvargen

import (
	"reflect"
	"strings"
)

// SysVarTag represents the parsed information from a struct tag.
type SysVarTag struct {
	Name        string
	Description string
	Type        string // "bool", "string", "int", "proto_enum", "enum", "nullable_duration", "nullable_int"
	ReadOnly    bool
	Setter      string // custom setter method name
	Getter      string // custom getter method name
	Prefix      string // for proto enums
	ProtoType   string // full proto type name
	EnumValues  string // comma-separated enum values
	Min         string // minimum value for range validation
	Max         string // maximum value for range validation
	Default     string // default value for nullable types
	Aliases     string // aliases for enum values (format: "ENUM_VALUE:alias1,alias2;ANOTHER_VALUE:alias3")
}

// ParseSysVarTag parses a sysvar struct tag.
// Example: `sysvar:"name=READONLY,desc='Read-only mode',setter=custom"`
func ParseSysVarTag(tag string) (*SysVarTag, error) {
	if tag == "" || tag == "-" {
		return nil, nil
	}

	result := &SysVarTag{}
	parts := splitTagParts(tag)

	for _, part := range parts {
		key, value := splitKeyValue(part)
		switch key {
		case "name":
			result.Name = value
		case "desc":
			result.Description = unquoteValue(value)
		case "type":
			result.Type = value
		case "readonly":
			result.ReadOnly = true
		case "setter":
			result.Setter = value
		case "getter":
			result.Getter = value
		case "prefix":
			result.Prefix = value
		case "proto":
			result.ProtoType = value
		case "enum":
			result.EnumValues = value
		case "min":
			result.Min = value
		case "max":
			result.Max = value
		case "default":
			result.Default = value
		case "aliases":
			result.Aliases = value
		}
	}

	return result, nil
}

// splitTagParts splits tag into parts, respecting quoted strings.
func splitTagParts(tag string) []string {
	var parts []string
	var current strings.Builder
	inQuote := false
	quoteChar := rune(0)
	prevChar := rune(0)

	for _, r := range tag {
		switch {
		case !inQuote && (r == '\'' || r == '"'):
			inQuote = true
			quoteChar = r
			current.WriteRune(r)
		case inQuote && r == quoteChar && prevChar != '\\':
			inQuote = false
			quoteChar = 0
			current.WriteRune(r)
		case !inQuote && r == ',':
			if current.Len() > 0 {
				parts = append(parts, strings.TrimSpace(current.String()))
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
		prevChar = r
	}

	if current.Len() > 0 {
		parts = append(parts, strings.TrimSpace(current.String()))
	}

	return parts
}

// splitKeyValue splits "key=value" into key and value.
func splitKeyValue(part string) (string, string) {
	idx := strings.Index(part, "=")
	if idx == -1 {
		return part, ""
	}
	return part[:idx], part[idx+1:]
}

// unquoteValue removes surrounding quotes from a value.
func unquoteValue(value string) string {
	if len(value) >= 2 {
		if (value[0] == '\'' && value[len(value)-1] == '\'') ||
			(value[0] == '"' && value[len(value)-1] == '"') {
			// Also unescape any escaped quotes
			unquoted := value[1 : len(value)-1]
			replacer := strings.NewReplacer(
				`\'`, "'",
				`\"`, `"`,
				`\\`, `\`,
			)
			return replacer.Replace(unquoted)
		}
	}
	return value
}

// VarDefinition represents a parsed variable definition from struct analysis.
type VarDefinition struct {
	FieldName  string
	FieldType  reflect.Type
	Tag        *SysVarTag
	StructName string
}

// NeedsCustomParser returns true if this variable needs custom parsing logic.
func (v *VarDefinition) NeedsCustomParser() bool {
	// Variables with custom setters or special types need custom parsers
	return v.Tag.Setter != "" || v.Tag.Type == "proto_enum" || v.Tag.Type == "enum"
}

// IsUnderscoreField returns true if this is an underscore field (computed variable).
func (v *VarDefinition) IsUnderscoreField() bool {
	return v.FieldName == "_"
}

// GetParserType returns the appropriate parser type for this variable.
func (v *VarDefinition) GetParserType() string {
	if v.Tag.Type != "" {
		return v.Tag.Type
	}

	// Infer from field type
	switch v.FieldType.Kind() {
	case reflect.Bool:
		return "bool"
	case reflect.String:
		return "string"
	case reflect.Int, reflect.Int32, reflect.Int64:
		return "int"
	default:
		// For read-only variables with custom getters that return strings,
		// treat them as string type
		if v.Tag.ReadOnly && v.Tag.Getter != "" {
			return "string"
		}
		return "unknown"
	}
}
