---
name: Review Status
description: Check current PR review and merge status
---

# Check Review Status

Please show me:

1. Every unresolved review thread, including outdated threads:
!gh pr-review threads list "$(gh pr view --json number --jq .number)" --unresolved -R apstndb/spanner-mycli

2. Review-level bodies and states:
!PR=$(gh pr view --json number -q .number) && gh api --paginate "repos/{owner}/{repo}/pulls/$PR/reviews" --jq '.[] | {id, user: .user.login, state, submitted_at, body}'

3. Exact-head PR merge, review-decision, and check status:
!gh pr view --json number,title,state,headRefOid,mergeable,mergeStateStatus,reviewDecision,statusCheckRollup

4. Leftover unsubmitted (PENDING) reviews — these hide drafted replies until submitted, so threads can look resolved with no visible reply (GitHub only lists your own):
!PR=$(gh pr view --json number -q .number) && gh api --paginate "repos/{owner}/{repo}/pulls/$PR/reviews" --jq '[.[] | select(.state=="PENDING")] | {pendingReviews: map({id, html_url})}'

5. Summary of what needs to be done before merging. Required CI checks are the merge gate; actionable thread or review-level feedback must be addressed, and independent review evidence must cover the exact current head. Consumer Gemini Code Assist review is unavailable (issue #693), and Copilot review must not be requested for this repository.
