# Development Tools Guide

This document covers the AI-friendly development tools created as part of issue #301 script reorganization.

## Overview

The tools in `dev-tools/` replace scattered shell scripts with structured Go commands optimized for AI assistant usage:

- **gh-helper**: Generic GitHub operations (reviews, threads)
- **spanner-mycli-dev**: Project-specific workflows (worktrees, docs, Gemini integration)

## Design Principles

### AI-Friendly Patterns

Based on extensive AI assistant testing, these patterns work best:

1. **stdin/heredoc over temporary files**: AI assistants can pipe content directly
2. **Self-documenting commands**: Comprehensive `--help` at every level
3. **Predictable timeouts**: Default 5 minutes based on real Gemini/CI performance data
4. **Separable concerns**: Reviews vs checks can be controlled independently
5. **No interactive prompts**: All input via flags or stdin

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

### spanner-mycli-dev (Project-Specific)

**Purpose**: spanner-mycli specific workflows and integrations.

**Key Commands**:
```bash
# Complete PR workflows
pr-workflow create [--wait-checks]     # Create + wait for initial review
pr-workflow review <PR> [--wait-checks] # Handle post-push review cycle

# Phantom worktree management
worktree setup <NAME>                  # Auto-fetch, create, configure
worktree list                          # Show existing worktrees
worktree delete <NAME> [--force]       # Safe deletion with checks

# Documentation maintenance
docs update-help                       # Generate README.md help sections

# Smart review workflows
review gemini <PR> [--force-request] [--wait-checks]  # Auto-detection
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

**Significant code reduction**: 88% fewer lines of code through Go implementation vs shell scripts.
Technical details: `dev-docs/lessons-learned/shell-to-go-migration.md#github-api-optimization-strategy`

## Integration Patterns

### Complete Workflow Examples

**New PR Creation**:
```bash
# All-in-one approach
spanner-mycli-dev pr-workflow create --wait-checks

# Step-by-step approach  
gh pr create --title "feat: new feature" --body "Description"
gh-helper reviews wait --request-review
```

**Post-Push Review Cycle**:
```bash
# After pushing fixes
spanner-mycli-dev pr-workflow review 306 --wait-checks

# Manual approach
gh-helper reviews wait 306 --request-review --exclude-checks
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