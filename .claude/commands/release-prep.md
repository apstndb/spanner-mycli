---
name: Release Prep
description: Analyze and prepare a new release by pushing a tag (goreleaser creates the release)
arguments: "<version> (e.g. v0.26.0)"
---

# Release Preparation

Prepare release **$ARGUMENTS**.

## Steps

1. Fetch latest state and derive PR range to analyze:
```bash
git fetch origin main
LAST_TAG=$(gh api repos/apstndb/spanner-mycli/releases/latest --jq '.tag_name')
PR_NUMS=$(git log ${LAST_TAG}..origin/main --oneline | grep -oE '\(#[0-9]+\)' | grep -oE '[0-9]+' | sort -n)
FIRST_PR=$(echo "$PR_NUMS" | head -1)
LAST_PR=$(echo "$PR_NUMS" | tail -1)
echo "PR range: ${FIRST_PR}-${LAST_PR}"
```

2. Analyze PRs merged since the last release tag:
```bash
go tool gh-helper releases analyze --pr-range ${FIRST_PR}-${LAST_PR}
```

3. Review the analysis output for:
   - PRs missing classification labels (`bug`, `enhancement`, `breaking-change`)
   - PRs that should have `ignore-for-release` label (dev-docs only, test-only changes)
   - Inconsistent labeling patterns

4. Fix missing labels based on analysis:
```bash
# Add classification labels to unlabeled PRs
go tool gh-helper labels add enhancement --items <PR_NUMBERS>

# Mark dev-docs-only or test-only PRs
go tool gh-helper labels add ignore-for-release --items <PR_NUMBERS>
```

5. Re-analyze to verify all PRs are properly labeled:
```bash
go tool gh-helper releases analyze --pr-range ${FIRST_PR}-${LAST_PR}
# Confirm: readyForRelease: true
```

6. Push the release tag to origin/main HEAD (goreleaser will create the draft release automatically):
```bash
git fetch origin main
git tag $ARGUMENTS origin/main
git push origin $ARGUMENTS
```

7. Wait for the goreleaser workflow to complete and review the draft release:
```bash
gh run watch --repo apstndb/spanner-mycli
gh release view $ARGUMENTS --repo apstndb/spanner-mycli
```

8. Enhance the release notes before publishing:
   - For spanemuboost bump PRs, look up the emulator version and add it as a sub-bullet:
     ```bash
     # Replace <VERSION> with the spanemuboost version from the PR
     gh api "repos/apstndb/spanemuboost/contents/spanemuboost.go?ref=<VERSION>" -q '.content' | base64 -d | grep DefaultEmulatorImage
     ```
   - For major new features, add an actual output example as a sub-bullet
   - Update the release notes via:
     ```bash
     gh release edit $ARGUMENTS --repo apstndb/spanner-mycli --notes "..."
     ```

9. Publish the release:
```bash
gh release edit $ARGUMENTS --repo apstndb/spanner-mycli --draft=false
gh release view $ARGUMENTS --repo apstndb/spanner-mycli --json isDraft,url
```

**Label → Release notes mapping:**
- `breaking-change` → "Breaking Changes"
- `bug` → "Bug Fixes"
- `enhancement` → "New Features"
- `ignore-for-release` → Excluded
- Other labels → "Misc"
