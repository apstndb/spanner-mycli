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

**Method 1: GraphQL Mutation (Recommended for direct thread replies)**

```bash
# Step 1: Get review thread IDs and existing comments
gh api graphql -f query='
{
  repository(owner: "apstndb", name: "spanner-mycli") {
    pullRequest(number: <PR_NUMBER>) {
      reviewThreads(first: 10) {
        nodes {
          id
          line
          path
          comments(first: 10) {
            nodes {
              body
              author {
                login
              }
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
        }
      }
    }
  }
}'

# Step 2: Create GraphQL mutation file
cat > /tmp/reply_mutation.graphql << 'EOF'
mutation {
  addPullRequestReviewThreadReply(input: {
    pullRequestReviewThreadId: "PRRT_kwDONC6gMM5SU-GH"
    pullRequestReviewId: "PRR_kwDONC6gMM6uvWPT"
    body: "Your detailed response here"
  }) {
    comment {
      id
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

**Key Points for GraphQL Approach:**
- Use existing review ID from the PR reviews list (not a pending review)
- Thread IDs and review IDs can be found via GraphQL query
- Include sufficient `first: N` limit to get all comments/threads/reviews
- Create mutation file to avoid shell escaping issues with complex text
- Use `-F query=@file.graphql` for file input to avoid quote escaping

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