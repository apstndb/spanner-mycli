# gh-helper Feature Requests: Remaining GraphQL Operations

Based on the latest analysis of spanner-mycli documentation, here are the remaining operations that still require direct GraphQL API calls and could benefit from gh-helper commands.

## 1. Sub-Issue Reordering

### Current GraphQL Required:
```bash
gh api graphql -f query='
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

### Proposed gh-helper Command:
```bash
gh-helper issues reorder-sub <sub-issue> --parent <parent> --after <other-sub-issue>
gh-helper issues reorder-sub <sub-issue> --parent <parent> --before <other-sub-issue>
gh-helper issues reorder-sub <sub-issue> --parent <parent> --position first
gh-helper issues reorder-sub <sub-issue> --parent <parent> --position last
```

### Use Cases:
- Prioritizing sub-tasks within a parent issue
- Organizing implementation order
- Moving completed items to the bottom

## 2. GraphQL Schema Introspection (SOLVED)

### Current Solution: github-schema tool
The `github-schema` tool (installed via `go get -tool`) provides all schema introspection capabilities:

```bash
# Type information
go tool github-schema type PullRequestReviewThread
go tool github-schema type Issue --json

# Mutation information
go tool github-schema mutation addSubIssue
go tool github-schema mutation removeSubIssue

# Schema search
go tool github-schema search "review"
go tool github-schema search "sub.*issue" --json

# Custom queries on schema
go tool github-schema query '.objects[] | select(.name | test("Issue")) | .name'
```

### Previous GraphQL Required:
```bash
# Type information (no longer needed)
gh api graphql -f query='{ __type(name: "PullRequestReviewThread") { fields { name description type { name kind } } } }'

# Mutation information (no longer needed)
gh api graphql -f query='{ __type(name: "AddSubIssueInput") { inputFields { name description type { name kind } } } }'

# Schema search (no longer needed)
gh api graphql -f query='{ __schema { types { name description } } }' | grep -i "review"
```

### No gh-helper Commands Needed
The standalone `github-schema` tool already provides comprehensive schema introspection without requiring GraphQL queries. No additional gh-helper features are needed for this use case.

## 3. Batch Node ID Resolution

### Current GraphQL Required:
```bash
# Batch retrieval for multiple issues
gh api graphql -f query='
{
  repository(owner: "apstndb", name: "spanner-mycli") {
    issue1: issue(number: 318) { id }
    issue2: issue(number: 319) { id }
    issue3: issue(number: 320) { id }
  }
}'
```

### Proposed gh-helper Command:
```bash
gh-helper node-id issue 318 319 320
gh-helper node-id pull 100 101 102
gh-helper node-id mixed issue/318 pull/100 issue/320
```

### Output Format:
```yaml
items:
  - type: Issue
    number: 318
    nodeId: I_kwDONC6gMM5z1234
  - type: Issue
    number: 319
    nodeId: I_kwDONC6gMM5z1235
  - type: Issue
    number: 320
    nodeId: I_kwDONC6gMM5z1236
```

### Use Cases:
- Preparing for GraphQL mutations that require node IDs
- Bulk operations that need multiple IDs
- Migration scripts
- Integration with other tools

## 4. Custom Field Selection

### Current Limitation:
gh-helper commands return predefined field sets. Sometimes users need specific fields not included in the default output.

### Proposed Enhancement:
```bash
# Add --fields flag to existing commands
gh-helper issues show 248 --fields "title,body,createdAt,author.login"
gh-helper reviews fetch 300 --fields "reviews.author,reviews.state,reviews.submittedAt"
```

### Alternative: GraphQL Query Templates
```bash
# Execute custom GraphQL with repository context
gh-helper graphql --template issues-with-labels --vars "numbers=[248,249,250]"
gh-helper graphql --query-file my-query.graphql --vars-file vars.json
```

## 5. Advanced PR Review Thread Operations

### Current GraphQL Required:
Complex thread filtering and analysis beyond what `reviews fetch` provides.

### Proposed Commands:
```bash
# Thread analysis
gh-helper threads analyze <PR> --by-author
gh-helper threads analyze <PR> --by-file
gh-helper threads analyze <PR> --unresolved-only --group-by path

# Thread search
gh-helper threads search <PR> --content "TODO"
gh-helper threads search <PR> --author gemini-code-assist
```

## 6. Repository-Wide Operations

### Proposed Commands:
```bash
# Find all issues with specific sub-issue patterns
gh-helper issues find-parents --min-sub-issues 5
gh-helper issues find-parents --incomplete-only

# Bulk sub-issue analysis
gh-helper issues analyze-subs --parent 248 --export csv
```

## Priority Ranking

1. **High Priority**: Sub-issue reordering (frequent use case, complex GraphQL)
2. **High Priority**: Batch node ID resolution (enables other operations)
3. ~~**Medium Priority**: Schema introspection~~ **SOLVED** by `github-schema` tool
4. **Medium Priority**: Custom field selection (power user feature)
5. **Low Priority**: Advanced thread operations (niche use cases)
6. **Low Priority**: Repository-wide operations (occasional analysis)

## Implementation Notes

- Sub-issue reordering is the most requested feature based on documentation analysis
- Node ID resolution would benefit many other operations
- Schema tools would help developers discover API capabilities
- Custom field selection could be implemented as a general enhancement to existing commands
- Consider caching for schema information since it rarely changes