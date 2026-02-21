---
name: Release Prep
description: Analyze and prepare a milestone for release
arguments: "<milestone>"
---

# Release Preparation

Prepare release for milestone **$ARGUMENTS**.

## Steps

1. Analyze PRs for proper release notes categorization:
```bash
go tool gh-helper releases analyze --milestone $ARGUMENTS
```

2. Review the analysis output for:
   - PRs missing classification labels (`bug`, `enhancement`, `breaking-change`)
   - PRs that should have `ignore-for-release` label (dev-docs only changes)
   - Inconsistent labeling patterns

3. Fix missing labels based on analysis:
```bash
# Add classification labels to unlabeled PRs
go tool gh-helper labels add enhancement --items <PR_NUMBERS>

# Mark dev-docs-only PRs
go tool gh-helper labels add ignore-for-release --items <PR_NUMBERS>
```

4. Re-analyze to verify all PRs are properly labeled:
```bash
go tool gh-helper releases analyze --milestone $ARGUMENTS
```

5. Generate draft release notes:
```bash
gh release create $ARGUMENTS --generate-notes --draft
```

6. Review the draft release notes and report any issues found.

**Label → Release notes mapping:**
- `breaking-change` → "Breaking Changes"
- `bug` → "Bug Fixes"
- `enhancement` → "New Features"
- `ignore-for-release` → Excluded
- Other labels → "Misc"
