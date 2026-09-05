---
name: Review Status
description: Check current PR review and merge status
---

# Check Review Status

Please show me:

1. Current review threads status:
!gh pr-review review view "$(gh pr view --json number --jq .number)" --unresolved --not_outdated -R apstndb/spanner-mycli

2. Exact-head PR merge and check status:
!gh pr view --json number,title,state,headRefOid,mergeable,mergeStateStatus,statusCheckRollup

3. Leftover unsubmitted (PENDING) reviews — these hide drafted replies until submitted, so threads can look resolved with no visible reply (GitHub only lists your own):
!PR=$(gh pr view --json number -q .number) && gh api --paginate "repos/{owner}/{repo}/pulls/$PR/reviews" --jq '[.[] | select(.state=="PENDING")] | {pendingReviews: map({id, html_url})}'

4. Summary of what needs to be done before merging. Required CI checks are the merge gate; actionable unresolved threads must be addressed, and independent review evidence must cover the exact current head. Consumer Gemini Code Assist review is unavailable (issue #693), and Copilot review must not be requested for this repository.
