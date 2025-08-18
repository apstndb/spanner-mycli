package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseGoogleSQLValue(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		// String literals
		{"single quoted string", "'hello'", "hello"},
		{"double quoted string", `"world"`, "world"},
		{"string with escapes", `'hello\nworld'`, "hello\nworld"},
		{"string with quotes", `"it's a test"`, "it's a test"},
		{"empty string", "''", ""},

		// Boolean literals
		{"TRUE uppercase", "TRUE", "true"},
		{"FALSE uppercase", "FALSE", "false"},
		{"true lowercase", "true", "true"},
		{"false lowercase", "false", "false"},
		{"True mixed case", "True", "true"},

		// Other values pass through
		{"number", "123", "123"},
		{"identifier", "SOME_VALUE", "SOME_VALUE"},
		{"NULL", "NULL", "NULL"},

		// Invalid expressions pass through
		{"unclosed quote", "'hello", "'hello"},
		{"mixed quotes", `"'hello'"`, "'hello'"},
		{"invalid syntax", "'''", "'''"},

		// Whitespace handling
		{"string with spaces", "  'hello'  ", "hello"},
		{"boolean with spaces", "  TRUE  ", "true"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseGoogleSQLValue(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}
