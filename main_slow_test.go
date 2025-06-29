//go:build !skip_slow_test

package main

import (
	"testing"
)

// TestRun ensure that --embedded-emulator doesn't need ADC.
func TestRun(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping emulator test in short mode")
	}
	
	tests := []struct {
		name    string
		opts    *spannerOptions
		wantErr bool
	}{
		{
			name:    "embedded emulator success",
			opts:    &spannerOptions{Execute: "SELECT 1", EmbeddedEmulator: true, Timeout: "1h"},
			wantErr: false,
		},
		{
			name:    "invalid log level",
			opts:    &spannerOptions{LogLevel: "INVALID_LEVEL"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := run(t.Context(), tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("run() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
