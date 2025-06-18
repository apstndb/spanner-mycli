package shared

import "fmt"

// Output message constants to eliminate duplication across dev-tools.
// These provide consistent user-facing messages throughout the codebase.

// Status and progress messages
const (
	// Review-related messages
	MsgCheckingReviews    = "🔄 Checking reviews for PR #%s in %s/%s..."
	MsgNoReviewsFound     = "No reviews found"
	MsgFoundReviews       = "📋 Found %d review(s) total:"
	MsgNewReviewsFound    = "🆕 Found %d new review(s):"
	MsgNoNewReviews       = "✅ No new reviews since last check"
	MsgNewReviewDetected  = "🎉 New review detected from %s at %s"
	MsgNewReviewsReady    = "🎉 Found %d review(s)"
	
	// Waiting and monitoring messages
	MsgWaitingForReviews  = "🔄 Waiting for reviews on PR #%s (timeout: %s)..."
	MsgWaitingForBoth     = "🔄 Waiting for both reviews AND PR checks for PR #%s (timeout: %s)..."
	MsgWaitingStatus      = "[%s] Status: Reviews: %v, Checks: %v (remaining: %v)"
	MsgMonitoringStarted  = "[%s] Monitoring started."
	MsgPressCtrlC         = "Press Ctrl+C to stop monitoring"
	
	// Status check messages
	MsgChecksPending      = "⏳ Checking..."
	MsgChecksAllPassed    = "✅ Checks: All passed"
	MsgChecksSomeFailed   = "❌ Checks: Some failed"
	MsgChecksError        = "🚨 Checks: Error occurred"
	MsgChecksCompleted    = "✅ Checks: Completed (%s)"
	MsgNoChecksRequired   = "✅ Checks: No checks required"
	
	// Merge status messages
	MsgMergeReady         = "✅ Merge: Ready"
	MsgMergeConflicts     = "❌ Merge: Conflicts"
	MsgMergeBlocked       = "🚫 Merge: Blocked"
	MsgMergeChecking      = "⏳ Merge: Checking..."
	
	// Timeout and completion messages
	MsgTimeoutReached     = "⏰ Timeout reached (%v)."
	MsgTimeoutNoReviews   = "⏰ Timeout reached (%v). No new reviews found."
	MsgReviewCheckComplete = "✅ Review check complete"
	MsgBothCompleted      = "✅ Both reviews and checks completed!"
	
	// Error and warning messages
	MsgMergeConflictsDetected = "❌ [%s] PR has merge conflicts (status: %s)"
	MsgCIWontRun             = "⚠️  CI checks will not run until conflicts are resolved"
	MsgResolveConflicts      = "💡 Resolve conflicts with: git rebase origin/main"
	MsgConflictsPreventCI    = "⚠️  Merge conflicts detected - CI may not run until resolved"
	MsgResolveAndPush        = "💡 Resolve conflicts and push to trigger CI checks"
	
	// Guidance messages
	MsgListThreads        = "💡 To list unresolved threads: bin/gh-helper threads list %s"
	MsgImportantRead      = "⚠️  IMPORTANT: Please read the review feedback carefully before proceeding"
	MsgExtendTimeout      = "💡 To extend timeout, set BASH_MAX_TIMEOUT_MS in ~/.claude/settings.json"
	MsgTimeoutExample     = "💡 Example: {\"env\": {\"BASH_MAX_TIMEOUT_MS\": \"900000\"}} for 15 minutes"
	MsgManualRetry        = "💡 Manual retry: bin/gh-helper reviews wait %s --timeout=%v"
	MsgContinueWaiting    = "💡 To continue waiting, run: bin/gh-helper reviews wait %s"
	
	// State tracking messages
	MsgTrackingReviews    = "📊 Tracking reviews since: %s"
	MsgLastKnownReview    = "Last known review: %s at %s"
	MsgUpdatedState       = "💾 Updated state: Latest review %s at %s"
	
	// Signal handling messages
	MsgReceivedSignal     = "🛑 Received signal %v - terminating gracefully"
	MsgClaudeCodeInterrupted = "💡 Claude Code timeout interrupted. To continue, run:"
)

// Message formatting helpers

// FormatCheckingReviews formats a "checking reviews" message
func FormatCheckingReviews(prNumber, owner, repo string) string {
	return fmt.Sprintf(MsgCheckingReviews, prNumber, owner, repo)
}

// FormatFoundReviews formats a "found reviews" message
func FormatFoundReviews(count int) string {
	return fmt.Sprintf(MsgFoundReviews, count)
}

// FormatNewReviewsFound formats a "new reviews found" message
func FormatNewReviewsFound(count int) string {
	return fmt.Sprintf(MsgNewReviewsFound, count)
}

// FormatNewReviewDetected formats a "new review detected" message
func FormatNewReviewDetected(author, createdAt string) string {
	return fmt.Sprintf(MsgNewReviewDetected, author, createdAt)
}

// FormatWaitingForReviews formats a "waiting for reviews" message
func FormatWaitingForReviews(prNumber, timeout string) string {
	return fmt.Sprintf(MsgWaitingForReviews, prNumber, timeout)
}

// FormatWaitingForBoth formats a "waiting for both" message
func FormatWaitingForBoth(prNumber, timeout string) string {
	return fmt.Sprintf(MsgWaitingForBoth, prNumber, timeout)
}

// FormatWaitingStatus formats a waiting status message
func FormatWaitingStatus(timestamp string, reviewsReady, checksComplete bool, remaining string) string {
	return fmt.Sprintf(MsgWaitingStatus, timestamp, reviewsReady, checksComplete, remaining)
}

// FormatMonitoringStarted formats a monitoring started message
func FormatMonitoringStarted(timestamp string) string {
	return fmt.Sprintf(MsgMonitoringStarted, timestamp)
}

// FormatMergeConflictsDetected formats a merge conflicts detected message
func FormatMergeConflictsDetected(timestamp, status string) string {
	return fmt.Sprintf(MsgMergeConflictsDetected, timestamp, status)
}

// FormatListThreads formats a list threads guidance message
func FormatListThreads(prNumber string) string {
	return fmt.Sprintf(MsgListThreads, prNumber)
}

// FormatManualRetry formats a manual retry guidance message
func FormatManualRetry(prNumber string, timeout interface{}) string {
	return fmt.Sprintf(MsgManualRetry, prNumber, timeout)
}

// FormatContinueWaiting formats a continue waiting guidance message
func FormatContinueWaiting(prNumber string) string {
	return fmt.Sprintf(MsgContinueWaiting, prNumber)
}