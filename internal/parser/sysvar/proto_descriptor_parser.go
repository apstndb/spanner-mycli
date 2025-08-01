package sysvar

import (
	"fmt"
	"slices"
	"strings"
)

// ProtoDescriptorFileParser is a specialized parser for CLI_PROTO_DESCRIPTOR_FILE.
// It supports both Set (replace all files) and Add (append a file) operations.
type ProtoDescriptorFileParser struct {
	name        string
	description string
	getter      func() []string
	setter      func([]string) error
	appender    func(string) error
	validator   func(string) error // Validates a single file path
}

// NewProtoDescriptorFileParser creates a parser for proto descriptor file lists.
func NewProtoDescriptorFileParser(
	name string,
	description string,
	getter func() []string,
	setter func([]string) error,
	appender func(string) error,
	validator func(string) error,
) AppendableVariableParser {
	return &ProtoDescriptorFileParser{
		name:        name,
		description: description,
		getter:      getter,
		setter:      setter,
		appender:    appender,
		validator:   validator,
	}
}

// Name returns the variable name.
func (p *ProtoDescriptorFileParser) Name() string {
	return p.name
}

// Description returns the variable description.
func (p *ProtoDescriptorFileParser) Description() string {
	return p.description
}

// ParseAndSetWithMode implements VariableParser.
// For GoogleSQL mode, expects a string literal containing comma-separated paths.
// For Simple mode, expects comma-separated paths without quotes.
func (p *ProtoDescriptorFileParser) ParseAndSetWithMode(value string, mode ParseMode) error {
	var paths []string

	switch mode {
	case ParseModeGoogleSQL:
		// Parse as GoogleSQL string literal
		parsed, err := GoogleSQLStringParser.Parse(value)
		if err != nil {
			return fmt.Errorf("invalid string literal: %w", err)
		}
		if parsed == "" {
			paths = []string{}
		} else {
			paths = strings.Split(parsed, ",")
		}
	case ParseModeSimple:
		// Parse as simple comma-separated string
		value = strings.TrimSpace(value)
		if value == "" {
			paths = []string{}
		} else {
			paths = strings.Split(value, ",")
		}
	default:
		return fmt.Errorf("unsupported parse mode: %v", mode)
	}

	// Trim spaces from each path
	for i := range paths {
		paths[i] = strings.TrimSpace(paths[i])
	}

	// Validate each path if validator is provided
	if p.validator != nil {
		for _, path := range paths {
			if err := p.validator(path); err != nil {
				return fmt.Errorf("invalid proto descriptor file %q: %w", path, err)
			}
		}
	}

	return p.setter(paths)
}

// GetValue implements VariableParser.
func (p *ProtoDescriptorFileParser) GetValue() (string, error) {
	files := p.getter()
	return strings.Join(files, ","), nil
}

// IsReadOnly implements VariableParser.
func (p *ProtoDescriptorFileParser) IsReadOnly() bool {
	return p.setter == nil && p.appender == nil
}

// AppendWithMode implements AppendableVariableParser.
// For GoogleSQL mode, expects a string literal containing a single path.
// For Simple mode, expects a single path without quotes.
func (p *ProtoDescriptorFileParser) AppendWithMode(value string, mode ParseMode) error {
	var path string
	var err error

	switch mode {
	case ParseModeGoogleSQL:
		// Parse as GoogleSQL string literal
		path, err = GoogleSQLStringParser.Parse(value)
		if err != nil {
			return fmt.Errorf("invalid string literal: %w", err)
		}
	case ParseModeSimple:
		// Use as-is
		path = strings.TrimSpace(value)
	default:
		return fmt.Errorf("unsupported parse mode: %v", mode)
	}

	// Validate the path if validator is provided
	if p.validator != nil {
		if err := p.validator(path); err != nil {
			return fmt.Errorf("invalid proto descriptor file %q: %w", path, err)
		}
	}

	// Check if already present
	current := p.getter()
	if slices.Contains(current, path) {
		// File already in list, call appender anyway (it might update the descriptor)
		return p.appender(path)
	}

	return p.appender(path)
}
