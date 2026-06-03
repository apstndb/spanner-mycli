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
	"errors"
	"fmt"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanner-mycli/internal/mycli/format"
	"github.com/apstndb/spanvalue/writer"
)

// executeStreamingSQLWithSpanvalueWriter is a PoC path that lets spanvalue drive
// RowIterator streaming directly for formats whose writer package can consume
// *spanner.Row values without the intermediate Row/Cell layer.
func executeStreamingSQLWithSpanvalueWriter(qe *queryExecution) (*Result, bool, error) {
	w, handled, err := newSpanvalueRowIteratorWriter(qe)
	if err != nil || !handled {
		return nil, handled, err
	}

	rowIterResult, rowCount, err := runSpanvalueRowIterator(qe, w)
	if err != nil {
		return nil, true, normalizeSpanvalueWriterError(err)
	}

	fields, queryStats, queryPlan := rowIteratorResultParts(rowIterResult)
	result := &Result{
		Rows:                  nil,
		TableHeader:           toTableHeader(fields),
		AffectedRows:          int(rowCount),
		Streamed:              true,
		HasSQLFormattedValues: qe.ValueFmtMode == format.SQLLiteralValues,
	}

	if err := finalizeQueryResult(
		result,
		queryStats,
		qe.ReadOnlyTxn,
		queryPlan,
		qe.SysVars,
		qe.Metrics,
	); err != nil {
		return nil, true, err
	}
	return result, true, nil
}

// executeStreamingSQLWithSpanvalueProcessor uses spanvalue's RowIterator hooks
// for formats that still need spanner-mycli's RowProcessor/StreamingFormatter
// stack, such as TABLE, TAB, VERTICAL, HTML, and XML.
func executeStreamingSQLWithSpanvalueProcessor(qe *queryExecution) (*Result, error) {
	rowTransform := spannerRowToRow(qe.FormatConfig, qe.SysVars.typeStyles, qe.SysVars.nullStyle)
	if qe.ValueFmtMode == format.JSONValues {
		rowTransform = withRawJSONMarker(rowTransform)
	}

	rowIterResult, rowCount, err := runSpanvalueRowIteratorWithProcessor(qe, rowTransform)
	if err != nil {
		return nil, err
	}

	fields, queryStats, queryPlan := rowIteratorResultParts(rowIterResult)
	result := &Result{
		Rows:                  nil,
		TableHeader:           toTableHeader(fields),
		AffectedRows:          int(rowCount),
		Streamed:              true,
		HasSQLFormattedValues: qe.ValueFmtMode == format.SQLLiteralValues,
	}

	if err := finalizeQueryResult(
		result,
		queryStats,
		qe.ReadOnlyTxn,
		queryPlan,
		qe.SysVars,
		qe.Metrics,
	); err != nil {
		return nil, err
	}
	return result, nil
}

func newSpanvalueRowIteratorWriter(qe *queryExecution) (writer.RowIteratorWriter, bool, error) {
	out := qe.SysVars.StreamManager.GetWriter()
	if out == nil {
		return nil, false, nil
	}

	switch qe.SysVars.Display.CLIFormat {
	case enums.DisplayModeCSV:
		w, err := writer.NewCSVWriter(
			out,
			writer.WithFormatter(qe.FormatConfig),
			writer.WithHeader(!qe.SysVars.Display.SkipColumnNames),
			writer.WithUnnamedFieldNamer(nil),
		)
		if err != nil {
			return nil, true, err
		}
		return w, true, nil
	case enums.DisplayModeJSONL:
		w, err := writer.NewJSONLWriter(
			out,
			writer.WithFormatter(qe.FormatConfig),
			writer.WithUnnamedFieldNamer(nil),
		)
		if err != nil {
			return nil, true, err
		}
		return w, true, nil
	case enums.DisplayModeSQLInsert, enums.DisplayModeSQLInsertOrIgnore, enums.DisplayModeSQLInsertOrUpdate:
		if qe.SysVars.Display.SQLTableName == "" {
			return nil, true, fmt.Errorf("SQL export requires a table name. Auto-detection failed (query may be too complex).\n" +
				"Options:\n" +
				"  1. Use DUMP TABLE for full table exports\n" +
				"  2. Set CLI_SQL_TABLE_NAME explicitly for complex queries\n" +
				"  3. Ensure your query matches: SELECT * FROM table_name [WHERE/ORDER BY/LIMIT]")
		}
		batchSize, err := spanvalueSQLBatchSize(qe.SysVars.Display.SQLBatchSize)
		if err != nil {
			return nil, true, err
		}
		w, err := writer.NewSQLInsertWriter(
			out,
			qe.SysVars.Display.SQLTableName,
			writer.WithFormatter(qe.FormatConfig),
			writer.WithSQLBatchSize(batchSize),
			writer.WithSQLDialect(qe.SysVars.Feature.DatabaseDialect),
			writer.WithSQLInsertKind(spanvalueSQLInsertKind(qe.SysVars.Display.CLIFormat)),
		)
		if err != nil {
			return nil, true, err
		}
		return w, true, nil
	default:
		return nil, false, nil
	}
}

func spanvalueSQLInsertKind(mode enums.DisplayMode) writer.SQLInsertKind {
	switch mode {
	case enums.DisplayModeSQLInsertOrIgnore:
		return writer.SQLInsertOrIgnore
	case enums.DisplayModeSQLInsertOrUpdate:
		return writer.SQLInsertOrUpdate
	default:
		return writer.SQLInsert
	}
}

func spanvalueSQLBatchSize(batchSize int64) (int, error) {
	if batchSize < 0 {
		return 0, fmt.Errorf("CLI_SQL_BATCH_SIZE cannot be negative: %d", batchSize)
	}

	const maxBatchSize = 10000
	if batchSize > maxBatchSize {
		return 0, fmt.Errorf("CLI_SQL_BATCH_SIZE %d exceeds maximum supported value of %d (limited for Spanner mutation constraints)", batchSize, maxBatchSize)
	}

	maxInt := int64(int(^uint(0) >> 1))
	if batchSize > maxInt {
		return 0, fmt.Errorf("CLI_SQL_BATCH_SIZE %d exceeds maximum supported value on this platform", batchSize)
	}
	return int(batchSize), nil
}

func runSpanvalueRowIterator(qe *queryExecution, w writer.RowIteratorWriter) (*writer.RowIteratorResult, int64, error) {
	hooks := writer.RowIteratorHooksFromWriter(w)

	return runRowIteratorTransform(
		qe.Iter,
		func(row *spanner.Row) (*spanner.Row, error) { return row, nil },
		rowIteratorSink[*spanner.Row]{
			PrepareMetadata: hooks.PrepareMetadata,
			Write:           hooks.WriteRow,
			Finish: func(result *writer.RowIteratorResult, _ int64) error {
				if hooks.Finish == nil {
					return nil
				}
				return hooks.Finish(result)
			},
		},
		withRowIteratorMetrics(qe.Metrics),
		withRowIteratorAfterWriteRow(func() error { return flushSpanvalueStreamingRow(w) }),
	)
}

func runSpanvalueRowIteratorWithProcessor(
	qe *queryExecution,
	rowTransform func(*spanner.Row) (Row, error),
) (*writer.RowIteratorResult, int64, error) {
	initialized := false

	initProcessor := func(md *sppb.ResultSetMetadata) error {
		if initialized || md == nil {
			return nil
		}
		if err := qe.Processor.Init(md, qe.SysVars); err != nil {
			return fmt.Errorf("failed to initialize processor: %w", err)
		}
		initialized = true
		return nil
	}

	return runRowIteratorTransform(
		qe.Iter,
		rowTransform,
		rowIteratorSink[Row]{
			PrepareMetadata: initProcessor,
			Write:           qe.Processor.ProcessRow,
			Finish: func(result *writer.RowIteratorResult, rowCount int64) error {
				_, queryStats, _ := rowIteratorResultParts(result)
				var metadata *sppb.ResultSetMetadata
				if result != nil {
					metadata = result.Metadata
				}
				if err := initProcessor(metadata); err != nil {
					return err
				}
				parsedStats, _ := parseQueryStats(queryStats)
				if err := qe.Processor.Finish(parsedStats, rowCount); err != nil {
					return fmt.Errorf("failed to finish processing: %w", err)
				}
				return nil
			},
		},
		withRowIteratorMetrics(qe.Metrics),
		withRowIteratorErrorLabels("failed to transform row", "failed to process row", ""),
	)
}

func rowIteratorResultParts(result *writer.RowIteratorResult) ([]*sppb.StructType_Field, map[string]any, *sppb.QueryPlan) {
	if result == nil {
		return nil, nil, nil
	}

	var fields []*sppb.StructType_Field
	if result.Metadata != nil {
		fields = result.Metadata.GetRowType().GetFields()
	}
	return fields, result.Stats.QueryStats, result.Stats.QueryPlan
}

func flushSpanvalueStreamingRow(w writer.RowIteratorWriter) error {
	// RowIteratorWriter always has Flush, but per-row flushing is only safe for
	// DelimitedWriter. SQLInsertWriter.Flush finalizes partial INSERT batches and
	// would defeat CLI_SQL_BATCH_SIZE if called after every row.
	if _, ok := w.(*writer.DelimitedWriter); !ok {
		return nil
	}
	return w.Flush()
}

func normalizeSpanvalueWriterError(err error) error {
	if errors.Is(err, writer.ErrEmptyColumnName) {
		return fmt.Errorf("column has no name; SQL export requires all columns to have names (consider using aliases in your query): %w", err)
	}
	return err
}
