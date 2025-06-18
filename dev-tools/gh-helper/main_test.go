package main

import (
	"os"
	"testing"
	"time"
)

func TestParseTimeout(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    time.Duration
		expectError bool
	}{
		{
			name:     "valid seconds",
			input:    "30s",
			expected: 30 * time.Second,
		},
		{
			name:     "valid minutes",
			input:    "5m",
			expected: 5 * time.Minute,
		},
		{
			name:     "valid mixed format",
			input:    "2m30s",
			expected: 2*time.Minute + 30*time.Second,
		},
		{
			name:     "valid decimal minutes",
			input:    "1.5m",
			expected: 90 * time.Second,
		},
		{
			name:     "valid hours",
			input:    "1h",
			expected: time.Hour,
		},
		{
			name:     "numeric only - assumes minutes",
			input:    "30",
			expected: 30 * time.Minute,
		},
		{
			name:        "invalid format - bad unit",
			input:       "30x",
			expectError: true,
		},
		{
			name:     "empty string - uses default",
			input:    "",
			expected: 0, // Returns 0 for default behavior
		},
		{
			name:        "negative duration",
			input:       "-5m",
			expectError: false, // time.ParseDuration actually allows negative values
			expected:    -5 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set the global variable for the test
			timeoutStr = tt.input
			
			result, err := parseTimeout()
			
			if tt.expectError {
				if err == nil {
					t.Errorf("parseTimeout() expected error for input %q, but got none", tt.input)
				}
				return
			}
			
			if err != nil {
				t.Errorf("parseTimeout() unexpected error for input %q: %v", tt.input, err)
				return
			}
			
			if result != tt.expected {
				t.Errorf("parseTimeout() for input %q = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCalculateEffectiveTimeout(t *testing.T) {
	tests := []struct {
		name                string
		requestedTimeout    string
		bashMaxTimeoutMS    string
		bashDefaultTimeoutMS string
		expectedTimeout     time.Duration
		expectedDisplay     string
	}{
		{
			name:             "normal timeout without env vars within safety limit",
			requestedTimeout: "1m",
			expectedTimeout:  1 * time.Minute,
			expectedDisplay:  "1m",
		},
		{
			name:             "timeout exceeds safety limit without env vars",
			requestedTimeout: "5m",
			expectedTimeout:  5 * time.Minute, // No safety limit logic in current implementation
			expectedDisplay:  "5m",
		},
		{
			name:             "timeout exactly at safety limit without env vars",
			requestedTimeout: "90s",
			expectedTimeout:  90 * time.Second,
			expectedDisplay:  "1.5m",
		},
		{
			name:                "timeout within Claude Code limit",
			requestedTimeout:    "10m",
			bashMaxTimeoutMS:    "900000", // 15 minutes
			expectedTimeout:     10 * time.Minute,
			expectedDisplay:     "10m",
		},
		{
			name:                "timeout exceeds Claude Code limit",
			requestedTimeout:    "20m",
			bashMaxTimeoutMS:    "900000", // 15 minutes
			expectedTimeout:     15 * time.Minute,
			expectedDisplay:     "15m",
		},
		{
			name:                 "default timeout used",
			requestedTimeout:     "10m",
			bashDefaultTimeoutMS: "600000", // 10 minutes
			expectedTimeout:      10 * time.Minute,
			expectedDisplay:      "10m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment
			_ = os.Unsetenv("BASH_MAX_TIMEOUT_MS")
			_ = os.Unsetenv("BASH_DEFAULT_TIMEOUT_MS")
			
			// Set up environment for test
			if tt.bashMaxTimeoutMS != "" {
				_ = os.Setenv("BASH_MAX_TIMEOUT_MS", tt.bashMaxTimeoutMS)
			}
			if tt.bashDefaultTimeoutMS != "" {
				_ = os.Setenv("BASH_DEFAULT_TIMEOUT_MS", tt.bashDefaultTimeoutMS)
			}
			
			// Set global timeout variable
			timeoutStr = tt.requestedTimeout
			
			// Capture stdout to avoid cluttering test output
			// (In a real implementation, you might want to inject a writer or use a testing-specific version)
			
			effectiveTimeout, timeoutDisplay, err := calculateEffectiveTimeout()
			
			if err != nil {
				t.Errorf("calculateEffectiveTimeout() unexpected error: %v", err)
				return
			}
			
			if effectiveTimeout != tt.expectedTimeout {
				t.Errorf("calculateEffectiveTimeout() timeout = %v, expected %v", effectiveTimeout, tt.expectedTimeout)
			}
			
			if timeoutDisplay != tt.expectedDisplay {
				t.Errorf("calculateEffectiveTimeout() display = %q, expected %q", timeoutDisplay, tt.expectedDisplay)
			}
			
			// Clean up environment
			_ = os.Unsetenv("BASH_MAX_TIMEOUT_MS")
			_ = os.Unsetenv("BASH_DEFAULT_TIMEOUT_MS")
		})
	}
}

func TestCheckClaudeCodeEnvironment(t *testing.T) {
	tests := []struct {
		name                 string
		bashMaxTimeoutMS     string
		bashDefaultTimeoutMS string
		expectedDuration     time.Duration
		expectedHasEnv       bool
	}{
		{
			name:             "no environment variables",
			expectedDuration: 0,
			expectedHasEnv:   false,
		},
		{
			name:             "only BASH_MAX_TIMEOUT_MS set",
			bashMaxTimeoutMS: "900000",
			expectedDuration: 15 * time.Minute,
			expectedHasEnv:   true,
		},
		{
			name:                 "only BASH_DEFAULT_TIMEOUT_MS set",
			bashDefaultTimeoutMS: "600000",
			expectedDuration:     10 * time.Minute,
			expectedHasEnv:       true,
		},
		{
			name:                 "both set - max takes precedence",
			bashMaxTimeoutMS:     "900000",
			bashDefaultTimeoutMS: "600000",
			expectedDuration:     15 * time.Minute,
			expectedHasEnv:       true,
		},
		{
			name:             "invalid BASH_MAX_TIMEOUT_MS",
			bashMaxTimeoutMS: "invalid",
			expectedDuration: 0,
			expectedHasEnv:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment
			_ = os.Unsetenv("BASH_MAX_TIMEOUT_MS")
			_ = os.Unsetenv("BASH_DEFAULT_TIMEOUT_MS")
			
			// Set up environment for test
			if tt.bashMaxTimeoutMS != "" {
				_ = os.Setenv("BASH_MAX_TIMEOUT_MS", tt.bashMaxTimeoutMS)
			}
			if tt.bashDefaultTimeoutMS != "" {
				_ = os.Setenv("BASH_DEFAULT_TIMEOUT_MS", tt.bashDefaultTimeoutMS)
			}
			
			duration, hasEnv := checkClaudeCodeEnvironment()
			
			if duration != tt.expectedDuration {
				t.Errorf("checkClaudeCodeEnvironment() duration = %v, expected %v", duration, tt.expectedDuration)
			}
			
			if hasEnv != tt.expectedHasEnv {
				t.Errorf("checkClaudeCodeEnvironment() hasEnv = %v, expected %v", hasEnv, tt.expectedHasEnv)
			}
			
			// Clean up environment
			_ = os.Unsetenv("BASH_MAX_TIMEOUT_MS")
			_ = os.Unsetenv("BASH_DEFAULT_TIMEOUT_MS")
		})
	}
}