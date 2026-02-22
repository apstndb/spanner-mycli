---
name: Review Respond
description: Reply to all review threads with commit hash and resolve
arguments: "[commit_message]"
---

# Respond to Review Threads

After addressing review feedback, please:

1. Identify the correct commit hash for each fix:
**IMPORTANT**: The commit hash should refer to the specific commit where the issue was fixed, which may NOT be HEAD.
- Use `git log --oneline -10` to review recent commits
- For each thread, identify which commit actually addressed that specific feedback
- If unsure, use `git log --grep="keyword"` or `git show <hash>` to verify the fix

2. Find all unresolved threads (including outdated ones) and respond to each one:
!go tool gh-helper reviews fetch --unresolved-only

For each thread ID found above, reply and resolve it, regardless of whether it's marked as outdated.

**Reply content guidelines — always write a meaningful reply:**
- Do NOT just post a commit hash. Explain what was changed and why.
- For code fixes: Describe the specific change made to address the feedback.
- For explanations: Provide concrete reasoning, not just "this is intentional."
- Keep it concise but substantive: 1-3 sentences is ideal.

**Response strategy per thread type:**

- **Code fix needed**: Make the fix, commit, then reply with commit hash and explanation, and resolve
- **Explanation only** (no code change needed): Reply with reasoning why current code is correct, then resolve
- **Praise/positive comment**: Acknowledge briefly (e.g., "Thank you!") and resolve — don't leave these unresolved

Examples:
```bash
# Code fix — explain what was changed
go tool gh-helper threads reply THREAD_ID --commit-hash abc123 --resolve \
  --message "Removed the redundant nil check. ListVariables() calls ensureRegistry() internally, so the explicit guard was preventing first-use initialization."

# Multi-line response for complex fixes
cat <<EOF | go tool gh-helper threads reply THREAD_ID --commit-hash abc123 --resolve
Switched from buffering to streaming output.
This prevents memory issues from commands with large output.
EOF

# Acknowledge praise comment (no code change)
go tool gh-helper threads reply THREAD_ID --message "Thank you!" --resolve

# Explanation-only response (no code change)
go tool gh-helper threads reply THREAD_ID --resolve \
  --message "This is intentional: the regex requires \\s+ after SET to avoid matching bare SET as a variable context."
```

Note: Even threads marked as "outdated" should be replied to and resolved, as they may contain valuable feedback that was addressed.

3. After all threads are resolved, request a new review:
!go tool gh-helper reviews wait --request-review
