package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/apstndb/gsqlutils"
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

// Tests for extracted validation functions

func TestShouldSubmitStatement(t *testing.T) {
	tests := []struct {
		name               string
		statements         []inputStatement
		err                error
		wantShouldSubmit   bool
		wantWaitingStatus  string
	}{
		{
			name:               "empty statements with no error",
			statements:         []inputStatement{},
			err:                nil,
			wantShouldSubmit:   false,
			wantWaitingStatus:  "",
		},
		{
			name: "single complete statement with delimiter",
			statements: []inputStatement{
				{statement: "SELECT 1", delim: ";"},
			},
			err:                nil,
			wantShouldSubmit:   true,
			wantWaitingStatus:  "",
		},
		{
			name: "single incomplete statement without delimiter",
			statements: []inputStatement{
				{statement: "SELECT 1", delim: delimiterUndefined},
			},
			err:                nil,
			wantShouldSubmit:   false,
			wantWaitingStatus:  "",
		},
		{
			name: "multiple statements",
			statements: []inputStatement{
				{statement: "SELECT 1", delim: ";"},
				{statement: "SELECT 2", delim: ";"},
			},
			err:                nil,
			wantShouldSubmit:   true,
			wantWaitingStatus:  "",
		},
		{
			name:               "lexer status error with waiting string",
			statements:         []inputStatement{},
			err:                &gsqlutils.ErrLexerStatus{WaitingString: "waiting for '"},
			wantShouldSubmit:   false,
			wantWaitingStatus:  "waiting for '",
		},
		{
			name:               "other error",
			statements:         []inputStatement{},
			err:                errors.New("some error"),
			wantShouldSubmit:   true,
			wantWaitingStatus:  "",
		},
		{
			name: "single statement with horizontal delimiter",
			statements: []inputStatement{
				{statement: "SELECT 1", delim: delimiterHorizontal},
			},
			err:                nil,
			wantShouldSubmit:   true,
			wantWaitingStatus:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotShouldSubmit, gotWaitingStatus := shouldSubmitStatement(tt.statements, tt.err)
			
			if gotShouldSubmit != tt.wantShouldSubmit {
				t.Errorf("shouldSubmitStatement() shouldSubmit = %v, want %v", gotShouldSubmit, tt.wantShouldSubmit)
			}
			if gotWaitingStatus != tt.wantWaitingStatus {
				t.Errorf("shouldSubmitStatement() waitingStatus = %v, want %v", gotWaitingStatus, tt.wantWaitingStatus)
			}
		})
	}
}

func TestProcessInputLines(t *testing.T) {
	tests := []struct {
		name      string
		lines     []string
		readErr   error
		wantStmt  *inputStatement
		wantErr   bool
		errString string
	}{
		{
			name:      "successful read with no error",
			lines:     []string{"SELECT 1"},
			readErr:   nil,
			wantStmt:  nil,
			wantErr:   false,
		},
		{
			name:      "read error with empty lines",
			lines:     []string{},
			readErr:   errors.New("read error"),
			wantStmt:  nil,
			wantErr:   true,
			errString: "failed to read input: read error",
		},
		{
			name:    "read error with non-empty lines",
			lines:   []string{"SELECT", "FROM table"},
			readErr: errors.New("interrupted"),
			wantStmt: &inputStatement{
				statement:                "SELECT\nFROM table",
				statementWithoutComments: "SELECT\nFROM table",
				delim:                    "",
			},
			wantErr:   true,
			errString: "interrupted",
		},
		{
			name:    "single line with error",
			lines:   []string{"SELECT 1;"},
			readErr: errors.New("some error"),
			wantStmt: &inputStatement{
				statement:                "SELECT 1;",
				statementWithoutComments: "SELECT 1;",
				delim:                    "",
			},
			wantErr:   true,
			errString: "some error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStmt, gotErr := processInputLines(tt.lines, tt.readErr)
			
			if (gotErr != nil) != tt.wantErr {
				t.Errorf("processInputLines() error = %v, wantErr %v", gotErr, tt.wantErr)
			}
			
			if tt.wantErr && gotErr != nil && gotErr.Error() != tt.errString {
				t.Errorf("processInputLines() error = %v, want %v", gotErr.Error(), tt.errString)
			}
			
			if tt.wantStmt != nil {
				if gotStmt == nil {
					t.Error("processInputLines() returned nil statement, want non-nil")
				} else if gotStmt.statement != tt.wantStmt.statement ||
					gotStmt.statementWithoutComments != tt.wantStmt.statementWithoutComments ||
					gotStmt.delim != tt.wantStmt.delim {
					t.Errorf("processInputLines() = %+v, want %+v", gotStmt, tt.wantStmt)
				}
			} else if gotStmt != nil {
				t.Errorf("processInputLines() returned non-nil statement %+v, want nil", gotStmt)
			}
		})
	}
}

func TestValidateInteractiveInput(t *testing.T) {
	tests := []struct {
		name       string
		statements []inputStatement
		wantStmt   *inputStatement
		wantErr    bool
		errString  string
	}{
		{
			name:       "no statements",
			statements: []inputStatement{},
			wantStmt:   nil,
			wantErr:    true,
			errString:  "no input",
		},
		{
			name: "single statement",
			statements: []inputStatement{
				{statement: "SELECT 1", statementWithoutComments: "SELECT 1", delim: ";"},
			},
			wantStmt: &inputStatement{
				statement: "SELECT 1", statementWithoutComments: "SELECT 1", delim: ";",
			},
			wantErr: false,
		},
		{
			name: "multiple statements",
			statements: []inputStatement{
				{statement: "SELECT 1", delim: ";"},
				{statement: "SELECT 2", delim: ";"},
			},
			wantStmt:  nil,
			wantErr:   true,
			errString: "sql queries are limited to single statements in interactive mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStmt, gotErr := validateInteractiveInput(tt.statements)
			
			if (gotErr != nil) != tt.wantErr {
				t.Errorf("validateInteractiveInput() error = %v, wantErr %v", gotErr, tt.wantErr)
			}
			
			if tt.wantErr && gotErr != nil && gotErr.Error() != tt.errString {
				t.Errorf("validateInteractiveInput() error = %v, want %v", gotErr.Error(), tt.errString)
			}
			
			if tt.wantStmt != nil {
				if gotStmt == nil {
					t.Error("validateInteractiveInput() returned nil statement, want non-nil")
				} else if gotStmt != &tt.statements[0] {
					t.Errorf("validateInteractiveInput() returned different statement pointer")
				}
			} else if gotStmt != nil {
				t.Errorf("validateInteractiveInput() returned non-nil statement %+v, want nil", gotStmt)
			}
		})
	}
}

func TestGeneratePS2Prompt(t *testing.T) {
	tests := []struct {
		name             string
		ps1              string
		ps2Template      string
		ps2Interpolated  string
		want             string
	}{
		{
			name:             "no padding needed",
			ps1:              "spanner> ",
			ps2Template:      "... ",
			ps2Interpolated:  "... ",
			want:             "... ",
		},
		{
			name:             "padding needed with %P prefix",
			ps1:              "spanner> ",
			ps2Template:      "%P... ",
			ps2Interpolated:  "... ",
			want:             "     ... ", // padded to match "spanner> " width
		},
		{
			name:             "multi-line PS1, uses last line",
			ps1:              "line1\nspanner> ",
			ps2Template:      "%P... ",
			ps2Interpolated:  "... ",
			want:             "     ... ", // padded to match "spanner> " width
		},
		{
			name:             "empty PS1",
			ps1:              "",
			ps2Template:      "%P... ",
			ps2Interpolated:  "... ",
			want:             "... ", // no padding for empty PS1
		},
		{
			name:             "longer PS2 than PS1",
			ps1:              "sql> ",
			ps2Template:      "%P..... ",
			ps2Interpolated:  "..... ",
			want:             "..... ", // no left-padding when PS2 is already longer
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generatePS2Prompt(tt.ps1, tt.ps2Template, tt.ps2Interpolated)
			if got != tt.want {
				t.Errorf("generatePS2Prompt() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPS1PS2FuncToPromptFunc(t *testing.T) {
	ps1Called := 0
	ps2Called := 0
	testPS1 := "test> "
	testPS2 := "... "
	
	ps1Func := func() string {
		ps1Called++
		return testPS1
	}
	
	ps2Func := func(ps1 string) string {
		ps2Called++
		if ps1 != testPS1 {
			t.Errorf("ps2Func received ps1 = %q, want %q", ps1, testPS1)
		}
		return testPS2
	}
	
	promptFunc := PS1PS2FuncToPromptFunc(ps1Func, ps2Func)
	
	tests := []struct {
		name      string
		lineNum   int
		wantStr   string
		wantPS1   int
		wantPS2   int
	}{
		{
			name:      "first line uses PS1",
			lineNum:   0,
			wantStr:   testPS1,
			wantPS1:   1,
			wantPS2:   0,
		},
		{
			name:      "second line uses PS2",
			lineNum:   1,
			wantStr:   testPS2,
			wantPS1:   1, // PS1 is called to pass to PS2
			wantPS2:   1,
		},
		{
			name:      "third line uses PS2",
			lineNum:   2,
			wantStr:   testPS2,
			wantPS1:   1,
			wantPS2:   1,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps1Called = 0
			ps2Called = 0
			
			var buf strings.Builder
			n, err := promptFunc(&buf, tt.lineNum)
			
			if err != nil {
				t.Errorf("promptFunc() error = %v", err)
			}
			
			if n != len(tt.wantStr) {
				t.Errorf("promptFunc() wrote %d bytes, want %d", n, len(tt.wantStr))
			}
			
			if buf.String() != tt.wantStr {
				t.Errorf("promptFunc() wrote %q, want %q", buf.String(), tt.wantStr)
			}
			
			if ps1Called != tt.wantPS1 {
				t.Errorf("ps1Func called %d times, want %d", ps1Called, tt.wantPS1)
			}
			
			if ps2Called != tt.wantPS2 {
				t.Errorf("ps2Func called %d times, want %d", ps2Called, tt.wantPS2)
			}
		})
	}
}