---
name: Review Cycle
description: Wait for Gemini review and check feedback
---

# Review Cycle Management

Please execute the following steps:

1. Wait for Gemini review (request if needed):
!go tool gh-helper reviews wait --request-review

2. Check for unresolved threads:
!go tool gh-helper reviews fetch --unresolved-only

3. If there are unresolved threads, please help me address the feedback and then use `/project:review-respond` to reply to all threads.