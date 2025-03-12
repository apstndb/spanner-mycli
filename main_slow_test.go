//go:build !skip_slow_test

package main

import (
	"context"
	"testing"
)

// TestRun ensure that --embedded-emulator doesn't need ADC.
func TestRun(t *testing.T) {
	exitCode := run(context.Background(), &spannerOptions{Execute: "SELECT 1", EmbeddedEmulator: true})
	if exitCode != exitCodeSuccess {
		t.Errorf("exitCode != exitCodeSuccess, exitCode: %v", exitCode)
	}
}
