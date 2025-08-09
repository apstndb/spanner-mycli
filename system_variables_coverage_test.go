package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"google.golang.org/protobuf/reflect/protoregistry"
)

// TestProjectPath tests the ProjectPath function
func TestProjectPath(t *testing.T) {
	sv := &systemVariables{
		Project: "test-project",
	}

	got := sv.ProjectPath()
	want := "projects/test-project"
	if got != want {
		t.Errorf("ProjectPath() = %q, want %q", got, want)
	}
}

// TestParseTimeString tests parseTimeString function
func TestParseTimeString(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid Spanner format", "2023-01-01 00:00:00.000000000 +0000 UTC", false},
		{"valid with timezone", "2023-01-01 15:30:45.123456789 +0900 JST", false},
		{"invalid", "not-a-time", true},
		{"empty", "", true},
		{"RFC3339 should fail", "2023-01-01T00:00:00Z", true}, // This format is not supported
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseTimeString(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseTimeString(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

// TestGetterSetterFunctions tests the getter/setter helper functions
func TestGetterSetterFunctions(t *testing.T) {
	sv := &systemVariables{}

	t.Run("int64Setter", func(t *testing.T) {
		setter := int64Setter(func(variables *systemVariables) *int64 {
			return &variables.TabWidth
		})

		// Test successful set
		if err := setter(sv, "TEST", "42"); err != nil {
			t.Errorf("int64Setter failed: %v", err)
		}
		if sv.TabWidth != 42 {
			t.Errorf("TabWidth = %d, want 42", sv.TabWidth)
		}

		// Test invalid value
		if err := setter(sv, "TEST", "not-a-number"); err == nil {
			t.Error("Expected error for invalid number")
		}

		// Test empty value
		if err := setter(sv, "TEST", ""); err == nil {
			t.Error("Expected error for empty value")
		}
	})

	t.Run("boolSetter", func(t *testing.T) {
		setter := boolSetter(func(variables *systemVariables) *bool {
			return &variables.Verbose
		})

		// Test successful set
		if err := setter(sv, "TEST", "true"); err != nil {
			t.Errorf("boolSetter failed: %v", err)
		}
		if !sv.Verbose {
			t.Error("Verbose should be true")
		}

		// Test various boolean values
		testCases := []struct {
			input string
			want  bool
		}{
			{"false", false},
			{"TRUE", true},
			{"FALSE", false},
			{"1", true},
			{"0", false},
		}

		for _, tc := range testCases {
			if err := setter(sv, "TEST", tc.input); err != nil {
				t.Errorf("boolSetter(%q) failed: %v", tc.input, err)
			}
			if sv.Verbose != tc.want {
				t.Errorf("Verbose = %v after setting %q, want %v", sv.Verbose, tc.input, tc.want)
			}
		}

		// Test invalid value
		if err := setter(sv, "TEST", "not-a-bool"); err == nil {
			t.Error("Expected error for invalid bool")
		}
	})

	t.Run("stringSetter", func(t *testing.T) {
		setter := stringSetter(func(variables *systemVariables) *string {
			return &variables.Prompt
		})

		// Test successful set
		if err := setter(sv, "TEST", "hello"); err != nil {
			t.Errorf("stringSetter failed: %v", err)
		}
		if sv.Prompt != "hello" {
			t.Errorf("Prompt = %q, want %q", sv.Prompt, "hello")
		}

		// Test with quotes
		if err := setter(sv, "TEST", "'quoted'"); err != nil {
			t.Errorf("stringSetter with quotes failed: %v", err)
		}
		if sv.Prompt != "quoted" {
			t.Errorf("Prompt = %q, want %q", sv.Prompt, "quoted")
		}

		// Test empty string
		if err := setter(sv, "TEST", ""); err != nil {
			t.Errorf("stringSetter with empty string failed: %v", err)
		}
		if sv.Prompt != "" {
			t.Errorf("Prompt = %q, want empty string", sv.Prompt)
		}
	})

	t.Run("stringGetter", func(t *testing.T) {
		sv.Prompt = "test-prompt"
		getter := stringGetter(func(variables *systemVariables) *string {
			return &variables.Prompt
		})

		got, err := getter(sv, "TEST")
		if err != nil {
			t.Errorf("stringGetter failed: %v", err)
		}
		if val, ok := got["TEST"]; !ok || val != "test-prompt" {
			t.Errorf("stringGetter returned %v, want map with TEST=test-prompt", got)
		}

		// Test with nil return
		nilGetter := stringGetter(func(variables *systemVariables) *string {
			return nil
		})
		_, err = nilGetter(sv, "TEST")
		if err == nil {
			t.Error("Expected error when getter returns nil")
		}
	})

	t.Run("int64Getter", func(t *testing.T) {
		sv.TabWidth = 4
		getter := int64Getter(func(variables *systemVariables) *int64 {
			return &variables.TabWidth
		})

		got, err := getter(sv, "TEST")
		if err != nil {
			t.Errorf("int64Getter failed: %v", err)
		}
		if val, ok := got["TEST"]; !ok || val != "4" {
			t.Errorf("int64Getter returned %v, want map with TEST=4", got)
		}

		// Test with nil return
		nilGetter := int64Getter(func(variables *systemVariables) *int64 {
			return nil
		})
		_, err = nilGetter(sv, "TEST")
		if err == nil {
			t.Error("Expected error when getter returns nil")
		}
	})
}

// TestGetSetters tests various accessor functions
func TestGetSetters(t *testing.T) {
	t.Run("int64Accessor", func(t *testing.T) {
		var value int64 = 42
		accessor := int64Accessor(func(sv *systemVariables) *int64 { return &value })

		// Test getter
		result, err := accessor.Getter(nil, "TEST")
		if err != nil {
			t.Fatalf("Getter failed: %v", err)
		}
		if result["TEST"] != "42" {
			t.Errorf("Getter returned %v, want TEST=42", result)
		}

		// Test setter
		err = accessor.Setter(nil, "TEST", "100")
		if err != nil {
			t.Fatalf("Setter failed: %v", err)
		}
		if value != 100 {
			t.Errorf("Value = %d, want 100", value)
		}

		// Test setter with invalid value
		err = accessor.Setter(nil, "TEST", "invalid")
		if err == nil {
			t.Error("Expected error for invalid value")
		}
	})

	t.Run("boolAccessor", func(t *testing.T) {
		value := true
		accessor := boolAccessor(func(sv *systemVariables) *bool { return &value })

		// Test getter
		result, err := accessor.Getter(&systemVariables{}, "TEST")
		if err != nil {
			t.Fatalf("Getter failed: %v", err)
		}
		if result["TEST"] != "TRUE" {
			t.Errorf("Getter returned %v, want TEST=TRUE", result)
		}

		// Test setter
		err = accessor.Setter(&systemVariables{}, "TEST", "false")
		if err != nil {
			t.Fatalf("Setter failed: %v", err)
		}
		if value != false {
			t.Errorf("Value = %v, want false", value)
		}
	})

	t.Run("stringAccessor", func(t *testing.T) {
		value := "hello"
		accessor := stringAccessor(func(sv *systemVariables) *string { return &value })

		// Test getter
		result, err := accessor.Getter(&systemVariables{}, "TEST")
		if err != nil {
			t.Fatalf("Getter failed: %v", err)
		}
		if result["TEST"] != "hello" {
			t.Errorf("Getter returned %v, want TEST=hello", result)
		}

		// Test setter
		err = accessor.Setter(&systemVariables{}, "TEST", "world")
		if err != nil {
			t.Fatalf("Setter failed: %v", err)
		}
		if value != "world" {
			t.Errorf("Value = %q, want world", value)
		}
	})
}

// TestAddFromSimple tests the AddFromSimple method
func TestAddFromSimple(t *testing.T) {
	t.Run("unknown variable", func(t *testing.T) {
		sv := newSystemVariablesWithDefaults()
		err := sv.AddFromSimple("UNKNOWN_VAR", "value")
		if err == nil {
			t.Error("Expected error for unknown variable")
		}
	})

	t.Run("variable without add support", func(t *testing.T) {
		sv := newSystemVariablesWithDefaults()
		// CLI_FORMAT exists but doesn't support ADD
		err := sv.AddFromSimple("CLI_FORMAT", "TABLE")
		if err == nil {
			t.Error("Expected error for variable without ADD support")
		}
	})

	t.Run("old system variable with add support", func(t *testing.T) {
		// Create a temporary proto file for testing
		tempFile := t.TempDir() + "/test.pb"
		if err := os.WriteFile(tempFile, []byte("test content"), 0o644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		sv := newSystemVariablesWithDefaults()
		// CLI_PROTO_DESCRIPTOR_FILE uses the old system and has Adder support
		err := sv.AddFromSimple("CLI_PROTO_DESCRIPTOR_FILE", tempFile)
		// This will fail because the file content is not a valid proto descriptor
		if err == nil {
			t.Error("Expected error for invalid proto descriptor file")
		}
	})
}

// TestAddFromGoogleSQL tests the AddFromGoogleSQL method
func TestAddFromGoogleSQL(t *testing.T) {
	t.Run("unknown variable", func(t *testing.T) {
		sv := newSystemVariablesWithDefaults()
		err := sv.AddFromGoogleSQL("UNKNOWN_VAR", "'value'")
		if err == nil {
			t.Error("Expected error for unknown variable")
		}
	})

	t.Run("variable without add support", func(t *testing.T) {
		sv := newSystemVariablesWithDefaults()
		// CLI_FORMAT exists but doesn't support ADD
		err := sv.AddFromGoogleSQL("CLI_FORMAT", "'TABLE'")
		if err == nil {
			t.Error("Expected error for variable without ADD support")
		}
	})

	t.Run("old system variable with add support", func(t *testing.T) {
		// Create a temporary proto file for testing
		tempFile := t.TempDir() + "/test.pb"
		if err := os.WriteFile(tempFile, []byte("test content"), 0o644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		sv := newSystemVariablesWithDefaults()
		// CLI_PROTO_DESCRIPTOR_FILE uses the old system and has Adder support
		// Need to quote the filename for GoogleSQL
		err := sv.AddFromGoogleSQL("CLI_PROTO_DESCRIPTOR_FILE", "'"+tempFile+"'")
		// This will fail because the file content is not a valid proto descriptor
		if err == nil {
			t.Error("Expected error for invalid proto descriptor file")
		}
	})
}

// TestAdd tests the Add method (which delegates to AddFromGoogleSQL)
func TestAdd(t *testing.T) {
	sv := newSystemVariablesWithDefaults()
	err := sv.Add("UNKNOWN_VAR", "'value'")
	if err == nil {
		t.Error("Expected error for unknown variable")
	}
}

// TestHttpResolveFunc tests the httpResolveFunc function
func TestHttpResolveFunc(t *testing.T) {
	// Create a test HTTP server
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/valid.proto":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("syntax = \"proto3\";"))
		case "/not_found.proto":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("404 Not Found"))
		case "/error.proto":
			// Simulate server error
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer httpServer.Close()

	t.Run("non-http path", func(t *testing.T) {
		result, err := httpResolveFunc("local/file.proto")
		if err != protoregistry.NotFound {
			t.Errorf("Expected protoregistry.NotFound, got %v", err)
		}
		if result.Source != nil {
			t.Error("Expected nil source for non-HTTP path")
		}
	})

	t.Run("valid http URL", func(t *testing.T) {
		result, err := httpResolveFunc(httpServer.URL + "/valid.proto")
		if err != nil {
			t.Errorf("Unexpected error for valid HTTP URL: %v", err)
		}
		if result.Source == nil {
			t.Error("Expected non-nil source for valid HTTP URL")
		}
	})

	t.Run("https URL", func(t *testing.T) {
		// Create HTTPS test server
		httpsServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("syntax = \"proto3\";"))
		}))
		defer httpsServer.Close()

		// Note: This will fail in tests due to certificate issues, but it tests the path matching
		_, err := httpResolveFunc(httpsServer.URL + "/test.proto")
		// We expect an error due to certificate issues in test environment
		if err == nil {
			t.Error("Expected error for HTTPS URL in test environment")
		}
		// But the function should have attempted to fetch it (not returned NotFound)
		if err == protoregistry.NotFound {
			t.Error("Should not return NotFound for HTTPS URL")
		}
	})

	t.Run("file path", func(t *testing.T) {
		result, err := httpResolveFunc("file:///path/to/file.proto")
		if err != protoregistry.NotFound {
			t.Errorf("Expected protoregistry.NotFound for file:// URL, got %v", err)
		}
		if result.Source != nil {
			t.Error("Expected nil source for file:// URL")
		}
	})
}
