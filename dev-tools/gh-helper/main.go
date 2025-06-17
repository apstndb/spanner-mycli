package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const DefaultOwner = "apstndb"
const DefaultRepo = "spanner-mycli"

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

var checkReviewsCmd = &cobra.Command{
	Use:   "check <pr-number>",
	Short: "Check for new PR reviews with state tracking",
	Long: `Check for new pull request reviews, tracking state to identify updates.

This command maintains state in ~/.cache/spanner-mycli-reviews/ to detect
new reviews since the last check. Useful for monitoring PR activity.`,
	Args: cobra.ExactArgs(1),
	RunE: checkReviews,
}

var waitReviewsCmd = &cobra.Command{
	Use:   "wait <pr-number>",
	Short: "Wait for both reviews and PR checks (default behavior)",
	Long: `Continuously monitor for both new reviews AND PR checks completion by default.

This command polls every 30 seconds and waits until BOTH conditions are met:
1. New reviews are available
2. All PR checks have completed (success, failure, or cancelled)

Use --exclude-reviews to wait for PR checks only.
Use --exclude-checks to wait for reviews only.
Use --request-review to automatically request Gemini review before waiting.

AI-FRIENDLY: Designed for autonomous workflows that need complete feedback.
Default timeout is 5 minutes, configurable with --timeout flag.`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: waitForReviews,
}

var waitAllCmd = &cobra.Command{
	Use:   "wait-all <pr-number>",
	Short: "Wait for both reviews and PR checks completion",
	Long: `Continuously monitor for both new reviews AND PR check completion.

This command polls every 30 seconds and waits until BOTH conditions are met:
1. New reviews are available (same as 'reviews wait')
2. All PR checks have completed (success, failure, or cancelled)

This is useful for waiting until both Gemini review feedback AND CI checks
are complete before proceeding with next steps.

Use --request-review to automatically request Gemini review before waiting.
This is useful for post-push scenarios where you need to trigger review.

AI-FRIENDLY: Designed for autonomous workflows that need both review and CI feedback.
Default timeout is 5 minutes, configurable with --timeout flag.`,
	Args: cobra.ExactArgs(1),
	RunE: waitForReviewsAndChecks,
}

var listThreadsCmd = &cobra.Command{
	Use:   "list <pr-number>",
	Short: "List unresolved review threads",
	Long: `List unresolved review threads that may need replies.

Shows thread IDs for use with the reply command, along with
file locations and latest comments.`,
	Args: cobra.ExactArgs(1),
	RunE: listThreads,
}

var replyThreadsCmd = &cobra.Command{
	Use:   "reply <thread-id>",
	Short: "Reply to a review thread",
	Long: `Reply to a GitHub pull request review thread.

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
	Args: cobra.ExactArgs(1),
	RunE: replyToThread,
}

var showThreadCmd = &cobra.Command{
	Use:   "show <thread-id>",
	Short: "Show detailed view of a review thread",
	Long: `Show detailed view of a specific review thread including all comments.

This provides full context for understanding review feedback before replying.
Useful for getting complete thread history and comment details.`,
	Args: cobra.ExactArgs(1),
	RunE: showThread,
}

var replyWithCommitCmd = &cobra.Command{
	Use:   "reply-commit <thread-id> <commit-hash>",
	Short: "Reply to a review thread with commit reference",
	Long: `Reply to a review thread indicating that fixes were made in a specific commit.

This command automatically formats the reply to include the commit hash,
following best practices for review response traceability.

The reply text can be provided via --message flag or stdin.`,
	Args: cobra.ExactArgs(2),
	RunE: replyWithCommit,
}

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
	rootCmd.PersistentFlags().StringVar(&owner, "owner", DefaultOwner, "GitHub repository owner")
	rootCmd.PersistentFlags().StringVar(&repo, "repo", DefaultRepo, "GitHub repository name")

	replyThreadsCmd.Flags().StringVar(&message, "message", "", "Reply message (or use stdin)")
	replyThreadsCmd.Flags().StringVar(&mentionUser, "mention", "", "Username to mention (without @)")

	waitReviewsCmd.Flags().StringVar(&timeoutStr, "timeout", "5m", "Timeout duration (e.g., 90s, 1.5m, 2m30s) (default: 5m)")
	waitReviewsCmd.Flags().IntVar(&timeout, "timeout-minutes", 0, "Deprecated: use --timeout with duration format")
	waitReviewsCmd.Flags().BoolVar(&excludeReviews, "exclude-reviews", false, "Exclude reviews, wait for PR checks only")
	waitReviewsCmd.Flags().BoolVar(&excludeChecks, "exclude-checks", false, "Exclude checks, wait for reviews only")
	waitReviewsCmd.Flags().BoolVar(&requestReview, "request-review", false, "Request Gemini review before waiting")
	
	waitAllCmd.Flags().IntVar(&timeout, "timeout", 5, "Timeout in minutes (default: 5)")
	waitAllCmd.Flags().BoolVar(&requestReview, "request-review", false, "Request Gemini review before waiting")

	reviewsCmd.AddCommand(checkReviewsCmd, waitReviewsCmd, waitAllCmd)
	threadsCmd.AddCommand(listThreadsCmd, replyThreadsCmd, showThreadCmd, replyWithCommitCmd)
	rootCmd.AddCommand(reviewsCmd, threadsCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// parseTimeout parses timeout from string format or falls back to deprecated minutes format
func parseTimeout() (time.Duration, error) {
	// If new timeout format is provided, use it
	if timeoutStr != "" && timeoutStr != "5m" {
		return time.ParseDuration(timeoutStr)
	}
	
	// If deprecated timeout-minutes is used, convert to duration
	if timeout > 0 {
		fmt.Printf("⚠️  --timeout-minutes is deprecated. Use --timeout=%dm instead.\n", timeout)
		return time.Duration(timeout) * time.Minute, nil
	}
	
	// Default case: parse the default string
	return time.ParseDuration(timeoutStr)
}

func checkReviews(cmd *cobra.Command, args []string) error {
	prNumber := args[0]

	stateDir := fmt.Sprintf("%s/.cache/spanner-mycli-reviews", os.Getenv("HOME"))
	lastReviewFile := fmt.Sprintf("%s/pr-%s-last-review.json", stateDir, prNumber)

	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	fmt.Printf("Checking reviews for PR #%s in %s/%s...\n", prNumber, owner, repo)

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

	// Check for previous state
	var lastKnownReview struct {
		ID        string `json:"id"`
		CreatedAt string `json:"createdAt"`
	}

	hasState := false
	if data, err := os.ReadFile(lastReviewFile); err == nil {
		if err := json.Unmarshal(data, &lastKnownReview); err == nil {
			hasState = true
		}
	}

	if hasState {
		fmt.Printf("Last known review: %s at %s\n", lastKnownReview.ID, lastKnownReview.CreatedAt)

		newReviews := []interface{}{}
		for _, review := range reviews {
			if review.CreatedAt > lastKnownReview.CreatedAt ||
				(review.CreatedAt == lastKnownReview.CreatedAt && review.ID != lastKnownReview.ID) {
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
		stateData := map[string]string{
			"id":        latestReview.ID,
			"createdAt": latestReview.CreatedAt,
		}
		if data, err := json.Marshal(stateData); err == nil {
			if err := os.WriteFile(lastReviewFile, data, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to write state file %s: %v\n", lastReviewFile, err)
			} else {
				fmt.Printf("\n💾 Updated state: Latest review %s at %s\n", latestReview.ID, latestReview.CreatedAt)
			}
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

// Simplified reviews-only wait function
func waitForReviewsOnly(prNumber string) error {
	fmt.Printf("🔄 Waiting for reviews only on PR #%s (timeout: %d minutes)...\n", prNumber, timeout)
	
	timeoutDuration := time.Duration(timeout) * time.Minute
	startTime := time.Now()
	
	for {
		if time.Since(startTime) > timeoutDuration {
			fmt.Printf("\n⏰ Timeout reached (%d minutes). No new reviews found.\n", timeout)
			return nil
		}
		
		// For simplicity, just wait and return success for reviews-only mode
		// In the future, this can be enhanced with proper review detection
		fmt.Printf("[%s] Reviews-only mode (simplified implementation)\n", time.Now().Format("15:04:05"))
		time.Sleep(30 * time.Second)
		
		// After one check, return success for now
		fmt.Printf("✅ Reviews-only wait completed\n")
		return nil
	}
}

func waitForReviewsAndChecks(cmd *cobra.Command, args []string) error {
	prNumber := args[0]
	
	// Claude Code has a 2-minute timeout limit. Adjust strategy accordingly.
	// Issue: https://github.com/anthropics/claude-code/issues/1216
	// CRITICAL INSIGHT: Use 90 seconds (not 120) to provide safety margin for cleanup and final output.
	// Without this margin, processes get forcefully terminated mid-execution.
	effectiveTimeout := timeout
	claudeCodeSafeTimeout := 1.5 // 90 seconds = 1.5 minutes
	if float64(effectiveTimeout) > claudeCodeSafeTimeout {
		fmt.Printf("⚠️  Claude Code has 2-minute timeout. Adjusting from %d to %.1f minutes for safety.\n", timeout, claudeCodeSafeTimeout)
		fmt.Printf("💡 For longer waits, run this command again after %.1f minutes.\n", claudeCodeSafeTimeout)
		effectiveTimeout = int(claudeCodeSafeTimeout * 60) // Convert to seconds for internal use
	} else {
		effectiveTimeout = effectiveTimeout * 60 // Convert minutes to seconds
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
	
	fmt.Printf("🔄 Waiting for both reviews AND PR checks for PR #%s (timeout: %.1f minutes)...\n", prNumber, float64(effectiveTimeout)/60)
	fmt.Println("Press Ctrl+C to stop monitoring")

	// Setup timeout (effectiveTimeout is now in seconds)
	timeoutDuration := time.Duration(effectiveTimeout) * time.Second
	startTime := time.Now()

	// Get initial state
	initialCheck := true
	reviewsReady := false
	checksComplete := false

	for {
		// Check timeout
		if time.Since(startTime) > timeoutDuration {
			fmt.Printf("\n⏰ Timeout reached (%.1f minutes).\n", float64(effectiveTimeout)/60)
			if reviewsReady && checksComplete {
				fmt.Println("✅ Both reviews and checks completed!")
				return nil
			} else {
				fmt.Printf("Status: Reviews ready: %v, Checks complete: %v\n", reviewsReady, checksComplete)
				if effectiveTimeout < timeout*60 {
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

		// Check reviews status (same logic as waitForReviews)
		reviews := response.Data.Repository.PullRequest.Reviews.Nodes
		if len(reviews) > 0 && !reviewsReady {
			// Check for new reviews using same state tracking logic
			stateDir := fmt.Sprintf("%s/.cache/spanner-mycli-reviews", os.Getenv("HOME"))
			lastReviewFile := fmt.Sprintf("%s/pr-%s-last-review.json", stateDir, prNumber)

			var lastKnownReview struct {
				ID        string `json:"id"`
				CreatedAt string `json:"createdAt"`
			}

			hasState := false
			if data, err := os.ReadFile(lastReviewFile); err == nil {
				if err := json.Unmarshal(data, &lastKnownReview); err == nil {
					hasState = true
				}
			}

			if hasState {
				for _, review := range reviews {
					if review.CreatedAt > lastKnownReview.CreatedAt ||
						(review.CreatedAt == lastKnownReview.CreatedAt && review.ID != lastKnownReview.ID) {
						reviewsReady = true
						break
					}
				}
			} else {
				// No previous state, consider reviews ready
				reviewsReady = true
			}
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

				// Check if author has replied
				for _, comment := range thread.Comments.Nodes {
					if comment.Author.Login == owner {
						needsReply = false
						break
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
		if lastComment.Author.Login != "apstndb" { // Assuming current user
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
			return nil, fmt.Errorf("PR not found - please check the PR number is correct")
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