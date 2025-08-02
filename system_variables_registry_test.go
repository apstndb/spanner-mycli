package main

import (
	"testing"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/internal/parser/sysvar"
)

func TestSystemVariableRegistry(t *testing.T) {
	// Create a system variables instance
	sv := newSystemVariablesWithDefaultsForTest()

	// Test boolean variables through the registry
	t.Run("READONLY", func(t *testing.T) {
		// Test initial value
		if sv.ReadOnly {
			t.Error("ReadOnly should be false by default")
		}

		// Test setting through registry (GoogleSQL mode)
		if err := sv.SetFromGoogleSQL("READONLY", "TRUE"); err != nil {
			t.Fatalf("Failed to set READONLY: %v", err)
		}
		if !sv.ReadOnly {
			t.Error("ReadOnly should be true after setting")
		}

		// Test getting through registry
		result, err := sv.Get("READONLY")
		if err != nil {
			t.Fatalf("Failed to get READONLY: %v", err)
		}
		if result["READONLY"] != "TRUE" {
			t.Errorf("Expected TRUE, got %s", result["READONLY"])
		}

		// Test setting back to false
		if err := sv.SetFromGoogleSQL("READONLY", "FALSE"); err != nil {
			t.Fatalf("Failed to set READONLY to false: %v", err)
		}
		if sv.ReadOnly {
			t.Error("ReadOnly should be false after setting")
		}

		// Test case insensitivity
		if err := sv.SetFromGoogleSQL("readonly", "true"); err != nil {
			t.Fatalf("Failed to set readonly (lowercase): %v", err)
		}
		if !sv.ReadOnly {
			t.Error("ReadOnly should be true after setting with lowercase")
		}
	})

	t.Run("AUTO_PARTITION_MODE", func(t *testing.T) {
		// Test initial value
		if sv.AutoPartitionMode {
			t.Error("AutoPartitionMode should be false by default")
		}

		// Test setting
		if err := sv.SetFromGoogleSQL("AUTO_PARTITION_MODE", "TRUE"); err != nil {
			t.Fatalf("Failed to set AUTO_PARTITION_MODE: %v", err)
		}
		if !sv.AutoPartitionMode {
			t.Error("AutoPartitionMode should be true after setting")
		}

		// Test getting
		result, err := sv.Get("AUTO_PARTITION_MODE")
		if err != nil {
			t.Fatalf("Failed to get AUTO_PARTITION_MODE: %v", err)
		}
		if result["AUTO_PARTITION_MODE"] != "TRUE" {
			t.Errorf("Expected TRUE, got %s", result["AUTO_PARTITION_MODE"])
		}
	})

	t.Run("RETURN_COMMIT_STATS", func(t *testing.T) {
		// Test initial value (should be true by default)
		if !sv.ReturnCommitStats {
			t.Error("ReturnCommitStats should be true by default")
		}

		// Test setting to false
		if err := sv.SetFromGoogleSQL("RETURN_COMMIT_STATS", "FALSE"); err != nil {
			t.Fatalf("Failed to set RETURN_COMMIT_STATS: %v", err)
		}
		if sv.ReturnCommitStats {
			t.Error("ReturnCommitStats should be false after setting")
		}
	})

	t.Run("CLI_VERBOSE", func(t *testing.T) {
		// Test initial value
		if sv.Verbose {
			t.Error("Verbose should be false by default")
		}

		// Test setting
		if err := sv.SetFromGoogleSQL("CLI_VERBOSE", "TRUE"); err != nil {
			t.Fatalf("Failed to set CLI_VERBOSE: %v", err)
		}
		if !sv.Verbose {
			t.Error("Verbose should be true after setting")
		}
	})

	t.Run("Invalid boolean values", func(t *testing.T) {
		// Test invalid values (should fail with new parser)
		testCases := []string{
			"1",
			"0",
			"yes",
			"no",
			"on",
			"off",
			"invalid",
		}

		for _, value := range testCases {
			if err := sv.SetFromGoogleSQL("CLI_VERBOSE", value); err == nil {
				t.Errorf("Expected error for invalid boolean value %q, but got none", value)
			}
		}
	})

	t.Run("Unknown variable", func(t *testing.T) {
		// Test setting unknown variable
		if err := sv.SetFromGoogleSQL("UNKNOWN_VARIABLE", "value"); err == nil {
			t.Error("Expected error for unknown variable")
		}

		// Test getting unknown variable
		if _, err := sv.Get("UNKNOWN_VARIABLE"); err == nil {
			t.Error("Expected error for unknown variable")
		}
	})

	t.Run("Fallback to old system", func(t *testing.T) {
		// Test a variable that's not yet migrated (should fall back to old system)
		// For example, READ_ONLY_STALENESS is a special variable not yet migrated
		if err := sv.SetFromGoogleSQL("READ_ONLY_STALENESS", "STRONG"); err != nil {
			t.Fatalf("Failed to set READ_ONLY_STALENESS (should use old system): %v", err)
		}

		result, err := sv.Get("READ_ONLY_STALENESS")
		if err != nil {
			t.Fatalf("Failed to get READ_ONLY_STALENESS: %v", err)
		}
		if result["READ_ONLY_STALENESS"] != "STRONG" {
			t.Errorf("Expected STRONG, got %s", result["READ_ONLY_STALENESS"])
		}
	})

	// Test integer variables
	t.Run("CLI_TAB_WIDTH", func(t *testing.T) {
		// Test initial value
		if sv.TabWidth != 0 {
			t.Errorf("TabWidth should be 0 by default, got %d", sv.TabWidth)
		}

		// Test setting value
		if err := sv.SetFromGoogleSQL("CLI_TAB_WIDTH", "8"); err != nil {
			t.Errorf("Failed to set CLI_TAB_WIDTH: %v", err)
		}
		if sv.TabWidth != 8 {
			t.Errorf("Expected TabWidth to be 8, got %d", sv.TabWidth)
		}

		// Test getting value
		result, err := sv.Get("CLI_TAB_WIDTH")
		if err != nil {
			t.Errorf("Failed to get CLI_TAB_WIDTH: %v", err)
		}
		if result["CLI_TAB_WIDTH"] != "8" {
			t.Errorf("Expected '8', got %s", result["CLI_TAB_WIDTH"])
		}
	})

	t.Run("MAX_PARTITIONED_PARALLELISM", func(t *testing.T) {
		// Test setting negative value (should be allowed since no constraints)
		if err := sv.SetFromGoogleSQL("MAX_PARTITIONED_PARALLELISM", "-1"); err != nil {
			t.Errorf("Failed to set MAX_PARTITIONED_PARALLELISM: %v", err)
		}
		if sv.MaxPartitionedParallelism != -1 {
			t.Errorf("Expected MaxPartitionedParallelism to be -1, got %d", sv.MaxPartitionedParallelism)
		}

		// Test setting positive value
		if err := sv.SetFromGoogleSQL("MAX_PARTITIONED_PARALLELISM", "100"); err != nil {
			t.Errorf("Failed to set MAX_PARTITIONED_PARALLELISM: %v", err)
		}
		if sv.MaxPartitionedParallelism != 100 {
			t.Errorf("Expected MaxPartitionedParallelism to be 100, got %d", sv.MaxPartitionedParallelism)
		}
	})

	t.Run("Invalid integer values", func(t *testing.T) {
		// Test invalid integer
		if err := sv.SetFromGoogleSQL("CLI_TAB_WIDTH", "not_a_number"); err == nil {
			t.Error("Expected error for invalid integer value")
		}

		// Test floating point (should fail)
		if err := sv.SetFromGoogleSQL("CLI_TAB_WIDTH", "3.14"); err == nil {
			t.Error("Expected error for floating point value")
		}
	})

	// Test string variables
	t.Run("OPTIMIZER_VERSION", func(t *testing.T) {
		// Test setting OPTIMIZER_VERSION (now migrated)
		// In GoogleSQL mode, strings need to be quoted
		if err := sv.SetFromGoogleSQL("OPTIMIZER_VERSION", `"LATEST"`); err != nil {
			t.Errorf("Failed to set OPTIMIZER_VERSION: %v", err)
		}
		if sv.OptimizerVersion != "LATEST" {
			t.Errorf("Expected OptimizerVersion to be LATEST, got %s", sv.OptimizerVersion)
		}

		// Test setting numeric version
		if err := sv.SetFromGoogleSQL("OPTIMIZER_VERSION", `"2"`); err != nil {
			t.Errorf("Failed to set OPTIMIZER_VERSION to numeric: %v", err)
		}
		if sv.OptimizerVersion != "2" {
			t.Errorf("Expected OptimizerVersion to be 2, got %s", sv.OptimizerVersion)
		}
	})

	t.Run("CLI_PROMPT", func(t *testing.T) {
		// Test setting custom prompt - need to use quoted strings in GoogleSQL mode
		customPrompt := "mydb> "
		if err := sv.SetFromGoogleSQL("CLI_PROMPT", `"mydb> "`); err != nil {
			t.Errorf("Failed to set CLI_PROMPT: %v", err)
		}
		if sv.Prompt != customPrompt {
			t.Errorf("Expected Prompt to be %q, got %q", customPrompt, sv.Prompt)
		}

		// Test getting value
		result, err := sv.Get("CLI_PROMPT")
		if err != nil {
			t.Errorf("Failed to get CLI_PROMPT: %v", err)
		}
		if result["CLI_PROMPT"] != customPrompt {
			t.Errorf("Expected %q, got %q", customPrompt, result["CLI_PROMPT"])
		}

		// Test with escape sequences
		// GoogleSQL interprets \t as a tab character
		if err := sv.SetFromGoogleSQL("CLI_PROMPT", `"hello\tworld"`); err != nil {
			t.Errorf("Failed to set CLI_PROMPT with tab: %v", err)
		}
		if sv.Prompt != "hello\tworld" {
			t.Errorf("Expected Prompt to be %q, got %q", "hello\tworld", sv.Prompt)
		}
	})

	// Test enum variables
	t.Run("RPC_PRIORITY", func(t *testing.T) {
		// Test setting RPC_PRIORITY (now migrated)
		// In GoogleSQL mode, enum values must be string literals
		if err := sv.SetFromGoogleSQL("RPC_PRIORITY", "'HIGH'"); err != nil {
			t.Errorf("Failed to set RPC_PRIORITY: %v", err)
		}
		if sv.RPCPriority != sppb.RequestOptions_PRIORITY_HIGH {
			t.Errorf("Expected RPCPriority to be HIGH, got %v", sv.RPCPriority)
		}

		// Test case insensitivity
		if err := sv.SetFromGoogleSQL("RPC_PRIORITY", "'low'"); err != nil {
			t.Errorf("Failed to set RPC_PRIORITY with lowercase: %v", err)
		}
		if sv.RPCPriority != sppb.RequestOptions_PRIORITY_LOW {
			t.Errorf("Expected RPCPriority to be LOW, got %v", sv.RPCPriority)
		}

		// Test invalid value
		if err := sv.SetFromGoogleSQL("RPC_PRIORITY", "'INVALID'"); err == nil {
			t.Error("Expected error for invalid priority value")
		}
	})

	t.Run("CLI_FORMAT", func(t *testing.T) {
		// Test setting display format
		// In GoogleSQL mode, enum values must be string literals
		if err := sv.SetFromGoogleSQL("CLI_FORMAT", "'CSV'"); err != nil {
			t.Errorf("Failed to set CLI_FORMAT: %v", err)
		}
		if sv.CLIFormat != DisplayModeCSV {
			t.Errorf("Expected CLIFormat to be CSV, got %v", sv.CLIFormat)
		}

		// Test another format
		if err := sv.SetFromGoogleSQL("CLI_FORMAT", "'VERTICAL'"); err != nil {
			t.Errorf("Failed to set CLI_FORMAT to VERTICAL: %v", err)
		}
		if sv.CLIFormat != DisplayModeVertical {
			t.Errorf("Expected CLIFormat to be VERTICAL, got %v", sv.CLIFormat)
		}

		// Test invalid format
		if err := sv.SetFromGoogleSQL("CLI_FORMAT", "'INVALID_FORMAT'"); err == nil {
			t.Error("Expected error for invalid format value")
		}
	})

	t.Run("CLI_EXPLAIN_FORMAT", func(t *testing.T) {
		// Test setting explain format
		// In GoogleSQL mode, enum values must be string literals
		if err := sv.SetFromGoogleSQL("CLI_EXPLAIN_FORMAT", "'COMPACT'"); err != nil {
			t.Errorf("Failed to set CLI_EXPLAIN_FORMAT: %v", err)
		}
		if sv.ExplainFormat != explainFormatCompact {
			t.Errorf("Expected ExplainFormat to be COMPACT, got %v", sv.ExplainFormat)
		}

		// Test empty string (unspecified)
		if err := sv.SetFromGoogleSQL("CLI_EXPLAIN_FORMAT", `""`); err != nil {
			t.Errorf("Failed to set CLI_EXPLAIN_FORMAT to empty: %v", err)
		}
		if sv.ExplainFormat != explainFormatUnspecified {
			t.Errorf("Expected ExplainFormat to be unspecified, got %v", sv.ExplainFormat)
		}
	})

	// Test duration variables
	t.Run("MAX_COMMIT_DELAY", func(t *testing.T) {
		// Test setting valid duration
		if err := sv.SetFromGoogleSQL("MAX_COMMIT_DELAY", `"100ms"`); err != nil {
			t.Errorf("Failed to set MAX_COMMIT_DELAY: %v", err)
		}
		if sv.MaxCommitDelay == nil || *sv.MaxCommitDelay != 100*time.Millisecond {
			t.Errorf("Expected MaxCommitDelay to be 100ms, got %v", sv.MaxCommitDelay)
		}

		// Test setting NULL
		if err := sv.SetFromGoogleSQL("MAX_COMMIT_DELAY", "NULL"); err != nil {
			t.Errorf("Failed to set MAX_COMMIT_DELAY to NULL: %v", err)
		}
		if sv.MaxCommitDelay != nil {
			t.Errorf("Expected MaxCommitDelay to be nil, got %v", sv.MaxCommitDelay)
		}

		// Test max value constraint (500ms)
		if err := sv.SetFromGoogleSQL("MAX_COMMIT_DELAY", `"600ms"`); err == nil {
			t.Error("Expected error for duration exceeding 500ms")
		}

		// Test negative duration
		if err := sv.SetFromGoogleSQL("MAX_COMMIT_DELAY", `"-100ms"`); err == nil {
			t.Error("Expected error for negative duration")
		}
	})

	t.Run("STATEMENT_TIMEOUT", func(t *testing.T) {
		// Test setting duration
		if err := sv.SetFromGoogleSQL("STATEMENT_TIMEOUT", `"5m"`); err != nil {
			t.Errorf("Failed to set STATEMENT_TIMEOUT: %v", err)
		}
		if sv.StatementTimeout == nil || *sv.StatementTimeout != 5*time.Minute {
			t.Errorf("Expected StatementTimeout to be 5m, got %v", sv.StatementTimeout)
		}

		// Test getting value
		result, err := sv.Get("STATEMENT_TIMEOUT")
		if err != nil {
			t.Errorf("Failed to get STATEMENT_TIMEOUT: %v", err)
		}
		if result["STATEMENT_TIMEOUT"] != "5m0s" {
			t.Errorf("Expected '5m0s', got %s", result["STATEMENT_TIMEOUT"])
		}

		// Test NULL
		if err := sv.SetFromGoogleSQL("STATEMENT_TIMEOUT", "NULL"); err != nil {
			t.Errorf("Failed to set STATEMENT_TIMEOUT to NULL: %v", err)
		}
		if sv.StatementTimeout != nil {
			t.Errorf("Expected StatementTimeout to be nil, got %v", sv.StatementTimeout)
		}

		// Test getting NULL value
		result, err = sv.Get("STATEMENT_TIMEOUT")
		if err != nil {
			t.Errorf("Failed to get NULL STATEMENT_TIMEOUT: %v", err)
		}
		if result["STATEMENT_TIMEOUT"] != "NULL" {
			t.Errorf("Expected 'NULL', got %s", result["STATEMENT_TIMEOUT"])
		}
	})
}

func TestReadOnlyTransactionCheck(t *testing.T) {
	// This test would require a more complex setup with actual session and transaction
	// For now, we'll skip this test as it requires deeper integration
	t.Skip("Skipping transaction check test - requires full session setup")
}

func TestParseModeEnum(t *testing.T) {
	// Create a test systemVariables instance
	sv := &systemVariables{}
	registry := createSystemVariableRegistry(sv)

	t.Run("Set with GoogleSQL mode", func(t *testing.T) {
		tests := []struct {
			name  string
			input string
			want  parseMode
			err   bool
		}{
			{"identifier FALLBACK", "FALLBACK", "", true},           // Identifiers not allowed in GoogleSQL mode
			{"identifier NO_MEMEFISH", "NO_MEMEFISH", "", true},     // Identifiers not allowed in GoogleSQL mode
			{"identifier MEMEFISH_ONLY", "MEMEFISH_ONLY", "", true}, // Identifiers not allowed in GoogleSQL mode
			{"identifier UNSPECIFIED", "UNSPECIFIED", "", true},     // Identifiers not allowed in GoogleSQL mode
			{"string literal FALLBACK", `"FALLBACK"`, parseModeFallback, false},
			{"string literal NO_MEMEFISH", `"NO_MEMEFISH"`, parseModeNoMemefish, false},
			{"string literal MEMEFISH_ONLY", `"MEMEFISH_ONLY"`, parseMemefishOnly, false},
			{"string literal UNSPECIFIED", `"UNSPECIFIED"`, parseModeUnspecified, false},
			{"empty string", `""`, parseModeUnspecified, false},
			{"invalid value", `"INVALID"`, "", true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := registry.SetFromGoogleSQL("CLI_PARSE_MODE", tt.input)
				if (err != nil) != tt.err {
					t.Errorf("SetFromGoogleSQL() error = %v, wantErr %v", err, tt.err)
					return
				}
				if !tt.err && sv.BuildStatementMode != tt.want {
					t.Errorf("SetFromGoogleSQL() set %v, want %v", sv.BuildStatementMode, tt.want)
				}
			})
		}
	})

	t.Run("Set with Simple mode", func(t *testing.T) {
		tests := []struct {
			name  string
			input string
			want  parseMode
			err   bool
		}{
			{"uppercase FALLBACK", "FALLBACK", parseModeFallback, false},
			{"lowercase fallback", "fallback", parseModeFallback, false},
			{"mixed case FaLLbAcK", "FaLLbAcK", parseModeFallback, false},
			{"with spaces", "  FALLBACK  ", parseModeFallback, false},
			{"empty string", "", parseModeUnspecified, false},
			{"invalid value", "INVALID", "", true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := registry.SetFromSimple("CLI_PARSE_MODE", tt.input)
				if (err != nil) != tt.err {
					t.Errorf("SetFromSimple() error = %v, wantErr %v", err, tt.err)
					return
				}
				if !tt.err && sv.BuildStatementMode != tt.want {
					t.Errorf("SetFromSimple() set %v, want %v", sv.BuildStatementMode, tt.want)
				}
			})
		}
	})

	t.Run("GetValue returns string representation", func(t *testing.T) {
		tests := []struct {
			mode parseMode
			want string
		}{
			{parseModeFallback, "FALLBACK"},
			{parseModeNoMemefish, "NO_MEMEFISH"},
			{parseMemefishOnly, "MEMEFISH_ONLY"},
			{parseModeUnspecified, ""}, // Empty string for unspecified in new behavior
		}

		for _, tt := range tests {
			sv.BuildStatementMode = tt.mode
			value, err := registry.Get("CLI_PARSE_MODE")
			if err != nil {
				t.Fatalf("Get() failed: %v", err)
			}
			if value != tt.want {
				t.Errorf("Get() = %q, want %q for mode %v", value, tt.want, tt.mode)
			}
		}
	})
}

func TestEnumParsersSimplification(t *testing.T) {
	// Test that NewSimpleEnumParser works correctly for various enum types
	t.Run("string enum", func(t *testing.T) {
		type testEnum string
		const (
			enumValue1 testEnum = "VALUE1"
			enumValue2 testEnum = "VALUE2"
		)

		var currentValue testEnum
		p := sysvar.NewSimpleEnumParser[testEnum](
			"TEST_ENUM",
			"Test enum",
			map[string]testEnum{
				"VALUE1": enumValue1,
				"VALUE2": enumValue2,
			},
			func() testEnum { return currentValue },
			func(v testEnum) error { currentValue = v; return nil },
		)

		// Test GoogleSQL mode
		err := p.ParseAndSetWithMode(`"VALUE1"`, sysvar.ParseModeGoogleSQL)
		if err != nil {
			t.Fatalf("ParseAndSetWithMode failed: %v", err)
		}
		if currentValue != enumValue1 {
			t.Errorf("Expected %v, got %v", enumValue1, currentValue)
		}

		// Test Simple mode
		err = p.ParseAndSetWithMode("VALUE2", sysvar.ParseModeSimple)
		if err != nil {
			t.Fatalf("ParseAndSetWithMode failed: %v", err)
		}
		if currentValue != enumValue2 {
			t.Errorf("Expected %v, got %v", enumValue2, currentValue)
		}

		// Test GetValue
		value, err := p.GetValue()
		if err != nil {
			t.Fatalf("GetValue failed: %v", err)
		}
		if value != "VALUE2" {
			t.Errorf("GetValue() = %q, want %q", value, "VALUE2")
		}
	})
}

// TestCLIAnalyzeColumnsAndInlineStats verifies that CLI_ANALYZE_COLUMNS and CLI_INLINE_STATS
// properly parse and store their template definitions.
func TestCLIAnalyzeColumnsAndInlineStats(t *testing.T) {
	t.Run("CLI_ANALYZE_COLUMNS", func(t *testing.T) {
		sv := newSystemVariablesWithDefaultsForTest()
		registry := createSystemVariableRegistry(sv)

		// Test valid template
		validTemplate := "Col1:{{.Col1}},Col2:{{.Col2}}:LEFT"
		err := registry.SetFromSimple("CLI_ANALYZE_COLUMNS", validTemplate)
		if err != nil {
			t.Fatalf("Failed to set CLI_ANALYZE_COLUMNS: %v", err)
		}

		// Verify string value is stored
		if sv.AnalyzeColumns != validTemplate {
			t.Errorf("Expected AnalyzeColumns to be %q, got %q", validTemplate, sv.AnalyzeColumns)
		}

		// Verify ParsedAnalyzeColumns is not nil and has correct count
		if sv.ParsedAnalyzeColumns == nil {
			t.Fatal("ParsedAnalyzeColumns should not be nil")
		}
		if len(sv.ParsedAnalyzeColumns) != 2 {
			t.Fatalf("Expected 2 parsed columns, got %d", len(sv.ParsedAnalyzeColumns))
		}

		// Test with invalid template (should fail)
		invalidTemplate := "Col1:{{.Unclosed"
		err = registry.SetFromSimple("CLI_ANALYZE_COLUMNS", invalidTemplate)
		if err == nil {
			t.Error("Expected error for invalid template")
		}

		// Test with GoogleSQL mode
		err = registry.SetFromGoogleSQL("CLI_ANALYZE_COLUMNS", `"Header:{{.Value}}:CENTER"`)
		if err != nil {
			t.Fatalf("Failed to set CLI_ANALYZE_COLUMNS in GoogleSQL mode: %v", err)
		}
		if sv.AnalyzeColumns != "Header:{{.Value}}:CENTER" {
			t.Errorf("Expected AnalyzeColumns to be updated")
		}
		if len(sv.ParsedAnalyzeColumns) != 1 {
			t.Errorf("Expected 1 parsed column after update")
		}
	})

	t.Run("CLI_INLINE_STATS", func(t *testing.T) {
		sv := newSystemVariablesWithDefaultsForTest()
		registry := createSystemVariableRegistry(sv)

		// Test valid template
		validTemplate := "cpu:{{.Cpu.Total}},mem:{{.Memory.Used}}"
		err := registry.SetFromSimple("CLI_INLINE_STATS", validTemplate)
		if err != nil {
			t.Fatalf("Failed to set CLI_INLINE_STATS: %v", err)
		}

		// Verify string value is stored
		if sv.InlineStats != validTemplate {
			t.Errorf("Expected InlineStats to be %q, got %q", validTemplate, sv.InlineStats)
		}

		// Verify ParsedInlineStats is not nil and has correct count
		if sv.ParsedInlineStats == nil {
			t.Fatal("ParsedInlineStats should not be nil")
		}
		if len(sv.ParsedInlineStats) != 2 {
			t.Fatalf("Expected 2 parsed stats, got %d", len(sv.ParsedInlineStats))
		}

		// Test with invalid format (should fail)
		invalidTemplate := "no colon separator"
		err = registry.SetFromSimple("CLI_INLINE_STATS", invalidTemplate)
		if err == nil {
			t.Error("Expected error for invalid format")
		}

		// Test with GoogleSQL mode
		err = registry.SetFromGoogleSQL("CLI_INLINE_STATS", `'rows:{{.Rows.Total}}'`)
		if err != nil {
			t.Fatalf("Failed to set CLI_INLINE_STATS in GoogleSQL mode: %v", err)
		}
		if sv.InlineStats != "rows:{{.Rows.Total}}" {
			t.Errorf("Expected InlineStats to be updated")
		}
		if len(sv.ParsedInlineStats) != 1 {
			t.Errorf("Expected 1 parsed stat after update")
		}
	})

	t.Run("Default values", func(t *testing.T) {
		// Test that default system variables have ParsedAnalyzeColumns initialized
		sv := newSystemVariablesWithDefaultsForTest()

		if sv.ParsedAnalyzeColumns == nil {
			t.Error("Default ParsedAnalyzeColumns should not be nil")
		}

		// The default should have been parsed from DefaultAnalyzeColumns
		expectedCount := len(DefaultParsedAnalyzeColumns)
		if len(sv.ParsedAnalyzeColumns) != expectedCount {
			t.Errorf("Expected %d default parsed columns, got %d", expectedCount, len(sv.ParsedAnalyzeColumns))
		}
	})
}
