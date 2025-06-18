package shared

import (
	"encoding/json"
	"fmt"
)

// NodeQueryConfig configures individual node queries (by ID)
type NodeQueryConfig struct {
	NodeID                string
	IncludeThreadMetadata bool // For isOutdated, subjectType, pullRequest
	IncludeCommentDetails bool // For diffHunk in comments
	CommentLimit          int
}

// NewNodeQueryConfig creates a basic node query configuration
func NewNodeQueryConfig(nodeID string) *NodeQueryConfig {
	return &NodeQueryConfig{
		NodeID:       nodeID,
		CommentLimit: 20,
	}
}

// ForThreadDetails configures for detailed thread display
func (c *NodeQueryConfig) ForThreadDetails() *NodeQueryConfig {
	c.IncludeThreadMetadata = true
	c.IncludeCommentDetails = true
	c.CommentLimit = 50
	return c
}

// ToGraphQLVariables converts config to GraphQL variables
func (c *NodeQueryConfig) ToGraphQLVariables() map[string]interface{} {
	return map[string]interface{}{
		"nodeID":                c.NodeID,
		"includeThreadMetadata": c.IncludeThreadMetadata,
		"includeCommentDetails": c.IncludeCommentDetails,
		"commentLimit":          c.CommentLimit,
	}
}

// NodeQuery for fetching individual nodes by ID
const NodeQuery = `
fragment ThreadCommentWithDetails on PullRequestReviewComment {
  id
  body
  author { login }
  createdAt
  diffHunk @include(if: $includeCommentDetails)
}

query NodeQuery(
  $nodeID: ID!
  $includeThreadMetadata: Boolean! = false
  $includeCommentDetails: Boolean! = false
  $commentLimit: Int = 20
) {
  node(id: $nodeID) {
    ... on PullRequestReviewThread {
      id
      path
      line
      isResolved
      isOutdated @include(if: $includeThreadMetadata)
      subjectType @include(if: $includeThreadMetadata)
      pullRequest @include(if: $includeThreadMetadata) {
        number
        title
      }
      comments(first: $commentLimit) {
        nodes {
          ...ThreadCommentWithDetails
        }
      }
    }
  }
}`

// NodeResponse for node-based queries
type NodeResponse struct {
	Data struct {
		Node struct {
			ID          string `json:"id"`
			Path        string `json:"path"`
			Line        *int   `json:"line"`
			IsResolved  bool   `json:"isResolved"`
			IsOutdated  *bool  `json:"isOutdated,omitempty"`
			SubjectType *string `json:"subjectType,omitempty"`
			PullRequest *struct {
				Number int    `json:"number"`
				Title  string `json:"title"`
			} `json:"pullRequest,omitempty"`
			Comments struct {
				Nodes []struct {
					ID        string `json:"id"`
					Body      string `json:"body"`
					Author    struct {
						Login string `json:"login"`
					} `json:"author"`
					CreatedAt string  `json:"createdAt"`
					DiffHunk  *string `json:"diffHunk,omitempty"`
				} `json:"nodes"`
			} `json:"comments"`
		} `json:"node"`
	} `json:"data"`
}

// FetchNodeData executes a node query with given configuration
func (client *GitHubClient) FetchNodeData(config *NodeQueryConfig) (*NodeResponse, error) {
	variables := config.ToGraphQLVariables()

	result, err := client.RunGraphQLQueryWithVariables(NodeQuery, variables)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch node data for %s: %w", config.NodeID, err)
	}

	var response NodeResponse
	if err := json.Unmarshal(result, &response); err != nil {
		return nil, fmt.Errorf("failed to parse node response: %w", err)
	}

	return &response, nil
}