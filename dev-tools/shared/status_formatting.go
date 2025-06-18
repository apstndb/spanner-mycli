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

// StatusStateInfo provides message and icon information for StatusState enum
// StatusState: EXPECTED, ERROR, FAILURE, PENDING, SUCCESS (commit status contexts)
var StatusStateInfo = map[string]StatusInfo{
	"SUCCESS":  {Message: "Success", Icon: "✅"},
	"FAILURE":  {Message: "Failure", Icon: "❌"},
	"ERROR":    {Message: "Error", Icon: "🚨"},
	"PENDING":  {Message: "Pending", Icon: "⏳"},
	"EXPECTED": {Message: "Expected", Icon: "⏳"},
}

// CheckStatusStateInfo provides message and icon information for CheckStatusState enum
// CheckStatusState: REQUESTED, QUEUED, IN_PROGRESS, COMPLETED, WAITING, PENDING (check run status)
var CheckStatusStateInfo = map[string]StatusInfo{
	"COMPLETED":   {Message: "Completed", Icon: "✅"},
	"IN_PROGRESS": {Message: "In progress", Icon: "⏳"},
	"PENDING":     {Message: "Pending", Icon: "⏳"},
	"QUEUED":      {Message: "Queued", Icon: "⏳"},
	"REQUESTED":   {Message: "Requested", Icon: "⏳"},
	"WAITING":     {Message: "Waiting", Icon: "⏳"},
}

// CheckConclusionStateInfo provides message and icon information for CheckConclusionState enum  
// CheckConclusionState: ACTION_REQUIRED, TIMED_OUT, CANCELLED, FAILURE, SUCCESS, NEUTRAL, SKIPPED, STARTUP_FAILURE, STALE (check run conclusion)
var CheckConclusionStateInfo = map[string]StatusInfo{
	"SUCCESS":         {Message: "Success", Icon: "✅"},
	"FAILURE":         {Message: "Failure", Icon: "❌"},
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

// FormatStatusState returns a formatted status message for StatusState enum
func FormatStatusState(state string, withIcon bool) string {
	return formatStatus(state, StatusStateInfo, withIcon)
}

// FormatCheckStatusState returns a formatted status message for CheckStatusState enum
func FormatCheckStatusState(state string, withIcon bool) string {
	return formatStatus(state, CheckStatusStateInfo, withIcon)
}

// FormatCheckConclusionState returns a formatted status message for CheckConclusionState enum
func FormatCheckConclusionState(state string, withIcon bool) string {
	return formatStatus(state, CheckConclusionStateInfo, withIcon)
}

// FormatStatusStateWithPrefix returns a formatted StatusState message with a custom prefix
func FormatStatusStateWithPrefix(state, prefix string, withIcon bool) string {
	baseMessage := FormatStatusState(state, withIcon)
	if prefix == "" {
		return baseMessage
	}
	return fmt.Sprintf("%s: %s", prefix, baseMessage)
}

// Note: Status checking can be done with map existence checks:
// _, exists := StatusStateInfo[state]         // Check if valid StatusState
// _, exists := CheckStatusStateInfo[state]    // Check if valid CheckStatusState  
// _, exists := CheckConclusionStateInfo[state] // Check if valid CheckConclusionState