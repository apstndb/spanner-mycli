package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/apstndb/spanner-mycli/dev-tools/shared"
	"github.com/goccy/go-yaml"
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
	
	// Get output format flags from persistent flags
	outputJSON, _ := cmd.Flags().GetBool("json")
	outputYAML, _ := cmd.Flags().GetBool("yaml")
	
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

	// Always log fetching info in structured format
	slog.Info("Fetching unified review data",
		"pr", prNumber,
		"threads", opts.IncludeThreads,
		"bodies", opts.IncludeReviewBodies,
		"reviewLimit", opts.ReviewLimit,
		"threadLimit", opts.ThreadLimit)

	data, err := client.GetUnifiedReviewData(prNumber, opts)
	if err != nil {
		return fmt.Errorf("failed to fetch unified data: %w", err)
	}

	// Handle specialized modes
	if listThreads {
		// Simple list mode: just thread IDs
		for _, thread := range data.Threads {
			if thread.NeedsReply && !thread.IsResolved {
				fmt.Println(thread.ID)
			}
		}
		return nil
	}
	
	if threadsOnly {
		// Filter to only threads needing replies
		threadsNeedingReply := []shared.ThreadData{}
		for _, thread := range data.Threads {
			if thread.NeedsReply && !thread.IsResolved {
				threadsNeedingReply = append(threadsNeedingReply, thread)
			}
		}
		if outputJSON {
			encoder := json.NewEncoder(os.Stdout)
			encoder.SetIndent("", "  ")
			return encoder.Encode(threadsNeedingReply)
		}
		if outputYAML {
			encoder := yaml.NewEncoder(os.Stdout)
			return encoder.Encode(threadsNeedingReply)
		}
		// Default to YAML
		encoder := yaml.NewEncoder(os.Stdout)
		return encoder.Encode(threadsNeedingReply)
	}
	
	// Full data output
	if outputJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(data)
	}
	
	if outputYAML {
		return outputFetchYAML(data, includeReviewBodies, includeThreads)
	}
	
	// Default to YAML
	return outputFetchYAML(data, includeReviewBodies, includeThreads)
}

func analyzeReviews(cmd *cobra.Command, args []string) error {
	client := shared.NewGitHubClient(owner, repo)
	prNumber, err := resolvePRNumberFromArgs(args, client)
	if err != nil {
		return err
	}
	
	jsonOutput, _ := cmd.Flags().GetBool("json")
	yamlOutput, _ := cmd.Flags().GetBool("yaml")

	// Always fetch everything for analysis
	opts := shared.DefaultUnifiedReviewOptions()

	data, err := client.GetUnifiedReviewData(prNumber, opts)
	if err != nil {
		return fmt.Errorf("failed to fetch data: %w", err)
	}

	// Get all actionable items
	items := data.GetActionableItems()

	if jsonOutput {
		return outputAnalysisJSON(data, items)
	}
	
	if yamlOutput {
		return outputAnalysisYAML(data, items)
	}

	// Default to YAML for simplicity
	return outputAnalysisYAML(data, items)
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

// outputAnalysisYAML outputs analysis results in YAML format for AI tools
func outputAnalysisYAML(data *shared.UnifiedReviewData, items []shared.ActionableItem) error {
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
	
	// Build simplified items for YAML output
	type SimpleItem struct {
		ID       string `yaml:"id"`
		Author   string `yaml:"author"`
		Location string `yaml:"location"`
		Summary  string `yaml:"summary"`
		Type     string `yaml:"type,omitempty"`
	}
	
	criticalItems := []SimpleItem{}
	highItems := []SimpleItem{}
	infoItems := []SimpleItem{}
	
	for _, item := range items {
		simpleItem := SimpleItem{
			ID:       item.ID,
			Author:   item.Author,
			Location: item.Location,
			Summary:  item.Summary,
			Type:     item.Type,
		}
		
		switch item.Severity {
		case shared.SeverityCritical:
			criticalItems = append(criticalItems, simpleItem)
		case shared.SeverityHigh:
			highItems = append(highItems, simpleItem)
		default:
			infoItems = append(infoItems, simpleItem)
		}
	}
	
	// Build threads needing reply
	type SimpleThread struct {
		ID           string `yaml:"id"`
		Location     string `yaml:"location"`
		LastReplier  string `yaml:"last_replier"`
	}
	
	threadsNeedingReply := []SimpleThread{}
	for _, thread := range data.Threads {
		if thread.NeedsReply && !thread.IsResolved {
			location := thread.Path
			if thread.Line != nil {
				location = fmt.Sprintf("%s:%d", thread.Path, *thread.Line)
			}
			threadsNeedingReply = append(threadsNeedingReply, SimpleThread{
				ID:          thread.ID,
				Location:    location,
				LastReplier: thread.LastReplier,
			})
		}
	}
	
	// Create structured output for YAML
	output := map[string]interface{}{
		"pr_number": data.PR.Number,
		"title":     data.PR.Title,
		"state":     data.PR.State,
		"mergeable": data.PR.Mergeable == "MERGEABLE",
		"action_required": len(criticalItems) > 0 || len(highItems) > 0 || threadsNeedReply > 0,
		"summary": map[string]int{
			"critical": severityCounts["CRITICAL"],
			"high":     severityCounts["HIGH"],
			"info":     severityCounts["INFO"],
			"threads_need_reply":   threadsNeedReply,
			"threads_unresolved":   threadsUnresolved,
		},
	}
	
	// Only include non-empty sections
	if len(criticalItems) > 0 {
		output["critical_items"] = criticalItems
	}
	if len(highItems) > 0 {
		output["high_priority_items"] = highItems
	}
	if len(infoItems) > 0 {
		output["info_items"] = infoItems
	}
	if len(threadsNeedingReply) > 0 {
		output["threads_needing_reply"] = threadsNeedingReply
	}
	
	// Output YAML
	encoder := yaml.NewEncoder(os.Stdout)
	return encoder.Encode(output)
}

// outputFetchYAML outputs fetch results in YAML format preserving all information
func outputFetchYAML(data *shared.UnifiedReviewData, includeReviewBodies bool, includeThreads bool) error {
	// Build output structure
	output := map[string]interface{}{
		"pr": map[string]interface{}{
			"number":       data.PR.Number,
			"title":        data.PR.Title,
			"state":        data.PR.State,
			"mergeable":    data.PR.Mergeable,
			"merge_status": data.PR.MergeStatus,
		},
		"current_user": data.CurrentUser,
		"fetched_at":   data.FetchedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	
	// Reviews section
	if includeReviewBodies {
		// Full review data with bodies
		reviews := []map[string]interface{}{}
		for _, review := range data.Reviews {
			reviewData := map[string]interface{}{
				"id":           review.ID,
				"author":       review.Author,
				"state":        review.State,
				"created_at":   review.CreatedAt,
				"severity":     string(review.Severity),
			}
			
			if review.Body != "" {
				reviewData["body"] = review.Body
			}
			
			if len(review.ActionItems) > 0 {
				reviewData["action_items"] = review.ActionItems
			}
			
			if len(review.Comments) > 0 {
				reviewData["inline_comments_count"] = len(review.Comments)
			}
			
			reviews = append(reviews, reviewData)
		}
		output["reviews"] = reviews
	} else {
		// Minimal review data without bodies
		reviews := []map[string]interface{}{}
		for _, review := range data.Reviews {
			reviews = append(reviews, map[string]interface{}{
				"id":         review.ID,
				"author":     review.Author,
				"state":      review.State,
				"created_at": review.CreatedAt,
			})
		}
		output["reviews"] = reviews
		output["reviews_bodies_fetched"] = false
	}
	
	// Threads section
	if includeThreads {
		unresolvedCount := 0
		needsReplyCount := 0
		threadsNeedingReply := []map[string]interface{}{}
		
		for _, thread := range data.Threads {
			if !thread.IsResolved {
				unresolvedCount++
			}
			if thread.NeedsReply {
				needsReplyCount++
			}
			
			if thread.NeedsReply && !thread.IsResolved {
				location := thread.Path
				if thread.Line != nil {
					location = fmt.Sprintf("%s:%d", thread.Path, *thread.Line)
				}
				
				threadData := map[string]interface{}{
					"id":       thread.ID,
					"location": location,
				}
				
				if len(thread.Comments) > 0 {
					last := thread.Comments[len(thread.Comments)-1]
					threadData["last_comment_by"] = last.Author
				}
				
				threadsNeedingReply = append(threadsNeedingReply, threadData)
			}
		}
		
		output["threads"] = map[string]interface{}{
			"total":         len(data.Threads),
			"unresolved":    unresolvedCount,
			"needs_reply":   needsReplyCount,
			"needing_reply": threadsNeedingReply,
		}
	}
	
	// Output YAML
	encoder := yaml.NewEncoder(os.Stdout)
	return encoder.Encode(output)
}