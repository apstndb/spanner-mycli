package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/apstndb/spanner-mycli/dev-tools/shared"
	"github.com/spf13/cobra"
)



// Helper functions for common patterns

// resolvePRNumberFromArgs provides backwards compatibility wrapper
func resolvePRNumberFromArgs(args []string, client *shared.GitHubClient) (string, error) {
	var input string
	if len(args) > 0 {
		input = args[0]
	}
	
	prNumberInt, message, err := client.ResolvePRNumber(input)
	if err != nil {
		return "", shared.FetchError("PR number", err)
	}
	
	if message != "" {
		shared.InfoMsg(message).Print()
	}
	
	return fmt.Sprintf("%d", prNumberInt), nil
}

// parseTimeout provides backwards compatibility wrapper  
func parseTimeout() (time.Duration, error) {
	return shared.ParseTimeoutString(timeoutStr)
}

// calculateEffectiveTimeout handles timeout calculation with Claude Code constraints consistently
// Returns the effective timeout and a user-friendly display string
func calculateEffectiveTimeout() (time.Duration, string, error) {
	result, err := shared.CalculateTimeoutFromString(timeoutStr)
	if err != nil {
		return 0, "", err
	}
	
	// Show warning if timeout was constrained
	if result.IsConstrained {
		shared.WarningMsg("Requested timeout (%v) exceeds Claude Code limit. Using %v.", 
			result.Requested, result.Effective).Print()
	}
	
	return result.Effective, result.Display, nil
}

var rootCmd = &cobra.Command{
	Use:   "gh-helper",
	Short: "Generic GitHub operations helper",
	Long: `Generic GitHub operations optimized for AI assistants.

COMMON PATTERNS:
  gh-helper reviews wait <PR> --request-review     # Complete review workflow
  gh-helper reviews fetch <PR> --list-threads      # List thread IDs needing replies
  gh-helper threads reply <THREAD_ID> --commit-hash abc123 --message "Fixed as suggested"

See dev-tools/gh-helper/README.md for detailed documentation, design rationale,
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

var checkReviewsCmd = shared.NewOperationalCommand(
	"check [pr-number]",
	"Check for new PR reviews with state tracking",
	`Check for new pull request reviews, tracking state to identify updates.

This command maintains state in ~/.cache/spanner-mycli-reviews/ to detect
new reviews since the last check. Useful for monitoring PR activity.

`+prNumberArgsHelp,
	checkReviews,
)

var waitReviewsCmd = shared.NewOperationalCommand(
	"wait [pr-number]",
	"Wait for both reviews and PR checks (default behavior)",
	`Continuously monitor for both new reviews AND PR checks completion by default.

This command polls every 30 seconds and waits until BOTH conditions are met:
1. New reviews are available
2. All PR checks have completed (success, failure, or cancelled)

Use --exclude-reviews to wait for PR checks only.
Use --exclude-checks to wait for reviews only.
Use --request-review to automatically request Gemini review before waiting.

`+prNumberArgsHelp+`

AI-FRIENDLY: Designed for autonomous workflows that need complete feedback.
Default timeout is 5 minutes, configurable with --timeout flag.`,
	waitForReviews,
)

// waitAllCmd removed - redundant with waitReviewsCmd which supports the same functionality


var replyThreadsCmd = shared.NewOperationalCommand(
	"reply <thread-id>",
	"Reply to a review thread",
	`Reply to a GitHub pull request review thread.

AI-FRIENDLY DESIGN (Issue #301): The reply text can be provided via:
- --message flag for single-line responses
- stdin for multi-line content or heredoc (preferred by AI assistants)
- --commit-hash for standardized commit references

Examples:
  gh-helper threads reply PRRT_kwDONC6gMM5SU-GH --message "Fixed as suggested"
  gh-helper threads reply PRRT_kwDONC6gMM5SU-GH --commit-hash abc123 --message "Addressed all feedback"
  gh-helper threads reply PRRT_kwDONC6gMM5SU-GH --commit-hash abc123  # Uses default message
  echo "Thank you for the feedback!" | gh-helper threads reply PRRT_kwDONC6gMM5SU-GH
  gh-helper threads reply PRRT_kwDONC6gMM5SU-GH <<EOF
  Fixed the issue. The implementation now:
  - Handles edge cases properly
  - Includes proper error handling
  EOF`,
	replyToThread,
)

var showThreadCmd = shared.NewOperationalCommand(
	"show <thread-id>",
	"Show detailed view of a review thread",
	`Show detailed view of a specific review thread including all comments.

This provides full context for understanding review feedback before replying.
Useful for getting complete thread history and comment details.`,
	showThread,
)

// replyWithCommitCmd removed - use 'threads reply' with --message for commit references

var (
	owner          string
	repo           string
	message        string
	mentionUser    string
	commitHash     string
	timeoutStr     string
	requestReview  bool
	excludeReviews bool
	excludeChecks  bool
)

// Common help text for PR number arguments
const prNumberArgsHelp = `Arguments:
- No argument: Uses current branch's PR
- Plain number (123): PR number
- Explicit PR (pull/123, pr/123): PR reference`

func init() {
	// Configure Args for operational commands (using shared.NewOperationalCommand)
	checkReviewsCmd.Args = cobra.MaximumNArgs(1)
	waitReviewsCmd.Args = cobra.MaximumNArgs(1)
	replyThreadsCmd.Args = cobra.ExactArgs(1)
	showThreadCmd.Args = cobra.ExactArgs(1)
	
	// Configure flags
	rootCmd.PersistentFlags().StringVar(&owner, "owner", shared.DefaultOwner, "GitHub repository owner")
	rootCmd.PersistentFlags().StringVar(&repo, "repo", shared.DefaultRepo, "GitHub repository name")
	rootCmd.PersistentFlags().StringVar(&timeoutStr, "timeout", "5m", "Timeout duration (e.g., 90s, 1.5m, 2m30s, 15m)")
	rootCmd.PersistentFlags().Bool("json", false, "Output structured JSON for programmatic use")

	replyThreadsCmd.Flags().StringVar(&message, "message", "", "Reply message (or use stdin)")
	replyThreadsCmd.Flags().StringVar(&mentionUser, "mention", "", "Username to mention (without @)")
	replyThreadsCmd.Flags().StringVar(&commitHash, "commit-hash", "", "Commit hash to reference in reply")

	waitReviewsCmd.Flags().BoolVar(&excludeReviews, "exclude-reviews", false, "Exclude reviews, wait for PR checks only")
	waitReviewsCmd.Flags().BoolVar(&excludeChecks, "exclude-checks", false, "Exclude checks, wait for reviews only")
	waitReviewsCmd.Flags().BoolVar(&requestReview, "request-review", false, "Request Gemini review before waiting")

	// Add subcommands
	reviewsCmd.AddCommand(checkReviewsCmd, analyzeReviewsCmd, fetchReviewsCmd, waitReviewsCmd)
	threadsCmd.AddCommand(showThreadCmd, replyThreadsCmd)
	rootCmd.AddCommand(reviewsCmd, threadsCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
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
func hasNewReviews(reviews []shared.ReviewFields, lastState *ReviewState) bool {
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
		if parsedTimeout, err := time.ParseDuration(maxTimeoutStr + "ms"); err == nil {
			fmt.Printf("🔧 Claude Code BASH_MAX_TIMEOUT_MS detected: %v\n", parsedTimeout)
			maxTimeout = parsedTimeout
			hasMax = true
		}
	}
	
	// Check for BASH_DEFAULT_TIMEOUT_MS (default when no timeout specified)
	if defaultTimeoutStr := os.Getenv("BASH_DEFAULT_TIMEOUT_MS"); defaultTimeoutStr != "" {
		if parsedTimeout, err := time.ParseDuration(defaultTimeoutStr + "ms"); err == nil {
			fmt.Printf("🔧 Claude Code BASH_DEFAULT_TIMEOUT_MS detected: %v\n", parsedTimeout)
			defaultTimeout = parsedTimeout
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
	client := shared.NewGitHubClient(owner, repo)
	
	var input string
	if len(args) > 0 {
		input = args[0]
	}
	
	prNumberInt, message, err := client.ResolvePRNumber(input)
	if err != nil {
		return shared.FetchError("PR number", err)
	}
	
	if message != "" {
		shared.InfoMsg(message).Print()
	}
	
	prNumberStr := fmt.Sprintf("%d", prNumberInt)

	stateDir := fmt.Sprintf("%s/.cache/spanner-mycli-reviews", os.Getenv("HOME"))

	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	shared.StatusMsg("Checking reviews for PR #%s in %s/%s...", prNumberStr, owner, repo).Print()

	// Convert to int for GraphQL
	prNumber, err := strconv.Atoi(prNumberStr)
	if err != nil {
		return shared.FormatError("PR number", err)
	}

	query := shared.AllReviewFragments + `
query($owner: String!, $repo: String!, $prNumber: Int!, $includeReviewBodies: Boolean!) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $prNumber) {
      reviews(last: 15) {
        nodes {
          ...ReviewFields
        }
      }
    }
  }
}`

	result, err := client.RunGraphQLQueryWithVariables(query, shared.BasicPRVariablesWithBodies(owner, repo, prNumber, true))
	if err != nil {
		return shared.FetchError("reviews", err)
	}

	var response shared.ReviewsResponse

	if err := json.Unmarshal(result, &response); err != nil {
		return shared.ParseError("response", err)
	}

	reviews := response.GetReviews()
	if len(reviews) == 0 {
		shared.InfoMsg("No reviews found").Print()
		return nil
	}

	// Sort reviews by creation time (latest first)
	// They should already be sorted, but let's be explicit

	// Load existing state
	lastState, err := loadReviewState(prNumberStr)
	if err == nil {
		fmt.Printf("Last known review: %s at %s\n", lastState.ID, lastState.CreatedAt)

		var newReviews []shared.ReviewFields
		for _, review := range reviews {
			if review.CreatedAt > lastState.CreatedAt ||
				(review.CreatedAt == lastState.CreatedAt && review.ID != lastState.ID) {
				newReviews = append(newReviews, review)
			}
		}

		if len(newReviews) > 0 {
			fmt.Println() // Add newline
			summary := &shared.ReviewSummary{NewCount: len(newReviews), IsNew: true}
			summary.Print()
			for _, review := range newReviews {
				fmt.Printf("  - %s at %s (%s)\n", review.Author.Login, review.CreatedAt, review.State)
			}
			fmt.Println()

			fmt.Println("📝 New review details:")
			for _, review := range newReviews {
				body := review.Body
				if body == "" {
					body = "No body"
				}
				fmt.Printf("Author: %s\nTime: %s\nState: %s\nBody: %s\n---\n",
					review.Author.Login, review.CreatedAt, review.State, body)
			}
		} else {
			shared.SuccessMsg("No new reviews since last check").Print()
		}
	} else {
		shared.InfoMsg("No previous state found, showing all recent reviews...").Print()
		fmt.Println() // Add spacing
		summary := &shared.ReviewSummary{Count: len(reviews), IsNew: false}
		summary.Print()
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
	client := shared.NewGitHubClient(owner, repo)
	prNumber, err := resolvePRNumberFromArgs(args, client)
	if err != nil {
		return err
	}
	
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
		if err := client.CreatePRComment(prNumber, "/gemini review"); err != nil {
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
	
	// Calculate timeout with Claude Code constraints
	_, timeoutDisplay, err := calculateEffectiveTimeout()
	if err != nil {
		return err
	}
	
	fmt.Printf("🔄 Waiting for %s on PR #%s (timeout: %s)...\n", 
		strings.Join(waitingFor, " and "), prNumber, timeoutDisplay)
	fmt.Println("Press Ctrl+C to stop monitoring")

	// For now, simply delegate to waitForReviewsAndChecks with appropriate flags
	// This ensures the new default behavior (both reviews and checks) works
	
	// Temporarily override global flags for delegation
	originalRequestReview := requestReview
	defer func() { requestReview = originalRequestReview }()
	
	// Disable review request in delegated function since we already handled it above
	requestReview = false
	
	// If we're only waiting for reviews, use the original simpler logic
	if waitForReviews && !waitForChecks {
		fmt.Printf("⚠️  Reviews-only mode: Using simplified wait logic\n")
		// Simple polling for reviews only (original behavior)
		return waitForReviewsOnly(prNumber)
	}
	
	// For all other cases (checks-only or both), delegate to the full implementation
	err = waitForReviewsAndChecks(cmd, args)
	// Don't wrap the error to avoid double error messages
	return err
}

// waitForReviewsOnly waits specifically for new reviews without checking PR status
func waitForReviewsOnly(prNumber string) error {
	// Convert PR number to integer for GraphQL
	prNumberInt, err := strconv.Atoi(prNumber)
	if err != nil {
		return fmt.Errorf("invalid PR number format: %w", err)
	}
	
	// Create GitHub client once for better performance (token caching)
	client := shared.NewGitHubClient(owner, repo)
	
	// Apply Claude Code timeout constraints
	effectiveTimeout, timeoutDisplay, err := calculateEffectiveTimeout()
	if err != nil {
		return err
	}
	
	fmt.Printf("🔄 Waiting for reviews only on PR #%s (timeout: %s)...\n", prNumber, timeoutDisplay)
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
		
		// Query for reviews using fragments
		query := shared.AllReviewFragments + `
		query($owner: String!, $repo: String!, $prNumber: Int!, $includeReviewBodies: Boolean!) {
		  repository(owner: $owner, name: $repo) {
		    pullRequest(number: $prNumber) {
		      reviews(last: 15) {
		        nodes {
		          ...ReviewFields
		        }
		      }
		    }
		  }
		}`
		
		variables := shared.BasicPRVariablesWithBodies(owner, repo, prNumberInt, true)
		
		result, err := client.RunGraphQLQueryWithVariables(query, variables)
		if err != nil {
			fmt.Printf("Error checking reviews: %v\n", err)
			time.Sleep(30 * time.Second)
			continue
		}
		
		var response shared.ReviewsResponse
		
		if err := json.Unmarshal(result, &response); err != nil {
			fmt.Printf("Error parsing response: %v\n", err)
			time.Sleep(30 * time.Second)
			continue
		}
		
		reviews := response.GetReviews()
		
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
				_ = saveReviewState(prNumber, newState) // Best effort state save
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
	// Create GitHub client once for better performance (token caching)
	client := shared.NewGitHubClient(owner, repo)
	
	prNumber, err := resolvePRNumberFromArgs(args, client)
	if err != nil {
		return err
	}

	// Convert to int for GraphQL
	prNumberInt, err := strconv.Atoi(prNumber)
	if err != nil {
		return fmt.Errorf("invalid PR number format: %w", err)
	}
	
	// Calculate timeout with Claude Code constraints
	effectiveTimeout, timeoutDisplay, err := calculateEffectiveTimeout()
	if err != nil {
		return err
	}
	
	// Show additional guidance for extending timeout if needed
	timeoutDuration, parseErr := parseTimeout()
	if parseErr == nil && effectiveTimeout < timeoutDuration {
		fmt.Printf("💡 To extend timeout, set BASH_MAX_TIMEOUT_MS in ~/.claude/settings.json\n")
		fmt.Printf("💡 Example: {\"env\": {\"BASH_MAX_TIMEOUT_MS\": \"900000\"}} for 15 minutes\n")
		fmt.Printf("💡 Manual retry: bin/gh-helper reviews wait %s --timeout=%v\n", prNumber, timeoutDuration)
	}
	
	// Request Gemini review if flag is set
	if requestReview {
		fmt.Printf("📝 Requesting Gemini review for PR #%s...\n", prNumber)
		if err := client.CreatePRComment(prNumber, "/gemini review"); err != nil {
			return fmt.Errorf("failed to request Gemini review: %w", err)
		}
		fmt.Println("✅ Gemini review requested")
	}
	
	fmt.Printf("🔄 Waiting for both reviews AND PR checks for PR #%s (timeout: %s)...\n", prNumber, timeoutDisplay)
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

		// Combined GraphQL query for both reviews and checks using fragments
		query := shared.ReviewFragment + shared.AllStatusFragments + `
query($owner: String!, $repo: String!, $prNumber: Int!, $includeReviewBodies: Boolean!) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $prNumber) {
      mergeable
      mergeStateStatus
      reviews(last: 15) {
        nodes {
          ...ReviewFields
        }
      }
      commits(last: 1) {
        nodes {
          commit {
            ...CommitWithStatusFields
          }
        }
      }
    }
  }
}`

		variables := shared.BasicPRVariablesWithBodies(owner, repo, prNumberInt, true)

		result, err := client.RunGraphQLQueryWithVariables(query, variables)
		if err != nil {
			fmt.Printf("Error checking PR status: %v\n", err)
			time.Sleep(30 * time.Second)
			continue
		}

		var response shared.ReviewsWithStatusResponse

		if err := json.Unmarshal(result, &response); err != nil {
			fmt.Printf("Error parsing response: %v\n", err)
			time.Sleep(30 * time.Second)
			continue
		}

		// Check reviews status using common state tracking
		reviews := response.GetReviews()
		if len(reviews) > 0 && !reviewsReady {
			lastState, _ := loadReviewState(prNumber)
			reviewsReady = hasNewReviews(reviews, lastState)
		}

		// Check mergeable status first - if conflicting, stop immediately
		// CRITICAL INSIGHT: statusCheckRollup is null when PR has merge conflicts,
		// which prevents CI from running. This is GitHub's intentional behavior.
		// Must check mergeable before assuming "no checks required" scenario.
		mergeable, mergeStatus := response.GetMergeStatus()
		if mergeable == "CONFLICTING" {
			fmt.Printf("\n❌ [%s] PR has merge conflicts (status: %s)\n", time.Now().Format("15:04:05"), mergeStatus)
			fmt.Println("⚠️  CI checks will not run until conflicts are resolved")
			fmt.Printf("💡 Resolve conflicts with: git rebase origin/main\n")
			fmt.Printf("💡 Then push and run: bin/gh-helper reviews wait %s\n", prNumber)
			return fmt.Errorf("merge conflicts prevent CI execution")
		}

		// Check PR checks status
		statusCheckRollup := response.GetStatusCheckRollup()
		if statusCheckRollup != nil {
			rollupState := statusCheckRollup.State
			checksComplete = (rollupState == "SUCCESS" || rollupState == "FAILURE" || rollupState == "ERROR")
		} else {
			// StatusCheckRollup is nil - this can mean:
			// 1. No checks are configured for this repository (truly complete)
			// 2. Checks are configured but haven't started yet (not complete)
			// 3. PR was just created or pushed (checks pending)
			
			// Check merge status for better determination
			mergeable, mergeStatus := response.GetMergeStatus()
			
			// If PR has conflicts, checks won't run until resolved
			if mergeable == "CONFLICTING" {
				checksComplete = true // No point waiting for checks that won't run
			} else if mergeStatus == "CLEAN" || mergeStatus == "HAS_HOOKS" {
				// CLEAN: No checks configured, ready to merge
				// HAS_HOOKS: Only merge hooks configured, no status checks
				checksComplete = true
			} else {
				// PENDING, BLOCKED, DIRTY, UNKNOWN, etc. - checks may still be starting
				checksComplete = false
			}
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
			if statusCheckRollup != nil {
				rollupState := statusCheckRollup.State
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
				if statusCheckRollup != nil {
					rollupState := statusCheckRollup.State
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


func showThread(cmd *cobra.Command, args []string) error {
	threadID := args[0]
	
	// Create GitHub client once for better performance (token caching)
	client := shared.NewGitHubClient(owner, repo)

	fmt.Printf("🔍 Fetching details for thread: %s\n", threadID)

	query := `
query($threadID: ID!) {
  node(id: $threadID) {
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
}`

	variables := map[string]interface{}{
		"threadID": threadID,
	}

	result, err := client.RunGraphQLQueryWithVariables(query, variables)
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
				fmt.Printf("   gh-helper threads reply %s --commit-hash abc123 --message \"Fixed as suggested\"\n", threadID)
				fmt.Printf("   echo \"Your reply\" | gh-helper threads reply %s\n", threadID)
			}
		} else {
			// If we can't determine current user, show reply option anyway
			fmt.Printf("💡 To reply to this thread:\n")
			fmt.Printf("   gh-helper threads reply %s --message \"Your reply\"\n", threadID)
			fmt.Printf("   gh-helper threads reply %s --commit-hash abc123 --message \"Fixed as suggested\"\n", threadID)
			fmt.Printf("   echo \"Your reply\" | gh-helper threads reply %s\n", threadID)
		}
	}

	return nil
}


func replyToThread(cmd *cobra.Command, args []string) error {
	threadID := args[0]
	
	// Create GitHub client once for better performance (token caching)
	client := shared.NewGitHubClient(owner, repo)

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
		// If no message but commit hash is provided, use default message
		if commitHash != "" {
			replyText = "Thank you for the feedback!"
		} else {
			return fmt.Errorf("reply text cannot be empty (use --message or pipe content to stdin)")
		}
	}

	// Add commit reference if provided
	if commitHash != "" {
		replyText = fmt.Sprintf("%s\n\nFixed in commit %s.", strings.TrimSpace(replyText), commitHash)
	}

	// Add mention if provided
	if mentionUser != "" {
		replyText = fmt.Sprintf("@%s %s", mentionUser, replyText)
	}

	fmt.Printf("🔄 Replying to review thread: %s\n", threadID)

	// CRITICAL INSIGHT (Issue #301): GitHub GraphQL API quirk
	// Do NOT include pullRequestReviewId field - causes null responses despite being marked "optional" in schema
	// This was discovered during AI-friendly tool development and is documented in dev-docs/development-insights.md
	
	mutation := `
mutation($threadID: ID!, $body: String!) {
  addPullRequestReviewThreadReply(input: {
    pullRequestReviewThreadId: $threadID
    body: $body
  }) {
    comment {
      id
      url
      body
    }
  }
}`

	variables := map[string]interface{}{
		"threadID": threadID,
		"body":     replyText,
	}

	result, err := client.RunGraphQLQueryWithVariables(mutation, variables)
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

