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

3. If there are no unresolved threads, report that the review cycle is clean and stop here.

4. If there are unresolved threads, address each one:

For each unresolved thread, evaluate the feedback and choose a response strategy:

- **Code fix needed**: Make the fix in code, then continue to step 5.
- **Explanation only** (no code change needed): Reply with reasoning why current code is correct, resolve, and move to the next thread.
- **Praise/positive comment**: Acknowledge briefly (e.g., "Thank you!") and resolve.

**Reply content guidelines — always write a meaningful reply:**
- Do NOT just post a commit hash. Explain what was changed and why.
- For code fixes: Describe the specific change made to address the feedback (e.g., "Removed the redundant nil check — `ListVariables()` calls `ensureRegistry()` internally, so the explicit guard was unnecessary and could prevent first-use initialization.")
- For explanations: Provide concrete reasoning, not just "this is intentional."
- Keep it concise but substantive: 1-3 sentences is ideal.

Thread reply examples:
```bash
# Code fix — explain what was changed
go tool gh-helper threads reply THREAD_ID --commit-hash abc123 --resolve \
  --message "Removed the redundant nil check. ListVariables() calls ensureRegistry() internally, so the explicit guard was preventing first-use initialization."

# Explanation only — provide reasoning
go tool gh-helper threads reply THREAD_ID --resolve \
  --message "This is intentional: the regex requires \\s+ after SET to avoid matching bare SET as a variable context, which would conflict with other SET usages."

# Acknowledge praise
go tool gh-helper threads reply THREAD_ID --message "Thank you!" --resolve
```

5. After addressing all threads with code changes, commit and push the fixes.

6. With the new commit hash, reply to and resolve all code-fix threads, then request a new review to re-validate:
!go tool gh-helper reviews wait --request-review

7. Repeat from step 2 until there are no unresolved threads.
