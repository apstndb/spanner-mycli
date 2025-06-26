package main

import (
	"testing"

	"github.com/cloudspannerecosystem/memefish/token"
	"github.com/fatih/color"
	"github.com/google/go-cmp/cmp"
)

func TestClassifyToken(t *testing.T) {
	tests := []struct {
		name     string
		token    token.Token
		expected string
	}{
		{
			name: "comment token",
			token: token.Token{
				Kind: token.TokenIdent,
				Comments: []token.TokenComment{
					{Pos: 0, End: 10},
				},
			},
			expected: "comment",
		},
		{
			name:     "string literal",
			token:    token.Token{Kind: token.TokenString},
			expected: "string",
		},
		{
			name:     "bytes literal",
			token:    token.Token{Kind: token.TokenBytes},
			expected: "string",
		},
		{
			name:     "float literal",
			token:    token.Token{Kind: token.TokenFloat},
			expected: "number",
		},
		{
			name:     "int literal",
			token:    token.Token{Kind: token.TokenInt},
			expected: "number",
		},
		{
			name:     "parameter",
			token:    token.Token{Kind: token.TokenParam},
			expected: "parameter",
		},
		{
			name:     "keyword SELECT",
			token:    token.Token{Kind: "SELECT"},
			expected: "keyword",
		},
		{
			name:     "keyword WHERE",
			token:    token.Token{Kind: "WHERE"},
			expected: "keyword",
		},
		{
			name:     "identifier",
			token:    token.Token{Kind: token.TokenIdent},
			expected: "identifier",
		},
		{
			name:     "unknown token",
			token:    token.Token{Kind: token.TokenKind("(")},
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyToken(tt.token)
			if result != tt.expected {
				t.Errorf("ClassifyToken() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestTokenCheckers(t *testing.T) {
	t.Run("isComment", func(t *testing.T) {
		tests := []struct {
			name     string
			token    token.Token
			expected bool
		}{
			{
				name: "token with comments",
				token: token.Token{
					Comments: []token.TokenComment{{Pos: 0, End: 10}},
				},
				expected: true,
			},
			{
				name:     "token without comments",
				token:    token.Token{},
				expected: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if got := isComment(tt.token); got != tt.expected {
					t.Errorf("isComment() = %v, want %v", got, tt.expected)
				}
			})
		}
	})

	t.Run("isStringLiteral", func(t *testing.T) {
		tests := []struct {
			name     string
			token    token.Token
			expected bool
		}{
			{"string", token.Token{Kind: token.TokenString}, true},
			{"bytes", token.Token{Kind: token.TokenBytes}, true},
			{"int", token.Token{Kind: token.TokenInt}, false},
			{"ident", token.Token{Kind: token.TokenIdent}, false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if got := isStringLiteral(tt.token); got != tt.expected {
					t.Errorf("isStringLiteral() = %v, want %v", got, tt.expected)
				}
			})
		}
	})

	t.Run("isNumericLiteral", func(t *testing.T) {
		tests := []struct {
			name     string
			token    token.Token
			expected bool
		}{
			{"float", token.Token{Kind: token.TokenFloat}, true},
			{"int", token.Token{Kind: token.TokenInt}, true},
			{"string", token.Token{Kind: token.TokenString}, false},
			{"ident", token.Token{Kind: token.TokenIdent}, false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if got := isNumericLiteral(tt.token); got != tt.expected {
					t.Errorf("isNumericLiteral() = %v, want %v", got, tt.expected)
				}
			})
		}
	})

	t.Run("isParameter", func(t *testing.T) {
		tests := []struct {
			name     string
			token    token.Token
			expected bool
		}{
			{"param", token.Token{Kind: token.TokenParam}, true},
			{"ident", token.Token{Kind: token.TokenIdent}, false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if got := isParameter(tt.token); got != tt.expected {
					t.Errorf("isParameter() = %v, want %v", got, tt.expected)
				}
			})
		}
	})

	t.Run("isKeyword", func(t *testing.T) {
		tests := []struct {
			name     string
			token    token.Token
			expected bool
		}{
			{"SELECT", token.Token{Kind: "SELECT"}, true},
			{"WHERE", token.Token{Kind: "WHERE"}, true},
			{"FROM", token.Token{Kind: "FROM"}, true},
			{"parenthesis", token.Token{Kind: "("}, false},
			{"comma", token.Token{Kind: ","}, false},
			{"star", token.Token{Kind: "*"}, false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if got := isKeyword(tt.token); got != tt.expected {
					t.Errorf("isKeyword() = %v, want %v", got, tt.expected)
				}
			})
		}
	})

	t.Run("isIdentifier", func(t *testing.T) {
		tests := []struct {
			name     string
			token    token.Token
			expected bool
		}{
			{"ident", token.Token{Kind: token.TokenIdent}, true},
			{"string", token.Token{Kind: token.TokenString}, false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if got := isIdentifier(tt.token); got != tt.expected {
					t.Errorf("isIdentifier() = %v, want %v", got, tt.expected)
				}
			})
		}
	})
}

func TestGetTokenRanges(t *testing.T) {
	tests := []struct {
		name     string
		token    token.Token
		expected []TokenRange
	}{
		{
			name: "token without comments",
			token: token.Token{
				Pos: 10,
				End: 20,
			},
			expected: []TokenRange{{Start: 10, End: 20}},
		},
		{
			name: "token with comments",
			token: token.Token{
				Pos: 10,
				End: 20,
				Comments: []token.TokenComment{
					{Pos: 0, End: 9},
					{Pos: 21, End: 30},
				},
			},
			expected: []TokenRange{
				{Start: 10, End: 20},
				{Start: 0, End: 9},
				{Start: 21, End: 30},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetTokenRanges(tt.token)
			if diff := cmp.Diff(tt.expected, result); diff != "" {
				t.Errorf("GetTokenRanges() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGetCommentRanges(t *testing.T) {
	tests := []struct {
		name     string
		token    token.Token
		expected []TokenRange
	}{
		{
			name:     "token without comments",
			token:    token.Token{Pos: 10, End: 20},
			expected: []TokenRange{},
		},
		{
			name: "token with comments",
			token: token.Token{
				Pos: 10,
				End: 20,
				Comments: []token.TokenComment{
					{Pos: 0, End: 9},
					{Pos: 21, End: 30},
				},
			},
			expected: []TokenRange{
				{Start: 0, End: 9},
				{Start: 21, End: 30},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetCommentRanges(tt.token)
			if result == nil {
				result = []TokenRange{}
			}
			if diff := cmp.Diff(tt.expected, result); diff != "" {
				t.Errorf("GetCommentRanges() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestMatchToken(t *testing.T) {
	tests := []struct {
		name     string
		token    token.Token
		kinds    []token.TokenKind
		expected bool
	}{
		{
			name:     "match single kind",
			token:    token.Token{Kind: token.TokenString},
			kinds:    []token.TokenKind{token.TokenString},
			expected: true,
		},
		{
			name:     "match multiple kinds",
			token:    token.Token{Kind: token.TokenString},
			kinds:    []token.TokenKind{token.TokenBytes, token.TokenString},
			expected: true,
		},
		{
			name:     "no match",
			token:    token.Token{Kind: token.TokenInt},
			kinds:    []token.TokenKind{token.TokenString, token.TokenBytes},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MatchToken(tt.token, tt.kinds...)
			if result != tt.expected {
				t.Errorf("MatchToken() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCreateColorSequence(t *testing.T) {
	// Save original NoColor state
	originalNoColor := color.NoColor
	defer func() { color.NoColor = originalNoColor }()

	t.Run("with color enabled", func(t *testing.T) {
		color.NoColor = false
		result := CreateColorSequence(color.FgRed, color.Bold)
		if result == "" {
			t.Error("CreateColorSequence() returned empty string when color is enabled")
		}
	})

	t.Run("with color disabled", func(t *testing.T) {
		color.NoColor = true
		result := CreateColorSequence(color.FgRed, color.Bold)
		if result != "" {
			t.Errorf("CreateColorSequence() = %v, want empty string when NoColor is true", result)
		}
	})
}

func TestIsErrorUnclosedString(t *testing.T) {
	tests := []struct {
		name     string
		message  string
		expected bool
	}{
		{"unclosed string literal", "unclosed string literal", true},
		{"unclosed triple-quoted string", "unclosed triple-quoted string literal", true},
		{"other error", "syntax error", false},
		{"empty message", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsErrorUnclosedString(tt.message)
			if result != tt.expected {
				t.Errorf("IsErrorUnclosedString(%q) = %v, want %v", tt.message, result, tt.expected)
			}
		})
	}
}

func TestIsErrorUnclosedComment(t *testing.T) {
	tests := []struct {
		name     string
		message  string
		expected bool
	}{
		{"unclosed comment", "unclosed comment", true},
		{"other error", "syntax error", false},
		{"empty message", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsErrorUnclosedComment(tt.message)
			if result != tt.expected {
				t.Errorf("IsErrorUnclosedComment(%q) = %v, want %v", tt.message, result, tt.expected)
			}
		})
	}
}

func TestCreateHighlightRules(t *testing.T) {
	rules := CreateHighlightRules()
	
	// Check that we have the expected number of rules
	expectedRuleCount := 6 // comments, strings, numbers, params, keywords, identifiers
	if len(rules) != expectedRuleCount {
		t.Errorf("CreateHighlightRules() returned %d rules, want %d", len(rules), expectedRuleCount)
	}

	// Test each rule with appropriate tokens
	testCases := []struct {
		name        string
		token       token.Token
		shouldMatch []int // indices of rules that should match
	}{
		{
			name:        "comment token",
			token:       token.Token{Comments: []token.TokenComment{{}}},
			shouldMatch: []int{0}, // comment rule
		},
		{
			name:        "string token",
			token:       token.Token{Kind: token.TokenString},
			shouldMatch: []int{1}, // string rule
		},
		{
			name:        "int token",
			token:       token.Token{Kind: token.TokenInt},
			shouldMatch: []int{2}, // number rule
		},
		{
			name:        "param token",
			token:       token.Token{Kind: token.TokenParam},
			shouldMatch: []int{3}, // param rule
		},
		{
			name:        "keyword token",
			token:       token.Token{Kind: "SELECT"},
			shouldMatch: []int{4}, // keyword rule
		},
		{
			name:        "identifier token",
			token:       token.Token{Kind: token.TokenIdent},
			shouldMatch: []int{5}, // identifier rule
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for i, rule := range rules {
				matched := rule.Matcher(tc.token)
				shouldMatch := false
				for _, idx := range tc.shouldMatch {
					if i == idx {
						shouldMatch = true
						break
					}
				}
				if matched != shouldMatch {
					t.Errorf("Rule %d match = %v, want %v for token %v", i, matched, shouldMatch, tc.token)
				}
			}
		})
	}
}

// Test edge cases
func TestEdgeCases(t *testing.T) {
	t.Run("empty input classification", func(t *testing.T) {
		emptyToken := token.Token{}
		result := ClassifyToken(emptyToken)
		if result != "unknown" {
			t.Errorf("ClassifyToken(empty) = %v, want 'unknown'", result)
		}
	})

	t.Run("token with multiple classifications prefers first", func(t *testing.T) {
		// A token with both comments and being an identifier should be classified as comment
		tok := token.Token{
			Kind:     token.TokenIdent,
			Comments: []token.TokenComment{{Pos: 0, End: 5}},
		}
		result := ClassifyToken(tok)
		if result != "comment" {
			t.Errorf("ClassifyToken(ident with comments) = %v, want 'comment'", result)
		}
	})

	t.Run("special characters in keywords", func(t *testing.T) {
		specialTokens := []token.Token{
			{Kind: token.TokenKind("(")},
			{Kind: token.TokenKind(")")},
			{Kind: token.TokenKind(",")},
			{Kind: token.TokenKind(";")},
			{Kind: token.TokenKind(".")},
			{Kind: token.TokenKind("*")},
		}
		
		for _, tok := range specialTokens {
			if isKeyword(tok) {
				t.Errorf("isKeyword(%v) = true, want false for special character", tok.Kind)
			}
		}
	})
}