package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/apstndb/spanner-mycli/dev-tools/shared"
	"github.com/spf13/cobra"
)

// prNumberArgsHelp is defined in main.go and used here for consistency

var analyzeReviewsCmd = &cobra.Command{
	Use:   "analyze [pr-number]",
	Short: "Analyze all review feedback for actionable items",
	Long: `Comprehensive analysis of all review feedback including severity detection.

This command addresses the lesson learned from PR #306 where critical feedback
in review bodies (like statusCheckRollup nil handling) was missed because
reviews and threads were fetched separately.

`+prNumberArgsHelp+`

Examples:
  # Complete analysis (reviews + threads)
  gh-helper reviews analyze 306
  gh-helper reviews analyze        # Auto-detect current branch PR

  # Focus on specific types
  gh-helper reviews analyze 306 --json | jq '.reviews[]'`,
	Args: cobra.MaximumNArgs(1),
	RunE: analyzeReviews,
}

var fetchReviewsCmd = &cobra.Command{
	Use:   "fetch [pr-number]",
	Short: "Fetch review data with configurable options",
	Long: `Fetch reviews and threads in a single optimized GraphQL query.

`+prNumberArgsHelp+`

Examples:
  # Full fetch (reviews + threads + bodies)
  gh-helper reviews fetch 306
  gh-helper reviews fetch        # Auto-detect current branch PR

  # Reviews only (no threads)
  gh-helper reviews fetch 306 --no-threads

  # Lightweight - just review states, no bodies
  gh-helper reviews fetch 306 --no-bodies

  # Custom limits and pagination
  gh-helper reviews fetch 306 --review-limit 10 --thread-limit 30
  gh-helper reviews fetch 306 --reviews-after CURSOR`,
	Args: cobra.MaximumNArgs(1),
	RunE: fetchReviews,
}

var (
	includeThreads        bool
	includeReviewBodies   bool
	threadLimit           int
	reviewLimit           int
	reviewAfterCursor     string
	reviewBeforeCursor    string
	threadAfterCursor     string
)

func init() {
	// Fetch command flags
	fetchReviewsCmd.Flags().BoolVar(&includeThreads, "threads", true, "Include review threads")
	fetchReviewsCmd.Flags().BoolVar(&includeReviewBodies, "bodies", true, "Include review bodies")
	fetchReviewsCmd.Flags().IntVar(&threadLimit, "thread-limit", 50, "Maximum threads to fetch")
	fetchReviewsCmd.Flags().IntVar(&reviewLimit, "review-limit", 20, "Maximum reviews to fetch")
	fetchReviewsCmd.Flags().Bool("threads-only", false, "Output only threads that need replies (implies --no-bodies --json)")
	fetchReviewsCmd.Flags().Bool("list-threads", false, "List thread IDs only, one per line (implies --threads-only)")
	fetchReviewsCmd.Flags().Bool("needs-reply-only", false, "Include only threads that need replies (filters at data level)")

	// Pagination flags
	fetchReviewsCmd.Flags().StringVar(&reviewAfterCursor, "reviews-after", "", "Reviews pagination: fetch after this cursor")
	fetchReviewsCmd.Flags().StringVar(&reviewBeforeCursor, "reviews-before", "", "Reviews pagination: fetch before this cursor") 
	fetchReviewsCmd.Flags().StringVar(&threadAfterCursor, "threads-after", "", "Threads pagination: fetch after this cursor")

	// Convenience flags
	fetchReviewsCmd.Flags().Bool("no-threads", false, "Exclude threads (shorthand for --threads=false)")
	fetchReviewsCmd.Flags().Bool("no-bodies", false, "Exclude bodies (shorthand for --bodies=false)")
}

func fetchReviews(cmd *cobra.Command, args []string) error {
	client := shared.NewGitHubClient(owner, repo)
	prNumber, err := resolvePRNumberFromArgs(args, client)
	if err != nil {
		return err
	}

	// Handle convenience and specialized flags
	if noThreads, _ := cmd.Flags().GetBool("no-threads"); noThreads {
		includeThreads = false
	}
	if noBodies, _ := cmd.Flags().GetBool("no-bodies"); noBodies {
		includeReviewBodies = false
	}
	
	// Check for specialized thread modes
	threadsOnly, _ := cmd.Flags().GetBool("threads-only")
	listThreads, _ := cmd.Flags().GetBool("list-threads")
	needsReplyOnly, _ := cmd.Flags().GetBool("needs-reply-only")
	
	// Get output format using unified resolver
	format := shared.ResolveFormat(cmd)
	
	// Adjust flags for thread-focused modes
	if listThreads || threadsOnly {
		format = shared.FormatJSON // Force JSON for thread-focused modes
		includeReviewBodies = false
		needsReplyOnly = true  // Implied for thread-focused modes
		if listThreads {
			threadsOnly = true
		}
	}
	
	opts := shared.UnifiedReviewOptions{
		IncludeThreads:      includeThreads,
		IncludeReviewBodies: includeReviewBodies,
		ThreadLimit:         threadLimit,
		ReviewLimit:         reviewLimit,
		NeedsReplyOnly:      needsReplyOnly,
	}

	// Use structured logging (slog) for consistent format with JSON/YAML output
	slog.Info("fetching review data",
		"pr", prNumber,
		"options", map[string]interface{}{
			"threads": opts.IncludeThreads,
			"bodies": opts.IncludeReviewBodies,
			"review_limit": opts.ReviewLimit,
			"thread_limit": opts.ThreadLimit,
			"needs_reply_only": opts.NeedsReplyOnly,
		})

	data, err := client.GetUnifiedReviewData(prNumber, opts)
	if err != nil {
		return fmt.Errorf("failed to fetch unified data: %w", err)
	}

	// Handle specialized modes
	if listThreads {
		// Simple list mode: just unresolved thread IDs
		for _, thread := range data.Threads {
			if !thread.IsResolved {
				fmt.Println(thread.ID)
			}
		}
		return nil
	}
	
	if threadsOnly {
		// Filter to only unresolved threads
		unresolvedThreads := []shared.ThreadData{}
		for _, thread := range data.Threads {
			if !thread.IsResolved {
				unresolvedThreads = append(unresolvedThreads, thread)
			}
		}
		return shared.EncodeOutput(os.Stdout, format, unresolvedThreads)
	}
	
	// Full data output
	if format == shared.FormatJSON {
		return shared.EncodeOutput(os.Stdout, format, data)
	}
	
	// Use specialized output function for better structure
	return outputFetch(data, includeReviewBodies, includeThreads, format)
}

func analyzeReviews(cmd *cobra.Command, args []string) error {
	client := shared.NewGitHubClient(owner, repo)
	prNumber, err := resolvePRNumberFromArgs(args, client)
	if err != nil {
		return err
	}
	
	// Get output format using unified resolver
	format := shared.ResolveFormat(cmd)

	// Always fetch everything for analysis
	opts := shared.DefaultUnifiedReviewOptions()

	data, err := client.GetUnifiedReviewData(prNumber, opts)
	if err != nil {
		return fmt.Errorf("failed to fetch data: %w", err)
	}

	// Get all actionable items
	items := data.GetActionableItems()

	// Use unified analysis output
	return outputAnalysis(data, items, format)
}

// outputAnalysis creates unified analysis output using GitHub GraphQL API types
func outputAnalysis(data *shared.UnifiedReviewData, items []shared.ActionableItem, format shared.OutputFormat) error {
	// Calculate severity counts
	severityCounts := make(map[string]int)
	for _, item := range items {
		severityCounts[string(item.Severity)]++
	}
	
	// Calculate thread counts
	threadsNeedReply := 0
	threadsUnresolved := 0
	for _, thread := range data.Threads {
		if thread.NeedsReply && !thread.IsResolved {
			threadsNeedReply++
		}
		if !thread.IsResolved {
			threadsUnresolved++
		}
	}
	
	// Build actionable items using consistent structure
	type ActionItem struct {
		ID       string `json:"id" yaml:"id"`
		Type     string `json:"type" yaml:"type"`
		Author   string `json:"author" yaml:"author"`
		Location string `json:"location" yaml:"location"`
		Summary  string `json:"summary" yaml:"summary"`
		Severity string `json:"severity" yaml:"severity"`
	}
	
	criticalItems := []ActionItem{}
	highItems := []ActionItem{}
	infoItems := []ActionItem{}
	
	for _, item := range items {
		actionItem := ActionItem{
			ID:       item.ID,
			Type:     item.Type,
			Author:   item.Author,
			Location: item.Location,
			Summary:  item.Summary,
			Severity: string(item.Severity),
		}
		
		switch item.Severity {
		case shared.SeverityCritical:
			criticalItems = append(criticalItems, actionItem)
		case shared.SeverityHigh:
			highItems = append(highItems, actionItem)
		default:
			infoItems = append(infoItems, actionItem)
		}
	}
	
	// Build unresolved threads using GitHub GraphQL thread structure
	type ThreadSummary struct {
		ID          string `json:"id" yaml:"id"`
		Path        string `json:"path" yaml:"path"`
		Line        *int   `json:"line,omitempty" yaml:"line,omitempty"`
		LastReplier string `json:"lastReplier" yaml:"lastReplier"`
		IsResolved  bool   `json:"isResolved" yaml:"isResolved"`
		IsOutdated  bool   `json:"isOutdated" yaml:"isOutdated"`
	}
	
	unresolvedThreads := []ThreadSummary{}
	for _, thread := range data.Threads {
		if !thread.IsResolved {
			unresolvedThreads = append(unresolvedThreads, ThreadSummary{
				ID:          thread.ID,
				Path:        thread.Path,
				Line:        thread.Line,
				LastReplier: thread.LastReplier,
				IsResolved:  thread.IsResolved,
				IsOutdated:  thread.IsOutdated,
			})
		}
	}
	
	// Create unified output structure using GitHub GraphQL PR metadata
	output := map[string]interface{}{
		// Use GitHub GraphQL PR fields directly
		"number":    data.PR.Number,
		"title":     data.PR.Title,
		"state":     data.PR.State,
		"mergeable": data.PR.Mergeable,
		"mergeStateStatus": data.PR.MergeStatus,
		
		// Analysis summary
		"actionRequired": len(criticalItems) > 0 || len(highItems) > 0 || len(unresolvedThreads) > 0,
		"summary": map[string]int{
			"critical":           severityCounts["CRITICAL"],
			"high":               severityCounts["HIGH"],
			"info":               severityCounts["INFO"],
			"threadsUnresolved":  len(unresolvedThreads),
			"actionableItems":    len(items),
		},
		
		// Current user from GitHub GraphQL viewer
		"currentUser": data.CurrentUser,
		"fetchedAt":   data.FetchedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	
	// Only include non-empty sections
	if len(criticalItems) > 0 {
		output["criticalItems"] = criticalItems
	}
	if len(highItems) > 0 {
		output["highPriorityItems"] = highItems
	}
	if len(infoItems) > 0 {
		output["infoItems"] = infoItems
	}
	if len(unresolvedThreads) > 0 {
		output["unresolvedThreads"] = unresolvedThreads
	}
	
	// Output using unified encoder
	return shared.EncodeOutput(os.Stdout, format, output)
}

// outputFetch creates unified fetch output using GitHub GraphQL API types
func outputFetch(data *shared.UnifiedReviewData, includeReviewBodies bool, includeThreads bool, format shared.OutputFormat) error {
	// Use GitHub GraphQL PR metadata structure directly
	output := map[string]interface{}{
		// GitHub GraphQL PullRequest fields
		"number":           data.PR.Number,
		"title":            data.PR.Title,
		"state":            data.PR.State,
		"mergeable":        data.PR.Mergeable,
		"mergeStateStatus": data.PR.MergeStatus,
		
		// GitHub GraphQL Viewer field
		"currentUser": data.CurrentUser,
		"fetchedAt":   data.FetchedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	
	// Reviews section using GitHub GraphQL Review structure
	if includeReviewBodies {
		// Full review data with bodies
		reviews := []map[string]interface{}{}
		for _, review := range data.Reviews {
			reviewData := map[string]interface{}{
				"id":        review.ID,
				"author":    map[string]string{"login": review.Author},
				"state":     review.State,
				"createdAt": review.CreatedAt,
				"severity":  string(review.Severity),
			}
			
			if review.Body != "" {
				reviewData["body"] = review.Body
			}
			
			if len(review.ActionItems) > 0 {
				reviewData["actionItems"] = review.ActionItems
			}
			
			if len(review.Comments) > 0 {
				reviewData["commentsCount"] = len(review.Comments)
			}
			
			reviews = append(reviews, reviewData)
		}
		output["reviews"] = reviews
	} else {
		// Minimal review data without bodies
		reviews := []map[string]interface{}{}
		for _, review := range data.Reviews {
			reviews = append(reviews, map[string]interface{}{
				"id":        review.ID,
				"author":    map[string]string{"login": review.Author},
				"state":     review.State,
				"createdAt": review.CreatedAt,
			})
		}
		output["reviews"] = reviews
		output["reviewBodiesFetched"] = false
	}
	
	// Threads section using GitHub GraphQL ReviewThread structure
	if includeThreads {
		unresolvedCount := 0
		unresolvedThreads := []map[string]interface{}{}
		
		for _, thread := range data.Threads {
			if !thread.IsResolved {
				unresolvedCount++
				
				threadData := map[string]interface{}{
					"id":         thread.ID,
					"path":       thread.Path,
					"line":       thread.Line,
					"isResolved": thread.IsResolved,
					"isOutdated": thread.IsOutdated,
				}
				
				// Add comment information
				if len(thread.Comments) > 0 {
					last := thread.Comments[len(thread.Comments)-1]
					threadData["lastCommentBy"] = last.Author
					
					// Include all comments with author information
					comments := []map[string]interface{}{}
					for _, comment := range thread.Comments {
						comments = append(comments, map[string]interface{}{
							"id":        comment.ID,
							"author":    comment.Author,
							"createdAt": comment.CreatedAt,
							"body":      comment.Body,
						})
					}
					threadData["comments"] = comments
				}
				
				unresolvedThreads = append(unresolvedThreads, threadData)
			}
		}
		
		output["reviewThreads"] = map[string]interface{}{
			"totalCount":       len(data.Threads),
			"unresolvedCount":  unresolvedCount,
			"needingReply":     unresolvedThreads, // Now simply unresolved threads
		}
	}
	
	// Output using unified encoder
	return shared.EncodeOutput(os.Stdout, format, output)
}