package main

import (
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
)

func TestSystemVariablesRegistry_BasicOperations(t *testing.T) {
	// Create system variables with registry
	sv := newSystemVariablesWithDefaults()
	sv.initializeRegistry()

	t.Run("CLI_FORMAT setting", func(t *testing.T) {
		// Test setting via SetFromSimple
		err := sv.SetFromSimple("CLI_FORMAT", "VERTICAL")
		if err != nil {
			t.Fatalf("Failed to set CLI_FORMAT: %v", err)
		}

		// Check the actual struct field
		if sv.CLIFormat != DisplayModeVertical {
			t.Errorf("CLIFormat = %v (%[1]T), want %v (%[2]T)", sv.CLIFormat, DisplayModeVertical)
		}

		// Test via Get
		result, err := sv.Get("CLI_FORMAT")
		if err != nil {
			t.Fatalf("Failed to get CLI_FORMAT: %v", err)
		}
		if result["CLI_FORMAT"] != "VERTICAL" {
			t.Errorf("Get(CLI_FORMAT) = %v, want VERTICAL", result["CLI_FORMAT"])
		}
	})

	t.Run("AUTOCOMMIT_DML_MODE setting", func(t *testing.T) {
		// Test setting via SetFromSimple
		err := sv.SetFromSimple("AUTOCOMMIT_DML_MODE", "PARTITIONED_NON_ATOMIC")
		if err != nil {
			t.Fatalf("Failed to set AUTOCOMMIT_DML_MODE: %v", err)
		}

		// Check the actual struct field
		if sv.AutocommitDMLMode != AutocommitDMLModePartitionedNonAtomic {
			t.Errorf("AutocommitDMLMode = %v (%[1]T), want %v (%[2]T)", sv.AutocommitDMLMode, AutocommitDMLModePartitionedNonAtomic)
		}

		// Test via Get
		result, err := sv.Get("AUTOCOMMIT_DML_MODE")
		if err != nil {
			t.Fatalf("Failed to get AUTOCOMMIT_DML_MODE: %v", err)
		}
		if result["AUTOCOMMIT_DML_MODE"] != "PARTITIONED_NON_ATOMIC" {
			t.Errorf("Get(AUTOCOMMIT_DML_MODE) = %v, want PARTITIONED_NON_ATOMIC", result["AUTOCOMMIT_DML_MODE"])
		}
	})

	t.Run("RPC_PRIORITY setting", func(t *testing.T) {
		// Test setting via SetFromSimple
		err := sv.SetFromSimple("RPC_PRIORITY", "HIGH")
		if err != nil {
			t.Fatalf("Failed to set RPC_PRIORITY: %v", err)
		}

		// Test via Get
		result, err := sv.Get("RPC_PRIORITY")
		if err != nil {
			t.Fatalf("Failed to get RPC_PRIORITY: %v", err)
		}
		if result["RPC_PRIORITY"] != "HIGH" {
			t.Errorf("Get(RPC_PRIORITY) = %v, want HIGH", result["RPC_PRIORITY"])
		}
	})
}

func TestInitializeSystemVariables_SetValues(t *testing.T) {
	// Test that --set values are properly applied during initialization
	opts := &spannerOptions{
		ProjectId:  "p",
		InstanceId: "i",
		DatabaseId: "d",
		Execute:    "SELECT 1",
		Set: map[string]string{
			"CLI_FORMAT":          "VERTICAL",
			"RPC_PRIORITY":        "HIGH",
			"AUTOCOMMIT_DML_MODE": "PARTITIONED_NON_ATOMIC",
		},
	}

	sysVars, err := initializeSystemVariables(opts)
	if err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	// Debug: Check if registry is initialized
	if sysVars.Registry == nil {
		t.Error("Registry is nil!")
	} else {
		// Registry is initialized properly
		if sysVars.Registry.Has("CLI_FORMAT") {
			t.Log("Registry has CLI_FORMAT")
		} else {
			t.Error("Registry does NOT have CLI_FORMAT")
		}
	}

	t.Run("CLI_FORMAT from --set", func(t *testing.T) {
		if sysVars.CLIFormat != DisplayModeVertical {
			t.Errorf("CLIFormat = %v (%[1]T), want %v (%[2]T)", sysVars.CLIFormat, DisplayModeVertical)
		}
	})

	t.Run("RPC_PRIORITY from --set", func(t *testing.T) {
		// Note: This was changed to use SetFromSimple, so it should work
		expected := sppb.RequestOptions_PRIORITY_HIGH
		if sysVars.RPCPriority != expected {
			t.Errorf("RPCPriority = %v, want %v", sysVars.RPCPriority, expected)
		}
	})

	t.Run("AUTOCOMMIT_DML_MODE from --set", func(t *testing.T) {
		if sysVars.AutocommitDMLMode != AutocommitDMLModePartitionedNonAtomic {
			t.Errorf("AutocommitDMLMode = %v (%[1]T), want %v (%[2]T)", sysVars.AutocommitDMLMode, AutocommitDMLModePartitionedNonAtomic)
		}
	})
}
