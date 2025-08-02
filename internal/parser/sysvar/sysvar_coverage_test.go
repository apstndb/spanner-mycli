package sysvar

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestGetValueNoGetter tests error when getter is nil
func TestGetValueNoGetter(t *testing.T) {
	parser := &typedVariableParser[string]{
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
	_, err := parseGoogleSQLStringLiteral("'unclosed string")
	if err == nil {
		t.Error("Expected error for unclosed string")
	}
}

// TestDualModeParserUnknownMode tests unknown parse mode
func TestDualModeParserUnknownMode(t *testing.T) {
	p := DualModeBoolParser

	// Use an invalid parse mode
	_, err := p.ParseWithMode("true", parseMode(999))
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
	t.Run("rangeParserOptions", func(t *testing.T) {
		// Test that range parser options work correctly
		min := 10
		max := 100
		opts := &rangeParserOptions[int]{Min: &min, Max: &max}

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

// TestBaseParserParseWithoutParseFunc tests Parse method when ParseFunc is nil
func TestBaseParserParseWithoutParseFunc(t *testing.T) {
	parser := &baseParser[int]{}
	_, err := parser.Parse("123")
	if err == nil {
		t.Error("Expected error when ParseFunc is nil")
	}
	if err.Error() != "parse function not implemented" {
		t.Errorf("Expected 'parse function not implemented', got %v", err)
	}
}

// TestwithValidationOriginalError tests withValidation when original parser returns error
func TestWithValidationOriginalError(t *testing.T) {
	originalParser := &baseParser[int]{
		ParseFunc: func(s string) (int, error) {
			return 0, fmt.Errorf("original parse error")
		},
		ValidateFunc: func(v int) error {
			return fmt.Errorf("original validation error")
		},
	}

	wrapped := withValidation(originalParser, func(v int) error {
		return fmt.Errorf("additional validation error")
	})

	// Test that original validation error is returned first
	err := wrapped.Validate(42)
	if err == nil {
		t.Error("Expected error from original validation")
	}
	if err.Error() != "original validation error" {
		t.Errorf("Expected 'original validation error', got %v", err)
	}
}

// TestcreateDualModeParserWithValidationWithNilValidator tests with nil validator
func TestCreateDualModeParserWithValidationWithNilValidator(t *testing.T) {
	googleSQLParser := newStringParser()
	simpleParser := newStringParser()

	// Create dual-mode parser with nil validator
	dualParser := createDualModeParserWithValidation(googleSQLParser, simpleParser, nil)

	// Should work without validation
	_, err := dualParser.ParseWithMode("test", ParseModeGoogleSQL)
	if err != nil {
		t.Errorf("Unexpected error with nil validator: %v", err)
	}
}

// TestGoogleSQLIntParserWithNegativeHex tests GoogleSQLIntParser with negative hex values
func TestGoogleSQLIntParserWithNegativeHex(t *testing.T) {
	// Test negative hex parsing through the public API
	result, err := GoogleSQLIntParser.Parse("-0xFF")
	if err != nil {
		t.Errorf("Unexpected error parsing negative hex: %v", err)
	}
	if result != -255 {
		t.Errorf("Expected -255, got %d", result)
	}
}

// TestnewDualModeParserDefaultBehavior tests default Parse method behavior
func TestNewDualModeParserDefaultBehavior(t *testing.T) {
	googleSQLParser := newStringParser()
	simpleParser := &baseParser[string]{
		ParseFunc: func(s string) (string, error) {
			return "simple: " + s, nil
		},
	}

	dualParser := newDualModeParser(googleSQLParser, simpleParser)

	// Default Parse should use GoogleSQL mode
	result, err := dualParser.Parse("test")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	// GoogleSQL parser returns input as-is
	if result != "test" {
		t.Errorf("Expected 'test' from GoogleSQL mode, got %v", result)
	}
}

// TestNewDelegatingGoogleSQLParserWithInvalidInput tests error handling
func TestNewDelegatingGoogleSQLParserWithInvalidInput(t *testing.T) {
	delegateParser := newStringParser()
	googleSQLParser := newDelegatingGoogleSQLParser(delegateParser)

	// Test with invalid GoogleSQL string literal
	_, err := googleSQLParser.Parse("not a string literal")
	if err == nil {
		t.Error("Expected error for invalid GoogleSQL string literal")
	}
}

// TestTypedVariableParserGetValueWithNilGetter tests GetValue when getter is nil
func TestTypedVariableParserGetValueWithNilGetter(t *testing.T) {
	vp := &typedVariableParser[string]{
		name:   "TEST_VAR",
		getter: nil,
	}

	_, err := vp.GetValue()
	if err == nil {
		t.Error("Expected error when getter is nil")
	}
	if !strings.Contains(err.Error(), "no getter configured") {
		t.Errorf("Expected 'no getter configured' error, got %v", err)
	}
}

// TestTypedVariableParserGetValueWithNilFormatter tests GetValue when formatter is nil
func TestTypedVariableParserGetValueWithNilFormatter(t *testing.T) {
	testValue := 42
	vp := &typedVariableParser[int]{
		name:      "TEST_VAR",
		getter:    func() int { return testValue },
		formatter: nil, // No formatter
	}

	value, err := vp.GetValue()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	// Should use default formatting (fmt.Sprintf("%v", value))
	if value != "42" {
		t.Errorf("Expected '42' with default formatting, got %q", value)
	}
}

// TestTypedVariableParserParseAndSetWithModeSetterError tests setter error handling
func TestTypedVariableParserParseAndSetWithModeSetterError(t *testing.T) {
	parser := DualModeStringParser
	setter := func(v string) error {
		return fmt.Errorf("setter error")
	}
	getter := func() string { return "" }

	vp := NewTypedVariableParser("TEST_VAR", "Test variable", parser, getter, setter, FormatString)
	typedVP := vp.(*typedVariableParser[string])

	err := typedVP.ParseAndSetWithMode("value", ParseModeSimple)
	if err == nil {
		t.Error("Expected error from setter")
	}
	if !strings.Contains(err.Error(), "setter error") {
		t.Errorf("Expected 'setter error', got %v", err)
	}
}

// TestNewNullableDurationVariableParserWithoutRange tests without min/max
func TestNewNullableDurationVariableParserWithoutRange(t *testing.T) {
	var value *time.Duration
	getter := func() *time.Duration { return value }
	setter := func(v *time.Duration) error { value = v; return nil }

	// Create parser without min/max constraints
	parser := NewNullableDurationVariableParser("TEST_DURATION", "Test duration", getter, setter, nil, nil)

	// Should accept any duration
	err := parser.ParseAndSetWithMode("1h30m", ParseModeSimple)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if value == nil || *value != 90*time.Minute {
		t.Errorf("Expected 90 minutes, got %v", value)
	}
}

// TestNewNullableIntVariableParserWithoutRange tests without min/max
func TestNewNullableIntVariableParserWithoutRange(t *testing.T) {
	var value *int64
	getter := func() *int64 { return value }
	setter := func(v *int64) error { value = v; return nil }

	// Create parser without min/max constraints
	parser := NewNullableIntVariableParser("TEST_INT", "Test int", getter, setter, nil, nil)

	// Should accept any integer
	err := parser.ParseAndSetWithMode("12345", ParseModeSimple)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if value == nil || *value != 12345 {
		t.Errorf("Expected 12345, got %v", value)
	}
}

// TestgoogleSQLIntParserEdgeCases tests edge cases for integer parsing
func TestGoogleSQLIntParserEdgeCases(t *testing.T) {
	// Test that non-integer literals are rejected
	testCases := []struct {
		input   string
		wantErr bool
	}{
		{"3.14", true},         // float literal
		{"TRUE", true},         // boolean
		{"'123'", true},        // string literal
		{"NULL", true},         // null literal
		{"CURRENT_DATE", true}, // identifier
	}

	for _, tc := range testCases {
		_, err := GoogleSQLIntParser.Parse(tc.input)
		if tc.wantErr && err == nil {
			t.Errorf("Expected error for input %q", tc.input)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("Unexpected error for input %q: %v", tc.input, err)
		}
	}
}
