package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/exemplar"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// QueryMetrics represents metrics collected for SQL queries
type QueryMetrics struct {
	queryDuration   metric.Float64Histogram
	queryCount      metric.Int64Counter
	rowsReturned    metric.Int64Histogram
	activeQueries   metric.Int64UpDownCounter
	errorRate       metric.Float64Counter
}

// CustomExemplarFilter creates a filter that samples based on query characteristics
func CustomExemplarFilter(ctx context.Context) bool {
	// Always include exemplars for:
	// 1. Slow queries (>100ms)
	// 2. Queries with errors
	// 3. Queries with tracing enabled
	// 4. Random 10% sampling for others

	// Check if there's a trace span
	if trace.SpanFromContext(ctx).SpanContext().IsValid() {
		return true
	}

	// Check baggage for error status
	bag := baggage.FromContext(ctx)
	if member := bag.Member("has_error"); member.Value() == "true" {
		return true
	}

	// For now, we'll use random sampling for slow queries
	// (we can't access measurement values in the new API)

	// Random sampling for 10% of other measurements
	return rand.Float64() < 0.1
}

func initializeMetricsAndTracing() (*QueryMetrics, trace.Tracer, func(), error) {
	ctx := context.Background()

	// Create trace exporter
	traceExporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	// Create trace provider
	traceProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("spanner-mycli"),
			semconv.ServiceVersion("v0.20.0"),
		)),
	)
	otel.SetTracerProvider(traceProvider)

	// Create metric exporter
	metricExporter, err := stdoutmetric.New(
		stdoutmetric.WithPrettyPrint(),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create metric exporter: %w", err)
	}

	// Create resource
	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName("spanner-mycli"),
		semconv.ServiceVersion("v0.20.0"),
		attribute.String("environment", "demo"),
	)

	// Create meter provider with custom exemplar filter
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(
			sdkmetric.NewPeriodicReader(
				metricExporter,
				sdkmetric.WithInterval(10*time.Second),
			),
		),
		// Use custom exemplar filter
		sdkmetric.WithExemplarFilter(exemplar.Filter(CustomExemplarFilter)),
	)

	// Set as global provider
	otel.SetMeterProvider(meterProvider)

	// Create meter
	meter := meterProvider.Meter("spanner-mycli")

	// Create metrics
	queryDuration, err := meter.Float64Histogram(
		"spanner.query.duration",
		metric.WithDescription("Duration of SQL query execution in milliseconds"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create query duration histogram: %w", err)
	}

	queryCount, err := meter.Int64Counter(
		"spanner.query.count",
		metric.WithDescription("Number of SQL queries executed"),
		metric.WithUnit("{query}"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create query count counter: %w", err)
	}

	rowsReturned, err := meter.Int64Histogram(
		"spanner.query.rows_returned",
		metric.WithDescription("Number of rows returned by SQL queries"),
		metric.WithUnit("{row}"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create rows returned histogram: %w", err)
	}

	activeQueries, err := meter.Int64UpDownCounter(
		"spanner.query.active",
		metric.WithDescription("Number of active queries"),
		metric.WithUnit("{query}"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create active queries counter: %w", err)
	}

	errorRate, err := meter.Float64Counter(
		"spanner.query.errors",
		metric.WithDescription("Number of query errors"),
		metric.WithUnit("{error}"),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create error rate counter: %w", err)
	}

	cleanup := func() {
		if err := meterProvider.Shutdown(ctx); err != nil {
			log.Printf("Error shutting down meter provider: %v", err)
		}
		if err := traceProvider.Shutdown(ctx); err != nil {
			log.Printf("Error shutting down trace provider: %v", err)
		}
	}

	tracer := traceProvider.Tracer("spanner-mycli")

	return &QueryMetrics{
		queryDuration:   queryDuration,
		queryCount:      queryCount,
		rowsReturned:    rowsReturned,
		activeQueries:   activeQueries,
		errorRate:       errorRate,
	}, tracer, cleanup, nil
}

// ExecuteQuery simulates a query execution with tracing and metrics
func ExecuteQuery(ctx context.Context, tracer trace.Tracer, metrics *QueryMetrics, queryType string, querySQL string) error {
	// Start a trace span
	ctx, span := tracer.Start(ctx, fmt.Sprintf("spanner.query.%s", queryType),
		trace.WithAttributes(
			attribute.String("query.type", queryType),
			attribute.String("query.sql", querySQL),
			attribute.String("db.system", "spanner"),
		),
	)
	defer span.End()

	// Track active queries
	metrics.activeQueries.Add(ctx, 1, metric.WithAttributes(
		attribute.String("query.type", queryType),
	))
	defer metrics.activeQueries.Add(ctx, -1, metric.WithAttributes(
		attribute.String("query.type", queryType),
	))

	// Simulate query execution
	startTime := time.Now()
	
	// Random duration between 10ms and 500ms
	duration := time.Duration(rand.Intn(490)+10) * time.Millisecond
	time.Sleep(duration)
	
	// Simulate occasional errors (10% chance)
	hasError := rand.Float64() < 0.1
	
	// Add error to baggage for exemplar filter
	if hasError {
		member, _ := baggage.NewMember("has_error", "true")
		bag, _ := baggage.New(member)
		ctx = baggage.ContextWithBaggage(ctx, bag)
	}

	// Calculate execution time
	execTime := time.Since(startTime)
	
	// Random row count
	var rowCount int64
	if queryType == "SELECT" {
		rowCount = int64(rand.Intn(1000))
	} else {
		rowCount = int64(rand.Intn(10))
	}

	// Create attributes for metrics
	attrs := []attribute.KeyValue{
		attribute.String("query.type", queryType),
		attribute.String("timestamp", time.Now().Format(time.RFC3339Nano)),
		attribute.Bool("has_error", hasError),
	}

	// Add trace ID if available
	if spanCtx := span.SpanContext(); spanCtx.IsValid() {
		attrs = append(attrs, 
			attribute.String("trace_id", spanCtx.TraceID().String()),
			attribute.String("span_id", spanCtx.SpanID().String()),
		)
	}

	// Record metrics
	metrics.queryDuration.Record(ctx, execTime.Seconds()*1000, metric.WithAttributes(attrs...))
	metrics.queryCount.Add(ctx, 1, metric.WithAttributes(attrs...))
	metrics.rowsReturned.Record(ctx, rowCount, metric.WithAttributes(attrs...))

	if hasError {
		metrics.errorRate.Add(ctx, 1, metric.WithAttributes(attrs...))
		span.RecordError(fmt.Errorf("simulated query error"))
		return fmt.Errorf("query failed: %s", querySQL)
	}

	// Add result attributes to span
	span.SetAttributes(
		attribute.Int64("query.rows_returned", rowCount),
		attribute.Float64("query.duration_ms", execTime.Seconds()*1000),
	)

	return nil
}

func main() {
	// Initialize metrics and tracing
	metrics, tracer, cleanup, err := initializeMetricsAndTracing()
	if err != nil {
		log.Fatalf("Failed to initialize: %v", err)
	}
	defer cleanup()

	ctx := context.Background()

	// Sample queries
	queries := []struct {
		Type string
		SQL  string
	}{
		{"SELECT", "SELECT * FROM users WHERE active = true"},
		{"INSERT", "INSERT INTO logs (timestamp, message) VALUES (?, ?)"},
		{"UPDATE", "UPDATE users SET last_seen = ? WHERE id = ?"},
		{"DELETE", "DELETE FROM sessions WHERE expired < ?"},
		{"SELECT", "SELECT COUNT(*) FROM orders WHERE status = 'pending'"},
	}

	fmt.Println("Starting advanced exemplar demo...")
	fmt.Println("This demo shows:")
	fmt.Println("- Exemplars with trace context")
	fmt.Println("- Custom exemplar filtering based on query characteristics")
	fmt.Println("- Error tracking with exemplars")
	fmt.Println("\nMetrics will be exported every 10 seconds")
	fmt.Println("Press Ctrl+C to stop\n")

	// Run queries
	for i := 0; i < 30; i++ {
		// Pick a random query
		query := queries[rand.Intn(len(queries))]
		
		// Execute with tracing and metrics
		err := ExecuteQuery(ctx, tracer, metrics, query.Type, query.SQL)
		
		if err != nil {
			fmt.Printf("❌ Error executing %s query: %v\n", query.Type, err)
		} else {
			fmt.Printf("✅ Executed %s query successfully\n", query.Type)
		}

		// Sleep between queries
		time.Sleep(time.Duration(rand.Intn(2000)+500) * time.Millisecond)
	}

	// Give time for final export
	fmt.Println("\nWaiting for final metric export...")
	time.Sleep(12 * time.Second)
	fmt.Println("Done!")
}