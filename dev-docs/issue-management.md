# Issue and Code Review Management

This document covers GitHub workflow, issue management, and code review processes for spanner-mycli.

## Issue Management

### Issue Lifecycle

- Issues managed through GitHub Issues with comprehensive labeling
- All fixes must go through Pull Requests - never close issues manually
- Use "claude-code" label for issues identified by automated code analysis
- Additional labels: bug, enhancement, documentation, testing, tech-debt, performance, blocked, concurrency

### Creating Issues

Use `gh` CLI for better control over formatting:

```bash
# Create issue with template
gh issue create --title "Issue title" --body "$(cat <<'EOF'
## Problem
Description of the issue...

## Expected vs Actual Behavior
What should happen vs what happens...

## Steps to Reproduce
1. Step one
2. Step two
3. Step three

## Environment
- spanner-mycli version: 
- Go version:
- OS:
EOF
)" --label "bug,enhancement"
```

### Updating Issues

```bash
# Update issue with comprehensive implementation plan
gh issue edit 209 --body "$(cat <<'EOF'
## Current Status
Analysis of current state...

## Implementation Plan

### Phase 1: Basic feature (independently mergeable)
- **System Variable**: `CLI_FEATURE_NAME` (default: false)
- **Implementation**: Location and approach
- **Testing**: Test strategy

### Phase 2: Advanced features
- **Extension**: Additional functionality
- **Integration**: How it connects with existing features
- **Documentation**: User guide updates

## Success Criteria
- [ ] Feature works as specified
- [ ] Tests pass
- [ ] Documentation updated
EOF
)"
```

## Sub-Issue Management

GitHub sub-issues can be managed using GraphQL mutations (REST API POST endpoints return 404).

### Adding Sub-Issues

```bash
# First get the node IDs for parent and child issues
gh api graphql -f query='
{
  repository(owner: "apstndb", name: "spanner-mycli") {
    parentIssue: issue(number: 5) { id title }
    childIssue: issue(number: 276) { id title }
  }
}'

# Add sub-issue using GraphQL mutation (requires GraphQL-Features header)
gh api graphql -H "GraphQL-Features: sub_issues" -f query='
mutation {
  addSubIssue(input: { 
    issueId: "I_kwDONC6gMM6a-_9T",     # Parent issue node ID
    subIssueId: "I_kwDONC6gMM67riuz"   # Child issue node ID
  }) {
    issue { title }
    subIssue { title }
  }
}'

# Verify sub-issues were added
gh api /repos/apstndb/spanner-mycli/issues/5/sub_issues
```

### Managing Sub-Issues

```bash
# Other useful GraphQL mutations for sub-issue management:

# Remove a sub-issue from parent
gh api graphql -H "GraphQL-Features: sub_issues" -f query='
mutation {
  removeSubIssue(input: {
    issueId: "PARENT_ISSUE_NODE_ID",
    subIssueId: "CHILD_ISSUE_NODE_ID"
  }) {
    issue { title }
  }
}'

# Change sub-issue position in parent's list
gh api graphql -H "GraphQL-Features: sub_issues" -f query='
mutation {
  reprioritizeSubIssue(input: {
    issueId: "PARENT_ISSUE_NODE_ID",
    subIssueId: "CHILD_ISSUE_NODE_ID",
    afterId: "ANOTHER_CHILD_ISSUE_NODE_ID"
  }) {
    issue { title }
  }
}'
```

### Efficient Sub-Issue Verification

To minimize token usage, request only needed fields when checking sub-issues:

```bash
# Get only sub-issue numbers (minimal token usage)
gh api /repos/apstndb/spanner-mycli/issues/5/sub_issues --jq '.[].number'

# Get number and title (moderate token usage)
gh api /repos/apstndb/spanner-mycli/issues/5/sub_issues --jq '.[] | {number, title}'

# Count sub-issues
gh api /repos/apstndb/spanner-mycli/issues/5/sub_issues --jq 'length'

# Check if specific sub-issue exists
gh api /repos/apstndb/spanner-mycli/issues/5/sub_issues --jq 'map(.number) | contains([276])'

# GraphQL approach (more efficient for multiple operations)
gh api graphql -f query='
{
  repository(owner: "apstndb", name: "spanner-mycli") {
    issue(number: 5) {
      subIssues(first: 10) {
        nodes { number title }
      }
    }
  }
}' --jq '.data.repository.issue.subIssues.nodes[]'
```

**Avoid**: Using full REST API response without `--jq` filtering as it returns extensive metadata and wastes tokens.

### Key Points

- REST API endpoints (`POST /repos/{owner}/{repo}/issues/{issue_number}/sub_issues`) return 404
- GraphQL mutations work with `GraphQL-Features: sub_issues` header
- Use `addSubIssue` mutation with issue node IDs (not issue numbers)
- Sub-issues can be verified using REST API GET endpoint

## Issue Review Workflow

### Implementation Planning Guidelines

- **DO NOT include time estimates** - they are meaningless for planning
- **Ensure phases are independently mergeable** - each phase should be a complete PR
- **Focus on spanner-mycli specific functionality** - distinguish from Spanner core features
- **Create system variables for new features** - follow existing patterns
- **Reference specific code locations**: Use `file_path:line_number` format

### Issue Planning Template

```markdown
## Implementation Plan

### Phase 1: Core Implementation (independently mergeable)
- **Files to Modify**: `main.go:90-95`, `system_variables.go:606`
- **System Variable**: `CLI_FEATURE_NAME` (default: false)
- **Implementation**: Brief description of approach
- **Testing**: Test strategy and coverage

### Phase 2: Integration (independently mergeable)
- **Files to Modify**: Additional integration points
- **Dependencies**: What Phase 1 provides
- **Testing**: Integration test requirements

## Success Criteria
- [ ] Feature works as specified
- [ ] All tests pass (`make test && make lint`)
- [ ] Documentation updated
- [ ] Backward compatibility maintained
```

## Pull Request Process

### Creating Pull Requests

- Link PRs to issues using "Fixes #issue-number" in commit messages and PR descriptions
- Use descriptive commit messages following conventional format
- Include clear description of changes and test plan
- Ensure `make test && make lint` pass before creating PR

### PR Creation with Insights Template

```bash
# Create PR with development insights template
gh pr create --title "feat: implement feature X" --body "$(cat <<'EOF'
## Summary
Brief description of changes...

## Key Changes
- **main.go**: Added CLI flag for feature X
- **system_variables.go**: Implemented FEATURE_X system variable
- **statements_feature.go**: Core feature implementation

## Development Insights

### Discoveries
- Pattern: Error handling approach for component X  
- Architecture: Component Y dependency should be reversed
- Testing: Integration test pattern for feature Z

### Process Insights  
- Workflow: phantom + tmux horizontal split works best for this type of issue
- Tool: Command X saves significant time for debugging

### CLAUDE.md Integration Candidates
- Add pattern X to Development Flow Insights section
- Update Dependencies section with library behavior Y

## Test Plan
- [ ] Unit tests pass
- [ ] Integration tests pass
- [ ] Manual testing completed
- [ ] Lint checks pass

Fixes #XXX
EOF
)"
```

### Adding Development Insights During Review

```bash
# Add additional insights discovered during review process
gh pr comment <PR-number> --body "$(cat <<'EOF'
## Review Process Insights

### Code Review Discoveries
- Pattern Y emerged during review discussion
- Alternative approach Z suggested by reviewer

### CI/Testing Insights
- Test failure revealed edge case in component A
- Performance consideration discovered for component B
EOF
)"
```

## Code Review Response Strategy

### Addressing Review Comments

1. **Address each comment individually** with focused commits
2. **Use descriptive commit messages** referencing specific issues
3. **Test thoroughly** before pushing changes
4. **For AI reviews (Gemini Code Assist)**: Use `/gemini review` to trigger re-review
5. **For praise comments**: Acknowledge briefly and resolve conversation

### Replying to Review Thread Comments

**Method 1: Automated Scripts (Recommended)**

```bash
# List unresolved review threads that need replies
scripts/dev/list-review-threads.sh 287

# Reply to a specific thread
scripts/dev/review-reply.sh PRRT_kwDONC6gMM5SU-GH "Thank you for the feedback!"

# Reply with mention for AI reviews
scripts/dev/review-reply.sh PRRT_kwDONC6gMM5SVHTH "Fixed as suggested!" gemini-code-assist
```

**Method 2: Manual GraphQL Mutation (For understanding the process)**

```bash
# Step 1: Get review thread IDs and existing comments (enhanced query)
gh api graphql -f query='
{
  repository(owner: "apstndb", name: "spanner-mycli") {
    pullRequest(number: <PR_NUMBER>) {
      reviewThreads(first: 10) {
        nodes {
          id
          line
          path
          isResolved
          subjectType
          comments(first: 10) {
            nodes {
              id
              body
              author {
                login
              }
              createdAt
            }
          }
        }
      }
      reviews(first: 10) {
        nodes {
          id
          author {
            login
          }
          state
          submittedAt
        }
      }
    }
  }
}' | jq '
  .data.repository.pullRequest |
  {
    "unresolved_threads": [
      .reviewThreads.nodes[] | 
      select(.isResolved == false) |
      {
        "thread_id": .id,
        "location": "\(.path):\(.line)",
        "subject_type": .subjectType,
        "latest_comment": .comments.nodes[-1].body[0:100] + "...",
        "needs_reply": (.comments.nodes | map(.author.login) | unique | contains(["apstndb"]) | not)
      }
    ],
    "available_review_ids": [
      .reviews.nodes[] |
      {
        "review_id": .id,
        "author": .author.login,
        "state": .state
      }
    ]
  }
'

# Step 2: Create GraphQL mutation file (recommended approach - omit pullRequestReviewId)
cat > /tmp/reply_mutation.graphql << 'EOF'
mutation {
  addPullRequestReviewThreadReply(input: {
    pullRequestReviewThreadId: "PRRT_kwDONC6gMM5SU-GH"
    body: "Your detailed response here"
  }) {
    comment {
      id
      url
      body
    }
  }
}
EOF

# Step 3: Execute the mutation
gh api graphql -F query=@/tmp/reply_mutation.graphql
```

**Method 2: General PR Comment (Simpler, but not thread-specific)**

```bash
gh pr comment <PR_NUMBER> --body "## Review Comment Responses

**Issue 1 (file.go:123):** Response to specific comment...
**Issue 2 (other.go:456):** Response to another comment..."
```

**Enhanced Features Using GraphQL Schema:**
- **Filtered Output**: Only shows unresolved threads that need replies
- **Smart Detection**: Identifies threads where you haven't replied yet
- **Context Information**: Shows file location, subject type, and comment previews
- **Available Review IDs**: Lists all review IDs you can use for replies

**Schema Exploration and Documentation:**

**Official Documentation:**
- [GitHub GraphQL API Documentation](https://docs.github.com/en/graphql) - Complete reference for all types, queries, and mutations
- [GraphQL Explorer](https://docs.github.com/en/graphql/overview/explorer) - Interactive query builder and schema browser

**Schema Introspection with gh CLI:**
```bash
# Discover available fields for any GraphQL type
gh api graphql -f query='{ __type(name: "PullRequestReviewThread") { fields { name description } } }'

# Find all input fields for mutations
gh api graphql -f query='{ __type(name: "AddPullRequestReviewThreadReplyInput") { inputFields { name description type { name } } } }'

# List all available mutations
gh api graphql -f query='{ __schema { mutationType { fields { name description } } } }' | jq '.data.__schema.mutationType.fields[] | select(.name | contains("pullRequest") or contains("review"))'

# Get full schema documentation for a specific type
gh api graphql -f query='{ __type(name: "PullRequest") { description fields { name description type { name kind } } } }'
```

**Useful Schema Query Patterns:**
```bash
# Find all review-related types
gh api graphql -f query='{ __schema { types { name } } }' | jq '.data.__schema.types[].name' | grep -i review

# Check required vs optional fields for mutations
gh api graphql -f query='{ __type(name: "AddPullRequestReviewThreadReplyInput") { inputFields { name type { kind name } } } }' | jq '.data.__type.inputFields[] | {name, required: (.type.kind == "NON_NULL")}'
```

**Reply with Mentions (Recommended for AI reviews):**
```bash
# Include @mention for acknowledgment and notification
cat > /tmp/reply_mention.graphql << 'EOF'
mutation {
  addPullRequestReviewThreadReply(input: {
    pullRequestReviewThreadId: "PRRT_kwDONC6gMM5SVHTH"
    pullRequestReviewId: "PRR_kwDONC6gMM6uvWPT"
    body: "@gemini-code-assist Thank you for the suggestion! I've addressed your feedback by [specific change made]."
  }) {
    comment { id url }
  }
}
EOF

gh api graphql -F query=@/tmp/reply_mention.graphql
```

**Quick Helper Script Template:**
```bash
#!/bin/bash
# scripts/dev/review-reply-helper.sh
PR_NUMBER=$1
THREAD_ID=$2
REVIEW_ID=$3
REPLY_TEXT="$4"
MENTION_USER="$5"  # Optional: @username for mention

if [ $# -lt 4 ]; then
    echo "Usage: $0 <pr_number> <thread_id> <review_id> <reply_text> [mention_user]"
    exit 1
fi

# Add mention if provided
if [ -n "$MENTION_USER" ]; then
    REPLY_TEXT="@$MENTION_USER $REPLY_TEXT"
fi

cat > /tmp/reply_mutation.graphql << EOF
mutation {
  addPullRequestReviewThreadReply(input: {
    pullRequestReviewThreadId: "$THREAD_ID"
    pullRequestReviewId: "$REVIEW_ID"
    body: "$REPLY_TEXT"
  }) {
    comment { id url }
  }
}
EOF

gh api graphql -F query=@/tmp/reply_mutation.graphql | jq '.data.addPullRequestReviewThreadReply.comment'
```

**Key Points for GraphQL Approach:**
- **Documentation First**: Always check [GitHub GraphQL API docs](https://docs.github.com/en/graphql) for official field descriptions and examples
- **Schema Introspection**: Use `gh api graphql` with `__type` and `__schema` queries to explore available fields dynamically
- **Interactive Development**: Use [GraphQL Explorer](https://docs.github.com/en/graphql/overview/explorer) for building and testing queries interactively
- **Required vs Optional Fields**: Use schema introspection to identify required fields (`NON_NULL` type kind)
- **pullRequestReviewId**: **IMPORTANT** - This field is optional and often causes failures when included. Omit it for simple thread replies
- **File-based Mutations**: Create mutation files to avoid shell escaping issues with complex text
- **Validation**: Always validate GraphQL queries against schema before implementing in scripts

**Common Issues and Solutions:**
- **Mutation returns null comment**: Often caused by including optional `pullRequestReviewId` field
- **Permission errors**: Ensure you have write access to the repository
- **Thread not found**: Verify thread ID is correct and thread still exists

**Common GraphQL API Patterns:**
```bash
# Rate limit checking
gh api graphql -f query='{ rateLimit { cost remaining resetAt } }'

# Node ID resolution (useful for debugging)
gh api graphql -f query='{ node(id: "PRRT_kwDONC6gMM5SU-GH") { __typename } }'

# Pagination with cursors (for large result sets)
gh api graphql -f query='{ repository(owner: "apstndb", name: "spanner-mycli") { pullRequests(first: 5, after: "cursor") { pageInfo { hasNextPage endCursor } } } }'
```

### Review Comment Response Template

```markdown
@reviewer Thank you for the code review! I've addressed your suggestion about [specific topic].

## Changes Made

✅ **[Change description]**: [Brief explanation of what was done]

✅ **[Additional improvement]**: [Any additional improvements made]

The improvement provides [benefit description], which is better than the previous approach because [reasoning].

Commit: [commit-hash]
```

### AI Code Review Integration

For automated reviews using Gemini Code Assist:

```bash
# Trigger re-review after addressing comments
gh pr comment <PR-number> --body "/gemini review"
```

## Knowledge Management

### Benefits of PR Comment Approach

- **Searchable**: GitHub search finds insights across all PRs
- **Contextual**: Insights linked directly to implementation
- **Persistent**: No worktree cleanup affects knowledge retention
- **Collaborative**: Team members can see and build on insights

### Knowledge Management Evolution

- **Improved Workflow**: PR comments provide persistent, searchable, contextual knowledge capture
- **Best Practice**: Add development insights as PR comments after implementation for future reference

## Git Practices

### Important Git Practices

- **CRITICAL**: Never push directly to main branch - always use Pull Requests
- Always use `git add <specific-files>` instead of `git add .`
- Check `git status` before committing to verify only intended files are staged
- Use `git show --name-only` to review committed files

### Linking Issues and Pull Requests

Use GitHub's issue linking syntax in commit messages and PR descriptions:

- **Supported Keywords**: `Closes`, `Fixes`, `Resolves` (and variations)
- Include links in both commit messages and PR descriptions for automatic closure

### Commit Message Format

```
type(scope): brief description

Detailed explanation of changes if needed.

Fixes #123
```

Examples:
```
feat(timeout): add --timeout flag for gcloud compatibility

Implement STATEMENT_TIMEOUT system variable with duration validation
and pointer type design. Apply timeouts consistently across all query
types while preserving 24h default for partitioned DML operations.

Fixes #276
```

## Related Documentation

- [Development Insights](development-insights.md) - Development patterns and workflows
- [Architecture Guide](architecture-guide.md) - System architecture and components
- [System Variable Patterns](patterns/system-variables.md) - Implementation patterns