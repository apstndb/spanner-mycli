---
name: Review Respond
description: Reply to addressed review threads with commit hash and resolve
arguments: "[commit_message]"
---

# Respond to Review Threads

After addressing review feedback, please:

1. Identify the correct commit hash for each fix:
**IMPORTANT**: The commit hash should refer to the specific commit where the issue was fixed, which may NOT be HEAD.
- Use `git log --oneline -10` to review recent commits
- For each thread, identify which commit actually addressed that specific feedback
- If unsure, use `git log --grep="keyword"` or `git show <hash>` to verify the fix

2. Find unresolved, non-outdated review threads and classify each one:
!gh pr-review review view "$(gh pr view --json number --jq .number)" --unresolved --not_outdated -R apstndb/spanner-mycli

Reply to and resolve each addressed actionable or informational thread. Do not
mechanically resolve outdated or unrelated threads; inspect them only when the
user asks or when they still describe a live problem.

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
PR="$(gh pr view --json number --jq .number)"

# Code fix — explain what was changed
gh pr-review comments reply "$PR" --thread-id THREAD_ID \
  --body "Addressed in abc123: removed the redundant nil check because ListVariables() performs first-use initialization." \
  -R apstndb/spanner-mycli
gh pr-review threads resolve "$PR" --thread-id THREAD_ID -R apstndb/spanner-mycli

# Multi-line response for complex fixes
BODY="Addressed in abc123: switched from buffering to streaming output. This prevents memory issues for commands with large output."
gh pr-review comments reply "$PR" --thread-id THREAD_ID --body "$BODY" -R apstndb/spanner-mycli
gh pr-review threads resolve "$PR" --thread-id THREAD_ID -R apstndb/spanner-mycli

# Acknowledge praise comment (no code change)
gh pr-review comments reply "$PR" --thread-id THREAD_ID --body "Thank you!" -R apstndb/spanner-mycli
gh pr-review threads resolve "$PR" --thread-id THREAD_ID -R apstndb/spanner-mycli

# Explanation-only response (no code change)
gh pr-review comments reply "$PR" --thread-id THREAD_ID \
  --body "This is intentional: the regex requires \\s+ after SET to avoid matching bare SET as a variable context." \
  -R apstndb/spanner-mycli
gh pr-review threads resolve "$PR" --thread-id THREAD_ID -R apstndb/spanner-mycli
```

Confirm each reply is visible before resolving its thread.

3. Verify no PENDING review is holding your replies:

A reply can remain invisible if an interrupted operation leaves it in a
*pending* review. Check for leftover pending reviews before treating the
thread workflow as complete (GitHub only lists your own):

!PR=$(gh pr view --json number -q .number) && gh api --paginate "repos/{owner}/{repo}/pulls/$PR/reviews" --jq '[.[] | select(.state=="PENDING")] | {pendingReviews: map({id, html_url})}'

If `pendingReviews` is non-empty, inspect each review's drafted comments, then submit it so the replies become visible:

```bash
# inspect what would be published first
gh api "repos/{owner}/{repo}/pulls/$PR/reviews/<REVIEW_ID>/comments" \
  --jq '.[] | {in_reply_to_id, body_head: (.body[0:80])}'
# then submit (publishes the drafted replies; does not change resolution)
gh api -X POST "repos/{owner}/{repo}/pulls/$PR/reviews/<REVIEW_ID>/events" -f event=COMMENT
```

Proceed only once `pendingReviews` is empty (`[]`).

4. After all threads are resolved, wait for CI checks on the pushed fixes — they are the merge gate:
!gh pr checks --required --watch --fail-fast

Consumer Gemini Code Assist review is unavailable (issue #693). Do not wait
for or request a GitHub bot review. Obtain independent review evidence for the
exact current head through an available local or delegated route.
