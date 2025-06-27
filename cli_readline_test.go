package main

import (
	"testing"

	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/token"
	"github.com/fatih/color"
)

func TestLexerHighlighterWithError(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantToken bool
		wantError bool
	}{
		{
			name:      "valid SQL",
			input:     "SELECT * FROM table",
			wantToken: true,
			wantError: false,
		},
		{
			name:      "unclosed string",
			input:     "SELECT 'unclosed",
			wantToken: true,
			wantError: true,
		},
		{
			name:      "unclosed comment",
			input:     "SELECT /* unclosed",
			wantToken: true,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenCount := 0
			errorCount := 0

			highlighter := lexerHighlighterWithError(
				func(tok token.Token) [][]int {
					tokenCount++
					return [][]int{{int(tok.Pos), int(tok.End)}}
				},
				func(me *memefish.Error) bool {
					errorCount++
					return true
				},
			)

			results := highlighter(tt.input, -1)

			if tt.wantToken && tokenCount == 0 {
				t.Error("expected tokens to be processed, but none were")
			}
			if tt.wantError && errorCount == 0 {
				t.Error("expected error to be processed, but none were")
			}
			if !tt.wantError && errorCount > 0 {
				t.Error("unexpected error was processed")
			}
			if len(results) == 0 && (tt.wantToken || tt.wantError) {
				t.Error("expected results, but got none")
			}
		})
	}
}

func TestTokenHighlighter(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		predicate func(token.Token) bool
		wantCount int
	}{
		{
			name:  "highlight keywords",
			input: "SELECT FROM WHERE",
			predicate: func(tok token.Token) bool {
				return alnumRe.MatchString(string(tok.Kind))
			},
			wantCount: 3, // SELECT, FROM, WHERE
		},
		{
			name:  "highlight strings",
			input: `SELECT 'string1', "string2"`,
			predicate: func(tok token.Token) bool {
				return tok.Kind == token.TokenString
			},
			wantCount: 2,
		},
		{
			name:  "highlight numbers",
			input: "SELECT 123, 45.67",
			predicate: func(tok token.Token) bool {
				return tok.Kind == token.TokenInt || tok.Kind == token.TokenFloat
			},
			wantCount: 2,
		},
		{
			name:  "no matches",
			input: "SELECT * FROM table",
			predicate: func(tok token.Token) bool {
				return false // never match
			},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			highlighter := tokenHighlighter(tt.predicate)
			results := highlighter(tt.input, -1)

			if len(results) != tt.wantCount {
				t.Errorf("tokenHighlighter() returned %d results, want %d", len(results), tt.wantCount)
			}
		})
	}
}

func TestKindHighlighter(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		kinds     []token.TokenKind
		wantCount int
	}{
		{
			name:      "highlight single kind",
			input:     `SELECT 'string' FROM table`,
			kinds:     []token.TokenKind{token.TokenString},
			wantCount: 1,
		},
		{
			name:      "highlight multiple kinds",
			input:     `SELECT 'string', 123, @param`,
			kinds:     []token.TokenKind{token.TokenString, token.TokenInt, token.TokenParam},
			wantCount: 3,
		},
		{
			name:      "no matches",
			input:     "SELECT * FROM table",
			kinds:     []token.TokenKind{token.TokenString},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			highlighter := kindHighlighter(tt.kinds...)
			results := highlighter(tt.input, -1)

			if len(results) != tt.wantCount {
				t.Errorf("kindHighlighter() returned %d results, want %d", len(results), tt.wantCount)
			}
		})
	}
}

func TestCommentHighlighter(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantComments int
		wantError    bool
	}{
		{
			name:         "single line comment",
			input:        "-- This is a comment\nSELECT * FROM table",
			wantComments: 1,
			wantError:    false,
		},
		{
			name:         "multi-line comment",
			input:        "/* This is\n   a comment */ SELECT",
			wantComments: 1,
			wantError:    false,
		},
		{
			name:         "multiple comments",
			input:        "-- Comment 1\nSELECT /* Comment 2 */ * FROM table",
			wantComments: 2,
			wantError:    false,
		},
		{
			name:         "unclosed comment",
			input:        "SELECT /* unclosed comment",
			wantComments: 0,
			wantError:    true,
		},
		{
			name:         "no comments",
			input:        "SELECT * FROM table",
			wantComments: 0,
			wantError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			highlighter := commentHighlighter()
			results := highlighter(tt.input, -1)

			// For unclosed comments, we expect one error highlight
			if tt.wantError {
				if len(results) != 1 {
					t.Errorf("commentHighlighter() returned %d results for error case, want 1", len(results))
				}
			} else if len(results) != tt.wantComments {
				t.Errorf("commentHighlighter() returned %d results, want %d", len(results), tt.wantComments)
			}
		})
	}
}

func TestColorToSequence(t *testing.T) {
	// Save original NoColor state
	originalNoColor := color.NoColor
	defer func() { color.NoColor = originalNoColor }()

	tests := []struct {
		name     string
		noColor  bool
		attrs    []color.Attribute
		wantEmpty bool
	}{
		{
			name:      "with color enabled",
			noColor:   false,
			attrs:     []color.Attribute{color.FgRed, color.Bold},
			wantEmpty: false,
		},
		{
			name:      "with color disabled",
			noColor:   true,
			attrs:     []color.Attribute{color.FgRed, color.Bold},
			wantEmpty: true,
		},
		{
			name:      "empty attributes",
			noColor:   false,
			attrs:     []color.Attribute{},
			wantEmpty: false, // Still returns reset sequence
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			color.NoColor = tt.noColor
			result := colorToSequence(tt.attrs...)

			if tt.wantEmpty && result != "" {
				t.Errorf("colorToSequence() = %q, want empty string", result)
			}
			if !tt.wantEmpty && result == "" {
				t.Error("colorToSequence() returned empty string, want non-empty")
			}
		})
	}
}

func TestDefaultHighlights(t *testing.T) {
	// Save and restore color setting
	originalNoColor := color.NoColor
	color.NoColor = false
	defer func() { color.NoColor = originalNoColor }()
	
	// Verify defaultHighlights is properly initialized
	if len(defaultHighlights) == 0 {
		t.Fatal("defaultHighlights is empty")
	}

	expectedPatterns := 7 // comments, strings, unclosed strings, numbers, params, keywords, idents
	if len(defaultHighlights) != expectedPatterns {
		t.Errorf("defaultHighlights has %d patterns, want %d", len(defaultHighlights), expectedPatterns)
	}

	// Test each highlight has required fields
	for i, h := range defaultHighlights {
		if h.Pattern == nil {
			t.Errorf("defaultHighlights[%d].Pattern is nil", i)
		}
		// Note: Sequence might be empty if color is disabled globally
	}
}

// Test error message constants
func TestErrorMessages(t *testing.T) {
	// Verify error constants are defined and not empty
	if errMessageUnclosedStringLiteral == "" {
		t.Error("errMessageUnclosedStringLiteral is empty")
	}
	if errMessageUnclosedTripleQuotedStringLiteral == "" {
		t.Error("errMessageUnclosedTripleQuotedStringLiteral is empty")
	}
	if errMessageUnclosedComment == "" {
		t.Error("errMessageUnclosedComment is empty")
	}
}

// Test with actual SQL patterns
func TestHighlightingPatterns(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		validate func(t *testing.T, results [][]int)
	}{
		{
			name:  "complex SQL with all token types",
			input: `-- Comment\nSELECT 'string', 123, @param FROM table WHERE id = 1`,
			validate: func(t *testing.T, results [][]int) {
				// Should have multiple highlights
				if len(results) == 0 {
					t.Error("expected highlights for complex SQL")
				}
			},
		},
		{
			name:  "SQL with errors",
			input: `SELECT 'unclosed string`,
			validate: func(t *testing.T, results [][]int) {
				// Comment highlighter won't catch string errors, so this might be empty
				// This test is more about ensuring no panic occurs
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test with comment highlighter
			commentHL := commentHighlighter()
			results := commentHL(tt.input, -1)
			if tt.validate != nil {
				tt.validate(t, results)
			}
		})
	}
}