package shared

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
)

// GitHubClient provides common GitHub operations
type GitHubClient struct {
	Owner string
	Repo  string
}

// PRInfo represents basic PR information  
type PRInfo struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
}

// NewGitHubClient creates a new GitHub client with default or custom owner/repo
func NewGitHubClient(owner, repo string) *GitHubClient {
	if owner == "" {
		owner = DefaultOwner
	}
	if repo == "" {
		repo = DefaultRepo
	}
	return &GitHubClient{
		Owner: owner,
		Repo:  repo,
	}
}

// RunGraphQLQuery executes a GraphQL query using gh CLI
func (c *GitHubClient) RunGraphQLQuery(query string) ([]byte, error) {
	cmd := exec.Command("gh", "api", "graphql", "-F", "query="+query)
	return cmd.Output()
}

// CreatePRComment creates a comment on a pull request
func (c *GitHubClient) CreatePRComment(prNumber, body string) error {
	return RunCommand("gh", "pr", "comment", prNumber, "--body", body)
}

// GetCurrentUser returns the current authenticated GitHub username
func (c *GitHubClient) GetCurrentUser() (string, error) {
	query := `
	{
	  viewer {
	    login
	  }
	}`
	
	result, err := c.RunGraphQLQuery(query)
	if err != nil {
		return "", fmt.Errorf("failed to get current user: %w", err)
	}
	
	var response struct {
		Data struct {
			Viewer struct {
				Login string `json:"login"`
			} `json:"viewer"`
		} `json:"data"`
	}
	
	if err := json.Unmarshal(result, &response); err != nil {
		return "", fmt.Errorf("failed to parse user response: %w", err)
	}
	
	return response.Data.Viewer.Login, nil
}

// GetCurrentBranchPR gets the PR associated with the current branch using gh CLI
func (c *GitHubClient) GetCurrentBranchPR() (*PRInfo, error) {
	cmd := exec.Command("gh", "pr", "view", "--json", "number,title,state")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("no PR found for current branch or gh command failed: %w", err)
	}

	var pr PRInfo
	if err := json.Unmarshal(output, &pr); err != nil {
		return nil, fmt.Errorf("failed to parse PR info: %w", err)
	}

	return &pr, nil
}

// NodeInfo represents a GitHub issue or PR
type NodeInfo struct {
	Number   int    `json:"number"`
	Title    string `json:"title"`
	State    string `json:"state"`
	NodeType string // "Issue" or "PullRequest"
}

// ResolveNumber determines if a number is an issue or PR and resolves accordingly
func (c *GitHubClient) ResolveNumber(number int) (*NodeInfo, error) {
	query := fmt.Sprintf(`
	{
	  repository(owner: "%s", name: "%s") {
	    issueOrPullRequest(number: %d) {
	      __typename
	      ... on Issue {
	        number
	        title
	        state
	      }
	      ... on PullRequest {
	        number
	        title
	        state
	      }
	    }
	  }
	}`, c.Owner, c.Repo, number)

	result, err := c.RunGraphQLQuery(query)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve number: %w", err)
	}

	var response struct {
		Data struct {
			Repository struct {
				IssueOrPullRequest struct {
					TypeName string `json:"__typename"`
					Number   int    `json:"number"`
					Title    string `json:"title"`
					State    string `json:"state"`
				} `json:"issueOrPullRequest"`
			} `json:"repository"`
		} `json:"data"`
	}

	if err := json.Unmarshal(result, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	node := response.Data.Repository.IssueOrPullRequest
	if node.Number == 0 {
		return nil, fmt.Errorf("number %d not found", number)
	}

	return &NodeInfo{
		Number:   node.Number,
		Title:    node.Title,
		State:    node.State,
		NodeType: node.TypeName,
	}, nil
}

// ResolvePRNumber resolves PR number using multiple strategies:
// 1. If empty input, use current branch's PR (via gh pr view)
// 2. If number provided, check if it's issue or PR and resolve accordingly
func (c *GitHubClient) ResolvePRNumber(input string) (int, string, error) {
	// Strategy 1: If no input, use current branch PR
	if input == "" {
		pr, err := c.GetCurrentBranchPR()
		if err != nil {
			return 0, "", fmt.Errorf("no PR number provided and current branch has no associated PR: %w", err)
		}
		return pr.Number, fmt.Sprintf("Using current branch PR #%d: %s", pr.Number, pr.Title), nil
	}

	// Strategy 2: Parse number and determine if it's issue or PR
	number, err := strconv.Atoi(input)
	if err != nil {
		return 0, "", fmt.Errorf("invalid number format: %s", input)
	}

	node, err := c.ResolveNumber(number)
	if err != nil {
		return 0, "", err
	}

	switch node.NodeType {
	case "PullRequest":
		return node.Number, fmt.Sprintf("Using PR #%d: %s", node.Number, node.Title), nil
	case "Issue":
		// Find associated open PR for this issue
		prs, err := c.FindPRsForIssue(node.Number)
		if err != nil {
			return 0, "", fmt.Errorf("failed to find PRs for issue #%d: %w", node.Number, err)
		}
		
		// Look for open PR
		for _, pr := range prs {
			if pr.State == "OPEN" {
				return pr.Number, fmt.Sprintf("Resolved issue #%d to open PR #%d: %s", node.Number, pr.Number, pr.Title), nil
			}
		}
		
		if len(prs) > 0 {
			return 0, "", fmt.Errorf("issue #%d has %d associated PR(s) but none are open", node.Number, len(prs))
		}
		return 0, "", fmt.Errorf("issue #%d has no associated PRs", node.Number)
	default:
		return 0, "", fmt.Errorf("unknown node type: %s", node.NodeType)
	}
}