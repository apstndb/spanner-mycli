package sysvar_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/apstndb/spanner-mycli/internal/parser"
	"github.com/apstndb/spanner-mycli/internal/parser/sysvar"
)

// Test-only parsers
var (
	// timeoutParser parses timeout values (0 or positive).
	timeoutParser = parser.NewDurationParser().WithMin(0)

	// portParser parses port numbers (0-65535).
	portParser = parser.NewIntParser().WithRange(0, 65535)
)

type testSystemVariables struct {
	Verbose          bool
	TabWidth         int64
	StatementTimeout *time.Duration
	OptimizerVersion string
	CLIFormat        string
}

func TestRegistry(t *testing.T) {
	sysVars := &testSystemVariables{
		TabWidth:  8,
		CLIFormat: "TABLE",
	}

	registry := sysvar.NewRegistry()

	// Register boolean variable
	if err := registry.Register(sysvar.NewBooleanParser(
		"CLI_VERBOSE",
		"Enable verbose output",
		func() bool { return sysVars.Verbose },
		func(v bool) error {
			sysVars.Verbose = v
			return nil
		},
	)); err != nil {
		t.Fatalf("Failed to register CLI_VERBOSE: %v", err)
	}

	// Register integer with range
	if err := registry.Register(sysvar.NewIntegerParser(
		"CLI_TAB_WIDTH",
		"Tab width",
		func() int64 { return sysVars.TabWidth },
		func(v int64) error {
			sysVars.TabWidth = v
			return nil
		},
		ptr(int64(1)), ptr(int64(100)),
	)); err != nil {
		t.Fatalf("Failed to register CLI_TAB_WIDTH: %v", err)
	}

	// Register string variable
	if err := registry.Register(sysvar.NewStringParser(
		"OPTIMIZER_VERSION",
		"Optimizer version",
		func() string { return sysVars.OptimizerVersion },
		func(v string) error {
			sysVars.OptimizerVersion = v
			return nil
		},
	)); err != nil {
		t.Fatalf("Failed to register OPTIMIZER_VERSION: %v", err)
	}

	// Test boolean parsing
	t.Run("boolean from Simple mode", func(t *testing.T) {
		if err := registry.SetFromSimple("CLI_VERBOSE", "true"); err != nil {
			t.Fatalf("SetFromCLI failed: %v", err)
		}
		if !sysVars.Verbose {
			t.Error("Expected Verbose to be true")
		}

		// Test various boolean representations
		testCases := []struct {
			value   string
			want    bool
			wantErr bool
		}{
			{"true", true, false},
			{"false", false, false},
			{"TRUE", true, false},
			{"FALSE", false, false},
			{"1", true, false},
			{"0", false, false},
			{"yes", false, true},
			{"no", false, true},
		}

		for _, tc := range testCases {
			err := registry.SetFromSimple("CLI_VERBOSE", tc.value)
			if tc.wantErr {
				if err == nil {
					t.Errorf("SetFromCLI(%q): expected error but got none", tc.value)
				}
			} else {
				if err != nil {
					t.Errorf("SetFromCLI(%q) failed: %v", tc.value, err)
					continue
				}
				if sysVars.Verbose != tc.want {
					t.Errorf("SetFromCLI(%q): got %v, want %v", tc.value, sysVars.Verbose, tc.want)
				}
			}
		}
	})

	t.Run("boolean from GoogleSQL mode", func(t *testing.T) {
		// GoogleSQL mode requires proper boolean literals
		if err := registry.SetFromGoogleSQL("CLI_VERBOSE", "TRUE"); err != nil {
			t.Fatalf("SetFromREPL failed: %v", err)
		}
		if !sysVars.Verbose {
			t.Error("Expected Verbose to be true")
		}

		if err := registry.SetFromGoogleSQL("CLI_VERBOSE", "FALSE"); err != nil {
			t.Fatalf("SetFromREPL failed: %v", err)
		}
		if sysVars.Verbose {
			t.Error("Expected Verbose to be false")
		}
	})

	t.Run("integer with range validation", func(t *testing.T) {
		// Valid value
		if err := registry.SetFromSimple("CLI_TAB_WIDTH", "4"); err != nil {
			t.Fatalf("SetFromCLI failed: %v", err)
		}
		if sysVars.TabWidth != 4 {
			t.Errorf("Expected TabWidth = 4, got %d", sysVars.TabWidth)
		}

		// Below minimum
		if err := registry.SetFromSimple("CLI_TAB_WIDTH", "0"); err == nil {
			t.Error("Expected error for value below minimum")
		}

		// Above maximum
		if err := registry.SetFromSimple("CLI_TAB_WIDTH", "200"); err == nil {
			t.Error("Expected error for value above maximum")
		}
	})

	t.Run("string parsing modes", func(t *testing.T) {
		// Simple mode - no quotes needed
		if err := registry.SetFromSimple("OPTIMIZER_VERSION", "LATEST"); err != nil {
			t.Fatalf("SetFromSimple failed: %v", err)
		}
		if sysVars.OptimizerVersion != "LATEST" {
			t.Errorf("Expected OptimizerVersion = LATEST, got %q", sysVars.OptimizerVersion)
		}

		// GoogleSQL mode - requires quotes
		if err := registry.SetFromGoogleSQL("OPTIMIZER_VERSION", "'5'"); err != nil {
			t.Fatalf("SetFromGoogleSQL failed: %v", err)
		}
		if sysVars.OptimizerVersion != "5" {
			t.Errorf("Expected OptimizerVersion = 5, got %q", sysVars.OptimizerVersion)
		}

		// Test whitespace handling
		if err := registry.SetFromSimple("OPTIMIZER_VERSION", "  spaced  "); err != nil {
			t.Fatalf("SetFromCLI with spaces failed: %v", err)
		}
		// Simple mode preserves the value as-is
		if sysVars.OptimizerVersion != "  spaced  " {
			t.Errorf("Expected spaces to be preserved, got %q", sysVars.OptimizerVersion)
		}
	})

	t.Run("unknown variable", func(t *testing.T) {
		if err := registry.SetFromSimple("UNKNOWN_VAR", "value"); err == nil {
			t.Error("Expected error for unknown variable")
		}
	})
}

func TestNewEnumParser(t *testing.T) {
	type Priority int
	const (
		PriorityLow Priority = iota
		PriorityMedium
		PriorityHigh
	)

	currentPriority := PriorityLow
	enumParser := sysvar.NewEnumParser(
		"PRIORITY",
		"Task priority",
		map[string]Priority{
			"LOW":    PriorityLow,
			"MEDIUM": PriorityMedium,
			"HIGH":   PriorityHigh,
		},
		func() Priority { return currentPriority },
		func(p Priority) error {
			currentPriority = p
			return nil
		},
		func(p Priority) string {
			switch p {
			case PriorityLow:
				return "LOW"
			case PriorityMedium:
				return "MEDIUM"
			case PriorityHigh:
				return "HIGH"
			default:
				return "UNKNOWN"
			}
		},
	)

	// Test basic properties
	if enumParser.Name() != "PRIORITY" {
		t.Errorf("Name() = %q, want %q", enumParser.Name(), "PRIORITY")
	}

	// Test parsing and setting
	if err := enumParser.ParseAndSetWithMode("HIGH", parser.ParseModeSimple); err != nil {
		t.Fatalf("ParseAndSetWithMode failed: %v", err)
	}
	if currentPriority != PriorityHigh {
		t.Errorf("Priority = %v, want %v", currentPriority, PriorityHigh)
	}

	// Test getting value
	value, err := enumParser.GetValue()
	if err != nil {
		t.Fatalf("GetValue failed: %v", err)
	}
	if value != "HIGH" {
		t.Errorf("GetValue() = %q, want %q", value, "HIGH")
	}

	// Test invalid value
	if err := enumParser.ParseAndSetWithMode("INVALID", parser.ParseModeSimple); err == nil {
		t.Error("Expected error for invalid enum value")
	}
}

func TestNewSimpleEnumParser(t *testing.T) {
	type Status string
	const (
		StatusActive   Status = "ACTIVE"
		StatusInactive Status = "INACTIVE"
		StatusPending  Status = "PENDING"
	)

	currentStatus := StatusPending
	simpleEnumParser := sysvar.NewSimpleEnumParser(
		"STATUS",
		"Item status",
		map[string]Status{
			"ACTIVE":   StatusActive,
			"INACTIVE": StatusInactive,
			"PENDING":  StatusPending,
		},
		func() Status { return currentStatus },
		func(s Status) error {
			currentStatus = s
			return nil
		},
	)

	// Test parsing and setting
	if err := simpleEnumParser.ParseAndSetWithMode("ACTIVE", parser.ParseModeSimple); err != nil {
		t.Fatalf("ParseAndSetWithMode failed: %v", err)
	}
	if currentStatus != StatusActive {
		t.Errorf("Status = %v, want %v", currentStatus, StatusActive)
	}

	// Test getting value (should use fmt.Sprint formatter)
	value, err := simpleEnumParser.GetValue()
	if err != nil {
		t.Fatalf("GetValue failed: %v", err)
	}
	if value != "ACTIVE" {
		t.Errorf("GetValue() = %q, want %q", value, "ACTIVE")
	}
}

func TestNewNullableDurationParser(t *testing.T) {
	var timeout *time.Duration
	min := 1 * time.Second
	max := 1 * time.Hour

	nullDurParser := sysvar.NewNullableDurationParser(
		"TIMEOUT",
		"Request timeout",
		func() *time.Duration { return timeout },
		func(d *time.Duration) error {
			timeout = d
			return nil
		},
		&min,
		&max,
	)

	// Test setting NULL
	if err := nullDurParser.ParseAndSetWithMode("NULL", parser.ParseModeSimple); err != nil {
		t.Fatalf("ParseAndSetWithMode(NULL) failed: %v", err)
	}
	if timeout != nil {
		t.Error("Expected timeout to be nil")
	}

	// Test getting NULL value
	value, err := nullDurParser.GetValue()
	if err != nil {
		t.Fatalf("GetValue failed: %v", err)
	}
	if value != "NULL" {
		t.Errorf("GetValue() = %q, want %q", value, "NULL")
	}

	// Test setting valid duration
	if err := nullDurParser.ParseAndSetWithMode("30s", parser.ParseModeSimple); err != nil {
		t.Fatalf("ParseAndSetWithMode(30s) failed: %v", err)
	}
	if timeout == nil || *timeout != 30*time.Second {
		t.Errorf("timeout = %v, want 30s", timeout)
	}

	// Test getting duration value
	value, err = nullDurParser.GetValue()
	if err != nil {
		t.Fatalf("GetValue failed: %v", err)
	}
	if value != "30s" {
		t.Errorf("GetValue() = %q, want %q", value, "30s")
	}

	// Test range validation - too small
	if err := nullDurParser.ParseAndSetWithMode("500ms", parser.ParseModeSimple); err == nil {
		t.Error("Expected error for duration below minimum")
	}

	// Test range validation - too large
	if err := nullDurParser.ParseAndSetWithMode("2h", parser.ParseModeSimple); err == nil {
		t.Error("Expected error for duration above maximum")
	}
}

func TestNewReadOnlyStringParser(t *testing.T) {
	version := "v1.2.3"
	roStringParser := sysvar.NewReadOnlyStringParser(
		"VERSION",
		"Application version",
		func() string { return version },
	)

	// Test basic properties
	if roStringParser.Name() != "VERSION" {
		t.Errorf("Name() = %q, want %q", roStringParser.Name(), "VERSION")
	}

	if !roStringParser.IsReadOnly() {
		t.Error("Expected parser to be read-only")
	}

	// Test getting value
	value, err := roStringParser.GetValue()
	if err != nil {
		t.Fatalf("GetValue failed: %v", err)
	}
	if value != "v1.2.3" {
		t.Errorf("GetValue() = %q, want %q", value, "v1.2.3")
	}

	// Test setting value should fail
	if err := roStringParser.ParseAndSetWithMode("v2.0.0", parser.ParseModeSimple); err == nil {
		t.Error("Expected error when setting read-only variable")
	}
}

func TestNewReadOnlyBooleanParser(t *testing.T) {
	debugMode := true
	roBoolParser := sysvar.NewReadOnlyBooleanParser(
		"DEBUG_MODE",
		"Debug mode status",
		func() bool { return debugMode },
	)

	// Test basic properties
	if !roBoolParser.IsReadOnly() {
		t.Error("Expected parser to be read-only")
	}

	// Test getting value
	value, err := roBoolParser.GetValue()
	if err != nil {
		t.Fatalf("GetValue failed: %v", err)
	}
	if value != "TRUE" {
		t.Errorf("GetValue() = %q, want %q", value, "TRUE")
	}

	// Change underlying value and test again
	debugMode = false
	value, err = roBoolParser.GetValue()
	if err != nil {
		t.Fatalf("GetValue failed: %v", err)
	}
	if value != "FALSE" {
		t.Errorf("GetValue() = %q, want %q", value, "FALSE")
	}

	// Test setting value should fail
	if err := roBoolParser.ParseAndSetWithMode("true", parser.ParseModeSimple); err == nil {
		t.Error("Expected error when setting read-only variable")
	}
}

func TestNewNullableIntParser(t *testing.T) {
	var maxRetries *int64
	min := int64(0)
	max := int64(10)

	nullIntParser := sysvar.NewNullableIntParser(
		"MAX_RETRIES",
		"Maximum retry attempts",
		func() *int64 { return maxRetries },
		func(i *int64) error {
			maxRetries = i
			return nil
		},
		&min,
		&max,
	)

	// Test setting NULL
	if err := nullIntParser.ParseAndSetWithMode("NULL", parser.ParseModeSimple); err != nil {
		t.Fatalf("ParseAndSetWithMode(NULL) failed: %v", err)
	}
	if maxRetries != nil {
		t.Error("Expected maxRetries to be nil")
	}

	// Test getting NULL value
	value, err := nullIntParser.GetValue()
	if err != nil {
		t.Fatalf("GetValue failed: %v", err)
	}
	if value != "NULL" {
		t.Errorf("GetValue() = %q, want %q", value, "NULL")
	}

	// Test setting valid integer
	if err := nullIntParser.ParseAndSetWithMode("5", parser.ParseModeSimple); err != nil {
		t.Fatalf("ParseAndSetWithMode(5) failed: %v", err)
	}
	if maxRetries == nil || *maxRetries != 5 {
		t.Errorf("maxRetries = %v, want 5", maxRetries)
	}

	// Test getting integer value
	value, err = nullIntParser.GetValue()
	if err != nil {
		t.Fatalf("GetValue failed: %v", err)
	}
	if value != "5" {
		t.Errorf("GetValue() = %q, want %q", value, "5")
	}

	// Test range validation - negative
	if err := nullIntParser.ParseAndSetWithMode("-1", parser.ParseModeSimple); err == nil {
		t.Error("Expected error for negative value")
	}

	// Test range validation - too large
	if err := nullIntParser.ParseAndSetWithMode("11", parser.ParseModeSimple); err == nil {
		t.Error("Expected error for value above maximum")
	}
}

func TestPredefinedParsers(t *testing.T) {
	// DisplayMode tests removed as DisplayMode is no longer in the sysvar package

	t.Run("TimeoutParser", func(t *testing.T) {
		// Valid timeout
		got, err := timeoutParser.Parse("10s")
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		if got != 10*time.Second {
			t.Errorf("Expected 10s, got %v", got)
		}

		// No maximum limit - 2h should be valid
		if _, err := timeoutParser.Parse("2h"); err != nil {
			t.Errorf("Parse(2h) failed: %v", err)
		}

		// Negative value
		if _, err := timeoutParser.ParseAndValidate("-5s"); err == nil {
			t.Error("Expected error for negative timeout")
		}
	})

	t.Run("PortParser", func(t *testing.T) {
		// Valid port
		got, err := portParser.Parse("8080")
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		if got != 8080 {
			t.Errorf("Expected 8080, got %d", got)
		}

		// Valid port 0 (allowed now)
		if _, err := portParser.Parse("0"); err != nil {
			t.Errorf("Parse(0) failed: %v", err)
		}

		// Invalid ports
		testCases := []string{"70000", "-1", "abc"}
		for _, tc := range testCases {
			if _, err := portParser.ParseAndValidate(tc); err == nil {
				t.Errorf("Expected error for invalid port %q", tc)
			}
		}
	})
}

func TestDualModeParsing(t *testing.T) {
	t.Run("integer parsing modes", func(t *testing.T) {
		// Simple mode
		got, err := parser.DualModeIntParser.ParseAndValidateWithMode("42", parser.ParseModeSimple)
		if err != nil {
			t.Fatalf("ParseWithMode failed: %v", err)
		}
		if got != 42 {
			t.Errorf("Expected 42, got %d", got)
		}

		// GoogleSQL mode with hex
		got, err = parser.DualModeIntParser.ParseAndValidateWithMode("0x2A", parser.ParseModeGoogleSQL)
		if err != nil {
			t.Fatalf("ParseWithMode hex failed: %v", err)
		}
		if got != 42 {
			t.Errorf("Expected 42 (0x2A), got %d", got)
		}

		// GoogleSQL mode with negative
		got, err = parser.DualModeIntParser.ParseAndValidateWithMode("-42", parser.ParseModeGoogleSQL)
		if err != nil {
			t.Fatalf("ParseWithMode negative failed: %v", err)
		}
		if got != -42 {
			t.Errorf("Expected -42, got %d", got)
		}
	})

	t.Run("string parsing modes", func(t *testing.T) {
		// Simple mode - no quote processing
		got, err := parser.DualModeStringParser.ParseAndValidateWithMode("'quoted'", parser.ParseModeSimple)
		if err != nil {
			t.Fatalf("ParseWithMode failed: %v", err)
		}
		if got != "'quoted'" {
			t.Errorf("Expected quotes to be preserved, got %q", got)
		}

		// GoogleSQL mode - proper string literal
		got, err = parser.DualModeStringParser.ParseAndValidateWithMode("'quoted'", parser.ParseModeGoogleSQL)
		if err != nil {
			t.Fatalf("ParseWithMode GoogleSQL failed: %v", err)
		}
		if got != "quoted" {
			t.Errorf("Expected quotes to be removed, got %q", got)
		}

		// GoogleSQL escape sequences
		got, err = parser.DualModeStringParser.ParseAndValidateWithMode(`'line1\nline2'`, parser.ParseModeGoogleSQL)
		if err != nil {
			t.Fatalf("ParseWithMode escape failed: %v", err)
		}
		if got != "line1\nline2" {
			t.Errorf("Expected escaped newline, got %q", got)
		}
	})
}

func ptr[T any](v T) *T {
	return &v
}

func TestErrVariableReadOnly(t *testing.T) {
	err := &sysvar.ErrVariableReadOnly{Name: "TEST_VAR"}
	expected := "variable TEST_VAR is read-only"
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}
}

func TestTypedVariableParserDescription(t *testing.T) {
	parser := sysvar.NewBooleanParser(
		"TEST_BOOL",
		"Test description",
		func() bool { return true },
		func(bool) error { return nil },
	)

	if parser.Description() != "Test description" {
		t.Errorf("Description() = %q, want %q", parser.Description(), "Test description")
	}
}

func TestNewDurationParser(t *testing.T) {
	duration := 5 * time.Second
	min := 1 * time.Second
	max := 10 * time.Second

	durationParser := sysvar.NewDurationParser(
		"TIMEOUT",
		"Request timeout",
		func() time.Duration { return duration },
		func(d time.Duration) error {
			duration = d
			return nil
		},
		&min,
		&max,
	)

	// Test setting valid duration
	if err := durationParser.ParseAndSetWithMode("3s", parser.ParseModeSimple); err != nil {
		t.Fatalf("ParseAndSetWithMode failed: %v", err)
	}
	if duration != 3*time.Second {
		t.Errorf("duration = %v, want 3s", duration)
	}

	// Test getting value
	value, err := durationParser.GetValue()
	if err != nil {
		t.Fatalf("GetValue failed: %v", err)
	}
	if value != "3s" {
		t.Errorf("GetValue() = %q, want %q", value, "3s")
	}

	// Test range validation - too small
	if err := durationParser.ParseAndSetWithMode("500ms", parser.ParseModeSimple); err == nil {
		t.Error("Expected error for duration below minimum")
	}

	// Test range validation - too large
	if err := durationParser.ParseAndSetWithMode("15s", parser.ParseModeSimple); err == nil {
		t.Error("Expected error for duration above maximum")
	}
}

func TestRegistryHas(t *testing.T) {
	registry := sysvar.NewRegistry()

	// Register a variable
	if err := registry.Register(sysvar.NewBooleanParser(
		"TEST_VAR",
		"Test variable",
		func() bool { return false },
		func(bool) error { return nil },
	)); err != nil {
		t.Fatalf("Failed to register TEST_VAR: %v", err)
	}

	// Test Has with exact case
	if !registry.Has("TEST_VAR") {
		t.Error("Expected Has(\"TEST_VAR\") to return true")
	}

	// Test Has with different case (should still work)
	if !registry.Has("test_var") {
		t.Error("Expected Has(\"test_var\") to return true")
	}

	// Test Has with non-existent variable
	if registry.Has("NON_EXISTENT") {
		t.Error("Expected Has(\"NON_EXISTENT\") to return false")
	}
}

func TestRegistryGet(t *testing.T) {
	registry := sysvar.NewRegistry()

	boolValue := true
	if err := registry.Register(sysvar.NewBooleanParser(
		"TEST_BOOL",
		"Test boolean",
		func() bool { return boolValue },
		func(b bool) error { boolValue = b; return nil },
	)); err != nil {
		t.Fatalf("Failed to register TEST_BOOL: %v", err)
	}

	// Test Get with existing variable
	value, err := registry.Get("TEST_BOOL")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if value != "TRUE" {
		t.Errorf("Get() = %q, want %q", value, "TRUE")
	}

	// Test Get with non-existent variable
	_, err = registry.Get("NON_EXISTENT")
	if err == nil {
		t.Error("Expected error for non-existent variable")
	}
}

func TestAppendableVariableParser(t *testing.T) {
	var files []string
	parser := sysvar.NewProtoDescriptorFileParser(
		"PROTO_FILES",
		"Proto descriptor files",
		func() []string { return files },
		func(f []string) error { files = f; return nil },
		func(f string) error {
			files = append(files, f)
			return nil
		},
		nil, // No validation for test
	)

	registry := sysvar.NewRegistry()
	if err := registry.Register(parser); err != nil {
		t.Fatalf("Failed to register parser: %v", err)
	}

	// Test initial set
	if err := registry.SetFromSimple("PROTO_FILES", "file1.pb,file2.pb"); err != nil {
		t.Fatalf("SetFromSimple failed: %v", err)
	}
	if len(files) != 2 || files[0] != "file1.pb" || files[1] != "file2.pb" {
		t.Errorf("files = %v, want [file1.pb, file2.pb]", files)
	}

	// Test append with GoogleSQL mode
	if err := registry.AppendFromGoogleSQL("PROTO_FILES", "'file3.pb'"); err != nil {
		t.Fatalf("AppendFromGoogleSQL failed: %v", err)
	}
	if len(files) != 3 || files[2] != "file3.pb" {
		t.Errorf("files = %v, want [file1.pb, file2.pb, file3.pb]", files)
	}

	// Test append with Simple mode
	if err := registry.AppendFromSimple("PROTO_FILES", "file4.pb"); err != nil {
		t.Fatalf("AppendFromSimple failed: %v", err)
	}
	if len(files) != 4 || files[3] != "file4.pb" {
		t.Errorf("files = %v, want [file1.pb, file2.pb, file3.pb, file4.pb]", files)
	}

	// Test HasAppendSupport
	if !registry.HasAppendSupport("PROTO_FILES") {
		t.Error("Expected HasAppendSupport to return true for PROTO_FILES")
	}

	// Test HasAppendSupport for non-appendable variable
	if err := registry.Register(sysvar.NewBooleanParser(
		"NON_APPENDABLE",
		"Non-appendable variable",
		func() bool { return false },
		func(bool) error { return nil },
	)); err != nil {
		t.Fatalf("Failed to register NON_APPENDABLE: %v", err)
	}

	if registry.HasAppendSupport("NON_APPENDABLE") {
		t.Error("Expected HasAppendSupport to return false for NON_APPENDABLE")
	}

	// Test HasAppendSupport for non-existent variable
	if registry.HasAppendSupport("NON_EXISTENT") {
		t.Error("Expected HasAppendSupport to return false for non-existent variable")
	}

	// Test append on non-appendable variable
	if err := registry.AppendFromSimple("NON_APPENDABLE", "value"); err == nil {
		t.Error("Expected error when appending to non-appendable variable")
	}

	// Test append on non-existent variable
	if err := registry.AppendFromSimple("NON_EXISTENT", "value"); err == nil {
		t.Error("Expected error when appending to non-existent variable")
	}
}

// Define a mock protobuf-style enum type for testing
type TestProtobufEnum int32

const (
	TestProtobufEnum_UNKNOWN TestProtobufEnum = 0
	TestProtobufEnum_VALUE_A TestProtobufEnum = 1
	TestProtobufEnum_VALUE_B TestProtobufEnum = 2
)

// String implements fmt.Stringer for TestProtobufEnum
func (e TestProtobufEnum) String() string {
	switch e {
	case TestProtobufEnum_UNKNOWN:
		return "TestProtobufEnum_UNKNOWN"
	case TestProtobufEnum_VALUE_A:
		return "TestProtobufEnum_VALUE_A"
	case TestProtobufEnum_VALUE_B:
		return "TestProtobufEnum_VALUE_B"
	default:
		return fmt.Sprintf("TestProtobufEnum(%d)", e)
	}
}

func TestCreateProtobufEnumVariableParserWithAutoFormatter(t *testing.T) {
	// Create enum map for parsing (should use full names)
	enumMap := map[string]int32{
		"TestProtobufEnum_UNKNOWN": int32(TestProtobufEnum_UNKNOWN),
		"TestProtobufEnum_VALUE_A": int32(TestProtobufEnum_VALUE_A),
		"TestProtobufEnum_VALUE_B": int32(TestProtobufEnum_VALUE_B),
	}

	// Current value
	currentValue := TestProtobufEnum_VALUE_A

	// Create parser
	p := sysvar.CreateProtobufEnumVariableParserWithAutoFormatter(
		"TEST_ENUM",
		"Test protobuf enum",
		enumMap,
		"TestProtobufEnum_",
		func() TestProtobufEnum { return currentValue },
		func(v TestProtobufEnum) error { currentValue = v; return nil },
	)

	// Test parsing with short name
	if err := p.ParseAndSetWithMode("VALUE_B", parser.ParseModeSimple); err != nil {
		t.Fatalf("ParseAndSetWithMode failed: %v", err)
	}
	if currentValue != TestProtobufEnum_VALUE_B {
		t.Errorf("Expected TestProtobufEnum_VALUE_B, got %v", currentValue)
	}

	// Test parsing with full name
	if err := p.ParseAndSetWithMode("TestProtobufEnum_UNKNOWN", parser.ParseModeSimple); err != nil {
		t.Fatalf("ParseAndSetWithMode with full name failed: %v", err)
	}
	if currentValue != TestProtobufEnum_UNKNOWN {
		t.Errorf("Expected TestProtobufEnum_UNKNOWN, got %v", currentValue)
	}

	// Test formatting (should strip prefix)
	currentValue = TestProtobufEnum_VALUE_A
	got, err := p.GetValue()
	if err != nil {
		t.Fatalf("GetValue failed: %v", err)
	}
	if got != "VALUE_A" {
		t.Errorf("GetValue() = %q, want %q", got, "VALUE_A")
	}

	// Test invalid value
	if err := p.ParseAndSetWithMode("INVALID", parser.ParseModeSimple); err == nil {
		t.Error("Expected error for invalid enum value")
	}

	// Test formatter with value that doesn't have the prefix
	// This tests the else branch in the formatter function
	// We need to create a custom test for this since normal enums always have the prefix
}

// SimpleEnum for testing enum without expected prefix
type SimpleEnum int32

const (
	SimpleEnum_A SimpleEnum = 1
	SimpleEnum_B SimpleEnum = 2
)

func (e SimpleEnum) String() string {
	switch e {
	case SimpleEnum_A:
		return "A" // No prefix
	case SimpleEnum_B:
		return "B" // No prefix
	default:
		return fmt.Sprintf("SimpleEnum(%d)", e)
	}
}

func TestCreateProtobufEnumVariableParserWithAutoFormatterNoPrefix(t *testing.T) {
	enumMap := map[string]int32{
		"A": int32(SimpleEnum_A),
		"B": int32(SimpleEnum_B),
	}

	currentValue := SimpleEnum_A

	// Create parser with a prefix that won't match
	p := sysvar.CreateProtobufEnumVariableParserWithAutoFormatter(
		"SIMPLE_ENUM",
		"Simple enum without matching prefix",
		enumMap,
		"NonExistent_", // This prefix won't match our values
		func() SimpleEnum { return currentValue },
		func(v SimpleEnum) error { currentValue = v; return nil },
	)

	// Test formatting - should return the value as-is since prefix doesn't match
	got, err := p.GetValue()
	if err != nil {
		t.Fatalf("GetValue failed: %v", err)
	}
	if got != "A" {
		t.Errorf("GetValue() = %q, want %q", got, "A")
	}
}

func TestFormatString(t *testing.T) {
	if got := sysvar.FormatString("hello"); got != "hello" {
		t.Errorf("FormatString(\"hello\") = %q, want %q", got, "hello")
	}
}

func TestNewEnumParserWithFormatter(t *testing.T) {
	type Color int
	const (
		Red Color = iota
		Green
		Blue
	)

	currentColor := Red
	colorParser := sysvar.NewEnumParser(
		"COLOR",
		"Color setting",
		map[string]Color{
			"RED":   Red,
			"GREEN": Green,
			"BLUE":  Blue,
		},
		func() Color { return currentColor },
		func(c Color) error {
			currentColor = c
			return nil
		},
		func(c Color) string {
			switch c {
			case Red:
				return "RED"
			case Green:
				return "GREEN"
			case Blue:
				return "BLUE"
			default:
				return fmt.Sprintf("Color(%d)", c)
			}
		},
	)

	// Test setting value
	if err := colorParser.ParseAndSetWithMode("GREEN", parser.ParseModeSimple); err != nil {
		t.Fatalf("ParseAndSetWithMode failed: %v", err)
	}
	if currentColor != Green {
		t.Errorf("currentColor = %v, want %v", currentColor, Green)
	}

	// Test getting value with formatter
	value, err := colorParser.GetValue()
	if err != nil {
		t.Fatalf("GetValue failed: %v", err)
	}
	if value != "GREEN" {
		t.Errorf("GetValue() = %q, want %q", value, "GREEN")
	}
}

func TestNewSimpleEnumParserWithFormatter(t *testing.T) {
	type Mode string
	const (
		ModeA Mode = "A"
		ModeB Mode = "B"
		ModeC Mode = "C"
	)

	currentMode := ModeA
	modeParser := sysvar.NewSimpleEnumParser(
		"MODE",
		"Operating mode",
		map[string]Mode{
			"A": ModeA,
			"B": ModeB,
			"C": ModeC,
		},
		func() Mode { return currentMode },
		func(m Mode) error {
			currentMode = m
			return nil
		},
	)

	// Test setting value
	if err := modeParser.ParseAndSetWithMode("B", parser.ParseModeSimple); err != nil {
		t.Fatalf("ParseAndSetWithMode failed: %v", err)
	}
	if currentMode != ModeB {
		t.Errorf("currentMode = %v, want %v", currentMode, ModeB)
	}

	// Test getting value (should use fmt.Sprint)
	value, err := modeParser.GetValue()
	if err != nil {
		t.Fatalf("GetValue failed: %v", err)
	}
	if value != "B" {
		t.Errorf("GetValue() = %q, want %q", value, "B")
	}
}

func TestTypedVariableParserReadOnly(t *testing.T) {
	roParser := sysvar.NewReadOnlyStringParser(
		"RO_VAR",
		"Read-only variable",
		func() string { return "readonly value" },
	)

	// Test IsReadOnly
	if !roParser.IsReadOnly() {
		t.Error("Expected IsReadOnly to return true")
	}

	// Test ParseAndSetWithMode should fail
	err := roParser.ParseAndSetWithMode("new value", parser.ParseModeSimple)
	if err == nil {
		t.Error("Expected error when setting read-only variable")
	}

	// Check error type
	var roErr *sysvar.ErrVariableReadOnly
	if !errors.As(err, &roErr) {
		t.Errorf("Expected ErrVariableReadOnly, got %T", err)
	}
}

func TestCreateDurationRangeParserNoRange(t *testing.T) {
	// Test with nil options
	p := sysvar.CreateDurationRangeParser(nil)

	// Should accept any valid duration
	testCases := []string{"1ns", "1h", "999h", "-5s"}
	for _, tc := range testCases {
		if _, err := p.ParseAndValidateWithMode(tc, parser.ParseModeSimple); err != nil {
			t.Errorf("ParseAndValidate(%s) failed: %v", tc, err)
		}
	}
}

func TestRegistryDuplicateRegistration(t *testing.T) {
	registry := sysvar.NewRegistry()

	parser1 := sysvar.NewBooleanParser(
		"DUP_VAR",
		"First parser",
		func() bool { return false },
		func(bool) error { return nil },
	)

	parser2 := sysvar.NewBooleanParser(
		"dup_var", // Different case, but should still conflict
		"Second parser",
		func() bool { return true },
		func(bool) error { return nil },
	)

	// First registration should succeed
	if err := registry.Register(parser1); err != nil {
		t.Fatalf("First registration failed: %v", err)
	}

	// Second registration should fail
	if err := registry.Register(parser2); err == nil {
		t.Error("Expected error for duplicate registration")
	}
}

func TestNewReadOnlyParsers(t *testing.T) {
	t.Run("NewReadOnlyStringParser", func(t *testing.T) {
		value := "test value"
		p := sysvar.NewReadOnlyStringParser(
			"RO_STRING",
			"Read-only string",
			func() string { return value },
		)

		// Test basic properties
		if p.Name() != "RO_STRING" {
			t.Errorf("Name() = %q, want %q", p.Name(), "RO_STRING")
		}

		if !p.IsReadOnly() {
			t.Error("Expected IsReadOnly to return true")
		}

		// Test GetValue
		got, err := p.GetValue()
		if err != nil {
			t.Fatalf("GetValue failed: %v", err)
		}
		if got != "test value" {
			t.Errorf("GetValue() = %q, want %q", got, "test value")
		}

		// Test setting should fail
		if err := p.ParseAndSetWithMode("new", parser.ParseModeSimple); err == nil {
			t.Error("Expected error when setting read-only variable")
		}
	})

	t.Run("NewReadOnlyBooleanParser", func(t *testing.T) {
		value := true
		p := sysvar.NewReadOnlyBooleanParser(
			"RO_BOOL",
			"Read-only boolean",
			func() bool { return value },
		)

		// Test basic properties
		if !p.IsReadOnly() {
			t.Error("Expected IsReadOnly to return true")
		}

		// Test GetValue
		got, err := p.GetValue()
		if err != nil {
			t.Fatalf("GetValue failed: %v", err)
		}
		if got != "TRUE" {
			t.Errorf("GetValue() = %q, want %q", got, "TRUE")
		}

		// Change underlying value
		value = false
		got, err = p.GetValue()
		if err != nil {
			t.Fatalf("GetValue failed: %v", err)
		}
		if got != "FALSE" {
			t.Errorf("GetValue() = %q, want %q", got, "FALSE")
		}

		// Test setting should fail
		if err := p.ParseAndSetWithMode("true", parser.ParseModeSimple); err == nil {
			t.Error("Expected error when setting read-only variable")
		}
	})
}
