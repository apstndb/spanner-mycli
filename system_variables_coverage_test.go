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

// TestAdd tests the AddFromGoogleSQL method
func TestAdd(t *testing.T) {
	sv := newSystemVariablesWithDefaults()
	err := sv.AddFromGoogleSQL("UNKNOWN_VAR", "'value'")
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
