// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mycli

import (
	"context"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
)

// The latency histogram bounds are in seconds. Merge the positive-count
// histograms before estimating percentiles, then convert to milliseconds.
// https://docs.cloud.google.com/spanner/docs/introspection/query-statistics#find_the_99th_percentile_latency_in_the_last_hour_for_a_query_fingerprint
const showQueryStatsGoogleSQL = `WITH histograms AS (
  SELECT INTERVAL_END, LATENCY_DISTRIBUTION[SAFE_OFFSET(0)] AS histogram
  FROM SPANNER_SYS.QUERY_STATS_TOP_MINUTE
  WHERE TEXT_FINGERPRINT = @fprint
    AND INTERVAL_END > TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 1 HOUR)
    AND INTERVAL_END <= CURRENT_TIMESTAMP()
), merged AS (
  SELECT COUNT(DISTINCT INTERVAL_END) AS observed_minutes,
         COALESCE(SUM(histogram.COUNT), 0) AS samples,
         SPANNER_SYS.DISTRIBUTION_AGG(histogram) AS distribution
  FROM histograms
  WHERE histogram.COUNT > 0
)
SELECT @fprint AS text_fingerprint, CURRENT_TIMESTAMP() AS window_end_utc,
       60 AS window_minutes, observed_minutes, samples,
       IF(samples > 0, 1000 * SPANNER_SYS.DISTRIBUTION_PERCENTILE(distribution, 50.0), NULL) AS p50_ms,
       IF(samples > 0, 1000 * SPANNER_SYS.DISTRIBUTION_PERCENTILE(distribution, 95.0), NULL) AS p95_ms,
       IF(samples > 0, 1000 * SPANNER_SYS.DISTRIBUTION_PERCENTILE(distribution, 99.0), NULL) AS p99_ms
FROM merged`

// The PostgreSQL interface exposes the same histogram as a JSON string.
const showQueryStatsPostgreSQL = `WITH histograms AS (
  SELECT interval_end, latency_distribution_json_string AS histogram,
         (latency_distribution_json_string::jsonb ->> 'COUNT')::bigint AS histogram_count
  FROM spanner_sys.query_stats_top_minute
  WHERE text_fingerprint = $1
    AND interval_end > spanner.timestamptz_subtract(now(), '1 HOUR')
    AND interval_end <= now()
), merged AS (
  SELECT COUNT(DISTINCT interval_end) AS observed_minutes,
         COALESCE(SUM(histogram_count), 0)::bigint AS samples,
         spanner_sys.distribution_agg(histogram) AS distribution
  FROM histograms
  WHERE histogram_count > 0
)
SELECT $1 AS text_fingerprint, now() AS window_end_utc,
       60::bigint AS window_minutes, observed_minutes, samples,
       CASE WHEN samples > 0 THEN 1000 * spanner_sys.distribution_percentile(distribution, 50.0) END AS p50_ms,
       CASE WHEN samples > 0 THEN 1000 * spanner_sys.distribution_percentile(distribution, 95.0) END AS p95_ms,
       CASE WHEN samples > 0 THEN 1000 * spanner_sys.distribution_percentile(distribution, 99.0) END AS p99_ms
FROM merged`

type ShowQueryStatsStatement struct {
	Fprint int64
}

func (s *ShowQueryStatsStatement) Execute(ctx context.Context, session *Session, _ OperationOutput) (*Result, error) {
	stmt := spanner.Statement{
		SQL:    showQueryStatsGoogleSQL,
		Params: map[string]any{"fprint": s.Fprint},
	}
	if session.systemVariables.Feature.DatabaseDialect == databasepb.DatabaseDialect_POSTGRESQL {
		stmt = spanner.Statement{
			SQL:    showQueryStatsPostgreSQL,
			Params: map[string]any{"p1": s.Fprint},
		}
	}
	return executeInformationSchemaBasedStatement(ctx, session, "SHOW QUERY STATS", stmt, nil)
}
