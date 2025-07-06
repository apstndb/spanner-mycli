package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc/v2"
)

func TestPrintTableDataHTML(t *testing.T) {
	tests := []struct {
		name         string
		result       *Result
		skipColNames bool
		wantContains []string
		wantOutput   string
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
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			sysVars := &systemVariables{
				CLIFormat:       DisplayModeHTML,
				SkipColumnNames: tt.skipColNames,
			}

			printTableData(sysVars, 0, &buf, tt.result)

			got := buf.String()

			if tt.wantOutput != "" && got != tt.wantOutput {
				t.Errorf("printTableData() = %q, want %q", got, tt.wantOutput)
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("printTableData() output missing %q", want)
				}
			}
		})
	}
}

func TestPrintTableDataXML(t *testing.T) {
	tests := []struct {
		name            string
		result          *Result
		skipColNames    bool
		wantContains    []string
		wantNotContains []string
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
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			sysVars := &systemVariables{
				CLIFormat:       DisplayModeXML,
				SkipColumnNames: tt.skipColNames,
			}

			printTableData(sysVars, 0, &buf, tt.result)

			got := buf.String()

			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("printTableData() output missing %q\nGot:\n%s", want, got)
				}
			}

			for _, notWant := range tt.wantNotContains {
				if strings.Contains(got, notWant) {
					t.Errorf("printTableData() output should not contain %q\nGot:\n%s", notWant, got)
				}
			}
		})
	}
}

func TestCLIFormatSystemVariable(t *testing.T) {
	tests := []struct {
		name      string
		setValue  string
		wantMode  DisplayMode
		wantError bool
	}{
		{"set TABLE", "TABLE", DisplayModeTable, false},
		{"set VERTICAL", "VERTICAL", DisplayModeVertical, false},
		{"set TAB", "TAB", DisplayModeTab, false},
		{"set HTML", "HTML", DisplayModeHTML, false},
		{"set XML", "XML", DisplayModeXML, false},
		{"set CSV", "CSV", DisplayModeCSV, false},
		{"set html lowercase", "html", DisplayModeHTML, false},
		{"set xml lowercase", "xml", DisplayModeXML, false},
		{"set csv lowercase", "csv", DisplayModeCSV, false},
		{"set invalid", "INVALID", DisplayModeTable, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sysVars := &systemVariables{
				CLIFormat: DisplayModeTable,
			}

			err := sysVars.Set("CLI_FORMAT", tt.setValue)

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
	tests := []struct {
		mode    DisplayMode
		wantStr string
	}{
		{DisplayModeTable, "TABLE"},
		{DisplayModeTableComment, "TABLE_COMMENT"},
		{DisplayModeTableDetailComment, "TABLE_DETAIL_COMMENT"},
		{DisplayModeVertical, "VERTICAL"},
		{DisplayModeTab, "TAB"},
		{DisplayModeHTML, "HTML"},
		{DisplayModeXML, "XML"},
		{DisplayModeCSV, "CSV"},
		{DisplayMode(999), "TABLE"}, // Invalid mode should return default
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
	tests := []struct {
		name       string
		mode       DisplayMode
		result     *Result
		wantOutput bool // whether we expect any output
	}{
		{
			name: "HTML with nil header and empty rows",
			mode: DisplayModeHTML,
			result: &Result{
				TableHeader: nil,
				Rows:        []Row{},
			},
			wantOutput: false,
		},
		{
			name: "XML with nil header and empty rows",
			mode: DisplayModeXML,
			result: &Result{
				TableHeader: nil,
				Rows:        []Row{},
			},
			wantOutput: false,
		},
		{
			name: "HTML with empty column names but data rows",
			mode: DisplayModeHTML,
			result: &Result{
				TableHeader: nil,
				Rows:        []Row{{"data"}},
			},
			wantOutput: false,
		},
		{
			name: "All display modes with unicode data",
			mode: DisplayModeHTML,
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
			var buf bytes.Buffer
			sysVars := &systemVariables{
				CLIFormat: tt.mode,
			}

			printTableData(sysVars, 0, &buf, tt.result)

			got := buf.String()
			if tt.wantOutput && got == "" {
				t.Error("expected output but got empty string")
			} else if !tt.wantOutput && got != "" {
				t.Errorf("expected no output but got: %q", got)
			}
		})
	}
}

func TestHTMLAndXMLHelpers(t *testing.T) {
	t.Run("printHTMLTable with empty input", func(t *testing.T) {
		var buf bytes.Buffer
		err := printHTMLTable(&buf, []string{}, []Row{}, false)
		if err == nil {
			t.Error("expected error for empty columns, got nil")
		}
		if err != nil && err.Error() != "no columns to output" {
			t.Errorf("unexpected error message: %v", err)
		}
		if buf.String() != "" {
			t.Errorf("expected empty output, got: %q", buf.String())
		}
	})

	t.Run("printXMLResultSet with empty input", func(t *testing.T) {
		var buf bytes.Buffer
		err := printXMLResultSet(&buf, []string{}, []Row{}, false)
		if err == nil {
			t.Error("expected error for empty columns, got nil")
		}
		if err != nil && err.Error() != "no columns to output" {
			t.Errorf("unexpected error message: %v", err)
		}
		if buf.String() != "" {
			t.Errorf("expected empty output, got: %q", buf.String())
		}
	})

	t.Run("printCSVTable with empty input", func(t *testing.T) {
		var buf bytes.Buffer
		err := printCSVTable(&buf, []string{}, []Row{}, false)
		if err == nil {
			t.Error("expected error for empty columns, got nil")
		}
		if err != nil && err.Error() != "no columns to output" {
			t.Errorf("unexpected error message: %v", err)
		}
		if buf.String() != "" {
			t.Errorf("expected empty output, got: %q", buf.String())
		}
	})

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
		err := printXMLResultSet(&buf, columns, rows, false)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		output := buf.String()
		if !strings.Contains(output, "<?xml version='1.0'?>") {
			t.Error("XML declaration missing")
		}
		if !strings.Contains(output, "</resultset>") {
			t.Error("XML closing tag missing")
		}
	})
}

func TestPrintTableDataCSV(t *testing.T) {
	tests := []struct {
		name         string
		result       *Result
		skipColNames bool
		wantOutput   string
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
			var buf bytes.Buffer
			sysVars := &systemVariables{
				CLIFormat:       DisplayModeCSV,
				SkipColumnNames: tt.skipColNames,
			}

			printTableData(sysVars, 0, &buf, tt.result)

			got := buf.String()
			if got != tt.wantOutput {
				t.Errorf("printTableData() = %q, want %q", got, tt.wantOutput)
			}
		})
	}
}
