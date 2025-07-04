package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintTableDataHTML(t *testing.T) {
	tests := []struct {
		name          string
		result        *Result
		skipColNames  bool
		wantContains  []string
		wantOutput    string
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
		name          string
		result        *Result
		skipColNames  bool
		wantContains  []string
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
		{"set html lowercase", "html", DisplayModeHTML, false},
		{"set xml lowercase", "xml", DisplayModeXML, false},
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
		mode     DisplayMode
		wantStr  string
	}{
		{DisplayModeTable, "TABLE"},
		{DisplayModeTableComment, "TABLE_COMMENT"},
		{DisplayModeTableDetailComment, "TABLE_DETAIL_COMMENT"},
		{DisplayModeVertical, "VERTICAL"},
		{DisplayModeTab, "TAB"},
		{DisplayModeHTML, "HTML"},
		{DisplayModeXML, "XML"},
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