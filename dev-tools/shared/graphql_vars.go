package shared

// GraphQL query variable builders to eliminate duplication across dev-tools.
// These functions provide consistent variable construction for common query patterns.

// BasicPRVariables creates the standard owner/repo/prNumber variable map
func BasicPRVariables(owner, repo string, prNumber int) map[string]interface{} {
	return map[string]interface{}{
		"owner":    owner,
		"repo":     repo,
		"prNumber": prNumber,
	}
}

// BasicPRVariablesWithBodies extends BasicPRVariables with includeReviewBodies flag
func BasicPRVariablesWithBodies(owner, repo string, prNumber int, includeReviewBodies bool) map[string]interface{} {
	vars := BasicPRVariables(owner, repo, prNumber)
	vars["includeReviewBodies"] = includeReviewBodies
	return vars
}

// WithPagination adds pagination parameters to an existing variable map
func WithPagination(base map[string]interface{}, after, before string, limit int) map[string]interface{} {
	// Create a copy to avoid modifying the original
	result := make(map[string]interface{})
	for k, v := range base {
		result[k] = v
	}
	
	if after != "" {
		result["after"] = after
	}
	if before != "" {
		result["before"] = before
	}
	if limit > 0 {
		result["first"] = limit
	}
	
	return result
}

// ThreadVariables creates variables for thread-specific queries
func ThreadVariables(threadID string) map[string]interface{} {
	return map[string]interface{}{
		"threadID": threadID,
	}
}

// ReviewThreadsVariables creates variables for fetching review threads
func ReviewThreadsVariables(owner, repo string, prNumber int, limit int) map[string]interface{} {
	vars := BasicPRVariables(owner, repo, prNumber)
	if limit > 0 {
		vars["first"] = limit
	}
	return vars
}

// StatusCheckVariables creates variables for status check queries
func StatusCheckVariables(owner, repo string, prNumber int, includeReviewBodies bool) map[string]interface{} {
	return map[string]interface{}{
		"owner":               owner,
		"repo":                repo,
		"prNumber":            prNumber,
		"includeReviewBodies": includeReviewBodies,
	}
}

// UnifiedReviewVariables creates variables for comprehensive review queries
func UnifiedReviewVariables(owner, repo string, prNumber int, includeReviewBodies bool, reviewLimit, threadLimit int) map[string]interface{} {
	vars := BasicPRVariablesWithBodies(owner, repo, prNumber, includeReviewBodies)
	
	if reviewLimit > 0 {
		vars["reviewFirst"] = reviewLimit
	}
	if threadLimit > 0 {
		vars["threadFirst"] = threadLimit
	}
	
	return vars
}