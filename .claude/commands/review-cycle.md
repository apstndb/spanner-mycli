---
name: Review Cycle
description: Wait for CI checks and address any review feedback
---

# Review Cycle Management

Please execute the following steps:

1. Wait for CI checks to complete — they are the merge gate:
!gh pr checks --required --watch --fail-fast

Consumer Gemini Code Assist review is unavailable (issue #693). Never wait for
or request a GitHub bot review. Review threads and independent current-head
review are separate from the CI gate.

2. Record the hosted head and review decision, inventory every unresolved
thread (including outdated threads), retrieve the complete inline comment
context for every listed thread ID with the procedure in
[Complete Thread Inventory and Context](../../dev-docs/issue-management.md#complete-thread-inventory-and-context),
and read review-level bodies and states:
!PR=$(gh pr view --json number --jq .number) && gh pr-review threads list "$PR" --unresolved -R apstndb/spanner-mycli
!PR=$(gh pr view --json number --jq .number) && gh api --paginate "repos/{owner}/{repo}/pulls/$PR/reviews" --jq '.[] | {id, user: .user.login, state, submitted_at, body}'
!gh pr view --json headRefOid,mergeable,mergeStateStatus,state,reviewDecision

3. Inspect every unresolved thread and every review body/state before deciding
whether feedback is actionable. Editing a commented line commonly makes its
thread outdated; outdated does not mean addressed. If there is no actionable
feedback, continue to step 7 rather than declaring the cycle complete.

4. Address actionable feedback:

For each unresolved thread, evaluate the feedback and choose a response strategy:

- **Code fix needed**: Make the fix in code, but defer the reply and resolution until after step 5.
- **Explanation only** (no code change needed): Reply with reasoning why current code is correct, resolve, and move to the next thread.
- **Praise/positive comment**: Acknowledge briefly (e.g., "Thank you!") and resolve.

**Reply content guidelines — always write a meaningful reply:**
- Do NOT just post a commit hash. Explain what was changed and why.
- For code fixes: Describe the specific change made to address the feedback (e.g., "Removed the redundant nil check — `ListVariables()` calls `ensureRegistry()` internally, so the explicit guard was unnecessary and could prevent first-use initialization.")
- For explanations: Provide concrete reasoning, not just "this is intentional."
- Keep it concise but substantive: 1-3 sentences is ideal.

Thread reply examples:
```bash
PR="$(gh pr view --json number --jq .number)"

# Explanation only — provide reasoning
gh pr-review comments reply "$PR" --thread-id THREAD_ID \
  --body "This is intentional: the regex requires \\s+ after SET to avoid matching bare SET as a variable context." \
  -R apstndb/spanner-mycli
gh pr-review threads resolve "$PR" --thread-id THREAD_ID -R apstndb/spanner-mycli

# Acknowledge praise
gh pr-review comments reply "$PR" --thread-id THREAD_ID --body "Thank you!" -R apstndb/spanner-mycli
gh pr-review threads resolve "$PR" --thread-id THREAD_ID -R apstndb/spanner-mycli
```

5. After making code changes, run the relevant validation and commit. Push only
when the user has authorized that separate boundary.

6. Reply to each code-fix thread with the verified commit hash and explanation,
confirm the reply is published, then resolve it. Make the block self-contained
and replace the example revision with the specific commit for that thread:
```bash
PR="$(gh pr view --json number --jq .number)"
FIX_COMMIT=abc123
FIX_SHA="$(git rev-parse "$FIX_COMMIT^{commit}")"
REMOTE_HEAD="$(gh pr view "$PR" --json headRefOid --jq .headRefOid)"
git merge-base --is-ancestor "$FIX_SHA" "$REMOTE_HEAD"

gh pr-review comments reply "$PR" --thread-id THREAD_ID \
  --body "Addressed in $FIX_SHA: removed the redundant nil check because ListVariables() performs first-use initialization." \
  -R apstndb/spanner-mycli
gh pr-review threads resolve "$PR" --thread-id THREAD_ID -R apstndb/spanner-mycli
```
If the ancestry check fails, the fixing commit is not published in the PR;
stop before replying or resolving.
Then wait for CI checks on the hosted head:
!gh pr checks --required --watch --fail-fast

7. Repeat step 2 and confirm there are no remaining actionable threads or
review-level blockers and that the recorded hosted head is still current.

8. Obtain independent review evidence for that exact hosted head through an
available local or delegated route. Repeat from step 2 after any change. Report
the cycle clean only when required checks are green, review feedback is clear,
and the exact current head has independent review evidence.
