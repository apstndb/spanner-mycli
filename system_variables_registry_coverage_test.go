package main

import (
	"testing"

	"github.com/apstndb/spanner-mycli/internal/parser/sysvar"
)

// TestMustRegisterPanic tests that mustRegister panics on error
func TestMustRegisterPanic(t *testing.T) {
	registry := sysvar.NewRegistry()

	// Register a parser
	parser := sysvar.NewBooleanParser(
		"TEST_VAR",
		"Test variable",
		func() bool { return false },
		func(bool) error { return nil },
	)

	if err := registry.Register(parser); err != nil {
		t.Fatalf("Failed to register parser: %v", err)
	}

	// Try to register the same parser again - should panic
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic when registering duplicate variable")
		}
	}()

	mustRegister(registry, parser)
}

// TestGeneratedRegistration tests that generated registration works
func TestGeneratedRegistration(t *testing.T) {
	sv := newSystemVariablesWithDefaultsForTest()
	registry := createSystemVariableRegistry(sv)

	// Check that some key variables are registered
	vars := []string{
		"AUTO_PARTITION_MODE",
		"RPC_PRIORITY",
		"READONLY",
		"CLI_FORMAT",
		"CLI_VERSION",
	}

	for _, name := range vars {
		if _, err := registry.Get(name); err != nil {
			t.Errorf("Expected variable %s to be registered, but got error: %v", name, err)
		}
	}
}

// TestSystemVariablesCoverage tests various edge cases
func TestSystemVariablesCoverage(t *testing.T) {
	// Create a fresh registry
	registry := sysvar.NewRegistry()

	// Register a read-only variable
	readOnlyVar := sysvar.NewStringVariableParser(
		"TEST_READONLY",
		"Test read-only variable",
		func() string { return "constant" },
		nil, // no setter makes it read-only
	)
	if err := registry.Register(readOnlyVar); err != nil {
		t.Fatalf("Failed to register read-only variable: %v", err)
	}

	// Register a writable variable
	var testValue string
	writableVar := sysvar.NewStringVariableParser(
		"TEST_WRITABLE",
		"Test writable variable",
		func() string { return testValue },
		func(v string) error { testValue = v; return nil },
	)
	if err := registry.Register(writableVar); err != nil {
		t.Fatalf("Failed to register writable variable: %v", err)
	}

	// Test some edge cases for coverage
	t.Run("SetFromGoogleSQL with read-only variable", func(t *testing.T) {
		err := registry.SetFromGoogleSQL("TEST_READONLY", "'1.0.0'")
		if err == nil {
			t.Error("Expected error when setting read-only variable")
		}
	})

	t.Run("SetFromGoogleSQL with invalid syntax", func(t *testing.T) {
		err := registry.SetFromGoogleSQL("TEST_WRITABLE", "invalid syntax")
		if err == nil {
			t.Error("Expected error for invalid GoogleSQL syntax")
		}
	})

	t.Run("SetFromGoogleSQL with valid syntax", func(t *testing.T) {
		err := registry.SetFromGoogleSQL("TEST_WRITABLE", "'valid value'")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if testValue != "valid value" {
			t.Errorf("Expected 'valid value', got %q", testValue)
		}
	})
}
