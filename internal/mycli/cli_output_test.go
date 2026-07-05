package mycli

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/apstndb/spanner-mycli/enums"
)

// Helper functions for common test operations

func runPrintTableData(t *testing.T, mode enums.DisplayMode, skipColNames bool, result *Result) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	sysVars := &systemVariables{
		Display: DisplayVars{
			CLIFormat:       mode,
			SkipColumnNames: skipColNames,
		},
	}
	err := printTableData(sysVars, 0, &buf, result)
	return buf.String(), err
}

func assertErrorStatus(t *testing.T, err error, wantError bool) {
	t.Helper()
	if (err != nil) != wantError {
		t.Errorf("error = %v, wantError %v", err, wantError)
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
		{"set TSV", "TSV", enums.DisplayModeTSV, false},
		{"set HTML", "HTML", enums.DisplayModeHTML, false},
		{"set XML", "XML", enums.DisplayModeXML, false},
		{"set CSV", "CSV", enums.DisplayModeCSV, false},
		{"set html lowercase", "html", enums.DisplayModeHTML, false},
		{"set xml lowercase", "xml", enums.DisplayModeXML, false},
		{"set csv lowercase", "csv", enums.DisplayModeCSV, false},
		{"set tsv lowercase", "tsv", enums.DisplayModeTSV, false},
		{"set invalid", "INVALID", enums.DisplayModeTable, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sysVars := newSystemVariablesWithDefaultsForTest()
			sysVars.Display.CLIFormat = enums.DisplayModeTable

			err := sysVars.SetFromSimple("CLI_FORMAT", tt.setValue)

			if (err != nil) != tt.wantError {
				t.Errorf("Set() error = %v, wantError %v", err, tt.wantError)
			}

			if !tt.wantError && sysVars.Display.CLIFormat != tt.wantMode {
				t.Errorf("CLIFormat = %v, want %v", sysVars.Display.CLIFormat, tt.wantMode)
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
		{enums.DisplayModeTSV, "TSV"},
		{enums.DisplayModeHTML, "HTML"},
		{enums.DisplayModeXML, "XML"},
		{enums.DisplayModeCSV, "CSV"},
		{enums.DisplayMode(999), "DisplayMode(999)"}, // Invalid mode returns Go's default format
	}

	for _, tt := range tests {
		t.Run(tt.wantStr, func(t *testing.T) {
			sysVars := &systemVariables{
				Display: DisplayVars{
					CLIFormat: tt.mode,
				},
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
				Rows:        []Row{toRow("data")},
			},
			wantOutput: false,
			wantError:  false, // Now we skip formatting for empty results
		},
		{
			name: "Non-table mode with buffered data rows uses streaming formatter",
			mode: enums.DisplayModeHTML,
			result: &Result{
				TableHeader: toTableHeader("列1", "列2"),
				Rows: []Row{
					toRow("データ1", "データ2"),
					toRow("🌟", "🌙"),
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

func TestPrintTableDataStreamsBufferedRowsForNonTableModes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		mode       enums.DisplayMode
		result     *Result
		wantOutput string
	}{
		{
			name: "CSV",
			mode: enums.DisplayModeCSV,
			result: &Result{
				TableHeader: toTableHeader("col1"),
				Rows:        []Row{toRow("value")},
			},
			wantOutput: "col1\nvalue\n",
		},
		{
			name: "SQL export without SQL literals falls back to table",
			mode: enums.DisplayModeSQLInsert,
			result: &Result{
				TableHeader:      toTableHeader("col1"),
				Rows:             []Row{toRow("value")},
				SQLExportAllowed: false,
			},
			wantOutput: "+-------+\n| col1  |\n+-------+\n| value |\n+-------+\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := runPrintTableData(t, tt.mode, false, tt.result)
			if err != nil {
				t.Fatalf("printTableData() error = %v", err)
			}
			if got != tt.wantOutput {
				t.Fatalf("printTableData() = %q, want %q", got, tt.wantOutput)
			}
		})
	}
}

func TestPrintResultWritesRenderedOutput(t *testing.T) {
	t.Parallel()

	result := &Result{
		TableHeader:    toTableHeader("col1"),
		Rows:           []Row{toRow("should not be formatted")},
		RenderedOutput: []byte("rendered output\n"),
		AffectedRows:   1,
	}
	sysVars := &systemVariables{
		Display: DisplayVars{CLIFormat: enums.DisplayModeTable},
	}

	var buf bytes.Buffer
	if err := printResult(sysVars, 80, &buf, result, true); err != nil {
		t.Fatalf("printResult() error = %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "rendered output\n") {
		t.Fatalf("printResult() = %q, want rendered output", got)
	}
	if strings.Contains(got, "should not be formatted") {
		t.Fatalf("printResult() formatted Rows despite RenderedOutput: %q", got)
	}
}

// Test helpers for result line suppression

var resultLineRegex = regexp.MustCompile(`(?m)^(Query OK|\d+ rows (in set|affected)|Empty set)`)

func testResultLineSuppression(t *testing.T, sysVars *systemVariables, result *Result, interactive bool, expectedHasResult bool) {
	t.Helper()

	var buf bytes.Buffer
	err := printResult(sysVars, 80, &buf, result, interactive)
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
				toRow("value1", "value2"),
				toRow("value3", "value4"),
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
			sysVars.Display.SuppressResultLines = tt.suppressResultLines
			sysVars.Display.Verbose = tt.verbose
			sysVars.Display.CLIFormat = enums.DisplayModeTable

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
			sysVars.Display.SuppressResultLines = tt.suppressResultLines
			sysVars.Display.Verbose = tt.verbose
			sysVars.Display.CLIFormat = enums.DisplayModeTable

			testResultLineSuppression(t, &sysVars, tt.result, tt.interactive, tt.expectedHasResult)
		})
	}
}
