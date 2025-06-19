package shared

import (
	"fmt"
	"io"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/spf13/cobra"
)

// OutputFormat represents the output format for structured data
type OutputFormat string

const (
	FormatYAML OutputFormat = "yaml"
	FormatJSON OutputFormat = "json"
)

// IsValid checks if the format is supported
func (f OutputFormat) IsValid() bool {
	return f == FormatYAML || f == FormatJSON
}

// ParseFormat parses a string into an OutputFormat
func ParseFormat(s string) (OutputFormat, error) {
	format := OutputFormat(strings.ToLower(s))
	if !format.IsValid() {
		return "", fmt.Errorf("invalid format: %s (valid: yaml, json)", s)
	}
	return format, nil
}

// NewEncoder creates a new encoder for the specified format using goccy/go-yaml
func NewEncoder(w io.Writer, format OutputFormat) *yaml.Encoder {
	switch format {
	case FormatJSON:
		return yaml.NewEncoder(w, yaml.JSON(), yaml.UseJSONMarshaler())
	case FormatYAML:
		return yaml.NewEncoder(w, yaml.UseJSONMarshaler())
	default:
		return yaml.NewEncoder(w, yaml.UseJSONMarshaler()) // Default to YAML
	}
}

// ResolveFormat resolves the output format from command flags
// Handles mutually exclusive --format, --json, --yaml flags (enforced by cobra), defaults to YAML
func ResolveFormat(cmd *cobra.Command) OutputFormat {
	// Check aliases first (these take precedence since they're more specific)
	if jsonFlag, _ := cmd.Flags().GetBool("json"); jsonFlag {
		return FormatJSON
	}
	if yamlFlag, _ := cmd.Flags().GetBool("yaml"); yamlFlag {
		return FormatYAML
	}
	
	// Check main format flag
	formatStr, _ := cmd.Flags().GetString("format")
	format, err := ParseFormat(formatStr)
	if err != nil {
		return FormatYAML // Default fallback for invalid format
	}
	return format
}

// EncodeOutput encodes data to the specified writer using the given format
func EncodeOutput(w io.Writer, format OutputFormat, data interface{}) error {
	encoder := NewEncoder(w, format)
	return encoder.Encode(data)
}

// Marshal marshals data using the specified format
func (f OutputFormat) Marshal(data interface{}) ([]byte, error) {
	options := []yaml.EncodeOption{yaml.UseJSONMarshaler()}
	if f == FormatJSON {
		options = append(options, yaml.JSON())
	}
	return yaml.MarshalWithOptions(data, options...)
}

// Unmarshal unmarshals data using consistent options (format-agnostic)
func Unmarshal(data []byte, v interface{}) error {
	return yaml.UnmarshalWithOptions(data, v, yaml.UseJSONUnmarshaler())
}

