package shared

// GraphQL Operation Types
// This file defines all GraphQL queries, mutations, and their corresponding Go types
// for type-safe GraphQL operations across the dev-tools codebase.


// =============================================================================
// Query Types
// =============================================================================

// Basic Query Variables
type BasicRepoVariables struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
}

type PRVariables struct {
	Owner    string `json:"owner"`
	Repo     string `json:"repo"`
	PRNumber int    `json:"prNumber"`
}

type NodeVariables struct {
	NodeID string `json:"nodeID"`
}

// =============================================================================
// Universal PR Query Types
// =============================================================================

type UniversalPRQueryVariables struct {
	Owner                   string `json:"owner"`
	Repo                    string `json:"repo"`
	PRNumber                int    `json:"prNumber"`
	IncludeReviews          bool   `json:"includeReviews"`
	IncludeThreads          bool   `json:"includeThreads"`
	IncludeStatus           bool   `json:"includeStatus"`
	IncludeMetadata         bool   `json:"includeMetadata"`
	IncludeReviewBodies     bool   `json:"includeReviewBodies"`
	IncludePagination       bool   `json:"includePagination"`
	IncludeThreadMetadata   bool   `json:"includeThreadMetadata"`
	IncludeCommentDetails   bool   `json:"includeCommentDetails"`
	ReviewLimit             int    `json:"reviewLimit"`
	ThreadLimit             int    `json:"threadLimit"`
}

// =============================================================================
// Node Query Types  
// =============================================================================

type NodeQueryVariables struct {
	NodeID                string `json:"nodeID"`
	IncludeThreadMetadata bool   `json:"includeThreadMetadata"`
	IncludeCommentDetails bool   `json:"includeCommentDetails"`
	CommentLimit          int    `json:"commentLimit"`
}

// =============================================================================
// Paginated Query Types
// =============================================================================

type PaginatedReviewQueryVariables struct {
	Owner               string `json:"owner"`
	Repo                string `json:"repo"`
	PRNumber            int    `json:"prNumber"`
	Limit               int    `json:"limit"`
	After               string `json:"after,omitempty"`
	Before              string `json:"before,omitempty"`
	IncludeReviewBodies bool   `json:"includeReviewBodies"`
}

// =============================================================================
// Mutation Types
// =============================================================================

// Thread Reply Mutation (for new comments in threads)
type AddPullRequestReviewCommentInput struct {
	ClientMutationID    *string `json:"clientMutationId,omitempty"`
	PullRequestID       *string `json:"pullRequestId,omitempty"`
	PullRequestReviewID *string `json:"pullRequestReviewId,omitempty"`
	CommitOID           *string `json:"commitOID,omitempty"`
	Body                string  `json:"body"`
	Path                *string `json:"path,omitempty"`
	Position            *int    `json:"position,omitempty"`
	InReplyTo           *string `json:"inReplyTo,omitempty"`
}

type AddPullRequestReviewCommentVariables struct {
	Input AddPullRequestReviewCommentInput `json:"input"`
}

type AddPullRequestReviewCommentResponse struct {
	Data struct {
		AddPullRequestReviewComment struct {
			Comment struct {
				ID  string `json:"id"`
				URL string `json:"url"`
			} `json:"comment"`
		} `json:"addPullRequestReviewComment"`
	} `json:"data"`
}

// Thread Reply Mutation (for replying to existing threads)
type AddPullRequestReviewThreadReplyInput struct {
	ClientMutationID              *string `json:"clientMutationId,omitempty"`
	PullRequestReviewID           *string `json:"pullRequestReviewId,omitempty"`
	PullRequestReviewThreadID     string  `json:"pullRequestReviewThreadId"`
	Body                          string  `json:"body"`
}

type AddPullRequestReviewThreadReplyVariables struct {
	Input AddPullRequestReviewThreadReplyInput `json:"input"`
}

type AddPullRequestReviewThreadReplyResponse struct {
	Data struct {
		AddPullRequestReviewThreadReply struct {
			Comment struct {
				ID   string `json:"id"`
				URL  string `json:"url"`
				Body string `json:"body"`
			} `json:"comment"`
		} `json:"addPullRequestReviewThreadReply"`
	} `json:"data"`
}

// Add Comment Mutation (for PR/Issue comments)
type AddCommentInput struct {
	ClientMutationID *string `json:"clientMutationId,omitempty"`
	SubjectID        string  `json:"subjectId"`
	Body             string  `json:"body"`
}

type AddCommentVariables struct {
	Input AddCommentInput `json:"input"`
}

type AddCommentResponse struct {
	Data struct {
		AddComment struct {
			CommentEdge struct {
				Node struct {
					ID  string `json:"id"`
					URL string `json:"url"`
				} `json:"node"`
			} `json:"commentEdge"`
		} `json:"addComment"`
	} `json:"data"`
}

// Create PR Mutation
type CreatePullRequestInput struct {
	ClientMutationID     *string `json:"clientMutationId,omitempty"`
	RepositoryID         string  `json:"repositoryId"`
	BaseRefName          string  `json:"baseRefName"`
	HeadRefName          string  `json:"headRefName"`
	HeadRepositoryID     *string `json:"headRepositoryId,omitempty"`
	Title                string  `json:"title"`
	Body                 *string `json:"body,omitempty"`
	MaintainerCanModify  *bool   `json:"maintainerCanModify,omitempty"`
	Draft                *bool   `json:"draft,omitempty"`
}

type CreatePullRequestVariables struct {
	Input CreatePullRequestInput `json:"input"`
}

type CreatePullRequestResponse struct {
	Data struct {
		CreatePullRequest struct {
			PullRequest PRInfo `json:"pullRequest"`
		} `json:"createPullRequest"`
	} `json:"data"`
}

// =============================================================================
// Repository Query Types
// =============================================================================

type RepositoryIDQueryVariables struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
}

type RepositoryIDResponse struct {
	Data struct {
		Repository struct {
			ID string `json:"id"`
		} `json:"repository"`
	} `json:"data"`
}

type PRIDQueryVariables struct {
	Owner    string `json:"owner"`
	Repo     string `json:"repo"`
	PRNumber int    `json:"prNumber"`
}

type PRIDResponse struct {
	Data struct {
		Repository struct {
			PullRequest struct {
				ID string `json:"id"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

// =============================================================================
// Issue/PR Resolution Types
// =============================================================================

type ResolveNumberVariables struct {
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Number int    `json:"number"`
}

type ResolveNumberResponse struct {
	Data struct {
		Repository struct {
			Issue       *IssueNode       `json:"issue"`
			PullRequest *PullRequestNode `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

type IssueNode struct {
	ID       string `json:"id"`
	Number   int    `json:"number"`
	Title    string `json:"title"`
	NodeType string `json:"__typename"`
}

type PullRequestNode struct {
	ID       string `json:"id"`
	Number   int    `json:"number"`
	Title    string `json:"title"`
	NodeType string `json:"__typename"`
}

// =============================================================================
// Current Branch PR Query Types
// =============================================================================

type CurrentBranchPRVariables struct {
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Branch string `json:"branch"`
}

type CurrentBranchPRResponse struct {
	Data struct {
		Repository struct {
			PullRequests struct {
				Nodes []PRInfo `json:"nodes"`
			} `json:"pullRequests"`
		} `json:"repository"`
	} `json:"data"`
}

// =============================================================================
// Associated PRs Query Types
// =============================================================================

type AssociatedPRsVariables struct {
	Owner       string `json:"owner"`
	Repo        string `json:"repo"`
	IssueNumber int    `json:"issueNumber"`
}

type AssociatedPRsResponse struct {
	Data struct {
		Repository struct {
			Issue struct {
				TimelineItems struct {
					Nodes []TimelineItem `json:"nodes"`
				} `json:"timelineItems"`
			} `json:"issue"`
		} `json:"repository"`
	} `json:"data"`
}

type TimelineItem struct {
	Type        string   `json:"__typename"`
	PullRequest *PRInfo  `json:"pullRequest,omitempty"`
}

// =============================================================================
// Domain Types (used across multiple operations)
// =============================================================================

// PRInfo and PRCreateOptions are defined in github.go

// =============================================================================
// Review Monitor Types
// =============================================================================

type ReviewMonitorVariables struct {
	Owner    string `json:"owner"`
	Repo     string `json:"repo"`
	PRNumber int    `json:"prNumber"`
}

type ReviewMonitorResponse struct {
	Data struct {
		Repository struct {
			PullRequest struct {
				Reviews struct {
					Nodes []ReviewFields `json:"nodes"`
				} `json:"reviews"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

// =============================================================================
// Common Types
// =============================================================================

type ErrorResponse struct {
	Errors []struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"errors"`
}

// =============================================================================
// Utility Types for Operation Configuration
// =============================================================================

type QueryConfig interface {
	ToGraphQLVariables() map[string]interface{}
}

type MutationConfig interface {
	ToGraphQLVariables() map[string]interface{}
}