package shared

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// UnifiedReviewData represents all review-related data fetched in a single GraphQL query
type UnifiedReviewData struct {
	PR                PRMetadata       `json:"pr"`
	Reviews           []ReviewData     `json:"reviews"`
	Threads           []ThreadData     `json:"threads"`
	CurrentUser       string           `json:"currentUser"`
	FetchedAt         time.Time        `json:"fetchedAt"`
	ReviewPageInfo    PageInfo         `json:"reviewPageInfo"`
	ThreadPageInfo    PageInfo         `json:"threadPageInfo"`
}

// PageInfo contains pagination metadata following GitHub's GraphQL Relay spec
type PageInfo struct {
	HasNextPage     bool   `json:"hasNextPage"`
	HasPreviousPage bool   `json:"hasPreviousPage"`
	StartCursor     string `json:"startCursor"`
	EndCursor       string `json:"endCursor"`
	TotalCount      int    `json:"totalCount,omitempty"` // Not always available
}

// PRMetadata contains basic PR information
type PRMetadata struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	State       string `json:"state"`
	Mergeable   string `json:"mergeable"`
	MergeStatus string `json:"mergeStatus"`
}

// ReviewData represents a complete review with all its context
type ReviewData struct {
	ID           string           `json:"id"`
	Author       string           `json:"author"`
	CreatedAt    string           `json:"createdAt"`
	State        string           `json:"state"`
	Body         string           `json:"body"`
	Comments     []ReviewComment  `json:"comments"`
	Severity     ReviewSeverity   `json:"severity"`
	ActionItems  []string         `json:"actionItems"`
}

// ReviewComment represents a comment within a review
type ReviewComment struct {
	ID        string `json:"id"`
	Body      string `json:"body"`
	Path      string `json:"path"`
	Line      *int   `json:"line"`
	CreatedAt string `json:"createdAt"`
}

// ThreadData represents a review thread with full context
type ThreadData struct {
	ID          string        `json:"id"`
	Path        string        `json:"path"`
	Line        *int          `json:"line"`
	IsResolved  bool          `json:"isResolved"`
	IsOutdated  bool          `json:"isOutdated"`
	Comments    []ThreadComment `json:"comments"`
	NeedsReply  bool          `json:"needsReply"`
	LastReplier string        `json:"lastReplier"`
}

// ThreadComment represents a comment in a thread
type ThreadComment struct {
	ID        string `json:"id"`
	Author    string `json:"author"`
	Body      string `json:"body"`
	CreatedAt string `json:"createdAt"`
}

// UnifiedReviewOptions controls what data to fetch
type UnifiedReviewOptions struct {
	IncludeThreads       bool   // Include review threads (inline comments)
	IncludeReviewBodies  bool   // Include full review bodies
	ThreadLimit          int    // Max threads to fetch (default: 50)
	ReviewLimit          int    // Max reviews to fetch (default: 20)
	ReviewAfterCursor    string // Pagination cursor for reviews (for next page)
	ReviewBeforeCursor   string // Pagination cursor for reviews (for previous page)
	ThreadAfterCursor    string // Pagination cursor for threads
	NeedsReplyOnly       bool   // Filter to only unresolved threads (GraphQL optimization)
}

// DefaultUnifiedReviewOptions returns sensible defaults
func DefaultUnifiedReviewOptions() UnifiedReviewOptions {
	return UnifiedReviewOptions{
		IncludeThreads:      true,
		IncludeReviewBodies: true,
		ThreadLimit:         50,
		ReviewLimit:         20,
	}
}

// GetUnifiedReviewData fetches all review data in a single optimized GraphQL query
// 
// CRITICAL LESSON: Unified fetching prevents missing feedback (review bodies contain architecture insights)
// Uses GraphQL @include directives for flexible data fetching and parameterized queries for safety
func (c *GitHubClient) GetUnifiedReviewData(prNumber string, opts UnifiedReviewOptions) (*UnifiedReviewData, error) {
	prNumberInt, err := strconv.Atoi(prNumber)
	if err != nil {
		return nil, fmt.Errorf("invalid PR number format: %w", err)
	}

	// Apply defaults
	if opts.ThreadLimit <= 0 {
		opts.ThreadLimit = 50
	}
	if opts.ReviewLimit <= 0 {
		opts.ReviewLimit = 20
	}

	// Determine pagination strategy based on provided cursors
	useReviewsAfter := opts.ReviewAfterCursor != ""
	useReviewsBefore := opts.ReviewBeforeCursor != ""
	useThreadsAfter := opts.ThreadAfterCursor != ""
	useDefaultReviews := !useReviewsAfter && !useReviewsBefore
	useDefaultThreads := opts.IncludeThreads && !useThreadsAfter

	// GraphQL query with safe parameterized pagination
	// Uses variables and conditional directives for safety
	query := `
query($owner: String!, $repo: String!, $prNumber: Int!, 
      $includeReviewBodies: Boolean!,
      $reviewLimit: Int!, $threadLimit: Int!,
      $useDefaultReviews: Boolean!,
      $useReviewsAfter: Boolean!, $reviewAfterCursor: String!,
      $useReviewsBefore: Boolean!, $reviewBeforeCursor: String!,
      $useDefaultThreads: Boolean!,
      $useThreadsAfter: Boolean!, $threadAfterCursor: String!) {
  viewer {
    login
  }
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $prNumber) {
      number
      title
      state
      mergeable
      mergeStateStatus
      
      # Default reviews (latest)
      reviews(last: $reviewLimit) @include(if: $useDefaultReviews) {
        totalCount
        pageInfo {
          hasNextPage
          hasPreviousPage
          startCursor
          endCursor
        }
        nodes {
          id
          author { login }
          createdAt
          state
          body @include(if: $includeReviewBodies)
          comments(first: 50) @include(if: $includeReviewBodies) {
            nodes {
              id
              body
              path
              line
              createdAt
            }
          }
        }
      }
      
      # Reviews paginated forward
      reviewsAfter: reviews(first: $reviewLimit, after: $reviewAfterCursor) @include(if: $useReviewsAfter) {
        totalCount
        pageInfo {
          hasNextPage
          hasPreviousPage
          startCursor
          endCursor
        }
        nodes {
          id
          author { login }
          createdAt
          state
          body @include(if: $includeReviewBodies)
          comments(first: 50) @include(if: $includeReviewBodies) {
            nodes {
              id
              body
              path
              line
              createdAt
            }
          }
        }
      }
      
      # Reviews paginated backward
      reviewsBefore: reviews(last: $reviewLimit, before: $reviewBeforeCursor) @include(if: $useReviewsBefore) {
        totalCount
        pageInfo {
          hasNextPage
          hasPreviousPage
          startCursor
          endCursor
        }
        nodes {
          id
          author { login }
          createdAt
          state
          body @include(if: $includeReviewBodies)
          comments(first: 50) @include(if: $includeReviewBodies) {
            nodes {
              id
              body
              path
              line
              createdAt
            }
          }
        }
      }
      
      # Default threads
      reviewThreads(first: $threadLimit) @include(if: $useDefaultThreads) {
        totalCount
        pageInfo {
          hasNextPage
          hasPreviousPage
          startCursor
          endCursor
        }
        nodes {
          id
          path
          line
          isResolved
          isOutdated
          comments(first: 20) {
            nodes {
              id
              author { login }
              body
              createdAt
            }
          }
        }
      }
      
      # Threads paginated forward
      reviewThreadsAfter: reviewThreads(first: $threadLimit, after: $threadAfterCursor) @include(if: $useThreadsAfter) {
        totalCount
        pageInfo {
          hasNextPage
          hasPreviousPage
          startCursor
          endCursor
        }
        nodes {
          id
          path
          line
          isResolved
          isOutdated
          comments(first: 20) {
            nodes {
              id
              author { login }
              body
              createdAt
            }
          }
        }
      }
    }
  }
}`

	variables := map[string]interface{}{
		"owner":               c.Owner,
		"repo":                c.Repo,
		"prNumber":            prNumberInt,
		"includeReviewBodies": opts.IncludeReviewBodies,
		"reviewLimit":         opts.ReviewLimit,
		"threadLimit":         opts.ThreadLimit,
		"useDefaultReviews":   useDefaultReviews,
		"useReviewsAfter":     useReviewsAfter,
		"reviewAfterCursor":   opts.ReviewAfterCursor,
		"useReviewsBefore":    useReviewsBefore,
		"reviewBeforeCursor":  opts.ReviewBeforeCursor,
		"useDefaultThreads":   useDefaultThreads,
		"useThreadsAfter":     useThreadsAfter,
		"threadAfterCursor":   opts.ThreadAfterCursor,
	}

	result, err := c.RunGraphQLQueryWithVariables(query, variables)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch unified review data: %w", err)
	}

	// Parse the comprehensive response
	var response struct {
		Data struct {
			Viewer struct {
				Login string `json:"login"`
			} `json:"viewer"`
			Repository struct {
				PullRequest struct {
					Number           int    `json:"number"`
					Title            string `json:"title"`
					State            string `json:"state"`
					Mergeable        string `json:"mergeable"`
					MergeStateStatus string `json:"mergeStateStatus"`
					Reviews struct {
						TotalCount int `json:"totalCount"`
						PageInfo   struct {
							HasNextPage     bool   `json:"hasNextPage"`
							HasPreviousPage bool   `json:"hasPreviousPage"`
							StartCursor     string `json:"startCursor"`
							EndCursor       string `json:"endCursor"`
						} `json:"pageInfo"`
						Nodes []struct {
							ID        string `json:"id"`
							Author    struct {
								Login string `json:"login"`
							} `json:"author"`
							CreatedAt string `json:"createdAt"`
							State     string `json:"state"`
							Body      string `json:"body"`
							Comments  struct {
								Nodes []struct {
									ID        string `json:"id"`
									Body      string `json:"body"`
									Path      string `json:"path"`
									Line      *int   `json:"line"`
									CreatedAt string `json:"createdAt"`
								} `json:"nodes"`
							} `json:"comments"`
						} `json:"nodes"`
					} `json:"reviews"`
					ReviewsAfter struct {
						TotalCount int `json:"totalCount"`
						PageInfo   struct {
							HasNextPage     bool   `json:"hasNextPage"`
							HasPreviousPage bool   `json:"hasPreviousPage"`
							StartCursor     string `json:"startCursor"`
							EndCursor       string `json:"endCursor"`
						} `json:"pageInfo"`
						Nodes []struct {
							ID        string `json:"id"`
							Author    struct {
								Login string `json:"login"`
							} `json:"author"`
							CreatedAt string `json:"createdAt"`
							State     string `json:"state"`
							Body      string `json:"body"`
							Comments  struct {
								Nodes []struct {
									ID        string `json:"id"`
									Body      string `json:"body"`
									Path      string `json:"path"`
									Line      *int   `json:"line"`
									CreatedAt string `json:"createdAt"`
								} `json:"nodes"`
							} `json:"comments"`
						} `json:"nodes"`
					} `json:"reviewsAfter"`
					ReviewsBefore struct {
						TotalCount int `json:"totalCount"`
						PageInfo   struct {
							HasNextPage     bool   `json:"hasNextPage"`
							HasPreviousPage bool   `json:"hasPreviousPage"`
							StartCursor     string `json:"startCursor"`
							EndCursor       string `json:"endCursor"`
						} `json:"pageInfo"`
						Nodes []struct {
							ID        string `json:"id"`
							Author    struct {
								Login string `json:"login"`
							} `json:"author"`
							CreatedAt string `json:"createdAt"`
							State     string `json:"state"`
							Body      string `json:"body"`
							Comments  struct {
								Nodes []struct {
									ID        string `json:"id"`
									Body      string `json:"body"`
									Path      string `json:"path"`
									Line      *int   `json:"line"`
									CreatedAt string `json:"createdAt"`
								} `json:"nodes"`
							} `json:"comments"`
						} `json:"nodes"`
					} `json:"reviewsBefore"`
					ReviewThreads struct {
						TotalCount int `json:"totalCount"`
						PageInfo   struct {
							HasNextPage     bool   `json:"hasNextPage"`
							HasPreviousPage bool   `json:"hasPreviousPage"`
							StartCursor     string `json:"startCursor"`
							EndCursor       string `json:"endCursor"`
						} `json:"pageInfo"`
						Nodes []struct {
							ID         string `json:"id"`
							Path       string `json:"path"`
							Line       *int   `json:"line"`
							IsResolved bool   `json:"isResolved"`
							IsOutdated bool   `json:"isOutdated"`
							Comments   struct {
								Nodes []struct {
									ID        string `json:"id"`
									Author    struct {
										Login string `json:"login"`
									} `json:"author"`
									Body      string `json:"body"`
									CreatedAt string `json:"createdAt"`
								} `json:"nodes"`
							} `json:"comments"`
						} `json:"nodes"`
					} `json:"reviewThreads"`
					ReviewThreadsAfter struct {
						TotalCount int `json:"totalCount"`
						PageInfo   struct {
							HasNextPage     bool   `json:"hasNextPage"`
							HasPreviousPage bool   `json:"hasPreviousPage"`
							StartCursor     string `json:"startCursor"`
							EndCursor       string `json:"endCursor"`
						} `json:"pageInfo"`
						Nodes []struct {
							ID         string `json:"id"`
							Path       string `json:"path"`
							Line       *int   `json:"line"`
							IsResolved bool   `json:"isResolved"`
							IsOutdated bool   `json:"isOutdated"`
							Comments   struct {
								Nodes []struct {
									ID        string `json:"id"`
									Author    struct {
										Login string `json:"login"`
									} `json:"author"`
									Body      string `json:"body"`
									CreatedAt string `json:"createdAt"`
								} `json:"nodes"`
							} `json:"comments"`
						} `json:"nodes"`
					} `json:"reviewThreadsAfter"`
				} `json:"pullRequest"`
			} `json:"repository"`
		} `json:"data"`
	}

	if err := Unmarshal(result, &response); err != nil {
		return nil, fmt.Errorf("failed to parse unified review response: %w", err)
	}

	pr := response.Data.Repository.PullRequest
	currentUser := response.Data.Viewer.Login

	// Determine which review data to use based on pagination
	var reviewNodes []interface{}
	var reviewPageInfo PageInfo
	
	if useReviewsAfter {
		for _, review := range pr.ReviewsAfter.Nodes {
			reviewNodes = append(reviewNodes, review)
		}
		reviewPageInfo = PageInfo{
			HasNextPage:     pr.ReviewsAfter.PageInfo.HasNextPage,
			HasPreviousPage: pr.ReviewsAfter.PageInfo.HasPreviousPage,
			StartCursor:     pr.ReviewsAfter.PageInfo.StartCursor,
			EndCursor:       pr.ReviewsAfter.PageInfo.EndCursor,
			TotalCount:      pr.ReviewsAfter.TotalCount,
		}
	} else if useReviewsBefore {
		for _, review := range pr.ReviewsBefore.Nodes {
			reviewNodes = append(reviewNodes, review)
		}
		reviewPageInfo = PageInfo{
			HasNextPage:     pr.ReviewsBefore.PageInfo.HasNextPage,
			HasPreviousPage: pr.ReviewsBefore.PageInfo.HasPreviousPage,
			StartCursor:     pr.ReviewsBefore.PageInfo.StartCursor,
			EndCursor:       pr.ReviewsBefore.PageInfo.EndCursor,
			TotalCount:      pr.ReviewsBefore.TotalCount,
		}
	} else {
		for _, review := range pr.Reviews.Nodes {
			reviewNodes = append(reviewNodes, review)
		}
		reviewPageInfo = PageInfo{
			HasNextPage:     pr.Reviews.PageInfo.HasNextPage,
			HasPreviousPage: pr.Reviews.PageInfo.HasPreviousPage,
			StartCursor:     pr.Reviews.PageInfo.StartCursor,
			EndCursor:       pr.Reviews.PageInfo.EndCursor,
			TotalCount:      pr.Reviews.TotalCount,
		}
	}

	// Process reviews with severity analysis
	var reviews []ReviewData
	for _, reviewNode := range reviewNodes {
		// Convert interface{} back to structured data
		review := reviewNode.(struct {
			ID        string `json:"id"`
			Author    struct {
				Login string `json:"login"`
			} `json:"author"`
			CreatedAt string `json:"createdAt"`
			State     string `json:"state"`
			Body      string `json:"body"`
			Comments  struct {
				Nodes []struct {
					ID        string `json:"id"`
					Body      string `json:"body"`
					Path      string `json:"path"`
					Line      *int   `json:"line"`
					CreatedAt string `json:"createdAt"`
				} `json:"nodes"`
			} `json:"comments"`
		})
		
		if review.State == "PENDING" {
			continue
		}

		// Analyze review body
		severity := analyzeReviewSeverity(review.Body)
		actionItems := extractActionItems(review.Body)

		// Process review comments
		var comments []ReviewComment
		for _, comment := range review.Comments.Nodes {
			comments = append(comments, ReviewComment{
				ID:        comment.ID,
				Body:      comment.Body,
				Path:      comment.Path,
				Line:      comment.Line,
				CreatedAt: comment.CreatedAt,
			})
		}

		reviews = append(reviews, ReviewData{
			ID:          review.ID,
			Author:      review.Author.Login,
			CreatedAt:   review.CreatedAt,
			State:       review.State,
			Body:        review.Body,
			Comments:    comments,
			Severity:    severity,
			ActionItems: actionItems,
		})
	}

	// Process threads - focus on resolved status only
	var threads []ThreadData
	for _, thread := range pr.ReviewThreads.Nodes {
		// Apply NeedsReplyOnly filter - simply means unresolved threads
		if opts.NeedsReplyOnly && thread.IsResolved {
			continue  // Skip resolved threads when only unresolved threads are requested
		}
		
		var comments []ThreadComment
		lastReplier := ""

		for _, comment := range thread.Comments.Nodes {
			comments = append(comments, ThreadComment{
				ID:        comment.ID,
				Author:    comment.Author.Login,
				Body:      comment.Body,
				CreatedAt: comment.CreatedAt,
			})

			lastReplier = comment.Author.Login
		}

		// Simplified: NeedsReply = !IsResolved
		// The workflow is: reply with fix/explanation → resolve thread
		// So unresolved threads are the ones that need attention
		needsReply := !thread.IsResolved

		threads = append(threads, ThreadData{
			ID:          thread.ID,
			Path:        thread.Path,
			Line:        thread.Line,
			IsResolved:  thread.IsResolved,
			IsOutdated:  thread.IsOutdated,
			Comments:    comments,
			NeedsReply:  needsReply,
			LastReplier: lastReplier,
		})
	}

	// Use the pagination info from the appropriate source

	var threadPageInfo PageInfo
	if opts.IncludeThreads {
		threadPageInfo = PageInfo{
			HasNextPage:     pr.ReviewThreads.PageInfo.HasNextPage,
			HasPreviousPage: pr.ReviewThreads.PageInfo.HasPreviousPage,
			StartCursor:     pr.ReviewThreads.PageInfo.StartCursor,
			EndCursor:       pr.ReviewThreads.PageInfo.EndCursor,
			TotalCount:      pr.ReviewThreads.TotalCount,
		}
	}

	return &UnifiedReviewData{
		PR: PRMetadata{
			Number:      pr.Number,
			Title:       pr.Title,
			State:       pr.State,
			Mergeable:   pr.Mergeable,
			MergeStatus: pr.MergeStateStatus,
		},
		Reviews:        reviews,
		Threads:        threads,
		CurrentUser:    currentUser,
		FetchedAt:      time.Now(),
		ReviewPageInfo: reviewPageInfo,
		ThreadPageInfo: threadPageInfo,
	}, nil
}

// analyzeReviewSeverity determines the severity of review feedback
func analyzeReviewSeverity(body string) ReviewSeverity {
	bodyLower := strings.ToLower(body)

	// Check explicit severity markers (like Gemini uses)
	if strings.Contains(body, "![critical]") || strings.Contains(bodyLower, "critical") {
		return SeverityCritical
	}
	if strings.Contains(body, "![high]") || strings.Contains(bodyLower, "high-severity") ||
		strings.Contains(bodyLower, "high-priority") {
		return SeverityHigh
	}
	// Removed medium/low levels for simplification

	// Check for critical keywords
	criticalPatterns := []string{
		"panic", "crash", "security", "vulnerability", "injection",
		"leak", "exposed", "hardcoded.*password", "hardcoded.*credential",
	}
	for _, pattern := range criticalPatterns {
		if strings.Contains(bodyLower, pattern) {
			return SeverityCritical
		}
	}

	// Check for high priority keywords
	highPatterns := []string{
		"bug", "error", "broken", "incorrect", "wrong",
		"fail", "nil.*handling", "null.*reference",
	}
	for _, pattern := range highPatterns {
		if strings.Contains(bodyLower, pattern) {
			return SeverityHigh
		}
	}

	return SeverityInfo
}

// extractActionItems finds specific issues mentioned in review
func extractActionItems(body string) []string {
	var items []string
	lines := strings.Split(body, "\n")
	
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		
		// Look for specific patterns that indicate action items
		actionPatterns := []string{
			"should", "must", "need to", "please", "consider",
			"fix", "update", "change", "remove", "add",
		}
		
		hasAction := false
		for _, pattern := range actionPatterns {
			if strings.Contains(lower, pattern) {
				hasAction = true
				break
			}
		}
		
		// Also check for issue indicators
		issuePatterns := []string{
			"issue", "problem", "concern", "bug", "error",
			"incorrect", "wrong", "missing", "todo", "fixme",
		}
		
		for _, pattern := range issuePatterns {
			if strings.Contains(lower, pattern) {
				hasAction = true
				break
			}
		}
		
		if hasAction && len(trimmed) > 20 && len(trimmed) < 200 {
			items = append(items, trimmed)
		}
	}
	
	// Deduplicate similar items
	seen := make(map[string]bool)
	unique := []string{}
	for _, item := range items {
		key := strings.ToLower(item)
		if !seen[key] {
			seen[key] = true
			unique = append(unique, item)
		}
	}
	
	return unique
}

// GetActionableItems returns all items requiring attention from unified data
func (data *UnifiedReviewData) GetActionableItems() []ActionableItem {
	var items []ActionableItem

	// Extract from reviews
	for _, review := range data.Reviews {
		// High severity reviews always need attention
		if review.Severity == SeverityCritical || review.Severity == SeverityHigh {
			items = append(items, ActionableItem{
				Type:        "review",
				ID:          review.ID,
				Severity:    review.Severity,
				Author:      review.Author,
				Summary:     fmt.Sprintf("Review requires attention: %s", truncateString(review.Body, 100)),
				Location:    "General Review",
				ActionItems: review.ActionItems,
			})
		}

		// Check review comments for issues
		for _, comment := range review.Comments {
			if containsIssueKeywords(comment.Body) {
				location := comment.Path
				if comment.Line != nil {
					location = fmt.Sprintf("%s:%d", comment.Path, *comment.Line)
				}
				items = append(items, ActionableItem{
					Type:     "review_comment",
					ID:       comment.ID,
					Severity: SeverityHigh,
					Author:   review.Author,
					Summary:  truncateString(comment.Body, 100),
					Location: location,
				})
			}
		}
	}

	// Extract from threads needing replies
	for _, thread := range data.Threads {
		if thread.NeedsReply && !thread.IsResolved && !thread.IsOutdated {
			location := thread.Path
			if thread.Line != nil {
				location = fmt.Sprintf("%s:%d", thread.Path, *thread.Line)
			}
			
			summary := "Thread needs reply"
			if len(thread.Comments) > 0 {
				lastComment := thread.Comments[len(thread.Comments)-1]
				summary = fmt.Sprintf("Reply needed to %s: %s", 
					lastComment.Author, 
					truncateString(lastComment.Body, 80))
			}

			items = append(items, ActionableItem{
				Type:     "thread",
				ID:       thread.ID,
				Severity: SeverityHigh,
				Author:   thread.LastReplier,
				Summary:  summary,
				Location: location,
			})
		}
	}

	return items
}

// ActionableItem represents something requiring developer attention
type ActionableItem struct {
	Type        string         `json:"type"`        // review, review_comment, thread
	ID          string         `json:"id"`
	Severity    ReviewSeverity `json:"severity"`
	Author      string         `json:"author"`
	Summary     string         `json:"summary"`
	Location    string         `json:"location"`
	ActionItems []string       `json:"actionItems,omitempty"`
}

// containsIssueKeywords checks if text contains keywords indicating issues
func containsIssueKeywords(text string) bool {
	lower := strings.ToLower(text)
	keywords := []string{
		"bug", "error", "issue", "problem", "incorrect", "wrong",
		"fix", "todo", "fixme", "broken", "fail", "missing",
	}
	
	for _, keyword := range keywords {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return false
}

// truncateString truncates a string to maxLen with ellipsis
func truncateString(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}