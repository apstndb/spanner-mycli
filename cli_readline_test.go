package main

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/apstndb/gsqlutils"
	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/token"
	"github.com/fatih/color"
	"github.com/nyaosorg/go-readline-ny/simplehistory"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// Test persistentHistory.Add method
func TestPersistentHistoryAdd(t *testing.T) {
	tests := []struct {
		name           string
		setupFS        func() afero.Fs
		filename       string
		itemsToAdd     []string
		wantError      bool
		validateFile   func(t *testing.T, fs afero.Fs, filename string)
		validateHistory func(t *testing.T, h History)
	}{
		{
			name: "add single item to new file",
			setupFS: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			filename:   "test_history.txt",
			itemsToAdd: []string{"SELECT * FROM users"},
			validateFile: func(t *testing.T, fs afero.Fs, filename string) {
				content, err := afero.ReadFile(fs, filename)
				require.NoError(t, err)
				assert.Equal(t, "\"SELECT * FROM users\"\n", string(content))
			},
			validateHistory: func(t *testing.T, h History) {
				assert.Equal(t, 1, h.Len())
				assert.Equal(t, "SELECT * FROM users", h.At(0))
			},
		},
		{
			name: "add multiple items",
			setupFS: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			filename:   "test_history.txt",
			itemsToAdd: []string{"SELECT 1", "SELECT 2", "SELECT 3"},
			validateFile: func(t *testing.T, fs afero.Fs, filename string) {
				content, err := afero.ReadFile(fs, filename)
				require.NoError(t, err)
				expected := "\"SELECT 1\"\n\"SELECT 2\"\n\"SELECT 3\"\n"
				assert.Equal(t, expected, string(content))
			},
			validateHistory: func(t *testing.T, h History) {
				assert.Equal(t, 3, h.Len())
				assert.Equal(t, "SELECT 1", h.At(0))
				assert.Equal(t, "SELECT 2", h.At(1))
				assert.Equal(t, "SELECT 3", h.At(2))
			},
		},
		{
			name: "add item with special characters",
			setupFS: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			filename:   "test_history.txt",
			itemsToAdd: []string{"SELECT \"quoted\"\nWHERE x = 'value'"},
			validateFile: func(t *testing.T, fs afero.Fs, filename string) {
				content, err := afero.ReadFile(fs, filename)
				require.NoError(t, err)
				// strconv.Quote should escape the special characters
				assert.Contains(t, string(content), "\\\"quoted\\\"")
				assert.Contains(t, string(content), "\\n")
			},
			validateHistory: func(t *testing.T, h History) {
				assert.Equal(t, 1, h.Len())
				assert.Equal(t, "SELECT \"quoted\"\nWHERE x = 'value'", h.At(0))
			},
		},
		{
			name: "append to existing file",
			setupFS: func() afero.Fs {
				fs := afero.NewMemMapFs()
				// Create file with existing content
				err := afero.WriteFile(fs, "test_history.txt", []byte("\"EXISTING COMMAND\"\n"), 0600)
				require.NoError(t, err)
				return fs
			},
			filename:   "test_history.txt",
			itemsToAdd: []string{"NEW COMMAND"},
			validateFile: func(t *testing.T, fs afero.Fs, filename string) {
				content, err := afero.ReadFile(fs, filename)
				require.NoError(t, err)
				expected := "\"EXISTING COMMAND\"\n\"NEW COMMAND\"\n"
				assert.Equal(t, expected, string(content))
			},
			validateHistory: func(t *testing.T, h History) {
				// Only the new item should be in the history container
				// (existing items are not loaded in this test)
				assert.Equal(t, 1, h.Len())
				assert.Equal(t, "NEW COMMAND", h.At(0))
			},
		},
		{
			name: "handle file system errors gracefully",
			setupFS: func() afero.Fs {
				// Create a read-only filesystem to trigger errors
				fs := afero.NewMemMapFs()
				return afero.NewReadOnlyFs(fs)
			},
			filename:   "test_history.txt",
			itemsToAdd: []string{"SELECT 1"},
			wantError:  false, // Add method logs errors but doesn't return them
			validateHistory: func(t *testing.T, h History) {
				// Item should still be added to in-memory history
				assert.Equal(t, 1, h.Len())
				assert.Equal(t, "SELECT 1", h.At(0))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := tt.setupFS()
			h := simplehistory.New()
			ph := &persistentHistory{
				filename: tt.filename,
				history:  h,
				fs:       fs,
			}

			// Add items
			for _, item := range tt.itemsToAdd {
				ph.Add(item)
			}

			// Validate file content if needed
			if tt.validateFile != nil {
				tt.validateFile(t, fs, tt.filename)
			}

			// Validate history state
			if tt.validateHistory != nil {
				tt.validateHistory(t, ph)
			}
		})
	}
}

// Test newPersistentHistoryWithFS function
func TestNewPersistentHistoryWithFS(t *testing.T) {
	tests := []struct {
		name          string
		setupFS       func() (afero.Fs, string)
		container     *simplehistory.Container
		wantError     bool
		errorContains string
		validate      func(t *testing.T, h History, err error)
	}{
		{
			name: "create new history when file doesn't exist",
			setupFS: func() (afero.Fs, string) {
				return afero.NewMemMapFs(), "new_history.txt"
			},
			container: simplehistory.New(),
			validate: func(t *testing.T, h History, err error) {
				require.NoError(t, err)
				require.NotNil(t, h)
				assert.Equal(t, 0, h.Len())
			},
		},
		{
			name: "load existing history file",
			setupFS: func() (afero.Fs, string) {
				fs := afero.NewMemMapFs()
				content := "\"SELECT 1\"\n\"SELECT 2\"\n\"SELECT 3\"\n"
				err := afero.WriteFile(fs, "history.txt", []byte(content), 0600)
				require.NoError(t, err)
				return fs, "history.txt"
			},
			container: simplehistory.New(),
			validate: func(t *testing.T, h History, err error) {
				require.NoError(t, err)
				require.NotNil(t, h)
				assert.Equal(t, 3, h.Len())
				assert.Equal(t, "SELECT 1", h.At(0))
				assert.Equal(t, "SELECT 2", h.At(1))
				assert.Equal(t, "SELECT 3", h.At(2))
			},
		},
		{
			name: "handle empty lines in history file",
			setupFS: func() (afero.Fs, string) {
				fs := afero.NewMemMapFs()
				content := "\"SELECT 1\"\n\n\"SELECT 2\"\n\n\n\"SELECT 3\"\n"
				err := afero.WriteFile(fs, "history.txt", []byte(content), 0600)
				require.NoError(t, err)
				return fs, "history.txt"
			},
			container: simplehistory.New(),
			validate: func(t *testing.T, h History, err error) {
				require.NoError(t, err)
				require.NotNil(t, h)
				assert.Equal(t, 3, h.Len())
				assert.Equal(t, "SELECT 1", h.At(0))
				assert.Equal(t, "SELECT 2", h.At(1))
				assert.Equal(t, "SELECT 3", h.At(2))
			},
		},
		{
			name: "handle commands with special characters",
			setupFS: func() (afero.Fs, string) {
				fs := afero.NewMemMapFs()
				// Use strconv.Quote to create properly escaped content
				content := "\"SELECT \\\"quoted\\\"\"\n\"WHERE x = 'value'\"\n\"Line1\\nLine2\"\n"
				err := afero.WriteFile(fs, "history.txt", []byte(content), 0600)
				require.NoError(t, err)
				return fs, "history.txt"
			},
			container: simplehistory.New(),
			validate: func(t *testing.T, h History, err error) {
				require.NoError(t, err)
				require.NotNil(t, h)
				assert.Equal(t, 3, h.Len())
				assert.Equal(t, "SELECT \"quoted\"", h.At(0))
				assert.Equal(t, "WHERE x = 'value'", h.At(1))
				assert.Equal(t, "Line1\nLine2", h.At(2))
			},
		},
		{
			name: "error on invalid quote format",
			setupFS: func() (afero.Fs, string) {
				fs := afero.NewMemMapFs()
				// Invalid format - not properly quoted
				content := "SELECT 1\nSELECT 2\n"
				err := afero.WriteFile(fs, "history.txt", []byte(content), 0600)
				require.NoError(t, err)
				return fs, "history.txt"
			},
			container:     simplehistory.New(),
			wantError:     true,
			errorContains: "history file format error",
			validate: func(t *testing.T, h History, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "history file format error")
				assert.Contains(t, err.Error(), "maybe you should remove")
			},
		},
		{
			name: "error on file read failure",
			setupFS: func() (afero.Fs, string) {
				// Create a custom filesystem that returns an error
				fs := &errorFS{
					Fs:        afero.NewMemMapFs(),
					readError: errors.New("read permission denied"),
				}
				return fs, "history.txt"
			},
			container:     simplehistory.New(),
			wantError:     true,
			errorContains: "read permission denied",
			validate: func(t *testing.T, h History, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "read permission denied")
			},
		},
		{
			name: "preserve existing container items",
			setupFS: func() (afero.Fs, string) {
				fs := afero.NewMemMapFs()
				content := "\"NEW ITEM 1\"\n\"NEW ITEM 2\"\n"
				err := afero.WriteFile(fs, "history.txt", []byte(content), 0600)
				require.NoError(t, err)
				return fs, "history.txt"
			},
			container: func() *simplehistory.Container {
				c := simplehistory.New()
				c.Add("EXISTING ITEM")
				return c
			}(),
			validate: func(t *testing.T, h History, err error) {
				require.NoError(t, err)
				require.NotNil(t, h)
				assert.Equal(t, 3, h.Len())
				assert.Equal(t, "EXISTING ITEM", h.At(0))
				assert.Equal(t, "NEW ITEM 1", h.At(1))
				assert.Equal(t, "NEW ITEM 2", h.At(2))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs, filename := tt.setupFS()
			h, err := newPersistentHistoryWithFS(filename, tt.container, fs)
			
			if tt.wantError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			}
			
			if tt.validate != nil {
				tt.validate(t, h, err)
			}
		})
	}
}

// Test persistentHistory.Len and persistentHistory.At methods
func TestPersistentHistoryLenAndAt(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *persistentHistory
		validate func(t *testing.T, h *persistentHistory)
	}{
		{
			name: "empty history",
			setup: func() *persistentHistory {
				return &persistentHistory{
					filename: "test.txt",
					history:  simplehistory.New(),
					fs:       afero.NewMemMapFs(),
				}
			},
			validate: func(t *testing.T, h *persistentHistory) {
				assert.Equal(t, 0, h.Len())
			},
		},
		{
			name: "history with single item",
			setup: func() *persistentHistory {
				c := simplehistory.New()
				c.Add("SELECT 1")
				return &persistentHistory{
					filename: "test.txt",
					history:  c,
					fs:       afero.NewMemMapFs(),
				}
			},
			validate: func(t *testing.T, h *persistentHistory) {
				assert.Equal(t, 1, h.Len())
				assert.Equal(t, "SELECT 1", h.At(0))
			},
		},
		{
			name: "history with multiple items",
			setup: func() *persistentHistory {
				c := simplehistory.New()
				items := []string{"SELECT 1", "SELECT 2", "SELECT 3", "SELECT 4", "SELECT 5"}
				for _, item := range items {
					c.Add(item)
				}
				return &persistentHistory{
					filename: "test.txt",
					history:  c,
					fs:       afero.NewMemMapFs(),
				}
			},
			validate: func(t *testing.T, h *persistentHistory) {
				assert.Equal(t, 5, h.Len())
				assert.Equal(t, "SELECT 1", h.At(0))
				assert.Equal(t, "SELECT 2", h.At(1))
				assert.Equal(t, "SELECT 3", h.At(2))
				assert.Equal(t, "SELECT 4", h.At(3))
				assert.Equal(t, "SELECT 5", h.At(4))
			},
		},
		{
			name: "access items in reverse order",
			setup: func() *persistentHistory {
				c := simplehistory.New()
				for i := 1; i <= 3; i++ {
					c.Add(string(rune('A' + i - 1)))
				}
				return &persistentHistory{
					filename: "test.txt",
					history:  c,
					fs:       afero.NewMemMapFs(),
				}
			},
			validate: func(t *testing.T, h *persistentHistory) {
				assert.Equal(t, 3, h.Len())
				// Access in reverse
				assert.Equal(t, "C", h.At(2))
				assert.Equal(t, "B", h.At(1))
				assert.Equal(t, "A", h.At(0))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := tt.setup()
			tt.validate(t, h)
		})
	}
}

// Test newPersistentHistory (the public function)
func TestNewPersistentHistory(t *testing.T) {
	// Create a temporary file for testing
	tmpFile, err := os.CreateTemp("", "test_history_*.txt")
	require.NoError(t, err)
	defer func() {
		_ = os.Remove(tmpFile.Name())
	}()
	
	// Write some test data
	testContent := "\"SELECT 1\"\n\"SELECT 2\"\n"
	_, err = tmpFile.WriteString(testContent)
	require.NoError(t, err)
	tmpFile.Close()
	
	// Test loading the file
	h, err := newPersistentHistory(tmpFile.Name(), simplehistory.New())
	require.NoError(t, err)
	require.NotNil(t, h)
	
	assert.Equal(t, 2, h.Len())
	assert.Equal(t, "SELECT 1", h.At(0))
	assert.Equal(t, "SELECT 2", h.At(1))
	
	// Test adding new item
	h.Add("SELECT 3")
	assert.Equal(t, 3, h.Len())
	assert.Equal(t, "SELECT 3", h.At(2))
	
	// Verify the file was updated
	content, err := os.ReadFile(tmpFile.Name())
	require.NoError(t, err)
	expected := "\"SELECT 1\"\n\"SELECT 2\"\n\"SELECT 3\"\n"
	assert.Equal(t, expected, string(content))
}

// errorFS is a test filesystem that returns errors
type errorFS struct {
	afero.Fs
	readError error
}

func (e *errorFS) Open(name string) (afero.File, error) {
	if e.readError != nil {
		return nil, e.readError
	}
	return e.Fs.Open(name)
}

func (e *errorFS) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	if e.readError != nil {
		return nil, e.readError
	}
	return e.Fs.OpenFile(name, flag, perm)
}