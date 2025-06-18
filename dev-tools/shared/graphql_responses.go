package shared

// Shared GraphQL response structures to eliminate duplication across the codebase.
// These types correspond to common GraphQL query response patterns used throughout dev-tools.

// BasicRepositoryResponse is the standard wrapper for repository-based queries
type BasicRepositoryResponse struct {
	Data struct {
		Repository struct {
			PullRequest struct {
				PRMetadataFields
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

// ReviewsResponse for queries that fetch PR reviews
type ReviewsResponse struct {
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

// ReviewsWithMetadataResponse for queries that fetch reviews plus PR metadata
type ReviewsWithMetadataResponse struct {
	Data struct {
		Repository struct {
			PullRequest struct {
				PRMetadataFields
				Reviews struct {
					Nodes []ReviewFields `json:"nodes"`
				} `json:"reviews"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

// ReviewsWithStatusResponse for queries that fetch reviews and status checks
type ReviewsWithStatusResponse struct {
	Data struct {
		Repository struct {
			PullRequest struct {
				Mergeable        string `json:"mergeable"`
				MergeStateStatus string `json:"mergeStateStatus"`
				Reviews struct {
					Nodes []ReviewFields `json:"nodes"`
				} `json:"reviews"`
				Commits struct {
					Nodes []struct {
						Commit CommitWithStatusFields `json:"commit"`
					} `json:"nodes"`
				} `json:"commits"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

// ThreadsResponse for queries that fetch review threads
type ThreadsResponse struct {
	Data struct {
		Repository struct {
			PullRequest struct {
				ReviewThreads ThreadConnectionFields `json:"reviewThreads"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

// FullReviewResponse for unified review queries with reviews, threads, and metadata
type FullReviewResponse struct {
	Data struct {
		Viewer struct {
			Login string `json:"login"`
		} `json:"viewer"`
		Repository struct {
			PullRequest struct {
				PRMetadataFields
				Reviews       ReviewConnectionFields `json:"reviews"`
				ReviewThreads ThreadConnectionFields `json:"reviewThreads"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

// ThreadDetailResponse for individual thread queries
type ThreadDetailResponse struct {
	Data struct {
		Node struct {
			ID           string `json:"id"`
			Line         *int   `json:"line"`
			Path         string `json:"path"`
			IsResolved   bool   `json:"isResolved"`
			SubjectType  string `json:"subjectType"`
			PullRequest struct {
				Number int    `json:"number"`
				Title  string `json:"title"`
			} `json:"pullRequest"`
			Comments struct {
				Nodes []struct {
					ID       string `json:"id"`
					Body     string `json:"body"`
					Author   struct {
						Login string `json:"login"`
					} `json:"author"`
					CreatedAt string `json:"createdAt"`
					DiffHunk  string `json:"diffHunk"`
				} `json:"nodes"`
			} `json:"comments"`
		} `json:"node"`
	} `json:"data"`
}

// Helper methods for common operations

// GetReviews extracts review nodes from ReviewsResponse
func (r *ReviewsResponse) GetReviews() []ReviewFields {
	return r.Data.Repository.PullRequest.Reviews.Nodes
}

// GetReviews extracts review nodes from ReviewsWithMetadataResponse
func (r *ReviewsWithMetadataResponse) GetReviews() []ReviewFields {
	return r.Data.Repository.PullRequest.Reviews.Nodes
}

// GetReviews extracts review nodes from ReviewsWithStatusResponse
func (r *ReviewsWithStatusResponse) GetReviews() []ReviewFields {
	return r.Data.Repository.PullRequest.Reviews.Nodes
}

// GetPRMetadata extracts PR metadata from ReviewsWithMetadataResponse
func (r *ReviewsWithMetadataResponse) GetPRMetadata() PRMetadataFields {
	return r.Data.Repository.PullRequest.PRMetadataFields
}

// GetStatusCheckRollup extracts status check information from ReviewsWithStatusResponse
func (r *ReviewsWithStatusResponse) GetStatusCheckRollup() *StatusCheckRollupFields {
	commits := r.Data.Repository.PullRequest.Commits.Nodes
	if len(commits) > 0 {
		return commits[0].Commit.StatusCheckRollup
	}
	return nil
}

// GetMergeStatus extracts merge status information from ReviewsWithStatusResponse
func (r *ReviewsWithStatusResponse) GetMergeStatus() (mergeable, mergeStatus string) {
	pr := r.Data.Repository.PullRequest
	return pr.Mergeable, pr.MergeStateStatus
}