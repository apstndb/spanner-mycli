package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/apstndb/spanner-mycli/dev-tools/shared"
	"github.com/spf13/cobra"
)

// newOperationalCommand creates a command optimized for operational tasks
// All operational commands should silence usage help since most errors are runtime issues
func newOperationalCommand(use, short, long string, runE func(*cobra.Command, []string) error) *cobra.Command {
	return &cobra.Command{
		Use:          use,
		Short:        short,
		Long:         long,
		SilenceUsage: true, // Don't show usage help for operational errors (API failures, etc.)
		RunE:         runE,
	}
}

var rootCmd = &cobra.Command{
	Use:   "gh-helper",
	Short: "Generic GitHub operations helper",
	Long: `Generic GitHub operations optimized for AI assistants.

COMMON PATTERNS:
  gh-helper reviews wait <PR> --request-review     # Complete review workflow
  gh-helper threads list <PR>                      # Show review threads
  gh-helper threads reply <THREAD_ID> --message "Fixed in commit abc123"

See cmd/gh-helper/README.md for detailed documentation, design rationale,
and migration guide from shell scripts.`,
	// Cobra assumes all errors are usage errors and shows help by default
	// For operational tools, most errors are runtime issues (API failures, merge conflicts)
	// not syntax errors, so we disable automatic error printing
	SilenceErrors: true,
}

var reviewsCmd = &cobra.Command{
	Use:   "reviews",
	Short: "GitHub Pull Request review operations",
}

var threadsCmd = &cobra.Command{
	Use:   "threads",
	Short: "GitHub review thread operations",
}

var checkReviewsCmd = newOperationalCommand(
	"check [pr-number-or-issue]",
	"Check for new PR reviews with state tracking",
	`Check for new pull request reviews, tracking state to identify updates.

This command maintains state in ~/.cache/spanner-mycli-reviews/ to detect
new reviews since the last check. Useful for monitoring PR activity.

Arguments:
- No argument: Uses current branch's PR
- Plain number (123): Auto-detects issue vs PR
- Explicit issue (issues/123, issue/123): Forces issue resolution
- Explicit PR (pull/123, pr/123): Forces PR usage`,
	checkReviews,
)

var waitReviewsCmd = newOperationalCommand(
	"wait [pr-number-or-issue]",
	"Wait for both reviews and PR checks (default behavior)",
	`Continuously monitor for both new reviews AND PR checks completion by default.

This command polls every 30 seconds and waits until BOTH conditions are met:
1. New reviews are available
2. All PR checks have completed (success, failure, or cancelled)

Use --exclude-reviews to wait for PR checks only.
Use --exclude-checks to wait for reviews only.
Use --request-review to automatically request Gemini review before waiting.

Arguments:
- No argument: Uses current branch's PR
- Plain number (123): Auto-detects issue vs PR
- Explicit issue (issues/123, issue/123): Forces issue resolution
- Explicit PR (pull/123, pr/123): Forces PR usage

AI-FRIENDLY: Designed for autonomous workflows that need complete feedback.
Default timeout is 5 minutes, configurable with --timeout flag.`,
	waitForReviews,
)

var waitAllCmd = newOperationalCommand(
	"wait-all <pr-number>",
	"Wait for both reviews and PR checks completion",
	`Continuously monitor for both new reviews AND PR check completion.

This command polls every 30 seconds and waits until BOTH conditions are met:
1. New reviews are available (same as 'reviews wait')
2. All PR checks have completed (success, failure, or cancelled)

This is useful for waiting until both Gemini review feedback AND CI checks
are complete before proceeding with next steps.

Use --request-review to automatically request Gemini review before waiting.
This is useful for post-push scenarios where you need to trigger review.

AI-FRIENDLY: Designed for autonomous workflows that need both review and CI feedback.
Default timeout is 5 minutes, configurable with --timeout flag.`,
	waitForReviewsAndChecks,
)

var listThreadsCmd = newOperationalCommand(
	"list <pr-number>",
	"List unresolved review threads",
	`List unresolved review threads that may need replies.

Shows thread IDs for use with the reply command, along with
file locations and latest comments.`,
	listThreads,
)

var replyThreadsCmd = newOperationalCommand(
	"reply <thread-id>",
	"Reply to a review thread",
	`Reply to a GitHub pull request review thread.

AI-FRIENDLY DESIGN (Issue #301): The reply text can be provided via:
- --message flag for single-line responses
- stdin for multi-line content or heredoc (preferred by AI assistants)

Examples:
  gh-helper threads reply PRRT_kwDONC6gMM5SU-GH --message "Fixed as suggested"
  echo "Thank you for the feedback!" | gh-helper threads reply PRRT_kwDONC6gMM5SU-GH
  gh-helper threads reply PRRT_kwDONC6gMM5SU-GH <<EOF
  Fixed the issue. The implementation now:
  - Handles edge cases properly
  - Includes proper error handling
  EOF`,
	replyToThread,
)

var showThreadCmd = newOperationalCommand(
	"show <thread-id>",
	"Show detailed view of a review thread",
	`Show detailed view of a specific review thread including all comments.

This provides full context for understanding review feedback before replying.
Useful for getting complete thread history and comment details.`,
	showThread,
)

var replyWithCommitCmd = newOperationalCommand(
	"reply-commit <thread-id> <commit-hash>",
	"Reply to a review thread with commit reference",
	`Reply to a review thread indicating that fixes were made in a specific commit.

This command automatically formats the reply to include the commit hash,
following best practices for review response traceability.

The reply text can be provided via --message flag or stdin.`,
	replyWithCommit,
)

var (
	owner          string
	repo           string
	message        string
	mentionUser    string
	timeoutStr     string
	timeout        int  // Deprecated: for backward compatibility only
	requestReview  bool
	excludeReviews bool
	excludeChecks  bool
)

func init() {
	// Configure Args for operational commands (using newOperationalCommand)
	checkReviewsCmd.Args = cobra.MaximumNArgs(1)
	waitReviewsCmd.Args = cobra.MaximumNArgs(1)
	waitAllCmd.Args = cobra.ExactArgs(1)
	listThreadsCmd.Args = cobra.ExactArgs(1)
	replyThreadsCmd.Args = cobra.ExactArgs(1)
	showThreadCmd.Args = cobra.ExactArgs(1)
	replyWithCommitCmd.Args = cobra.ExactArgs(2)
	
	// Configure flags
	rootCmd.PersistentFlags().StringVar(&owner, "owner", shared.DefaultOwner, "GitHub repository owner")
	rootCmd.PersistentFlags().StringVar(&repo, "repo", shared.DefaultRepo, "GitHub repository name")

	replyThreadsCmd.Flags().StringVar(&message, "message", "", "Reply message (or use stdin)")
	replyThreadsCmd.Flags().StringVar(&mentionUser, "mention", "", "Username to mention (without @)")

	waitReviewsCmd.Flags().StringVar(&timeoutStr, "timeout", "5m", "Timeout duration (e.g., 90s, 1.5m, 2m30s) (default: 5m)")
	waitReviewsCmd.Flags().IntVar(&timeout, "timeout-minutes", 0, "Deprecated: use --timeout with duration format")
	waitReviewsCmd.Flags().BoolVar(&excludeReviews, "exclude-reviews", false, "Exclude reviews, wait for PR checks only")
	waitReviewsCmd.Flags().BoolVar(&excludeChecks, "exclude-checks", false, "Exclude checks, wait for reviews only")
	waitReviewsCmd.Flags().BoolVar(&requestReview, "request-review", false, "Request Gemini review before waiting")
	
	waitAllCmd.Flags().StringVar(&timeoutStr, "timeout", "5m", "Timeout duration (e.g., 90s, 1.5m, 2m30s) (default: 5m)")
	waitAllCmd.Flags().IntVar(&timeout, "timeout-minutes", 0, "Deprecated: use --timeout with duration format")
	waitAllCmd.Flags().BoolVar(&requestReview, "request-review", false, "Request Gemini review before waiting")

	// Add subcommands
	reviewsCmd.AddCommand(checkReviewsCmd, waitReviewsCmd, waitAllCmd)
	threadsCmd.AddCommand(listThreadsCmd, replyThreadsCmd, showThreadCmd, replyWithCommitCmd)
	rootCmd.AddCommand(reviewsCmd, threadsCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// parseTimeout parses timeout from string format or falls back to deprecated minutes format
func parseTimeout() (time.Duration, error) {
	// If new timeout format is provided, use it
	if timeoutStr != "" && timeoutStr != "5m" {
		duration, err := time.ParseDuration(timeoutStr)
		if err != nil {
			return 0, fmt.Errorf("invalid timeout format '%s'. Use formats like: 30s, 1.5m, 2m30s, 15m", timeoutStr)
		}
		return duration, nil
	}
	
	// If deprecated timeout-minutes is used, convert to duration
	if timeout > 0 {
		fmt.Printf("⚠️  --timeout-minutes is deprecated. Use --timeout=%dm instead.\n", timeout)
		return time.Duration(timeout) * time.Minute, nil
	}
	
	// Default case: parse the default string
	duration, err := time.ParseDuration(timeoutStr)
	if err != nil {
		return 0, fmt.Errorf("invalid default timeout format '%s': %w", timeoutStr, err)
	}
	return duration, nil
}

// getCurrentUser returns the current authenticated GitHub username
func getCurrentUser() (string, error) {
	client := shared.NewGitHubClient(owner, repo)
	return client.GetCurrentUser()
}

// ReviewState represents the state of the last known review
type ReviewState struct {
	ID        string `json:"id"`
	CreatedAt string `json:"createdAt"`
}

// loadReviewState loads the last known review state from cache
func loadReviewState(prNumber string) (*ReviewState, error) {
	stateDir := fmt.Sprintf("%s/.cache/spanner-mycli-reviews", os.Getenv("HOME"))
	lastReviewFile := fmt.Sprintf("%s/pr-%s-last-review.json", stateDir, prNumber)
	
	data, err := os.ReadFile(lastReviewFile)
	if err != nil {
		return nil, err
	}
	
	var state ReviewState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	
	return &state, nil
}

// saveReviewState saves the review state to cache
func saveReviewState(prNumber string, state ReviewState) error {
	stateDir := fmt.Sprintf("%s/.cache/spanner-mycli-reviews", os.Getenv("HOME"))
	lastReviewFile := fmt.Sprintf("%s/pr-%s-last-review.json", stateDir, prNumber)
	
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}
	
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}
	
	if err := os.WriteFile(lastReviewFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}
	
	return nil
}

// hasNewReviews checks if there are new reviews since the last known state
func hasNewReviews(reviews []struct {
	ID        string `json:"id"`
	Author    struct {
		Login string `json:"login"`
	} `json:"author"`
	CreatedAt string `json:"createdAt"`
	State     string `json:"state"`
	Body      string `json:"body"`
}, lastState *ReviewState) bool {
	if lastState == nil {
		// No previous state, any review is "new"
		return len(reviews) > 0
	}
	
	for _, review := range reviews {
		if review.CreatedAt > lastState.CreatedAt ||
			(review.CreatedAt == lastState.CreatedAt && review.ID != lastState.ID) {
			return true
		}
	}
	
	return false
}

// checkClaudeCodeEnvironment checks for Claude Code timeout environment variables
// and provides guidance based on GitHub issues research.
//
// Key findings from anthropics/claude-code#1039, anthropics/claude-code#1216, anthropics/claude-code#1717:
// - BASH_MAX_TIMEOUT_MS: Upper limit for explicit timeout requests (our use case)
// - BASH_DEFAULT_TIMEOUT_MS: Default timeout when no explicit timeout specified  
// - Claude Code defaults to 2-minute hard limit when no env vars are set
// - Environment variables are read from ~/.claude/settings.json or project .claude/settings.json
// - Project settings should be committed, local settings (.claude/settings.local.json) should not
func checkClaudeCodeEnvironment() (time.Duration, bool) {
	var maxTimeout, defaultTimeout time.Duration
	var hasMax, hasDefault bool
	
	// Check for BASH_MAX_TIMEOUT_MS (upper limit for explicit timeouts)
	if maxTimeoutStr := os.Getenv("BASH_MAX_TIMEOUT_MS"); maxTimeoutStr != "" {
		if maxTimeoutMs, err := time.ParseDuration(maxTimeoutStr + "ms"); err == nil {
			fmt.Printf("🔧 Claude Code BASH_MAX_TIMEOUT_MS detected: %v\n", maxTimeoutMs)
			maxTimeout = maxTimeoutMs
			hasMax = true
		}
	}
	
	// Check for BASH_DEFAULT_TIMEOUT_MS (default when no timeout specified)
	if defaultTimeoutStr := os.Getenv("BASH_DEFAULT_TIMEOUT_MS"); defaultTimeoutStr != "" {
		if defaultTimeoutMs, err := time.ParseDuration(defaultTimeoutStr + "ms"); err == nil {
			fmt.Printf("🔧 Claude Code BASH_DEFAULT_TIMEOUT_MS detected: %v\n", defaultTimeoutMs)
			defaultTimeout = defaultTimeoutMs
			hasDefault = true
		}
	}
	
	// For explicit timeout requests (our use case), BASH_MAX_TIMEOUT_MS takes precedence
	if hasMax {
		return maxTimeout, true
	}
	if hasDefault {
		return defaultTimeout, true
	}
	
	return 0, false
}

func checkReviews(cmd *cobra.Command, args []string) error {
	// Resolve PR number using smart resolution
	var input string
	if len(args) > 0 {
		input = args[0]
	}
	
	client := shared.NewGitHubClient(owner, repo)
	prNumber, resolveMessage, err := client.ResolvePRNumber(input)
	if err != nil {
		return fmt.Errorf("failed to resolve PR number: %w", err)
	}
	
	fmt.Printf("🔍 %s\n", resolveMessage)
	prNumberStr := fmt.Sprintf("%d", prNumber)

	stateDir := fmt.Sprintf("%s/.cache/spanner-mycli-reviews", os.Getenv("HOME"))

	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	fmt.Printf("Checking reviews for PR #%s in %s/%s...\n", prNumberStr, owner, repo)

	query := fmt.Sprintf(`
query {
  repository(owner: "%s", name: "%s") {
    pullRequest(number: %s) {
      reviews(last: 15) {
        nodes {
          id
          author {
            login
          }
          createdAt
          state
          body
        }
      }
    }
  }
}`, owner, repo, prNumber)

	result, err := runGraphQLQuery(query)
	if err != nil {
		return fmt.Errorf("failed to fetch reviews: %w", err)
	}

	var response struct {
		Data struct {
			Repository struct {
				PullRequest struct {
					Reviews struct {
						Nodes []struct {
							ID        string `json:"id"`
							Author    struct {
								Login string `json:"login"`
							} `json:"author"`
							CreatedAt string `json:"createdAt"`
							State     string `json:"state"`
							Body      string `json:"body"`
						} `json:"nodes"`
					} `json:"reviews"`
				} `json:"pullRequest"`
			} `json:"repository"`
		} `json:"data"`
	}

	if err := json.Unmarshal(result, &response); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	reviews := response.Data.Repository.PullRequest.Reviews.Nodes
	if len(reviews) == 0 {
		fmt.Println("No reviews found")
		return nil
	}

	// Sort reviews by creation time (latest first)
	// They should already be sorted, but let's be explicit

	// Load existing state
	lastState, err := loadReviewState(prNumberStr)
	if err == nil {
		fmt.Printf("Last known review: %s at %s\n", lastState.ID, lastState.CreatedAt)

		newReviews := []interface{}{}
		for _, review := range reviews {
			if review.CreatedAt > lastState.CreatedAt ||
				(review.CreatedAt == lastState.CreatedAt && review.ID != lastState.ID) {
				newReviews = append(newReviews, review)
			}
		}

		if len(newReviews) > 0 {
			fmt.Printf("\n🆕 Found %d new review(s):\n", len(newReviews))
			for _, review := range newReviews {
				r := review.(struct {
					ID        string `json:"id"`
					Author    struct {
						Login string `json:"login"`
					} `json:"author"`
					CreatedAt string `json:"createdAt"`
					State     string `json:"state"`
					Body      string `json:"body"`
				})
				fmt.Printf("  - %s at %s (%s)\n", r.Author.Login, r.CreatedAt, r.State)
			}
			fmt.Println()

			fmt.Println("📝 New review details:")
			for _, review := range newReviews {
				r := review.(struct {
					ID        string `json:"id"`
					Author    struct {
						Login string `json:"login"`
					} `json:"author"`
					CreatedAt string `json:"createdAt"`
					State     string `json:"state"`
					Body      string `json:"body"`
				})
				body := r.Body
				if body == "" {
					body = "No body"
				}
				fmt.Printf("Author: %s\nTime: %s\nState: %s\nBody: %s\n---\n",
					r.Author.Login, r.CreatedAt, r.State, body)
			}
		} else {
			fmt.Println("✓ No new reviews since last check")
		}
	} else {
		fmt.Printf("No previous state found, showing all recent reviews...\n\n")
		fmt.Printf("📋 Found %d review(s) total:\n", len(reviews))
		for _, review := range reviews {
			fmt.Printf("  - %s at %s (%s)\n", review.Author.Login, review.CreatedAt, review.State)
		}
	}

	// Update state with latest review (last element since we use GraphQL 'last: 15')
	if len(reviews) > 0 {
		latestReview := reviews[len(reviews)-1]  // Fix: use last element as latest
		newState := ReviewState{
			ID:        latestReview.ID,
			CreatedAt: latestReview.CreatedAt,
		}
		if err := saveReviewState(prNumberStr, newState); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to save state: %v\n", err)
		} else {
			fmt.Printf("\n💾 Updated state: Latest review %s at %s\n", latestReview.ID, latestReview.CreatedAt)
		}
	}

	fmt.Println("\n✅ Review check complete")
	return nil
}

func waitForReviews(cmd *cobra.Command, args []string) error {
	prNumber := args[0]
	
	// Validate flags
	if excludeReviews && excludeChecks {
		return fmt.Errorf("cannot exclude both reviews and checks")
	}
	
	// Determine what to wait for
	waitForReviews := !excludeReviews
	waitForChecks := !excludeChecks
	
	// Request Gemini review if flag is set
	if requestReview && waitForReviews {
		fmt.Printf("📝 Requesting Gemini review for PR #%s...\n", prNumber)
		ghCmd := exec.Command("gh", "pr", "comment", prNumber, "--body", "/gemini review")
		if err := ghCmd.Run(); err != nil {
			return fmt.Errorf("failed to request Gemini review: %w", err)
		}
		fmt.Println("✅ Gemini review requested")
	}
	
	// Display what we're waiting for
	waitingFor := []string{}
	if waitForReviews {
		waitingFor = append(waitingFor, "reviews")
	}
	if waitForChecks {
		waitingFor = append(waitingFor, "PR checks")
	}
	
	fmt.Printf("🔄 Waiting for %s on PR #%s (timeout: %d minutes)...\n", 
		strings.Join(waitingFor, " and "), prNumber, timeout)
	fmt.Println("Press Ctrl+C to stop monitoring")

	// For now, simply delegate to waitForReviewsAndChecks with appropriate flags
	// This ensures the new default behavior (both reviews and checks) works
	
	// Temporarily override global flags for delegation
	originalRequestReview := requestReview
	defer func() { requestReview = originalRequestReview }()
	
	// If we're only waiting for reviews, use the original simpler logic
	if waitForReviews && !waitForChecks {
		fmt.Printf("⚠️  Reviews-only mode: Using simplified wait logic\n")
		// Simple polling for reviews only (original behavior)
		return waitForReviewsOnly(prNumber)
	}
	
	// For all other cases (checks-only or both), delegate to the full implementation
	err := waitForReviewsAndChecks(cmd, args)
	// Don't wrap the error to avoid double error messages
	return err
}

// waitForReviewsOnly waits specifically for new reviews without checking PR status
func waitForReviewsOnly(prNumber string) error {
	// Parse timeout duration
	timeoutDuration, err := parseTimeout()
	if err != nil {
		return fmt.Errorf("invalid timeout format: %w", err)
	}
	
	// Apply Claude Code timeout constraints
	claudeCodeEnvTimeout, hasClaudeCodeEnv := checkClaudeCodeEnvironment()
	var effectiveTimeout time.Duration
	if hasClaudeCodeEnv {
		effectiveTimeout = timeoutDuration
		if timeoutDuration > claudeCodeEnvTimeout {
			fmt.Printf("⚠️  Requested timeout (%v) exceeds Claude Code limit (%v). Using %v.\n", 
				timeoutDuration, claudeCodeEnvTimeout, claudeCodeEnvTimeout)
			effectiveTimeout = claudeCodeEnvTimeout
		}
	} else {
		claudeCodeLimit := 90 * time.Second
		effectiveTimeout = timeoutDuration
		if timeoutDuration > claudeCodeLimit {
			fmt.Printf("⚠️  Claude Code has 2-minute timeout (no env config detected). Using %v for safety.\n", claudeCodeLimit)
			effectiveTimeout = claudeCodeLimit
		}
	}
	
	fmt.Printf("🔄 Waiting for reviews only on PR #%s (timeout: %v)...\n", prNumber, effectiveTimeout)
	fmt.Println("Press Ctrl+C to stop monitoring")
	
	// Load existing state
	lastState, err := loadReviewState(prNumber)
	if err == nil {
		fmt.Printf("📊 Tracking reviews since: %s\n", lastState.CreatedAt)
	}
	
	startTime := time.Now()
	for {
		// Check timeout
		if time.Since(startTime) > effectiveTimeout {
			fmt.Printf("\n⏰ Timeout reached (%v). No new reviews found.\n", effectiveTimeout)
			return nil
		}
		
		// Query for reviews
		query := fmt.Sprintf(`
		{
		  repository(owner: "%s", name: "%s") {
		    pullRequest(number: %s) {
		      reviews(last: 15) {
		        nodes {
		          id
		          author { login }
		          createdAt
		          state
		          body
		        }
		      }
		    }
		  }
		}`, owner, repo, prNumber)
		
		result, err := runGraphQLQuery(query)
		if err != nil {
			fmt.Printf("Error checking reviews: %v\n", err)
			time.Sleep(30 * time.Second)
			continue
		}
		
		var response struct {
			Data struct {
				Repository struct {
					PullRequest struct {
						Reviews struct {
							Nodes []struct {
								ID        string `json:"id"`
								Author    struct {
									Login string `json:"login"`
								} `json:"author"`
								CreatedAt string `json:"createdAt"`
								State     string `json:"state"`
								Body      string `json:"body"`
							} `json:"nodes"`
						} `json:"reviews"`
					} `json:"pullRequest"`
				} `json:"repository"`
			} `json:"data"`
		}
		
		if err := json.Unmarshal(result, &response); err != nil {
			fmt.Printf("Error parsing response: %v\n", err)
			time.Sleep(30 * time.Second)
			continue
		}
		
		reviews := response.Data.Repository.PullRequest.Reviews.Nodes
		
		if hasNewReviews(reviews, lastState) {
			// Find and display new reviews
			if lastState == nil {
				fmt.Printf("\n🎉 Found %d review(s)\n", len(reviews))
			} else {
				// Show details of new reviews
				for _, review := range reviews {
					if review.CreatedAt > lastState.CreatedAt ||
						(review.CreatedAt == lastState.CreatedAt && review.ID != lastState.ID) {
						fmt.Printf("\n🎉 New review detected from %s at %s\n", review.Author.Login, review.CreatedAt)
						if review.Body != "" && len(review.Body) > 100 {
							fmt.Printf("Preview: %s...\n", review.Body[:100])
						}
						break // Show only the first new review for brevity
					}
				}
			}
			
			// Update state with latest review
			if len(reviews) > 0 {
				latestReview := reviews[len(reviews)-1]
				newState := ReviewState{
					ID:        latestReview.ID,
					CreatedAt: latestReview.CreatedAt,
				}
				saveReviewState(prNumber, newState)
			}
			
			fmt.Println("\n✅ New reviews available!")
			fmt.Printf("💡 To list unresolved threads: bin/gh-helper threads list %s\n", prNumber)
			fmt.Println("⚠️  IMPORTANT: Please read the review feedback carefully before proceeding")
			return nil
		}
		
		elapsed := time.Since(startTime)
		remaining := effectiveTimeout - elapsed
		fmt.Printf("[%s] No new reviews yet (remaining: %v)\n",
			time.Now().Format("15:04:05"), remaining.Truncate(time.Second))
		
		time.Sleep(30 * time.Second)
	}
}

func waitForReviewsAndChecks(cmd *cobra.Command, args []string) error {
	prNumber := args[0]
	
	// Parse timeout duration with new flexible format
	timeoutDuration, err := parseTimeout()
	if err != nil {
		return fmt.Errorf("invalid timeout format: %w", err)
	}
	
	// Check Claude Code environment and adjust timeout accordingly
	// Based on anthropics/claude-code#1039, anthropics/claude-code#1216, anthropics/claude-code#1717
	claudeCodeEnvTimeout, hasClaudeCodeEnv := checkClaudeCodeEnvironment()
	
	var effectiveTimeout time.Duration
	if hasClaudeCodeEnv {
		// User has configured Claude Code environment variables
		effectiveTimeout = timeoutDuration
		if timeoutDuration > claudeCodeEnvTimeout {
			fmt.Printf("⚠️  Requested timeout (%v) exceeds Claude Code limit (%v). Using %v.\n", 
				timeoutDuration, claudeCodeEnvTimeout, claudeCodeEnvTimeout)
			effectiveTimeout = claudeCodeEnvTimeout
		}
	} else {
		// Default Claude Code 2-minute limit - use safe margin
		// anthropics/claude-code#1216: Commands timeout at exactly 2m 0.0s
		claudeCodeLimit := 90 * time.Second  // Safe margin
		effectiveTimeout = timeoutDuration
		
		if timeoutDuration > claudeCodeLimit {
			fmt.Printf("⚠️  Claude Code has 2-minute timeout (no env config detected). Using %v for safety.\n", claudeCodeLimit)
			fmt.Printf("💡 To extend timeout, set BASH_MAX_TIMEOUT_MS in ~/.claude/settings.json\n")
			fmt.Printf("💡 Example: {\"env\": {\"BASH_MAX_TIMEOUT_MS\": \"900000\"}} for 15 minutes\n")
			fmt.Printf("💡 Manual retry: bin/gh-helper reviews wait %s --timeout=%v\n", prNumber, timeoutDuration)
			effectiveTimeout = claudeCodeLimit
		}
	}
	
	// Request Gemini review if flag is set
	if requestReview {
		fmt.Printf("📝 Requesting Gemini review for PR #%s...\n", prNumber)
		ghCmd := exec.Command("gh", "pr", "comment", prNumber, "--body", "/gemini review")
		if err := ghCmd.Run(); err != nil {
			return fmt.Errorf("failed to request Gemini review: %w", err)
		}
		fmt.Println("✅ Gemini review requested")
	}
	
	fmt.Printf("🔄 Waiting for both reviews AND PR checks for PR #%s (timeout: %v)...\n", prNumber, effectiveTimeout)
	fmt.Println("Press Ctrl+C to stop monitoring")

	// Setup signal handling for graceful termination with proper guidance
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	// Exit code 130 is standard for SIGINT (Ctrl+C)
	go func() {
		sig := <-sigChan
		fmt.Printf("\n🛑 Received signal %v - terminating gracefully\n", sig)
		if effectiveTimeout < timeoutDuration {
			fmt.Printf("💡 Claude Code timeout interrupted. To continue, run:\n")
			fmt.Printf("    bin/gh-helper reviews wait %s --timeout=%v\n", prNumber, timeoutDuration)
		}
		os.Exit(130) // Standard exit code for SIGINT
	}()

	// Setup timeout (effectiveTimeout is already time.Duration)
	// timeoutDuration := timeoutDuration  // Already defined above
	startTime := time.Now()

	// Get initial state
	initialCheck := true
	reviewsReady := false
	checksComplete := false

	for {
		// Check timeout
		if time.Since(startTime) > effectiveTimeout {
			fmt.Printf("\n⏰ Timeout reached (%v).\n", effectiveTimeout)
			if reviewsReady && checksComplete {
				fmt.Println("✅ Both reviews and checks completed!")
				return nil
			} else {
				fmt.Printf("Status: Reviews ready: %v, Checks complete: %v\n", reviewsReady, checksComplete)
				if effectiveTimeout < timeoutDuration {
					fmt.Printf("💡 To continue waiting, run: bin/gh-helper reviews wait %s\n", prNumber)
				}
				return nil
			}
		}

		// Combined GraphQL query for both reviews and checks
		query := fmt.Sprintf(`
{
  repository(owner: "%s", name: "%s") {
    pullRequest(number: %s) {
      mergeable
      mergeStateStatus
      reviews(last: 15) {
        nodes {
          id
          author { login }
          createdAt
          state
          body
        }
      }
      commits(last: 1) {
        nodes {
          commit {
            statusCheckRollup {
              state
              contexts(first: 50) {
                nodes {
                  ... on StatusContext {
                    context
                    state
                    targetUrl
                  }
                  ... on CheckRun {
                    name
                    status
                    conclusion
                    detailsUrl
                  }
                }
              }
            }
          }
        }
      }
    }
  }
}`, owner, repo, prNumber)

		result, err := runGraphQLQuery(query)
		if err != nil {
			fmt.Printf("Error checking PR status: %v\n", err)
			time.Sleep(30 * time.Second)
			continue
		}

		var response struct {
			Data struct {
				Repository struct {
					PullRequest struct {
						Mergeable        string `json:"mergeable"`
						MergeStateStatus string `json:"mergeStateStatus"`
						Reviews struct {
							Nodes []struct {
								ID        string `json:"id"`
								Author    struct {
									Login string `json:"login"`
								} `json:"author"`
								CreatedAt string `json:"createdAt"`
								State     string `json:"state"`
								Body      string `json:"body"`
							} `json:"nodes"`
						} `json:"reviews"`
						Commits struct {
							Nodes []struct {
								Commit struct {
									StatusCheckRollup *struct {
										State    string `json:"state"`
										Contexts struct {
											Nodes []interface{} `json:"nodes"`
										} `json:"contexts"`
									} `json:"statusCheckRollup"`
								} `json:"commit"`
							} `json:"nodes"`
						} `json:"commits"`
					} `json:"pullRequest"`
				} `json:"repository"`
			} `json:"data"`
		}

		if err := json.Unmarshal(result, &response); err != nil {
			fmt.Printf("Error parsing response: %v\n", err)
			time.Sleep(30 * time.Second)
			continue
		}

		// Check reviews status using common state tracking
		reviews := response.Data.Repository.PullRequest.Reviews.Nodes
		if len(reviews) > 0 && !reviewsReady {
			lastState, _ := loadReviewState(prNumber)
			reviewsReady = hasNewReviews(reviews, lastState)
		}

		// Check mergeable status first - if conflicting, stop immediately
		// CRITICAL INSIGHT: statusCheckRollup is null when PR has merge conflicts,
		// which prevents CI from running. This is GitHub's intentional behavior.
		// Must check mergeable before assuming "no checks required" scenario.
		mergeable := response.Data.Repository.PullRequest.Mergeable
		mergeStatus := response.Data.Repository.PullRequest.MergeStateStatus
		if mergeable == "CONFLICTING" {
			fmt.Printf("\n❌ [%s] PR has merge conflicts (status: %s)\n", time.Now().Format("15:04:05"), mergeStatus)
			fmt.Println("⚠️  CI checks will not run until conflicts are resolved")
			fmt.Printf("💡 Resolve conflicts with: git rebase origin/main\n")
			fmt.Printf("💡 Then push and run: bin/gh-helper reviews wait %s\n", prNumber)
			return fmt.Errorf("merge conflicts prevent CI execution")
		}

		// Check PR checks status
		commits := response.Data.Repository.PullRequest.Commits.Nodes
		if len(commits) > 0 && commits[0].Commit.StatusCheckRollup != nil {
			rollupState := commits[0].Commit.StatusCheckRollup.State
			checksComplete = (rollupState == "SUCCESS" || rollupState == "FAILURE" || rollupState == "ERROR")
		} else {
			// No StatusCheckRollup means either no checks configured or checks haven't started yet
			// For better user experience, we'll consider this as "no checks required" and complete
			checksComplete = true
		}

		if initialCheck {
			fmt.Printf("[%s] Monitoring started.\n", time.Now().Format("15:04:05"))
			fmt.Printf("   Reviews: %d found, Ready: %v\n", len(reviews), reviewsReady)
			
			// Show mergeable status
			mergeable := response.Data.Repository.PullRequest.Mergeable
			mergeStatus := response.Data.Repository.PullRequest.MergeStateStatus
			switch mergeable {
			case "MERGEABLE":
				fmt.Printf("   Merge: ✅ Ready to merge\n")
			case "CONFLICTING":
				fmt.Printf("   Merge: ❌ Has conflicts (status: %s)\n", mergeStatus)
			case "UNKNOWN":
				fmt.Printf("   Merge: ⏳ Checking...\n")
			default:
				fmt.Printf("   Merge: %s (status: %s)\n", mergeable, mergeStatus)
			}
			if len(commits) > 0 && commits[0].Commit.StatusCheckRollup != nil {
				rollupState := commits[0].Commit.StatusCheckRollup.State
				statusMsg := rollupState
				switch rollupState {
				case "SUCCESS":
					statusMsg = "All passed"
				case "FAILURE": 
					statusMsg = "Some failed"
				case "ERROR":
					statusMsg = "Error occurred"
				case "PENDING":
					statusMsg = "Still running"
				}
				fmt.Printf("   Checks: %s, Complete: %v\n", statusMsg, checksComplete)
			} else {
				fmt.Printf("   Checks: None required, Complete: %v\n", checksComplete)
			}
			initialCheck = false
		}

		// Check if both conditions are met
		if reviewsReady && checksComplete {
			fmt.Printf("\n🎉 [%s] Both reviews and checks are ready!\n", time.Now().Format("15:04:05"))
			
			if reviewsReady {
				fmt.Println("✅ Reviews: New reviews available")
				
				// Output review details to reduce subsequent API calls
				fmt.Println("\n📋 Recent Reviews:")
				for i, review := range reviews {
					if i >= 5 { // Limit to 5 most recent reviews
						break
					}
					fmt.Printf("   • %s by %s (%s) - %s\n", 
						review.ID, 
						review.Author.Login, 
						review.State,
						review.CreatedAt)
					if review.Body != "" && len(review.Body) > 100 {
						fmt.Printf("     Preview: %s...\n", review.Body[:100])
					} else if review.Body != "" {
						fmt.Printf("     Preview: %s\n", review.Body)
					}
				}
				
				fmt.Printf("\n💡 To list unresolved threads: bin/gh-helper threads list %s\n", prNumber)
				fmt.Println("⚠️  IMPORTANT: Please read the review feedback carefully before proceeding")
			}
			
			// Show merge conflicts warning if present
			mergeable := response.Data.Repository.PullRequest.Mergeable
			if mergeable == "CONFLICTING" {
				fmt.Printf("\n⚠️  Merge conflicts detected - CI may not run until resolved\n")
				fmt.Printf("💡 Resolve conflicts and push to trigger CI checks\n")
			}
			if checksComplete {
				if len(commits) > 0 && commits[0].Commit.StatusCheckRollup != nil {
					rollupState := commits[0].Commit.StatusCheckRollup.State
					switch rollupState {
					case "SUCCESS":
						fmt.Println("✅ Checks: All passed")
					case "FAILURE":
						fmt.Println("❌ Checks: Some failed")
					case "ERROR":
						fmt.Println("🚨 Checks: Error occurred")
					case "PENDING":
						fmt.Println("⏳ Checks: Still running")
					default:
						fmt.Printf("✅ Checks: Completed (%s)\n", rollupState)
					}
				} else {
					fmt.Println("✅ Checks: No checks required")
				}
			}
			
			return nil
		}

		elapsed := time.Since(startTime)
		remaining := timeoutDuration - elapsed
		fmt.Printf("[%s] Status: Reviews: %v, Checks: %v (remaining: %v)\n",
			time.Now().Format("15:04:05"), reviewsReady, checksComplete, remaining.Truncate(time.Second))
		
		time.Sleep(30 * time.Second)
	}
}

func listThreads(cmd *cobra.Command, args []string) error {
	prNumber := args[0]

	fmt.Printf("🔍 Fetching review threads for PR #%s...\n", prNumber)

	query := fmt.Sprintf(`
{
  repository(owner: "%s", name: "%s") {
    pullRequest(number: %s) {
      reviewThreads(first: 20) {
        nodes {
          id
          line
          path
          isResolved
          subjectType
          comments(first: 10) {
            nodes {
              id
              body
              author {
                login
              }
              createdAt
            }
          }
        }
      }
    }
  }
}`, owner, repo, prNumber)

	result, err := runGraphQLQuery(query)
	if err != nil {
		return fmt.Errorf("failed to fetch threads: %w", err)
	}

	var response struct {
		Data struct {
			Repository struct {
				PullRequest struct {
					ReviewThreads struct {
						Nodes []struct {
							ID          string `json:"id"`
							Line        *int   `json:"line"`
							Path        string `json:"path"`
							IsResolved  bool   `json:"isResolved"`
							SubjectType string `json:"subjectType"`
							Comments    struct {
								Nodes []struct {
									ID     string `json:"id"`
									Body   string `json:"body"`
									Author struct {
										Login string `json:"login"`
									} `json:"author"`
									CreatedAt string `json:"createdAt"`
								} `json:"nodes"`
							} `json:"comments"`
						} `json:"nodes"`
					} `json:"reviewThreads"`
				} `json:"pullRequest"`
			} `json:"repository"`
		} `json:"data"`
	}

	if err := json.Unmarshal(result, &response); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	unresolvedThreads := []interface{}{}
	for _, thread := range response.Data.Repository.PullRequest.ReviewThreads.Nodes {
		if !thread.IsResolved {
			location := thread.Path
			if thread.Line != nil {
				location = fmt.Sprintf("%s:%d", thread.Path, *thread.Line)
			} else {
				location = fmt.Sprintf("%s:file", thread.Path)
			}

			latestComment := ""
			latestAuthor := ""
			needsReply := true

			if len(thread.Comments.Nodes) > 0 {
				last := thread.Comments.Nodes[len(thread.Comments.Nodes)-1]
				latestComment = last.Body
				if len(latestComment) > 100 {
					latestComment = latestComment[:100]
				}
				latestAuthor = last.Author.Login

				// Check if current user has replied
				if currentUser, err := getCurrentUser(); err == nil {
					for _, comment := range thread.Comments.Nodes {
						if comment.Author.Login == currentUser {
							needsReply = false
							break
						}
					}
				}
			}

			unresolvedThreads = append(unresolvedThreads, map[string]interface{}{
				"thread_id":      thread.ID,
				"location":       location,
				"subject_type":   thread.SubjectType,
				"latest_comment": latestComment,
				"latest_author":  latestAuthor,
				"needs_reply":    needsReply,
			})
		}
	}

	if len(unresolvedThreads) == 0 {
		fmt.Println("✅ No unresolved review threads found")
		return nil
	}

	fmt.Printf("📋 Found %d unresolved review thread(s):\n\n", len(unresolvedThreads))
	for _, thread := range unresolvedThreads {
		t := thread.(map[string]interface{})
		fmt.Printf("Thread ID: %s\n", t["thread_id"])
		fmt.Printf("Location: %s\n", t["location"])
		fmt.Printf("Author: %s\n", t["latest_author"])
		fmt.Printf("Preview: %s\n", t["latest_comment"])
		fmt.Printf("Needs Reply: %v\n\n", t["needs_reply"])
	}

	return nil
}

func showThread(cmd *cobra.Command, args []string) error {
	threadID := args[0]

	fmt.Printf("🔍 Fetching details for thread: %s\n", threadID)

	query := fmt.Sprintf(`
{
  node(id: "%s") {
    ... on PullRequestReviewThread {
      id
      line
      path
      isResolved
      subjectType
      pullRequest {
        number
        title
      }
      comments(first: 20) {
        nodes {
          id
          body
          author {
            login
          }
          createdAt
          diffHunk
        }
      }
    }
  }
}`, threadID)

	result, err := runGraphQLQuery(query)
	if err != nil {
		return fmt.Errorf("failed to fetch thread details: %w", err)
	}

	var response struct {
		Data struct {
			Node struct {
				ID          string `json:"id"`
				Line        *int   `json:"line"`
				Path        string `json:"path"`
				IsResolved  bool   `json:"isResolved"`
				SubjectType string `json:"subjectType"`
				PullRequest struct {
					Number int    `json:"number"`
					Title  string `json:"title"`
				} `json:"pullRequest"`
				Comments struct {
					Nodes []struct {
						ID       string `json:"id"`
						Body     string `json:"body"`
						Author   struct {
							Login string `json:"login"`
						} `json:"author"`
						CreatedAt string    `json:"createdAt"`
						DiffHunk  *string   `json:"diffHunk"`
					} `json:"nodes"`
				} `json:"comments"`
			} `json:"node"`
		} `json:"data"`
	}

	if err := json.Unmarshal(result, &response); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	thread := response.Data.Node
	if thread.ID == "" {
		return fmt.Errorf("thread not found or invalid thread ID")
	}

	// Display thread header
	fmt.Printf("\n📋 Thread Details\n")
	fmt.Printf("Thread ID: %s\n", thread.ID)
	fmt.Printf("PR: #%d - %s\n", thread.PullRequest.Number, thread.PullRequest.Title)
	
	location := thread.Path
	if thread.Line != nil {
		location = fmt.Sprintf("%s:%d", thread.Path, *thread.Line)
	}
	fmt.Printf("Location: %s\n", location)
	fmt.Printf("Subject Type: %s\n", thread.SubjectType)
	fmt.Printf("Resolved: %v\n", thread.IsResolved)
	fmt.Printf("Comments: %d\n\n", len(thread.Comments.Nodes))

	// Display diff context if available
	if len(thread.Comments.Nodes) > 0 && thread.Comments.Nodes[0].DiffHunk != nil {
		fmt.Printf("📄 Code Context:\n")
		fmt.Printf("```diff\n%s\n```\n\n", *thread.Comments.Nodes[0].DiffHunk)
	}

	// Display all comments
	fmt.Printf("💬 Comments:\n")
	for i, comment := range thread.Comments.Nodes {
		fmt.Printf("─────────────────────────────────────────────────────────────\n")
		fmt.Printf("%d. %s (%s)\n", i+1, comment.Author.Login, comment.CreatedAt)
		fmt.Printf("─────────────────────────────────────────────────────────────\n")
		fmt.Printf("%s\n\n", comment.Body)
	}

	if !thread.IsResolved && len(thread.Comments.Nodes) > 0 {
		lastComment := thread.Comments.Nodes[len(thread.Comments.Nodes)-1]
		// Check if the last comment is from the current user
		if currentUser, err := getCurrentUser(); err == nil {
			if lastComment.Author.Login != currentUser {
				fmt.Printf("💡 To reply to this thread:\n")
				fmt.Printf("   gh-helper threads reply %s --message \"Your reply\"\n", threadID)
				fmt.Printf("   echo \"Your reply\" | gh-helper threads reply %s\n", threadID)
			}
		} else {
			// If we can't determine current user, show reply option anyway
			fmt.Printf("💡 To reply to this thread:\n")
			fmt.Printf("   gh-helper threads reply %s --message \"Your reply\"\n", threadID)
			fmt.Printf("   echo \"Your reply\" | gh-helper threads reply %s\n", threadID)
		}
	}

	return nil
}

func replyWithCommit(cmd *cobra.Command, args []string) error {
	threadID := args[0]
	commitHash := args[1]

	var baseReplyText string
	if message != "" {
		baseReplyText = message
	} else {
		// Read from stdin - AI assistants prefer this over temporary files (Issue #301 insight)
		scanner := bufio.NewScanner(os.Stdin)
		var lines []string
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("failed to read from stdin: %w", err)
		}
		baseReplyText = strings.Join(lines, "\n")
	}

	// Format reply with commit reference
	var replyText string
	if strings.TrimSpace(baseReplyText) == "" {
		replyText = fmt.Sprintf("Thank you for the feedback! Fixed in commit %s.", commitHash)
	} else {
		replyText = fmt.Sprintf("%s\n\nFixed in commit %s.", strings.TrimSpace(baseReplyText), commitHash)
	}

	// Add mention if provided
	if mentionUser != "" {
		replyText = fmt.Sprintf("@%s %s", mentionUser, replyText)
	}

	fmt.Printf("🔄 Replying to review thread: %s (with commit %s)\n", threadID, commitHash)

	// CRITICAL: Do NOT include pullRequestReviewId field - causes null responses
	// despite being marked "optional" in GraphQL schema (discovered in #301)
	
	escapedText, err := jsonEscape(replyText)
	if err != nil {
		return fmt.Errorf("failed to escape reply text: %w", err)
	}
	
	mutation := fmt.Sprintf(`
mutation {
  addPullRequestReviewThreadReply(input: {
    pullRequestReviewThreadId: "%s"
    body: %s
  }) {
    comment {
      id
      url
      body
    }
  }
}`, threadID, escapedText)

	result, err := runGraphQLQuery(mutation)
	if err != nil {
		return fmt.Errorf("failed to post reply: %w", err)
	}

	var response struct {
		Data struct {
			AddPullRequestReviewThreadReply struct {
				Comment struct {
					ID  string `json:"id"`
					URL string `json:"url"`
					Body string `json:"body"`
				} `json:"comment"`
			} `json:"addPullRequestReviewThreadReply"`
		} `json:"data"`
	}

	if err := json.Unmarshal(result, &response); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	comment := response.Data.AddPullRequestReviewThreadReply.Comment
	if comment.ID == "" {
		fmt.Println("❌ Failed to post reply. Response:")
		fmt.Println(string(result))
		return fmt.Errorf("reply posting failed")
	}

	fmt.Println("✅ Reply posted successfully!")
	fmt.Printf("   Comment ID: %s\n", comment.ID)
	fmt.Printf("   URL: %s\n", comment.URL)

	return nil
}

func replyToThread(cmd *cobra.Command, args []string) error {
	threadID := args[0]

	var replyText string
	if message != "" {
		replyText = message
	} else {
		// Read from stdin - AI assistants prefer this over temporary files (Issue #301 insight)
		scanner := bufio.NewScanner(os.Stdin)
		var lines []string
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("failed to read from stdin: %w", err)
		}
		replyText = strings.Join(lines, "\n")
	}

	if strings.TrimSpace(replyText) == "" {
		return fmt.Errorf("reply text cannot be empty (use --message or pipe content to stdin)")
	}

	// Add mention if provided
	if mentionUser != "" {
		replyText = fmt.Sprintf("@%s %s", mentionUser, replyText)
	}

	fmt.Printf("🔄 Replying to review thread: %s\n", threadID)

	// CRITICAL INSIGHT (Issue #301): GitHub GraphQL API quirk
	// Do NOT include pullRequestReviewId field - causes null responses despite being marked "optional" in schema
	// This was discovered during AI-friendly tool development and is documented in dev-docs/development-insights.md
	
	escapedText, err := jsonEscape(replyText)
	if err != nil {
		return fmt.Errorf("failed to escape reply text: %w", err)
	}
	
	mutation := fmt.Sprintf(`
mutation {
  addPullRequestReviewThreadReply(input: {
    pullRequestReviewThreadId: "%s"
    body: %s
  }) {
    comment {
      id
      url
      body
    }
  }
}`, threadID, escapedText)

	result, err := runGraphQLQuery(mutation)
	if err != nil {
		return fmt.Errorf("failed to post reply: %w", err)
	}

	var response struct {
		Data struct {
			AddPullRequestReviewThreadReply struct {
				Comment struct {
					ID  string `json:"id"`
					URL string `json:"url"`
					Body string `json:"body"`
				} `json:"comment"`
			} `json:"addPullRequestReviewThreadReply"`
		} `json:"data"`
	}

	if err := json.Unmarshal(result, &response); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	comment := response.Data.AddPullRequestReviewThreadReply.Comment
	if comment.ID == "" {
		fmt.Println("❌ Failed to post reply. Response:")
		fmt.Println(string(result))
		return fmt.Errorf("reply posting failed")
	}

	fmt.Println("✅ Reply posted successfully!")
	fmt.Printf("   Comment ID: %s\n", comment.ID)
	fmt.Printf("   URL: %s\n", comment.URL)

	return nil
}

func runGraphQLQuery(query string) ([]byte, error) {
	cmd := exec.Command("gh", "api", "graphql", "-F", "query="+query)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Try to extract meaningful error message from stderr
		stderrStr := stderr.String()
		if strings.Contains(stderrStr, "Could not resolve to a PullRequest") {
			return nil, fmt.Errorf("PR not found - please check the PR number is correct\n💡 Tip: Use 'gh pr list --state open' to see available PR numbers")
		}
		if strings.Contains(stderrStr, "authentication") || strings.Contains(stderrStr, "token") {
			return nil, fmt.Errorf("GitHub authentication failed\n💡 Tip: Run 'gh auth login' to authenticate")
		}
		return nil, fmt.Errorf("gh api failed: %w, stderr: %s", err, stderrStr)
	}

	// Check for GraphQL errors in the response
	var errorCheck struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	
	result := stdout.Bytes()
	if err := json.Unmarshal(result, &errorCheck); err == nil && len(errorCheck.Errors) > 0 {
		return nil, fmt.Errorf("GraphQL error: %s", errorCheck.Errors[0].Message)
	}

	return result, nil
}

func jsonEscape(s string) (string, error) {
	b, err := json.Marshal(s)
	if err != nil {
		return "", fmt.Errorf("failed to marshal string for json escaping: %w", err)
	}
	return string(b), nil
}