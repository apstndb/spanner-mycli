# Issue and Code Review Management

This document covers GitHub workflow, issue management, code review processes,
and development tools usage for spanner-mycli.

> [!NOTE]
> **This tooling is primarily intended for use by AI assistants.** The
> commands and workflows here are designed for programmatic execution. Human
> contributors may work manually; agents should follow them as written.

## gh-helper Command Reference

Tools are managed via the Go tool directive (`go install tool` installs them).

```bash
# Review operations
go tool gh-helper reviews fetch <PR>                     # Fetch review data including threads
go tool gh-helper reviews fetch <PR> --unresolved-only   # Only unresolved threads
go tool gh-helper reviews fetch <PR> --needs-reply-only  # Only threads needing replies
go tool gh-helper reviews fetch <PR> --no-bodies         # Exclude review bodies (lightweight)
go tool gh-helper reviews wait <PR>                      # Wait for reviews and checks
go tool gh-helper reviews wait <PR> --async              # Check once (non-blocking)
go tool gh-helper reviews wait <PR> --exclude-reviews    # Wait for PR checks only

# Thread operations
go tool gh-helper threads show <ID1> <ID2>               # Show threads
go tool gh-helper threads reply <ID> --commit-hash <HASH> --resolve
go tool gh-helper threads resolve <ID1> <ID2> <ID3>      # Batch resolve

# Issue operations
go tool gh-helper issues show <N> --include-sub          # Show issue with sub-issues
go tool gh-helper issues create --parent <P> --title ... # Create sub-issue
go tool gh-helper issues edit <N> --parent <P>           # Link as sub-issue
go tool gh-helper issues edit <N> --unlink-parent        # Remove parent relationship
go tool gh-helper issues edit <N> --parent <P> --overwrite  # Move to different parent
go tool gh-helper issues edit <N> --before <M> | --after <M> | --position first|last

# Label operations (auto-detects PR vs Issue)
go tool gh-helper labels add bug,enhancement --items 254,267
go tool gh-helper labels remove needs-review --items pull/302,issue/301
go tool gh-helper labels add enhancement --title-pattern "^feat:"
go tool gh-helper labels add-from-issues --pr 254        # Inherit labels from closed issues
# Add --dry-run to preview any label operation

# Release notes analysis
go tool gh-helper releases analyze --milestone v0.19.0
go tool gh-helper releases analyze --since 2024-01-01 --until 2024-01-31
```

Use gh-helper for all sub-issue operations; raw GraphQL is only needed for
custom field selections it does not expose. Always verify linkage after
creation with `issues show <parent> --include-sub`.

## Review Workflow

**CI checks are the merge gate.** Consumer Gemini Code Assist code review is
best-effort until its sunset on 2026-07-17 and unavailable after that date
(#693). Until then a review may still arrive automatically on PR creation;
if it does, address the feedback like any other review. Never block on a
Gemini review, extend waits for one, or re-request one (`--request-review`
and `--request-summary` are no longer part of the workflow).

```bash
# 1. Create PR
gh pr create --title "feat: new feature" --body-file body.md

# 2. Merge gate: wait for CI checks to pass
go tool gh-helper reviews wait <PR> --exclude-reviews --timeout 15m

# 3. Best-effort (until 2026-07-17): check once whether review feedback
#    arrived — non-blocking; do not wait or re-request if it did not
go tool gh-helper reviews fetch <PR> --unresolved-only

# 4. After additional commits: push, then re-run the checks gate
git push
go tool gh-helper reviews wait <PR> --exclude-reviews --timeout 15m
```

Thread resolution order matters: commit, push, then reply with the commit
hash and resolve (`threads reply <ID> --commit-hash <HASH> --resolve`). A
reply without a pushed commit is not verifiable. Also read review bodies, not
just threads - severity notes ("critical", "high") may appear only there.

See AGENTS.md for the authoritative merge-gate rules. Never request Copilot
reviews for this repository.

## Issue Management

### Lifecycle

- All fixes go through Pull Requests - never close issues manually.
- Issues are labeled for filtering by agents; most issues carry 2-4 labels.

### Labels

Primary classification (choose one):
`enhancement`, `bug`, `documentation`, `tech-debt`

Functional domain (multiple allowed):
`system variable`, `output-formatting`, `operations`, `postgresql`,
`jdbc-compatibility`, `memefish`, `testing`

Technical characteristics (multiple allowed):
`performance`, `concurrency`, `emulator-related`, `breaking-change`

Work status (choose one; expresses implementation readiness, not business
priority):
- `low hanging fruit` - ready to implement, clear scope
- `design-needed` - requires design work first (should gain clear acceptance
  criteria before implementation)
- `blocked` - blocked by an external dependency (reference it)
- no label - standard complexity

Management:
`umbrella` (parent issue in a parent-child hierarchy), `claude-code`,
`question`, `wontfix`

Documentation:
- `docs-user` - user-facing documentation (README.md, docs/)
- `docs-dev` - developer/internal documentation (dev-docs/, AGENTS.md, CLAUDE.md)

### Issue Planning Guidelines

- DO NOT include time estimates - they are meaningless for planning.
- Ensure phases are independently mergeable - each phase is a complete PR.
- Create system variables for new features - follow existing patterns.
- Reference specific code locations as `file_path:line_number`.
- Use `gh issue create/edit` with a heredoc or `--body-file` for bodies
  containing backticks or other special characters.

## Pull Request Process

### Release Notes Labels

PR labels drive automatic release notes (`.github/release.yml`):

- `breaking-change` - "Breaking Changes" section
- `enhancement` - "New Features" section
- `bug` - "Bug Fixes" section
- `ignore-for-release` - excluded entirely. Use for dev-docs/AGENTS.md and
  internal tooling changes. User-facing documentation (README.md, docs/)
  MUST NOT have this label.

All other labels land in "Misc". For release preparation, use
`/release-prep <milestone>`; for PR creation, `/create-pr`; for review
response, `/review-respond` and `/review-cycle`.

### Creating Pull Requests

- Link PRs to issues with "Fixes #N" in the PR description.
- Apply release-notes labels at creation time.
- Ensure `make check` passes before creating the PR.

## Git Practices

- CRITICAL: never commit or push directly to main - feature branches + PRs only.
- Always `git add <specific-files>`; check `git status` before committing.
- Conflict resolution: `git merge origin/main` (not rebase; squash merge makes
  branch history irrelevant, and merge preserves context).

### Commit Message Format

```
type(scope): brief description

Detailed explanation if needed.

Fixes #123
```

### Phantom Worktree Management

- Create: `make worktree-setup WORKTREE_NAME=issue-123-feature` (fetches and
  bases on `origin/main`)
- Work: `phantom shell issue-123-feature --tmux-horizontal`
- From issue/PR: `phantom github checkout <number>`
- Delete: `phantom delete <name>` - see rules below

Agent permission rules for worktrees and destructive operations:

- **Always request user permission**: any `--force` operation, deleting a
  worktree with uncommitted changes (state what would be lost), history
  rewrites (rebase, amend of pushed commits), branch deletion.
- **Autonomous actions allowed**: deleting a clean worktree whose issue is
  resolved, standard git operations on feature branches (add/commit/push),
  test execution, documentation updates.
- **Best practices**: run `git status` before any destructive operation;
  offer safer alternatives (e.g., commit before deleting); prioritize
  preserving work over convenience.

## Related Documentation

- [Development Insights](development-insights.md) - development workflow notes
- [Architecture Guide](architecture-guide.md) - code map and authoritative doc comments
- [System Variable Patterns](patterns/system-variables.md) - implementation patterns
