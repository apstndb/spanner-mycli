// Copyright 2026 apstndb
//
// spanenc_bridge wires github.com/apstndb/spanenc encoders into client-side Result
// construction. spanenc mirrors cloud.google.com/go/spanner encodeValue semantics
// and composes with spanvalue formatters.
//
// SHOW VARIABLES and HELP VARIABLES are all-string rows — the spanenc adoption
// guide's "poor fit" case where a GCV round trip adds work with no display change.
// They use the bridge anyway as scaffolding: one styled-cell path shared with
// server query results, and a landing zone for genuinely typed virtual columns later
// (PROTO-typed SHOW output, MUTATE preview via MutationColumnsAndValues).
// Do not "simplify" these back to toRow without checking for those call sites.

package mycli

import (
	"fmt"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanenc"
	"github.com/apstndb/spanner-mycli/internal/mycli/decoder"
	"github.com/apstndb/spanner-mycli/internal/mycli/format"
	"github.com/apstndb/spanvalue"
	"google.golang.org/protobuf/types/known/structpb"
)

// helpVariableRow is the row shape for HELP VARIABLES.
type helpVariableRow struct {
	Name        string `spanner:"name"`
	Operations  string `spanner:"operations"`
	Description string `spanner:"desc"`
}

// nameValueRow is the row shape for SHOW VARIABLES.
type nameValueRow struct {
	Name  string `spanner:"name"`
	Value string `spanner:"value"`
}

var (
	helpVariablesRowEncoder = mustNewRowEncoder[helpVariableRow]()
	nameValueRowEncoder     = mustNewRowEncoder[nameValueRow]()
)

func mustNewRowEncoder[T any]() *spanenc.RowEncoder[T] {
	enc, err := spanenc.NewRowEncoder[T]()
	if err != nil {
		panic(fmt.Sprintf("spanenc.NewRowEncoder[%T]: %v", *new(T), err))
	}
	return enc
}

// clientSideFormatConfig returns the display formatter used for virtual result sets.
func clientSideFormatConfig(session *Session) (*spanvalue.FormatConfig, error) {
	if session == nil {
		return spanvalue.SpannerCLICompatibleFormatConfig(), nil
	}
	return decoder.FormatConfigWithProto(
		session.systemVariables.Internal.ProtoDescriptor,
		session.systemVariables.Display.MultilineProtoText,
	)
}

// resultFromRowEncoder builds a buffered Result from one or more rows of struct
// type T using a compiled spanenc.RowEncoder. Column names come from the encoder;
// cell text uses spanvalue.FormatRowColumns on RowEncoder.Values output.
func resultFromRowEncoder[T any](enc *spanenc.RowEncoder[T], items []T, fc *spanvalue.FormatConfig, typeStyles map[sppb.TypeCode]string, nullStyle string) (*Result, error) {
	columnNames := enc.Columns()

	rows := make([]Row, 0, len(items))
	for _, item := range items {
		gcvs, err := enc.Values(item)
		if err != nil {
			return nil, err
		}
		row, err := rowFromGCVs(columnNames, gcvs, fc, typeStyles, nullStyle)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}

	return &Result{
		TableHeader:  toTableHeader(columnNames),
		Rows:         rows,
		AffectedRows: len(rows),
	}, nil
}

// rowFromGCVs formats a parallel GCV slice into a display Row.
func rowFromGCVs(columnNames []string, gcvs []spanner.GenericColumnValue, fc *spanvalue.FormatConfig, typeStyles map[sppb.TypeCode]string, nullStyle string) (Row, error) {
	texts, err := spanvalue.FormatRowColumns(fc, columnNames, gcvs)
	if err != nil {
		return nil, err
	}

	result := make(Row, len(texts))
	for i, text := range texts {
		result[i] = gcvTextToCell(text, gcvs[i], typeStyles, nullStyle)
	}
	return result, nil
}

func gcvTextToCell(text string, gcv spanner.GenericColumnValue, typeStyles map[sppb.TypeCode]string, nullStyle string) format.Cell {
	// gcvctor treats a nil Value as NULL; spanenc-produced GCVs are always populated,
	// but this helper may format foreign GCVs (e.g. rows read back from the server).
	if gcv.Value == nil {
		return format.NoWrapCell{Cell: format.StyledCell{Text: text, Style: nullStyle}}
	}
	if _, isNull := gcv.Value.GetKind().(*structpb.Value_NullValue); isNull {
		return format.NoWrapCell{Cell: format.StyledCell{Text: text, Style: nullStyle}}
	}
	if gcv.Type != nil {
		if style, ok := typeStyles[gcv.Type.GetCode()]; ok {
			return format.StyledCell{Text: text, Style: style}
		}
	}
	return format.PlainCell{Text: text}
}
