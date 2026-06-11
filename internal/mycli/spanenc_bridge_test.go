// Copyright 2026 apstndb

package mycli

import (
	"bytes"
	"io"
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanenc"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanner-mycli/internal/mycli/format"
	"github.com/apstndb/spanner-mycli/internal/mycli/streamio"
	"github.com/google/go-cmp/cmp"
)

func TestResultFromStructRows_helpVariablesShape(t *testing.T) {
	t.Parallel()

	items := []helpVariableRow{
		{Name: "CLI_ECHO_INPUT", Operations: "read,write", Description: "echo SQL input"},
		{Name: "COMMIT_RESPONSE", Operations: "read", Description: "virtual commit stats"},
	}
	result, err := resultFromStructRows(helpVariablesRowEncoder, items, nil)
	if err != nil {
		t.Fatalf("resultFromStructRows: %v", err)
	}

	if got := result.TableHeader.Render(false); !cmp.Equal(got, []string{"name", "operations", "desc"}) {
		t.Fatalf("headers: %v", got)
	}
	wantRows := []Row{
		toRow("CLI_ECHO_INPUT", "read,write", "echo SQL input"),
		toRow("COMMIT_RESPONSE", "read", "virtual commit stats"),
	}
	if diff := cmp.Diff(wantRows, result.Rows); diff != "" {
		t.Fatalf("rows mismatch (-want +got):\n%s", diff)
	}
	if result.AffectedRows != 2 {
		t.Fatalf("AffectedRows: got %d, want 2", result.AffectedRows)
	}
}

func TestResultFromStructRows_showVariablesShape(t *testing.T) {
	t.Parallel()

	items := []nameValueRow{
		{Name: "CLI_DATABASE", Value: "my-db"},
		{Name: "CLI_FORMAT", Value: "TABLE"},
	}
	result, err := resultFromStructRows(nameValueRowEncoder, items, nil)
	if err != nil {
		t.Fatalf("resultFromStructRows: %v", err)
	}

	if result.AffectedRows != 2 {
		t.Fatalf("AffectedRows: got %d, want 2", result.AffectedRows)
	}
	if got := result.TableHeader.Render(false); !cmp.Equal(got, []string{"name", "value"}) {
		t.Fatalf("headers: %v", got)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("rows: got %d, want 2", len(result.Rows))
	}
}

// TestResultFromStructRows_typedHeader verifies that virtual result sets carry
// row-type metadata like server result sets: verbose header rendering includes
// column types via typesTableHeader.
func TestResultFromStructRows_typedHeader(t *testing.T) {
	t.Parallel()

	result, err := resultFromStructRows(nameValueRowEncoder, []nameValueRow{{Name: "a", Value: "b"}}, nil)
	if err != nil {
		t.Fatalf("resultFromStructRows: %v", err)
	}

	fields, ok := result.TableHeader.structFields()
	if !ok {
		t.Fatal("TableHeader should carry struct fields (typesTableHeader)")
	}
	if len(fields) != 2 || fields[0].GetType().GetCode() != sppb.TypeCode_STRING {
		t.Fatalf("unexpected fields: %v", fields)
	}
	if got := result.TableHeader.Render(true); !cmp.Equal(got, []string{"name\nSTRING", "value\nSTRING"}) {
		t.Fatalf("verbose headers: %v", got)
	}
}

// typedVirtualRow exercises non-STRING columns to show that virtual result
// sets format typed values exactly like server result sets per CLI_FORMAT.
type typedVirtualRow struct {
	Name    string `spanner:"name"`
	Count   int64  `spanner:"count"`
	Enabled bool   `spanner:"enabled"`
	Note    *string
}

// TestResultFromStructRows_jsonValueMode verifies that JSONL mode produces
// RawJSONCell cells whose text is native JSON (numbers, booleans, null), the
// same behavior the server query path gets from withRawJSONMarker.
func TestResultFromStructRows_jsonValueMode(t *testing.T) {
	t.Parallel()

	enc, err := spanenc.NewRowEncoder[typedVirtualRow]()
	if err != nil {
		t.Fatalf("NewRowEncoder: %v", err)
	}

	sysVars := newSystemVariablesWithDefaults()
	sysVars.Display.CLIFormat = enums.DisplayModeJSONL

	result, err := resultFromStructRows(enc, []typedVirtualRow{{Name: "x", Count: 42, Enabled: true, Note: nil}}, &sysVars)
	if err != nil {
		t.Fatalf("resultFromStructRows: %v", err)
	}

	row := result.Rows[0]
	wantTexts := []string{`"x"`, "42", "true", "null"}
	for i, want := range wantTexts {
		if !format.IsRawJSON(row[i]) {
			t.Errorf("cell %d should be RawJSONCell, got %T", i, row[i])
		}
		if got := row[i].RawText(); got != want {
			t.Errorf("cell %d: got %q, want %q", i, got, want)
		}
	}
}

// TestResultFromStructRows_displayMode verifies display formatting and typed
// NULL handling (NULL text, NoWrapCell) matching the server row pipeline.
func TestResultFromStructRows_displayMode(t *testing.T) {
	t.Parallel()

	enc, err := spanenc.NewRowEncoder[typedVirtualRow]()
	if err != nil {
		t.Fatalf("NewRowEncoder: %v", err)
	}

	sysVars := newSystemVariablesWithDefaults()
	result, err := resultFromStructRows(enc, []typedVirtualRow{{Name: "x", Count: 42, Enabled: true, Note: nil}}, &sysVars)
	if err != nil {
		t.Fatalf("resultFromStructRows: %v", err)
	}

	row := result.Rows[0]
	wantTexts := []string{"x", "42", "true", "NULL"}
	for i, want := range wantTexts {
		if got := row[i].RawText(); got != want {
			t.Errorf("cell %d: got %q, want %q", i, got, want)
		}
	}
	if _, ok := row[3].(format.NoWrapCell); !ok {
		t.Errorf("NULL cell should be NoWrapCell, got %T", row[3])
	}
}

// TestExecuteStructRows_streaming verifies that client-side virtual result
// sets stream through the spanvalue writers (writer.WriteRowSeq) for formats
// that have one, and fall back to the buffered cell pipeline otherwise.
func TestExecuteStructRows_streaming(t *testing.T) {
	t.Parallel()

	items := []nameValueRow{{Name: "A", Value: "1"}, {Name: "B", Value: "2"}}

	tests := []struct {
		desc       string
		format     enums.DisplayMode
		wantStream bool
		wantOutput string
	}{
		{
			desc:       "CSV streams via spanvalue writer",
			format:     enums.DisplayModeCSV,
			wantStream: true,
			wantOutput: "name,value\nA,1\nB,2\n",
		},
		{
			desc:       "JSONL streams via spanvalue writer",
			format:     enums.DisplayModeJSONL,
			wantStream: true,
			wantOutput: "{\"name\":\"A\",\"value\":\"1\"}\n{\"name\":\"B\",\"value\":\"2\"}\n",
		},
		{
			desc:       "TABLE falls back to buffered cells",
			format:     enums.DisplayModeTable,
			wantStream: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			t.Parallel()

			sysVars := newSystemVariablesWithDefaults()
			sysVars.Display.CLIFormat = tt.format
			var buf bytes.Buffer
			sysVars.StreamManager = streamio.NewStreamManager(io.NopCloser(bytes.NewReader(nil)), &buf, &buf)

			result, err := executeStructRows(nameValueRowEncoder, items, &sysVars)
			if err != nil {
				t.Fatalf("executeStructRows: %v", err)
			}

			if result.Streamed != tt.wantStream {
				t.Fatalf("Streamed = %v, want %v", result.Streamed, tt.wantStream)
			}
			if result.AffectedRows != 2 {
				t.Errorf("AffectedRows = %d, want 2", result.AffectedRows)
			}
			if tt.wantStream {
				if len(result.Rows) != 0 {
					t.Errorf("Rows = %v, want none for streamed result", result.Rows)
				}
				if got := buf.String(); got != tt.wantOutput {
					t.Errorf("output = %q, want %q", got, tt.wantOutput)
				}
			} else {
				if len(result.Rows) != 2 {
					t.Errorf("Rows = %v, want 2 buffered rows", result.Rows)
				}
				if buf.Len() != 0 {
					t.Errorf("output = %q, want empty for buffered result", buf.String())
				}
			}
		})
	}
}
