//go:build !skip_slow_test

package main

import (
	"testing"
)

// TestRun ensure that --embedded-emulator doesn't need ADC.
func TestRun(t *testing.T) {
	exitCode := run(t.Context(), &spannerOptions{Execute: "SELECT 1", EmbeddedEmulator: true})
	if exitCode != exitCodeSuccess {
		t.Errorf("exitCode != exitCodeSuccess, exitCode: %v", exitCode)
	}
}
