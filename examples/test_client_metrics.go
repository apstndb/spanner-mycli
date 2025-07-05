package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"cloud.google.com/go/spanner"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	ctx := context.Background()

	// Create metrics collector
	collector, err := NewMetricsCollector()
	if err != nil {
		log.Fatalf("Failed to create metrics collector: %v", err)
	}
	defer collector.Shutdown(ctx)

	// Enable OpenTelemetry metrics
	spanner.EnableOpenTelemetryMetrics()

	// Create client config with our meter provider
	clientConfig := spanner.ClientConfig{
		SessionPoolConfig: spanner.SessionPoolConfig{
			MinOpened: 1,
			MaxOpened: 1,
		},
		OpenTelemetryMeterProvider: collector.MeterProvider(),
	}

	// Get database path from environment or use emulator
	dbPath := os.Getenv("SPANNER_DATABASE")
	if dbPath == "" {
		dbPath = "projects/test-project/instances/test-instance/databases/test-db"
	}

	// Client options
	opts := []option.ClientOption{}
	
	// Use emulator if SPANNER_EMULATOR_HOST is set
	if emulatorHost := os.Getenv("SPANNER_EMULATOR_HOST"); emulatorHost != "" {
		opts = append(opts, 
			option.WithEndpoint(emulatorHost),
			option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
			option.WithoutAuthentication(),
		)
		fmt.Printf("Using emulator at %s\n", emulatorHost)
	}

	// Create Spanner client
	client, err := spanner.NewClientWithConfig(ctx, dbPath, clientConfig, opts...)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Execute a simple query
	fmt.Println("\nExecuting query...")
	collector.Reset()
	
	stmt := spanner.Statement{SQL: "SELECT 1"}
	iter := client.Single().Query(ctx, stmt)
	defer iter.Stop()

	// Consume results
	var count int
	err = iter.Do(func(row *spanner.Row) error {
		count++
		return nil
	})
	
	if err != nil {
		log.Printf("Query error: %v", err)
	}

	// Collect metrics
	metrics, err := collector.Collect(ctx)
	if err != nil {
		log.Fatalf("Failed to collect metrics: %v", err)
	}

	// Display results
	fmt.Printf("\nQuery executed successfully, returned %d row(s)\n", count)
	fmt.Println("\nClient-Side Metrics:")
	fmt.Printf("  GFE Latency:       %.1f ms\n", metrics.GFELatencyMs)
	fmt.Printf("  AFE Latency:       %.1f ms\n", metrics.AFELatencyMs)
	fmt.Printf("  Operation Latency: %.1f ms\n", metrics.OperationLatencyMs)
	fmt.Printf("  Attempt Latency:   %.1f ms\n", metrics.AttemptLatencyMs)
	fmt.Printf("  Attempt Count:     %d\n", metrics.AttemptCount)
	fmt.Printf("  GFE Error Count:   %d\n", metrics.GFEErrorCount)
	fmt.Printf("  AFE Error Count:   %d\n", metrics.AFEErrorCount)

	// Test with error (invalid query)
	fmt.Println("\n\nExecuting invalid query to test error metrics...")
	collector.Reset()
	
	stmt2 := spanner.Statement{SQL: "INVALID SQL"}
	iter2 := client.Single().Query(ctx, stmt2)
	defer iter2.Stop()

	err = iter2.Do(func(row *spanner.Row) error {
		return nil
	})
	
	if err != nil {
		fmt.Printf("Expected error: %v\n", err)
	}

	// Collect metrics after error
	metrics2, err := collector.Collect(ctx)
	if err != nil {
		log.Fatalf("Failed to collect metrics: %v", err)
	}

	fmt.Println("\nClient-Side Metrics (after error):")
	fmt.Printf("  GFE Latency:       %.1f ms\n", metrics2.GFELatencyMs)
	fmt.Printf("  AFE Latency:       %.1f ms\n", metrics2.AFELatencyMs)
	fmt.Printf("  Operation Latency: %.1f ms\n", metrics2.OperationLatencyMs)
	fmt.Printf("  Attempt Latency:   %.1f ms\n", metrics2.AttemptLatencyMs)
	fmt.Printf("  Attempt Count:     %d\n", metrics2.AttemptCount)
	fmt.Printf("  GFE Error Count:   %d\n", metrics2.GFEErrorCount)
	fmt.Printf("  AFE Error Count:   %d\n", metrics2.AFEErrorCount)
}