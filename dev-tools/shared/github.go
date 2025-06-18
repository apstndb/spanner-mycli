package shared

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// GitHubClient provides common GitHub operations with token caching
type GitHubClient struct {
	Owner      string
	Repo       string
	httpClient *http.Client
}

// Token cache to avoid repeated gh auth token calls
var (
	cachedToken     string
	tokenCacheTime  time.Time
	tokenCacheMutex sync.RWMutex
	tokenCacheTTL   = 10 * time.Minute // Cache token for 10 minutes
)

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
		Owner:      owner,
		Repo:       repo,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// getToken retrieves and caches GitHub token from gh CLI
func getToken() (string, error) {
	tokenCacheMutex.RLock()
	if cachedToken != "" && time.Since(tokenCacheTime) < tokenCacheTTL {
		defer tokenCacheMutex.RUnlock()
		return cachedToken, nil
	}
	tokenCacheMutex.RUnlock()

	// Need to refresh token
	tokenCacheMutex.Lock()
	defer tokenCacheMutex.Unlock()

	// Double-check after acquiring write lock
	if cachedToken != "" && time.Since(tokenCacheTime) < tokenCacheTTL {
		return cachedToken, nil
	}

	// Get token from gh CLI
	cmd := exec.Command("gh", "auth", "token")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get GitHub token: %w\n💡 Tip: Run 'gh auth login' to authenticate", err)
	}

	token := strings.TrimSpace(string(output))
	if token == "" {
		return "", fmt.Errorf("empty token returned from gh auth token")
	}

	// Cache the token
	cachedToken = token
	tokenCacheTime = time.Now()

	return token, nil
}

// GraphQLRequest represents a GraphQL request payload
type GraphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

// GraphQLResponse represents a GraphQL response
type GraphQLResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// RunGraphQLQuery executes a GraphQL query using HTTP client (legacy compatibility)
func (c *GitHubClient) RunGraphQLQuery(query string) ([]byte, error) {
	return c.RunGraphQLQueryWithVariables(query, nil)
}

// RunGraphQLQueryWithVariables executes a GraphQL query with variables using HTTP client
// Performance benefits vs gh command:
// - Token caching (10min TTL) eliminates repeated 'gh auth token' calls
// - Direct HTTP requests reduce process overhead from spawning gh CLI
// - Single HTTP client with connection reuse for multiple requests
func (c *GitHubClient) RunGraphQLQueryWithVariables(query string, variables map[string]interface{}) ([]byte, error) {
	token, err := getToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get GitHub token: %w", err)
	}

	// Prepare GraphQL request
	reqPayload := GraphQLRequest{
		Query:     query,
		Variables: variables,
	}

	jsonData, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal GraphQL request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequest("POST", "https://api.github.com/graphql", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "spanner-mycli-dev-tools/1.0")

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute GraphQL request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	var buf bytes.Buffer
	_, err = buf.ReadFrom(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GraphQL request failed with status %d: %s", resp.StatusCode, buf.String())
	}

	// Parse GraphQL response for errors
	var graphqlResp GraphQLResponse
	if err := json.Unmarshal(buf.Bytes(), &graphqlResp); err == nil {
		if len(graphqlResp.Errors) > 0 {
			return nil, fmt.Errorf("GraphQL error: %s", graphqlResp.Errors[0].Message)
		}
	}

	return buf.Bytes(), nil
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
	query := `
	query($owner: String!, $repo: String!, $number: Int!) {
	  repository(owner: $owner, name: $repo) {
	    issueOrPullRequest(number: $number) {
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
	}`

	variables := map[string]interface{}{
		"owner":  c.Owner,
		"repo":   c.Repo,
		"number": number,
	}

	result, err := c.RunGraphQLQueryWithVariables(query, variables)
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

// ParseInputFormat parses various input formats for issues/PRs
type InputFormat struct {
	Type   string // "issue", "pr", or "auto" 
	Number int
}

// ParseInput parses input in various formats:
// - "123" -> auto-detect
// - "issues/123" or "issue/123" -> force issue
// - "pull/123" or "pr/123" -> force PR
// - "" -> current branch PR
func ParseInput(input string) (*InputFormat, error) {
	if input == "" {
		return &InputFormat{Type: "current"}, nil
	}

	// Check for explicit path formats using Cut
	prefix, numberStr, found := strings.Cut(input, "/")
	if found {
		switch prefix {
		case "issues", "issue":
			number, err := strconv.Atoi(numberStr)
			if err != nil {
				return nil, fmt.Errorf("invalid issue number in '%s': %w", input, err)
			}
			return &InputFormat{Type: "issue", Number: number}, nil
			
		case "pull", "pr":
			number, err := strconv.Atoi(numberStr)
			if err != nil {
				return nil, fmt.Errorf("invalid PR number in '%s': %w", input, err)
			}
			return &InputFormat{Type: "pr", Number: number}, nil
			
		default:
			// Has "/" but unknown prefix - treat as invalid
			return nil, fmt.Errorf("unknown format '%s': use 'issues/N', 'pull/N', 'pr/N', or plain number", input)
		}
	}

	// Plain number - auto-detect
	number, err := strconv.Atoi(input)
	if err != nil {
		return nil, fmt.Errorf("invalid number format: %s", input)
	}
	return &InputFormat{Type: "auto", Number: number}, nil
}

// ResolvePRNumber resolves PR number using multiple strategies:
// 1. If empty input, use current branch's PR (via gh pr view)
// 2. If explicit format (issues/N, pull/N), skip auto-detection
// 3. If plain number, auto-detect issue vs PR
func (c *GitHubClient) ResolvePRNumber(input string) (int, string, error) {
	format, err := ParseInput(input)
	if err != nil {
		return 0, "", err
	}

	switch format.Type {
	case "current":
		// Strategy 1: Use current branch PR
		pr, err := c.GetCurrentBranchPR()
		if err != nil {
			return 0, "", fmt.Errorf("no PR number provided and current branch has no associated PR: %w", err)
		}
		return pr.Number, fmt.Sprintf("Using current branch PR #%d: %s", pr.Number, pr.Title), nil

	case "pr":
		// Strategy 2a: Explicit PR format - skip auto-detection
		return format.Number, fmt.Sprintf("Using explicit PR #%d", format.Number), nil

	case "issue":
		// Strategy 2b: Explicit issue format - directly find associated PRs
		prs, err := c.FindPRsForIssue(format.Number)
		if err != nil {
			return 0, "", fmt.Errorf("failed to find PRs for explicit issue #%d: %w", format.Number, err)
		}
		
		// Look for open PR
		for _, pr := range prs {
			if pr.State == "OPEN" {
				return pr.Number, fmt.Sprintf("Resolved explicit issue #%d to open PR #%d: %s", format.Number, pr.Number, pr.Title), nil
			}
		}
		
		if len(prs) > 0 {
			return 0, "", fmt.Errorf("explicit issue #%d has %d associated PR(s) but none are open", format.Number, len(prs))
		}
		return 0, "", fmt.Errorf("explicit issue #%d has no associated PRs", format.Number)

	case "auto":
		// Strategy 3: Auto-detect for plain numbers
		node, err := c.ResolveNumber(format.Number)
		if err != nil {
			return 0, "", err
		}

		switch node.NodeType {
		case "PullRequest":
			return node.Number, fmt.Sprintf("Auto-detected PR #%d: %s", node.Number, node.Title), nil
		case "Issue":
			// Find associated open PR for this issue
			prs, err := c.FindPRsForIssue(node.Number)
			if err != nil {
				return 0, "", fmt.Errorf("failed to find PRs for auto-detected issue #%d: %w", node.Number, err)
			}
			
			// Look for open PR
			for _, pr := range prs {
				if pr.State == "OPEN" {
					return pr.Number, fmt.Sprintf("Auto-detected issue #%d → open PR #%d: %s", node.Number, pr.Number, pr.Title), nil
				}
			}
			
			if len(prs) > 0 {
				return 0, "", fmt.Errorf("auto-detected issue #%d has %d associated PR(s) but none are open", node.Number, len(prs))
			}
			return 0, "", fmt.Errorf("auto-detected issue #%d has no associated PRs", node.Number)
		default:
			return 0, "", fmt.Errorf("unknown node type: %s", node.NodeType)
		}

	default:
		return 0, "", fmt.Errorf("unknown input format type: %s", format.Type)
	}
}