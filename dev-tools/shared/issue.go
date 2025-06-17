package shared

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// IssueInfo represents an issue with its associated PRs
type IssueInfo struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	PRs    []PRInfo `json:"prs"`
}

// FindPRsForIssue finds all PRs associated with an issue number
func (c *GitHubClient) FindPRsForIssue(issueNumber int) ([]PRInfo, error) {
	query := fmt.Sprintf(`
	{
	  repository(owner: "%s", name: "%s") {
	    issue(number: %d) {
	      number
	      title
	      timelineItems(itemTypes: CROSS_REFERENCED_EVENT, last: 20) {
	        nodes {
	          ... on CrossReferencedEvent {
	            source {
	              ... on PullRequest {
	                number
	                title
	                state
	              }
	            }
	          }
	        }
	      }
	    }
	  }
	}`, c.Owner, c.Repo, issueNumber)

	result, err := c.RunGraphQLQuery(query)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch issue info: %w", err)
	}

	var response struct {
		Data struct {
			Repository struct {
				Issue struct {
					Number        int    `json:"number"`
					Title         string `json:"title"`
					TimelineItems struct {
						Nodes []struct {
							Source struct {
								Number int    `json:"number"`
								Title  string `json:"title"`
								State  string `json:"state"`
							} `json:"source"`
						} `json:"nodes"`
					} `json:"timelineItems"`
				} `json:"issue"`
			} `json:"repository"`
		} `json:"data"`
	}

	if err := json.Unmarshal(result, &response); err != nil {
		return nil, fmt.Errorf("failed to parse issue response: %w", err)
	}

	var prs []PRInfo
	for _, node := range response.Data.Repository.Issue.TimelineItems.Nodes {
		if node.Source.Number > 0 {
			prs = append(prs, PRInfo{
				Number: node.Source.Number,
				Title:  node.Source.Title,
				State:  node.Source.State,
			})
		}
	}

	return prs, nil
}

// GetActivePRForIssue returns the active (open) PR for an issue, if any
func (c *GitHubClient) GetActivePRForIssue(issueNumber int) (*PRInfo, error) {
	prs, err := c.FindPRsForIssue(issueNumber)
	if err != nil {
		return nil, err
	}

	// Look for open PRs first
	for _, pr := range prs {
		if pr.State == "OPEN" {
			return &pr, nil
		}
	}

	return nil, fmt.Errorf("no active PR found for issue #%d", issueNumber)
}

// ResolveIssueOrPR resolves either an issue number to its active PR, or returns the PR number as-is
// This allows users to use either issue numbers or PR numbers interchangeably
func (c *GitHubClient) ResolveIssueOrPR(numberStr string) (int, string, error) {
	number, err := strconv.Atoi(numberStr)
	if err != nil {
		return 0, "", fmt.Errorf("invalid number format: %s", numberStr)
	}

	// First, try to find if this is an issue with associated PRs
	if prs, err := c.FindPRsForIssue(number); err == nil && len(prs) > 0 {
		// Look for open PR
		for _, pr := range prs {
			if pr.State == "OPEN" {
				return pr.Number, fmt.Sprintf("Resolved issue #%d to PR #%d: %s", number, pr.Number, pr.Title), nil
			}
		}
		// If no open PR, show available closed PRs for context
		if len(prs) > 0 {
			return 0, "", fmt.Errorf("issue #%d has no open PRs (found %d closed PR(s))", number, len(prs))
		}
	}

	// If not an issue with PRs, assume it's already a PR number
	return number, fmt.Sprintf("Using PR #%d directly", number), nil
}