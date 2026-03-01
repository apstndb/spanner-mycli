---
name: Release Prep
description: Analyze and prepare a new release by pushing a tag (goreleaser creates the release)
arguments: "[version] (e.g. v0.27.0, optional - auto-increments minor if omitted)"
---

# Release Preparation

Prepare a release. Target version: **$ARGUMENTS** (if empty, auto-increment minor version from latest release).

## Steps

1. Determine the last release tag and target version:
```bash
git fetch origin main
LAST_TAG=$(gh api repos/apstndb/spanner-mycli/releases/latest --jq '.tag_name')
echo "Last release: ${LAST_TAG}"
```

If `$ARGUMENTS` is empty, compute the next minor version by incrementing the minor component of `LAST_TAG` (e.g., v0.26.0 → v0.27.0). Confirm with the user before proceeding.

2. Derive PR range from git log between the last tag and origin/main:
```bash
# Extract PR numbers from squash-merge commit messages
git log ${LAST_TAG}..origin/main --oneline
```

Parse PR numbers from the output (format: `... (#NNN)`). Use the first and last PR numbers to form the range.

3. Analyze PRs for label classification:
```bash
go tool gh-helper releases analyze --pr-range ${FIRST_PR}-${LAST_PR}
```

4. Review the analysis output for:
   - PRs missing classification labels (`bug`, `enhancement`, `breaking-change`)
   - PRs that should have `ignore-for-release` label (dev-docs only, test-only, chore changes)
   - Inconsistent labeling patterns

5. Fix missing labels based on analysis:
```bash
# Add classification labels to unlabeled PRs
go tool gh-helper labels add enhancement --items <PR_NUMBERS>

# Mark dev-docs-only or test-only PRs
go tool gh-helper labels add ignore-for-release --items <PR_NUMBERS>
```

6. Re-analyze to verify all PRs are properly labeled:
```bash
go tool gh-helper releases analyze --pr-range ${FIRST_PR}-${LAST_PR}
# Confirm: readyForRelease: true
```

7. Confirm with the user, then push the release tag to origin/main HEAD (goreleaser creates the draft release automatically):
```bash
git fetch origin main
git tag <VERSION> origin/main
git push origin <VERSION>
```

8. Wait for the goreleaser workflow to complete and review the draft release:
```bash
gh run watch --repo apstndb/spanner-mycli
gh release view <VERSION> --repo apstndb/spanner-mycli
```

9. Enhance the release notes before publishing:
   - For spanemuboost bump PRs, look up the emulator version and add it as a sub-bullet:
     ```bash
     # Replace <BOOST_VERSION> with the spanemuboost version from the PR
     gh api "repos/apstndb/spanemuboost/contents/spanemuboost.go?ref=<BOOST_VERSION>" -q '.content' | base64 -d | grep DefaultEmulatorImage
     ```
   - For major new features, add an actual output example as a sub-bullet
   - Update the release notes via `gh release edit` with `--notes-file`

10. Publish the release:
```bash
gh release edit <VERSION> --repo apstndb/spanner-mycli --draft=false
gh release view <VERSION> --repo apstndb/spanner-mycli --json isDraft,url
```

**Label → Release notes mapping:**
- `breaking-change` → "Breaking Changes"
- `bug` → "Bug Fixes"
- `enhancement` → "New Features"
- `ignore-for-release` → Excluded
- Other labels → "Misc"
