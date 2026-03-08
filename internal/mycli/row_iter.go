package mycli

import (
	"slices"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/internal/mycli/format"
	"github.com/apstndb/spanner-mycli/internal/mycli/metrics"
	"github.com/apstndb/spanvalue"
	"github.com/go-json-experiment/json"
	"github.com/ngicks/go-iterator-helper/hiter"
	"google.golang.org/protobuf/types/known/structpb"
)

// extractColumnNames extract column names from ResultSetMetadata.RowType.Fields.
func extractColumnNames(fields []*sppb.StructType_Field) []string {
	return slices.Collect(hiter.Map((*sppb.StructType_Field).GetName, slices.Values(fields)))
}

// parseQueryStats parses spanner.RowIterator.QueryStats.
func parseQueryStats(stats map[string]any) (QueryStats, error) {
	var queryStats QueryStats

	b, err := json.Marshal(stats)
	if err != nil {
		return queryStats, err
	}

	err = json.Unmarshal(b, &queryStats)
	if err != nil {
		return queryStats, err
	}
	return queryStats, nil
}

// consumeRowIterDiscard calls iter.Stop().
func consumeRowIterDiscard(iter *spanner.RowIterator) (queryStats map[string]interface{}, rowCount int64, metadata *sppb.ResultSetMetadata, queryPlan *sppb.QueryPlan, err error) {
	return consumeRowIter(iter, func(*spanner.Row) error { return nil })
}

// consumeRowIter calls iter.Stop().
func consumeRowIter(iter *spanner.RowIterator, f func(*spanner.Row) error) (queryStats map[string]interface{}, rowCount int64, metadata *sppb.ResultSetMetadata, queryPlan *sppb.QueryPlan, err error) {
	defer iter.Stop()
	err = iter.Do(f)
	if err != nil {
		return nil, 0, nil, nil, err
	}

	return iter.QueryStats, iter.RowCount, iter.Metadata, iter.QueryPlan, nil
}

func consumeRowIterCollect[T any](iter *spanner.RowIterator, f func(*spanner.Row) (T, error)) (rows []T, queryStats map[string]interface{}, rowCount int64, metadata *sppb.ResultSetMetadata, queryPlan *sppb.QueryPlan, err error) {
	var results []T
	stats, count, metadata, plan, err := consumeRowIter(iter, func(row *spanner.Row) error {
		v, err := f(row)
		if err != nil {
			return err
		}
		results = append(results, v)
		return nil
	})

	return results, stats, count, metadata, plan, err
}

// consumeRowIterCollectWithMetrics is like consumeRowIterCollect but collects metrics during execution.
func consumeRowIterCollectWithMetrics[T any](iter *spanner.RowIterator, f func(*spanner.Row) (T, error), m *metrics.ExecutionMetrics) (rows []T, queryStats map[string]interface{}, rowCount int64, metadata *sppb.ResultSetMetadata, queryPlan *sppb.QueryPlan, err error) {
	var results []T
	firstRow := true

	stats, count, metadata, plan, err := consumeRowIter(iter, func(row *spanner.Row) error {
		now := time.Now()

		// Record TTFB on first row
		if firstRow {
			firstRow = false
			m.FirstRowTime = &now
		}

		v, err := f(row)
		if err != nil {
			return err
		}
		results = append(results, v)

		// Update last row time
		m.LastRowTime = &now
		m.RowCount = int64(len(results))

		return nil
	})

	return results, stats, count, metadata, plan, err
}

// spannerRowToRow converts a Spanner row to a format.Row with appropriate Cell types.
// typeStyles maps Spanner type codes to ANSI SGR sequences; nil or empty means
// all non-NULL values use PlainCell (the default behavior).
func spannerRowToRow(fc *spanvalue.FormatConfig, typeStyles map[sppb.TypeCode]string) func(row *spanner.Row) (Row, error) {
	return func(row *spanner.Row) (Row, error) {
		result := make(Row, row.Size())
		for i := range row.Size() {
			var gcv spanner.GenericColumnValue
			if err := row.Column(i, &gcv); err != nil {
				return nil, err
			}

			text, err := fc.FormatToplevelColumn(gcv)
			if err != nil {
				return nil, err
			}

			// Cell type is chosen by value/type semantics.
			// NULL overrides type styling. Type→style mapping determines StyledCell.
			// The rendering layer (FormatConfig.Styled) decides whether to call
			// Format() (styled) or RawText() (plain).
			if _, isNull := gcv.Value.GetKind().(*structpb.Value_NullValue); isNull {
				result[i] = format.NullCell{Text: text}
			} else if style, ok := typeStyles[gcv.Type.GetCode()]; ok {
				result[i] = format.StyledCell{Text: text, Style: style}
			} else {
				result[i] = format.PlainCell{Text: text}
			}
		}
		return result, nil
	}
}
