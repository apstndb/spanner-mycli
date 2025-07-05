package main

import (
	"context"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// MetricsCollector collects Spanner client-side metrics in memory for per-query analysis.
type MetricsCollector struct {
	reader        *sdkmetric.ManualReader
	meterProvider *sdkmetric.MeterProvider
}

// NewMetricsCollector creates a new metrics collector for Spanner client metrics.
func NewMetricsCollector() (*MetricsCollector, error) {
	reader := sdkmetric.NewManualReader()
	
	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			semconv.ServiceName("spanner-mycli"),
			semconv.ServiceVersion("v0.20.0"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(reader),
	)

	return &MetricsCollector{
		reader:        reader,
		meterProvider: provider,
	}, nil
}

// MeterProvider returns the OpenTelemetry MeterProvider to be used with Spanner client.
func (mc *MetricsCollector) MeterProvider() metric.MeterProvider {
	return mc.meterProvider
}

// Reset clears all collected metrics for a fresh measurement.
func (mc *MetricsCollector) Reset() {
	// Collect and discard current metrics to reset
	var rm metricdata.ResourceMetrics
	err := mc.reader.Collect(context.Background(), &rm)
	if err != nil {
		slog.Warn("Failed to reset metrics", "error", err)
	}
	slog.Debug("Metrics reset", "scopes", len(rm.ScopeMetrics))
}

// Collect gathers the current metrics and returns ClientSideMetrics.
func (mc *MetricsCollector) Collect(ctx context.Context) (*ClientSideMetrics, error) {
	var rm metricdata.ResourceMetrics
	err := mc.reader.Collect(ctx, &rm)
	if err != nil {
		return nil, fmt.Errorf("failed to collect metrics: %w", err)
	}

	result := &ClientSideMetrics{}
	
	// Debug: log all collected metrics
	slog.Debug("Raw metrics collected", "scopes", len(rm.ScopeMetrics))
	
	// Log all scope names first
	var scopeNames []string
	for _, sm := range rm.ScopeMetrics {
		scopeNames = append(scopeNames, sm.Scope.Name)
	}
	slog.Debug("All scope names", "names", scopeNames)
	
	for _, scopeMetrics := range rm.ScopeMetrics {
		slog.Debug("Scope metrics", "name", scopeMetrics.Scope.Name, "version", scopeMetrics.Scope.Version, "metrics", len(scopeMetrics.Metrics))
		for _, m := range scopeMetrics.Metrics {
			slog.Debug("Metric found", "name", m.Name, "type", fmt.Sprintf("%T", m.Data))
			switch m.Name {
			// Check both with and without spanner/ prefix since different versions may use different formats
			case "spanner/gfe_latency", "gfe_latencies":
				if hist, ok := m.Data.(metricdata.Histogram[int64]); ok {
					slog.Debug("GFE latency histogram", "datapoints", len(hist.DataPoints))
					for i, dp := range hist.DataPoints {
						attrs := dp.Attributes.ToSlice()
						attrMap := make(map[string]string)
						for _, attr := range attrs {
							attrMap[string(attr.Key)] = attr.Value.AsString()
						}
						slog.Debug("GFE latency datapoint", 
							"index", i, 
							"sum", dp.Sum, 
							"count", dp.Count,
							"attributes", attrMap,
							"startTime", dp.StartTime,
							"time", dp.Time)
					}
					result.GFELatencyMs = extractLatencyMsInt64WithFilter(hist)
				} else if hist, ok := m.Data.(metricdata.Histogram[float64]); ok {
					result.GFELatencyMs = extractLatencyMsWithFilter(hist)
				}
			case "spanner/afe_latency", "afe_latencies":
				if hist, ok := m.Data.(metricdata.Histogram[float64]); ok {
					result.AFELatencyMs = extractLatencyMsWithFilter(hist)
				} else if hist, ok := m.Data.(metricdata.Histogram[int64]); ok {
					result.AFELatencyMs = extractLatencyMsInt64WithFilter(hist)
				}
			case "spanner/operation_latencies", "operation_latencies":
				if hist, ok := m.Data.(metricdata.Histogram[float64]); ok {
					result.OperationLatencyMs = extractLatencyMsWithFilter(hist)
				} else if hist, ok := m.Data.(metricdata.Histogram[int64]); ok {
					result.OperationLatencyMs = extractLatencyMsInt64WithFilter(hist)
				}
			case "spanner/attempt_latencies", "attempt_latencies":
				if hist, ok := m.Data.(metricdata.Histogram[float64]); ok {
					result.AttemptLatencyMs = extractLatencyMsWithFilter(hist)
				} else if hist, ok := m.Data.(metricdata.Histogram[int64]); ok {
					result.AttemptLatencyMs = extractLatencyMsInt64WithFilter(hist)
				}
			case "spanner/attempt_count", "attempt_count":
				if sum, ok := m.Data.(metricdata.Sum[int64]); ok {
					result.AttemptCount = extractCount(sum)
				}
			case "spanner/gfe_connectivity_error_count", "gfe_connectivity_error_count":
				if sum, ok := m.Data.(metricdata.Sum[int64]); ok {
					result.GFEErrorCount = extractCount(sum)
				}
			case "spanner/afe_connectivity_error_count", "afe_connectivity_error_count":
				if sum, ok := m.Data.(metricdata.Sum[int64]); ok {
					result.AFEErrorCount = extractCount(sum)
				}
			}
		}
	}

	return result, nil
}

// Shutdown gracefully shuts down the metrics collector.
func (mc *MetricsCollector) Shutdown(ctx context.Context) error {
	return mc.meterProvider.Shutdown(ctx)
}

// extractLatencyMs extracts the mean latency in milliseconds from a histogram.
func extractLatencyMs(hist metricdata.Histogram[float64]) float64 {
	var total float64
	var count uint64
	
	for _, dp := range hist.DataPoints {
		total += dp.Sum
		count += dp.Count
	}
	
	if count == 0 {
		return 0
	}
	
	// Convert from microseconds to milliseconds
	return (total / float64(count)) / 1000
}

// extractLatencyMsWithFilter extracts the mean latency in milliseconds from a float64 histogram,
// filtering to only include data points for query/read operations.
func extractLatencyMsWithFilter(hist metricdata.Histogram[float64]) float64 {
	if len(hist.DataPoints) == 0 {
		return 0
	}
	
	// Priority order for methods we want to report
	methodPriority := []string{
		"query",           // ExecuteStreamingSql
		"read",            // StreamingRead
		"executeUpdate",   // ExecuteSql (for DML)
		"executeBatchDml", // ExecuteBatchDml
		"commit",          // Commit
	}
	
	// Try to find data points for priority methods
	for _, targetMethod := range methodPriority {
		for _, dp := range hist.DataPoints {
			method, ok := dp.Attributes.Value("grpc_client_method")
			if ok && method.AsString() == targetMethod && dp.Count > 0 {
				return dp.Sum / float64(dp.Count) / 1000
			}
		}
	}
	
	// If no priority method found, exclude session management operations
	excludeMethods := map[string]bool{
		"executeBatchCreateSessions": true,
		"createSession":              true,
		"deleteSession":              true,
		"getSession":                 true,
	}
	
	// Find the most recent non-excluded method
	var mostRecent *metricdata.HistogramDataPoint[float64]
	for i := range hist.DataPoints {
		dp := &hist.DataPoints[i]
		method, ok := dp.Attributes.Value("grpc_client_method")
		if ok && !excludeMethods[method.AsString()] && dp.Count > 0 {
			if mostRecent == nil || dp.Time.After(mostRecent.Time) {
				mostRecent = dp
			}
		}
	}
	
	if mostRecent != nil {
		return mostRecent.Sum / float64(mostRecent.Count) / 1000
	}
	
	// Fallback: use any data point if nothing else matches
	if hist.DataPoints[0].Count > 0 {
		return hist.DataPoints[0].Sum / float64(hist.DataPoints[0].Count) / 1000
	}
	
	return 0
}

// extractCount extracts the total count from a sum metric.
func extractCount(sum metricdata.Sum[int64]) int64 {
	if len(sum.DataPoints) == 0 {
		return 0
	}
	
	// If there's only one data point, use it
	if len(sum.DataPoints) == 1 {
		return sum.DataPoints[0].Value
	}
	
	// Multiple data points - use the most recent one
	var mostRecent *metricdata.DataPoint[int64]
	for i := range sum.DataPoints {
		dp := &sum.DataPoints[i]
		if mostRecent == nil || dp.Time.After(mostRecent.Time) {
			mostRecent = dp
		}
	}
	
	if mostRecent == nil {
		return 0
	}
	
	return mostRecent.Value
}

// extractLatencyMsInt64 extracts the mean latency in milliseconds from an int64 histogram.
func extractLatencyMsInt64(hist metricdata.Histogram[int64]) float64 {
	var total int64
	var count uint64
	
	for _, dp := range hist.DataPoints {
		total += dp.Sum
		count += dp.Count
	}
	
	if count == 0 {
		return 0
	}
	
	// Convert from microseconds to milliseconds
	return float64(total) / float64(count) / 1000
}

// extractLatencyMsInt64WithFilter extracts the mean latency in milliseconds from an int64 histogram,
// filtering to only include data points for query/read operations.
func extractLatencyMsInt64WithFilter(hist metricdata.Histogram[int64]) float64 {
	if len(hist.DataPoints) == 0 {
		return 0
	}
	
	// Priority order for methods we want to report
	methodPriority := []string{
		"query",           // ExecuteStreamingSql
		"read",            // StreamingRead
		"executeUpdate",   // ExecuteSql (for DML)
		"executeBatchDml", // ExecuteBatchDml
		"commit",          // Commit
	}
	
	// Try to find data points for priority methods
	for _, targetMethod := range methodPriority {
		for _, dp := range hist.DataPoints {
			method, ok := dp.Attributes.Value("grpc_client_method")
			if ok && method.AsString() == targetMethod && dp.Count > 0 {
				return float64(dp.Sum) / float64(dp.Count) / 1000
			}
		}
	}
	
	// If no priority method found, exclude session management operations
	excludeMethods := map[string]bool{
		"executeBatchCreateSessions": true,
		"createSession":              true,
		"deleteSession":              true,
		"getSession":                 true,
	}
	
	// Find the most recent non-excluded method
	var mostRecent *metricdata.HistogramDataPoint[int64]
	for i := range hist.DataPoints {
		dp := &hist.DataPoints[i]
		method, ok := dp.Attributes.Value("grpc_client_method")
		if ok && !excludeMethods[method.AsString()] && dp.Count > 0 {
			if mostRecent == nil || dp.Time.After(mostRecent.Time) {
				mostRecent = dp
			}
		}
	}
	
	if mostRecent != nil {
		return float64(mostRecent.Sum) / float64(mostRecent.Count) / 1000
	}
	
	// Fallback: use any data point if nothing else matches
	if hist.DataPoints[0].Count > 0 {
		return float64(hist.DataPoints[0].Sum) / float64(hist.DataPoints[0].Count) / 1000
	}
	
	return 0
}