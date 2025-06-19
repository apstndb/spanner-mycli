# Development Tools Guide

This document covers the AI-friendly development tools created as part of issue #301 script reorganization.

## Overview

The tools in `dev-tools/` replace scattered shell scripts with structured Go commands optimized for AI assistant usage:

- **gh-helper**: Generic GitHub operations (reviews, threads)
- **Makefile targets**: Project-specific workflows (worktrees, docs) using simple shell commands

## Design Principles

### AI-Friendly Patterns

Based on extensive AI assistant testing, these patterns work best:

1. **stdin/heredoc over temporary files**: AI assistants can pipe content directly
2. **Self-documenting commands**: Comprehensive `--help` at every level
3. **Predictable timeouts**: Default 5 minutes based on real Gemini/CI performance data
4. **Separable concerns**: Reviews vs checks can be controlled independently
5. **No interactive prompts**: All input via flags or stdin
6. **Unified output formats**: YAML default with JSON support for all structured data

### Default Behavior Philosophy

**Both reviews AND checks by default**: Based on real development workflows, developers typically need both pieces of information before proceeding. Individual exclusion is possible but rare.

```bash
# Most common pattern (new default)
gh-helper reviews wait 306 --request-review

# Edge cases (explicit exclusion)
gh-helper reviews wait 306 --exclude-checks    # Reviews only
gh-helper reviews wait 306 --exclude-reviews   # Checks only
```

## Tool Architecture

### gh-helper (Generic GitHub Operations)

**Purpose**: Reusable GitHub operations that work across projects.

**Key Commands**:
```bash
# Review monitoring with automatic detection
reviews wait <PR> [--request-review] [--exclude-checks|--exclude-reviews]

# Thread management with built-in filtering
reviews fetch <PR> --list-threads        # List thread IDs needing replies (most efficient)
reviews fetch <PR> --threads-only        # JSON output of threads needing replies
reviews fetch <PR> --needs-reply-only    # Include only unresolved threads in full data
threads show <THREAD_ID>
threads reply <THREAD_ID> [--message "text" | via stdin]
threads reply-commit <THREAD_ID> <COMMIT_HASH>
```

**State Management**: 
- Tracks last review state in `~/.cache/spanner-mycli-reviews/`
- Enables incremental monitoring without API spam

### Makefile Targets (Project-Specific)

**Purpose**: Simple spanner-mycli specific workflows using standard tools.

**Key Targets**:
```bash
# Phantom worktree management
make worktree-setup WORKTREE_NAME=issue-123     # Auto-fetch, create, configure
make worktree-list                               # Show existing worktrees
make worktree-delete WORKTREE_NAME=issue-123    # Safe deletion with checks

# Documentation maintenance
make docs-update                                 # Generate README.md help sections
```

**PR Workflows** (use tools directly):
```bash
# Initial PR creation
gh pr create                                     # Interactive title/body input
gh-helper reviews wait --timeout 15m            # Wait for automatic review

# Post-push review cycles  
gh-helper reviews wait <PR> --request-review --timeout 15m  # Request + wait
```

## Performance Data and Timeouts

Data-driven timeout optimization (detailed analysis in `dev-docs/lessons-learned/shell-to-go-migration.md:110-120`):

| Operation | 95th Percentile | Default Timeout |
|-----------|----------------|-----------------|
| Gemini Review | 4 minutes | 5 minutes |
| CI Checks | 6 minutes | 5 minutes |
| Combined Workflow | 6 minutes | 5 minutes |

## Key Features

### Unified Review Analysis 

**Critical advancement**: Single GraphQL query fetches reviews + threads to prevent missing feedback (review bodies often contain architecture concerns not found in inline threads).

```bash
# Comprehensive analysis in one command
gh-helper reviews analyze 306    # Shows all actionable items with severity
```

### GitHub API Optimization

**Optimized GitHub interactions**: 88% reduction in `gh` CLI invocations (17 → 2) through GraphQL consolidation.
Technical details: `dev-docs/lessons-learned/shell-to-go-migration.md#github-api-optimization-strategy`

## Integration Patterns

### Complete Workflow Examples

**New PR Creation**:
```bash
# Interactive PR creation + wait for automatic review
gh pr create
gh-helper reviews wait --timeout 15m

# Alternative: specify title/body via flags
gh pr create --title "feat: new feature" --body "Description"
gh-helper reviews wait --timeout 15m
```

**Post-Push Review Cycle**:
```bash
# Request Gemini review + wait
gh-helper reviews wait 306 --request-review --timeout 15m

# Manual step-by-step approach
gh pr comment 306 --body "/gemini review"
gh-helper reviews wait 306 --timeout 15m
gh-helper reviews fetch 306 --list-threads
gh-helper threads reply-commit THREAD_ID abc1234
```

### Makefile Integration

The tools integrate with existing Makefile targets:

```bash
make build-tools                    # Build both tools
make help-dev                       # Show integration patterns
make worktree-setup WORKTREE_NAME=issue-123  # Legacy wrapper
```

## Error Handling and Edge Cases

### Timeout Behavior

When timeouts occur, tools provide clear status:

```bash
⏰ Timeout reached (5 minutes).
Status: Reviews: true, Checks: false
```

This helps users understand what completed and what didn't.

### State Recovery

Review state tracking handles various edge cases:
- Corrupted state files (recreated automatically)
- Clock skew between local/GitHub (uses GitHub timestamps)
- Concurrent tool usage (last-writer-wins)

### Phantom Worktree Safety

`worktree delete` includes safety checks:
- Uncommitted changes detection
- Untracked files preservation
- `--force` flag for override scenarios

## AI Assistant Usage Patterns

### Autonomous Workflows

Tools are designed for AI assistants to run autonomously:

```bash
# AI can run this and handle all responses
gh-helper reviews wait 306 --request-review --timeout 8

# AI can pipe complex responses
gh-helper threads reply THREAD_ID <<EOF
Thank you for the detailed feedback!

I've addressed the issues:
- Fixed error handling in commit abc123
- Added tests for edge cases
- Updated documentation

The implementation now handles all mentioned scenarios.
EOF
```

### Permission Boundaries

Clear separation between read and write operations:
- `reviews fetch --list-threads/threads show`: Read-only, safe for autonomous use
- `threads reply`: Write operation, AI should confirm with user
- `reviews wait`: Read-only monitoring, safe for autonomous use

## Troubleshooting

### Common Issues

**Tool not found**:
```bash
# Build tools first
make build-tools

# Or use explicit paths
./bin/gh-helper reviews wait 306
```

**Permission errors**:
```bash
# Ensure gh CLI is authenticated
gh auth status
gh auth login
```

**State corruption**:
```bash
# Clear review state
rm -rf ~/.cache/spanner-mycli-reviews/
```

### Debug Information

All tools support verbose output via environment:
```bash
# Enable debug output
export DEBUG=1
gh-helper reviews wait 306
```

## Unified Output Format System

### Architecture Overview

All dev-tools implement a **unified output format system** that eliminates inconsistencies and provides AI-friendly structured data:

```go
// shared/output_format.go - Central format handling
type OutputFormat string

const (
    FormatYAML OutputFormat = "yaml"  // Default for AI tools
    FormatJSON OutputFormat = "json"  // Programmatic integration
)

// Format-specific marshaling
func (f OutputFormat) Marshal(data interface{}) ([]byte, error)

// Format-agnostic unmarshaling  
func Unmarshal(data []byte, v interface{}) error
```

### Key Design Decisions

**YAML as Default**: Chosen over JSON for AI tool integration because:
- More readable for humans and AI assistants
- Superset of JSON (compatible with both processing tools)
- Direct `gojq --yaml-input` support without conversion
- Cleaner diff output in version control

**GitHub GraphQL API Compliance**: All field names match GitHub's GraphQL API exactly:
```yaml
# Example output structure
number: 306
title: "PR Title"
isResolved: true          # Not "resolved"
createdAt: "2025-06-19"   # Not "created_at"
pullRequest:              # Nested structure matches API
  number: 306
  title: "Title"
```

### Implementation Benefits

1. **Zero Redundancy**: Eliminated 288 lines of duplicate output handling
2. **Type Safety**: `OutputFormat.Marshal()` prevents format-specific bugs
3. **Extensibility**: New formats can be added without touching existing code
4. **Consistency**: All commands use identical output patterns

### Usage Examples

```bash
# Default YAML output for AI processing
gh-helper reviews analyze 306 | gojq --yaml-input '.criticalItems[]'

# JSON for traditional tools  
gh-helper reviews analyze 306 --json | jq '.summary.critical'

# Thread analysis with structured data
gh-helper threads show THREAD_ID --format yaml | gojq --yaml-input '.comments.nodes[]'
```

### Processing Patterns

**AI Tool Integration**:
```bash
# Extract all high-priority items
gh-helper reviews analyze 306 | gojq --yaml-input '.highPriorityItems[] | .summary'

# Count critical issues by type
gh-helper reviews analyze 306 | gojq --yaml-input '[.criticalItems[] | .type] | group_by(.) | map({type: .[0], count: length})'

# Get threads needing replies with location
gh-helper reviews fetch 306 | gojq --yaml-input '.reviewThreads.needingReply[] | {id, path, line}'
```

**CI/CD Pipeline Integration**:
```bash
# Check if PR is ready for merge (no critical issues)
CRITICAL_COUNT=$(gh-helper reviews analyze 306 | gojq --yaml-input '.summary.critical')
if [ "$CRITICAL_COUNT" -eq 0 ]; then
  echo "PR ready for merge"
fi
```

## Future Enhancements

### Planned Features

1. **Timestamp Output**: Show when reviews/checks completed
2. **Webhook Integration**: Real-time notifications instead of polling
3. **Parallel Review Handling**: Multiple PRs simultaneously
4. **Custom Check Definitions**: Beyond GitHub's statusCheckRollup

### Extensibility

Tools are designed for extension:
- Plugin architecture for custom commands
- Configuration file support for defaults
- Integration with other CI systems

## Related Documentation

- [Shell-to-Go Migration](lessons-learned/shell-to-go-migration.md) - Technical architecture and optimization details
- [Issue Management](issue-management.md) - GitHub workflow integration
- [Development Insights](development-insights.md) - General development patterns