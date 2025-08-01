package sysvar_test

import (
	"testing"
	"time"

	"github.com/apstndb/spanner-mycli/internal/parser"
	"github.com/apstndb/spanner-mycli/internal/parser/sysvar"
)

func TestRangeParserOptions(t *testing.T) {
	t.Run("HasRange", func(t *testing.T) {
		// No options
		var opts *sysvar.RangeParserOptions[int64]
		if opts.HasRange() {
			t.Error("nil options should not have range")
		}

		// Empty options
		opts = &sysvar.RangeParserOptions[int64]{}
		if opts.HasRange() {
			t.Error("empty options should not have range")
		}

		// With min only
		min := int64(10)
		opts = &sysvar.RangeParserOptions[int64]{Min: &min}
		if !opts.HasRange() {
			t.Error("options with min should have range")
		}

		// With max only
		max := int64(100)
		opts = &sysvar.RangeParserOptions[int64]{Max: &max}
		if !opts.HasRange() {
			t.Error("options with max should have range")
		}

		// With both
		opts = &sysvar.RangeParserOptions[int64]{Min: &min, Max: &max}
		if !opts.HasRange() {
			t.Error("options with min and max should have range")
		}
	})
}

// testParserCases is a helper to test valid and invalid cases for a parser
func testParserCases(t *testing.T, p parser.DualModeParser[int64], validCases, invalidCases []string) {
	t.Helper()
	for _, tc := range validCases {
		if _, err := p.ParseAndValidateWithMode(tc, parser.ParseModeSimple); err != nil {
			t.Errorf("ParseAndValidate(%s) failed: %v", tc, err)
		}
	}
	for _, tc := range invalidCases {
		if _, err := p.ParseAndValidateWithMode(tc, parser.ParseModeSimple); err == nil {
			t.Errorf("Expected error for value %s", tc)
		}
	}
}

func TestCreateIntRangeParser(t *testing.T) {
	t.Run("no range", func(t *testing.T) {
		p := sysvar.CreateIntRangeParser(nil)
		testParserCases(t, p, 
			[]string{"-1000", "0", "1000", "9223372036854775807"},
			nil)
	})

	t.Run("with min", func(t *testing.T) {
		min := int64(10)
		p := sysvar.CreateIntRangeParser(&sysvar.RangeParserOptions[int64]{Min: &min})
		testParserCases(t, p, 
			[]string{"10", "100"},
			[]string{"5"})
	})

	t.Run("with max", func(t *testing.T) {
		max := int64(100)
		p := sysvar.CreateIntRangeParser(&sysvar.RangeParserOptions[int64]{Max: &max})
		testParserCases(t, p,
			[]string{"0", "100"},
			[]string{"200"})
	})

	t.Run("with range", func(t *testing.T) {
		min, max := int64(10), int64(100)
		p := sysvar.CreateIntRangeParser(&sysvar.RangeParserOptions[int64]{Min: &min, Max: &max})
		testParserCases(t, p,
			[]string{"10", "50", "100"},
			[]string{"5", "200", "-10"})
	})
}

func TestCreateDurationRangeParser(t *testing.T) {
	t.Run("with range", func(t *testing.T) {
		min := time.Duration(0)
		max := 500 * time.Millisecond
		p := sysvar.CreateDurationRangeParser(&sysvar.RangeParserOptions[time.Duration]{Min: &min, Max: &max})

		// Valid values
		testCases := []string{"0s", "100ms", "500ms"}
		for _, tc := range testCases {
			if _, err := p.ParseAndValidateWithMode(tc, parser.ParseModeSimple); err != nil {
				t.Errorf("ParseAndValidate(%s) failed: %v", tc, err)
			}
		}

		// Invalid values
		invalidCases := []string{"-1s", "1s", "600ms"}
		for _, tc := range invalidCases {
			if _, err := p.ParseAndValidateWithMode(tc, parser.ParseModeSimple); err == nil {
				t.Errorf("Expected error for value %s", tc)
			}
		}
	})
}

func TestCreateEnumVariableParser(t *testing.T) {
	type TestEnum int
	const (
		TestEnumA TestEnum = iota
		TestEnumB
		TestEnumC
	)

	var currentValue TestEnum
	varParser := sysvar.CreateEnumVariableParser(
		"TEST_ENUM",
		"Test enum variable",
		map[string]TestEnum{
			"A": TestEnumA,
			"B": TestEnumB,
			"C": TestEnumC,
		},
		func() TestEnum { return currentValue },
		func(v TestEnum) error { currentValue = v; return nil },
		func(v TestEnum) string {
			switch v {
			case TestEnumA:
				return "A"
			case TestEnumB:
				return "B"
			case TestEnumC:
				return "C"
			default:
				return "UNKNOWN"
			}
		},
	)

	// Test parsing
	if err := varParser.ParseAndSetWithMode("B", parser.ParseModeSimple); err != nil {
		t.Fatalf("ParseAndSetWithMode failed: %v", err)
	}
	if currentValue != TestEnumB {
		t.Errorf("Expected TestEnumB, got %v", currentValue)
	}

	// Test formatting
	got, err := varParser.GetValue()
	if err != nil {
		t.Fatalf("GetValue failed: %v", err)
	}
	if got != "B" {
		t.Errorf("Expected 'B', got %q", got)
	}

	// Test invalid value
	if err := varParser.ParseAndSetWithMode("D", parser.ParseModeSimple); err == nil {
		t.Error("Expected error for invalid enum value")
	}
}

func TestCreateStringEnumVariableParser(t *testing.T) {
	type LogLevel string
	const (
		LogLevelDebug LogLevel = "DEBUG"
		LogLevelInfo  LogLevel = "INFO"
		LogLevelWarn  LogLevel = "WARN"
		LogLevelError LogLevel = "ERROR"
	)

	currentLevel := LogLevelWarn
	varParser := sysvar.CreateStringEnumVariableParser(
		"LOG_LEVEL",
		"Log level",
		map[string]LogLevel{
			"DEBUG": LogLevelDebug,
			"INFO":  LogLevelInfo,
			"WARN":  LogLevelWarn,
			"ERROR": LogLevelError,
		},
		func() LogLevel { return currentLevel },
		func(v LogLevel) error { currentLevel = v; return nil },
	)

	// Test parsing
	if err := varParser.ParseAndSetWithMode("ERROR", parser.ParseModeSimple); err != nil {
		t.Fatalf("ParseAndSetWithMode failed: %v", err)
	}
	if currentLevel != LogLevelError {
		t.Errorf("Expected LogLevelError, got %v", currentLevel)
	}

	// Test formatting (should return string value as-is)
	got, err := varParser.GetValue()
	if err != nil {
		t.Fatalf("GetValue failed: %v", err)
	}
	if got != "ERROR" {
		t.Errorf("Expected 'ERROR', got %q", got)
	}
}

func TestFormatters(t *testing.T) {
	t.Run("FormatBool", func(t *testing.T) {
		if got := sysvar.FormatBool(true); got != "TRUE" {
			t.Errorf("FormatBool(true) = %q, want 'TRUE'", got)
		}
		if got := sysvar.FormatBool(false); got != "FALSE" {
			t.Errorf("FormatBool(false) = %q, want 'FALSE'", got)
		}
	})

	t.Run("FormatInt", func(t *testing.T) {
		testCases := []struct {
			value int64
			want  string
		}{
			{0, "0"},
			{-42, "-42"},
			{12345, "12345"},
		}
		for _, tc := range testCases {
			if got := sysvar.FormatInt(tc.value); got != tc.want {
				t.Errorf("FormatInt(%d) = %q, want %q", tc.value, got, tc.want)
			}
		}
	})

	t.Run("FormatDuration", func(t *testing.T) {
		testCases := []struct {
			value time.Duration
			want  string
		}{
			{0, "0s"},
			{5 * time.Second, "5s"},
			{time.Hour + 30*time.Minute, "1h30m0s"},
		}
		for _, tc := range testCases {
			if got := sysvar.FormatDuration(tc.value); got != tc.want {
				t.Errorf("FormatDuration(%v) = %q, want %q", tc.value, got, tc.want)
			}
		}
	})

	t.Run("FormatNullable", func(t *testing.T) {
		formatter := sysvar.FormatNullable(sysvar.FormatInt)

		// Test nil
		if got := formatter(nil); got != "NULL" {
			t.Errorf("FormatNullable(nil) = %q, want 'NULL'", got)
		}

		// Test non-nil
		value := int64(42)
		if got := formatter(&value); got != "42" {
			t.Errorf("FormatNullable(&42) = %q, want '42'", got)
		}
	})

}

