---
allowed-tools: Bash, mcp__github__issue_read, mcp__Phantom__phantom_create_worktree
description: Start working on a GitHub issue by fetching latest state and creating a worktree
arguments: "<issue number>"
---

## Context

You are starting work on issue #$ARGUMENTS in the spanner-mycli repository.

## Your task

1. Fetch the latest state from origin:
   ```bash
   git fetch origin
   ```

2. Read the issue details to understand the task:
   - Use `mcp__github__issue_read` with method `get` for issue #$ARGUMENTS
   - Also check sub-issues: `go tool gh-helper issues show $ARGUMENTS --include-sub`

3. Derive a descriptive worktree name from the issue title:
   - Format: `issue-{N}-{slug}` (e.g., `issue-483-read-lock-mode`)
   - Slug: lowercase, hyphens only, strip conventional prefixes (`feat:`, `fix:`, `chore:`, etc.)
   - Keep it concise (3-5 meaningful words max)

4. Create the worktree:
   - Use `mcp__Phantom__phantom_create_worktree` with `name=issue-{N}-{slug}` and `baseBranch=origin/main`

5. Report:
   - The issue title and summary
   - The worktree path and branch name created
   - Key implementation points to address
