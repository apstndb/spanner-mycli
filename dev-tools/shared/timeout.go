package shared

import (
	"fmt"
	"os"
	"time"
)

// Simplified timeout handling - delegates to Go's time.Duration except for zero value and environment constraints

// ParseTimeout parses timeout string using Go's time.ParseDuration
func ParseTimeout(timeoutStr string) (time.Duration, error) {
	if timeoutStr == "" {
		return 0, nil // Zero value for default behavior
	}
	
	// Use Go's standard duration parsing
	return time.ParseDuration(timeoutStr)
}

// GetClaudeCodeTimeout returns the effective timeout considering Claude Code constraints
func GetClaudeCodeTimeout(requested time.Duration) time.Duration {
	if requested == 0 {
		// Handle zero value - use environment default or fallback
		if defaultTimeout, _ := ParseClaudeCodeTimeoutEnv("BASH_DEFAULT_TIMEOUT_MS"); defaultTimeout > 0 {
			return defaultTimeout
		}
		return 5 * time.Minute // Fallback default
	}
	
	// Apply Claude Code maximum if configured
	if maxTimeout, _ := ParseClaudeCodeTimeoutEnv("BASH_MAX_TIMEOUT_MS"); maxTimeout > 0 && requested > maxTimeout {
		return maxTimeout
	}
	
	return requested
}

// Helper functions for Claude Code environment variables

// ParseClaudeCodeTimeoutEnv parses a Claude Code timeout environment variable
// Returns the duration and any parsing error. Returns 0, nil if variable is not set.
func ParseClaudeCodeTimeoutEnv(envVarName string) (time.Duration, error) {
	ms := os.Getenv(envVarName)
	if ms == "" {
		return 0, nil // Variable not set
	}
	
	// Use time.ParseDuration for consistent parsing
	duration, err := time.ParseDuration(ms + "ms")
	if err != nil {
		return 0, fmt.Errorf("invalid %s value %q: %w", envVarName, ms, err)
	}
	
	if duration < 0 {
		return 0, fmt.Errorf("invalid %s value %v: must be non-negative", envVarName, duration)
	}
	
	return duration, nil
}


// Legacy compatibility functions

// ParseTimeoutString - legacy wrapper for ParseTimeout
func ParseTimeoutString(timeoutStr string) (time.Duration, error) {
	return ParseTimeout(timeoutStr)
}

// CalculateTimeoutFromString - simplified version without complex result struct
func CalculateTimeoutFromString(timeoutStr string) (*TimeoutResult, error) {
	requested, err := ParseTimeout(timeoutStr)
	if err != nil {
		return nil, fmt.Errorf("invalid timeout format: %w", err)
	}
	
	effective := GetClaudeCodeTimeout(requested)
	
	return &TimeoutResult{
		Effective: effective,
		Display:   effective.String(), // Use Go's standard string representation
		Requested: requested,
	}, nil
}

// TimeoutResult - simplified version
type TimeoutResult struct {
	Effective time.Duration
	Display   string
	Requested time.Duration
}

func (tr *TimeoutResult) String() string {
	return tr.Display
}
