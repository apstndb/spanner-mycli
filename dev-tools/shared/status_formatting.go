package shared

import "fmt"

// GitHub GraphQL status enums have different values and contexts:
// StatusState: EXPECTED, ERROR, FAILURE, PENDING, SUCCESS (commit status contexts)
// CheckStatusState: REQUESTED, QUEUED, IN_PROGRESS, COMPLETED, WAITING, PENDING (check run status)
// CheckConclusionState: ACTION_REQUIRED, TIMED_OUT, CANCELLED, FAILURE, SUCCESS, NEUTRAL, SKIPPED, STARTUP_FAILURE, STALE (check run conclusion)
// This module provides unified formatting for all status-like enum values

// StatusInfo contains message and icon for a status value
type StatusInfo struct {
	Message string
	Icon    string
}

// Unified status information for all GitHub status enums
// Values with the same meaning across different enums are shared
var UnifiedStatusInfo = map[string]StatusInfo{
	// Common success states
	"SUCCESS":   {Message: "Success", Icon: "✅"},
	"COMPLETED": {Message: "Completed", Icon: "✅"},
	
	// Common failure states  
	"FAILURE": {Message: "Failure", Icon: "❌"},
	"ERROR":   {Message: "Error", Icon: "🚨"},
	
	// Common pending/waiting states
	"PENDING":     {Message: "Pending", Icon: "⏳"},
	"IN_PROGRESS": {Message: "In progress", Icon: "⏳"},
	"QUEUED":      {Message: "Queued", Icon: "⏳"},
	"REQUESTED":   {Message: "Requested", Icon: "⏳"},
	"WAITING":     {Message: "Waiting", Icon: "⏳"},
	"EXPECTED":    {Message: "Expected", Icon: "⏳"},
	
	// Check conclusion specific states
	"NEUTRAL":         {Message: "Neutral", Icon: "❔"},
	"CANCELLED":       {Message: "Cancelled", Icon: "🚫"},
	"SKIPPED":         {Message: "Skipped", Icon: "⏭️"},
	"TIMED_OUT":       {Message: "Timed out", Icon: "⏰"},
	"ACTION_REQUIRED": {Message: "Action required", Icon: "⚠️"},
	"STARTUP_FAILURE": {Message: "Startup failure", Icon: "🚨"},
	"STALE":           {Message: "Stale", Icon: "🔄"},
}

// formatStatus returns a formatted status message using StatusInfo map
func formatStatus(state string, statusMap map[string]StatusInfo, withIcon bool) string {
	if info, exists := statusMap[state]; exists {
		if withIcon {
			return fmt.Sprintf("%s %s", info.Icon, info.Message)
		}
		return info.Message
	}
	
	// Fallback for unknown state
	if withIcon {
		return fmt.Sprintf("❓ Unknown (%s)", state)
	}
	return fmt.Sprintf("Unknown (%s)", state)
}

// FormatStatus returns a formatted status message for any GitHub status enum
// This unified function works with StatusState, CheckStatusState, and CheckConclusionState
func FormatStatus(state string, withIcon bool) string {
	return formatStatus(state, UnifiedStatusInfo, withIcon)
}

// FormatStatusWithPrefix returns a formatted status message with a custom prefix
func FormatStatusWithPrefix(state, prefix string, withIcon bool) string {
	baseMessage := FormatStatus(state, withIcon)
	if prefix == "" {
		return baseMessage
	}
	return fmt.Sprintf("%s: %s", prefix, baseMessage)
}

// Unified status checking functions that work across all enum types

// IsStatusComplete checks if any status represents a completed/final state
func IsStatusComplete(state string) bool {
	// Success states
	if state == "SUCCESS" || state == "COMPLETED" {
		return true
	}
	// Failure/error states
	if state == "FAILURE" || state == "ERROR" || state == "STARTUP_FAILURE" || state == "TIMED_OUT" {
		return true
	}
	// Other final states
	if state == "CANCELLED" || state == "SKIPPED" || state == "NEUTRAL" {
		return true
	}
	return false
}

// IsStatusSuccess checks if any status represents a successful state
func IsStatusSuccess(state string) bool {
	return state == "SUCCESS" || state == "COMPLETED"
}

// IsStatusFailure checks if any status represents a failure/error state
func IsStatusFailure(state string) bool {
	return state == "FAILURE" || state == "ERROR" || state == "STARTUP_FAILURE" || state == "TIMED_OUT"
}

// IsStatusPending checks if any status represents a pending/in-progress state
func IsStatusPending(state string) bool {
	return state == "PENDING" || state == "IN_PROGRESS" || state == "QUEUED" || 
		   state == "REQUESTED" || state == "WAITING" || state == "EXPECTED"
}