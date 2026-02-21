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

3. Summary of what needs to be done before merging.