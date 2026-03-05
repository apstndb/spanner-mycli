package metrics

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestTTFB(t *testing.T) {
	t.Parallel()

	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	firstRow := start.Add(100 * time.Millisecond)

	tests := []struct {
		name string
		m    ExecutionMetrics
		want *time.Duration
	}{
		{
			name: "with first row time",
			m: ExecutionMetrics{
				QueryStartTime: start,
				FirstRowTime:   &firstRow,
			},
			want: durationPtr(100 * time.Millisecond),
		},
		{
			name: "no first row time",
			m: ExecutionMetrics{
				QueryStartTime: start,
				FirstRowTime:   nil,
			},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.m.TTFB()
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("TTFB() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestTotalElapsed(t *testing.T) {
	t.Parallel()

	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	m := ExecutionMetrics{
		QueryStartTime: start,
		CompletionTime: start.Add(500 * time.Millisecond),
	}

	got := m.TotalElapsed()
	want := 500 * time.Millisecond
	if got != want {
		t.Errorf("TotalElapsed() = %v, want %v", got, want)
	}
}

func TestRowIterationTime(t *testing.T) {
	t.Parallel()

	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	firstRow := start.Add(100 * time.Millisecond)
	lastRow := start.Add(300 * time.Millisecond)

	tests := []struct {
		name string
		m    ExecutionMetrics
		want *time.Duration
	}{
		{
			name: "both times available",
			m: ExecutionMetrics{
				FirstRowTime: &firstRow,
				LastRowTime:  &lastRow,
			},
			want: durationPtr(200 * time.Millisecond),
		},
		{
			name: "no first row time",
			m: ExecutionMetrics{
				FirstRowTime: nil,
				LastRowTime:  &lastRow,
			},
			want: nil,
		},
		{
			name: "no last row time",
			m: ExecutionMetrics{
				FirstRowTime: &firstRow,
				LastRowTime:  nil,
			},
			want: nil,
		},
		{
			name: "neither available",
			m:    ExecutionMetrics{},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.m.RowIterationTime()
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("RowIterationTime() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestClientOverhead(t *testing.T) {
	t.Parallel()

	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		m    ExecutionMetrics
		want *time.Duration
	}{
		{
			name: "server time in msec",
			m: ExecutionMetrics{
				QueryStartTime:    start,
				CompletionTime:    start.Add(200 * time.Millisecond),
				ServerElapsedTime: "100 msecs",
			},
			want: durationPtr(100 * time.Millisecond),
		},
		{
			name: "no server time",
			m: ExecutionMetrics{
				QueryStartTime:    start,
				CompletionTime:    start.Add(200 * time.Millisecond),
				ServerElapsedTime: "",
			},
			want: nil,
		},
		{
			name: "invalid server time",
			m: ExecutionMetrics{
				QueryStartTime:    start,
				CompletionTime:    start.Add(200 * time.Millisecond),
				ServerElapsedTime: "invalid",
			},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.m.ClientOverhead()
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("ClientOverhead() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestMemoryUsedMB(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		m    ExecutionMetrics
		want float64
	}{
		{
			name: "with memory stats",
			m: ExecutionMetrics{
				MemoryBefore: &MemoryStats{AllocMB: 10},
				MemoryAfter:  &MemoryStats{AllocMB: 25},
			},
			want: 15,
		},
		{
			name: "no before stats",
			m: ExecutionMetrics{
				MemoryAfter: &MemoryStats{AllocMB: 25},
			},
			want: -1,
		},
		{
			name: "no after stats",
			m: ExecutionMetrics{
				MemoryBefore: &MemoryStats{AllocMB: 10},
			},
			want: -1,
		},
		{
			name: "neither stats",
			m:    ExecutionMetrics{},
			want: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.m.MemoryUsedMB()
			if got != tt.want {
				t.Errorf("MemoryUsedMB() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTotalAllocatedMB(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		m    ExecutionMetrics
		want float64
	}{
		{
			name: "with memory stats",
			m: ExecutionMetrics{
				MemoryBefore: &MemoryStats{TotalAllocMB: 100},
				MemoryAfter:  &MemoryStats{TotalAllocMB: 150},
			},
			want: 50,
		},
		{
			name: "no before stats",
			m: ExecutionMetrics{
				MemoryAfter: &MemoryStats{TotalAllocMB: 150},
			},
			want: -1,
		},
		{
			name: "no after stats",
			m: ExecutionMetrics{
				MemoryBefore: &MemoryStats{TotalAllocMB: 100},
			},
			want: -1,
		},
		{
			name: "no stats",
			m:    ExecutionMetrics{},
			want: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.m.TotalAllocatedMB()
			if got != tt.want {
				t.Errorf("TotalAllocatedMB() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGCCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		m    ExecutionMetrics
		want int32
	}{
		{
			name: "with gc data",
			m: ExecutionMetrics{
				MemoryBefore: &MemoryStats{NumGC: 5},
				MemoryAfter:  &MemoryStats{NumGC: 8},
			},
			want: 3,
		},
		{
			name: "no before stats",
			m: ExecutionMetrics{
				MemoryAfter: &MemoryStats{NumGC: 8},
			},
			want: -1,
		},
		{
			name: "no after stats",
			m: ExecutionMetrics{
				MemoryBefore: &MemoryStats{NumGC: 5},
			},
			want: -1,
		},
		{
			name: "no stats",
			m:    ExecutionMetrics{},
			want: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.m.GCCount()
			if got != tt.want {
				t.Errorf("GCCount() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseServerTime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    time.Duration
		wantErr bool
	}{
		{name: "milliseconds", input: "100 msecs", want: 100 * time.Millisecond},
		{name: "millisecond singular", input: "1 msec", want: 1 * time.Millisecond},
		{name: "seconds", input: "1.5 secs", want: 1500 * time.Millisecond},
		{name: "second singular", input: "2 sec", want: 2 * time.Second},
		{name: "microseconds", input: "500 usecs", want: 500 * time.Microsecond},
		{name: "microsecond singular", input: "1 usec", want: 1 * time.Microsecond},
		{name: "fractional msec", input: "10.5 msecs", want: time.Duration(10.5 * float64(time.Millisecond))},
		{name: "unknown unit", input: "100 hours", wantErr: true},
		{name: "invalid format", input: "invalid", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseServerTime(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("parseServerTime(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestGetMemoryStats(t *testing.T) {
	t.Parallel()

	// Smoke test to verify GetMemoryStats doesn't panic.
	// No assertions on values since SysMB uses integer division (m.Sys / 1024 / 1024)
	// which could be 0 in minimal container environments.
	_ = GetMemoryStats()
}

func durationPtr(d time.Duration) *time.Duration {
	return &d
}
