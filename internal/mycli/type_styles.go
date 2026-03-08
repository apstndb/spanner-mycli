// Copyright 2025 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mycli

import (
	"fmt"
	"strconv"
	"strings"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
)

// defaultTypeStyles is the default value for CLI_TYPE_STYLES.
// NULL values are displayed with dim styling by default.
const defaultTypeStyles = "NULL=dim"

// namedStyles maps human-readable style/color names to SGR parameter numbers.
var namedStyles = map[string]string{
	// Attributes
	"bold":          "1",
	"dim":           "2",
	"italic":        "3",
	"underline":     "4",
	"blink":         "5",
	"reverse":       "7",
	"hidden":        "8",
	"strikethrough": "9",
	// Foreground colors
	"black":   "30",
	"red":     "31",
	"green":   "32",
	"yellow":  "33",
	"blue":    "34",
	"magenta": "35",
	"cyan":    "36",
	"white":   "37",
}

// typeCodeNames maps type name strings to Spanner TypeCode values.
var typeCodeNames = map[string]sppb.TypeCode{
	"BOOL":      sppb.TypeCode_BOOL,
	"INT64":     sppb.TypeCode_INT64,
	"FLOAT32":   sppb.TypeCode_FLOAT32,
	"FLOAT64":   sppb.TypeCode_FLOAT64,
	"NUMERIC":   sppb.TypeCode_NUMERIC,
	"STRING":    sppb.TypeCode_STRING,
	"BYTES":     sppb.TypeCode_BYTES,
	"JSON":      sppb.TypeCode_JSON,
	"DATE":      sppb.TypeCode_DATE,
	"TIMESTAMP": sppb.TypeCode_TIMESTAMP,
	"ARRAY":     sppb.TypeCode_ARRAY,
	"STRUCT":    sppb.TypeCode_STRUCT,
	"PROTO":     sppb.TypeCode_PROTO,
	"ENUM":      sppb.TypeCode_ENUM,
	"INTERVAL":  sppb.TypeCode_INTERVAL,
	"UUID":      sppb.TypeCode_UUID,
}

// typeStyleConfig holds the parsed result of CLI_TYPE_STYLES.
type typeStyleConfig struct {
	typeStyles map[sppb.TypeCode]string // TypeCode → ANSI SGR escape sequence
	nullStyle  string                   // ANSI SGR escape sequence for NULL; empty = no styling
}

// parseTypeStyles parses a CLI_TYPE_STYLES value string.
// Format: colon-separated TYPE=STYLE pairs (e.g., "STRING=green:INT64=bold:NULL=dim").
// Style values support named colors/attributes and raw SGR numbers, separated by semicolons.
// Returns an error if any type name or style value is invalid.
func parseTypeStyles(s string) (typeStyleConfig, error) {
	var config typeStyleConfig
	s = strings.TrimSpace(s)
	if s == "" {
		return config, nil
	}

	config.typeStyles = make(map[sppb.TypeCode]string)

	for _, pair := range strings.Split(s, ":") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}

		key, value, ok := strings.Cut(pair, "=")
		if !ok {
			return typeStyleConfig{}, fmt.Errorf("invalid type style pair %q: expected TYPE=STYLE", pair)
		}

		key = strings.TrimSpace(strings.ToUpper(key))
		value = strings.TrimSpace(value)

		if value == "" {
			return typeStyleConfig{}, fmt.Errorf("empty style value for type %q", key)
		}

		// Resolve style value to SGR parameters
		sgr, err := resolveStyle(value)
		if err != nil {
			return typeStyleConfig{}, fmt.Errorf("invalid style for type %q: %w", key, err)
		}

		// Convert SGR params to full escape sequence
		escape := "\033[" + sgr + "m"

		if key == "NULL" {
			config.nullStyle = escape
		} else if tc, ok := typeCodeNames[key]; ok {
			config.typeStyles[tc] = escape
		} else {
			return typeStyleConfig{}, fmt.Errorf("unknown type name %q", key)
		}
	}

	return config, nil
}

// resolveStyle converts a style value (names and/or raw SGR numbers) to SGR parameter string.
// Supports semicolon-separated values: "bold;green" → "1;32", "38;5;214" → "38;5;214".
func resolveStyle(value string) (string, error) {
	parts := strings.Split(value, ";")
	resolved := make([]string, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if sgr, ok := namedStyles[strings.ToLower(part)]; ok {
			resolved = append(resolved, sgr)
		} else if _, err := strconv.Atoi(part); err == nil {
			// Raw SGR number
			resolved = append(resolved, part)
		} else {
			return "", fmt.Errorf("unknown style %q (expected a color name like 'red', an attribute like 'bold', or an SGR number)", part)
		}
	}

	if len(resolved) == 0 {
		return "", fmt.Errorf("empty style value")
	}

	return strings.Join(resolved, ";"), nil
}

// formatTypeStyles converts a typeStyleConfig back to a CLI_TYPE_STYLES string for display.
func formatTypeStyles(config typeStyleConfig) string {
	if len(config.typeStyles) == 0 && config.nullStyle == "" {
		return ""
	}

	var parts []string

	// Output type styles in a stable order (sorted by TypeCode numeric value)
	for _, name := range sortedTypeNames() {
		tc := typeCodeNames[name]
		if escape, ok := config.typeStyles[tc]; ok {
			parts = append(parts, name+"="+escapeToDisplay(escape))
		}
	}

	// NULL at the end
	if config.nullStyle != "" {
		parts = append(parts, "NULL="+escapeToDisplay(config.nullStyle))
	}

	return strings.Join(parts, ":")
}

// sortedTypeNames returns type names in TypeCode numeric order for stable output.
func sortedTypeNames() []string {
	// TypeCode values in numeric order
	return []string{
		"BOOL", "INT64", "FLOAT64", "FLOAT32",
		"TIMESTAMP", "DATE", "STRING", "BYTES",
		"ARRAY", "STRUCT", "NUMERIC", "JSON",
		"PROTO", "ENUM", "INTERVAL", "UUID",
	}
}

// escapeToDisplay converts an ANSI escape sequence back to its SGR parameter string.
// e.g., "\033[1;32m" → "1;32"
func escapeToDisplay(escape string) string {
	s := strings.TrimPrefix(escape, "\033[")
	s = strings.TrimSuffix(s, "m")
	return s
}
