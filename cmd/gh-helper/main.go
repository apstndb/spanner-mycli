package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

const DefaultOwner = "apstndb"
const DefaultRepo = "spanner-mycli"

var rootCmd = &cobra.Command{
	Use:   "gh-helper",
	Short: "Generic GitHub operations helper",
	Long: `gh-helper provides reusable GitHub operations that work well with AI assistants.

This tool focuses on AI-friendly patterns:
- stdin/heredoc support for content input
- Separate read/write commands for permission clarity  
- Self-documenting with comprehensive --help
- No temporary file dependencies`,
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

var (
	owner       string
	repo        string
	message     string
	mentionUser string
)

func init() {
	rootCmd.PersistentFlags().StringVar(&owner, "owner", DefaultOwner, "GitHub repository owner")
	rootCmd.PersistentFlags().StringVar(&repo, "repo", DefaultRepo, "GitHub repository name")

	replyThreadsCmd.Flags().StringVar(&message, "message", "", "Reply message (or use stdin)")
	replyThreadsCmd.Flags().StringVar(&mentionUser, "mention", "", "Username to mention (without @)")

	reviewsCmd.AddCommand(checkReviewsCmd)
	threadsCmd.AddCommand(listThreadsCmd, replyThreadsCmd)
	rootCmd.AddCommand(reviewsCmd, threadsCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
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

	// Update state with latest review
	if len(reviews) > 0 {
		latestReview := reviews[0]
		stateData := map[string]string{
			"id":        latestReview.ID,
			"createdAt": latestReview.CreatedAt,
		}
		if data, err := json.Marshal(stateData); err == nil {
			if err := os.WriteFile(lastReviewFile, data, 0644); err == nil {
				fmt.Printf("\n💾 Updated state: Latest review %s at %s\n", latestReview.ID, latestReview.CreatedAt)
			}
		}
	}

	fmt.Println("\n✅ Review check complete")
	return nil
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
}`, threadID, jsonEscape(replyText))

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
		return nil, fmt.Errorf("gh api failed: %w, stderr: %s", err, stderr.String())
	}

	return stdout.Bytes(), nil
}

func jsonEscape(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}