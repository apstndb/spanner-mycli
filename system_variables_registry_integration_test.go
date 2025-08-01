package main

import (
	"testing"
)

// TestRegistryIntegration tests the integration of the new parser registry
// with existing system variable functionality
func TestRegistryIntegration(t *testing.T) {
	t.Run("boolean variables through registry", func(t *testing.T) {
		sv := newSystemVariablesWithDefaults()

		// Test READONLY
		if sv.ReadOnly {
			t.Error("ReadOnly should be false by default")
		}

		// Set through registry (GoogleSQL mode)
		if err := sv.Set("READONLY", "TRUE"); err != nil {
			t.Fatalf("Failed to set READONLY: %v", err)
		}
		if !sv.ReadOnly {
			t.Error("ReadOnly should be true after setting")
		}

		// Get through registry
		result, err := sv.Get("READONLY")
		if err != nil {
			t.Fatalf("Failed to get READONLY: %v", err)
		}
		if result["READONLY"] != "TRUE" {
			t.Errorf("Expected TRUE, got %s", result["READONLY"])
		}
	})

	t.Run("new parser rejects old boolean formats", func(t *testing.T) {
		sv := newSystemVariablesWithDefaults()
		sv.initializeRegistry()

		// These should fail with the new parser
		invalidValues := []string{"1", "0", "yes", "no", "on", "off"}
		for _, value := range invalidValues {
			if err := sv.Set("CLI_VERBOSE", value); err == nil {
				t.Errorf("Expected error for invalid boolean value %q", value)
			}
		}

		// These should work
		validValues := []string{"true", "false", "TRUE", "FALSE", "True", "False"}
		for _, value := range validValues {
			if err := sv.Set("CLI_VERBOSE", value); err != nil {
				t.Errorf("Failed to set CLI_VERBOSE with valid value %q: %v", value, err)
			}
		}
	})

	t.Run("unmigrated variables fall back to old system", func(t *testing.T) {
		sv := newSystemVariablesWithDefaults()

		// READ_ONLY_STALENESS is not migrated yet (complex parsing)
		// Using a simple duration value that the old parser accepts
		if err := sv.SetFromSimple("READ_ONLY_STALENESS", "5m"); err != nil {
			t.Fatalf("Failed to set unmigrated variable: %v", err)
		}

		result, err := sv.Get("READ_ONLY_STALENESS")
		if err != nil {
			t.Fatalf("Failed to get unmigrated variable: %v", err)
		}
		// Just check it was set (exact value depends on timestamp parsing)
		if _, ok := result["READ_ONLY_STALENESS"]; !ok {
			t.Errorf("Expected READ_ONLY_STALENESS to be set")
		}
	})
}
