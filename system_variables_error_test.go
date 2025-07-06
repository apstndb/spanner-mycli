package main

import (
	"strings"
	"testing"
)

// TestSystemVariables_ErrorTypes tests all error type Error() methods for coverage
func TestSystemVariables_ErrorTypes(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "errSetterUnimplemented",
			err:  errSetterUnimplemented{Name: "TEST_VAR"},
			want: "unimplemented setter: TEST_VAR",
		},
		{
			name: "errGetterUnimplemented",
			err:  errGetterUnimplemented{Name: "TEST_VAR"},
			want: "unimplemented getter: TEST_VAR",
		},
		{
			name: "errAdderUnimplemented",
			err:  errAdderUnimplemented{Name: "TEST_VAR"},
			want: "unimplemented adder: TEST_VAR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.want {
				t.Errorf("Error() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSystemVariables_Set_Errors tests error cases in the Set method
func TestSystemVariables_Set_Errors(t *testing.T) {
	tests := []struct {
		name      string
		varName   string
		value     string
		wantError string
	}{
		{
			name:      "unknown variable",
			varName:   "UNKNOWN_VAR",
			value:     "value",
			wantError: "unknown variable name: UNKNOWN_VAR",
		},
		{
			name:      "read-only variable with nil setter",
			varName:   "AUTOCOMMIT", // This has only Getter, no Setter
			value:     "true",
			wantError: "unimplemented setter: AUTOCOMMIT",
		},
		{
			name:      "invalid boolean value",
			varName:   "CLI_VERBOSE",
			value:     "not-a-bool",
			wantError: "strconv.ParseBool: parsing \"not-a-bool\": invalid syntax",
		},
		{
			name:      "invalid integer value",
			varName:   "CLI_FIXED_WIDTH", // This parses int64
			value:     "not-a-number",
			wantError: "strconv.ParseInt: parsing \"not-a-number\": invalid syntax",
		},
		{
			name:      "invalid duration value",
			varName:   "STATEMENT_TIMEOUT",
			value:     "invalid-duration",
			wantError: "invalid timeout format: time: invalid duration \"invalid-duration\"",
		},
		{
			name:      "invalid statement timeout negative",
			varName:   "STATEMENT_TIMEOUT",
			value:     "-1s",
			wantError: "timeout cannot be negative",
		},
		{
			name:      "invalid default isolation level",
			varName:   "DEFAULT_ISOLATION_LEVEL",
			value:     "INVALID_LEVEL",
			wantError: "invalid isolation level: INVALID_LEVEL",
		},
		// CLI_FORMAT doesn't return error for invalid values, it just ignores them
		{
			name:      "invalid autocommit dml mode",
			varName:   "AUTOCOMMIT_DML_MODE",
			value:     "INVALID_MODE",
			wantError: "invalid AUTOCOMMIT_DML_MODE value: INVALID_MODE",
		},
		{
			name:      "invalid cli parse mode",
			varName:   "CLI_PARSE_MODE",
			value:     "INVALID_MODE",
			wantError: "invalid value: INVALID_MODE",
		},
		{
			name:      "invalid cli query mode",
			varName:   "CLI_QUERY_MODE",
			value:     "INVALID_MODE",
			wantError: "invalid value: INVALID_MODE",
		},
		{
			name:      "invalid rpc priority",
			varName:   "RPC_PRIORITY",
			value:     "INVALID_PRIORITY",
			wantError: "invalid priority: \"PRIORITY_INVALID_PRIORITY\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sv := &systemVariables{}
			err := sv.Set(tt.varName, tt.value)
			if err == nil {
				t.Fatal("expected error but got nil")
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Errorf("Set() error = %v, want error containing %v", err, tt.wantError)
			}
		})
	}
}

// TestSystemVariables_Get_Errors tests error cases in the Get method
func TestSystemVariables_Get_Errors(t *testing.T) {
	tests := []struct {
		name      string
		varName   string
		wantError string
	}{
		{
			name:      "unknown variable",
			varName:   "UNKNOWN_VAR",
			wantError: "unknown variable name: UNKNOWN_VAR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sv := &systemVariables{}
			_, err := sv.Get(tt.varName)
			if err == nil {
				t.Fatal("expected error but got nil")
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Errorf("Get() error = %v, want error containing %v", err, tt.wantError)
			}
		})
	}
}

// TestSystemVariables_Add_Errors tests error cases in the Add method
func TestSystemVariables_Add_Errors(t *testing.T) {
	tests := []struct {
		name      string
		varName   string
		value     string
		wantError string
	}{
		{
			name:      "unknown variable",
			varName:   "UNKNOWN_VAR",
			value:     "value",
			wantError: "unknown variable name: UNKNOWN_VAR",
		},
		{
			name:      "variable without adder",
			varName:   "CLI_VERBOSE", // This has Setter but no Adder
			value:     "true",
			wantError: "unimplemented adder: CLI_VERBOSE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sv := &systemVariables{}
			err := sv.Add(tt.varName, tt.value)
			if err == nil {
				t.Fatal("expected error but got nil")
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Errorf("Add() error = %v, want error containing %v", err, tt.wantError)
			}
		})
	}
}

// TestSystemVariables_CaseInsensitive tests that variable names are case-insensitive
func TestSystemVariables_CaseInsensitive(t *testing.T) {
	// Test Set with various cases
	testCases := []string{"cli_verbose", "CLI_VERBOSE", "Cli_Verbose", "cLi_VeRbOsE"}
	for _, varName := range testCases {
		t.Run("Set_"+varName, func(t *testing.T) {
			sv := &systemVariables{}
			err := sv.Set(varName, "true")
			if err != nil {
				t.Errorf("Set(%s) failed: %v", varName, err)
			}

			// Verify that Get works with the same case
			result, err := sv.Get(varName)
			if err != nil {
				t.Errorf("Get(%s) failed: %v", varName, err)
			}
			// Get returns with the case used in the request
			if result[varName] != "TRUE" {
				t.Errorf("Get(%s) = %v, want %s=TRUE", varName, result, varName)
			}
		})
	}

	// Test cross-case: Set with one case, Get with another
	t.Run("CrossCase", func(t *testing.T) {
		sv := &systemVariables{}
		// Set with lowercase
		err := sv.Set("cli_verbose", "true")
		if err != nil {
			t.Errorf("Set(cli_verbose) failed: %v", err)
		}

		// Get with uppercase - should still work
		result, err := sv.Get("CLI_VERBOSE")
		if err != nil {
			t.Errorf("Get(CLI_VERBOSE) failed: %v", err)
		}
		if result["CLI_VERBOSE"] != "TRUE" {
			t.Errorf("Get(CLI_VERBOSE) = %v, want CLI_VERBOSE=TRUE", result)
		}
	})
}
