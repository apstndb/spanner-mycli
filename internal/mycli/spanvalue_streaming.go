package mycli

import (
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/spanner"
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

	result := &Result{
		Rows:                  nil,
		TableHeader:           toTableHeader(rowIterResult.Metadata.GetRowType().GetFields()),
		AffectedRows:          int(rowCount),
		Streamed:              true,
		HasSQLFormattedValues: qe.ValueFmtMode == format.SQLLiteralValues,
	}

	if err := finalizeQueryResult(
		result,
		rowIterResult.Stats.QueryStats,
		qe.ReadOnlyTxn,
		rowIterResult.Stats.QueryPlan,
		qe.SysVars,
		qe.Metrics,
	); err != nil {
		return nil, true, err
	}
	return result, true, nil
}

func newSpanvalueRowIteratorWriter(qe *queryExecution) (writer.RowIteratorWriter, bool, error) {
	out := qe.SysVars.StreamManager.GetWriter()
	if out == nil {
		return nil, false, nil
	}

	switch qe.SysVars.Display.CLIFormat {
	case enums.DisplayModeCSV:
		return writer.NewCSVWriter(
			out,
			writer.WithFormatter(qe.FormatConfig),
			writer.WithHeader(!qe.SysVars.Display.SkipColumnNames),
			writer.WithUnnamedFieldNamer(nil),
		), true, nil
	case enums.DisplayModeJSONL:
		return writer.NewJSONLWriter(
			out,
			writer.WithFormatter(qe.FormatConfig),
			writer.WithUnnamedFieldNamer(nil),
		), true, nil
	case enums.DisplayModeSQLInsert, enums.DisplayModeSQLInsertOrIgnore, enums.DisplayModeSQLInsertOrUpdate:
		if qe.SysVars.Display.SQLTableName == "" {
			return nil, false, nil
		}
		batchSize, err := spanvalueSQLBatchSize(qe.SysVars.Display.SQLBatchSize)
		if err != nil {
			return nil, true, err
		}
		return writer.NewSQLInsertWriter(
			out,
			qe.SysVars.Display.SQLTableName,
			writer.WithFormatter(qe.FormatConfig),
			writer.WithSQLBatchSize(batchSize),
			writer.WithSQLDialect(qe.SysVars.Feature.DatabaseDialect),
			writer.WithSQLInsertKind(spanvalueSQLInsertKind(qe.SysVars.Display.CLIFormat)),
		), true, nil
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
	writeRow := hooks.WriteRow

	var rowCount int64
	hooks.WriteRow = func(row *spanner.Row) error {
		now := time.Now()
		if qe.Metrics != nil && qe.Metrics.FirstRowTime == nil {
			qe.Metrics.FirstRowTime = &now
		}

		if writeRow != nil {
			if err := writeRow(row); err != nil {
				return err
			}
		}

		rowCount++
		if qe.Metrics != nil {
			qe.Metrics.LastRowTime = &now
			qe.Metrics.RowCount = rowCount
		}
		return flushSpanvalueStreamingRow(w)
	}

	result, err := writer.RunRowIterator(qe.Iter, hooks)
	return result, rowCount, err
}

func flushSpanvalueStreamingRow(w writer.RowIteratorWriter) error {
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
