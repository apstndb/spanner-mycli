# gh-helper

Generic GitHub operations tool optimized for AI assistants.

## Quick Start

```bash
# Most common: Complete review workflow
gh-helper reviews wait <PR> --request-review

# Handle review feedback
gh-helper threads list <PR>
gh-helper threads reply <THREAD_ID> --message "Fixed in commit abc123"
```

## Design Philosophy

### AI-Friendly Patterns

- **stdin over temp files**: `echo "reply" | gh-helper threads reply THREAD_ID`
- **Default timeouts**: 5 minutes based on real Gemini/CI performance data
- **Both reviews + checks**: Default behavior covers 95% of real workflows
- **Self-documenting**: Comprehensive `--help` at every level

### Why Both Reviews AND Checks by Default?

Based on real development workflows, developers typically need:
1. Review feedback (Gemini Code Assist, human reviewers)
2. CI status (tests, linting, security checks)

Making this the default reduces cognitive load. Individual exclusion available for edge cases.

## Commands Overview

### reviews

**Default behavior**: Wait for both reviews AND PR checks

```bash
# Complete workflow (recommended)
reviews wait <PR> --request-review

# Edge cases
reviews wait <PR> --exclude-checks    # Reviews only
reviews wait <PR> --exclude-reviews   # Checks only

# Monitoring
reviews check <PR>                    # One-time check with state tracking
```

### threads

**Purpose**: Handle review thread conversations

```bash
# List unresolved threads
threads list <PR>

# Show detailed thread context
threads show <THREAD_ID>

# Reply to thread (AI-friendly stdin support)
threads reply <THREAD_ID> --message "text"
echo "multi-line reply" | threads reply <THREAD_ID>

# Reply with commit reference (best practice)
threads reply-commit <THREAD_ID> <COMMIT_HASH> --message "Fixed as suggested"
```

## State Management

Review state tracking in `~/.cache/spanner-mycli-reviews/`:
- Enables incremental monitoring (only new reviews detected)
- Prevents API rate limiting
- Survives tool restarts

File format: `pr-{number}-last-review.json`
```json
{
  "id": "PRR_kwDONC6gMM6vB1Fv",
  "createdAt": "2025-06-17T17:20:47Z"
}
```

## Performance and Timeouts

| Operation | Typical Time | Default Timeout |
|-----------|-------------|----------------|
| Gemini Review | 1-3 minutes | 5 minutes |
| CI Checks | 2-5 minutes | 5 minutes |
| Combined | 3-6 minutes | 5 minutes |

**Historical note**: Originally 15 minutes, reduced based on 95th percentile real-world data.

## GraphQL API Usage

### Efficient Queries

The tool uses optimized GraphQL queries:

**Combined review + checks** (most common):
```graphql
{
  repository(owner: "owner", name: "repo") {
    pullRequest(number: 123) {
      reviews(last: 15) { nodes { id author { login } createdAt state } }
      commits(last: 1) {
        nodes {
          commit {
            statusCheckRollup { state }
          }
        }
      }
    }
  }
}
```

**Reviews only** (with `--exclude-checks`):
```graphql
{
  repository(owner: "owner", name: "repo") {
    pullRequest(number: 123) {
      reviews(last: 15) { nodes { id author { login } createdAt state } }
    }
  }
}
```

### Rate Limiting

- Single query per 30-second poll cycle
- State tracking reduces redundant API calls
- Respects GitHub's secondary rate limits

## Error Handling

### Common Scenarios

**Timeout reached**:
```
⏰ Timeout reached (5 minutes).
Status: Reviews: true, Checks: false
```

**Network issues**: Automatic retry with exponential backoff

**Permission errors**: Clear error messages pointing to `gh auth login`

### Recovery Patterns

**Corrupted state**: Automatically recreated on next run

**API rate limits**: Tool waits and retries with appropriate delays

## Integration with spanner-mycli-dev

gh-helper is called by spanner-mycli-dev for:
- `pr-workflow` commands (when `--wait-checks` used)
- `review gemini` commands (for both review waiting and thread listing)

## Examples

### Complete Review Workflow

```bash
# 1. Request Gemini review and wait for both reviews + checks
gh-helper reviews wait 306 --request-review

# 2. If there are review threads, handle them
gh-helper threads list 306

# 3. Show detailed context for a specific thread
gh-helper threads show PRRT_kwDONC6gMM5SU-GH

# 4. Reply with fixes
gh-helper threads reply-commit PRRT_kwDONC6gMM5SU-GH abc1234 \
  --message "Fixed the error handling as suggested"

# 5. Request follow-up review if needed
gh pr comment 306 --body "/gemini review"
```

### AI Assistant Usage

```bash
# AI can run autonomously
gh-helper reviews wait 306 --request-review --timeout 8

# AI can provide complex responses via stdin
gh-helper threads reply PRRT_kwDONC6gMM5SU-GH <<EOF
Thank you for the thorough review!

I've addressed all the points:
- Fixed the memory leak in commit abc123
- Added error handling for edge cases
- Updated tests to cover new scenarios

The implementation now handles all the cases you mentioned.
EOF
```

## Migration from Shell Scripts

| Old Script | New Command |
|------------|-------------|
| `scripts/dev/list-review-threads.sh` | `gh-helper threads list` |
| `scripts/dev/review-reply.sh` | `gh-helper threads reply` |
| Custom review waiting scripts | `gh-helper reviews wait` |

## Configuration

### Environment Variables

- `DEBUG=1`: Enable verbose output
- `GH_TOKEN`: GitHub token (usually handled by `gh` CLI)

### Global Flags

- `--owner`: Repository owner (default: apstndb)
- `--repo`: Repository name (default: spanner-mycli)

## Related Tools

- **spanner-mycli-dev**: Project-specific workflows that use gh-helper
- **gh CLI**: Underlying GitHub CLI tool for authentication and basic operations
- **phantom**: Worktree management (used by spanner-mycli-dev)