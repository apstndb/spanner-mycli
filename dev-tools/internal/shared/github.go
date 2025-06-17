package shared

import (
	"encoding/json"
	"os/exec"
	"fmt"
)

// GitHubClient provides common GitHub operations
type GitHubClient struct {
	Owner string
	Repo  string
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