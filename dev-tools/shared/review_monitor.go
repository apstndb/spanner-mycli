package shared

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// ReviewMonitor provides comprehensive review tracking to prevent missing important feedback
//
// LESSON LEARNED FROM PR #306:
// We missed a critical review about statusCheckRollup nil handling because:
// 1. Review comments were embedded in a general review body (not inline comments)
// 2. "Needs Reply" detection only checked thread-based comments
// 3. No systematic tracking of review body content for actionable items
//
// This monitor addresses these issues by:
// - Extracting and highlighting severity indicators (critical, high, medium, low)
// - Detecting technical keywords that require attention
// - Tracking review bodies separately from thread comments
// - Providing structured output for AI assistants to process
type ReviewMonitor struct {
	client *GitHubClient
}

// ReviewSeverity represents the priority level of review feedback
type ReviewSeverity string

const (
	SeverityCritical ReviewSeverity = "CRITICAL"
	SeverityHigh     ReviewSeverity = "HIGH"
	SeverityInfo     ReviewSeverity = "INFO"
)

// ReviewItem represents an actionable item from a review
type ReviewItem struct {
	ReviewID    string          `json:"reviewId"`
	Author      string          `json:"author"`
	CreatedAt   string          `json:"createdAt"`
	Severity    ReviewSeverity  `json:"severity"`
	BodyPreview string          `json:"bodyPreview"`
}

// NewReviewMonitor creates a new review monitor
func NewReviewMonitor(client *GitHubClient) *ReviewMonitor {
	return &ReviewMonitor{client: client}
}

// AnalyzeReviews fetches and analyzes all reviews for actionable items
func (m *ReviewMonitor) AnalyzeReviews(prNumber string) ([]ReviewItem, error) {
	prNumberInt, err := strconv.Atoi(prNumber)
	if err != nil {
		return nil, fmt.Errorf("invalid PR number format: %w", err)
	}

	// Fetch reviews with full body content
	query := `
query($owner: String!, $repo: String!, $prNumber: Int!) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $prNumber) {
      reviews(last: 20) {
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
}`

	variables := map[string]interface{}{
		"owner":    m.client.Owner,
		"repo":     m.client.Repo,
		"prNumber": prNumberInt,
	}

	result, err := m.client.RunGraphQLQueryWithVariables(query, variables)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch reviews: %w", err)
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
		return nil, fmt.Errorf("failed to parse reviews: %w", err)
	}

	var items []ReviewItem
	for _, review := range response.Data.Repository.PullRequest.Reviews.Nodes {
		if review.Body == "" || review.State == "PENDING" {
			continue
		}

		// Analyze review body for actionable items
		severity := m.detectSeverity(review.Body)
		
		items = append(items, ReviewItem{
			ReviewID:    review.ID,
			Author:      review.Author.Login,
			CreatedAt:   review.CreatedAt,
			Severity:    severity,
			BodyPreview: m.truncateBody(review.Body, 200),
		})
	}

	return items, nil
}

// detectSeverity analyzes review text for Gemini severity markers
func (m *ReviewMonitor) detectSeverity(body string) ReviewSeverity {
	// Only recognize Gemini's explicit severity markers
	if strings.Contains(body, "![critical]") {
		return SeverityCritical
	}
	if strings.Contains(body, "![high]") {
		return SeverityHigh
	}
	return SeverityInfo
}



// truncateBody creates a preview of the review body
func (m *ReviewMonitor) truncateBody(body string, maxLen int) string {
	// Remove excessive whitespace
	body = strings.TrimSpace(body)
	body = strings.ReplaceAll(body, "\n\n\n", "\n\n")
	
	if len(body) <= maxLen {
		return body
	}
	
	// Try to break at word boundary
	truncated := body[:maxLen]
	lastSpace := strings.LastIndex(truncated, " ")
	if lastSpace > maxLen*3/4 {
		truncated = truncated[:lastSpace]
	}
	
	return truncated + "..."
}


// FormatReviewItems creates a formatted output of review items
func FormatReviewItems(items []ReviewItem) string {
	if len(items) == 0 {
		return "No actionable review items found"
	}
	
	var output strings.Builder
	output.WriteString(fmt.Sprintf("📋 Found %d actionable review item(s):\n\n", len(items)))
	
	// Group by severity
	bySeverity := make(map[ReviewSeverity][]ReviewItem)
	for _, item := range items {
		bySeverity[item.Severity] = append(bySeverity[item.Severity], item)
	}
	
	// Output in priority order (only 3 levels now)
	for _, severity := range []ReviewSeverity{SeverityCritical, SeverityHigh, SeverityInfo} {
		items, ok := bySeverity[severity]
		if !ok || len(items) == 0 {
			continue
		}
		
		severityIcon := "ℹ️"
		switch severity {
		case SeverityCritical:
			severityIcon = "🚨"
		case SeverityHigh:
			severityIcon = "⚠️"
		}
		
		output.WriteString(fmt.Sprintf("%s %s Priority:\n", severityIcon, severity))
		for _, item := range items {
			output.WriteString(fmt.Sprintf("  • %s by %s at %s\n", item.ReviewID, item.Author, item.CreatedAt))
			if item.BodyPreview != "" {
				// Indent body preview for readability
				previewLines := strings.Split(item.BodyPreview, "\n")
				for i, line := range previewLines {
					if i == 0 {
						output.WriteString("    Preview: ")
					} else {
						output.WriteString("             ")
					}
					output.WriteString(line + "\n")
				}
			}
			output.WriteString("\n")
		}
	}
	
	return output.String()
}