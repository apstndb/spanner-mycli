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

2. Check unresolved, non-outdated threads:
!gh pr-review review view "$(gh pr view --json number --jq .number)" --unresolved --not_outdated -R apstndb/spanner-mycli

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
PR="$(gh pr view --json number --jq .number)"

# Code fix — explain what was changed
gh pr-review comments reply "$PR" --thread-id THREAD_ID \
  --body "Addressed in abc123: removed the redundant nil check because ListVariables() performs first-use initialization." \
  -R apstndb/spanner-mycli
gh pr-review threads resolve "$PR" --thread-id THREAD_ID -R apstndb/spanner-mycli

# Explanation only — provide reasoning
gh pr-review comments reply "$PR" --thread-id THREAD_ID \
  --body "This is intentional: the regex requires \\s+ after SET to avoid matching bare SET as a variable context." \
  -R apstndb/spanner-mycli
gh pr-review threads resolve "$PR" --thread-id THREAD_ID -R apstndb/spanner-mycli

# Acknowledge praise
gh pr-review comments reply "$PR" --thread-id THREAD_ID --body "Thank you!" -R apstndb/spanner-mycli
gh pr-review threads resolve "$PR" --thread-id THREAD_ID -R apstndb/spanner-mycli
```

5. After addressing all threads with code changes, commit and push the fixes.

6. With the new commit hash, reply to each code-fix thread, confirm the reply is published, resolve it, then wait for CI checks on the new commits:
!gh pr checks --required --watch --fail-fast

7. Obtain independent review evidence for the exact current head through an available local or delegated route. Repeat from step 2 after any code change until there are no actionable threads, checks are green, and the current head is reviewed.
