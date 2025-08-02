package main

import (
	"fmt"
	"strings"

	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
)

// EnumVar handles enum-like variables
type EnumVar[T comparable] struct {
	ptr         *T
	values      map[string]T
	description string
}

func (e *EnumVar[T]) Get() (string, error) {
	for name, value := range e.values {
		if value == *e.ptr {
			return name, nil
		}
	}
	// Fallback to string representation if not found in map
	return fmt.Sprintf("%v", *e.ptr), nil
}

func (e *EnumVar[T]) Set(value string) error {
	upperValue := strings.ToUpper(value)
	v, ok := e.values[upperValue]
	if !ok {
		// Try without case conversion for some enums
		v, ok = e.values[value]
		if !ok {
			validValues := make([]string, 0, len(e.values))
			for k := range e.values {
				validValues = append(validValues, k)
			}
			return fmt.Errorf("invalid value \"%s\", must be one of: %s", value, strings.Join(validValues, ", "))
		}
	}
	*e.ptr = v
	return nil
}

func (e *EnumVar[T]) Description() string {
	return e.description
}

func (e *EnumVar[T]) IsReadOnly() bool {
	return false
}

// ProtoEnumVar handles Protocol Buffer enum variables
type ProtoEnumVar[T ~int32] struct {
	ptr         *T
	values      map[string]int32
	prefix      string
	aliases     map[string]string // Additional aliases for enum values
	description string
}

func (p *ProtoEnumVar[T]) Get() (string, error) {
	intValue := int32(*p.ptr)

	// First check for exact matches
	for name, value := range p.values {
		if value == intValue {
			// Remove prefix for display
			if p.prefix != "" && strings.HasPrefix(name, p.prefix) {
				return strings.TrimPrefix(name, p.prefix), nil
			}
			return name, nil
		}
	}

	// If not found, return the numeric value
	return fmt.Sprintf("%d", intValue), nil
}

func (p *ProtoEnumVar[T]) Set(value string) error {
	// Try with prefix first
	if p.prefix != "" {
		prefixedValue := p.prefix + strings.ToUpper(value)
		if v, ok := p.values[prefixedValue]; ok {
			*p.ptr = T(v)
			return nil
		}
	}

	// Try exact match
	upperValue := strings.ToUpper(value)
	if v, ok := p.values[upperValue]; ok {
		*p.ptr = T(v)
		return nil
	}

	// Try aliases
	if p.aliases != nil {
		if aliasValue, ok := p.aliases[upperValue]; ok {
			if v, ok := p.values[aliasValue]; ok {
				*p.ptr = T(v)
				return nil
			}
		}
	}

	// Try numeric value
	var intValue int32
	if _, err := fmt.Sscanf(value, "%d", &intValue); err == nil {
		// Validate it's a known value
		for _, v := range p.values {
			if v == intValue {
				*p.ptr = T(intValue)
				return nil
			}
		}
	}

	validValues := make([]string, 0, len(p.values))
	for k := range p.values {
		displayName := k
		if p.prefix != "" && strings.HasPrefix(k, p.prefix) {
			displayName = strings.TrimPrefix(k, p.prefix)
		}
		validValues = append(validValues, displayName)
	}

	return fmt.Errorf("invalid value \"%s\", must be one of: %s", value, strings.Join(validValues, ", "))
}

func (p *ProtoEnumVar[T]) Description() string {
	return p.description
}

func (p *ProtoEnumVar[T]) IsReadOnly() bool {
	return false
}

// Helper functions to create proto enum handlers for common types

func RPCPriorityVar(ptr *sppb.RequestOptions_Priority, desc string) *ProtoEnumVar[sppb.RequestOptions_Priority] {
	return &ProtoEnumVar[sppb.RequestOptions_Priority]{
		ptr:         ptr,
		values:      sppb.RequestOptions_Priority_value,
		prefix:      "PRIORITY_",
		description: desc,
	}
}

func IsolationLevelVar(ptr *sppb.TransactionOptions_IsolationLevel, desc string) *ProtoEnumVar[sppb.TransactionOptions_IsolationLevel] {
	return &ProtoEnumVar[sppb.TransactionOptions_IsolationLevel]{
		ptr:    ptr,
		values: sppb.TransactionOptions_IsolationLevel_value,
		prefix: "ISOLATION_LEVEL_",
		aliases: map[string]string{
			"UNSPECIFIED": "ISOLATION_LEVEL_UNSPECIFIED",
		},
		description: desc,
	}
}

func DatabaseDialectVar(ptr *databasepb.DatabaseDialect, desc string) *ProtoEnumVar[databasepb.DatabaseDialect] {
	return &ProtoEnumVar[databasepb.DatabaseDialect]{
		ptr:    ptr,
		values: databasepb.DatabaseDialect_value,
		aliases: map[string]string{
			"": "DATABASE_DIALECT_UNSPECIFIED",
		},
		description: desc,
	}
}

func QueryModeVar(ptr *sppb.ExecuteSqlRequest_QueryMode, desc string) *ProtoEnumVar[sppb.ExecuteSqlRequest_QueryMode] {
	return &ProtoEnumVar[sppb.ExecuteSqlRequest_QueryMode]{
		ptr:         ptr,
		values:      sppb.ExecuteSqlRequest_QueryMode_value,
		description: desc,
	}
}

// DisplayModeVar creates an enum handler for DisplayMode
func DisplayModeVar(ptr *DisplayMode, desc string) *EnumVar[DisplayMode] {
	return &EnumVar[DisplayMode]{
		ptr: ptr,
		values: map[string]DisplayMode{
			"TABLE":                DisplayModeTable,
			"TABLE_COMMENT":        DisplayModeTableComment,
			"TABLE_DETAIL_COMMENT": DisplayModeTableDetailComment,
			"VERTICAL":             DisplayModeVertical,
			"TAB":                  DisplayModeTab,
			"CSV":                  DisplayModeCSV,
			"HTML":                 DisplayModeHTML,
			"XML":                  DisplayModeXML,
		},
		description: desc,
	}
}

// ParseModeVar creates an enum handler for parseMode
func ParseModeVar(ptr *parseMode, desc string) *EnumVar[parseMode] {
	return &EnumVar[parseMode]{
		ptr: ptr,
		values: map[string]parseMode{
			"FALLBACK":      parseModeFallback,
			"NO_MEMEFISH":   parseModeNoMemefish,
			"MEMEFISH_ONLY": parseMemefishOnly,
			"UNSPECIFIED":   parseModeUnspecified,
		},
		description: desc,
	}
}

// ExplainFormatVar creates an enum handler for explainFormat
func ExplainFormatVar(ptr *explainFormat, desc string) *EnumVar[explainFormat] {
	return &EnumVar[explainFormat]{
		ptr: ptr,
		values: map[string]explainFormat{
			"CURRENT":     explainFormatCurrent,
			"TRADITIONAL": explainFormatTraditional,
			"COMPACT":     explainFormatCompact,
		},
		description: desc,
	}
}
