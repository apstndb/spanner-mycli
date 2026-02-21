---
name: Review Respond
description: Reply to all review threads with commit hash and resolve
arguments: "[commit-message]"
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

For each thread ID found above, reply with the CORRECT commit hash where that specific issue was fixed and resolve it, regardless of whether it's marked as outdated.

**Response strategy per thread type:**

- **Code fix needed**: Make the fix, commit, then reply with commit hash and resolve
- **Explanation only** (no code change needed): Reply with reasoning why current code is correct, then resolve
- **Praise/positive comment**: Acknowledge briefly (e.g., "Thank you!") and resolve — don't leave these unresolved

When replying to threads, include a brief explanation of the fix:
- Use `--message "Brief explanation of what was fixed"` for single-line responses
- For multi-line responses, use stdin with heredoc
- Default format: "Fixed in [commit-hash] - [brief explanation]$ARGUMENTS"

Examples:
```bash
# Single-line response
go tool gh-helper threads reply THREAD_ID --commit-hash abc123 --message "Fixed by making CLI_SKIP_SYSTEM_COMMAND read-only" --resolve

# Multi-line response for complex fixes
cat <<EOF | go tool gh-helper threads reply THREAD_ID --commit-hash abc123 --resolve
Fixed by switching from buffering to streaming output.
This prevents DoS attacks from commands with large output.
EOF

# Acknowledge praise comment (no code change)
go tool gh-helper threads reply THREAD_ID --message "Thank you!" --resolve

# Explanation-only response (no code change)
go tool gh-helper threads reply THREAD_ID --message "This is intentional because ..." --resolve
```

Note: Even threads marked as "outdated" should be replied to and resolved, as they may contain valuable feedback that was addressed.

3. After all threads are resolved, request a new review:
!go tool gh-helper reviews wait --request-review