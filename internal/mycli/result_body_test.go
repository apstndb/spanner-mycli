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
	"bytes"
	"strings"
	"testing"
	"text/template"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spancodec"
	"github.com/apstndb/spanner-mycli/enums"
)

func (r *Result) presentationRows() []Row {
	if r == nil {
		return nil
	}
	rows, ok := r.Body.PresentationRows()
	if !ok {
		return nil
	}
	return rows
}

func (r *Result) typedPayload() *TypedRows {
	if r == nil {
		return nil
	}
	typed, ok := r.Body.Typed()
	if !ok {
		return nil
	}
	return typed
}

func (r *Result) preparedOutput() []byte {
	if r == nil {
		return nil
	}
	output, ok := r.Body.PreparedBytes()
	if !ok {
		return nil
	}
	return output
}

func (r *Result) alreadyDelivered() bool {
	return r != nil && r.Body.AlreadyDelivered()
}

func deliveredBodyIf(delivered bool) ResultBody {
	if delivered {
		return DeliveredBody()
	}
	return ResultBody{}
}

func printResultForBodyTest(t *testing.T, result *Result, mode enums.DisplayMode, suppress bool) string {
	t.Helper()
	sv := &systemVariables{
		Display: DisplayVars{
			CLIFormat:           mode,
			SuppressResultLines: suppress,
			OutputTemplate:      template.Must(template.New("empty").Parse("")),
		},
	}
	var buf bytes.Buffer
	if err := printResult(sv, 80, &buf, result, true); err != nil {
		t.Fatalf("printResult: %v", err)
	}
	return buf.String()
}

func TestPrintResultBodyKinds(t *testing.T) {
	t.Parallel()

	header := toTableHeader("n")
	emptyAppendix := ResultAppendix{Title: "Appendix", Lines: []string{"note"}}

	t.Run("no-body is distinct from delivered and prints only the summary", func(t *testing.T) {
		t.Parallel()
		got := printResultForBodyTest(t, &Result{}, enums.DisplayModeTable, false)
		if got != "Query OK\n" {
			t.Fatalf("no-body output = %q, want Query OK summary only", got)
		}
		if !(&Result{}).Body.IsNone() || (&Result{}).Body.AlreadyDelivered() {
			t.Fatal("zero Result must be no-body, not delivered")
		}
	})

	t.Run("zero-row presentation headers still render a table", func(t *testing.T) {
		t.Parallel()
		result := &Result{TableHeader: header, Body: PresentationBody(nil)}
		// CSV writes column names for a zero-row presentation table. Buffered
		// TABLE still omits a header-only ASCII frame unless CLI_VERBOSE, which
		// is the existing format rule, not a body-kind change.
		got, err := runPrintTableData(t, enums.DisplayModeCSV, false, result)
		if err != nil {
			t.Fatal(err)
		}
		if got != "n\n" {
			t.Fatalf("empty presentation table = %q, want CSV header", got)
		}
		rows, ok := result.Body.PresentationRows()
		if !ok {
			t.Fatal("empty presentation must still be a presentation body")
		}
		if len(rows) != 0 {
			t.Fatalf("presentation rows = %v, want empty", rows)
		}
	})

	t.Run("zero-row typed data keeps metadata and renders headers", func(t *testing.T) {
		t.Parallel()
		enc := spancodec.MustNewRowEncoder[thenReturnRow]()
		md, err := enc.ResultSetMetadata()
		if err != nil {
			t.Fatal(err)
		}
		result := &Result{
			TableHeader: toTableHeader(md.GetRowType().GetFields()),
			Body: TypedBody(&TypedRows{
				Metadata: md,
				Rows:     []*spanner.Row{},
			}),
		}
		got, err := runPrintTableData(t, enums.DisplayModeCSV, false, result)
		if err != nil {
			t.Fatal(err)
		}
		if got != "n\n" {
			t.Fatalf("zero-row typed output = %q, want typed column header", got)
		}
		typed, ok := result.Body.Typed()
		if !ok || typed == nil || typed.Metadata == nil {
			t.Fatal("typed body must retain metadata with zero rows")
		}
		if len(typed.Rows) != 0 {
			t.Fatalf("typed rows = %v, want empty", typed.Rows)
		}
		if _, ok := result.Body.PresentationRows(); ok {
			t.Fatal("typed body must not also be a presentation table")
		}
	})

	t.Run("empty prepared bytes bypass table rendering", func(t *testing.T) {
		t.Parallel()
		result := &Result{
			TableHeader: header,
			Body:        PreparedBody(nil),
		}
		got := printResultForBodyTest(t, result, enums.DisplayModeTable, true)
		if got != "" {
			t.Fatalf("empty prepared body = %q, want no table", got)
		}
		output, ok := result.Body.PreparedBytes()
		if !ok {
			t.Fatal("zero-length prepared bytes must still be a prepared body")
		}
		if len(output) != 0 {
			t.Fatalf("prepared bytes = %q, want empty", output)
		}
	})

	t.Run("delivered output skips the body and still prints appendix and summary", func(t *testing.T) {
		t.Parallel()
		result := &Result{
			TableHeader: header,
			Body:        DeliveredBody(),
			Appendices:  []ResultAppendix{emptyAppendix},
		}
		got := printResultForBodyTest(t, result, enums.DisplayModeTable, false)
		if strings.Contains(got, "+") || strings.Contains(got, "| n") {
			t.Fatalf("delivered output reprinted a table: %q", got)
		}
		if !strings.Contains(got, "Appendix\n note\n") {
			t.Fatalf("delivered output missing appendix: %q", got)
		}
		if !strings.Contains(got, "Empty set") && !strings.Contains(got, "Query OK") && !strings.Contains(got, "rows in set") {
			t.Fatalf("delivered output missing summary: %q", got)
		}
		if result.Body.IsNone() {
			t.Fatal("delivered must not be no-body")
		}
	})
}

func TestPrintResultBodyExportEligibility(t *testing.T) {
	t.Parallel()

	header := toTableHeader("n")
	enc := spancodec.MustNewRowEncoder[thenReturnRow]()
	md, err := enc.ResultSetMetadata()
	if err != nil {
		t.Fatal(err)
	}
	var rawRows []*spanner.Row
	for row, err := range enc.Rows([]thenReturnRow{{N: 1}}) {
		if err != nil {
			t.Fatal(err)
		}
		rawRows = append(rawRows, row)
	}

	t.Run("presentation table falls back from SQL export", func(t *testing.T) {
		t.Parallel()
		result := &Result{
			TableHeader:      header,
			Body:             PresentationBody([]Row{toRow("1")}),
			SQLExportAllowed: false,
		}
		got, err := runPrintTableData(t, enums.DisplayModeSQLInsert, false, result)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got, "+") || strings.Contains(got, "INSERT") {
			t.Fatalf("presentation SQL fallback = %q, want table", got)
		}
	})

	t.Run("typed header metadata exports CSV from values", func(t *testing.T) {
		t.Parallel()
		result := &Result{
			TableHeader: toTableHeader(md.GetRowType().GetFields()),
			Body: TypedBody(&TypedRows{
				Metadata:         md,
				Rows:             rawRows,
				SQLExportAllowed: true,
			}),
		}
		got, err := runPrintTableData(t, enums.DisplayModeCSV, false, result)
		if err != nil {
			t.Fatal(err)
		}
		if got != "n\n1\n" {
			t.Fatalf("typed CSV = %q, want header and INT64 cell", got)
		}
	})

	t.Run("typed SQL export uses literals when allowed", func(t *testing.T) {
		t.Parallel()
		result := &Result{
			TableHeader:           toTableHeader(md.GetRowType().GetFields()),
			SQLTableNameForExport: "Items",
			Body: TypedBody(&TypedRows{
				Metadata:         md,
				Rows:             rawRows,
				SQLExportAllowed: true,
			}),
		}
		got, err := runPrintTableData(t, enums.DisplayModeSQLInsert, false, result)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got, "INSERT INTO `Items`") || !strings.Contains(got, "1") {
			t.Fatalf("typed SQL = %q, want INSERT of INT64", got)
		}
	})

	t.Run("affected-row text is independent of body kind", func(t *testing.T) {
		t.Parallel()
		dml := &Result{IsExecutedDML: true, AffectedRows: 3}
		got := printResultForBodyTest(t, dml, enums.DisplayModeTable, false)
		if !strings.Contains(got, "3 rows affected") {
			t.Fatalf("no-body DML summary = %q, want affected-row text", got)
		}
	})
}

func TestPrintResultEmptyPreparedBytes(t *testing.T) {
	t.Parallel()
	md := &sppb.ResultSetMetadata{RowType: &sppb.StructType{Fields: []*sppb.StructType_Field{
		{Name: "n", Type: &sppb.Type{Code: sppb.TypeCode_INT64}},
	}}}
	result := &Result{
		TableHeader: toTableHeader(md.GetRowType().GetFields()),
		Body:        PreparedBody([]byte{}),
	}
	got := printResultForBodyTest(t, result, enums.DisplayModeJSONL, true)
	if got != "" {
		t.Fatalf("empty prepared JSONL = %q, want no replayed typed/presentation table", got)
	}
}
