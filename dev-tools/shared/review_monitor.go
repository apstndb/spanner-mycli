package shared

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
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
	SeverityMedium   ReviewSeverity = "MEDIUM"
	SeverityLow      ReviewSeverity = "LOW"
	SeverityInfo     ReviewSeverity = "INFO"
)

// ReviewItem represents an actionable item from a review
type ReviewItem struct {
	ReviewID    string          `json:"reviewId"`
	Author      string          `json:"author"`
	CreatedAt   string          `json:"createdAt"`
	Severity    ReviewSeverity  `json:"severity"`
	Keywords    []string        `json:"keywords"`
	Summary     string          `json:"summary"`
	BodyPreview string          `json:"bodyPreview"`
	RequiresResponse bool       `json:"requiresResponse"`
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
		keywords := m.extractKeywords(review.Body)
		
		// Skip if no actionable content
		if severity == SeverityInfo && len(keywords) == 0 {
			continue
		}

		// Generate summary
		summary := m.generateSummary(review.Body, keywords)
		
		// Determine if response is required
		requiresResponse := severity == SeverityCritical || severity == SeverityHigh ||
			containsAny(keywords, []string{"bug", "error", "issue", "problem", "incorrect", "wrong"})

		items = append(items, ReviewItem{
			ReviewID:    review.ID,
			Author:      review.Author.Login,
			CreatedAt:   review.CreatedAt,
			Severity:    severity,
			Keywords:    keywords,
			Summary:     summary,
			BodyPreview: m.truncateBody(review.Body, 200),
			RequiresResponse: requiresResponse,
		})
	}

	return items, nil
}

// detectSeverity analyzes review text for severity indicators
func (m *ReviewMonitor) detectSeverity(body string) ReviewSeverity {
	bodyLower := strings.ToLower(body)
	
	// Check for explicit severity markers
	if strings.Contains(bodyLower, "critical") || strings.Contains(bodyLower, "![critical]") {
		return SeverityCritical
	}
	if strings.Contains(bodyLower, "high-severity") || strings.Contains(bodyLower, "![high]") ||
		strings.Contains(bodyLower, "high-priority") {
		return SeverityHigh
	}
	if strings.Contains(bodyLower, "medium-severity") || strings.Contains(bodyLower, "![medium]") {
		return SeverityMedium
	}
	if strings.Contains(bodyLower, "low-severity") || strings.Contains(bodyLower, "![low]") {
		return SeverityLow
	}
	
	// Check for critical keywords
	criticalKeywords := []string{"panic", "crash", "security", "vulnerability", "leak", "injection"}
	for _, keyword := range criticalKeywords {
		if strings.Contains(bodyLower, keyword) {
			return SeverityCritical
		}
	}
	
	// Check for high priority keywords
	highKeywords := []string{"bug", "error", "incorrect", "wrong", "broken", "fail"}
	for _, keyword := range highKeywords {
		if strings.Contains(bodyLower, keyword) {
			return SeverityHigh
		}
	}
	
	return SeverityInfo
}

// extractKeywords finds technical keywords in review text
func (m *ReviewMonitor) extractKeywords(body string) []string {
	bodyLower := strings.ToLower(body)
	keywords := []string{}
	
	// Technical keywords to look for
	technicalKeywords := []string{
		// Error-related
		"panic", "error", "bug", "issue", "problem", "incorrect", "wrong", "broken",
		// Security-related
		"security", "vulnerability", "injection", "leak", "hardcoded", "credential",
		// Code quality
		"nil", "null", "undefined", "placeholder", "stub", "todo", "fixme",
		// Specific to our case
		"statuscheckrollup", "graphql", "api", "timeout", "race condition",
	}
	
	found := make(map[string]bool)
	for _, keyword := range technicalKeywords {
		if strings.Contains(bodyLower, keyword) && !found[keyword] {
			keywords = append(keywords, keyword)
			found[keyword] = true
		}
	}
	
	return keywords
}

// generateSummary creates a brief summary of the main issue
func (m *ReviewMonitor) generateSummary(body string, keywords []string) string {
	// Find sentences containing keywords
	sentences := strings.Split(body, ".")
	for _, sentence := range sentences {
		sentenceLower := strings.ToLower(sentence)
		for _, keyword := range keywords {
			if strings.Contains(sentenceLower, keyword) {
				summary := strings.TrimSpace(sentence)
				if len(summary) > 100 {
					summary = summary[:100] + "..."
				}
				return summary
			}
		}
	}
	
	// Fallback to first meaningful sentence
	for _, sentence := range sentences {
		trimmed := strings.TrimSpace(sentence)
		if len(trimmed) > 20 {
			if len(trimmed) > 100 {
				return trimmed[:100] + "..."
			}
			return trimmed
		}
	}
	
	return "Review requires attention"
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

// containsAny checks if any of the target strings are in the list
func containsAny(list []string, targets []string) bool {
	for _, item := range list {
		for _, target := range targets {
			if item == target {
				return true
			}
		}
	}
	return false
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
	
	// Output in priority order
	for _, severity := range []ReviewSeverity{SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow, SeverityInfo} {
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
		case SeverityMedium:
			severityIcon = "⚡"
		case SeverityLow:
			severityIcon = "💡"
		}
		
		output.WriteString(fmt.Sprintf("%s %s Priority:\n", severityIcon, severity))
		for _, item := range items {
			output.WriteString(fmt.Sprintf("  • %s by %s\n", item.ReviewID, item.Author))
			output.WriteString(fmt.Sprintf("    Summary: %s\n", item.Summary))
			if len(item.Keywords) > 0 {
				output.WriteString(fmt.Sprintf("    Keywords: %s\n", strings.Join(item.Keywords, ", ")))
			}
			if item.RequiresResponse {
				output.WriteString("    ⚠️  Requires response\n")
			}
			output.WriteString("\n")
		}
	}
	
	return output.String()
}