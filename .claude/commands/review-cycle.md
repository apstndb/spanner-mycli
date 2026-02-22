---
name: Review Cycle
description: Wait for Gemini review and check feedback
---

# Review Cycle Management

Please execute the following steps:

1. Wait for Gemini code review:
!go tool gh-helper reviews wait

**IMPORTANT**: `/gemini summary` and `/gemini review` are DIFFERENT commands.
- `/gemini summary` generates a "Summary of Changes" — this is posted **automatically** on PR creation. Never request it manually.
- `/gemini review` triggers an **inline code review** — this is also automatic but may take several minutes for large PRs.
- If the wait times out without a review, **wait longer or request `/gemini review`** (NOT `/gemini summary`).

2. Check for unresolved threads:
!go tool gh-helper reviews fetch --unresolved-only

3. If there are unresolved threads, please help me address the feedback and then use `/project:review-respond` to reply to all threads.