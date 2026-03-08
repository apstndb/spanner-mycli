package format

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestNewFormatter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mode    Mode
		wantErr bool
	}{
		{name: "unspecified", mode: "UNSPECIFIED"},
		{name: "empty", mode: ""},
		{name: "table", mode: ModeTable},
		{name: "table_comment", mode: ModeTableComment},
		{name: "table_detail_comment", mode: ModeTableDetailComment},
		{name: "vertical", mode: ModeVertical},
		{name: "tab", mode: ModeTab},
		{name: "csv", mode: ModeCSV},
		{name: "html", mode: ModeHTML},
		{name: "xml", mode: ModeXML},
		{name: "invalid", mode: Mode("NONEXISTENT"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fn, err := NewFormatter(tt.mode)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if fn == nil {
				t.Error("expected non-nil FormatFunc")
			}
		})
	}
}

func TestNewStreamingFormatter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mode    Mode
		config  FormatConfig
		out     io.Writer
		wantErr bool
	}{
		{name: "csv", mode: ModeCSV, out: io.Discard},
		{name: "tab", mode: ModeTab, out: io.Discard},
		{name: "vertical", mode: ModeVertical, out: io.Discard},
		{name: "html", mode: ModeHTML, out: io.Discard},
		{name: "xml", mode: ModeXML, out: io.Discard},
		// Table formats with io.Discard are allowed (for isStreamingSupported check)
		{name: "table_discard", mode: ModeTable, out: io.Discard},
		// Table formats with real writer require screenWidth
		{name: "table_real_writer", mode: ModeTable, out: &bytes.Buffer{}, wantErr: true},
		{name: "invalid", mode: Mode("NONEXISTENT"), out: io.Discard, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sf, err := NewStreamingFormatter(tt.mode, tt.out, tt.config)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if sf == nil {
				t.Error("expected non-nil StreamingFormatter")
			}
		})
	}
}

func TestNewFormatter_RegisteredMode(t *testing.T) {
	// No t.Parallel(): mutates global registry
	testMode := Mode("TEST_CUSTOM")
	RegisterFormatFunc(func(mode Mode) (FormatFunc, error) {
		return func(out io.Writer, rows []Row, columnNames []string, config FormatConfig, screenWidth int) error {
			return nil
		}, nil
	}, testMode)
	t.Cleanup(func() { unregisterFormatFunc(testMode) })

	fn, err := NewFormatter(testMode)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fn == nil {
		t.Error("expected non-nil FormatFunc from registered mode")
	}
}

func TestNewStreamingFormatter_RegisteredMode(t *testing.T) {
	// No t.Parallel(): mutates global registry

	// Register a custom streaming mode
	testMode := Mode("TEST_STREAMING_CUSTOM")
	RegisterStreamingFormatter(func(mode Mode, out io.Writer, config FormatConfig) (StreamingFormatter, error) {
		return NewCSVFormatter(out, false), nil
	}, testMode)
	t.Cleanup(func() { unregisterStreamingFormatter(testMode) })

	sf, err := NewStreamingFormatter(testMode, io.Discard, FormatConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sf == nil {
		t.Error("expected non-nil StreamingFormatter from registered mode")
	}
}

func TestValueFormatModeFor(t *testing.T) {
	// No t.Parallel(): mutates global registry

	// Built-in modes should return DisplayValues
	for _, mode := range []Mode{ModeTable, ModeCSV, ModeTab, ModeVertical, ModeHTML, ModeXML} {
		if got := ValueFormatModeFor(mode); got != DisplayValues {
			t.Errorf("ValueFormatModeFor(%s) = %d, want DisplayValues", mode, got)
		}
	}

	// Unknown mode should return DisplayValues
	if got := ValueFormatModeFor(Mode("NONEXISTENT")); got != DisplayValues {
		t.Errorf("ValueFormatModeFor(NONEXISTENT) = %d, want DisplayValues", got)
	}

	// Registered mode with SQLLiteralValues
	testMode := Mode("TEST_SQL_LITERAL")
	RegisterValueFormatMode(SQLLiteralValues, testMode)
	t.Cleanup(func() { unregisterValueFormatMode(testMode) })
	if got := ValueFormatModeFor(testMode); got != SQLLiteralValues {
		t.Errorf("ValueFormatModeFor(%s) = %d, want SQLLiteralValues", testMode, got)
	}
}

func TestExecuteWithFormatter_EmptyColumns(t *testing.T) {
	t.Parallel()

	err := ExecuteWithFormatter(NewCSVFormatter(io.Discard, false), nil, nil, FormatConfig{})
	if err != nil {
		t.Fatalf("expected nil error for empty columns, got: %v", err)
	}
}

func TestExecuteWithFormatter_CSVOutput(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	columns := []string{"id", "name"}
	rows := []Row{StringsToRow("1", "Alice"), StringsToRow("2", "Bob")}

	err := ExecuteWithFormatter(NewCSVFormatter(&buf, false), rows, columns, FormatConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "id,name\n1,Alice\n2,Bob\n"
	if diff := cmp.Diff(want, buf.String()); diff != "" {
		t.Errorf("output mismatch (-want +got):\n%s", diff)
	}
}

func TestFormatCSV(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		columns     []string
		rows        []Row
		skipHeaders bool
		want        string
	}{
		{
			name:    "basic",
			columns: []string{"id", "name"},
			rows:    []Row{StringsToRow("1", "Alice"), StringsToRow("2", "Bob")},
			want:    "id,name\n1,Alice\n2,Bob\n",
		},
		{
			name:        "skip headers",
			columns:     []string{"id", "name"},
			rows:        []Row{StringsToRow("1", "Alice")},
			skipHeaders: true,
			want:        "1,Alice\n",
		},
		{
			name:    "special characters",
			columns: []string{"col"},
			rows:    []Row{StringsToRow("value with, comma"), StringsToRow("value with \"quotes\"")},
			want:    "col\n\"value with, comma\"\n\"value with \"\"quotes\"\"\"\n",
		},
		{
			name:    "empty rows",
			columns: []string{"id"},
			rows:    nil,
			want:    "id\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			config := FormatConfig{SkipColumnNames: tt.skipHeaders}
			err := formatCSV(&buf, tt.rows, tt.columns, config, 0)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, buf.String()); diff != "" {
				t.Errorf("output mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFormatTab(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		columns     []string
		rows        []Row
		skipHeaders bool
		want        string
	}{
		{
			name:    "basic",
			columns: []string{"id", "name"},
			rows:    []Row{StringsToRow("1", "Alice"), StringsToRow("2", "Bob")},
			want:    "id\tname\n1\tAlice\n2\tBob\n",
		},
		{
			name:        "skip headers",
			columns:     []string{"id", "name"},
			rows:        []Row{StringsToRow("1", "Alice")},
			skipHeaders: true,
			want:        "1\tAlice\n",
		},
		{
			name:    "empty rows",
			columns: []string{"id"},
			rows:    nil,
			want:    "id\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			config := FormatConfig{SkipColumnNames: tt.skipHeaders}
			err := formatTab(&buf, tt.rows, tt.columns, config, 0)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, buf.String()); diff != "" {
				t.Errorf("output mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFormatVertical(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		columns []string
		rows    []Row
		want    string
	}{
		{
			name:    "single row",
			columns: []string{"id", "name"},
			rows:    []Row{StringsToRow("1", "Alice")},
			want: "*************************** 1. row ***************************\n" +
				"  id: 1\n" +
				"name: Alice\n",
		},
		{
			name:    "multiple rows",
			columns: []string{"id", "name"},
			rows:    []Row{StringsToRow("1", "Alice"), StringsToRow("2", "Bob")},
			want: "*************************** 1. row ***************************\n" +
				"  id: 1\n" +
				"name: Alice\n" +
				"*************************** 2. row ***************************\n" +
				"  id: 2\n" +
				"name: Bob\n",
		},
		{
			name:    "empty rows",
			columns: []string{"id"},
			rows:    nil,
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			err := formatVertical(&buf, tt.rows, tt.columns, FormatConfig{}, 0)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, buf.String()); diff != "" {
				t.Errorf("output mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFormatHTML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		columns     []string
		rows        []Row
		skipHeaders bool
		want        string
	}{
		{
			name:    "basic",
			columns: []string{"id", "name"},
			rows:    []Row{StringsToRow("1", "Alice")},
			want:    "<TABLE BORDER='1'><TR><TH>id</TH><TH>name</TH></TR><TR><TD>1</TD><TD>Alice</TD></TR></TABLE>\n",
		},
		{
			name:        "skip headers",
			columns:     []string{"id"},
			rows:        []Row{StringsToRow("1")},
			skipHeaders: true,
			want:        "<TABLE BORDER='1'><TR><TD>1</TD></TR></TABLE>\n",
		},
		{
			name:    "html escaping",
			columns: []string{"col"},
			rows:    []Row{StringsToRow("<b>bold</b>")},
			want:    "<TABLE BORDER='1'><TR><TH>col</TH></TR><TR><TD>&lt;b&gt;bold&lt;/b&gt;</TD></TR></TABLE>\n",
		},
		{
			name:    "empty rows",
			columns: []string{"id"},
			rows:    nil,
			want:    "<TABLE BORDER='1'><TR><TH>id</TH></TR></TABLE>\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			config := FormatConfig{SkipColumnNames: tt.skipHeaders}
			err := formatHTML(&buf, tt.rows, tt.columns, config, 0)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, buf.String()); diff != "" {
				t.Errorf("output mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFormatXML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		columns     []string
		rows        []Row
		skipHeaders bool
		want        string
	}{
		{
			name:    "basic",
			columns: []string{"id", "name"},
			rows:    []Row{StringsToRow("1", "Alice")},
			want: `<?xml version='1.0'?>` + "\n" +
				`<resultset xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">` +
				`<header><field>id</field><field>name</field></header>` +
				`<row><field>1</field><field>Alice</field></row>` +
				"</resultset>\n",
		},
		{
			name:        "skip headers",
			columns:     []string{"id"},
			rows:        []Row{StringsToRow("1")},
			skipHeaders: true,
			want: `<?xml version='1.0'?>` + "\n" +
				`<resultset xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">` +
				`<row><field>1</field></row>` +
				"</resultset>\n",
		},
		{
			name:    "xml escaping",
			columns: []string{"col"},
			rows:    []Row{StringsToRow("<tag>&value</tag>")},
			want: `<?xml version='1.0'?>` + "\n" +
				`<resultset xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">` +
				`<header><field>col</field></header>` +
				`<row><field>&lt;tag&gt;&amp;value&lt;/tag&gt;</field></row>` +
				"</resultset>\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			config := FormatConfig{SkipColumnNames: tt.skipHeaders}
			err := formatXML(&buf, tt.rows, tt.columns, config, 0)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, buf.String()); diff != "" {
				t.Errorf("output mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestXMLEscape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "plain text", input: "hello", want: "hello"},
		{name: "ampersand", input: "a&b", want: "a&amp;b"},
		{name: "less than", input: "a<b", want: "a&lt;b"},
		{name: "greater than", input: "a>b", want: "a&gt;b"},
		{name: "quotes", input: `a"b`, want: "a&#34;b"},
		{name: "single quote", input: "a'b", want: "a&#39;b"},
		{name: "empty", input: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := xmlEscape(tt.input)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("xmlEscape mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestCSVFormatterLifecycle(t *testing.T) {
	t.Parallel()

	t.Run("write before init", func(t *testing.T) {
		t.Parallel()
		f := NewCSVFormatter(io.Discard, false)
		err := f.WriteRow(StringsToRow("1"))
		if err == nil {
			t.Error("expected error writing before init")
		}
	})

	t.Run("double init is noop", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		f := NewCSVFormatter(&buf, false)
		if err := f.InitFormat([]string{"id"}, FormatConfig{}, nil); err != nil {
			t.Fatalf("first init: %v", err)
		}
		if err := f.InitFormat([]string{"id"}, FormatConfig{}, nil); err != nil {
			t.Fatalf("second init: %v", err)
		}
		// Header should only appear once
		if err := f.FinishFormat(); err != nil {
			t.Fatalf("finish: %v", err)
		}
		if diff := cmp.Diff("id\n", buf.String()); diff != "" {
			t.Errorf("double init should not duplicate headers (-want +got):\n%s", diff)
		}
	})

	t.Run("empty column names", func(t *testing.T) {
		t.Parallel()
		f := NewCSVFormatter(io.Discard, false)
		if err := f.InitFormat(nil, FormatConfig{}, nil); err != nil {
			t.Fatalf("init with empty columns: %v", err)
		}
	})
}

func TestTabFormatterLifecycle(t *testing.T) {
	t.Parallel()

	t.Run("write before init", func(t *testing.T) {
		t.Parallel()
		f := NewTabFormatter(io.Discard, false)
		err := f.WriteRow(StringsToRow("1"))
		if err == nil {
			t.Error("expected error writing before init")
		}
	})
}

func TestVerticalFormatterLifecycle(t *testing.T) {
	t.Parallel()

	t.Run("write before init", func(t *testing.T) {
		t.Parallel()
		f := NewVerticalFormatter(io.Discard)
		err := f.WriteRow(StringsToRow("1"))
		if err == nil {
			t.Error("expected error writing before init")
		}
	})
}

func TestHTMLFormatterLifecycle(t *testing.T) {
	t.Parallel()

	t.Run("write before init", func(t *testing.T) {
		t.Parallel()
		f := NewHTMLFormatter(io.Discard, false)
		err := f.WriteRow(StringsToRow("1"))
		if err == nil {
			t.Error("expected error writing before init")
		}
	})
}

func TestXMLFormatterLifecycle(t *testing.T) {
	t.Parallel()

	t.Run("write before init", func(t *testing.T) {
		t.Parallel()
		f := NewXMLFormatter(io.Discard, false)
		err := f.WriteRow(StringsToRow("1"))
		if err == nil {
			t.Error("expected error writing before init")
		}
	})
}

func TestWriteTable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		columns     []string
		rows        []Row
		mode        Mode
		screenWidth int
		config      FormatConfig
		wantContain []string
	}{
		{
			name:        "basic table",
			columns:     []string{"id", "name"},
			rows:        []Row{StringsToRow("1", "Alice")},
			mode:        ModeTable,
			screenWidth: 80,
			wantContain: []string{"id", "name", "1", "Alice", "+"},
		},
		{
			name:        "empty rows no render",
			columns:     []string{"id"},
			rows:        nil,
			mode:        ModeTable,
			screenWidth: 80,
			wantContain: nil,
		},
		{
			name:        "table comment mode",
			columns:     []string{"id"},
			rows:        []Row{StringsToRow("1")},
			mode:        ModeTableComment,
			screenWidth: 80,
			wantContain: []string{"/*", "*/"},
		},
		{
			name:        "skip column names",
			columns:     []string{"id", "name"},
			rows:        []Row{StringsToRow("1", "Alice")},
			mode:        ModeTable,
			screenWidth: 80,
			config:      FormatConfig{SkipColumnNames: true},
			wantContain: []string{"1", "Alice"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			err := WriteTable(&buf, tt.rows, tt.columns, tt.config, tt.screenWidth, tt.mode)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			output := buf.String()
			for _, s := range tt.wantContain {
				if !strings.Contains(output, s) {
					t.Errorf("output missing %q, got:\n%s", s, output)
				}
			}
		})
	}
}

func TestWriteTableDetailComment(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	columns := []string{"id"}
	rows := []Row{StringsToRow("1")}

	err := WriteTable(&buf, rows, columns, FormatConfig{}, 80, ModeTableDetailComment)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	// TableDetailComment uses /* but NOT */
	if !strings.Contains(output, "/*") {
		t.Error("expected /* in detail comment output")
	}
	if strings.Contains(output, "*/") {
		t.Error("detail comment should NOT contain */")
	}
}

func TestWriteTableSanitizesCommentClosure(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	columns := []string{"data"}
	rows := []Row{StringsToRow("value with */ inside")}

	err := WriteTable(&buf, rows, columns, FormatConfig{}, 80, ModeTableComment)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	// The closing */ should be only the table border replacement, not from data
	if strings.Count(output, "*/") != 1 {
		t.Errorf("expected exactly 1 */ (table closure), got %d in:\n%s",
			strings.Count(output, "*/"), output)
	}
}

func TestNullCell(t *testing.T) {
	t.Parallel()

	t.Run("Format adds ANSI dim", func(t *testing.T) {
		t.Parallel()
		c := NullCell{Text: "NULL"}
		got := c.Format()
		want := "\033[2mNULL\033[0m"
		if got != want {
			t.Errorf("Format() = %q, want %q", got, want)
		}
	})

	t.Run("Format applies ANSI dim per line for wrapped text", func(t *testing.T) {
		t.Parallel()
		c := NullCell{Text: "NU\nLL"}
		got := c.Format()
		want := "\033[2mNU\033[0m\n\033[2mLL\033[0m"
		if got != want {
			t.Errorf("Format() = %q, want %q", got, want)
		}
	})

	t.Run("RawText returns plain text", func(t *testing.T) {
		t.Parallel()
		c := NullCell{Text: "NULL"}
		if got := c.RawText(); got != "NULL" {
			t.Errorf("RawText() = %q, want %q", got, "NULL")
		}
	})

	t.Run("WithText preserves NullCell type", func(t *testing.T) {
		t.Parallel()
		c := NullCell{Text: "NULL"}
		c2 := c.WithText("wrapped")
		if _, ok := c2.(NullCell); !ok {
			t.Errorf("WithText returned %T, want NullCell", c2)
		}
		if got := c2.RawText(); got != "wrapped" {
			t.Errorf("WithText().RawText() = %q, want %q", got, "wrapped")
		}
	})
}

func TestStyledCell(t *testing.T) {
	t.Parallel()

	t.Run("Format wraps text with SGR sequence", func(t *testing.T) {
		t.Parallel()
		c := StyledCell{Text: "hello", Style: "\033[32m"}
		got := c.Format()
		want := "\033[32mhello\033[0m"
		if got != want {
			t.Errorf("Format() = %q, want %q", got, want)
		}
	})

	t.Run("Format with bold style", func(t *testing.T) {
		t.Parallel()
		c := StyledCell{Text: "bold", Style: "\033[1m"}
		got := c.Format()
		want := "\033[1mbold\033[0m"
		if got != want {
			t.Errorf("Format() = %q, want %q", got, want)
		}
	})

	t.Run("RawText returns plain text", func(t *testing.T) {
		t.Parallel()
		c := StyledCell{Text: "hello", Style: "\033[32m"}
		if got := c.RawText(); got != "hello" {
			t.Errorf("RawText() = %q, want %q", got, "hello")
		}
	})

	t.Run("WithText preserves StyledCell type and Style", func(t *testing.T) {
		t.Parallel()
		c := StyledCell{Text: "original", Style: "\033[32m"}
		c2 := c.WithText("wrapped")
		sc, ok := c2.(StyledCell)
		if !ok {
			t.Fatalf("WithText returned %T, want StyledCell", c2)
		}
		if sc.Style != "\033[32m" {
			t.Errorf("WithText().Style = %q, want %q", sc.Style, "\033[32m")
		}
		if got := c2.RawText(); got != "wrapped" {
			t.Errorf("WithText().RawText() = %q, want %q", got, "wrapped")
		}
	})

	t.Run("empty Style degrades to plain text with reset", func(t *testing.T) {
		t.Parallel()
		c := StyledCell{Text: "hello", Style: ""}
		got := c.Format()
		// Empty style still appends ansiReset — not ideal but harmless.
		want := "hello\033[0m"
		if got != want {
			t.Errorf("Format() = %q, want %q", got, want)
		}
		// RawText is always clean
		if got := c.RawText(); got != "hello" {
			t.Errorf("RawText() = %q, want %q", got, "hello")
		}
	})
}

func TestStyledCellInNonTableFormats(t *testing.T) {
	t.Parallel()

	// Verify all non-table formatters use RawText() — no ANSI codes in output
	rows := []Row{
		{PlainCell{Text: "1"}, StyledCell{Text: "styled", Style: "\033[32m"}},
	}
	columns := []string{"id", "value"}

	tests := []struct {
		name     string
		formatFn func(io.Writer, []Row, []string, FormatConfig, int) error
	}{
		{"csv", formatCSV},
		{"tab", formatTab},
		{"vertical", formatVertical},
		{"html", formatHTML},
		{"xml", formatXML},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			err := tt.formatFn(&buf, rows, columns, FormatConfig{}, 0)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			output := buf.String()
			if strings.Contains(output, "\033") {
				t.Errorf("%s output should not contain ANSI codes, got:\n%s", tt.name, output)
			}
			if !strings.Contains(output, "styled") {
				t.Errorf("%s output should contain text 'styled', got:\n%s", tt.name, output)
			}
		})
	}
}

func TestStyledCellInTable(t *testing.T) {
	t.Parallel()

	rows := []Row{
		{PlainCell{Text: "1"}, StyledCell{Text: "hello", Style: "\033[32m"}},
		{PlainCell{Text: "2"}, StyledCell{Text: "world", Style: "\033[32m"}},
	}
	columns := []string{"id", "value"}

	t.Run("styled output includes ANSI codes", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		err := WriteTable(&buf, rows, columns, FormatConfig{Styled: true}, 80, ModeTable)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		output := buf.String()
		if !strings.Contains(output, "\033[32m") {
			t.Error("expected ANSI green code in styled output")
		}
		verifyTableAlignment(t, output)
	})

	t.Run("unstyled output has no ANSI codes", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		err := WriteTable(&buf, rows, columns, FormatConfig{Styled: false}, 80, ModeTable)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		output := buf.String()
		if strings.Contains(output, "\033[") {
			t.Error("expected no ANSI codes in unstyled output")
		}
		if !strings.Contains(output, "hello") {
			t.Error("expected plain text in output")
		}
		verifyTableAlignment(t, output)
	})
}

func TestStyledCellWrappedStyled(t *testing.T) {
	t.Parallel()

	// Use a narrow screen to force StyledCell text to wrap.
	// This verifies SGR carry-over: each wrapped line should have styling.
	rows := []Row{
		{PlainCell{Text: "1"}, StyledCell{Text: "long styled text", Style: "\033[32m"}},
	}
	columns := []string{"id", "data"}

	var buf bytes.Buffer
	err := WriteTable(&buf, rows, columns, FormatConfig{Styled: true}, 20, ModeTable)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	t.Logf("Table output:\n%s", output)

	// Each data line with styled text should have both open and close SGR
	for _, line := range strings.Split(output, "\n") {
		if !strings.HasPrefix(line, "|") {
			continue
		}
		if strings.Contains(line, "\033[32m") {
			if !strings.Contains(line, "\033[0m") {
				t.Errorf("line has style without reset: %q", line)
			}
		}
	}

	verifyTableAlignment(t, output)
}

func TestNullCellWrappedStyled(t *testing.T) {
	t.Parallel()

	// Use a narrow screen to force NullCell text to wrap.
	// This verifies SGR carry-over: each wrapped line should have dim styling.
	rows := []Row{
		{PlainCell{Text: "1"}, NullCell{Text: "NULL value"}},
	}
	columns := []string{"id", "data"}

	var buf bytes.Buffer
	// screenWidth=20 forces "NULL value" to wrap
	err := WriteTable(&buf, rows, columns, FormatConfig{Styled: true}, 20, ModeTable)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	t.Logf("Table output:\n%s", output)

	// Each visible line containing NULL/value text should have ANSI dim codes
	for _, line := range strings.Split(output, "\n") {
		if !strings.HasPrefix(line, "|") {
			continue
		}
		// Data lines containing dim text should have both open and close
		if strings.Contains(line, "\033[2m") {
			if !strings.Contains(line, "\033[0m") {
				t.Errorf("line has dim without reset: %q", line)
			}
		}
	}

	verifyTableAlignment(t, output)
}

func TestNullCellInTable(t *testing.T) {
	t.Parallel()

	// Mix NullCell and PlainCell in the same table
	rows := []Row{
		{PlainCell{Text: "1"}, PlainCell{Text: "Alice"}, NullCell{Text: "NULL"}},
		{PlainCell{Text: "2"}, NullCell{Text: "NULL"}, PlainCell{Text: "active"}},
		{PlainCell{Text: "3"}, PlainCell{Text: "Charlie"}, PlainCell{Text: "inactive"}},
	}
	columns := []string{"id", "name", "status"}

	t.Run("styled", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		err := WriteTable(&buf, rows, columns, FormatConfig{Styled: true}, 80, ModeTable)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		output := buf.String()
		t.Logf("Table output:\n%s", output)

		// Verify ANSI codes are present
		if !strings.Contains(output, "\033[2m") {
			t.Error("expected ANSI dim code in output")
		}
		if !strings.Contains(output, "\033[0m") {
			t.Error("expected ANSI reset code in output")
		}

		verifyTableAlignment(t, output)
	})

	t.Run("unstyled", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		err := WriteTable(&buf, rows, columns, FormatConfig{Styled: false}, 80, ModeTable)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		output := buf.String()
		t.Logf("Table output:\n%s", output)

		// Verify NO ANSI codes in output
		if strings.Contains(output, "\033[") {
			t.Error("expected no ANSI codes in unstyled output")
		}
		// Verify NULL text is still present
		if !strings.Contains(output, "NULL") {
			t.Error("expected NULL text in output")
		}

		verifyTableAlignment(t, output)
	})
}

// verifyTableAlignment checks that all data lines in a table have consistent column separators.
func verifyTableAlignment(t *testing.T, output string) {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var dataLines []string
	for _, line := range lines {
		if strings.HasPrefix(line, "|") {
			dataLines = append(dataLines, line)
		}
	}
	if len(dataLines) < 2 {
		t.Fatalf("expected at least 2 data lines (header + rows), got %d", len(dataLines))
	}

	expectedPipes := strings.Count(dataLines[0], "|")
	for i, line := range dataLines[1:] {
		if got := strings.Count(line, "|"); got != expectedPipes {
			t.Errorf("line %d has %d pipes, want %d (alignment broken)\nline: %s", i+1, got, expectedPipes, line)
		}
	}
}

func TestNullCellInNonTableFormats(t *testing.T) {
	t.Parallel()

	// Verify all non-table formatters use RawText() — no ANSI codes in output
	rows := []Row{
		{PlainCell{Text: "1"}, NullCell{Text: "NULL"}},
	}
	columns := []string{"id", "value"}

	tests := []struct {
		name      string
		formatFn  func(io.Writer, []Row, []string, FormatConfig, int) error
		wantNoESC bool
	}{
		{"tab", formatTab, true},
		{"vertical", formatVertical, true},
		{"html", formatHTML, true},
		{"xml", formatXML, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			err := tt.formatFn(&buf, rows, columns, FormatConfig{}, 0)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			output := buf.String()
			if strings.Contains(output, "\033") {
				t.Errorf("%s output should not contain ANSI codes, got:\n%s", tt.name, output)
			}
			if !strings.Contains(output, "NULL") {
				t.Errorf("%s output should contain NULL text, got:\n%s", tt.name, output)
			}
		})
	}
}

func TestNullCellInCSV(t *testing.T) {
	t.Parallel()

	// Verify that CSV uses RawText() — no ANSI codes in output
	rows := []Row{
		{PlainCell{Text: "1"}, NullCell{Text: "NULL"}},
	}
	columns := []string{"id", "value"}

	var buf bytes.Buffer
	err := formatCSV(&buf, rows, columns, FormatConfig{}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if strings.Contains(output, "\033") {
		t.Errorf("CSV output should not contain ANSI codes, got:\n%s", output)
	}
	want := "id,value\n1,NULL\n"
	if output != want {
		t.Errorf("got:\n%s\nwant:\n%s", output, want)
	}
}
