// Copyright 2026 apstndb
//
// spancodec_bridge wires github.com/apstndb/spancodec encoders into client-side Result
// construction. Client-side (virtual) result sets are encoded into real
// *spanner.Row values via spancodec.RowEncoder.Rows, then routed through the same
// pipelines as server query results: spanvalue RowIteratorWriter streaming
// (writer.WriteRowSeq) for formats that have one, or the buffered
// spannerRowToRow/withRawJSONMarker cell transform otherwise. Cell styling,
// NULL handling, value formatting, and header metadata therefore stay identical
// to server-side result sets by construction rather than by parallel
// implementation.

package mycli

import (
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spancodec"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanner-mycli/internal/mycli/decoder"
	"github.com/apstndb/spanner-mycli/internal/mycli/format"
	"github.com/apstndb/spanvalue"
	"github.com/apstndb/spanvalue/writer"
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
	helpVariablesRowEncoder = spancodec.MustNewRowEncoder[helpVariableRow]()
	nameValueRowEncoder     = spancodec.MustNewRowEncoder[nameValueRow]()
)

// clientSideFormatContext derives the FormatConfig and value-format mode for
// rendering client-side (virtual) result sets. It mirrors prepareFormatConfig
// for the query path, with one intentional divergence: SQL-literal modes fall
// back to display formatting because client-side results never set
// HasSQLFormattedValues and therefore render as tables in SQL export modes.
func clientSideFormatContext(sysVars *systemVariables) (*spanvalue.FormatConfig, format.ValueFormatMode, error) {
	if sysVars == nil {
		fc, err := decoder.FormatConfigWithProto(nil, false)
		return fc, format.DisplayValues, err
	}

	vfm := format.ValueFormatModeFor(format.Mode(sysVars.Display.CLIFormat.String()))
	if vfm == format.JSONValues {
		return decoder.JSONFormatConfig(), vfm, nil
	}

	fc, err := decoder.FormatConfigWithProto(sysVars.Internal.ProtoDescriptor, sysVars.Display.MultilineProtoText)
	return fc, format.DisplayValues, err
}

// executeStructRows renders a client-side (virtual) result set from rows of
// struct type T: streamed through the same spanvalue writers as server query
// results when the current format has one, or as a buffered Result otherwise.
// sysVars may be nil (e.g., HELP VARIABLES without a session); defaults apply.
func executeStructRows[T any](enc *spancodec.RowEncoder[T], items []T, sysVars *systemVariables) (*Result, error) {
	result, handled, err := streamStructRows(enc, items, sysVars)
	if err != nil {
		return nil, err
	}
	if handled {
		return result, nil
	}
	return resultFromStructRows(enc, items, sysVars)
}

// streamStructRows streams items through a spanvalue RowIteratorWriter via
// writer.WriteRowSeq, the client-side counterpart of
// executeStreamingSQLWithSpanvalueWriter. Only CSV and JSONL are streamed:
// SQL export modes deliberately keep the buffered table-format fallback
// (client-side results have no source table), and the remaining formats keep
// the buffered cell pipeline. Returns handled=false to request that fallback.
func streamStructRows[T any](enc *spancodec.RowEncoder[T], items []T, sysVars *systemVariables) (*Result, bool, error) {
	if sysVars == nil || sysVars.StreamManager == nil {
		return nil, false, nil
	}
	switch sysVars.Display.CLIFormat {
	case enums.DisplayModeCSV, enums.DisplayModeJSONL:
	default:
		return nil, false, nil
	}

	fc, _, err := clientSideFormatContext(sysVars)
	if err != nil {
		return nil, false, err
	}
	w, handled, err := newSpanvalueRowIteratorWriterFor(sysVars, fc)
	if err != nil || !handled {
		return nil, handled, err
	}

	metadata, err := enc.ResultSetMetadata()
	if err != nil {
		return nil, true, err
	}
	res, err := writer.WriteRowSeq(metadata, enc.Rows(items), w)
	if err != nil {
		return nil, true, err
	}

	return &Result{
		TableHeader:  toTableHeader(metadata.GetRowType().GetFields()),
		AffectedRows: res.RowsRead,
		Streamed:     true,
	}, true, nil
}

// resultFromStructRows builds a buffered Result from rows of struct type T.
// Each item is encoded into a *spanner.Row by the compiled spancodec.RowEncoder
// and converted with the same transform used for server query results, and the
// table header carries the row type from RowEncoder.ResultSetMetadata so
// verbose rendering shows column types like server result sets do.
func resultFromStructRows[T any](enc *spancodec.RowEncoder[T], items []T, sysVars *systemVariables) (*Result, error) {
	fc, vfm, err := clientSideFormatContext(sysVars)
	if err != nil {
		return nil, err
	}

	var typeStyles map[sppb.TypeCode]string
	var nullStyle string
	if sysVars != nil {
		typeStyles = sysVars.typeStyles
		nullStyle = sysVars.nullStyle
	}
	transform := spannerRowToRow(fc, typeStyles, nullStyle)
	if vfm == format.JSONValues {
		transform = withRawJSONMarker(transform)
	}

	metadata, err := enc.ResultSetMetadata()
	if err != nil {
		return nil, err
	}

	rows := make([]Row, 0, len(items))
	for row, err := range enc.Rows(items) {
		if err != nil {
			return nil, err
		}
		cells, err := transform(row)
		if err != nil {
			return nil, err
		}
		rows = append(rows, cells)
	}

	return &Result{
		TableHeader:  toTableHeader(metadata.GetRowType().GetFields()),
		Rows:         rows,
		AffectedRows: len(rows),
	}, nil
}
