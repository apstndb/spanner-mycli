package sysvar_test

import (
	"testing"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
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

func TestCreateIntRangeParser(t *testing.T) {
	t.Run("no range", func(t *testing.T) {
		p := sysvar.CreateIntRangeParser(nil)

		// Should accept any valid int64
		testCases := []string{"-1000", "0", "1000", "9223372036854775807"}
		for _, tc := range testCases {
			if _, err := p.ParseAndValidateWithMode(tc, parser.ParseModeSimple); err != nil {
				t.Errorf("ParseAndValidate(%s) failed: %v", tc, err)
			}
		}
	})

	t.Run("with min", func(t *testing.T) {
		min := int64(10)
		p := sysvar.CreateIntRangeParser(&sysvar.RangeParserOptions[int64]{Min: &min})

		// Valid values
		if _, err := p.ParseAndValidateWithMode("10", parser.ParseModeSimple); err != nil {
			t.Errorf("ParseAndValidate(10) failed: %v", err)
		}
		if _, err := p.ParseAndValidateWithMode("100", parser.ParseModeSimple); err != nil {
			t.Errorf("ParseAndValidate(100) failed: %v", err)
		}

		// Invalid value
		if _, err := p.ParseAndValidateWithMode("5", parser.ParseModeSimple); err == nil {
			t.Error("Expected error for value below minimum")
		}
	})

	t.Run("with max", func(t *testing.T) {
		max := int64(100)
		p := sysvar.CreateIntRangeParser(&sysvar.RangeParserOptions[int64]{Max: &max})

		// Valid values
		if _, err := p.ParseAndValidateWithMode("0", parser.ParseModeSimple); err != nil {
			t.Errorf("ParseAndValidate(0) failed: %v", err)
		}
		if _, err := p.ParseAndValidateWithMode("100", parser.ParseModeSimple); err != nil {
			t.Errorf("ParseAndValidate(100) failed: %v", err)
		}

		// Invalid value
		if _, err := p.ParseAndValidateWithMode("200", parser.ParseModeSimple); err == nil {
			t.Error("Expected error for value above maximum")
		}
	})

	t.Run("with range", func(t *testing.T) {
		min := int64(10)
		max := int64(100)
		p := sysvar.CreateIntRangeParser(&sysvar.RangeParserOptions[int64]{Min: &min, Max: &max})

		// Valid values
		testCases := []string{"10", "50", "100"}
		for _, tc := range testCases {
			if _, err := p.ParseAndValidateWithMode(tc, parser.ParseModeSimple); err != nil {
				t.Errorf("ParseAndValidate(%s) failed: %v", tc, err)
			}
		}

		// Invalid values
		invalidCases := []string{"5", "200", "-10"}
		for _, tc := range invalidCases {
			if _, err := p.ParseAndValidateWithMode(tc, parser.ParseModeSimple); err == nil {
				t.Errorf("Expected error for value %s", tc)
			}
		}
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

func TestCreateProtobufEnumVariableParser(t *testing.T) {
	var currentPriority sppb.RequestOptions_Priority
	varParser := sysvar.CreateProtobufEnumVariableParser(
		"RPC_PRIORITY",
		"RPC priority",
		sppb.RequestOptions_Priority_value,
		"PRIORITY_",
		func() sppb.RequestOptions_Priority { return currentPriority },
		func(v sppb.RequestOptions_Priority) error { currentPriority = v; return nil },
		func(v sppb.RequestOptions_Priority) string {
			// Strip prefix for display
			fullName := v.String()
			if len(fullName) > 9 && fullName[:9] == "PRIORITY_" {
				return fullName[9:]
			}
			return fullName
		},
	)

	// Test with short name
	if err := varParser.ParseAndSetWithMode("HIGH", parser.ParseModeSimple); err != nil {
		t.Fatalf("ParseAndSetWithMode(HIGH) failed: %v", err)
	}
	if currentPriority != sppb.RequestOptions_PRIORITY_HIGH {
		t.Errorf("Expected PRIORITY_HIGH, got %v", currentPriority)
	}

	// Test with full name
	if err := varParser.ParseAndSetWithMode("PRIORITY_LOW", parser.ParseModeSimple); err != nil {
		t.Fatalf("ParseAndSetWithMode(PRIORITY_LOW) failed: %v", err)
	}
	if currentPriority != sppb.RequestOptions_PRIORITY_LOW {
		t.Errorf("Expected PRIORITY_LOW, got %v", currentPriority)
	}

	// Test formatting
	got, err := varParser.GetValue()
	if err != nil {
		t.Fatalf("GetValue failed: %v", err)
	}
	if got != "LOW" {
		t.Errorf("Expected 'LOW', got %q", got)
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

	t.Run("FormatStringer", func(t *testing.T) {
		priority := sppb.RequestOptions_PRIORITY_HIGH
		if got := sysvar.FormatStringer(priority); got != "PRIORITY_HIGH" {
			t.Errorf("FormatStringer(PRIORITY_HIGH) = %q, want 'PRIORITY_HIGH'", got)
		}
	})
}

func TestVariableBuilder(t *testing.T) {
	var testValue string

	builder := sysvar.NewVariable[string]("TEST_VAR", "Test variable").
		WithParser(parser.DualModeStringParser).
		WithGetter(func() string { return testValue }).
		WithSetter(func(v string) error { testValue = v; return nil }).
		WithFormatter(func(v string) string { return v })

	variable := builder.Build()

	// Test basic properties
	if variable.Name() != "TEST_VAR" {
		t.Errorf("Name() = %q, want 'TEST_VAR'", variable.Name())
	}
	if variable.Description() != "Test variable" {
		t.Errorf("Description() = %q, want 'Test variable'", variable.Description())
	}
	if variable.IsReadOnly() {
		t.Error("Variable should not be read-only")
	}

	// Test set and get
	if err := variable.ParseAndSetWithMode("test value", parser.ParseModeSimple); err != nil {
		t.Fatalf("ParseAndSetWithMode failed: %v", err)
	}
	got, err := variable.GetValue()
	if err != nil {
		t.Fatalf("GetValue failed: %v", err)
	}
	if got != "test value" {
		t.Errorf("GetValue() = %q, want 'test value'", got)
	}

	// Test read-only builder
	roBuilder := sysvar.NewVariable[string]("RO_VAR", "Read-only variable").
		WithParser(parser.DualModeStringParser).
		WithGetter(func() string { return "constant" }).
		WithFormatter(func(v string) string { return v }).
		ReadOnly()

	roVariable := roBuilder.Build()
	if !roVariable.IsReadOnly() {
		t.Error("Variable should be read-only")
	}
	if err := roVariable.ParseAndSetWithMode("new value", parser.ParseModeSimple); err == nil {
		t.Error("Expected error when setting read-only variable")
	}
}
