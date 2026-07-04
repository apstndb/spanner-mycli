---
allowed-tools: Bash, mcp__github__issue_read, mcp__github__list_pull_requests, mcp__github__pull_request_read, mcp__github__add_issue_comment, mcp__github__merge_pull_request
description: Complete PR workflow for an issue including squash merge and cleanup
---

## Context

You are completing the PR workflow for #$ARGUMENTS in the spanner-mycli repository.
`#$ARGUMENTS` may be either an issue number or a PR number — check which one it is first.

## Your task

1. Identify the PR: if #$ARGUMENTS is a PR, use it directly; if it is an issue, find the PR associated with it
2. Wait for reviews and checks to complete using `go tool gh-helper reviews wait <PR>`
3. Squash merge the PR with a descriptive commit message that includes:
   - Clear summary of changes
   - Reference to the issue being fixed (if any)
4. Clean up the phantom worktree for this issue if it exists

Important notes:
- `/gemini summary` is posted automatically on PR creation. Do not request it manually during merge.
- **Gemini sunset (issue #693)**: consumer Gemini Code Assist code review ceases on **2026-07-17**. If `reviews wait` times out with checks green because no review will arrive, proceed on green checks — they are the merge gate until the replacement flow in #693 is decided.
- Use the squash merge method as enforced by the repository ruleset
- Include a meaningful commit message that describes the changes made in the PR
