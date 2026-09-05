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
3. Check unresolved, non-outdated review threads with `gh pr-review review view <PR> --unresolved --not_outdated -R apstndb/spanner-mycli`; address actionable feedback, publish a meaningful reply, and resolve each addressed thread before merging
4. Verify the exact head and merge state with `gh pr view <PR> --json headRefOid,mergeable,mergeStateStatus,state`, and confirm independent review evidence covers that head
5. Squash merge the PR with a descriptive commit message that includes:
   - Clear summary of changes
   - Reference to the issue being fixed (if any)
6. Report any related phantom worktree as a cleanup candidate; do not delete it without explicit user permission

Important notes:
- **CI checks are the merge gate.** Consumer Gemini Code Assist review is unavailable (issue #693); never wait for or request it. Do not request Copilot review for this repository.
- Review threads and independent current-head review are separate evidence from CI checks
- Use the squash merge method as enforced by the repository ruleset
- Include a meaningful commit message that describes the changes made in the PR
