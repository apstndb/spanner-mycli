package mycli

import (
	"testing"
	"time"
)

func TestFormatTimestamp(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		timestamp    time.Time
		defaultValue string
		want         string
	}{
		{
			name:         "non-zero timestamp",
			timestamp:    time.Date(2024, 1, 15, 10, 30, 45, 123456789, time.UTC),
			defaultValue: "NULL",
			want:         "2024-01-15T10:30:45.123456789Z",
		},
		{
			name:         "zero timestamp with empty default",
			timestamp:    time.Time{},
			defaultValue: "",
			want:         "",
		},
		{
			name:         "zero timestamp with NULL default",
			timestamp:    time.Time{},
			defaultValue: "NULL",
			want:         "NULL",
		},
		{
			name:         "zero timestamp with custom default",
			timestamp:    time.Time{},
			defaultValue: "N/A",
			want:         "N/A",
		},
		{
			name:         "timestamp with nanoseconds",
			timestamp:    time.Date(2024, 12, 31, 23, 59, 59, 999999999, time.UTC),
			defaultValue: "",
			want:         "2024-12-31T23:59:59.999999999Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatTimestamp(tt.timestamp, tt.defaultValue)
			if got != tt.want {
				t.Errorf("formatTimestamp() = %v, want %v", got, tt.want)
			}
		})
	}
}
