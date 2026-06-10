// Copyright 2026 apstndb

package mycli

import (
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanenc"
	"github.com/apstndb/spanner-mycli/internal/mycli/decoder"
	"github.com/apstndb/spanvalue"
	"github.com/google/go-cmp/cmp"
)

func TestSpanencAPIOverview(t *testing.T) {
	t.Parallel()

	// ValueOf: Go value → GenericColumnValue (encodeValue parity).
	gcv, err := spanenc.ValueOf(int64(42))
	if err != nil {
		t.Fatalf("ValueOf: %v", err)
	}
	text, err := spanvalue.SpannerCLICompatibleFormatConfig().FormatToplevelColumn(gcv)
	if err != nil {
		t.Fatalf("FormatToplevelColumn: %v", err)
	}
	if text != "42" {
		t.Fatalf("got %q, want 42", text)
	}

	// TypeFor / StructColumns / RowTypeFor: schema from Go types.
	type singerRow struct {
		SingerID  int64  `spanner:"SingerId"`
		FirstName string `spanner:"FirstName"`
		Internal  string `spanner:"-"`
	}
	cols, err := spanenc.StructColumns[singerRow]()
	if err != nil {
		t.Fatalf("StructColumns: %v", err)
	}
	if diff := cmp.Diff([]string{"SingerId", "FirstName"}, cols); diff != "" {
		t.Fatalf("StructColumns mismatch (-got +want):\n%s", diff)
	}

	rowType, err := spanenc.RowTypeFor[singerRow]()
	if err != nil {
		t.Fatalf("RowTypeFor: %v", err)
	}
	if len(rowType.GetFields()) != 2 {
		t.Fatalf("RowTypeFor fields: got %d, want 2", len(rowType.GetFields()))
	}

	metadata, err := spanenc.ResultSetMetadataFor[singerRow]()
	if err != nil {
		t.Fatalf("ResultSetMetadataFor: %v", err)
	}
	if metadata.GetRowType() == nil {
		t.Fatal("ResultSetMetadataFor: missing row type")
	}

	// ParamsMap: struct → Statement.Params (read-only fields included).
	type paramStruct struct {
		ID   int64  `spanner:"id"`
		Name string `spanner:"name"`
		Note string `spanner:"note;readonly"`
	}
	params, err := spanenc.ParamsMap(paramStruct{ID: 1, Name: "alice", Note: "hidden"})
	if err != nil {
		t.Fatalf("ParamsMap: %v", err)
	}
	if params["id"] != int64(1) || params["name"] != "alice" || params["note"] != "hidden" {
		t.Fatalf("ParamsMap: %+v", params)
	}

	// MutationMap: write-shaped listing excludes read-only fields.
	mutMap, err := spanenc.MutationMap(paramStruct{ID: 1, Name: "alice", Note: "hidden"})
	if err != nil {
		t.Fatalf("MutationMap: %v", err)
	}
	if _, ok := mutMap["note"]; ok {
		t.Fatalf("MutationMap should exclude read-only field, got %+v", mutMap)
	}

	// ValuesFromSlice / ArrayValueFromSlice: homogeneous slices.
	elemType, values, err := spanenc.ValuesFromSlice([]int64{1, 2, 3})
	if err != nil {
		t.Fatalf("ValuesFromSlice: %v", err)
	}
	if elemType.GetCode() != sppb.TypeCode_INT64 {
		t.Fatalf("element type: %v", elemType)
	}
	if len(values) != 3 {
		t.Fatalf("values len: got %d, want 3", len(values))
	}
	arrGCV, err := spanenc.ArrayValueFromSlice([]string{"a", "b"})
	if err != nil {
		t.Fatalf("ArrayValueFromSlice: %v", err)
	}
	if arrGCV.Type.GetCode() != sppb.TypeCode_ARRAY {
		t.Fatalf("array type: %v", arrGCV.Type)
	}
}

func TestResultFromRowEncoder_helpVariablesShape(t *testing.T) {
	t.Parallel()

	enc, err := spanenc.NewRowEncoder[helpVariableRow]()
	if err != nil {
		t.Fatalf("NewRowEncoder: %v", err)
	}

	fc, err := decoder.FormatConfigWithProto(nil, false)
	if err != nil {
		t.Fatalf("FormatConfigWithProto: %v", err)
	}

	items := []helpVariableRow{
		{Name: "CLI_ECHO_INPUT", Operations: "read,write", Description: "echo SQL input"},
		{Name: "COMMIT_RESPONSE", Operations: "read", Description: "virtual commit stats"},
	}
	result, err := resultFromRowEncoder(enc, items, fc, nil, "")
	if err != nil {
		t.Fatalf("resultFromRowEncoder: %v", err)
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

func TestResultFromRowEncoder_showVariablesShape(t *testing.T) {
	t.Parallel()

	enc, err := spanenc.NewRowEncoder[nameValueRow]()
	if err != nil {
		t.Fatalf("NewRowEncoder: %v", err)
	}

	fc, err := decoder.FormatConfigWithProto(nil, false)
	if err != nil {
		t.Fatalf("FormatConfigWithProto: %v", err)
	}

	items := []nameValueRow{
		{Name: "CLI_DATABASE", Value: "my-db"},
		{Name: "CLI_FORMAT", Value: "TABLE"},
	}
	result, err := resultFromRowEncoder(enc, items, fc, nil, "")
	if err != nil {
		t.Fatalf("resultFromRowEncoder: %v", err)
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
