package shared

import (
	"fmt"
	"time"
)

// Typed message system for consistent user interface across dev-tools.
// This provides type-safe message formatting with consistent styling and behavior.

// MessageType represents different categories of messages
type MessageType int

const (
	StatusMessage MessageType = iota
	SuccessMessage
	ErrorMessage
	WarningMessage
	InfoMessage
	ProgressMessage
)

// MessageStyle holds styling information for different message types
type MessageStyle struct {
	Prefix string
	Color  string // Future: ANSI color codes
}

var messageStyles = map[MessageType]MessageStyle{
	StatusMessage:   {Prefix: "🔄", Color: "blue"},
	SuccessMessage:  {Prefix: "✅", Color: "green"},
	ErrorMessage:    {Prefix: "❌", Color: "red"},
	WarningMessage:  {Prefix: "⚠️", Color: "yellow"},
	InfoMessage:     {Prefix: "💡", Color: "cyan"},
	ProgressMessage: {Prefix: "⏳", Color: "yellow"},
}

// Message represents a typed message with formatting capabilities
type Message struct {
	Type     MessageType
	Template string
	Args     []interface{}
}

// NewMessage creates a new typed message
func NewMessage(msgType MessageType, template string, args ...interface{}) *Message {
	return &Message{
		Type:     msgType,
		Template: template,
		Args:     args,
	}
}

// String formats the message with appropriate styling
func (m *Message) String() string {
	style := messageStyles[m.Type]
	formatted := fmt.Sprintf(m.Template, m.Args...)
	return fmt.Sprintf("%s %s", style.Prefix, formatted)
}

// Print outputs the message to stdout
func (m *Message) Print() {
	fmt.Println(m.String())
}

// Printf outputs the message with additional formatting
func (m *Message) Printf(format string, args ...interface{}) {
	content := fmt.Sprintf(format, args...)
	style := messageStyles[m.Type]
	fmt.Printf("%s %s", style.Prefix, content)
}

// Common message constructors

// StatusMsg creates a status message (🔄)
func StatusMsg(template string, args ...interface{}) *Message {
	return NewMessage(StatusMessage, template, args...)
}

// SuccessMsg creates a success message (✅)
func SuccessMsg(template string, args ...interface{}) *Message {
	return NewMessage(SuccessMessage, template, args...)
}

// ErrorMsg creates an error message (❌)
func ErrorMsg(template string, args ...interface{}) *Message {
	return NewMessage(ErrorMessage, template, args...)
}

// WarningMsg creates a warning message (⚠️)
func WarningMsg(template string, args ...interface{}) *Message {
	return NewMessage(WarningMessage, template, args...)
}

// InfoMsg creates an info message (💡)
func InfoMsg(template string, args ...interface{}) *Message {
	return NewMessage(InfoMessage, template, args...)
}

// ProgressMsg creates a progress message (⏳)
func ProgressMsg(template string, args ...interface{}) *Message {
	return NewMessage(ProgressMessage, template, args...)
}

// Specialized message types for common patterns

// ReviewStatus represents the status of review monitoring
type ReviewStatus struct {
	PRNumber     string
	ReviewsReady bool
	ChecksReady  bool
	Timestamp    string
	TimeRemaining string
}

// String formats the review status message
func (rs *ReviewStatus) String() string {
	return fmt.Sprintf("[%s] Status: Reviews: %v, Checks: %v (remaining: %s)",
		rs.Timestamp, rs.ReviewsReady, rs.ChecksReady, rs.TimeRemaining)
}

// Print outputs the review status
func (rs *ReviewStatus) Print() {
	fmt.Println(rs.String())
}

// ReviewSummary represents a summary of found reviews
type ReviewSummary struct {
	Count    int
	NewCount int
	IsNew    bool
}

// String formats the review summary message
func (rs *ReviewSummary) String() string {
	if rs.IsNew && rs.NewCount > 0 {
		return fmt.Sprintf("🆕 Found %d new review(s)", rs.NewCount)
	}
	return fmt.Sprintf("📋 Found %d review(s) total", rs.Count)
}

// Print outputs the review summary
func (rs *ReviewSummary) Print() {
	fmt.Println(rs.String())
}

// TimeoutInfo represents timeout-related information
type TimeoutInfo struct {
	Duration    time.Duration
	DisplayText string
	Effective   bool // Whether this is the effective timeout (vs requested)
}

// String formats the timeout information
func (ti *TimeoutInfo) String() string {
	if ti.Effective {
		return fmt.Sprintf("⏰ Timeout reached (%v)", ti.Duration)
	}
	return fmt.Sprintf("Timeout: %s", ti.DisplayText)
}

// MergeStatus represents the merge status of a PR
type MergeStatus struct {
	Mergeable string
	Status    string
}

// String formats the merge status message with appropriate styling
func (ms *MergeStatus) String() string {
	switch ms.Mergeable {
	case "MERGEABLE":
		return "✅ Merge: Ready"
	case "CONFLICTING":
		return "❌ Merge: Conflicts"
	case "UNKNOWN":
		return "⏳ Merge: Checking..."
	default:
		return fmt.Sprintf("🚫 Merge: %s (status: %s)", ms.Mergeable, ms.Status)
	}
}

// CheckStatus represents the status of CI checks
type CheckStatus struct {
	State    string
	HasChecks bool
}

// String formats the check status message with appropriate styling
func (cs *CheckStatus) String() string {
	if !cs.HasChecks {
		return "✅ Checks: No checks required"
	}
	
	switch cs.State {
	case "SUCCESS":
		return "✅ Checks: All passed"
	case "FAILURE":
		return "❌ Checks: Some failed"
	case "ERROR":
		return "🚨 Checks: Error occurred"
	case "PENDING":
		return "⏳ Checks: Running..."
	default:
		return fmt.Sprintf("✅ Checks: Completed (%s)", cs.State)
	}
}

// GuidanceMessage represents helpful guidance for users
type GuidanceMessage struct {
	Action      string
	Command     string
	Description string
}

// String formats the guidance message
func (gm *GuidanceMessage) String() string {
	if gm.Command != "" {
		return fmt.Sprintf("💡 %s: %s", gm.Action, gm.Command)
	}
	return fmt.Sprintf("💡 %s", gm.Description)
}

// Print outputs the guidance message
func (gm *GuidanceMessage) Print() {
	fmt.Println(gm.String())
}

// Common guidance messages
func ExtendTimeoutGuidance() *GuidanceMessage {
	return &GuidanceMessage{
		Action:      "To extend timeout",
		Description: "set BASH_MAX_TIMEOUT_MS in ~/.claude/settings.json",
	}
}

func ManualRetryGuidance(prNumber string, timeout interface{}) *GuidanceMessage {
	return &GuidanceMessage{
		Action:  "Manual retry",
		Command: fmt.Sprintf("bin/gh-helper reviews wait %s --timeout=%v", prNumber, timeout),
	}
}

func ListThreadsGuidance(prNumber string) *GuidanceMessage {
	return &GuidanceMessage{
		Action:  "To list unresolved threads",
		Command: fmt.Sprintf("bin/gh-helper threads list %s", prNumber),
	}
}

// Batch message operations

// MessageGroup represents a collection of related messages
type MessageGroup struct {
	Title    string
	Messages []*Message
}

// NewMessageGroup creates a new message group
func NewMessageGroup(title string) *MessageGroup {
	return &MessageGroup{
		Title:    title,
		Messages: make([]*Message, 0),
	}
}

// Add adds a message to the group
func (mg *MessageGroup) Add(msg *Message) {
	mg.Messages = append(mg.Messages, msg)
}

// Print outputs all messages in the group
func (mg *MessageGroup) Print() {
	if mg.Title != "" {
		fmt.Printf("\n=== %s ===\n", mg.Title)
	}
	for _, msg := range mg.Messages {
		msg.Print()
	}
	if mg.Title != "" {
		fmt.Println()
	}
}