package shared

import "fmt"

// StatusState/CheckStatusState enum values are identical across GraphQL types
// This unified system handles both types with the same formatting logic

// StatusStateMessages provides text-only status messages for StatusState/CheckStatusState enums
var StatusStateMessages = map[string]string{
	"SUCCESS": "All passed",
	"FAILURE": "Some failed",
	"ERROR":   "Error occurred", 
	"PENDING": "Still running",
}

// StatusStateIconMessages provides emoji-enhanced status messages
var StatusStateIconMessages = map[string]string{
	"SUCCESS": "✅ All passed",
	"FAILURE": "❌ Some failed",
	"ERROR":   "🚨 Error occurred",
	"PENDING": "⏳ Still running",
}

// FormatStatusState returns a formatted status message for StatusState/CheckStatusState enums
// withIcon determines whether to include emoji icons in the message
func FormatStatusState(state string, withIcon bool) string {
	if withIcon {
		if msg, exists := StatusStateIconMessages[state]; exists {
			return msg
		}
		// Fallback with default icon for unknown states
		return fmt.Sprintf("✅ Completed (%s)", state)
	}
	
	// Simple text without icons
	if msg, exists := StatusStateMessages[state]; exists {
		return msg
	}
	// Fallback to raw state for unknown values
	return state
}

// FormatStatusStateWithPrefix returns a formatted status message with a custom prefix
func FormatStatusStateWithPrefix(state, prefix string, withIcon bool) string {
	baseMessage := FormatStatusState(state, withIcon)
	if prefix == "" {
		return baseMessage
	}
	return fmt.Sprintf("%s: %s", prefix, baseMessage)
}

// IsStatusComplete checks if a status represents a completed state (not pending)
func IsStatusComplete(state string) bool {
	return state == "SUCCESS" || state == "FAILURE" || state == "ERROR"
}

// IsStatusSuccess checks if a status represents a successful state
func IsStatusSuccess(state string) bool {
	return state == "SUCCESS"
}

// IsStatusFailure checks if a status represents a failure or error state
func IsStatusFailure(state string) bool {
	return state == "FAILURE" || state == "ERROR"
}