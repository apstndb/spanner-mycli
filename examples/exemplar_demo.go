package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/exemplar"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// QueryMetrics represents metrics collected for SQL queries
type QueryMetrics struct {
	queryDuration metric.Float64Histogram
	queryCount    metric.Int64Counter
	rowsReturned  metric.Int64Histogram
}

func initializeMetrics() (*QueryMetrics, func(), error) {
	ctx := context.Background()

	// Create stdout exporter for demonstration
	exporter, err := stdoutmetric.New(
		stdoutmetric.WithPrettyPrint(),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create stdout exporter: %w", err)
	}

	// Create resource with only our attributes (avoid schema conflicts)
	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName("spanner-mycli"),
		semconv.ServiceVersion("v0.20.0"),
		attribute.String("environment", "demo"),
	)

	// Create meter provider with exemplar filter that always samples
	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(
			sdkmetric.NewPeriodicReader(
				exporter,
				sdkmetric.WithInterval(5*time.Second),
			),
		),
		// Configure exemplar filter to always sample for demo
		sdkmetric.WithExemplarFilter(exemplar.AlwaysOnFilter),
	)

	// Set as global provider
	otel.SetMeterProvider(provider)

	// Create meter
	meter := provider.Meter("spanner-mycli")

	// Create metrics
	queryDuration, err := meter.Float64Histogram(
		"spanner.query.duration",
		metric.WithDescription("Duration of SQL query execution in milliseconds"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create query duration histogram: %w", err)
	}

	queryCount, err := meter.Int64Counter(
		"spanner.query.count",
		metric.WithDescription("Number of SQL queries executed"),
		metric.WithUnit("{query}"),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create query count counter: %w", err)
	}

	rowsReturned, err := meter.Int64Histogram(
		"spanner.query.rows_returned",
		metric.WithDescription("Number of rows returned by SQL queries"),
		metric.WithUnit("{row}"),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create rows returned histogram: %w", err)
	}

	cleanup := func() {
		if err := provider.Shutdown(ctx); err != nil {
			log.Printf("Error shutting down meter provider: %v", err)
		}
	}

	return &QueryMetrics{
		queryDuration: queryDuration,
		queryCount:    queryCount,
		rowsReturned:  rowsReturned,
	}, cleanup, nil
}

// RecordQueryExecution records metrics for a query execution
func (qm *QueryMetrics) RecordQueryExecution(ctx context.Context, queryType string, duration time.Duration, rowCount int64) {
	// Create attributes that will be attached to the exemplar
	attrs := []attribute.KeyValue{
		attribute.String("query.type", queryType),
		attribute.String("timestamp", time.Now().Format(time.RFC3339Nano)),
	}

	// Record duration with exemplar
	qm.queryDuration.Record(ctx, duration.Seconds()*1000, metric.WithAttributes(attrs...))

	// Record query count
	qm.queryCount.Add(ctx, 1, metric.WithAttributes(attrs...))

	// Record rows returned
	qm.rowsReturned.Record(ctx, rowCount, metric.WithAttributes(attrs...))
}

func main() {
	// Initialize metrics
	metrics, cleanup, err := initializeMetrics()
	if err != nil {
		log.Fatalf("Failed to initialize metrics: %v", err)
	}
	defer cleanup()

	ctx := context.Background()

	// Simulate query executions
	queryTypes := []string{"SELECT", "INSERT", "UPDATE", "DELETE"}
	
	fmt.Println("Starting query simulation...")
	fmt.Println("Metrics will be exported every 5 seconds")
	fmt.Println("Press Ctrl+C to stop")
	fmt.Println()

	// Run for a while to see multiple metric exports
	for i := 0; i < 20; i++ {
		// Random query type
		queryType := queryTypes[rand.Intn(len(queryTypes))]
		
		// Random duration between 10ms and 500ms
		duration := time.Duration(rand.Intn(490)+10) * time.Millisecond
		
		// Random row count
		var rowCount int64
		if queryType == "SELECT" {
			rowCount = int64(rand.Intn(1000))
		} else {
			rowCount = int64(rand.Intn(10))
		}

		// Record the metrics
		metrics.RecordQueryExecution(ctx, queryType, duration, rowCount)

		fmt.Printf("Executed %s query: duration=%v, rows=%d\n", queryType, duration, rowCount)

		// Sleep between queries
		time.Sleep(time.Duration(rand.Intn(1000)+500) * time.Millisecond)
	}

	// Give time for final export
	time.Sleep(6 * time.Second)
	fmt.Println("\nDone!")
}