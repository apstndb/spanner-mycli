// Copyright 2026 apstndb
//
// Licensed under the MIT License.

package mycli

import (
	"strings"
	"testing"
)

// TestParseEndpoint covers endpoint string parsing for the --endpoint flag.
// (CLI_ENDPOINT itself is read-only; see TestStartupConfigVariablesAreReadOnly.)
func TestParseEndpoint(t *testing.T) {
	t.Parallel()
	tests := []struct {
		desc        string
		value       string
		wantHost    string
		wantPort    int
		errContains string
	}{
		{desc: "valid endpoint", value: "example.com:443", wantHost: "example.com", wantPort: 443},
		{desc: "endpoint with IPv6", value: "[2001:db8::1]:443", wantHost: "2001:db8::1", wantPort: 443},
		{desc: "invalid endpoint - no port", value: "example.com", errContains: "invalid endpoint format"},
		{desc: "invalid endpoint - bare IPv6 without port", value: "2001:db8::1", errContains: "invalid endpoint format"},
		{desc: "invalid endpoint - non-numeric port", value: "example.com:abc", errContains: "invalid port in endpoint"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			t.Parallel()
			host, port, err := parseEndpoint(tt.value)
			if tt.errContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("parseEndpoint(%q) error = %v, want containing %q", tt.value, err, tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseEndpoint(%q) unexpected error: %v", tt.value, err)
			}
			if host != tt.wantHost || port != tt.wantPort {
				t.Errorf("parseEndpoint(%q) = (%q, %d), want (%q, %d)", tt.value, host, port, tt.wantHost, tt.wantPort)
			}
		})
	}
}

// TestStartupConfigVariablesAreReadOnly asserts that every system variable
// backed by StartupConfig rejects SET. This is load-bearing for
// CLI_SKIP_SYSTEM_COMMAND: it is a security feature, and if it were settable a
// user in a restricted environment could re-enable shell access with SET.
// CLI_ENABLE_ADC_PLUS is excluded: it is session-init-only by design.
func TestStartupConfigVariablesAreReadOnly(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		varName string
		value   string
	}{
		{varName: "CLI_HOST", value: "example.com"},
		{varName: "CLI_PORT", value: "443"},
		{varName: "CLI_ENDPOINT", value: "example.com:443"},
		{varName: "CLI_INSECURE", value: "TRUE"},
		{varName: "CLI_IMPERSONATE_SERVICE_ACCOUNT", value: "sa@example.com"},
		{varName: "CLI_EMULATOR_PLATFORM", value: "linux/amd64"},
		{varName: "CLI_LOG_GRPC", value: "TRUE"},
		{varName: "CLI_MCP", value: "TRUE"},
		{varName: "CLI_SKIP_SYSTEM_COMMAND", value: "FALSE"},
	} {
		t.Run(tt.varName, func(t *testing.T) {
			t.Parallel()
			sysVars := newSystemVariablesWithDefaultsForTest()
			err := sysVars.SetFromSimple(tt.varName, tt.value)
			if err == nil || !strings.Contains(err.Error(), "read-only") {
				t.Errorf("SET %s: got error %v, want read-only error", tt.varName, err)
			}
		})
	}
}
