// Copyright 2026 apstndb
//
// spancodec_bridge wires github.com/apstndb/spancodec encoders into client-side Result
// construction. Client-side (virtual) result sets are encoded into real
// *spanner.Row values via spancodec.RowEncoder.Rows, then routed through the same
// pipelines as server query results: spanvalue RowIteratorWriter streaming
// (writer.WriteRowSeq) for formats that have one, or a typed buffered Result
// (Result.Typed) otherwise, which printTableData renders lazily with the same
// transforms as server query results. Cell styling, NULL handling, value
// formatting, and header metadata therefore stay identical to server-side
// result sets by construction rather than by parallel implementation.

package mycli

import (
	"io"

	"cloud.google.com/go/spanner"
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
// SQLExportAllowed and therefore render as tables in SQL export modes.
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
// session may be nil (e.g., HELP without a connection); defaults apply and
// the result is buffered.
func executeStructRows[T any](enc *spancodec.RowEncoder[T], items []T, session *Session) (*Result, error) {
	var sysVars *systemVariables
	var out io.Writer
	if session != nil {
		sysVars = session.systemVariables
		out = session.outputWriter()
	}

	result, handled, err := streamStructRows(enc, items, sysVars, out)
	if err != nil {
		return nil, err
	}
	if handled {
		return result, nil
	}
	return resultFromStructRows(enc, items)
}

// streamStructRows streams items through a spanvalue RowIteratorWriter via
// writer.WriteRowSeq, the client-side counterpart of
// executeStreamingSQLWithSpanvalueWriter. Only CSV and JSONL are streamed:
// SQL export modes deliberately keep the buffered table-format fallback
// (client-side results have no source table), and the remaining formats keep
// the buffered cell pipeline. Returns handled=false to request that fallback.
func streamStructRows[T any](enc *spancodec.RowEncoder[T], items []T, sysVars *systemVariables, out io.Writer) (*Result, bool, error) {
	if sysVars == nil || out == nil {
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
	w, handled, err := newSpanvalueRowIteratorWriterFor(out, sysVars, fc)
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

// resultFromStructRows builds a typed buffered Result from rows of struct type
// T. Each item is encoded into a *spanner.Row by the compiled
// spancodec.RowEncoder and carried raw in Result.Typed, so export formats
// re-render from values through the single spanvalue emitters while
// table-family formats derive display cells lazily in printTableData with the
// same transform as the server query path. The table header carries the row
// type from RowEncoder.ResultSetMetadata so verbose rendering shows column
// types like server result sets do.
//
// SQLExportAllowed stays false: client-side result sets have no source table
// and fall back to TABLE format under SQL_INSERT* modes, preserving the prior
// behavior where clientSideFormatContext forced display formatting there.
func resultFromStructRows[T any](enc *spancodec.RowEncoder[T], items []T) (*Result, error) {
	metadata, err := enc.ResultSetMetadata()
	if err != nil {
		return nil, err
	}

	rows := make([]*spanner.Row, 0, len(items))
	for row, err := range enc.Rows(items) {
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}

	return &Result{
		TableHeader: toTableHeader(metadata.GetRowType().GetFields()),
		Typed: &TypedRows{
			Metadata:         metadata,
			Rows:             rows,
			SQLExportAllowed: false,
		},
		AffectedRows: len(rows),
	}, nil
}
