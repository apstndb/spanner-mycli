---
allowed-tools: Bash, mcp__github__issue_read, mcp__github__list_pull_requests, mcp__github__pull_request_read, mcp__github__add_issue_comment, mcp__github__merge_pull_request
description: Complete PR workflow for an issue including squash merge and cleanup
---

## Context

You are completing the PR workflow for #$ARGUMENTS in the spanner-mycli repository.
`#$ARGUMENTS` may be either an issue number or a PR number — check which one it is first.

## Your task

1. Identify the PR: if #$ARGUMENTS is a PR, use it directly; if it is an issue, find the PR associated with it
2. Wait for CI checks to complete using `go tool gh-helper reviews wait <PR> --exclude-reviews` — passing checks are the merge gate
3. Check for unresolved review threads with `go tool gh-helper reviews fetch <PR> --unresolved-only`; if any exist, address and resolve them before merging. Do not wait for new reviews to arrive.
4. Squash merge the PR with a descriptive commit message that includes:
   - Clear summary of changes
   - Reference to the issue being fixed (if any)
5. Clean up the phantom worktree for this issue if it exists

Important notes:
- **CI checks are the merge gate.** Gemini review is best-effort until its sunset on **2026-07-17** and unavailable after (issue #693): if a review already arrived, address its feedback; never wait for one or request one (no `--request-review`, no `--request-summary`, no `/gemini` comments).
- Use the squash merge method as enforced by the repository ruleset
- Include a meaningful commit message that describes the changes made in the PR
