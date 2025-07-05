package main

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/exemplar"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// SessionMetrics holds the metrics for a database session
type SessionMetrics struct {
	queryDuration   metric.Float64Histogram
	queryCount      metric.Int64Counter
	rowsReturned    metric.Int64Histogram
	activeQueries   metric.Int64UpDownCounter
	errorRate       metric.Float64Counter
	cacheHitRate    metric.Float64Histogram
}

// SessionTelemetry encapsulates metrics and tracing for the session
type SessionTelemetry struct {
	metrics  *SessionMetrics
	tracer   trace.Tracer
	enabled  bool
}

// InitSessionTelemetry initializes telemetry for spanner-mycli
func InitSessionTelemetry(ctx context.Context, endpoint string) (*SessionTelemetry, func(), error) {
	if endpoint == "" {
		// Telemetry disabled
		return &SessionTelemetry{enabled: false}, func() {}, nil
	}

	// Create OTLP metric exporter
	exporter, err := otlpmetrichttp.New(ctx,
		otlpmetrichttp.WithEndpoint(endpoint),
		otlpmetrichttp.WithInsecure(), // For local development
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
	}

	// Create resource
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("spanner-mycli"),
			semconv.ServiceVersion("v0.20.0"),
			attribute.String("environment", getEnvironment()),
		),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create meter provider with exemplar support
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(
			sdkmetric.NewPeriodicReader(
				exporter,
				sdkmetric.WithInterval(30*time.Second),
			),
		),
		// Use trace-based exemplar filter for production
		sdkmetric.WithExemplarFilter(exemplar.TraceBasedFilter),
	)

	otel.SetMeterProvider(meterProvider)
	meter := meterProvider.Meter("spanner-mycli")

	// Create metrics
	queryDuration, err := meter.Float64Histogram(
		"spanner.query.duration",
		metric.WithDescription("Duration of SQL query execution in milliseconds"),
		metric.WithUnit("ms"),
		metric.WithExplicitBucketBoundaries(0.1, 1, 10, 100, 1000, 10000),
	)
	if err != nil {
		return nil, nil, err
	}

	queryCount, err := meter.Int64Counter(
		"spanner.query.count",
		metric.WithDescription("Number of SQL queries executed"),
		metric.WithUnit("{query}"),
	)
	if err != nil {
		return nil, nil, err
	}

	rowsReturned, err := meter.Int64Histogram(
		"spanner.query.rows_returned",
		metric.WithDescription("Number of rows returned by SQL queries"),
		metric.WithUnit("{row}"),
	)
	if err != nil {
		return nil, nil, err
	}

	activeQueries, err := meter.Int64UpDownCounter(
		"spanner.query.active",
		metric.WithDescription("Number of active queries"),
		metric.WithUnit("{query}"),
	)
	if err != nil {
		return nil, nil, err
	}

	errorRate, err := meter.Float64Counter(
		"spanner.query.errors",
		metric.WithDescription("Number of query errors"),
		metric.WithUnit("{error}"),
	)
	if err != nil {
		return nil, nil, err
	}

	cacheHitRate, err := meter.Float64Histogram(
		"spanner.cache.hit_rate",
		metric.WithDescription("Cache hit rate for query results"),
		metric.WithUnit("%"),
		metric.WithExplicitBucketBoundaries(0, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100),
	)
	if err != nil {
		return nil, nil, err
	}

	cleanup := func() {
		if err := meterProvider.Shutdown(ctx); err != nil {
			fmt.Printf("Error shutting down meter provider: %v\n", err)
		}
	}

	return &SessionTelemetry{
		metrics: &SessionMetrics{
			queryDuration:   queryDuration,
			queryCount:      queryCount,
			rowsReturned:    rowsReturned,
			activeQueries:   activeQueries,
			errorRate:       errorRate,
			cacheHitRate:    cacheHitRate,
		},
		enabled: true,
	}, cleanup, nil
}

// RecordQuery records metrics for a query execution
func (st *SessionTelemetry) RecordQuery(ctx context.Context, queryType string, startTime time.Time, rowCount int64, err error) {
	if !st.enabled {
		return
	}

	duration := time.Since(startTime)
	attrs := []attribute.KeyValue{
		attribute.String("query.type", queryType),
		attribute.String("project_id", getProjectFromContext(ctx)),
		attribute.String("instance_id", getInstanceFromContext(ctx)),
		attribute.String("database_id", getDatabaseFromContext(ctx)),
		attribute.Bool("has_error", err != nil),
	}

	// Add error details if present
	if err != nil {
		attrs = append(attrs, 
			attribute.String("error.type", getErrorType(err)),
			attribute.String("error.message", err.Error()),
		)
		st.metrics.errorRate.Add(ctx, 1, metric.WithAttributes(attrs...))
	}

	// Record metrics
	st.metrics.queryDuration.Record(ctx, duration.Seconds()*1000, metric.WithAttributes(attrs...))
	st.metrics.queryCount.Add(ctx, 1, metric.WithAttributes(attrs...))
	st.metrics.rowsReturned.Record(ctx, rowCount, metric.WithAttributes(attrs...))
}

// TrackActiveQuery tracks an active query
func (st *SessionTelemetry) TrackActiveQuery(ctx context.Context, queryType string) func() {
	if !st.enabled {
		return func() {}
	}

	attrs := metric.WithAttributes(attribute.String("query.type", queryType))
	st.metrics.activeQueries.Add(ctx, 1, attrs)
	
	return func() {
		st.metrics.activeQueries.Add(ctx, -1, attrs)
	}
}

// RecordCacheHit records cache hit rate
func (st *SessionTelemetry) RecordCacheHit(ctx context.Context, cacheType string, hit bool) {
	if !st.enabled {
		return
	}

	hitRate := 0.0
	if hit {
		hitRate = 100.0
	}
	
	st.metrics.cacheHitRate.Record(ctx, hitRate, metric.WithAttributes(
		attribute.String("cache.type", cacheType),
		attribute.Bool("cache.hit", hit),
	))
}

// Helper functions (would be implemented in the actual integration)
func getEnvironment() string {
	// Would read from config or environment variable
	return "production"
}

func getProjectFromContext(ctx context.Context) string {
	// Would extract from session context
	return "example-project"
}

func getInstanceFromContext(ctx context.Context) string {
	// Would extract from session context
	return "example-instance"
}

func getDatabaseFromContext(ctx context.Context) string {
	// Would extract from session context
	return "example-database"
}

func getErrorType(err error) string {
	// Would categorize errors (e.g., "syntax", "permission", "timeout", etc.)
	return "unknown"
}

// Example usage in spanner-mycli
func main() {
	ctx := context.Background()

	// Initialize telemetry (would be done in session setup)
	telemetry, cleanup, err := InitSessionTelemetry(ctx, "localhost:4318")
	if err != nil {
		fmt.Printf("Failed to initialize telemetry: %v\n", err)
		return
	}
	defer cleanup()

	// Example: Recording a query execution
	queryType := "SELECT"
	startTime := time.Now()
	
	// Track active query
	done := telemetry.TrackActiveQuery(ctx, queryType)
	defer done()

	// Simulate query execution
	time.Sleep(100 * time.Millisecond)
	rowCount := int64(42)
	var queryErr error = nil

	// Record query metrics
	telemetry.RecordQuery(ctx, queryType, startTime, rowCount, queryErr)

	// Example: Recording cache hit
	telemetry.RecordCacheHit(ctx, "query_plan", true)

	fmt.Println("Telemetry example completed. Metrics would be exported to the OTLP endpoint.")
}