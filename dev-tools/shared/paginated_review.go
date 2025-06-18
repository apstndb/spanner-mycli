package shared

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// PaginatedReviewOptions provides safe, parameterized pagination for reviews
type PaginatedReviewOptions struct {
	IncludeReviewBodies bool   // Include full review bodies
	Limit               int    // Number of reviews to fetch (default: 20)
	After               string // Cursor for forward pagination
	Before              string // Cursor for backward pagination
}

// PaginatedReviewResponse contains reviews with pagination metadata
type PaginatedReviewResponse struct {
	PR           PRMetadata    `json:"pr"`
	Reviews      []ReviewData  `json:"reviews"`
	CurrentUser  string        `json:"currentUser"`
	PageInfo     PageInfo      `json:"pageInfo"`
	FetchedAt    time.Time     `json:"fetchedAt"`
}

// GetPaginatedReviews fetches reviews with safe pagination using GraphQL variables
//
// DESIGN RATIONALE: Separate Pagination Approach
// Instead of using fmt.Sprintf which can be unsafe, this function uses:
// 1. Separate, focused GraphQL queries for each pagination direction
// 2. Proper variable binding for all parameters
// 3. Conditional logic in Go rather than complex GraphQL directives
// 4. Clear separation of concerns (reviews only, no threads)
//
// This addresses the lesson learned from PR #306 where critical review feedback
// was missed because it was in review bodies, not thread comments.
func (c *GitHubClient) GetPaginatedReviews(prNumber string, opts PaginatedReviewOptions) (*PaginatedReviewResponse, error) {
	prNumberInt, err := strconv.Atoi(prNumber)
	if err != nil {
		return nil, fmt.Errorf("invalid PR number format: %w", err)
	}

	// Apply defaults
	if opts.Limit <= 0 {
		opts.Limit = 20
	}

	// Base query with fragments for reusability
	baseQuery := `
fragment PageInfoFields on PageInfo {
  hasNextPage
  hasPreviousPage
  startCursor
  endCursor
}

fragment ReviewCommentFields on PullRequestReviewComment {
  id
  body
  path
  line
  createdAt
}

fragment ReviewFields on PullRequestReview {
  id
  author { login }
  createdAt
  state
  body @include(if: $includeReviewBodies)
  comments(first: 50) @include(if: $includeReviewBodies) {
    nodes {
      ...ReviewCommentFields
    }
  }
}

fragment ReviewConnectionFields on PullRequestReviewConnection {
  totalCount
  pageInfo {
    ...PageInfoFields
  }
  nodes {
    ...ReviewFields
  }
}

fragment PRMetadataFields on PullRequest {
  number
  title
  state
  mergeable
  mergeStateStatus
}`

	// Determine which query to use based on pagination direction
	var query string
	var variables map[string]interface{}

	if opts.After != "" {
		// Forward pagination
		query = baseQuery + `
query($owner: String!, $repo: String!, $prNumber: Int!, $limit: Int!, $after: String!, $includeReviewBodies: Boolean!) {
  viewer { login }
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $prNumber) {
      ...PRMetadataFields
      reviews(first: $limit, after: $after) {
        ...ReviewConnectionFields
      }
    }
  }
}`
		variables = map[string]interface{}{
			"owner":               c.Owner,
			"repo":                c.Repo,
			"prNumber":            prNumberInt,
			"limit":               opts.Limit,
			"after":               opts.After,
			"includeReviewBodies": opts.IncludeReviewBodies,
		}
	} else if opts.Before != "" {
		// Backward pagination
		query = baseQuery + `
query($owner: String!, $repo: String!, $prNumber: Int!, $limit: Int!, $before: String!, $includeReviewBodies: Boolean!) {
  viewer { login }
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $prNumber) {
      ...PRMetadataFields
      reviews(last: $limit, before: $before) {
        ...ReviewConnectionFields
      }
    }
  }
}`
		variables = map[string]interface{}{
			"owner":               c.Owner,
			"repo":                c.Repo,
			"prNumber":            prNumberInt,
			"limit":               opts.Limit,
			"before":              opts.Before,
			"includeReviewBodies": opts.IncludeReviewBodies,
		}
	} else {
		// Default: fetch latest reviews
		query = baseQuery + `
query($owner: String!, $repo: String!, $prNumber: Int!, $limit: Int!, $includeReviewBodies: Boolean!) {
  viewer { login }
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $prNumber) {
      ...PRMetadataFields
      reviews(last: $limit) {
        ...ReviewConnectionFields
      }
    }
  }
}`
		variables = map[string]interface{}{
			"owner":               c.Owner,
			"repo":                c.Repo,
			"prNumber":            prNumberInt,
			"limit":               opts.Limit,
			"includeReviewBodies": opts.IncludeReviewBodies,
		}
	}

	result, err := c.RunGraphQLQueryWithVariables(query, variables)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch paginated reviews: %w", err)
	}

	// Parse response (same structure for all three queries)
	var response struct {
		Data struct {
			Viewer struct {
				Login string `json:"login"`
			} `json:"viewer"`
			Repository struct {
				PullRequest struct {
					Number           int    `json:"number"`
					Title            string `json:"title"`
					State            string `json:"state"`
					Mergeable        string `json:"mergeable"`
					MergeStateStatus string `json:"mergeStateStatus"`
					Reviews struct {
						TotalCount int `json:"totalCount"`
						PageInfo   struct {
							HasNextPage     bool   `json:"hasNextPage"`
							HasPreviousPage bool   `json:"hasPreviousPage"`
							StartCursor     string `json:"startCursor"`
							EndCursor       string `json:"endCursor"`
						} `json:"pageInfo"`
						Nodes []struct {
							ID        string `json:"id"`
							Author    struct {
								Login string `json:"login"`
							} `json:"author"`
							CreatedAt string `json:"createdAt"`
							State     string `json:"state"`
							Body      string `json:"body"`
							Comments  struct {
								Nodes []struct {
									ID        string `json:"id"`
									Body      string `json:"body"`
									Path      string `json:"path"`
									Line      *int   `json:"line"`
									CreatedAt string `json:"createdAt"`
								} `json:"nodes"`
							} `json:"comments"`
						} `json:"nodes"`
					} `json:"reviews"`
				} `json:"pullRequest"`
			} `json:"repository"`
		} `json:"data"`
	}

	if err := json.Unmarshal(result, &response); err != nil {
		return nil, fmt.Errorf("failed to parse paginated reviews response: %w", err)
	}

	pr := response.Data.Repository.PullRequest
	currentUser := response.Data.Viewer.Login

	// Process reviews with severity analysis
	var reviews []ReviewData
	for _, review := range pr.Reviews.Nodes {
		if review.State == "PENDING" {
			continue
		}

		// Analyze review body
		severity := analyzeReviewSeverity(review.Body)
		actionItems := extractActionItems(review.Body)

		// Process review comments
		var comments []ReviewComment
		for _, comment := range review.Comments.Nodes {
			comments = append(comments, ReviewComment{
				ID:        comment.ID,
				Body:      comment.Body,
				Path:      comment.Path,
				Line:      comment.Line,
				CreatedAt: comment.CreatedAt,
			})
		}

		reviews = append(reviews, ReviewData{
			ID:          review.ID,
			Author:      review.Author.Login,
			CreatedAt:   review.CreatedAt,
			State:       review.State,
			Body:        review.Body,
			Comments:    comments,
			Severity:    severity,
			ActionItems: actionItems,
		})
	}

	return &PaginatedReviewResponse{
		PR: PRMetadata{
			Number:      pr.Number,
			Title:       pr.Title,
			State:       pr.State,
			Mergeable:   pr.Mergeable,
			MergeStatus: pr.MergeStateStatus,
		},
		Reviews:     reviews,
		CurrentUser: currentUser,
		PageInfo: PageInfo{
			HasNextPage:     pr.Reviews.PageInfo.HasNextPage,
			HasPreviousPage: pr.Reviews.PageInfo.HasPreviousPage,
			StartCursor:     pr.Reviews.PageInfo.StartCursor,
			EndCursor:       pr.Reviews.PageInfo.EndCursor,
			TotalCount:      pr.Reviews.TotalCount,
		},
		FetchedAt: time.Now(),
	}, nil
}