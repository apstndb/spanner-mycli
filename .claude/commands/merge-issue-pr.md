---
allowed-tools: Bash, mcp__github__issue_read, mcp__github__list_pull_requests, mcp__github__pull_request_read, mcp__github__add_issue_comment, mcp__github__merge_pull_request
description: Complete PR workflow for an issue including squash merge and cleanup
---

## Context

You are completing the PR workflow for #$ARGUMENTS in the spanner-mycli repository.
`#$ARGUMENTS` may be either an issue number or a PR number — check which one it is first.

## Your task

1. Identify the PR: if #$ARGUMENTS is a PR, use it directly; if it is an issue, find the PR associated with it
2. Wait for required CI checks with `gh pr checks <PR> --required --watch --fail-fast` — passing checks are the merge gate
3. Set `PR` to the selected pull request number and preserve it while inventorying every unresolved review thread with `gh pr-review threads list "$PR" --unresolved -R apstndb/spanner-mycli`, including outdated threads; retrieve the complete inline comment context for every listed ID with the procedure in `dev-docs/issue-management.md`, also read all review bodies and states with paginated `gh api`, then address actionable feedback, publish a meaningful reply, and resolve each addressed thread before merging
4. Record `REVIEWED_HEAD` from `gh pr view "$PR" --json headRefOid,mergeable,mergeStateStatus,state,reviewDecision`, and confirm independent review evidence covers that exact head
5. Squash merge with `gh pr merge "$PR" --squash --match-head-commit "$REVIEWED_HEAD"` and a descriptive commit message that includes:
   - Clear summary of changes
   - Reference to the issue being fixed (if any)
6. Report any related phantom worktree as a cleanup candidate; do not delete it without explicit user permission

Important notes:
- **CI checks are the merge gate.** Consumer Gemini Code Assist review is unavailable (issue #693); never wait for or request it. Do not request Copilot review for this repository.
- Review threads and independent current-head review are separate evidence from CI checks
- Use the squash merge method as enforced by the repository ruleset
- Include a meaningful commit message that describes the changes made in the PR
