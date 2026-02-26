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
	"fmt"
	"strings"
	"testing"
	"time"

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
		// Argument completion: MUTATE → table
		{
			name:               "MUTATE with trailing space",
			input:              "MUTATE ",
			wantCompletionType: fuzzyCompleteTable,
			wantArgPrefix:      "",
			wantArgStartPos:    7,
		},
		{
			name:               "MUTATE with partial table",
			input:              "MUTATE Sin",
			wantCompletionType: fuzzyCompleteTable,
			wantArgPrefix:      "Sin",
			wantArgStartPos:    7,
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
		// Argument completion: SHOW OPERATION → operation
		{
			name:               "SHOW OPERATION with trailing space",
			input:              "SHOW OPERATION ",
			wantCompletionType: fuzzyCompleteOperation,
			wantArgPrefix:      "",
			wantArgStartPos:    15,
		},
		{
			name:               "SHOW OPERATION with partial id",
			input:              "SHOW OPERATION _auto",
			wantCompletionType: fuzzyCompleteOperation,
			wantArgPrefix:      "_auto",
			wantArgStartPos:    15,
		},
		{
			name:               "show operation lowercase",
			input:              "show operation ",
			wantCompletionType: fuzzyCompleteOperation,
			wantArgPrefix:      "",
			wantArgStartPos:    15,
		},
		// Argument completion: SHOW CREATE VIEW → view
		{
			name:               "SHOW CREATE VIEW with trailing space",
			input:              "SHOW CREATE VIEW ",
			wantCompletionType: fuzzyCompleteView,
			wantArgPrefix:      "",
			wantArgStartPos:    17,
		},
		{
			name:               "SHOW CREATE VIEW with partial name",
			input:              "SHOW CREATE VIEW My",
			wantCompletionType: fuzzyCompleteView,
			wantArgPrefix:      "My",
			wantArgStartPos:    17,
		},
		{
			name:               "show create view lowercase",
			input:              "show create view ",
			wantCompletionType: fuzzyCompleteView,
			wantArgPrefix:      "",
			wantArgStartPos:    17,
		},
		// Argument completion: SHOW CREATE INDEX → index
		{
			name:               "SHOW CREATE INDEX with trailing space",
			input:              "SHOW CREATE INDEX ",
			wantCompletionType: fuzzyCompleteIndex,
			wantArgPrefix:      "",
			wantArgStartPos:    18,
		},
		{
			name:               "SHOW CREATE INDEX with partial name",
			input:              "SHOW CREATE INDEX Idx",
			wantCompletionType: fuzzyCompleteIndex,
			wantArgPrefix:      "Idx",
			wantArgStartPos:    18,
		},
		{
			name:               "show create index lowercase",
			input:              "show create index ",
			wantCompletionType: fuzzyCompleteIndex,
			wantArgPrefix:      "",
			wantArgStartPos:    18,
		},
		// Argument completion: SHOW CREATE CHANGE STREAM → change stream
		{
			name:               "SHOW CREATE CHANGE STREAM with trailing space",
			input:              "SHOW CREATE CHANGE STREAM ",
			wantCompletionType: fuzzyCompleteChangeStream,
			wantArgPrefix:      "",
			wantArgStartPos:    26,
		},
		{
			name:               "SHOW CREATE CHANGE STREAM with partial name",
			input:              "SHOW CREATE CHANGE STREAM My",
			wantCompletionType: fuzzyCompleteChangeStream,
			wantArgPrefix:      "My",
			wantArgStartPos:    26,
		},
		{
			name:               "show create change stream lowercase",
			input:              "show create change stream ",
			wantCompletionType: fuzzyCompleteChangeStream,
			wantArgPrefix:      "",
			wantArgStartPos:    26,
		},
		// Argument completion: SHOW CREATE SEQUENCE → sequence
		{
			name:               "SHOW CREATE SEQUENCE with trailing space",
			input:              "SHOW CREATE SEQUENCE ",
			wantCompletionType: fuzzyCompleteSequence,
			wantArgPrefix:      "",
			wantArgStartPos:    21,
		},
		{
			name:               "SHOW CREATE SEQUENCE with partial name",
			input:              "SHOW CREATE SEQUENCE My",
			wantCompletionType: fuzzyCompleteSequence,
			wantArgPrefix:      "My",
			wantArgStartPos:    21,
		},
		{
			name:               "show create sequence lowercase",
			input:              "show create sequence ",
			wantCompletionType: fuzzyCompleteSequence,
			wantArgPrefix:      "",
			wantArgStartPos:    21,
		},
		// Argument completion: SHOW CREATE MODEL → model
		{
			name:               "SHOW CREATE MODEL with trailing space",
			input:              "SHOW CREATE MODEL ",
			wantCompletionType: fuzzyCompleteModel,
			wantArgPrefix:      "",
			wantArgStartPos:    18,
		},
		{
			name:               "SHOW CREATE MODEL with partial name",
			input:              "SHOW CREATE MODEL My",
			wantCompletionType: fuzzyCompleteModel,
			wantArgPrefix:      "My",
			wantArgStartPos:    18,
		},
		{
			name:               "show create model lowercase",
			input:              "show create model ",
			wantCompletionType: fuzzyCompleteModel,
			wantArgPrefix:      "",
			wantArgStartPos:    18,
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

func TestBuildStatementNameItems(t *testing.T) {
	items := buildStatementNameItems()

	// Should have candidates (at least as many as defs with descriptions)
	assert.Greater(t, len(items), 0)

	// Each item should have non-empty Value and Label
	for _, item := range items {
		assert.NotEmpty(t, item.Value, "Value should not be empty")
		assert.NotEmpty(t, item.Label, "Label should not be empty")
	}

	// Verify specific well-known candidates exist
	labelToValue := make(map[string]string)
	for _, item := range items {
		labelToValue[item.Label] = item.Value
	}

	// No-arg statement: full text, no trailing space
	assert.Equal(t, "SHOW DATABASES", labelToValue["SHOW DATABASES"])

	// Arg statement: keyword prefix with trailing space
	assert.Equal(t, "USE ", labelToValue["USE <database> [ROLE <role>]"])

	// Multi-keyword prefix
	assert.Equal(t, "SHOW COLUMNS FROM ", labelToValue["SHOW COLUMNS FROM <table_fqn>"])
}

func TestToFzfItems(t *testing.T) {
	input := []string{"alpha", "beta", "gamma"}
	items := toFzfItems(input)

	assert.Equal(t, len(input), len(items))
	for i, item := range items {
		assert.Equal(t, input[i], item.Value)
		assert.Empty(t, item.Label, "Label should be empty for simple items")
	}
}

func TestPrepareFzfOptions(t *testing.T) {
	tests := []struct {
		name               string
		candidates         []fzfItem
		header             string
		wantHasLabels      bool
		wantDelimiterArg   bool
		wantMultilineArgs  bool
		wantFixedHeight    bool // true = fixed height (no ~), false = shrink height (~N)
		wantFormattedCount int
	}{
		{
			name: "simple candidates without labels",
			candidates: []fzfItem{
				{Value: "db1"},
				{Value: "db2"},
			},
			header:             "",
			wantHasLabels:      false,
			wantDelimiterArg:   false,
			wantMultilineArgs:  false,
			wantFixedHeight:    false,
			wantFormattedCount: 2,
		},
		{
			name: "candidates with labels",
			candidates: []fzfItem{
				{Value: "USE ", Label: "USE <database> [ROLE <role>]"},
				{Value: "SHOW DATABASES", Label: "SHOW DATABASES"},
			},
			header:             "Statements",
			wantHasLabels:      true,
			wantDelimiterArg:   true,
			wantMultilineArgs:  false,
			wantFixedHeight:    false,
			wantFormattedCount: 2,
		},
		{
			name: "multiline candidates use fixed height",
			candidates: []fzfItem{
				{Value: "op1", Label: "[DONE] CREATE TABLE t1(\n  col1 INT64\n)"},
				{Value: "op2", Label: "[RUNNING] ALTER TABLE t2\n  ADD COLUMN c STRING(MAX)"},
			},
			header:             "Operations",
			wantHasLabels:      true,
			wantDelimiterArg:   true,
			wantMultilineArgs:  true,
			wantFixedHeight:    true,
			wantFormattedCount: 2,
		},
		{
			name: "mixed single and multiline",
			candidates: []fzfItem{
				{Value: "op1", Label: "[DONE] single line"},
				{Value: "op2", Label: "[RUNNING] CREATE TABLE t1(\n  col1 INT64\n)"},
			},
			header:             "",
			wantHasLabels:      true,
			wantDelimiterArg:   true,
			wantMultilineArgs:  true,
			wantFixedHeight:    true,
			wantFormattedCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := prepareFzfOptions(tt.candidates, tt.header)

			assert.Equal(t, tt.wantHasLabels, p.hasLabels)
			assert.Equal(t, tt.wantFormattedCount, len(p.formattedLines))

			// Check delimiter args presence.
			hasDelimiter := false
			hasWithNth := false
			hasReadZero := false
			hasMultiLine := false
			hasGap := false
			hasFixedHeight := false
			for _, arg := range p.args {
				if strings.HasPrefix(arg, "--delimiter=") {
					hasDelimiter = true
				}
				if strings.HasPrefix(arg, "--with-nth=") {
					hasWithNth = true
				}
				if arg == "--read0" {
					hasReadZero = true
				}
				if arg == "--multi-line" {
					hasMultiLine = true
				}
				if arg == "--gap" {
					hasGap = true
				}
				if heightVal, ok := strings.CutPrefix(arg, "--height="); ok {
					hasFixedHeight = !strings.HasPrefix(heightVal, "~")
				}
			}

			assert.Equal(t, tt.wantDelimiterArg, hasDelimiter, "delimiter arg")
			assert.Equal(t, tt.wantDelimiterArg, hasWithNth, "with-nth arg")
			assert.Equal(t, tt.wantMultilineArgs, hasReadZero, "read0 arg")
			assert.Equal(t, tt.wantMultilineArgs, hasMultiLine, "multi-line arg")
			assert.Equal(t, tt.wantMultilineArgs, hasGap, "gap arg")
			assert.Equal(t, tt.wantFixedHeight, hasFixedHeight, "fixed height")

			// Verify formatted lines structure.
			if tt.wantHasLabels {
				for _, line := range p.formattedLines {
					assert.Contains(t, line, fzfDelimiter, "label lines should contain delimiter")
				}
			}
		})
	}
}

func TestPrepareFzfOptions_DynamicHeight(t *testing.T) {
	// Verify height calculation for multiline items.
	candidates := []fzfItem{
		{Value: "op1", Label: "[DONE] line1\nline2\nline3"}, // 3 lines
		{Value: "op2", Label: "[RUNNING] line1\nline2"},     // 2 lines
	}

	p := prepareFzfOptions(candidates, "Operations")

	// Find the height arg.
	var heightVal string
	for _, arg := range p.args {
		if v, ok := strings.CutPrefix(arg, "--height="); ok {
			heightVal = v
		}
	}

	// Expected: totalDisplayLines(5) + gaps(1) + extra(5: border(2)+prompt(1)+separator(1)+header(1))
	// = 11, capped at min(11, 20) = 11
	assert.Equal(t, "11", heightVal)
}

func TestPrepareFzfOptions_HeightCap(t *testing.T) {
	// Many multiline items should cap at 20.
	var candidates []fzfItem
	for i := range 10 {
		candidates = append(candidates, fzfItem{
			Value: fmt.Sprintf("op%d", i),
			Label: "[DONE] line1\nline2\nline3\nline4",
		})
	}

	p := prepareFzfOptions(candidates, "Operations")

	var heightVal string
	for _, arg := range p.args {
		if v, ok := strings.CutPrefix(arg, "--height="); ok {
			heightVal = v
		}
	}

	// totalDisplayLines(40) + gaps(9) + extra(5) = 54, capped at 20
	assert.Equal(t, "20", heightVal)
}

func TestRunFzfFilter_SimpleMatch(t *testing.T) {
	candidates := []fzfItem{
		{Value: "alpha"},
		{Value: "beta"},
		{Value: "gamma"},
		{Value: "alphabet"},
	}

	results := runFzfFilter(candidates, "alph", "")
	assert.Contains(t, results, "alpha")
	assert.Contains(t, results, "alphabet")
	assert.NotContains(t, results, "beta")
	assert.NotContains(t, results, "gamma")
}

func TestRunFzfFilter_LabelValueSeparation(t *testing.T) {
	candidates := []fzfItem{
		{Value: "op-123", Label: "[DONE] CREATE TABLE users"},
		{Value: "op-456", Label: "[RUNNING] ALTER TABLE orders"},
		{Value: "op-789", Label: "[ERROR] DROP TABLE temp"},
	}

	// Filter by label text, but results should be Values.
	results := runFzfFilter(candidates, "CREATE", "Operations")
	assert.Contains(t, results, "op-123")
	assert.NotContains(t, results, "op-456")
	assert.NotContains(t, results, "op-789")
}

func TestRunFzfFilter_StatementNames(t *testing.T) {
	items := buildStatementNameItems()

	// Filter for "SHOW DATABASES" — should match the statement and return its insert text.
	results := runFzfFilter(items, "SHOW DATABASES", "Statements")
	assert.Contains(t, results, "SHOW DATABASES", "should find the no-arg statement")
}

func TestExtractValue(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		hasLabels bool
		want      string
	}{
		{
			name:      "no labels",
			line:      "alpha",
			hasLabels: false,
			want:      "alpha",
		},
		{
			name:      "with labels",
			line:      "op-123" + fzfDelimiter + "[DONE] CREATE TABLE",
			hasLabels: true,
			want:      "op-123",
		},
		{
			name:      "with labels no delimiter in line",
			line:      "alpha",
			hasLabels: true,
			want:      "alpha",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractValue(tt.line, tt.hasLabels)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFuzzyCacheEntryValid(t *testing.T) {
	session1 := &Session{}
	session2 := &Session{}
	session1WithDDL := &Session{schemaGeneration: 1}

	tests := []struct {
		name        string
		entry       *fuzzyCacheEntry
		session     *Session
		checkSchema bool
		want        bool
	}{
		{
			name:    "nil entry",
			entry:   nil,
			session: session1,
			want:    false,
		},
		{
			name: "different session",
			entry: &fuzzyCacheEntry{
				session:   session1,
				expiresAt: time.Now().Add(time.Minute),
			},
			session: session2,
			want:    false,
		},
		{
			name: "expired",
			entry: &fuzzyCacheEntry{
				session:   session1,
				expiresAt: time.Now().Add(-time.Second),
			},
			session: session1,
			want:    false,
		},
		{
			name: "valid without schema check",
			entry: &fuzzyCacheEntry{
				session:    session1,
				expiresAt:  time.Now().Add(time.Minute),
				candidates: []fzfItem{{Value: "db1"}},
			},
			session: session1,
			want:    true,
		},
		{
			name: "valid with matching schema generation",
			entry: &fuzzyCacheEntry{
				session:          session1,
				expiresAt:        time.Now().Add(time.Minute),
				schemaGeneration: 0,
				candidates:       []fzfItem{{Value: "table1"}},
			},
			session:     session1,
			checkSchema: true,
			want:        true,
		},
		{
			name: "stale schema generation",
			entry: &fuzzyCacheEntry{
				session:          session1WithDDL,
				expiresAt:        time.Now().Add(time.Minute),
				schemaGeneration: 0,
			},
			session:     session1WithDDL,
			checkSchema: true,
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.entry.valid(tt.session, tt.checkSchema)
			assert.Equal(t, tt.want, got)
		})
	}
}
