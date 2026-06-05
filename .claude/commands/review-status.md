---
name: Review Status
description: Check current PR review and merge status
---

# Check Review Status

Please show me:

1. Current review threads status:
!go tool gh-helper reviews fetch --unresolved-only

2. PR merge status:
!gh pr status --json "number,title,state,mergeable,statusCheckRollup"

3. Leftover unsubmitted (PENDING) reviews — these hide drafted replies until submitted, so threads can look resolved with no visible reply (GitHub only lists your own):
!PR=$(gh pr view --json number -q .number) && gh api --paginate "repos/{owner}/{repo}/pulls/$PR/reviews" --jq '[.[] | select(.state=="PENDING")] | {pendingReviews: map({id, html_url})}'

4. Summary of what needs to be done before merging.