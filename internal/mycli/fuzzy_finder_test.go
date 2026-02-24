// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mycli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectFuzzyContext(t *testing.T) {
	tests := []struct {
		name               string
		input              string
		wantCompletionType fuzzyCompletionType
		wantArgPrefix      string
		wantArgStartPos    int
		wantContext        string
		wantSuffix         string
	}{
		// Argument completion: USE → database
		{
			name:               "USE with trailing space",
			input:              "USE ",
			wantCompletionType: fuzzyCompleteDatabase,
			wantArgPrefix:      "",
			wantArgStartPos:    4,
		},
		{
			name:               "USE with partial db name",
			input:              "USE apst",
			wantCompletionType: fuzzyCompleteDatabase,
			wantArgPrefix:      "apst",
			wantArgStartPos:    4,
		},
		{
			name:               "USE with full db name",
			input:              "USE my-database",
			wantCompletionType: fuzzyCompleteDatabase,
			wantArgPrefix:      "my-database",
			wantArgStartPos:    4,
		},
		{
			name:               "use lowercase",
			input:              "use db",
			wantCompletionType: fuzzyCompleteDatabase,
			wantArgPrefix:      "db",
			wantArgStartPos:    4,
		},
		{
			name:               "USE with leading space",
			input:              "  USE ",
			wantCompletionType: fuzzyCompleteDatabase,
			wantArgPrefix:      "",
			wantArgStartPos:    6,
		},
		// Argument completion: SET → variable (with " = " suffix)
		{
			name:               "SET with partial name",
			input:              "SET CLI_",
			wantCompletionType: fuzzyCompleteVariable,
			wantArgPrefix:      "CLI_",
			wantArgStartPos:    4,
			wantSuffix:         " = ",
		},
		{
			name:               "SET with no name",
			input:              "SET ",
			wantCompletionType: fuzzyCompleteVariable,
			wantArgPrefix:      "",
			wantArgStartPos:    4,
			wantSuffix:         " = ",
		},
		{
			name:               "set lowercase",
			input:              "set cli_f",
			wantCompletionType: fuzzyCompleteVariable,
			wantArgPrefix:      "cli_f",
			wantArgStartPos:    4,
			wantSuffix:         " = ",
		},
		// Argument completion: SHOW VARIABLE → variable
		{
			name:               "SHOW VARIABLE with partial name",
			input:              "SHOW VARIABLE CLI_",
			wantCompletionType: fuzzyCompleteVariable,
			wantArgPrefix:      "CLI_",
			wantArgStartPos:    14,
		},
		{
			name:               "SHOW VARIABLE with no name",
			input:              "SHOW VARIABLE ",
			wantCompletionType: fuzzyCompleteVariable,
			wantArgPrefix:      "",
			wantArgStartPos:    14,
		},
		{
			name:               "show variable lowercase",
			input:              "show variable read",
			wantCompletionType: fuzzyCompleteVariable,
			wantArgPrefix:      "read",
			wantArgStartPos:    14,
		},
		// Argument completion: SHOW COLUMNS FROM → table
		{
			name:               "SHOW COLUMNS FROM with trailing space",
			input:              "SHOW COLUMNS FROM ",
			wantCompletionType: fuzzyCompleteTable,
			wantArgPrefix:      "",
			wantArgStartPos:    18,
		},
		{
			name:               "SHOW COLUMNS FROM with partial table",
			input:              "SHOW COLUMNS FROM Sin",
			wantCompletionType: fuzzyCompleteTable,
			wantArgPrefix:      "Sin",
			wantArgStartPos:    18,
		},
		// Argument completion: SHOW INDEX FROM → table
		{
			name:               "SHOW INDEX FROM with trailing space",
			input:              "SHOW INDEX FROM ",
			wantCompletionType: fuzzyCompleteTable,
			wantArgPrefix:      "",
			wantArgStartPos:    16,
		},
		{
			name:               "SHOW INDEXES FROM with partial table",
			input:              "SHOW INDEXES FROM Al",
			wantCompletionType: fuzzyCompleteTable,
			wantArgPrefix:      "Al",
			wantArgStartPos:    18,
		},
		{
			name:               "SHOW KEYS FROM",
			input:              "SHOW KEYS FROM ",
			wantCompletionType: fuzzyCompleteTable,
			wantArgPrefix:      "",
			wantArgStartPos:    15,
		},
		// Argument completion: TRUNCATE TABLE → table
		{
			name:               "TRUNCATE TABLE with trailing space",
			input:              "TRUNCATE TABLE ",
			wantCompletionType: fuzzyCompleteTable,
			wantArgPrefix:      "",
			wantArgStartPos:    15,
		},
		{
			name:               "TRUNCATE TABLE with partial table",
			input:              "TRUNCATE TABLE Sin",
			wantCompletionType: fuzzyCompleteTable,
			wantArgPrefix:      "Sin",
			wantArgStartPos:    15,
		},
		// Argument completion: DROP DATABASE → database
		{
			name:               "DROP DATABASE with trailing space",
			input:              "DROP DATABASE ",
			wantCompletionType: fuzzyCompleteDatabase,
			wantArgPrefix:      "",
			wantArgStartPos:    14,
		},
		{
			name:               "DROP DATABASE with partial name",
			input:              "DROP DATABASE my",
			wantCompletionType: fuzzyCompleteDatabase,
			wantArgPrefix:      "my",
			wantArgStartPos:    14,
		},
		// Argument completion: SET <name> = → variable value
		{
			name:               "SET value completion with space after =",
			input:              "SET CLI_FORMAT = ",
			wantCompletionType: fuzzyCompleteVariableValue,
			wantArgPrefix:      "",
			wantArgStartPos:    17,
			wantContext:        "CLI_FORMAT",
		},
		{
			name:               "SET value completion with partial value",
			input:              "SET CLI_FORMAT = TA",
			wantCompletionType: fuzzyCompleteVariableValue,
			wantArgPrefix:      "TA",
			wantArgStartPos:    17,
			wantContext:        "CLI_FORMAT",
		},
		{
			name:               "SET value completion no space after =",
			input:              "SET CLI_FORMAT=",
			wantCompletionType: fuzzyCompleteVariableValue,
			wantArgPrefix:      "",
			wantArgStartPos:    15,
			wantContext:        "CLI_FORMAT",
		},
		{
			name:               "SET value completion no space after = with value",
			input:              "SET CLI_FORMAT=TABLE",
			wantCompletionType: fuzzyCompleteVariableValue,
			wantArgPrefix:      "TABLE",
			wantArgStartPos:    15,
			wantContext:        "CLI_FORMAT",
		},
		{
			name:               "SET value completion for boolean var",
			input:              "SET READONLY = ",
			wantCompletionType: fuzzyCompleteVariableValue,
			wantArgPrefix:      "",
			wantArgStartPos:    15,
			wantContext:        "READONLY",
		},
		{
			name:               "set value completion lowercase",
			input:              "set cli_format = ta",
			wantCompletionType: fuzzyCompleteVariableValue,
			wantArgPrefix:      "ta",
			wantArgStartPos:    17,
			wantContext:        "cli_format",
		},
		// Argument completion: USE <db> ROLE → role
		{
			name:               "USE db ROLE with trailing space",
			input:              "USE mydb ROLE ",
			wantCompletionType: fuzzyCompleteRole,
			wantArgPrefix:      "",
			wantArgStartPos:    14,
			wantContext:        "mydb",
		},
		{
			name:               "USE db ROLE with partial role",
			input:              "USE mydb ROLE ad",
			wantCompletionType: fuzzyCompleteRole,
			wantArgPrefix:      "ad",
			wantArgStartPos:    14,
			wantContext:        "mydb",
		},
		{
			name:               "use db role lowercase",
			input:              "use db role ",
			wantCompletionType: fuzzyCompleteRole,
			wantArgPrefix:      "",
			wantArgStartPos:    12,
			wantContext:        "db",
		},
		{
			name:               "USE db ROLE with leading spaces",
			input:              "  USE mydb ROLE ",
			wantCompletionType: fuzzyCompleteRole,
			wantArgPrefix:      "",
			wantArgStartPos:    16,
			wantContext:        "mydb",
		},
		// Statement name completion (fallback)
		{
			name:               "USE without space falls through to statement name",
			input:              "USE",
			wantCompletionType: 0,
			wantArgPrefix:      "USE",
			wantArgStartPos:    0,
		},
		{
			name:               "SET without space falls through to statement name",
			input:              "SET",
			wantCompletionType: 0,
			wantArgPrefix:      "SET",
			wantArgStartPos:    0,
		},
		{
			name:               "partial statement name SHO",
			input:              "SHO",
			wantCompletionType: 0,
			wantArgPrefix:      "SHO",
			wantArgStartPos:    0,
		},
		{
			name:               "empty input falls through to statement name",
			input:              "",
			wantCompletionType: 0,
			wantArgPrefix:      "",
			wantArgStartPos:    0,
		},
		{
			name:               "SELECT query falls through to statement name",
			input:              "SELECT 1",
			wantCompletionType: 0,
			wantArgPrefix:      "SELECT 1",
			wantArgStartPos:    0,
		},
		{
			name:               "SHOW DATABASES falls through to statement name",
			input:              "SHOW DATABASES",
			wantCompletionType: 0,
			wantArgPrefix:      "SHOW DATABASES",
			wantArgStartPos:    0,
		},
		{
			name:               "SHOW VARIABLES falls through to statement name",
			input:              "SHOW VARIABLES",
			wantCompletionType: 0,
			wantArgPrefix:      "SHOW VARIABLES",
			wantArgStartPos:    0,
		},
		{
			name:               "leading spaces preserved in statement name fallback",
			input:              "  SHO",
			wantCompletionType: 0,
			wantArgPrefix:      "SHO",
			wantArgStartPos:    2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectFuzzyContext(tt.input)
			assert.Equal(t, tt.wantCompletionType, got.completionType)
			assert.Equal(t, tt.wantArgPrefix, got.argPrefix)
			assert.Equal(t, tt.wantArgStartPos, got.argStartPos)
			assert.Equal(t, tt.wantContext, got.context)
			assert.Equal(t, tt.wantSuffix, got.suffix)
		})
	}
}

func TestExtractFixedPrefix(t *testing.T) {
	tests := []struct {
		name   string
		syntax string
		want   string
	}{
		{
			name:   "no-arg statement",
			syntax: "SHOW DATABASES",
			want:   "SHOW DATABASES",
		},
		{
			name:   "single-arg with angle bracket",
			syntax: "USE <database> [ROLE <role>]",
			want:   "USE ",
		},
		{
			name:   "multi-keyword prefix",
			syntax: "SHOW COLUMNS FROM <table_fqn>",
			want:   "SHOW COLUMNS FROM ",
		},
		{
			name:   "optional bracket",
			syntax: "SHOW TABLES [<schema>]",
			want:   "SHOW TABLES ",
		},
		{
			name:   "curly brace alternatives",
			syntax: "START BATCH {DDL|DML}",
			want:   "START BATCH ",
		},
		{
			name:   "no-arg single word",
			syntax: "HELP",
			want:   "HELP",
		},
		{
			name:   "no-arg two words",
			syntax: "EXIT",
			want:   "EXIT",
		},
		{
			name:   "ellipsis in args",
			syntax: "DUMP TABLES <table1> [, <table2>, ...]",
			want:   "DUMP TABLES ",
		},
		{
			name:   "complex multi-keyword with angle bracket",
			syntax: "SHOW INDEX FROM <table_fqn>",
			want:   "SHOW INDEX FROM ",
		},
		{
			name:   "EXPLAIN with angle bracket",
			syntax: "EXPLAIN [FORMAT=<format>] [WIDTH=<width>] <sql>",
			want:   "EXPLAIN ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractFixedPrefix(tt.syntax)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildStatementNameCandidates(t *testing.T) {
	candidates := buildStatementNameCandidates()

	// Should have candidates (at least as many as defs with descriptions)
	assert.Greater(t, len(candidates), 0)

	// Each candidate should have non-empty DisplayText and InsertText
	for _, c := range candidates {
		assert.NotEmpty(t, c.DisplayText, "DisplayText should not be empty")
		assert.NotEmpty(t, c.InsertText, "InsertText should not be empty")
	}

	// Verify specific well-known candidates exist
	displayTexts := make(map[string]string)
	for _, c := range candidates {
		displayTexts[c.DisplayText] = c.InsertText
	}

	// No-arg statement: full text, no trailing space
	assert.Equal(t, "SHOW DATABASES", displayTexts["SHOW DATABASES"])

	// Arg statement: keyword prefix with trailing space
	assert.Equal(t, "USE ", displayTexts["USE <database> [ROLE <role>]"])

	// Multi-keyword prefix
	assert.Equal(t, "SHOW COLUMNS FROM ", displayTexts["SHOW COLUMNS FROM <table_fqn>"])
}

func TestStatementNameInsertText(t *testing.T) {
	tests := []struct {
		name        string
		displayText string
		want        string
	}{
		{
			name:        "known no-arg statement",
			displayText: "SHOW DATABASES",
			want:        "SHOW DATABASES",
		},
		{
			name:        "known arg statement",
			displayText: "USE <database> [ROLE <role>]",
			want:        "USE ",
		},
		{
			name:        "unknown display text falls back to input",
			displayText: "NONEXISTENT COMMAND",
			want:        "NONEXISTENT COMMAND",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := statementNameInsertText(tt.displayText)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestStatementNameDisplayTexts(t *testing.T) {
	texts := statementNameDisplayTexts()
	assert.Equal(t, len(statementNameCandidates), len(texts))
	for i, c := range statementNameCandidates {
		assert.Equal(t, c.DisplayText, texts[i])
	}
}
