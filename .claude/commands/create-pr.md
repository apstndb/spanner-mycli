---
name: Create PR
description: Create a pull request with structured description and insights
arguments: "[issue-number]"
---

# Create Pull Request

Create a PR for the current branch, linking to issue #$ARGUMENTS if provided.

## Steps

1. Verify readiness:
```bash
make check
```

2. Check current branch status:
```bash
git status
git log --oneline origin/main..HEAD
```

3. Create the PR using `gh pr create` with a structured body:

**Title format**: `type(scope): brief description` (e.g., `feat(timeout): add statement timeout support`)

**Body structure**:
```markdown
## Summary
Brief description of what this PR does and why.

## Key Changes
- **file.go**: What changed and why
- **other_file.go**: What changed and why

## Development Insights
(Optional - include if discoveries were made during implementation)

### Discoveries
- Pattern/architecture/testing insights worth preserving

### AGENTS.md Integration Candidates
- Patterns or rules to add to project docs

## Test Plan
- [ ] `make check` passes
- [ ] Manual testing completed (if applicable)

Fixes #ISSUE_NUMBER
```

4. Apply appropriate labels for release notes categorization:
   - `bug` → "Bug Fixes" section
   - `enhancement` → "New Features" section
   - `breaking-change` → "Breaking Changes" section
   - `ignore-for-release` → excluded (dev-docs only PRs)

   Inherit labels from linked issues when possible:
   ```bash
   go tool gh-helper labels add-from-issues --pr <PR_NUMBER>
   ```

5. Wait for CI checks — they are the merge gate:
```bash
go tool gh-helper reviews wait --exclude-reviews --timeout 15m
```

6. Check once (non-blocking) whether review feedback arrived, and address any unresolved threads with `/review-cycle`:
```bash
go tool gh-helper reviews fetch --unresolved-only
```
Gemini review is best-effort until 2026-07-17 and unavailable after (issue #693) — if none arrived, do not wait for one or request one.

**Important**: Use `--body-file` or heredoc for PR body content with special characters. Never pass backtick-containing strings directly in shell commands.
