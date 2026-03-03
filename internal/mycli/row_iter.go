package mycli

import (
	"slices"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/internal/mycli/metrics"
	"github.com/apstndb/spanvalue"
	"github.com/go-json-experiment/json"
	"github.com/ngicks/go-iterator-helper/hiter"
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

func spannerRowToRow(fc *spanvalue.FormatConfig) func(row *spanner.Row) (Row, error) {
	return func(row *spanner.Row) (Row, error) {
		columns, err := fc.FormatRow(row)
		if err != nil {
			return Row{}, err
		}
		return toRow(columns...), nil
	}
}
