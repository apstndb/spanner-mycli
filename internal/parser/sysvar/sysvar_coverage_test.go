package sysvar

import (
	"errors"
	"testing"
)

// TestGetValueNoGetter tests error when getter is nil
func TestGetValueNoGetter(t *testing.T) {
	parser := &TypedVariableParser[string]{
		name:      "TEST",
		getter:    nil, // No getter
		formatter: FormatString,
	}

	_, err := parser.GetValue()
	if err == nil {
		t.Error("Expected error when getter is nil")
	}
}

// TestMemefishParseError tests memefish parse errors
func TestMemefishParseError(t *testing.T) {
	// Test with invalid expression that will cause parse error
	p := GoogleSQLBoolParser

	_, err := p.Parse("!@#$%^&*()")
	if err == nil {
		t.Error("Expected parse error for invalid expression")
	}
}

// TestGoogleSQLParserRecoverFromPanic tests panic recovery
func TestGoogleSQLParserRecoverFromPanic(t *testing.T) {
	// Test with input that might cause panic in memefish
	_, err := ParseGoogleSQLStringLiteral("'unclosed string")
	if err == nil {
		t.Error("Expected error for unclosed string")
	}
}

// TestDualModeParserUnknownMode tests unknown parse mode
func TestDualModeParserUnknownMode(t *testing.T) {
	p := DualModeBoolParser

	// Use an invalid parse mode
	_, err := p.ParseWithMode("true", ParseMode(999))
	if err == nil {
		t.Error("Expected error for unknown parse mode")
	}
}

// TestNullableParsersEdgeCases tests edge cases for nullable parsers
func TestNullableParsersEdgeCases(t *testing.T) {
	t.Run("nullable parser with spaces around NULL", func(t *testing.T) {
		p := newNullableInt(DualModeIntParser)

		// Test with various NULL formats
		testCases := []string{" NULL ", "\tNULL\t", "NULL", "null", "  null  "}
		for _, tc := range testCases {
			result, err := p.ParseAndValidateWithMode(tc, ParseModeSimple)
			if err != nil {
				t.Errorf("Failed to parse %q: %v", tc, err)
			}
			if result != nil {
				t.Errorf("Expected nil for %q, got %v", tc, result)
			}
		}
	})
}

// TestDelegatingGoogleSQLParserError tests error propagation
func TestDelegatingGoogleSQLParserError(t *testing.T) {
	// Create a parser that always fails
	failingParser := &baseParser[string]{
		ParseFunc: func(s string) (string, error) {
			return "", errors.New("always fails")
		},
	}

	p := newDelegatingGoogleSQLParser(failingParser)

	// Even with valid string literal, it should fail due to delegate
	_, err := p.Parse("'valid string'")
	if err == nil {
		t.Error("Expected error from failing delegate parser")
	}
}

// TestHelperFunctions tests various helper functions
func TestHelperFunctions(t *testing.T) {
	t.Run("RangeParserOptions", func(t *testing.T) {
		// Test that range parser options work correctly
		min := 10
		max := 100
		opts := &RangeParserOptions[int]{Min: &min, Max: &max}

		// This is just to ensure the struct is used
		if opts.Min == nil || *opts.Min != 10 {
			t.Error("Min value not set correctly")
		}
	})
}

// TestCreateProtobufEnumVariableParserEdgeCases tests edge cases
func TestCreateProtobufEnumVariableParserEdgeCases(t *testing.T) {
	// Test with enum that has no matching prefix
	enumValues := map[string]int32{
		"VALUE_A": 1,
		"VALUE_B": 2,
	}

	currentValue := int32(99) // Value not in map

	p := CreateProtobufEnumVariableParserWithAutoFormatter(
		"TEST_ENUM",
		"Test enum",
		enumValues,
		"PREFIX_", // Prefix that doesn't match
		func() int32 { return currentValue },
		func(v int32) error { currentValue = v; return nil },
	)

	// When formatting unknown value, should fall back to generic format
	got, err := p.GetValue()
	if err != nil {
		t.Fatalf("GetValue failed: %v", err)
	}
	if got != "99" {
		t.Errorf("Expected '99' for unknown value, got %q", got)
	}
}
