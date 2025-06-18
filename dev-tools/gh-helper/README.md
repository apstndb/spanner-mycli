# gh-helper

Generic GitHub operations tool optimized for AI assistants.

## Quick Start

```bash
# Most common: Complete review workflow
gh-helper reviews wait <PR> --request-review

# Handle review feedback
gh-helper reviews fetch <PR> --list-threads
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
# List thread IDs needing replies (most efficient)
reviews fetch <PR> --list-threads

# Get full thread data needing replies (JSON)
reviews fetch <PR> --threads-only

# Filter all data to only threads needing replies
reviews fetch <PR> --needs-reply-only

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

### Claude Code Timeout Configuration

**IMPORTANT**: Claude Code has a default 2-minute timeout that affects all bash commands.

#### Quick Setup for Extended Timeouts

Add to your `~/.claude/settings.json`:
```json
{
  "env": {
    "BASH_MAX_TIMEOUT_MS": "900000",
    "BASH_DEFAULT_TIMEOUT_MS": "900000"
  }
}
```

This extends timeouts to 15 minutes for operations that need longer execution times.

#### Automatic Detection and Safety

gh-helper automatically:
- **Detects** `BASH_MAX_TIMEOUT_MS` and `BASH_DEFAULT_TIMEOUT_MS` environment variables
- **Uses safe 90-second margin** when no environment config is detected
- **Provides guidance** for extending timeouts when needed
- **Shows continuation commands** when timeouts are reduced

#### Environment Variable Behavior

| Environment State | Timeout Behavior | Example Output |
|------------------|------------------|----------------|
| **No variables set** | Uses 90s safety margin | `⚠️ Claude Code has 2-minute timeout (no env config detected). Using 1m30s for safety.` |
| **BASH_MAX_TIMEOUT_MS set** | Respects full configured timeout | `🔧 Claude Code BASH_MAX_TIMEOUT_MS detected: 15m0s` |
| **Requested > Available** | Uses available limit with warning | `⚠️ Requested timeout (20m) exceeds Claude Code limit (15m). Using 15m.` |

#### Based on GitHub Issues Research

This implementation addresses common timeout issues documented in:
- [anthropics/claude-code#1039](https://github.com/anthropics/claude-code/issues/1039): Configurable timeout feature request (BASH_MAX_TIMEOUT_MS implemented)
- [anthropics/claude-code#1216](https://github.com/anthropics/claude-code/issues/1216): Commands timeout at exactly 2m 0.0s (default behavior)
- [anthropics/claude-code#1717](https://github.com/anthropics/claude-code/issues/1717): Environment variable configuration in ~/.claude/settings.json

#### Key Implementation Details

Based on extensive testing and research:

1. **Claude Code auto-retry**: Claude Code automatically retries commands that approach the 2-minute limit
2. **Safe margin strategy**: We use 90-second margin when no env config is detected to avoid relying on auto-retry
3. **Environment variable precedence**: BASH_MAX_TIMEOUT_MS takes precedence for explicit timeout requests
4. **Settings file location**: Project .claude/settings.json should be committed for team consistency

## Critical Implementation Insights

### GitHub statusCheckRollup vs Merge Conflicts

**Key Discovery**: When a PR has merge conflicts (`mergeable: "CONFLICTING"`), GitHub's `statusCheckRollup` field returns `null` instead of check status. This prevents CI workflows from running until conflicts are resolved.

**Implementation Impact**:
- Must check `mergeable` field before interpreting `statusCheckRollup: null` as "no checks required"
- Prevents infinite waiting for CI that will never start
- Provides immediate actionable feedback to users

**GraphQL Query Pattern**:
```graphql
pullRequest(number: $prNumber) {
  mergeable          # CRITICAL: Check this first
  mergeStateStatus   # Additional context (DIRTY, etc.)
  statusCheckRollup { state }  # May be null if conflicts exist
}
```

### Claude Code Timeout Handling

**Key Insight**: Claude Code enforces a hard 2-minute timeout for bash commands, but safety margins are essential.

**Lessons Learned**:
- Use 90 seconds (not 120) for effective timeout to prevent mid-execution termination
- Provide clear continuation instructions when timeout is reduced
- Auto-detect execution environment and adjust behavior accordingly

Reference: [GitHub Issue #1216](https://github.com/anthropics/claude-code/issues/1216)

### Cobra Error Handling for Operational vs Usage Errors

**Problem**: Default Cobra behavior shows confusing usage help on operational errors (like merge conflicts).

**Root Cause**: Cobra assumes all `RunE` errors are user command syntax problems.

**Complete Solution Pattern**:
```go
var rootCmd = &cobra.Command{
    SilenceErrors: true,  // Disable Cobra's automatic error printing
}

var operationalCmd = &cobra.Command{
    SilenceUsage: true,   // No usage help for operational errors
    RunE: func(cmd *cobra.Command, args []string) error {
        if operationalProblem {
            // Show detailed operational guidance
            fmt.Printf("❌ Problem description\n")
            fmt.Printf("💡 Specific solution\n")
            return fmt.Errorf("operational error")  // Clean error for exit code
        }
        return nil
    },
}

func main() {
    if err := rootCmd.Execute(); err != nil {
        os.Exit(1)  // Clean exit - detailed messages already shown
    }
}
```

**Key Insights**:
- `SilenceUsage: true` prevents usage help on operational errors
- `SilenceErrors: true` prevents duplicate error messages
- Detailed guidance in the operational flow, not in generic error handling
- Clean exit codes without redundant messaging

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
gh-helper reviews fetch 306 --list-threads

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
| `scripts/dev/list-review-threads.sh` | `gh-helper reviews fetch <PR> --list-threads` |
| `scripts/dev/review-reply.sh` | `gh-helper threads reply` |
| Custom review waiting scripts | `gh-helper reviews wait` |

## JSON Output and Programmatic Usage

### JSON Output Modes

All commands support `--json` for structured output:

```bash
# Full review and thread data
gh-helper reviews fetch 306 --json

# Analysis with actionable items and severity detection
gh-helper reviews analyze 306 --json

# Thread-focused outputs
gh-helper reviews fetch 306 --list-threads        # Thread IDs only (one per line)
gh-helper reviews fetch 306 --threads-only        # Threads needing replies (JSON array)
```

### JSON Output Structure

All JSON outputs are derived from GitHub's GraphQL API with additional processing for actionable insights.

**analyze command** - Comprehensive analysis combining reviews and threads:
- **Source**: GraphQL query fetching `pullRequest { reviews { nodes { id, author, body, state } } }` and review threads
- **Processing**: Severity detection, keyword extraction, actionable item identification
- **Output structure**:
  - `pr`: Basic PR metadata from GitHub GraphQL
  - `currentUser`: Current authenticated user from GitHub API
  - `summary`: Computed statistics (actionable counts, severity distribution, thread states)
  - `actionableItems`: Extracted from review bodies and comments using severity/keyword analysis
  - `threads`: Full thread data with `needsReply` and `isResolved` computed from comment patterns
  - `reviews`: Raw review data with computed severity and action items

**fetch command modes**:
- `--json`: Complete unified data from single GraphQL query (reviews + threads + PR metadata)
- `--threads-only`: Filtered subset showing only threads where `needsReply=true` and `isResolved=false`
- `--list-threads`: Plain text IDs of threads needing replies (one per line, for shell processing)

### Programmatic Usage

```bash
# Get thread IDs for automated replies
for thread_id in $(gh-helper reviews fetch 306 --list-threads 2>/dev/null); do
  echo "Processing thread: $thread_id"
done

# Check for critical issues
critical_count=$(gh-helper reviews analyze 306 --json | jq '.summary.critical')
if [ "$critical_count" -gt 0 ]; then
  echo "⚠️ Critical issues found: $critical_count"
fi
```

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
- **phantom**: Worktree management (used by spanner-mycli-dev)# Test timestamp: Wed Jun 18 04:17:57 JST 2025
