package sysvar_test

import (
	"testing"
	"time"

	"github.com/apstndb/spanner-mycli/internal/parser"
	"github.com/apstndb/spanner-mycli/internal/parser/sysvar"
)

type testSystemVariables struct {
	Verbose          bool
	TabWidth         int64
	StatementTimeout *time.Duration
	OptimizerVersion string
	CLIFormat        sysvar.DisplayMode
}

func TestRegistry(t *testing.T) {
	sysVars := &testSystemVariables{
		TabWidth:  8,
		CLIFormat: sysvar.DisplayModeTable,
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
			{"1", false, true},
			{"0", false, true},
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

func TestPredefinedParsers(t *testing.T) {
	t.Run("DisplayModeParser", func(t *testing.T) {
		testCases := []struct {
			input string
			want  sysvar.DisplayMode
		}{
			{"TABLE", sysvar.DisplayModeTable},
			{"table", sysvar.DisplayModeTable}, // Case insensitive
			{"VERTICAL", sysvar.DisplayModeVertical},
			{"CSV", sysvar.DisplayModeCSV},
		}

		for _, tc := range testCases {
			got, err := sysvar.DisplayModeParser.Parse(tc.input)
			if err != nil {
				t.Errorf("Parse(%q) failed: %v", tc.input, err)
				continue
			}
			if got != tc.want {
				t.Errorf("Parse(%q) = %v, want %v", tc.input, got, tc.want)
			}
		}

		// Invalid value
		if _, err := sysvar.DisplayModeParser.Parse("INVALID"); err == nil {
			t.Error("Expected error for invalid display mode")
		}
	})

	t.Run("TimeoutParser", func(t *testing.T) {
		// Valid timeout
		got, err := sysvar.TimeoutParser.Parse("10s")
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		if got != 10*time.Second {
			t.Errorf("Expected 10s, got %v", got)
		}

		// No maximum limit - 2h should be valid
		if _, err := sysvar.TimeoutParser.Parse("2h"); err != nil {
			t.Errorf("Parse(2h) failed: %v", err)
		}

		// Negative value
		if _, err := sysvar.TimeoutParser.ParseAndValidate("-5s"); err == nil {
			t.Error("Expected error for negative timeout")
		}
	})

	t.Run("PortParser", func(t *testing.T) {
		// Valid port
		got, err := sysvar.PortParser.Parse("8080")
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		if got != 8080 {
			t.Errorf("Expected 8080, got %d", got)
		}

		// Valid port 0 (allowed now)
		if _, err := sysvar.PortParser.Parse("0"); err != nil {
			t.Errorf("Parse(0) failed: %v", err)
		}

		// Invalid ports
		testCases := []string{"70000", "-1", "abc"}
		for _, tc := range testCases {
			if _, err := sysvar.PortParser.ParseAndValidate(tc); err == nil {
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
