package parser_test

import (
	"testing"

	"github.com/apstndb/spanner-mycli/internal/parser"
)

func TestParseGoogleSQLStringLiteral(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		// Valid string literals
		{
			name:  "single quoted string",
			input: "'hello world'",
			want:  "hello world",
		},
		{
			name:  "double quoted string",
			input: `"hello world"`,
			want:  "hello world",
		},
		{
			name:  "string with escaped quotes",
			input: `'it\'s a test'`,
			want:  "it's a test",
		},
		{
			name:  "string with newline escape",
			input: `'line1\nline2'`,
			want:  "line1\nline2",
		},
		{
			name:  "raw string",
			input: `r'no\nescape'`,
			want:  `no\nescape`,
		},
		{
			name:  "bytes literal",
			input: `b'bytes'`,
			want:  "bytes",
		},
		{
			name: "triple quoted string",
			input: `'''multi
line'''`,
			want: `multi
line`,
		},

		// Invalid inputs
		{
			name:    "unquoted identifier",
			input:   "hello",
			wantErr: true,
		},
		{
			name:    "number",
			input:   "123",
			wantErr: true,
		},
		{
			name:    "boolean",
			input:   "TRUE",
			wantErr: true,
		},
		{
			name:    "NULL",
			input:   "NULL",
			wantErr: true,
		},
		{
			name:    "unclosed string",
			input:   "'unclosed",
			wantErr: true,
		},
		{
			name:    "string with trailing content",
			input:   "'hello' world",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.ParseGoogleSQLStringLiteral(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseGoogleSQLStringLiteral() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseGoogleSQLStringLiteral() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGoogleSQLStringLiteralParser(t *testing.T) {
	// Test that the parser instance works correctly
	parser := parser.GoogleSQLStringLiteralParser

	// Valid string
	got, err := parser.Parse("'test string'")
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}
	if got != "test string" {
		t.Errorf("Parse() = %q, want %q", got, "test string")
	}

	// Invalid input (identifier)
	_, err = parser.Parse("unquoted")
	if err == nil {
		t.Error("Expected error for unquoted identifier, got nil")
	}
}

func TestDualModeStringParser(t *testing.T) {
	t.Run("GoogleSQL mode", func(t *testing.T) {
		// Should only accept string literals
		got, err := parser.DualModeStringParser.ParseWithMode("'quoted string'", parser.ParseModeGoogleSQL)
		if err != nil {
			t.Fatalf("ParseWithMode() failed: %v", err)
		}
		if got != "quoted string" {
			t.Errorf("ParseWithMode() = %q, want %q", got, "quoted string")
		}

		// Should reject identifiers
		_, err = parser.DualModeStringParser.ParseWithMode("unquoted", parser.ParseModeGoogleSQL)
		if err == nil {
			t.Error("Expected error for unquoted identifier in GoogleSQL mode")
		}
	})

	t.Run("Simple mode", func(t *testing.T) {
		// Should accept any string as-is
		got, err := parser.DualModeStringParser.ParseWithMode("unquoted value", parser.ParseModeSimple)
		if err != nil {
			t.Fatalf("ParseWithMode() failed: %v", err)
		}
		if got != "unquoted value" {
			t.Errorf("ParseWithMode() = %q, want %q", got, "unquoted value")
		}

		// Should preserve quotes if present
		got, err = parser.DualModeStringParser.ParseWithMode("'quoted'", parser.ParseModeSimple)
		if err != nil {
			t.Fatalf("ParseWithMode() failed: %v", err)
		}
		if got != "'quoted'" {
			t.Errorf("ParseWithMode() = %q, want %q", got, "'quoted'")
		}
	})
}
