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
# For a new installation:
gh extension install Agynio/gh-pr-review --pin v1.6.2
gh extension list
```

If it is already installed, obtain local tool-update authority, then replace
it to establish the exact pin; `install --force` upgrades to latest and drops
the requested pin:

```bash
gh extension remove pr-review
gh extension install Agynio/gh-pr-review --pin v1.6.2
gh extension list
```

Confirm that `gh extension list` reports `gh pr-review` at v1.6.2.

```bash
# PR checks and exact merge state (native gh)
gh pr checks <PR> --required --watch --fail-fast
gh pr view <PR> --json headRefOid,mergeable,mergeStateStatus,state,reviewDecision

# Review thread operations (gh pr-review v1.6.2)
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

### Complete Thread Inventory and Context

`gh pr-review threads list` is the authoritative paginated inventory, but it
does not include inline comment bodies. `gh pr-review review view` includes
body context but is capped at the first 100 reviews, threads, and comments in
v1.6.2. Therefore, fetch every listed unresolved thread ID directly and
paginate its comments before classifying feedback:

```bash
set -euo pipefail
: "${PR:=$(gh pr view --json number --jq .number)}"
THREADS_JSON="$(gh pr-review threads list "$PR" --unresolved -R apstndb/spanner-mycli)"
printf '%s\n' "$THREADS_JSON"

while IFS= read -r THREAD_ID; do
  gh api graphql --paginate -F threadId="$THREAD_ID" -f query='query($threadId: ID!, $endCursor: String) {
    node(id: $threadId) {
      ... on PullRequestReviewThread {
        id isResolved isOutdated path line
        comments(first: 100, after: $endCursor) {
          nodes { id databaseId author { login } body createdAt url }
          pageInfo { hasNextPage endCursor }
        }
      }
    }
  }' --jq '.data.node'
done < <(jq -r '.[].threadId' <<<"$THREADS_JSON")
```

Treat the IDs in `THREADS_JSON` as the coverage checklist. Do not declare the
review clean until full context was retrieved for every ID and review-level
bodies/states were inspected separately.

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

# 3. Inventory every unresolved review thread, including outdated threads
gh pr-review threads list <PR> --unresolved -R apstndb/spanner-mycli

# 4. Read review-level bodies and states with pagination
gh api --paginate "repos/{owner}/{repo}/pulls/<PR>/reviews" \
  --jq '.[] | {id, user: .user.login, state, submitted_at, body}'

# 5. Verify the exact head, review decision, and merge state
gh pr view <PR> --json headRefOid,mergeable,mergeStateStatus,state,reviewDecision

# 6. After additional commits: obtain push authorization, push, verify the fixing commit is contained in
#    the hosted head, then repeat checks, thread and review-body inventory,
#    exact-head merge-state inspection, and independent current-head review
set -euo pipefail
git push
PR="$(gh pr view --json number --jq .number)"
FIX_COMMIT=abc123
FIX_SHA="$(git rev-parse "$FIX_COMMIT^{commit}")"
REMOTE_HEAD="$(gh pr view "$PR" --json headRefOid --jq .headRefOid)"
git merge-base --is-ancestor "$FIX_SHA" "$REMOTE_HEAD" || {
  echo "Fix commit is not contained in the hosted PR head" >&2
  exit 1
}
gh pr checks "$PR" --required --watch --fail-fast
```

Thread resolution order matters: commit, authorized push, verify the fixing
commit is contained in the hosted PR head, reply with a message that names that
hash, confirm that the reply is published, then resolve with
`gh pr-review threads resolve`. A reply without a pushed commit is not
verifiable. Inventory all unresolved threads before filtering; a fixed thread
commonly becomes outdated. Also read review bodies and states, not just threads
- blockers and severity notes may appear only there.

`gh pr-review comments reply` v1.6.2 accepts `--body` but not `--body-file`.
Use a safely quoted shell variable for multi-line or special-character content.

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
- **Autonomous actions allowed**: local feature-branch operations (add/commit),
  test execution, documentation updates, and reporting clean worktrees as
  cleanup candidates without deleting them. Push is a separate publication
  boundary and always requires explicit user authorization.
- **Best practices**: run `git status` before any destructive operation;
  offer safer alternatives (e.g., commit before deleting); prioritize
  preserving work over convenience.

## Related Documentation

- [Development Insights](development-insights.md) - development workflow notes
- [Architecture Guide](architecture-guide.md) - code map and authoritative doc comments
- [System Variable Patterns](patterns/system-variables.md) - implementation patterns
