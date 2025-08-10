package main

import (
	"fmt"
	"log/slog"
	"runtime"
)

// MemoryStats captures memory usage statistics
type MemoryStats struct {
	AllocMB      uint64 // Allocated memory in MB
	TotalAllocMB uint64 // Total allocated memory in MB
	SysMB        uint64 // System memory in MB
	NumGC        uint32 // Number of GC cycles
}

// GetMemoryStats returns current memory statistics
// Note: runtime.ReadMemStats() causes stop-the-world (STW) pause, 
// so this should only be called when performance profiling is needed
func GetMemoryStats() MemoryStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m) // STW occurs here
	return MemoryStats{
		AllocMB:      m.Alloc / 1024 / 1024,
		TotalAllocMB: m.TotalAlloc / 1024 / 1024,
		SysMB:        m.Sys / 1024 / 1024,
		NumGC:        m.NumGC,
	}
}

// LogMemoryStats logs current memory usage
func LogMemoryStats(label string) {
	stats := GetMemoryStats()
	slog.Debug("Memory stats",
		"label", label,
		"allocMB", stats.AllocMB,
		"totalAllocMB", stats.TotalAllocMB,
		"sysMB", stats.SysMB,
		"numGC", stats.NumGC)
}

// CompareMemoryStats compares two memory snapshots
func CompareMemoryStats(before, after MemoryStats, label string) {
	allocDiff := int64(after.AllocMB) - int64(before.AllocMB)
	totalDiff := int64(after.TotalAllocMB) - int64(before.TotalAllocMB)
	gcDiff := after.NumGC - before.NumGC
	
	slog.Info(fmt.Sprintf("Memory usage for %s", label),
		"allocDiffMB", allocDiff,
		"totalAllocDiffMB", totalDiff,
		"gcCycles", gcDiff)
}