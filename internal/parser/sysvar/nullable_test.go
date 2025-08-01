package sysvar

import (
	"testing"
	"time"
)

func TestNullableParser(t *testing.T) {
	t.Run("NullableDurationParser", func(t *testing.T) {
		innerParser := DualModeDurationParser
		p := newNullableDuration(innerParser)

		t.Run("Parse NULL in GoogleSQL mode", func(t *testing.T) {
			result, err := p.ParseAndValidateWithMode("NULL", ParseModeGoogleSQL)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			if result != nil {
				t.Errorf("Expected nil, got %v", result)
			}
		})

		t.Run("Parse NULL in Simple mode", func(t *testing.T) {
			result, err := p.ParseAndValidateWithMode("NULL", ParseModeSimple)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			if result != nil {
				t.Errorf("Expected nil, got %v", result)
			}
		})

		t.Run("Parse null (lowercase) in Simple mode", func(t *testing.T) {
			result, err := p.ParseAndValidateWithMode("null", ParseModeSimple)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			if result != nil {
				t.Errorf("Expected nil, got %v", result)
			}
		})

		t.Run("Parse NULL with spaces in Simple mode", func(t *testing.T) {
			result, err := p.ParseAndValidateWithMode("  NULL  ", ParseModeSimple)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			if result != nil {
				t.Errorf("Expected nil, got %v", result)
			}
		})

		t.Run("Parse valid duration in GoogleSQL mode", func(t *testing.T) {
			// GoogleSQL mode expects string literals for duration values
			result, err := p.ParseAndValidateWithMode(`'5s'`, ParseModeGoogleSQL)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			if result == nil {
				t.Fatal("Expected non-nil result")
			}
			if *result != 5*time.Second {
				t.Errorf("Expected 5s, got %v", *result)
			}
		})

		t.Run("Parse valid duration in Simple mode", func(t *testing.T) {
			result, err := p.ParseAndValidateWithMode("5s", ParseModeSimple)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			if result == nil {
				t.Fatal("Expected non-nil result")
			}
			if *result != 5*time.Second {
				t.Errorf("Expected 5s, got %v", *result)
			}
		})

		t.Run("ParseWithMode method", func(t *testing.T) {
			result, err := p.ParseWithMode("10s", ParseModeSimple)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			if result == nil {
				t.Fatal("Expected non-nil result")
			}
			if *result != 10*time.Second {
				t.Errorf("Expected 10s, got %v", *result)
			}
		})

		t.Run("ParseAndValidate method (simple mode)", func(t *testing.T) {
			result, err := p.ParseAndValidate("15s")
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			if result == nil {
				t.Fatal("Expected non-nil result")
			}
			if *result != 15*time.Second {
				t.Errorf("Expected 15s, got %v", *result)
			}
		})

		t.Run("Parse invalid duration", func(t *testing.T) {
			_, err := p.ParseAndValidateWithMode("invalid", ParseModeSimple)
			if err == nil {
				t.Error("Expected error for invalid duration")
			}
		})
	})

	t.Run("NullableIntParser", func(t *testing.T) {
		innerParser := DualModeIntParser
		p := newNullableInt(innerParser)

		t.Run("Parse NULL", func(t *testing.T) {
			result, err := p.ParseAndValidateWithMode("NULL", ParseModeSimple)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			if result != nil {
				t.Errorf("Expected nil, got %v", result)
			}
		})

		t.Run("Parse valid integer", func(t *testing.T) {
			result, err := p.ParseAndValidateWithMode("42", ParseModeSimple)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			if result == nil {
				t.Fatal("Expected non-nil result")
			}
			if *result != 42 {
				t.Errorf("Expected 42, got %v", *result)
			}
		})

		t.Run("Parse negative integer", func(t *testing.T) {
			result, err := p.ParseAndValidateWithMode("-100", ParseModeSimple)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			if result == nil {
				t.Fatal("Expected non-nil result")
			}
			if *result != -100 {
				t.Errorf("Expected -100, got %v", *result)
			}
		})

		t.Run("Parse invalid integer", func(t *testing.T) {
			_, err := p.ParseAndValidateWithMode("not-a-number", ParseModeSimple)
			if err == nil {
				t.Error("Expected error for invalid integer")
			}
		})
	})

	t.Run("Generic NullableParser", func(t *testing.T) {
		// Test with a custom type
		innerParser := DualModeBoolParser
		p := NewNullableParser(innerParser)

		t.Run("Parse NULL", func(t *testing.T) {
			result, err := p.ParseAndValidateWithMode("NULL", ParseModeSimple)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			if result != nil {
				t.Errorf("Expected nil, got %v", result)
			}
		})

		t.Run("Parse valid bool", func(t *testing.T) {
			result, err := p.ParseAndValidateWithMode("true", ParseModeSimple)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			if result == nil {
				t.Fatal("Expected non-nil result")
			}
			if *result != true {
				t.Errorf("Expected true, got %v", *result)
			}
		})
	})
}
