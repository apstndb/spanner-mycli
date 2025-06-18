package shared

import "time"

// GraphQL Fragment type definitions
// These types correspond directly to the GraphQL fragments for type safety and reusability

// PageInfoFields corresponds to fragment PageInfoFields on PageInfo
type PageInfoFields struct {
	HasNextPage     bool   `json:"hasNextPage"`
	HasPreviousPage bool   `json:"hasPreviousPage"`
	StartCursor     string `json:"startCursor"`
	EndCursor       string `json:"endCursor"`
}

// ReviewCommentFields corresponds to fragment ReviewCommentFields on PullRequestReviewComment
type ReviewCommentFields struct {
	ID        string `json:"id"`
	Body      string `json:"body"`
	Path      string `json:"path"`
	Line      *int   `json:"line"`
	CreatedAt string `json:"createdAt"`
}

// ReviewFields corresponds to fragment ReviewFields on PullRequestReview
type ReviewFields struct {
	ID        string `json:"id"`
	Author    struct {
		Login string `json:"login"`
	} `json:"author"`
	CreatedAt string                   `json:"createdAt"`
	State     string                   `json:"state"`
	Body      string                   `json:"body"`
	Comments  struct {
		Nodes []ReviewCommentFields `json:"nodes"`
	} `json:"comments"`
}

// ReviewConnectionFields corresponds to fragment ReviewConnectionFields on PullRequestReviewConnection
type ReviewConnectionFields struct {
	TotalCount int               `json:"totalCount"`
	PageInfo   PageInfoFields    `json:"pageInfo"`
	Nodes      []ReviewFields    `json:"nodes"`
}

// PRMetadataFields corresponds to fragment PRMetadataFields on PullRequest
type PRMetadataFields struct {
	Number           int    `json:"number"`
	Title            string `json:"title"`
	State            string `json:"state"`
	Mergeable        string `json:"mergeable"`
	MergeStateStatus string `json:"mergeStateStatus"`
}

// ThreadCommentFields corresponds to fragment ThreadCommentFields on PullRequestReviewComment
type ThreadCommentFields struct {
	ID        string `json:"id"`
	Author    struct {
		Login string `json:"login"`
	} `json:"author"`
	Body      string `json:"body"`
	CreatedAt string `json:"createdAt"`
}

// ThreadFields corresponds to fragment ThreadFields on PullRequestReviewThread
type ThreadFields struct {
	ID         string `json:"id"`
	Path       string `json:"path"`
	Line       *int   `json:"line"`
	IsResolved bool   `json:"isResolved"`
	IsOutdated bool   `json:"isOutdated"`
	Comments   struct {
		Nodes []ThreadCommentFields `json:"nodes"`
	} `json:"comments"`
}

// ThreadConnectionFields corresponds to fragment ThreadConnectionFields on PullRequestReviewThreadConnection
type ThreadConnectionFields struct {
	TotalCount int              `json:"totalCount"`
	PageInfo   PageInfoFields   `json:"pageInfo"`
	Nodes      []ThreadFields   `json:"nodes"`
}

// StatusCheckRollupFields corresponds to fragment StatusCheckRollupFields on StatusCheckRollup
type StatusCheckRollupFields struct {
	State    string `json:"state"`
	Contexts struct {
		Nodes []interface{} `json:"nodes"`
	} `json:"contexts"`
}

// CommitWithStatusFields corresponds to fragment CommitWithStatusFields on Commit
type CommitWithStatusFields struct {
	StatusCheckRollup *StatusCheckRollupFields `json:"statusCheckRollup"`
}

// GraphQL Fragment definitions as constants
// These correspond exactly to the Go types above for consistency
const (
	PageInfoFragment = `
fragment PageInfoFields on PageInfo {
  hasNextPage
  hasPreviousPage
  startCursor
  endCursor
}`

	ReviewCommentFragment = `
fragment ReviewCommentFields on PullRequestReviewComment {
  id
  body
  path
  line
  createdAt
}`

	ReviewFragment = `
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
}`

	ReviewConnectionFragment = `
fragment ReviewConnectionFields on PullRequestReviewConnection {
  totalCount
  pageInfo {
    ...PageInfoFields
  }
  nodes {
    ...ReviewFields
  }
}`

	PRMetadataFragment = `
fragment PRMetadataFields on PullRequest {
  number
  title
  state
  mergeable
  mergeStateStatus
}`

	ThreadCommentFragment = `
fragment ThreadCommentFields on PullRequestReviewComment {
  id
  author { login }
  body
  createdAt
}`

	ThreadFragment = `
fragment ThreadFields on PullRequestReviewThread {
  id
  path
  line
  isResolved
  isOutdated
  comments(first: 20) {
    nodes {
      ...ThreadCommentFields
    }
  }
}`

	ThreadConnectionFragment = `
fragment ThreadConnectionFields on PullRequestReviewThreadConnection {
  totalCount
  pageInfo {
    ...PageInfoFields
  }
  nodes {
    ...ThreadFields
  }
}`

	StatusCheckRollupFragment = `
fragment StatusCheckRollupFields on StatusCheckRollup {
  state
  contexts(first: 50) {
    nodes {
      ... on StatusContext {
        context
        state
        targetUrl
      }
      ... on CheckRun {
        name
        status
        conclusion
        detailsUrl
      }
    }
  }
}`

	CommitWithStatusFragment = `
fragment CommitWithStatusFields on Commit {
  statusCheckRollup {
    ...StatusCheckRollupFields
  }
}`

	// Combined fragments for reuse
	AllReviewFragments = PageInfoFragment + ReviewCommentFragment + ReviewFragment + ReviewConnectionFragment + PRMetadataFragment

	AllThreadFragments = PageInfoFragment + ThreadCommentFragment + ThreadFragment + ThreadConnectionFragment

	AllStatusFragments = StatusCheckRollupFragment + CommitWithStatusFragment

	AllFragments = PageInfoFragment + ReviewCommentFragment + ReviewFragment + ReviewConnectionFragment + 
		PRMetadataFragment + ThreadCommentFragment + ThreadFragment + ThreadConnectionFragment + 
		StatusCheckRollupFragment + CommitWithStatusFragment
)

// Conversion functions between fragment types and domain types

// ToReviewData converts ReviewFields to ReviewData with analysis
func (rf ReviewFields) ToReviewData() ReviewData {
	// Convert comments
	var comments []ReviewComment
	for _, comment := range rf.Comments.Nodes {
		comments = append(comments, ReviewComment(comment))
	}

	// Analyze review content
	severity := analyzeReviewSeverity(rf.Body)
	actionItems := extractActionItems(rf.Body)

	return ReviewData{
		ID:          rf.ID,
		Author:      rf.Author.Login,
		CreatedAt:   rf.CreatedAt,
		State:       rf.State,
		Body:        rf.Body,
		Comments:    comments,
		Severity:    severity,
		ActionItems: actionItems,
	}
}

// ToThreadData converts ThreadFields to ThreadData with reply analysis
func (tf ThreadFields) ToThreadData(currentUser string) ThreadData {
	// Convert comments and determine reply status
	var comments []ThreadComment
	needsReply := !tf.IsResolved && !tf.IsOutdated
	lastReplier := ""

	for _, comment := range tf.Comments.Nodes {
		comments = append(comments, ThreadComment{
			ID:        comment.ID,
			Author:    comment.Author.Login,
			Body:      comment.Body,
			CreatedAt: comment.CreatedAt,
		})

		// Update reply status
		if comment.Author.Login == currentUser {
			needsReply = false
		}
		lastReplier = comment.Author.Login
	}

	return ThreadData{
		ID:          tf.ID,
		Path:        tf.Path,
		Line:        tf.Line,
		IsResolved:  tf.IsResolved,
		IsOutdated:  tf.IsOutdated,
		Comments:    comments,
		NeedsReply:  needsReply,
		LastReplier: lastReplier,
	}
}

// ToPRMetadata converts PRMetadataFields to PRMetadata
func (prf PRMetadataFields) ToPRMetadata() PRMetadata {
	return PRMetadata{
		Number:      prf.Number,
		Title:       prf.Title,
		State:       prf.State,
		Mergeable:   prf.Mergeable,
		MergeStatus: prf.MergeStateStatus,
	}
}

// ToPageInfo converts PageInfoFields to PageInfo with optional total count
func (pif PageInfoFields) ToPageInfo(totalCount int) PageInfo {
	return PageInfo{
		HasNextPage:     pif.HasNextPage,
		HasPreviousPage: pif.HasPreviousPage,
		StartCursor:     pif.StartCursor,
		EndCursor:       pif.EndCursor,
		TotalCount:      totalCount,
	}
}

// FragmentBasedResponse represents a response using fragment-based types
type FragmentBasedResponse struct {
	Viewer struct {
		Login string `json:"login"`
	} `json:"viewer"`
	Repository struct {
		PullRequest struct {
			PRMetadataFields
			Reviews ReviewConnectionFields `json:"reviews"`
			ReviewThreads ThreadConnectionFields `json:"reviewThreads,omitempty"`
		} `json:"pullRequest"`
	} `json:"repository"`
}

// ToUnifiedReviewData converts fragment-based response to domain types
func (fbr FragmentBasedResponse) ToUnifiedReviewData() *UnifiedReviewData {
	currentUser := fbr.Viewer.Login
	pr := fbr.Repository.PullRequest

	// Convert reviews
	var reviews []ReviewData
	for _, reviewField := range pr.Reviews.Nodes {
		if reviewField.State == "PENDING" {
			continue
		}
		reviews = append(reviews, reviewField.ToReviewData())
	}

	// Convert threads
	var threads []ThreadData
	for _, threadField := range pr.ReviewThreads.Nodes {
		threads = append(threads, threadField.ToThreadData(currentUser))
	}

	return &UnifiedReviewData{
		PR:             pr.ToPRMetadata(),
		Reviews:        reviews,
		Threads:        threads,
		CurrentUser:    currentUser,
		FetchedAt:      time.Now(),
		ReviewPageInfo: pr.Reviews.PageInfo.ToPageInfo(pr.Reviews.TotalCount),
		ThreadPageInfo: pr.ReviewThreads.PageInfo.ToPageInfo(pr.ReviewThreads.TotalCount),
	}
}