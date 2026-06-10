// Copyright 2026 apstndb

package mycli

import (
	"testing"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanenc"
	"github.com/apstndb/spanner-mycli/internal/mycli/decoder"
	"github.com/apstndb/spanner-mycli/internal/mycli/format"
	"github.com/apstndb/spantype/typector"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestGcvTextToCell_nilGuards(t *testing.T) {
	t.Parallel()

	nullCell := format.NoWrapCell{Cell: format.StyledCell{Text: "NULL", Style: ""}}
	plainCell := format.PlainCell{Text: "hello"}

	if diff := cmp.Diff(nullCell, gcvTextToCell("NULL", spanner.GenericColumnValue{}, nil, "")); diff != "" {
		t.Fatalf("nil Value (-got +want):\n%s", diff)
	}
	if diff := cmp.Diff(plainCell, gcvTextToCell("hello", spanner.GenericColumnValue{
		Value: structpb.NewStringValue("hello"),
		Type:  nil,
	}, nil, "")); diff != "" {
		t.Fatalf("nil Type (-got +want):\n%s", diff)
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

func TestGcvTextToCell_typeStyle(t *testing.T) {
	t.Parallel()

	gcv := spanner.GenericColumnValue{
		Value: structpb.NewStringValue("42"),
		Type:  typector.CodeToSimpleType(sppb.TypeCode_INT64),
	}
	cell := gcvTextToCell("42", gcv, map[sppb.TypeCode]string{sppb.TypeCode_INT64: "\x1b[31m"}, "")
	styled, ok := cell.(format.StyledCell)
	if !ok {
		t.Fatalf("got %T, want StyledCell", cell)
	}
	if styled.Style != "\x1b[31m" || styled.Text != "42" {
		t.Fatalf("styled cell: %+v", styled)
	}
}
