# gh-helper

Generic GitHub operations tool optimized for AI assistants.

## Quick Start

```bash
# Most common: Complete review workflow
gh-helper reviews wait [PR] --request-review

# Handle review feedback
gh-helper reviews fetch [PR] --list-threads
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

**PR Number Resolution**: `[PR]` is optional in all commands. When omitted, uses current branch's associated PR. Supports plain numbers (123) and explicit PR references (pull/123, pr/123). Issue references are not supported.

### reviews

**Default behavior**: Wait for both reviews AND PR checks

```bash
# Complete workflow (recommended)
reviews wait [PR] --request-review

# Edge cases
reviews wait [PR] --exclude-checks    # Reviews only
reviews wait [PR] --exclude-reviews   # Checks only

# Monitoring and checking
reviews check [PR]                    # One-time check (uses current branch if omitted)
```

### threads

**Purpose**: Handle review thread conversations

```bash
# List thread IDs needing replies (most efficient)
reviews fetch [PR] --list-threads

# Get full thread data needing replies (JSON)
reviews fetch [PR] --threads-only

# Filter all data to only threads needing replies
reviews fetch [PR] --needs-reply-only

# Show detailed thread context
threads show <THREAD_ID>

# Reply to thread (AI-friendly stdin support)
threads reply <THREAD_ID> --message "text"
echo "multi-line reply" | threads reply <THREAD_ID>

# Reply with commit reference (best practice)
threads reply <THREAD_ID> --commit-hash abc123 --message "Fixed as suggested"
threads reply <THREAD_ID> --commit-hash abc123  # Uses default message
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

# 2. Or just check current status without waiting
gh-helper reviews check 306

# 3. If there are review threads, handle them
gh-helper reviews fetch 306 --list-threads

# 4. Show detailed context for a specific thread
gh-helper threads show PRRT_kwDONC6gMM5SU-GH

# 5. Reply with fixes
gh-helper threads reply PRRT_kwDONC6gMM5SU-GH --commit-hash abc1234 \
  --message "Fixed the error handling as suggested"

# 6. Request follow-up review if needed
gh pr comment 306 --body "/gemini review"
```

### AI Assistant Usage

```bash
# AI can run autonomously with structured output
gh-helper reviews wait 306 --request-review --timeout 8

# AI can analyze results programmatically
action_needed=$(gh-helper reviews analyze 306 | gojq --yaml-input '.action_required')
critical_count=$(gh-helper reviews analyze 306 | gojq --yaml-input '.summary.critical')

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
| `scripts/dev/list-review-threads.sh` | `gh-helper reviews fetch [PR] --list-threads` |
| `scripts/dev/review-reply.sh` | `gh-helper threads reply` |
| Custom review waiting scripts | `gh-helper reviews wait` |

## Output Format and Programmatic Usage

### Structured Output Modes

All commands default to YAML output with JSON available via `--json`:

```bash
# YAML output (default)
gh-helper reviews analyze 306

# JSON output
gh-helper reviews analyze 306 --json

# Thread-focused outputs
gh-helper reviews fetch 306 --list-threads        # Thread IDs only (one per line)
gh-helper reviews fetch 306 --threads-only        # Threads needing replies (YAML/JSON)
```

### YAML Output Example

```yaml
# gh-helper reviews analyze 306
pr_number: 306
title: "feat: reorganize scripts into AI-friendly development tools"
state: OPEN
mergeable: true
action_required: true
critical_items:
  - id: PRR_kwDONC6gMM6vI3tk
    author: gemini-code-assist
    location: General Review
    summary: "Review requires attention: ## Code Review..."
    type: review
high_priority_items:
  - id: PRRC_kwDONC6gMM6AZCa3
    author: gemini-code-assist
    location: dev-tools/shared/commands.go
    summary: "![critical] The initializeDefaults function..."
    type: review_comment
summary:
  critical: 3
  high: 11
  info: 2
  threads_need_reply: 0
  threads_unresolved: 25
```

### Processing with gojq

```bash
# Get critical items only
gh-helper reviews analyze 306 | gojq --yaml-input '.critical_items[].id'

# Check if action is required
if [[ $(gh-helper reviews analyze 306 | gojq --yaml-input '.action_required') == "true" ]]; then
  echo "Action needed!"
fi

# Count threads needing reply
gh-helper reviews analyze 306 | gojq --yaml-input '.summary.threads_need_reply'

# Get PR state
gh-helper reviews fetch 306 | gojq --yaml-input '.pr.state'

# Filter high priority items by author
gh-helper reviews analyze 306 | gojq --yaml-input '.high_priority_items[] | select(.author == "gemini-code-assist")'

# Get all locations with issues
gh-helper reviews analyze 306 | gojq --yaml-input '[.critical_items[], .high_priority_items[]] | .[].location' | sort -u
```

### Output Structure

All outputs are derived from GitHub's GraphQL API with additional processing:

**analyze command** - Comprehensive analysis:
- **Source**: GraphQL query fetching reviews + threads with full metadata
- **Processing**: Simplified 3-level severity detection (CRITICAL/HIGH/INFO) using Gemini markers only
- **Structure**:
  - `pr_number`, `title`, `state`, `mergeable`: Basic PR metadata
  - `action_required`: Boolean indicating if any action is needed
  - `critical_items`, `high_priority_items`, `info_items`: Items grouped by severity
  - `threads_needing_reply`: Threads requiring responses
  - `summary`: Computed statistics

**fetch command modes**:
- Default: Complete unified data (reviews + threads + PR metadata)
- `--threads-only`: Filtered subset showing only threads needing replies
- `--list-threads`: Plain text IDs (one per line, for shell processing)

### Programmatic Usage Examples

```bash
# Get thread IDs for automated replies
for thread_id in $(gh-helper reviews fetch 306 --list-threads 2>/dev/null); do
  echo "Processing thread: $thread_id"
done

# Check for critical issues
critical_count=$(gh-helper reviews analyze 306 | gojq --yaml-input '.summary.critical')
if [ "$critical_count" -gt 0 ]; then
  echo "⚠️ Critical issues found: $critical_count"
fi

# List all files with issues
gh-helper reviews analyze 306 | gojq --yaml-input '
  [.critical_items[], .high_priority_items[], .info_items[]] 
  | map(.location) 
  | unique 
  | map(select(. != "General Review"))
  | .[]
'

# Generate report for CI
gh-helper reviews analyze 306 | gojq --yaml-input '
  {
    pr: .pr_number,
    action_required: .action_required,
    critical: .summary.critical,
    high: .summary.high,
    status: (if .action_required then "FAILED" else "PASSED" end)
  }
'
```

## Shell Completion

Shell completion is available via standard Cobra `completion` command for bash, zsh, fish, and PowerShell.

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
