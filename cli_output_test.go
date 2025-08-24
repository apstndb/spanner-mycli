package main

import (
	"bytes"
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc/v2"
	"github.com/apstndb/spanner-mycli/enums"
)

// Helper functions for common test operations

func runPrintTableData(t *testing.T, mode enums.DisplayMode, skipColNames bool, result *Result) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	sysVars := &systemVariables{
		CLIFormat:       mode,
		SkipColumnNames: skipColNames,
	}
	err := printTableData(sysVars, 0, &buf, result)
	return buf.String(), err
}

func assertContains(t *testing.T, output string, want []string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(output, w) {
			t.Errorf("output missing %q\nGot:\n%s", w, output)
		}
	}
}

func assertNotContains(t *testing.T, output string, notWant []string) {
	t.Helper()
	for _, nw := range notWant {
		if strings.Contains(output, nw) {
			t.Errorf("output should not contain %q\nGot:\n%s", nw, output)
		}
	}
}

func assertErrorStatus(t *testing.T, err error, wantError bool) {
	t.Helper()
	if (err != nil) != wantError {
		t.Errorf("error = %v, wantError %v", err, wantError)
	}
}

func TestPrintTableDataHTML(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		result       *Result
		skipColNames bool
		wantContains []string
		wantOutput   string
		wantError    bool
	}{
		{
			name: "simple HTML output",
			result: &Result{
				TableHeader: toTableHeader("num", "str", "bool"),
				Rows: []Row{
					{"1", "test", "true"},
					{"2", "data", "false"},
				},
			},
			wantOutput: `<TABLE BORDER='1'><TR><TH>num</TH><TH>str</TH><TH>bool</TH></TR><TR><TD>1</TD><TD>test</TD><TD>true</TD></TR><TR><TD>2</TD><TD>data</TD><TD>false</TD></TR></TABLE>
`,
		},
		{
			name: "HTML with special characters escaping",
			result: &Result{
				TableHeader: toTableHeader("xml_chars", "quote", "ampersand"),
				Rows: []Row{
					{"<tag>", "\"quotes\"", "A&B"},
					{"<script>alert('xss')</script>", "'single'", "C&D"},
				},
			},
			wantContains: []string{
				"&lt;tag&gt;",
				"&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;",
				"&#34;quotes&#34;",
				"&#39;single&#39;",
				"A&amp;B",
				"C&amp;D",
			},
		},
		{
			name: "HTML with skip column names",
			result: &Result{
				TableHeader: toTableHeader("col1", "col2"),
				Rows: []Row{
					{"val1", "val2"},
				},
			},
			skipColNames: true,
			wantOutput: `<TABLE BORDER='1'><TR><TD>val1</TD><TD>val2</TD></TR></TABLE>
`,
		},
		{
			name: "empty result",
			result: &Result{
				TableHeader: nil,
				Rows:        []Row{},
			},
			wantOutput: "",
			wantError:  false, // Now we skip formatting for empty results
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := runPrintTableData(t, enums.DisplayModeHTML, tt.skipColNames, tt.result)
			assertErrorStatus(t, err, tt.wantError)
			if err != nil {
				return
			}

			if tt.wantOutput != "" && got != tt.wantOutput {
				t.Errorf("printTableData() = %q, want %q", got, tt.wantOutput)
			}

			assertContains(t, got, tt.wantContains)
		})
	}
}

func TestPrintTableDataXML(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		result          *Result
		skipColNames    bool
		wantContains    []string
		wantNotContains []string
		wantError       bool
	}{
		{
			name: "simple XML output",
			result: &Result{
				TableHeader: toTableHeader("num", "str", "bool"),
				Rows: []Row{
					{"1", "test", "true"},
					{"2", "data", "false"},
				},
			},
			wantContains: []string{
				"<?xml version='1.0'?>",
				`<resultset xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">`,
				"<header>",
				"<field>num</field>",
				"<field>str</field>",
				"<field>bool</field>",
				"</header>",
				"<row>",
				"<field>1</field>",
				"<field>test</field>",
				"<field>true</field>",
				"</row>",
				"<row>",
				"<field>2</field>",
				"<field>data</field>",
				"<field>false</field>",
				"</row>",
				"</resultset>",
			},
		},
		{
			name: "XML with special characters escaping",
			result: &Result{
				TableHeader: toTableHeader("xml_chars", "quote", "ampersand"),
				Rows: []Row{
					{"<tag>", "\"quotes\"", "A&B"},
					{"<script>alert('xss')</script>", "'single'", "C&D"},
				},
			},
			wantContains: []string{
				"&lt;tag&gt;",
				"&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;",
				"&#34;quotes&#34;",
				"&#39;single&#39;",
				"A&amp;B",
				"C&amp;D",
			},
		},
		{
			name: "XML with skip column names",
			result: &Result{
				TableHeader: toTableHeader("col1", "col2"),
				Rows: []Row{
					{"val1", "val2"},
				},
			},
			skipColNames: true,
			wantNotContains: []string{
				"<header>",
				"col1",
				"col2",
			},
			wantContains: []string{
				"<row>",
				"<field>val1</field>",
				"<field>val2</field>",
			},
		},
		{
			name: "empty result",
			result: &Result{
				TableHeader: nil,
				Rows:        []Row{},
			},
			wantContains: []string{},
			wantError:    false, // Now we skip formatting for empty results
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := runPrintTableData(t, enums.DisplayModeXML, tt.skipColNames, tt.result)
			assertErrorStatus(t, err, tt.wantError)
			if err != nil {
				return
			}

			assertContains(t, got, tt.wantContains)
			assertNotContains(t, got, tt.wantNotContains)
		})
	}
}

func TestCLIFormatSystemVariable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		setValue  string
		wantMode  enums.DisplayMode
		wantError bool
	}{
		{"set TABLE", "TABLE", enums.DisplayModeTable, false},
		{"set VERTICAL", "VERTICAL", enums.DisplayModeVertical, false},
		{"set TAB", "TAB", enums.DisplayModeTab, false},
		{"set HTML", "HTML", enums.DisplayModeHTML, false},
		{"set XML", "XML", enums.DisplayModeXML, false},
		{"set CSV", "CSV", enums.DisplayModeCSV, false},
		{"set html lowercase", "html", enums.DisplayModeHTML, false},
		{"set xml lowercase", "xml", enums.DisplayModeXML, false},
		{"set csv lowercase", "csv", enums.DisplayModeCSV, false},
		{"set invalid", "INVALID", enums.DisplayModeTable, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sysVars := newSystemVariablesWithDefaultsForTest()
			sysVars.CLIFormat = enums.DisplayModeTable

			err := sysVars.SetFromSimple("CLI_FORMAT", tt.setValue)

			if (err != nil) != tt.wantError {
				t.Errorf("Set() error = %v, wantError %v", err, tt.wantError)
			}

			if !tt.wantError && sysVars.CLIFormat != tt.wantMode {
				t.Errorf("CLIFormat = %v, want %v", sysVars.CLIFormat, tt.wantMode)
			}
		})
	}
}

func TestCLIFormatSystemVariableGetter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		mode    enums.DisplayMode
		wantStr string
	}{
		{enums.DisplayModeTable, "TABLE"},
		{enums.DisplayModeTableComment, "TABLE_COMMENT"},
		{enums.DisplayModeTableDetailComment, "TABLE_DETAIL_COMMENT"},
		{enums.DisplayModeVertical, "VERTICAL"},
		{enums.DisplayModeTab, "TAB"},
		{enums.DisplayModeHTML, "HTML"},
		{enums.DisplayModeXML, "XML"},
		{enums.DisplayModeCSV, "CSV"},
		{enums.DisplayMode(999), "DisplayMode(999)"}, // Invalid mode returns Go's default format
	}

	for _, tt := range tests {
		t.Run(tt.wantStr, func(t *testing.T) {
			sysVars := &systemVariables{
				CLIFormat: tt.mode,
			}

			got, err := sysVars.Get("CLI_FORMAT")
			if err != nil {
				t.Fatalf("Get() error = %v", err)
			}

			if got["CLI_FORMAT"] != tt.wantStr {
				t.Errorf("Get() = %v, want %v", got["CLI_FORMAT"], tt.wantStr)
			}
		})
	}
}

func TestPrintTableDataEdgeCases(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		mode       enums.DisplayMode
		result     *Result
		wantOutput bool // whether we expect any output
		wantError  bool
	}{
		{
			name: "HTML with nil header and empty rows",
			mode: enums.DisplayModeHTML,
			result: &Result{
				TableHeader: nil,
				Rows:        []Row{},
			},
			wantOutput: false,
			wantError:  false, // Now we skip formatting for empty results
		},
		{
			name: "XML with nil header and empty rows",
			mode: enums.DisplayModeXML,
			result: &Result{
				TableHeader: nil,
				Rows:        []Row{},
			},
			wantOutput: false,
			wantError:  false, // Now we skip formatting for empty results
		},
		{
			name: "HTML with empty column names but data rows",
			mode: enums.DisplayModeHTML,
			result: &Result{
				TableHeader: nil,
				Rows:        []Row{{"data"}},
			},
			wantOutput: false,
			wantError:  false, // Now we skip formatting for empty results
		},
		{
			name: "All display modes with unicode data",
			mode: enums.DisplayModeHTML,
			result: &Result{
				TableHeader: toTableHeader("列1", "列2"),
				Rows: []Row{
					{"データ1", "データ2"},
					{"🌟", "🌙"},
				},
			},
			wantOutput: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := runPrintTableData(t, tt.mode, false, tt.result)
			assertErrorStatus(t, err, tt.wantError)
			if err != nil {
				return
			}

			if tt.wantOutput && got == "" {
				t.Error("expected output but got empty string")
			} else if !tt.wantOutput && got != "" {
				t.Errorf("expected no output but got: %q", got)
			}
		})
	}
}

func TestFormatHelpers(t *testing.T) {
	t.Parallel()

	// Helper to test empty input handling for different formatters
	testEmptyFormatter := func(t *testing.T, name string, formatter func(io.Writer, *Result, []string, *systemVariables, int) error) {
		t.Helper()
		t.Run(name+" with empty input", func(t *testing.T) {
			var buf bytes.Buffer
			result := &Result{Rows: []Row{}}
			sysVars := &systemVariables{SkipColumnNames: false}
			err := formatter(&buf, result, []string{}, sysVars, 0)
			if err != nil {
				t.Errorf("expected nil for empty columns, got error: %v", err)
			}
			if buf.String() != "" {
				t.Errorf("expected empty output, got: %q", buf.String())
			}
		})
	}

	testEmptyFormatter(t, "formatHTML", formatHTML)
	testEmptyFormatter(t, "formatXML", formatXML)
	testEmptyFormatter(t, "formatCSV", formatCSV)

	t.Run("XML with large dataset", func(t *testing.T) {
		// Test with a larger dataset to ensure performance
		columns := []string{"id", "name", "value"}
		rows := make([]Row, 100)
		for i := range rows {
			rows[i] = Row{
				strings.Repeat("a", 100),
				strings.Repeat("b", 100),
				strings.Repeat("c", 100),
			}
		}

		var buf bytes.Buffer
		result := &Result{Rows: rows}
		sysVars := &systemVariables{SkipColumnNames: false}
		err := formatXML(&buf, result, columns, sysVars, 0)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		output := buf.String()
		assertContains(t, output, []string{
			"<?xml version='1.0'?>",
			"</resultset>",
		})
	})
}

func TestPrintTableDataCSV(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		result       *Result
		skipColNames bool
		wantOutput   string
		wantError    bool
	}{
		{
			name: "simple CSV output",
			result: &Result{
				TableHeader: toTableHeader("num", "str", "bool"),
				Rows: []Row{
					{"1", "test", "true"},
					{"2", "data", "false"},
				},
			},
			wantOutput: heredoc.Doc(`
				num,str,bool
				1,test,true
				2,data,false
			`),
		},
		{
			name: "CSV with special characters",
			result: &Result{
				TableHeader: toTableHeader("name", "description", "value"),
				Rows: []Row{
					{"John, Jr.", "Says \"Hello\"", "100"},
					{"Jane\nDoe", "Has,comma", "$50"},
					{"Bob", "Normal text", "75"},
				},
			},
			wantOutput: heredoc.Doc(`
				name,description,value
				"John, Jr.","Says ""Hello""",100
				"Jane
				Doe","Has,comma",$50
				Bob,Normal text,75
			`),
		},
		{
			name: "CSV with skip column names",
			result: &Result{
				TableHeader: toTableHeader("col1", "col2"),
				Rows: []Row{
					{"val1", "val2"},
					{"val3", "val4"},
				},
			},
			skipColNames: true,
			wantOutput: heredoc.Doc(`
				val1,val2
				val3,val4
			`),
		},
		{
			name: "empty result",
			result: &Result{
				TableHeader: nil,
				Rows:        []Row{},
			},
			wantOutput: "",
			wantError:  false, // Now we skip formatting for empty results
		},
		{
			name: "CSV with quotes and newlines",
			result: &Result{
				TableHeader: toTableHeader("text"),
				Rows: []Row{
					{"Line 1\nLine 2"},
					{"\"Quoted\""},
					{"Normal"},
				},
			},
			wantOutput: heredoc.Doc(`
				text
				"Line 1
				Line 2"
				"""Quoted"""
				Normal
			`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := runPrintTableData(t, enums.DisplayModeCSV, tt.skipColNames, tt.result)
			assertErrorStatus(t, err, tt.wantError)
			if err != nil {
				return
			}

			if got != tt.wantOutput {
				t.Errorf("printTableData() = %q, want %q", got, tt.wantOutput)
			}
		})
	}
}

func TestSQLExportFallbackForNonDataStatements(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		hasSQLFormatted bool
		cliFormat       enums.DisplayMode
		expectSQLOutput bool
	}{
		{
			name:            "SQL export with SQL-formatted values (SELECT result)",
			hasSQLFormatted: true,
			cliFormat:       enums.DisplayModeSQLInsert,
			expectSQLOutput: true,
		},
		{
			name:            "SQL export without SQL-formatted values (SHOW CREATE TABLE)",
			hasSQLFormatted: false,
			cliFormat:       enums.DisplayModeSQLInsert,
			expectSQLOutput: false, // Falls back to table
		},
		{
			name:            "SQL INSERT OR UPDATE without SQL-formatted values (EXPLAIN)",
			hasSQLFormatted: false,
			cliFormat:       enums.DisplayModeSQLInsertOrUpdate,
			expectSQLOutput: false,
		},
		{
			name:            "SQL INSERT OR IGNORE without SQL-formatted values (SHOW TABLES)",
			hasSQLFormatted: false,
			cliFormat:       enums.DisplayModeSQLInsertOrIgnore,
			expectSQLOutput: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &Result{
				TableHeader:           toTableHeader("Name", "DDL"),
				Rows:                  []Row{{"TestTable", "CREATE TABLE TestTable (id INT64)"}},
				HasSQLFormattedValues: tt.hasSQLFormatted,
			}

			sysVars := &systemVariables{
				CLIFormat:    tt.cliFormat,
				SQLTableName: "TargetTable",
				SQLBatchSize: 0,
			}

			var buf bytes.Buffer
			err := printTableData(sysVars, 0, &buf, result)

			output := buf.String()
			containsInsert := strings.Contains(output, "INSERT")

			if tt.expectSQLOutput {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if !containsInsert {
					t.Errorf("Expected SQL INSERT output, got: %s", output)
				}
			} else {
				// For metadata results, should fall back to table format
				if containsInsert {
					t.Errorf("Should not contain INSERT for metadata results, got: %s", output)
				}
				// Should see table format markers or be table output
				if tt.hasSQLFormatted == false && output != "" {
					// Verify it's table format (has column headers at minimum)
					if !strings.Contains(output, "Name") || !strings.Contains(output, "DDL") {
						t.Errorf("Expected table format with headers, got: %s", output)
					}
				}
			}
		})
	}
}

// Test helpers for result line suppression

var resultLineRegex = regexp.MustCompile(`(?m)^(Query OK|\d+ rows (in set|affected)|Empty set)`)

func testResultLineSuppression(t *testing.T, sysVars *systemVariables, result *Result, interactive bool, expectedHasResult bool) {
	t.Helper()

	var buf bytes.Buffer
	err := printResult(sysVars, 80, &buf, result, interactive, "")
	if err != nil {
		t.Fatalf("printResult failed: %v", err)
	}

	output := buf.String()
	hasResultLine := resultLineRegex.MatchString(output)

	if hasResultLine != expectedHasResult {
		t.Errorf("Result line presence mismatch: got %v, want %v\nOutput:\n%s",
			hasResultLine, expectedHasResult, output)
	}
}

func TestSuppressResultLines(t *testing.T) {
	t.Parallel()

	// Create a standard test result for reuse
	createTestResult := func() *Result {
		return &Result{
			Rows: []Row{
				{"value1", "value2"},
				{"value3", "value4"},
			},
			TableHeader:  toTableHeader("Column1", "Column2"),
			AffectedRows: 2,
			Stats: QueryStats{
				ElapsedTime: "0.5 sec",
			},
		}
	}

	tests := []struct {
		name                string
		suppressResultLines bool
		verbose             bool
		interactive         bool
		expectedHasResult   bool
	}{
		{
			name:                "Normal mode - show result line",
			suppressResultLines: false,
			interactive:         true,
			expectedHasResult:   true,
		},
		{
			name:                "Suppress enabled - no result line",
			suppressResultLines: true,
			interactive:         true,
			expectedHasResult:   false,
		},
		{
			name:                "Batch mode without verbose - no result line",
			suppressResultLines: false,
			interactive:         false,
			verbose:             false,
			expectedHasResult:   false,
		},
		{
			name:                "Batch mode with verbose - show result line",
			suppressResultLines: false,
			interactive:         false,
			verbose:             true,
			expectedHasResult:   true,
		},
		{
			name:                "Batch mode with verbose but suppressed - no result line",
			suppressResultLines: true,
			interactive:         false,
			verbose:             true,
			expectedHasResult:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sysVars := newSystemVariablesWithDefaults()
			sysVars.SuppressResultLines = tt.suppressResultLines
			sysVars.Verbose = tt.verbose
			sysVars.CLIFormat = enums.DisplayModeTable

			testResultLineSuppression(t, &sysVars, createTestResult(), tt.interactive, tt.expectedHasResult)
		})
	}
}

func TestSuppressResultLinesDMLAndDDL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                string
		suppressResultLines bool
		verbose             bool
		interactive         bool
		result              *Result
		expectedHasResult   bool
	}{
		{
			name:                "DML with suppression - no result line",
			suppressResultLines: true,
			interactive:         true,
			result: &Result{
				IsExecutedDML: true,
				AffectedRows:  5,
				Stats: QueryStats{
					ElapsedTime: "0.2 sec",
				},
			},
			expectedHasResult: false,
		},
		{
			name:                "DML without suppression - show result line",
			suppressResultLines: false,
			interactive:         true,
			result: &Result{
				IsExecutedDML: true,
				AffectedRows:  5,
				Stats: QueryStats{
					ElapsedTime: "0.2 sec",
				},
			},
			expectedHasResult: true,
		},
		{
			name:                "DDL with suppression - no result line",
			suppressResultLines: true,
			interactive:         true,
			result: &Result{
				Stats: QueryStats{
					ElapsedTime: "1.5 sec",
				},
			},
			expectedHasResult: false,
		},
		{
			name:                "DDL without suppression - show result line",
			suppressResultLines: false,
			interactive:         true,
			result: &Result{
				Stats: QueryStats{
					ElapsedTime: "1.5 sec",
				},
			},
			expectedHasResult: true,
		},
		{
			name:                "Empty result set with suppression",
			suppressResultLines: true,
			interactive:         true,
			result: &Result{
				Rows:         []Row{},
				TableHeader:  toTableHeader("Column1"),
				AffectedRows: 0,
				Stats: QueryStats{
					ElapsedTime: "0.1 sec",
				},
			},
			expectedHasResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sysVars := newSystemVariablesWithDefaults()
			sysVars.SuppressResultLines = tt.suppressResultLines
			sysVars.Verbose = tt.verbose
			sysVars.CLIFormat = enums.DisplayModeTable

			testResultLineSuppression(t, &sysVars, tt.result, tt.interactive, tt.expectedHasResult)
		})
	}
}
