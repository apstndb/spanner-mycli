package sysvar

import (
	"errors"
	"testing"
	"time"
)

// TestIntParserWithMinMax tests WithMin and WithMax methods
func TestIntParserWithMinMax(t *testing.T) {
	t.Run("WithMin", func(t *testing.T) {
		p := NewIntParser().WithMin(10)

		// Valid value
		got, err := p.ParseAndValidate("15")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if got != 15 {
			t.Errorf("Expected 15, got %d", got)
		}

		// Below minimum
		_, err = p.ParseAndValidate("5")
		if err == nil {
			t.Error("Expected error for value below minimum")
		}
	})

	t.Run("WithMax", func(t *testing.T) {
		p := NewIntParser().WithMax(100)

		// Valid value
		got, err := p.ParseAndValidate("50")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if got != 50 {
			t.Errorf("Expected 50, got %d", got)
		}

		// Above maximum
		_, err = p.ParseAndValidate("150")
		if err == nil {
			t.Error("Expected error for value above maximum")
		}
	})
}

// TestDurationParserWithRange tests WithRange and WithMax methods
func TestDurationParserWithRange(t *testing.T) {
	t.Run("WithRange", func(t *testing.T) {
		p := NewDurationParser().WithRange(1*time.Second, 10*time.Second)

		// Valid value
		got, err := p.ParseAndValidate("5s")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if got != 5*time.Second {
			t.Errorf("Expected 5s, got %v", got)
		}

		// Below minimum
		_, err = p.ParseAndValidate("500ms")
		if err == nil {
			t.Error("Expected error for duration below minimum")
		}

		// Above maximum
		_, err = p.ParseAndValidate("15s")
		if err == nil {
			t.Error("Expected error for duration above maximum")
		}
	})

	t.Run("WithMax", func(t *testing.T) {
		p := NewDurationParser().WithMax(1 * time.Hour)

		// Valid value
		got, err := p.ParseAndValidate("30m")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if got != 30*time.Minute {
			t.Errorf("Expected 30m, got %v", got)
		}

		// Above maximum
		_, err = p.ParseAndValidate("2h")
		if err == nil {
			t.Error("Expected error for duration above maximum")
		}
	})
}

// TestStringParserWithLengthRange tests string validation
func TestStringParserWithLengthRange(t *testing.T) {
	p := NewStringParser().WithLengthRange(3, 10)

	// Valid length
	got, err := p.ParseAndValidate("hello")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if got != "hello" {
		t.Errorf("Expected 'hello', got %q", got)
	}

	// Too short
	_, err = p.ParseAndValidate("hi")
	if err == nil {
		t.Error("Expected error for string too short")
	}

	// Too long
	_, err = p.ParseAndValidate("this is too long")
	if err == nil {
		t.Error("Expected error for string too long")
	}
}

// TestEnumStringParser tests NewEnumStringParser
func TestEnumStringParser(t *testing.T) {
	p := NewEnumStringParser("ACTIVE", "INACTIVE", "PENDING")

	// Valid value
	got, err := p.ParseAndValidate("ACTIVE")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if got != "ACTIVE" {
		t.Errorf("Expected 'ACTIVE', got %q", got)
	}

	// Case insensitive
	got2, err := p.ParseAndValidate("pending")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if got2 != "PENDING" {
		t.Errorf("Expected 'PENDING', got %q", got2)
	}

	// Invalid value
	_, err = p.ParseAndValidate("UNKNOWN")
	if err == nil {
		t.Error("Expected error for invalid enum value")
	}
}

// TestEnumIntParser tests NewEnumIntParser
func TestEnumIntParser(t *testing.T) {
	values := map[string]int{
		"LOW":    1,
		"MEDIUM": 5,
		"HIGH":   10,
	}
	p := NewEnumIntParser(values)

	// Valid value
	got, err := p.ParseAndValidate("HIGH")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if got != 10 {
		t.Errorf("Expected 10, got %d", got)
	}

	// Invalid value
	_, err = p.ParseAndValidate("EXTREME")
	if err == nil {
		t.Error("Expected error for invalid enum value")
	}
}

// TestRegistryGetAll tests the GetAll method
func TestRegistryGetAll(t *testing.T) {
	registry := NewRegistry()

	// Register some parsers
	p1 := NewBooleanParser("TEST_BOOL", "Test boolean", func() bool { return true }, nil)
	p2 := NewStringVariableParser("TEST_STRING", "Test string", func() string { return "test" }, nil)

	if err := registry.Register(p1); err != nil {
		t.Fatalf("Failed to register p1: %v", err)
	}
	if err := registry.Register(p2); err != nil {
		t.Fatalf("Failed to register p2: %v", err)
	}

	// Get all parsers
	all := registry.GetAll()
	if len(all) != 2 {
		t.Errorf("Expected 2 parsers, got %d", len(all))
	}

	// Check both are present
	if _, ok := all["TEST_BOOL"]; !ok {
		t.Error("TEST_BOOL not found in GetAll result")
	}
	if _, ok := all["TEST_STRING"]; !ok {
		t.Error("TEST_STRING not found in GetAll result")
	}
}

// TestNullableIntVariableParser tests NewNullableIntVariableParser
func TestNullableIntVariableParser(t *testing.T) {
	var value *int64
	min := int64(0)
	max := int64(100)

	p := NewNullableIntVariableParser(
		"TEST_NULLABLE_INT",
		"Test nullable int",
		func() *int64 { return value },
		func(v *int64) error { value = v; return nil },
		&min, &max,
	)

	// Test NULL value
	if err := p.ParseAndSetWithMode("NULL", ParseModeSimple); err != nil {
		t.Fatalf("Failed to set NULL: %v", err)
	}
	if value != nil {
		t.Error("Expected nil value after setting NULL")
	}

	// Test valid integer
	if err := p.ParseAndSetWithMode("50", ParseModeSimple); err != nil {
		t.Fatalf("Failed to set 50: %v", err)
	}
	if value == nil || *value != 50 {
		t.Errorf("Expected 50, got %v", value)
	}

	// Test range validation
	if err := p.ParseAndSetWithMode("150", ParseModeSimple); err == nil {
		t.Error("Expected error for value above maximum")
	}
}

// TestCreateStringEnumVariableParserFunc tests CreateStringEnumVariableParser
func TestCreateStringEnumVariableParserFunc(t *testing.T) {
	currentValue := "INFO"

	p := CreateStringEnumVariableParser[string](
		"LOG_LEVEL",
		"Log level",
		map[string]string{
			"DEBUG": "DEBUG",
			"INFO":  "INFO",
			"WARN":  "WARN",
			"ERROR": "ERROR",
		},
		func() string { return currentValue },
		func(v string) error { currentValue = v; return nil },
	)

	// Test setting value
	if err := p.ParseAndSetWithMode("WARN", ParseModeSimple); err != nil {
		t.Fatalf("Failed to set WARN: %v", err)
	}
	if currentValue != "WARN" {
		t.Errorf("Expected WARN, got %s", currentValue)
	}

	// Test getting value
	got, err := p.GetValue()
	if err != nil {
		t.Fatalf("Failed to get value: %v", err)
	}
	if got != "WARN" {
		t.Errorf("Expected WARN, got %s", got)
	}
}

// TestNullableParserMethods tests Parse and Validate methods on nullable parser
func TestNullableParserMethods(t *testing.T) {
	p := NewNullableParser(DualModeIntParser)

	// Test Parse method
	got, err := p.Parse("42")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if got == nil || *got != 42 {
		t.Errorf("Expected 42, got %v", got)
	}

	// Test Validate method
	val := int64(100)
	if err := p.Validate(&val); err != nil {
		t.Errorf("Validate failed: %v", err)
	}

	// Test Validate with nil
	if err := p.Validate(nil); err != nil {
		t.Errorf("Validate nil failed: %v", err)
	}
}

// TestCreateDualModeParserWithValidation tests error paths
func TestCreateDualModeParserWithValidationErrors(t *testing.T) {
	// Create parser that always fails validation
	validator := func(v int64) error {
		if v > 10 {
			return errors.New("value too large")
		}
		return nil
	}

	p := CreateDualModeParserWithValidation(
		GoogleSQLIntParser,
		NewIntParser(),
		validator,
	)

	// Test validation failure
	_, err := p.ParseAndValidateWithMode("20", ParseModeSimple)
	if err == nil {
		t.Error("Expected validation error")
	}
}

// TestSetFromGoogleSQLError tests error handling in SetFromGoogleSQL
func TestSetFromGoogleSQLError(t *testing.T) {
	registry := NewRegistry()

	// Try to set non-existent variable
	err := registry.SetFromGoogleSQL("NON_EXISTENT", "'value'")
	if err == nil {
		t.Error("Expected error for non-existent variable")
	}
}

// TestParserValidationPropagation tests that validation errors propagate correctly
func TestParserValidationPropagation(t *testing.T) {
	// Create a parser with both parse and validate functions
	p := &baseParser[int]{
		ParseFunc: func(s string) (int, error) {
			if s == "error" {
				return 0, errors.New("parse error")
			}
			return 42, nil
		},
		ValidateFunc: func(v int) error {
			if v == 42 {
				return errors.New("validation error")
			}
			return nil
		},
	}

	// Parse should work
	got, err := p.Parse("ok")
	if err != nil {
		t.Errorf("Parse failed: %v", err)
	}
	if got != 42 {
		t.Errorf("Expected 42, got %d", got)
	}

	// ParseAndValidate should fail on validation
	_, err = p.ParseAndValidate("ok")
	if err == nil {
		t.Error("Expected validation error")
	}
}
