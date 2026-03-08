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
	t.Parallel()

	// Register a custom mode
	testMode := Mode("TEST_CUSTOM")
	RegisterFormatFunc(func(mode Mode) (FormatFunc, error) {
		return func(out io.Writer, rows []Row, columnNames []string, config FormatConfig, screenWidth int) error {
			return nil
		}, nil
	}, testMode)

	fn, err := NewFormatter(testMode)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fn == nil {
		t.Error("expected non-nil FormatFunc from registered mode")
	}
}

func TestNewStreamingFormatter_RegisteredMode(t *testing.T) {
	t.Parallel()

	// Register a custom streaming mode
	testMode := Mode("TEST_STREAMING_CUSTOM")
	RegisterStreamingFormatter(func(mode Mode, out io.Writer, config FormatConfig) (StreamingFormatter, error) {
		return NewCSVFormatter(out, false), nil
	}, testMode)

	sf, err := NewStreamingFormatter(testMode, io.Discard, FormatConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sf == nil {
		t.Error("expected non-nil StreamingFormatter from registered mode")
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
	rows := []Row{{"1", "Alice"}, {"2", "Bob"}}

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
			rows:    []Row{{"1", "Alice"}, {"2", "Bob"}},
			want:    "id,name\n1,Alice\n2,Bob\n",
		},
		{
			name:        "skip headers",
			columns:     []string{"id", "name"},
			rows:        []Row{{"1", "Alice"}},
			skipHeaders: true,
			want:        "1,Alice\n",
		},
		{
			name:    "special characters",
			columns: []string{"col"},
			rows:    []Row{{"value with, comma"}, {"value with \"quotes\""}},
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
			rows:    []Row{{"1", "Alice"}, {"2", "Bob"}},
			want:    "id\tname\n1\tAlice\n2\tBob\n",
		},
		{
			name:        "skip headers",
			columns:     []string{"id", "name"},
			rows:        []Row{{"1", "Alice"}},
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
			rows:    []Row{{"1", "Alice"}},
			want: "*************************** 1. row ***************************\n" +
				"  id: 1\n" +
				"name: Alice\n",
		},
		{
			name:    "multiple rows",
			columns: []string{"id", "name"},
			rows:    []Row{{"1", "Alice"}, {"2", "Bob"}},
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
			rows:    []Row{{"1", "Alice"}},
			want:    "<TABLE BORDER='1'><TR><TH>id</TH><TH>name</TH></TR><TR><TD>1</TD><TD>Alice</TD></TR></TABLE>\n",
		},
		{
			name:        "skip headers",
			columns:     []string{"id"},
			rows:        []Row{{"1"}},
			skipHeaders: true,
			want:        "<TABLE BORDER='1'><TR><TD>1</TD></TR></TABLE>\n",
		},
		{
			name:    "html escaping",
			columns: []string{"col"},
			rows:    []Row{{"<b>bold</b>"}},
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
			rows:    []Row{{"1", "Alice"}},
			want: `<?xml version='1.0'?>` + "\n" +
				`<resultset xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">` +
				`<header><field>id</field><field>name</field></header>` +
				`<row><field>1</field><field>Alice</field></row>` +
				"</resultset>\n",
		},
		{
			name:        "skip headers",
			columns:     []string{"id"},
			rows:        []Row{{"1"}},
			skipHeaders: true,
			want: `<?xml version='1.0'?>` + "\n" +
				`<resultset xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">` +
				`<row><field>1</field></row>` +
				"</resultset>\n",
		},
		{
			name:    "xml escaping",
			columns: []string{"col"},
			rows:    []Row{{"<tag>&value</tag>"}},
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
		err := f.WriteRow(Row{"1"})
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
		err := f.WriteRow(Row{"1"})
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
		err := f.WriteRow(Row{"1"})
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
		err := f.WriteRow(Row{"1"})
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
		err := f.WriteRow(Row{"1"})
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
			rows:        []Row{{"1", "Alice"}},
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
			rows:        []Row{{"1"}},
			mode:        ModeTableComment,
			screenWidth: 80,
			wantContain: []string{"/*", "*/"},
		},
		{
			name:        "skip column names",
			columns:     []string{"id", "name"},
			rows:        []Row{{"1", "Alice"}},
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
	rows := []Row{{"1"}}

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
	rows := []Row{{"value with */ inside"}}

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
