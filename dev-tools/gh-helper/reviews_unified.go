package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"

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
	
	// Get JSON flag from persistent flags
	outputJSON, _ := cmd.Flags().GetBool("json")
	
	// Adjust flags for thread-focused modes
	if listThreads || threadsOnly {
		outputJSON = true
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

	if outputJSON {
		// JSON mode: structured logging, data to stdout
		slog.Info("Fetching unified review data",
			"pr", prNumber,
			"threads", opts.IncludeThreads,
			"bodies", opts.IncludeReviewBodies,
			"reviewLimit", opts.ReviewLimit,
			"threadLimit", opts.ThreadLimit)
	} else {
		// Human mode: friendly output to stdout
		fmt.Printf("🔄 Fetching unified review data for PR #%s...\n", prNumber)
		fmt.Printf("   Options: threads=%v, bodies=%v, limits=[reviews:%d, threads:%d]\n",
			opts.IncludeThreads, opts.IncludeReviewBodies, opts.ReviewLimit, opts.ThreadLimit)
	}

	data, err := client.GetUnifiedReviewData(prNumber, opts)
	if err != nil {
		return fmt.Errorf("failed to fetch unified data: %w", err)
	}

	if outputJSON {
		if listThreads {
			// Output only thread IDs that need replies, one per line
			for _, thread := range data.Threads {
				if thread.NeedsReply && !thread.IsResolved {
					fmt.Println(thread.ID)
				}
			}
			return nil
		} else if threadsOnly {
			// Output only threads that need replies
			threadsNeedingReply := []shared.ThreadData{}
			for _, thread := range data.Threads {
				if thread.NeedsReply && !thread.IsResolved {
					threadsNeedingReply = append(threadsNeedingReply, thread)
				}
			}
			encoder := json.NewEncoder(os.Stdout)
			encoder.SetIndent("", "  ")
			return encoder.Encode(threadsNeedingReply)
		} else {
			// Output raw JSON for processing
			encoder := json.NewEncoder(os.Stdout)
			encoder.SetIndent("", "  ")
			return encoder.Encode(data)
		}
	}

	// Human-readable output
	fmt.Printf("\n📋 PR #%d: %s (%s)\n", data.PR.Number, data.PR.Title, data.PR.State)
	fmt.Printf("   Mergeable: %s, Status: %s\n", data.PR.Mergeable, data.PR.MergeStatus)
	fmt.Printf("   Current User: %s\n", data.CurrentUser)

	if includeReviewBodies {
		fmt.Printf("\n📝 Reviews (%d):\n", len(data.Reviews))
		for _, review := range data.Reviews {
			severityIcon := "ℹ️"
			switch review.Severity {
			case shared.SeverityCritical:
				severityIcon = "🚨"
			case shared.SeverityHigh:
				severityIcon = "⚠️"
			case shared.SeverityMedium:
				severityIcon = "⚡"
			}

			fmt.Printf("\n   %s Review %s by %s (%s)\n", 
				severityIcon, review.ID, review.Author, review.State)
			
			if review.Body != "" {
				preview := strings.TrimSpace(review.Body)
				if len(preview) > 150 {
					preview = preview[:150] + "..."
				}
				fmt.Printf("   Body: %s\n", preview)
			}

			if len(review.ActionItems) > 0 {
				fmt.Printf("   Action Items:\n")
				for _, item := range review.ActionItems {
					fmt.Printf("     • %s\n", item)
				}
			}

			if len(review.Comments) > 0 {
				fmt.Printf("   Inline Comments: %d\n", len(review.Comments))
			}
		}
	} else {
		fmt.Printf("\n📝 Reviews (%d) - bodies not fetched\n", len(data.Reviews))
		for _, review := range data.Reviews {
			fmt.Printf("   • %s by %s (%s) at %s\n", 
				review.ID, review.Author, review.State, review.CreatedAt)
		}
	}

	if includeThreads {
		unresolvedCount := 0
		needsReplyCount := 0
		for _, thread := range data.Threads {
			if !thread.IsResolved {
				unresolvedCount++
			}
			if thread.NeedsReply {
				needsReplyCount++
			}
		}

		fmt.Printf("\n💬 Threads (%d total, %d unresolved, %d need reply):\n", 
			len(data.Threads), unresolvedCount, needsReplyCount)

		// Show threads needing replies
		for _, thread := range data.Threads {
			if thread.NeedsReply && !thread.IsResolved {
				location := thread.Path
				if thread.Line != nil {
					location = fmt.Sprintf("%s:%d", thread.Path, *thread.Line)
				}
				
				fmt.Printf("\n   🔴 Thread %s at %s\n", thread.ID, location)
				if len(thread.Comments) > 0 {
					last := thread.Comments[len(thread.Comments)-1]
					fmt.Printf("   Last comment by %s\n", last.Author)
				}
			}
		}
	}

	return nil
}

func analyzeReviews(cmd *cobra.Command, args []string) error {
	client := shared.NewGitHubClient(owner, repo)
	prNumber, err := resolvePRNumberFromArgs(args, client)
	if err != nil {
		return err
	}
	
	jsonOutput, _ := cmd.Flags().GetBool("json")

	// Always fetch everything for analysis
	opts := shared.DefaultUnifiedReviewOptions()

	if jsonOutput {
		// JSON mode: structured logging
		slog.Info("Analyzing review feedback", "pr", prNumber)
	} else {
		// Human mode: friendly output
		fmt.Printf("🔍 Analyzing all review feedback for PR #%s...\n", prNumber)
	}

	data, err := client.GetUnifiedReviewData(prNumber, opts)
	if err != nil {
		return fmt.Errorf("failed to fetch data: %w", err)
	}

	// Get all actionable items
	items := data.GetActionableItems()

	if jsonOutput {
		return outputAnalysisJSON(data, items)
	}

	if len(items) == 0 {
		fmt.Println("\n✅ No actionable items found!")
		return nil
	}

	// Sort by severity
	sort.Slice(items, func(i, j int) bool {
		severityOrder := map[shared.ReviewSeverity]int{
			shared.SeverityCritical: 0,
			shared.SeverityHigh:     1,
			shared.SeverityMedium:   2,
			shared.SeverityLow:      3,
			shared.SeverityInfo:     4,
		}
		return severityOrder[items[i].Severity] < severityOrder[items[j].Severity]
	})

	fmt.Printf("\n🎯 Found %d actionable item(s):\n", len(items))

	// Group by type
	byType := make(map[string][]shared.ActionableItem)
	for _, item := range items {
		byType[item.Type] = append(byType[item.Type], item)
	}

	// Show review issues first (these are often missed!)
	if reviews, ok := byType["review"]; ok && len(reviews) > 0 {
		fmt.Printf("\n📝 REVIEW BODY ISSUES (Often Missed!):\n")
		for _, item := range reviews {
			severityIcon := getSeverityIcon(item.Severity)
			fmt.Printf("\n%s %s - %s\n", severityIcon, item.Severity, item.ID)
			fmt.Printf("   Author: %s\n", item.Author)
			fmt.Printf("   Summary: %s\n", item.Summary)
			if len(item.ActionItems) > 0 {
				fmt.Printf("   Actions Required:\n")
				for _, action := range item.ActionItems {
					fmt.Printf("     • %s\n", action)
				}
			}
		}
	}

	// Show review comment issues
	if comments, ok := byType["review_comment"]; ok && len(comments) > 0 {
		fmt.Printf("\n💭 REVIEW COMMENT ISSUES:\n")
		for _, item := range comments {
			fmt.Printf("\n• %s at %s\n", item.ID, item.Location)
			fmt.Printf("  %s\n", item.Summary)
		}
	}

	// Show threads needing replies
	if threads, ok := byType["thread"]; ok && len(threads) > 0 {
		fmt.Printf("\n💬 THREADS NEEDING REPLIES:\n")
		for _, item := range threads {
			fmt.Printf("\n• %s at %s\n", item.ID, item.Location)
			fmt.Printf("  %s\n", item.Summary)
		}
	}

	// Summary
	criticalCount := 0
	highCount := 0
	for _, item := range items {
		switch item.Severity {
		case shared.SeverityCritical:
			criticalCount++
		case shared.SeverityHigh:
			highCount++
		}
	}

	if criticalCount > 0 || highCount > 0 {
		fmt.Printf("\n⚠️  ATTENTION REQUIRED: %d critical, %d high priority issues\n", 
			criticalCount, highCount)
	}

	fmt.Println("\n💡 This unified view helps prevent missing important feedback that")
	fmt.Println("   appears in review bodies but not in thread comments.")

	return nil
}

func getSeverityIcon(severity shared.ReviewSeverity) string {
	switch severity {
	case shared.SeverityCritical:
		return "🚨"
	case shared.SeverityHigh:
		return "⚠️"
	case shared.SeverityMedium:
		return "⚡"
	case shared.SeverityLow:
		return "💡"
	default:
		return "ℹ️"
	}
}

// outputAnalysisJSON outputs analysis results in JSON format for programmatic use
func outputAnalysisJSON(data *shared.UnifiedReviewData, items []shared.ActionableItem) error {
	// Create structured output for easy parsing
	output := struct {
		PR          shared.PRMetadata           `json:"pr"`
		CurrentUser string                      `json:"currentUser"`
		FetchedAt   string                      `json:"fetchedAt"`
		Summary     map[string]int              `json:"summary"`
		Items       []shared.ActionableItem     `json:"actionableItems"`
		Threads     []shared.ThreadData         `json:"threads"`
		Reviews     []shared.ReviewData         `json:"reviews"`
	}{
		PR:          data.PR,
		CurrentUser: data.CurrentUser,
		FetchedAt:   data.FetchedAt.Format("2006-01-02T15:04:05Z07:00"),
		Items:       items,
		Threads:     data.Threads,
		Reviews:     data.Reviews,
	}

	// Calculate summary
	severityCounts := make(map[string]int)
	for _, item := range items {
		severityCounts[string(item.Severity)]++
	}
	
	threadCounts := make(map[string]int)
	for _, thread := range data.Threads {
		if thread.NeedsReply && !thread.IsResolved {
			threadCounts["needsReply"]++
		}
		if !thread.IsResolved {
			threadCounts["unresolved"]++
		}
		threadCounts["total"]++
	}

	output.Summary = map[string]int{
		"actionableItems": len(items),
		"critical":        severityCounts["critical"],
		"high":           severityCounts["high"], 
		"medium":         severityCounts["medium"],
		"low":            severityCounts["low"],
		"threadsTotal":   threadCounts["total"],
		"threadsNeedReply": threadCounts["needsReply"],
		"threadsUnresolved": threadCounts["unresolved"],
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}