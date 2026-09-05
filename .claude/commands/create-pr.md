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
gh pr checks --required --watch --fail-fast
```

6. Inventory unresolved, non-outdated review threads, verify the exact head and merge state, and address any actionable threads with `/review-cycle`:
```bash
PR="$(gh pr view --json number --jq .number)"
gh pr-review review view "$PR" --unresolved --not_outdated -R apstndb/spanner-mycli
gh pr view "$PR" --json headRefOid,mergeable,mergeStateStatus,state
```

Consumer Gemini Code Assist review is unavailable (issue #693). Do not wait
for or request a GitHub bot review. Obtain independent review evidence for the
exact current head through an available local or delegated route.

**Important**: Use `--body-file` or heredoc for PR body content with special characters. Never pass backtick-containing strings directly in shell commands.
