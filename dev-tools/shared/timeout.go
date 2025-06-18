package shared

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Timeout calculation logic to eliminate duplication across dev-tools.
// This provides consistent timeout handling with Claude Code environment integration.

// TimeoutResult holds calculated timeout information
type TimeoutResult struct {
	Effective     time.Duration // The actual timeout that will be used
	Display       string        // Human-readable display text
	Requested     time.Duration // The originally requested timeout
	IsConstrained bool          // Whether the timeout was constrained by environment
	HasMax        bool          // Whether a maximum timeout is configured
	HasDefault    bool          // Whether a default timeout is configured
}

// String returns a human-readable description of the timeout
func (tr *TimeoutResult) String() string {
	return tr.Display
}

// CalculateEffectiveTimeout determines the effective timeout considering Claude Code constraints
func CalculateEffectiveTimeout(requested time.Duration) (*TimeoutResult, error) {
	// Check Claude Code environment variables
	maxTimeout, hasMax := getClaudeCodeMaxTimeout()
	defaultTimeout, hasDefault := getClaudeCodeDefaultTimeout()
	
	effective := requested
	isConstrained := false
	
	// Apply maximum timeout constraint if configured
	if hasMax && requested > maxTimeout {
		effective = maxTimeout
		isConstrained = true
	}
	
	// Use default timeout if no specific timeout requested and default is configured
	if requested == 0 && hasDefault {
		effective = defaultTimeout
	}
	
	// Generate display text
	display := formatTimeoutDisplay(effective, requested, isConstrained)
	
	return &TimeoutResult{
		Effective:     effective,
		Display:       display,
		Requested:     requested,
		IsConstrained: isConstrained,
		HasMax:        hasMax,
		HasDefault:    hasDefault,
	}, nil
}

// ParseTimeoutString parses a timeout string into a duration
func ParseTimeoutString(timeoutStr string) (time.Duration, error) {
	if timeoutStr == "" {
		return 0, nil // Use default behavior
	}
	
	// Handle numeric-only input (assume minutes for backward compatibility)
	if parsed, err := strconv.Atoi(timeoutStr); err == nil {
		return time.Duration(parsed) * time.Minute, nil
	}
	
	// Parse as duration string
	return time.ParseDuration(timeoutStr)
}

// CalculateTimeoutFromString parses a string and calculates effective timeout
func CalculateTimeoutFromString(timeoutStr string) (*TimeoutResult, error) {
	requested, err := ParseTimeoutString(timeoutStr)
	if err != nil {
		return nil, fmt.Errorf("invalid timeout format: %w", err)
	}
	
	return CalculateEffectiveTimeout(requested)
}

// getClaudeCodeMaxTimeout retrieves the maximum timeout from Claude Code environment
func getClaudeCodeMaxTimeout() (time.Duration, bool) {
	maxTimeoutStr := os.Getenv("BASH_MAX_TIMEOUT_MS")
	if maxTimeoutStr == "" {
		return 0, false
	}
	
	if parsedTimeout, err := time.ParseDuration(maxTimeoutStr + "ms"); err == nil {
		return parsedTimeout, true
	}
	
	return 0, false
}

// getClaudeCodeDefaultTimeout retrieves the default timeout from Claude Code environment
func getClaudeCodeDefaultTimeout() (time.Duration, bool) {
	defaultTimeoutStr := os.Getenv("BASH_DEFAULT_TIMEOUT_MS")
	if defaultTimeoutStr == "" {
		return 0, false
	}
	
	if parsedTimeout, err := time.ParseDuration(defaultTimeoutStr + "ms"); err == nil {
		return parsedTimeout, true
	}
	
	return 0, false
}

// formatTimeoutDisplay creates a human-readable timeout display string
func formatTimeoutDisplay(effective, requested time.Duration, isConstrained bool) string {
	if effective == 0 {
		return "undefined"
	}
	
	// Round to reasonable precision for display
	switch {
	case effective >= time.Hour:
		hours := effective.Hours()
		if hours == float64(int(hours)) {
			return fmt.Sprintf("%.0fh", hours)
		}
		return fmt.Sprintf("%.1fh", hours)
	case effective >= time.Minute:
		minutes := effective.Minutes()
		if minutes == float64(int(minutes)) {
			return fmt.Sprintf("%.0fm", minutes)
		}
		return fmt.Sprintf("%.1fm", minutes)
	default:
		seconds := effective.Seconds()
		if seconds == float64(int(seconds)) {
			return fmt.Sprintf("%.0fs", seconds)
		}
		return fmt.Sprintf("%.1fs", seconds)
	}
}

// Common timeout configurations

// DefaultTimeout returns a sensible default timeout
func DefaultTimeout() time.Duration {
	return 5 * time.Minute
}

// ShortTimeout returns a short timeout for quick operations
func ShortTimeout() time.Duration {
	return 30 * time.Second
}

// LongTimeout returns a long timeout for extended operations
func LongTimeout() time.Duration {
	return 15 * time.Minute
}

// TimeoutWithContext creates a timeout result with context information
func TimeoutWithContext(duration time.Duration, context string) *TimeoutResult {
	display := fmt.Sprintf("%s (%s)", formatTimeoutDisplay(duration, duration, false), context)
	return &TimeoutResult{
		Effective: duration,
		Display:   display,
		Requested: duration,
	}
}

// TimeoutManager manages timeout operations for a session
type TimeoutManager struct {
	startTime time.Time
	timeout   time.Duration
}

// NewTimeoutManager creates a new timeout manager
func NewTimeoutManager(timeout time.Duration) *TimeoutManager {
	return &TimeoutManager{
		startTime: time.Now(),
		timeout:   timeout,
	}
}

// IsExpired checks if the timeout has been exceeded
func (tm *TimeoutManager) IsExpired() bool {
	return time.Since(tm.startTime) > tm.timeout
}

// Remaining returns the remaining time before timeout
func (tm *TimeoutManager) Remaining() time.Duration {
	elapsed := time.Since(tm.startTime)
	if elapsed >= tm.timeout {
		return 0
	}
	return tm.timeout - elapsed
}

// Elapsed returns the time elapsed since start
func (tm *TimeoutManager) Elapsed() time.Duration {
	return time.Since(tm.startTime)
}

// Progress returns the progress as a percentage (0.0 to 1.0)
func (tm *TimeoutManager) Progress() float64 {
	elapsed := time.Since(tm.startTime)
	if elapsed >= tm.timeout {
		return 1.0
	}
	return float64(elapsed) / float64(tm.timeout)
}

// Reset resets the timeout manager with a new start time
func (tm *TimeoutManager) Reset() {
	tm.startTime = time.Now()
}

// FormatRemaining returns a human-readable remaining time string
func (tm *TimeoutManager) FormatRemaining() string {
	remaining := tm.Remaining()
	if remaining == 0 {
		return "expired"
	}
	return formatTimeoutDisplay(remaining, remaining, false)
}