---
allowed-tools: Bash, mcp__github__issue_read, mcp__github__list_pull_requests, mcp__github__pull_request_read, mcp__github__add_issue_comment, mcp__github__merge_pull_request
description: Complete PR workflow for an issue including squash merge and cleanup
---

## Context

You are completing the PR workflow for issue #$ARGUMENTS in the spanner-mycli repository.

## Your task

1. Find the PR associated with issue #$ARGUMENTS
2. Check if a Gemini summary exists after the last commit:
   - Use `gh pr view <PR> --comments` to check for recent summary
   - Look for a comment starting with "## Summary of Changes" that was posted after the last commit
   - If no recent summary exists, request one by commenting `/gemini summary`
3. Wait for reviews and checks to complete using `go tool gh-helper reviews wait <PR>`
4. Squash merge the PR with a descriptive commit message that includes:
   - Clear summary of changes
   - Reference to the issue being fixed
5. Clean up the phantom worktree for this issue if it exists

Important notes:
- Only request a new Gemini summary if one doesn't exist after the last commit push
- Use the squash merge method as enforced by the repository ruleset
- Include a meaningful commit message that describes the changes made in the PR