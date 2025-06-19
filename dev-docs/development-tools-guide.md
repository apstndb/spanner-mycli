# Development Tools Usage Guide

This document covers how to use development tools in spanner-mycli.

## Overview

spanner-mycli uses Go 1.24's tool management system for development tools:

- **gh-helper**: GitHub operations tool (managed via go.mod tool directive)
- **Makefile targets**: Project-specific workflows (worktrees, docs)

## Tool Installation

```bash
# Install all development tools
make build-tools

# This runs: go install tool
# Creates symlinks in bin/ directory for convenience
```

## gh-helper Usage

### Basic Commands

```bash
# Review operations
go tool gh-helper reviews analyze <PR>     # Comprehensive review analysis
go tool gh-helper reviews wait <PR>        # Wait for reviews and checks
go tool gh-helper reviews fetch <PR>       # Get review data

# Thread operations  
go tool gh-helper threads show <THREAD_ID>
go tool gh-helper threads reply <THREAD_ID>
```

### Common Workflows

**PR Review Workflow:**
```bash
# 1. Create PR and wait for automatic Gemini review (initial PR only)
gh pr create
go tool gh-helper reviews wait --timeout 15m

# 2. For subsequent pushes: ALWAYS request Gemini review
git add . && git commit -m "fix: address feedback" && git push
go tool gh-helper reviews wait <PR> --request-review --timeout 15m
```

**IMPORTANT**: Gemini automatically reviews initial PR creation. For any pushes after PR creation, you MUST use `--request-review` flag to trigger Gemini review.

**Thread Reply Workflow:**
```bash
# List threads needing replies
go tool gh-helper reviews fetch <PR> --list-threads

# Reply to specific thread
go tool gh-helper threads reply <THREAD_ID> --message "Fixed as suggested"

# Reply with commit reference
go tool gh-helper threads reply <THREAD_ID> --commit-hash abc123 --message "Addressed in commit"
```

### Output Formats

```bash
# Default YAML output (AI-friendly)
go tool gh-helper reviews analyze 306 | gojq --yaml-input '.summary.critical'

# JSON output for scripting
go tool gh-helper reviews analyze 306 --json | jq '.summary.critical'
```

## Makefile Workflows

**Phantom Worktree Management:**
```bash
# Create new worktree
make worktree-setup WORKTREE_NAME=issue-123-feature

# List existing worktrees
make worktree-list

# Delete worktree
make worktree-delete WORKTREE_NAME=issue-123-feature
```

**Documentation:**
```bash
# Update README.md help sections
make docs-update
```

## Quick Reference

### Development Cycle
```bash
make build-tools                    # Install tools
make check                          # Run tests and lint (required before push)
go tool gh-helper reviews wait      # Monitor PR reviews
```

### Convenience Symlinks

After running `make build-tools`, you can also use:
```bash
bin/gh-helper reviews analyze 306   # Same as: go tool gh-helper reviews analyze 306
```

## Integration with GitHub Workflows

**Complete PR Workflow:**
```bash
# 1. Create PR (Gemini automatically reviews initial creation)
gh pr create --title "feat: new feature" --body "Description"

# 2. Wait for automatic Gemini review (initial PR only)
go tool gh-helper reviews wait --timeout 15m

# 3. Address feedback and request re-review (REQUIRED for subsequent pushes)
git add . && git commit -m "fix: address review feedback" && git push
go tool gh-helper reviews wait <PR> --request-review --timeout 15m

# 4. Handle thread replies
go tool gh-helper reviews fetch <PR> --list-threads
go tool gh-helper threads reply <THREAD_ID> --commit-hash <HASH> --message "Fixed as suggested"

# 5. Repeat steps 3-4 as needed (always use --request-review after initial PR)
```

**Gemini Review Rules:**
- ✅ Initial PR creation: Automatic review (no flag needed)
- ⚠️ All subsequent pushes: MUST use `--request-review` flag
- 🔄 Always wait for review completion before proceeding

## Troubleshooting

**Tool not found:**
```bash
# Install tools first
make build-tools

# Verify installation
go tool gh-helper --help
```

**Permission errors:**
```bash
# Ensure gh CLI is authenticated
gh auth status
gh auth login
```

## Related Documentation

- [Issue Management](issue-management.md) - GitHub workflow and processes
- [Development Insights](development-insights.md) - Development patterns and best practices