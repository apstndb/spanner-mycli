package main

import (
	"fmt"
	"time"
)

// ExecutionMetrics captures timing and performance metrics for query execution.
// This supports both streaming and buffered modes with unified metrics collection.
//
// Timeline of events:
//
//	QueryStartTime -> [server processing] -> FirstRowTime -> [row iteration] -> LastRowTime -> [cleanup] -> CompletionTime
type ExecutionMetrics struct {
	// Core timing points
	QueryStartTime time.Time  // When executeSQL started (before sending query to server)
	FirstRowTime   *time.Time // Time to First Byte (TTFB) - when first row arrived from server
	LastRowTime    *time.Time // When last row was received/processed by the client
	CompletionTime time.Time  // When entire operation completed (including cleanup)

	// Row statistics
	RowCount int64 // Total number of rows processed

	// Server-side statistics (from QueryStats)
	ServerElapsedTime string // Server-reported elapsed time (actual query execution time on server)
	ServerCPUTime     string // Server-reported CPU time used on server

	// Memory statistics (client-side)
	MemoryBefore *MemoryStats // Memory snapshot before query execution
	MemoryAfter  *MemoryStats // Memory snapshot after query execution

	// Execution mode
	IsStreaming bool // Whether streaming mode was used

	// Profile flag
	Profile bool // Whether profiling is enabled (controls display in template)
}

// TTFB returns the Time to First Byte duration, or nil if not available.
// This measures the time from when the query was sent to when the first row arrived.
// It includes network latency, server query planning, and time to produce the first row.
func (m *ExecutionMetrics) TTFB() *time.Duration {
	if m.FirstRowTime == nil {
		return nil
	}
	ttfb := m.FirstRowTime.Sub(m.QueryStartTime)
	return &ttfb
}

// TotalElapsed returns the total elapsed time for the entire operation.
// This includes everything from query start to completion, including all network transfers,
// server processing, client processing, and cleanup.
func (m *ExecutionMetrics) TotalElapsed() time.Duration {
	return m.CompletionTime.Sub(m.QueryStartTime)
}

// RowIterationTime returns the time spent iterating through rows, or nil if not available.
// This is the duration from when the first row arrived to when the last row was processed.
// Note: This is NOT the actual network transfer time, but rather the time span during which
// rows were being received and processed. It includes both network transfer and client processing
// for each row.
func (m *ExecutionMetrics) RowIterationTime() *time.Duration {
	if m.FirstRowTime == nil || m.LastRowTime == nil {
		return nil
	}
	duration := m.LastRowTime.Sub(*m.FirstRowTime)
	return &duration
}

// ClientOverhead estimates the client-side overhead beyond server processing time.
// This includes network latency, client-side processing, and any buffering.
// Returns nil if server elapsed time is not available.
func (m *ExecutionMetrics) ClientOverhead() *time.Duration {
	if m.ServerElapsedTime == "" {
		return nil
	}

	// Parse server elapsed time
	serverDuration, err := parseServerTime(m.ServerElapsedTime)
	if err != nil {
		return nil
	}

	elapsed := m.CompletionTime.Sub(m.QueryStartTime)
	overhead := elapsed - serverDuration
	return &overhead
}

// MemoryUsedMB returns the memory used during query execution in MB, or -1 if not available.
func (m *ExecutionMetrics) MemoryUsedMB() float64 {
	if m.MemoryBefore == nil || m.MemoryAfter == nil {
		return -1
	}
	return float64(m.MemoryAfter.AllocMB) - float64(m.MemoryBefore.AllocMB)
}

// TotalAllocatedMB returns the total memory allocated during execution in MB, or -1 if not available.
func (m *ExecutionMetrics) TotalAllocatedMB() float64 {
	if m.MemoryBefore == nil || m.MemoryAfter == nil {
		return -1
	}
	return float64(m.MemoryAfter.TotalAllocMB) - float64(m.MemoryBefore.TotalAllocMB)
}

// GCCount returns the number of garbage collections during execution, or -1 if not available.
func (m *ExecutionMetrics) GCCount() int32 {
	if m.MemoryBefore == nil || m.MemoryAfter == nil {
		return -1
	}
	return int32(m.MemoryAfter.NumGC - m.MemoryBefore.NumGC)
}

// parseServerTime parses the server-reported time string into a Duration.
// Server times can be in formats like "10 msec", "1.5 secs", etc.
func parseServerTime(serverTime string) (time.Duration, error) {
	// Handle various formats from QueryStats
	var value float64
	var unit string

	_, err := fmt.Sscanf(serverTime, "%f %s", &value, &unit)
	if err != nil {
		return 0, err
	}

	switch unit {
	case "msec", "msecs":
		return time.Duration(value * float64(time.Millisecond)), nil
	case "sec", "secs":
		return time.Duration(value * float64(time.Second)), nil
	case "usec", "usecs":
		return time.Duration(value * float64(time.Microsecond)), nil
	default:
		return 0, fmt.Errorf("unknown time unit: %s", unit)
	}
}
