# Issue and Code Review Management

This document covers GitHub workflow, issue management, code review processes,
and development tools usage for spanner-mycli.

> [!NOTE]
> **This tooling is primarily intended for use by AI assistants.** The
> commands and workflows here are designed for programmatic execution. Human
> contributors may work manually; agents should follow them as written.

## GitHub Tooling Reference

`gh-helper` is managed via the Go tool directive (`go install tool` installs
it). Review threads use the `gh pr-review` extension pinned to the version
validated by this workflow:

```bash
gh extension install Agynio/gh-pr-review --pin v1.6.2
```

```bash
# PR checks and exact merge state (native gh)
gh pr checks <PR> --required --watch --fail-fast
gh pr view <PR> --json headRefOid,mergeable,mergeStateStatus,state

# Review thread operations (gh pr-review v1.6.2)
gh pr-review review view <PR> --unresolved --not_outdated -R apstndb/spanner-mycli
gh pr-review threads list <PR> --unresolved -R apstndb/spanner-mycli
gh pr-review comments reply <PR> --thread-id <ID> --body <TEXT> -R apstndb/spanner-mycli
gh pr-review threads resolve <PR> --thread-id <ID> -R apstndb/spanner-mycli

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

Use gh-helper for sub-issue, label, and release-analysis operations. Use native
`gh` for PR checks and merge state, and `gh pr-review` for review threads. If
the pinned extension is unavailable, fall back to `gh api graphql` for thread
semantics rather than coupling check waiting to a review aggregator. Always
verify issue linkage after creation with `issues show <parent> --include-sub`.

## Review Workflow

**CI checks are the merge gate.** Keep checks, review-thread handling, and
independent code review as separate evidence. Consumer Gemini Code Assist code
review is unavailable (#693), so never wait for or request it. Historical
Gemini-authored threads remain ordinary feedback. Do not request Copilot review
for this repository.

```bash
# 1. Create PR
gh pr create --title "feat: new feature" --body-file body.md

# 2. Merge gate: wait for CI checks to pass
gh pr checks <PR> --required --watch --fail-fast

# 3. Inventory unresolved, non-outdated review threads
gh pr-review review view <PR> --unresolved --not_outdated -R apstndb/spanner-mycli

# 4. Verify the exact head and merge state
gh pr view <PR> --json headRefOid,mergeable,mergeStateStatus,state

# 5. After additional commits: push, then repeat checks, thread inventory,
#    exact-head merge-state inspection, and independent current-head review
git push
gh pr checks <PR> --required --watch --fail-fast
```

Thread resolution order matters: commit, push, reply with a message that names
the fixing commit hash, confirm that the reply is published, then resolve with
`gh pr-review threads resolve`. A reply without a pushed commit is not
verifiable. Also read review bodies, not just threads - severity notes
("critical", "high") may appear only there.

See AGENTS.md for the authoritative merge-gate rules. Obtain independent review
evidence for the exact current head through an available local or delegated
route; this is separate from GitHub's required checks.

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
  rewrites (rebase, amend of pushed commits), branch deletion, and deleting any
  worktree even when it is clean.
- **Autonomous actions allowed**: standard git operations on feature branches
  (add/commit/push), test execution, documentation updates, and reporting clean
  worktrees as cleanup candidates without deleting them.
- **Best practices**: run `git status` before any destructive operation;
  offer safer alternatives (e.g., commit before deleting); prioritize
  preserving work over convenience.

## Related Documentation

- [Development Insights](development-insights.md) - development workflow notes
- [Architecture Guide](architecture-guide.md) - code map and authoritative doc comments
- [System Variable Patterns](patterns/system-variables.md) - implementation patterns
