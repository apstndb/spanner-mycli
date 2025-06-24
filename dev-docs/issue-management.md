# Issue and Code Review Management

This document covers GitHub workflow, issue management, code review processes, and development tools usage for spanner-mycli.

## Development Tools Usage

### Tool Installation

```bash
# Install all development tools using Go 1.24 tool management
make build-tools

# This runs: go install tool
```

### gh-helper Usage

**Basic Commands:**
```bash
# Review operations
go tool gh-helper reviews fetch <PR>       # Fetch review data including threads
go tool gh-helper reviews fetch <PR> --threads-only     # Only fetch thread data (lightweight)
go tool gh-helper reviews fetch <PR> --needs-reply-only # Only threads needing replies
go tool gh-helper reviews fetch <PR> --no-bodies        # Exclude review bodies (lightweight)
go tool gh-helper reviews fetch <PR> --list-threads     # List thread IDs only
go tool gh-helper reviews wait <PR>        # Wait for reviews and checks
go tool gh-helper reviews wait <PR> --async # Check reviews once (non-blocking)
go tool gh-helper reviews wait <PR> --exclude-checks    # Wait for reviews only
go tool gh-helper reviews wait <PR> --exclude-reviews   # Wait for PR checks only

# Thread operations  
go tool gh-helper threads show <THREAD_ID>              # Show single thread
go tool gh-helper threads show <ID1> <ID2> <ID3>       # Show multiple threads
go tool gh-helper threads reply <THREAD_ID>             # Reply to thread
go tool gh-helper threads reply <ID> --commit-hash <HASH> --resolve  # Reply with commit
go tool gh-helper threads resolve <ID1> <ID2> <ID3>    # Batch resolve threads
```

**Gemini Review Workflow:**
```bash
# 1. Create PR (Gemini automatically reviews initial creation)
gh pr create --title "feat: new feature" --body "Description"

# 2. Wait for automatic Gemini review (initial PR only)
go tool gh-helper reviews wait --timeout 15m

# 3. For subsequent pushes: ALWAYS request Gemini review
git add . && git commit -m "fix: address feedback" && git push
go tool gh-helper reviews wait <PR> --request-review --timeout 15m
```

**Gemini Review Rules:**
- ✅ Initial PR creation: Automatic review (no flag needed)
- ✅ All subsequent pushes: MUST use `--request-review` flag
- ✅ Always wait for review completion before proceeding

## Issue Management

### Issue Lifecycle

- Issues managed through GitHub Issues with comprehensive labeling
- All fixes must go through Pull Requests - never close issues manually
- Use systematic labeling approach for effective categorization and filtering

### Label System and Guidelines

spanner-mycli uses a systematic labeling approach to categorize and prioritize issues effectively.

#### **Label Classification Framework**

**🎯 Primary Classification Labels** (Choose One)
- `enhancement` - New features or improvements
- `bug` - Something isn't working
- `documentation` - Documentation improvements
- `tech-debt` - Technical debt and code quality

**🔧 Functional Domain Labels** (Multiple Allowed)
- `system variable` - System variable implementation/modification
- `output-formatting` - Output format and display improvements
- `operations` - Database operations and management features  
- `postgresql` - PostgreSQL dialect support
- `jdbc-compatibility` - Java Spanner JDBC driver compatibility features
- `memefish` - Memefish parser utilization
- `testing` - Testing improvements and coverage

**⚙️ Technical Characteristics** (Multiple Allowed)
- `performance` - Performance-related improvements
- `concurrency` - Thread safety and concurrent access
- `emulator-related` - Cloud Spanner emulator specific issues
- `breaking-change` - Changes that break backward compatibility

**📋 Work Status Labels** (Choose One)
- `design-needed` - Requires architecture/design work before implementation
- `blocked` - Blocked by external dependencies
- `low hanging fruit` - Easy implementation with good value

**🗂️ Issue Management Labels**
- `claude-code` - Issues identified by Claude Code
- `question` - Information requests from third parties
- `wontfix` - Will not be implemented (policy decisions)

**📖 Documentation Labels**
- `docs-user` - User-facing documentation (README.md, docs/)
- `docs-dev` - Developer/internal documentation (dev-docs/, CLAUDE.md)

#### **Labeling Best Practices**

**Multi-Label Examples**:
```
enhancement + system variable + jdbc-compatibility
bug + output-formatting + emulator-related
enhancement + operations + design-needed
testing + postgresql + low hanging fruit
```

**Issue Lifecycle Labeling**:
1. **New Issues**: Start with primary classification
2. **Analysis Phase**: Add functional domain labels
3. **Planning Phase**: Add work status labels as needed
4. **Implementation**: Update work status labels during development
5. **Completion**: Remove work status labels before closing

#### **Domain-Specific Guidelines**

**JDBC Compatibility (`jdbc-compatibility`)**
- Issues implementing features from java-spanner JDBC driver
- Often combined with `system variable` for system variables
- Reference issue #47 for comprehensive compatibility tracking
- Maintain compatibility table updates in issue descriptions

**Output Formatting (`output-formatting`)**
- Display improvements, format options, table rendering
- Often combined with `enhancement` or `performance`
- Consider terminal width constraints and user experience
- Examples: CSV/JSON output, table formatting, query plan display

**Operations (`operations`)**
- Database management features: backups, monitoring, administration
- Distinguish from instance-level operations (out of scope)
- Focus on development/testing value over pure operational features
- Examples: database copy, session metrics, change streams

**PostgreSQL Support (`postgresql`)**
- PostgreSQL dialect specific issues
- Parser improvements, system catalog support
- Feature parity considerations with GoogleSQL
- Testing with both dialects

**Emulator-Related (`emulator-related`)**
- Features that behave differently in emulator vs production
- Emulator limitations that affect development workflow
- Testing strategies for emulator environments
- Documentation of emulator-specific behavior

#### **Label Application Guidelines**

**When to Use Multiple Labels**
Most issues should have 2-4 labels:
- Always: One primary classification (`enhancement`, `bug`, etc.)
- Usually: One or more functional domain labels
- Sometimes: Technical characteristic labels
- As needed: Work status labels

**Label Maintenance**
- **Issue Creation**: Apply primary classification immediately
- **Triage**: Add functional domain labels within 24-48 hours
- **Planning**: Add work status labels before implementation begins
- **Progress**: Update work status labels as issues move through workflow
- **Completion**: Remove work status labels before closing

**Quality Assurance**
- Issues without functional domain labels should be rare
- `design-needed` issues should have clear acceptance criteria before implementation
- `blocked` issues should reference the blocking dependency

#### **Common Label Combinations**

**High-Value Development Features**
```
enhancement + system variable + jdbc-compatibility + low hanging fruit
enhancement + output-formatting + performance
enhancement + operations + design-needed
```

**Quality and Testing**
```
tech-debt + testing + low hanging fruit
bug + concurrency + blocked
enhancement + testing + postgresql
```

**Complex Features**
```
enhancement + operations + design-needed + performance
enhancement + output-formatting + breaking-change
enhancement + jdbc-compatibility + design-needed
```

This systematic labeling approach enables owner and AI agent development:
- **Efficient Filtering**: Find issues by functional area or complexity
- **Priority Assessment**: Identify high-value, implementable features for AI implementation
- **Resource Planning**: Balance complex vs simple implementations across development sessions
- **Progress Tracking**: Monitor implementation status across functional domains
- **AI Context**: Provide structured context for AI agents to understand issue scope and requirements

#### Label System Maintenance

**Optimization Process**:
- **Usage Analysis**: Review actual label usage patterns across all issues
- **Owner + AI Focus**: Remove labels not aligned with "owner and AI agent development"
- **Cognitive Load Reduction**: Eliminate redundant labels covered by GitHub standard features
- **Maintenance Overhead**: Balance label granularity with practical management effort

**Historical Optimization (2025-06-17)**:
- **Removed Labels**: `good first issue`, `help wanted` (no external contributors expected)
- **Removed Labels**: `duplicate`, `invalid` (GitHub provides built-in detection)
- **Result**: 25 → 21 labels, reduced AI decision complexity while maintaining functional coverage
- **Rationale**: Focus on actionable categorization for development prioritization

### Creating Issues

Use `gh` CLI for better control over formatting:

```bash
# Create issue with template
gh issue create --title "Issue title" --body "$(cat <<'EOF'
## Problem
Description of the issue...

## Expected vs Actual Behavior
What should happen vs what happens...

## Steps to Reproduce
1. Step one
2. Step two
3. Step three

## Environment
- spanner-mycli version: 
- Go version:
- OS:
EOF
)" --label "bug,enhancement"
```

### Updating Issues

```bash
# Update issue with comprehensive implementation plan
gh issue edit 209 --body "$(cat <<'EOF'
## Current Status
Analysis of current state...

## Implementation Plan

### Phase 1: Basic feature (independently mergeable)
- **System Variable**: `CLI_FEATURE_NAME` (default: false)
- **Implementation**: Location and approach
- **Testing**: Test strategy

### Phase 2: Advanced features
- **Extension**: Additional functionality
- **Integration**: How it connects with existing features
- **Documentation**: User guide updates

## Success Criteria
- [ ] Feature works as specified
- [ ] Tests pass
- [ ] Documentation updated
EOF
)"
```

## Sub-Issue Management

GitHub sub-issues provide a native way to break down large issues into smaller, manageable tasks. This is different from simple issue references and creates a true parent-child relationship in GitHub's issue hierarchy.

### Understanding GitHub Sub-Issues

**What are sub-issues?**
- Native GitHub feature for creating hierarchical issue relationships
- Visible in the GitHub UI with special sub-issue indicators
- Automatically track completion status in parent issue
- Different from simple issue references (e.g., "Related to #123")

**Visibility:**
- GitHub Web UI: Shows sub-issues in parent issue view
- `gh issue` CLI: Does not support viewing sub-issues (use GraphQL API)
- GraphQL API: Full access to sub-issue relationships

**When to use sub-issues:**
- Breaking down large features into independently implementable tasks
- Organizing test coverage improvements by component
- Managing phased rollouts of complex features
- Tracking parallel work streams that contribute to a larger goal

### Creating Sub-Issues with Proper Hierarchy

**Simple workflow using gh-helper:**

```bash
# Method 1: Create sub-issue directly
go tool gh-helper issues create \
  --title "[Test Coverage] Add comprehensive tests for component X" \
  --body "Part of #248 - Parent issue description..." \
  --label "testing,test-coverage-improvement" \
  --parent 248

# Method 2: Link existing issue as sub-issue (deprecated: use edit --parent)
go tool gh-helper issues edit 318 --parent 248

# Verify the sub-issue was properly linked
go tool gh-helper issues show 248 --include-sub
```

**Complete workflow for manual GraphQL approach (if gh-helper not available):**

```bash
# 1. Create the sub-issues first (using standard gh issue create)
gh issue create --title "[Test Coverage] Add comprehensive tests for component X" \
  --body "Part of #248 - Parent issue description..." \
  --label "testing,test-coverage-improvement"

# 2. Get the parent issue's node ID
PARENT_ID=$(gh api graphql -f query='
{
  repository(owner: "apstndb", name: "spanner-mycli") {
    issue(number: 248) { id }
  }
}' --jq '.data.repository.issue.id')

# 3. Get the sub-issue's node ID
SUB_ISSUE_ID=$(gh api graphql -f query='
{
  repository(owner: "apstndb", name: "spanner-mycli") {
    issue(number: 318) { id }
  }
}' --jq '.data.repository.issue.id')

# 4. Link as sub-issue using GraphQL mutation
gh api graphql -f query="
mutation {
  addSubIssue(input: {
    issueId: \"$PARENT_ID\",
    subIssueId: \"$SUB_ISSUE_ID\"
  }) {
    issue { number }
    subIssue { number }
  }
}"
```

### Batch Sub-Issue Creation Script

For creating multiple sub-issues efficiently:

```bash
#!/bin/bash
# create-sub-issues.sh - Create and link multiple sub-issues

PARENT_ISSUE=$1
shift

echo "Creating sub-issues for parent #$PARENT_ISSUE"

# Create each sub-issue directly as a child
for title in "$@"; do
  ISSUE_NUM=$(go tool gh-helper issues create \
    --parent $PARENT_ISSUE \
    --title "$title" \
    --body "Part of #$PARENT_ISSUE" \
    --label "testing" \
    --json | jq -r '.number')
  
  echo "✓ Created sub-issue #$ISSUE_NUM: $title"
done

# Verify all sub-issues were properly linked
echo "Verifying sub-issue linkage..."
gh api graphql -f query="
{
  repository(owner: \"apstndb\", name: \"spanner-mycli\") {
    issue(number: $PARENT_ISSUE) {
      subIssues(first: 20) {
        totalCount
        nodes {
          number
          title
        }
      }
    }
  }
}" --jq '.data.repository.issue.subIssues.totalCount as $count | "Total sub-issues: \($count)"'
```

**Alternative: Manual GraphQL approach for batch creation:**

```bash
#!/bin/bash
# For environments without gh-helper

PARENT_ISSUE=$1
shift

# Get parent issue ID once
PARENT_ID=$(gh api graphql -f query="
{
  repository(owner: \"apstndb\", name: \"spanner-mycli\") {
    issue(number: $PARENT_ISSUE) { id }
  }
}" --jq '.data.repository.issue.id')

echo "Parent issue #$PARENT_ISSUE ID: $PARENT_ID"

# Create each sub-issue and link it
for title in "$@"; do
  # Create the issue
  ISSUE_NUM=$(gh issue create --title "$title" \
    --body "Part of #$PARENT_ISSUE" \
    --label "testing" \
    --json number --jq '.number')
  
  # Get its ID
  SUB_ID=$(gh api graphql -f query="
  {
    repository(owner: \"apstndb\", name: \"spanner-mycli\") {
      issue(number: $ISSUE_NUM) { id }
    }
  }" --jq '.data.repository.issue.id')
  
  # Link as sub-issue
  gh api graphql -f query="
  mutation {
    addSubIssue(input: {
      issueId: \"$PARENT_ID\",
      subIssueId: \"$SUB_ID\"
    }) {
      subIssue { number }
    }
  }" > /dev/null
  
  echo "✓ Created and linked #$ISSUE_NUM: $title"
done
```

### Managing Sub-Issues

```bash
# Move sub-issue to different parent (verified working)
go tool gh-helper issues edit 318 --parent 250 --overwrite

# Remove a sub-issue from parent (verified working)
go tool gh-helper issues edit 318 --unlink-parent

# Reorder sub-issues in parent's list (verified working)
go tool gh-helper issues edit 318 --after 319  # Place #318 after #319
go tool gh-helper issues edit 318 --before 319  # Place #318 before #319
go tool gh-helper issues edit 318 --position first  # Move to beginning
go tool gh-helper issues edit 318 --position last   # Move to end
```

### Efficient Sub-Issue Verification

**Best practices for checking sub-issue status:**

```bash
# Get issue information with sub-issues list and completion stats
go tool gh-helper issues show 248 --include-sub

# Get detailed information for each sub-issue
go tool gh-helper issues show 248 --include-sub --detailed

# JSON output for programmatic use
go tool gh-helper issues show 248 --include-sub --json | jq '.issueShow.subIssues'

# Check specific sub-issue linkage
go tool gh-helper issues show 248 --include-sub --json | jq '
  .issueShow.subIssues.items | 
  map(.number) | 
  if contains([318]) 
  then "✓ #318 is a sub-issue" 
  else "✗ #318 is NOT a sub-issue" end'

# Get completion statistics
go tool gh-helper issues show 248 --include-sub --json | jq '
  .issueShow.subIssues | 
  {
    total: .totalCount,
    completed: .completedCount,
    percentage: .completionPercentage,
    open: [.items[] | select(.state == "OPEN") | .number]
  }'
```

### Important Technical Details

**gh-helper vs GraphQL:**
- **Most operations now use gh-helper** - Simple issue numbers work, no node IDs needed
- **GraphQL still required for**:
  - Removing sub-issue relationships (--unlink-parent has issues)
  - Reordering sub-issues within a parent
  - Custom field selections beyond what gh-helper provides

**Tested and Working gh-helper Commands:**
- ✅ `issues create --parent` - Create new sub-issue
- ✅ `issues edit --parent` - Link existing issue as sub-issue
- ✅ `issues edit --parent --overwrite` - Move sub-issue to different parent
- ✅ `issues show --include-sub` - List sub-issues with stats
- ❌ `issues edit --unlink-parent` - Currently returns error

**Common Pitfalls and Solutions:**
1. **REST API returns 404**: The REST API POST endpoint doesn't exist - use gh-helper or GraphQL
2. **"Field does not exist" errors**: Ensure you're using the correct GraphQL schema (when using GraphQL)
3. **Silent failures**: Always check the response to verify success
4. **Wrong ID format**: When using GraphQL, use node IDs (e.g., `I_kwDONC6gMM...`) not issue numbers

**Node ID Discovery Methods:**
```bash
# Method 1: Direct GraphQL query
gh api graphql -f query='{ repository(owner: "apstndb", name: "spanner-mycli") { issue(number: 248) { id } } }'

# Method 2: From issue URL (if you have the full GitHub URL)
gh api /repos/apstndb/spanner-mycli/issues/248 --jq '.node_id'

# Method 3: Batch retrieval for multiple issues
gh api graphql -f query='
{
  repository(owner: "apstndb", name: "spanner-mycli") {
    issue1: issue(number: 318) { id }
    issue2: issue(number: 319) { id }
    issue3: issue(number: 320) { id }
  }
}'
```

### AI Assistant Guidelines for Sub-Issues

When working with sub-issues:

1. **Always verify linkage after creation** - Use `gh-helper issues show <parent> --include-sub`
2. **Use descriptive titles** - Sub-issue titles should clearly indicate their relationship
3. **Include "Part of #X" in body** - Makes the relationship clear even without the sub-issue link
4. **Check for existing sub-issues** - Use `gh-helper issues show <parent> --include-sub` before creating
5. **Batch operations when possible** - Create all sub-issues together for better organization
6. **Use gh-helper for all sub-issue operations** - Avoid GraphQL unless absolutely necessary

## Issue Review Workflow

### Implementation Planning Guidelines

- **DO NOT include time estimates** - they are meaningless for planning
- **Ensure phases are independently mergeable** - each phase should be a complete PR
- **Focus on spanner-mycli specific functionality** - distinguish from Spanner core features
- **Create system variables for new features** - follow existing patterns
- **Reference specific code locations**: Use `file_path:line_number` format

### Issue Planning Template

```markdown
## Implementation Plan

### Phase 1: Core Implementation (independently mergeable)
- **Files to Modify**: `main.go:90-95`, `system_variables.go:606`
- **System Variable**: `CLI_FEATURE_NAME` (default: false)
- **Implementation**: Brief description of approach
- **Testing**: Test strategy and coverage

### Phase 2: Integration (independently mergeable)
- **Files to Modify**: Additional integration points
- **Dependencies**: What Phase 1 provides
- **Testing**: Integration test requirements

## Success Criteria
- [ ] Feature works as specified
- [ ] All tests pass (`make test && make lint`)
- [ ] Documentation updated
- [ ] Backward compatibility maintained
```

## Pull Request Process

### Pull Request Labels

Pull requests should be labeled according to the repository's automatic release notes configuration (`.github/release.yml`). 

#### **Release Notes Labels**
- `breaking-change` - Categorized in "Breaking Changes" section
- `bug` - Categorized in "Bug Fixes" section  
- `enhancement` - Categorized in "New Features" section
- `ignore-for-release` - Excluded from release notes entirely
  - **Use for**: dev-docs updates (including CLAUDE.md), internal tooling changes
  - **Criteria**: Changes that don't affect spanner-mycli functionality
  - **Note**: User-facing documentation (README.md, docs/) MUST NOT have this label

All other labels are categorized in "Misc" section automatically.

#### **Additional Labels** (Optional)
Any other repository labels can be applied as needed for categorization and filtering purposes.

### Label Management with gh-helper

**Bulk label operations for efficient management:**

```bash
# Add labels to multiple items (auto-detects PR vs Issue)
go tool gh-helper labels add bug,high-priority --items 254,267,238,245

# Remove labels from specific items
go tool gh-helper labels remove needs-review,waiting-on-author --items pull/302,issue/301

# Pattern-based labeling
go tool gh-helper labels add enhancement --title-pattern "^feat:"

# Dry-run to preview changes
go tool gh-helper labels add tech-debt --items 100,101,102 --dry-run

# Interactive confirmation for safety
go tool gh-helper labels remove breaking-change --items 200,201,202 --confirm
```

**PR label inheritance from linked issues:**

```bash
# Automatically inherit labels from issues that a PR closes
go tool gh-helper labels add-from-issues --pr 254

# Preview what would be added
go tool gh-helper labels add-from-issues --pr 254 --dry-run
```

**Common label management workflows:**

```bash
# Label all test-related PRs
go tool gh-helper labels add testing --title-pattern "test:|tests:"

# Remove outdated status labels
go tool gh-helper labels remove needs-review,in-progress --items 100,101,102,103,104

# Bulk categorize documentation PRs
go tool gh-helper labels add docs-user --title-pattern "docs.*README"
go tool gh-helper labels add docs-dev --title-pattern "docs.*dev-docs"
```

### Release Notes Analysis

**Analyze PRs for proper release notes categorization:**

```bash
# Analyze by milestone
go tool gh-helper releases analyze --milestone v0.19.0

# Analyze by date range
go tool gh-helper releases analyze --since 2024-01-01 --until 2024-01-31

# Analyze specific PR range
go tool gh-helper releases analyze --pr-range 250-300

# Include draft PRs in analysis
go tool gh-helper releases analyze --milestone v0.19.0 --include-drafts
```

**Release analysis outputs:**
- PRs missing classification labels (bug, enhancement, feature)
- PRs that should have 'ignore-for-release' label
- Inconsistent labeling patterns
- Release readiness status

**Common release preparation workflow:**

```bash
# 1. Analyze current milestone
go tool gh-helper releases analyze --milestone v0.19.0 > tmp/release-analysis.yaml

# 2. Fix missing labels based on analysis
go tool gh-helper labels add enhancement --items 301,302,303
go tool gh-helper labels add ignore-for-release --items 310,311  # dev-docs only PRs

# 3. Re-analyze to verify
go tool gh-helper releases analyze --milestone v0.19.0

# 4. Generate release notes preview
gh release create v0.19.0 --generate-notes --draft
```

### Creating Pull Requests

- Link PRs to issues using "Fixes #issue-number" in commit messages and PR descriptions
- Use descriptive commit messages following conventional format
- Include clear description of changes and test plan
- **Apply appropriate labels** for automatic release notes categorization
- Ensure `make test && make lint` pass before creating PR

### PR Creation with Insights Template

```bash
# Create PR with development insights template
gh pr create --title "feat: implement feature X" --body "$(cat <<'EOF'
## Summary
Brief description of changes...

## Key Changes
- **main.go**: Added CLI flag for feature X
- **system_variables.go**: Implemented FEATURE_X system variable
- **statements_feature.go**: Core feature implementation

## Development Insights

### Discoveries
- Pattern: Error handling approach for component X  
- Architecture: Component Y dependency should be reversed
- Testing: Integration test pattern for feature Z

### Process Insights  
- Workflow: phantom + tmux horizontal split works best for this type of issue
- Tool: Command X saves significant time for debugging

### CLAUDE.md Integration Candidates
- Add pattern X to Development Flow Insights section
- Update Dependencies section with library behavior Y

## Test Plan
- [ ] Unit tests pass
- [ ] Integration tests pass
- [ ] Manual testing completed
- [ ] Lint checks pass

Fixes #XXX
EOF
)"
```

### Adding Development Insights During Review

```bash
# Add additional insights discovered during review process
gh pr comment <PR-number> --body "$(cat <<'EOF'
## Review Process Insights

### Code Review Discoveries
- Pattern Y emerged during review discussion
- Alternative approach Z suggested by reviewer

### CI/Testing Insights
- Test failure revealed edge case in component A
- Performance consideration discovered for component B
EOF
)"
```

## Code Review Response Strategy

### Addressing Review Comments

1. **Address each comment individually** with focused commits
2. **Use descriptive commit messages** referencing specific issues
3. **Test thoroughly** before pushing changes
4. **For AI reviews (Gemini Code Assist)**: Use `/gemini review` to trigger re-review
5. **For praise comments**: Acknowledge briefly and resolve conversation

### Replying to Review Thread Comments

**Method 1: Automated Scripts (Recommended)**

```bash
# List unresolved review threads that need replies
go tool gh-helper reviews fetch 287 --list-threads

# Show detailed thread context before replying
go tool gh-helper threads show PRRT_kwDONC6gMM5SU-GH

# Show multiple threads at once (new feature)
go tool gh-helper threads show PRRT_kwDONC6gMM5SU-GH PRRT_kwDONC6gMM5SU-GI

# Reply to a specific thread (code changes made) - standard workflow
go tool gh-helper threads reply PRRT_kwDONC6gMM5SU-GH --message "Thank you for the feedback! Fixed in commit abc1234." --resolve

# Batch resolve multiple threads (if you forgot to use --resolve earlier)
go tool gh-helper threads resolve PRRT_kwDONC6gMM5SU-GH PRRT_kwDONC6gMM5SU-GI PRRT_kwDONC6gMM5SU-GJ

# Reply to a specific thread (no code changes needed)
go tool gh-helper threads reply PRRT_kwDONC6gMM5SU-GH --message "Thank you for the feedback! This is working as intended."

# Reply with mention for AI reviews
go tool gh-helper threads reply PRRT_kwDONC6gMM5SVHTH --message "Fixed as suggested in commit def5678!" --mention gemini-code-assist

# Multi-line reply with stdin (AI-friendly)
go tool gh-helper threads reply PRRT_kwDONC6gMM5SU-GH <<EOF
Thank you for the detailed review!

I've addressed the issue in commit abc1234:
- Improved error handling as suggested
- Added proper validation
- Updated tests to cover edge cases

The implementation now handles all the scenarios you mentioned.
EOF
```

### Review Response Best Practices

**For code changes (recommended workflow):**
1. Make the necessary fixes
2. Commit and push the changes
3. Reply with commit hash and resolve: `go tool gh-helper threads reply <THREAD_ID> --commit-hash <HASH> --message "Fixed as suggested" --resolve`
   - Quick commit reference: `go tool gh-helper threads reply <THREAD_ID> --commit-hash <HASH> --resolve` (uses default message)
   - The --commit-hash flag automatically includes the hash in the reply message

**For explanations without code changes:**
1. Reply with explanation
2. No commit hash needed

**Template for code fix responses:**
```
Thank you for the feedback! Fixed in commit [hash].

[Brief explanation of what was changed]
```

**Template for explanation responses:**
```
Thank you for the feedback! 

[Explanation of why current implementation is correct or why change isn't needed]
```

### Gemini Code Review Integration (Project-Specific)

**Complete Review Workflow with Gemini:**

```bash
# 1. Create PR (Gemini automatically reviews initial creation)
gh pr create --title "feat: implement feature" --body "Description"

# 2. Wait for automatic Gemini review (initial PR only)
go tool gh-helper reviews wait <PR_NUMBER> --timeout 15m

# 3. Fetch all review data
go tool gh-helper reviews fetch <PR_NUMBER> > tmp/review-data.yaml

# 4. Create fix plan based on all feedback
mkdir -p tmp
cat > tmp/fix-plan.md << 'EOF'
# Fix Plan for PR <PR_NUMBER>

## Review Comments Summary
[Analysis of all critical/high priority items]

## Planned Changes
1. **Fix A**: Description and affected files
   - Files: file1.go, file2.md
   - Approach: Specific implementation plan
   
2. **Fix B**: Description and affected files
   - Files: file3.go
   - Approach: Specific implementation plan

## Thread Resolution Plan
- THREAD_ID_1: Fix A addresses this
- THREAD_ID_2: Fix B addresses this
- THREAD_ID_3: Explanation only (no code change)
EOF

# 5. Execute fixes according to plan
# [Make all planned changes]

# 6. Commit and push all related fixes together
git add <specific-files>
git commit -m "fix: address review feedback - implement fixes A, B, C"
git push

# 7. Request Gemini review for subsequent pushes (REQUIRED)
go tool gh-helper reviews wait <PR_NUMBER> --request-review --timeout 15m

# 8. Reply to threads with commit hash and resolve immediately
COMMIT_HASH=$(git rev-parse HEAD)
go tool gh-helper threads reply <THREAD_ID_1> --commit-hash $COMMIT_HASH --message "Fixed as planned in fix A" --resolve
go tool gh-helper threads reply <THREAD_ID_2> --commit-hash $COMMIT_HASH --message "Fixed as planned in fix B" --resolve
go tool gh-helper threads reply <THREAD_ID_3> --message "This works as intended because..." --resolve

# 8a. Verify all threads are resolved (new reviews may arrive after push)
go tool gh-helper reviews fetch <PR_NUMBER> --list-threads

# Alternative: Batch resolve threads (useful for resolving forgotten threads)
# If you forgot to use --resolve flag, you can resolve multiple threads at once:
go tool gh-helper threads resolve <THREAD_ID_1> <THREAD_ID_2> <THREAD_ID_3>

# 9. Clean up planning files
rm tmp/review-data.yaml tmp/fix-plan.md
```

**When to request Gemini review:**
- **IMPORTANT**: Only after pushes made AFTER PR creation (not for initial PR)
- After addressing review feedback with code changes
- When adding new functionality or making architectural changes  
- After significant refactoring or bug fixes
- Before final merge to ensure all concerns are addressed

**Note**: Gemini automatically reviews the initial PR creation, so `/gemini review` is only needed for subsequent pushes to the same PR.

**Gemini-specific reply patterns:**
```bash
# For AI code review feedback
go tool gh-helper threads reply-commit <THREAD_ID> <HASH> --mention gemini-code-assist

# Multi-line response to AI suggestions
go tool gh-helper threads reply <THREAD_ID> --mention gemini-code-assist <<EOF
Thank you for the detailed analysis!

I've implemented your suggestions:
- [Specific change 1]
- [Specific change 2]

Fixed in commit abc1234.
EOF
```

**Method 2: Manual GraphQL Mutation (For understanding the process)**

```bash
# Step 1: Get review thread IDs and existing comments (enhanced query)
gh api graphql -f query='
{
  repository(owner: "apstndb", name: "spanner-mycli") {
    pullRequest(number: <PR_NUMBER>) {
      reviewThreads(first: 10) {
        nodes {
          id
          line
          path
          isResolved
          subjectType
          comments(first: 10) {
            nodes {
              id
              body
              author {
                login
              }
              createdAt
            }
          }
        }
      }
      reviews(first: 10) {
        nodes {
          id
          author {
            login
          }
          state
          submittedAt
        }
      }
    }
  }
}' | jq '
  .data.repository.pullRequest |
  {
    "unresolved_threads": [
      .reviewThreads.nodes[] | 
      select(.isResolved == false) |
      {
        "thread_id": .id,
        "location": "\(.path):\(.line)",
        "subject_type": .subjectType,
        "latest_comment": .comments.nodes[-1].body[0:100] + "...",
        "needs_reply": (.comments.nodes | map(.author.login) | unique | contains(["apstndb"]) | not)
      }
    ],
    "available_review_ids": [
      .reviews.nodes[] |
      {
        "review_id": .id,
        "author": .author.login,
        "state": .state
      }
    ]
  }
'

# Step 2: Create GraphQL mutation file (recommended approach - omit pullRequestReviewId)
cat > /tmp/reply_mutation.graphql << 'EOF'
mutation {
  addPullRequestReviewThreadReply(input: {
    pullRequestReviewThreadId: "PRRT_kwDONC6gMM5SU-GH"
    body: "Your detailed response here"
  }) {
    comment {
      id
      url
      body
    }
  }
}
EOF

# Step 3: Execute the mutation
gh api graphql -F query=@/tmp/reply_mutation.graphql
```

**Method 2: General PR Comment (Simpler, but not thread-specific)**

```bash
gh pr comment <PR_NUMBER> --body "## Review Comment Responses

**Issue 1 (file.go:123):** Response to specific comment...
**Issue 2 (other.go:456):** Response to another comment..."
```

**Enhanced Features Using GraphQL Schema:**
- **Filtered Output**: Only shows unresolved threads that need replies
- **Smart Detection**: Identifies threads where you haven't replied yet
- **Context Information**: Shows file location, subject type, and comment previews
- **Available Review IDs**: Lists all review IDs you can use for replies

**Schema Exploration and Documentation:**

**Official Documentation:**
- [GitHub GraphQL API Documentation](https://docs.github.com/en/graphql) - Complete reference for all types, queries, and mutations
- [GraphQL Explorer](https://docs.github.com/en/graphql/overview/explorer) - Interactive query builder and schema browser

**Schema Introspection with gh CLI:**
```bash
# Discover available fields for any GraphQL type
gh api graphql -f query='{ __type(name: "PullRequestReviewThread") { fields { name description } } }'

# Find all input fields for mutations
gh api graphql -f query='{ __type(name: "AddPullRequestReviewThreadReplyInput") { inputFields { name description type { name } } } }'

# List all available mutations
gh api graphql -f query='{ __schema { mutationType { fields { name description } } } }' | jq '.data.__schema.mutationType.fields[] | select(.name | contains("pullRequest") or contains("review"))'

# Get full schema documentation for a specific type
gh api graphql -f query='{ __type(name: "PullRequest") { description fields { name description type { name kind } } } }'
```

**Useful Schema Query Patterns:**
```bash
# Find all review-related types
gh api graphql -f query='{ __schema { types { name } } }' | jq '.data.__schema.types[].name' | grep -i review

# Check required vs optional fields for mutations
gh api graphql -f query='{ __type(name: "AddPullRequestReviewThreadReplyInput") { inputFields { name type { kind name } } } }' | jq '.data.__type.inputFields[] | {name, required: (.type.kind == "NON_NULL")}'
```

**Reply with Mentions (Recommended for AI reviews):**
```bash
# Include @mention for acknowledgment and notification
cat > /tmp/reply_mention.graphql << 'EOF'
mutation {
  addPullRequestReviewThreadReply(input: {
    pullRequestReviewThreadId: "PRRT_kwDONC6gMM5SVHTH"
    pullRequestReviewId: "PRR_kwDONC6gMM6uvWPT"
    body: "@gemini-code-assist Thank you for the suggestion! I've addressed your feedback by [specific change made]."
  }) {
    comment { id url }
  }
}
EOF

gh api graphql -F query=@/tmp/reply_mention.graphql
```

**Quick Helper Script Template:**
```bash
#!/bin/bash
# go tool gh-helper threads reply
PR_NUMBER=$1
THREAD_ID=$2
REVIEW_ID=$3
REPLY_TEXT="$4"
MENTION_USER="$5"  # Optional: @username for mention

if [ $# -lt 4 ]; then
    echo "Usage: $0 <pr_number> <thread_id> <review_id> <reply_text> [mention_user]"
    exit 1
fi

# Add mention if provided
if [ -n "$MENTION_USER" ]; then
    REPLY_TEXT="@$MENTION_USER $REPLY_TEXT"
fi

cat > /tmp/reply_mutation.graphql << EOF
mutation {
  addPullRequestReviewThreadReply(input: {
    pullRequestReviewThreadId: "$THREAD_ID"
    pullRequestReviewId: "$REVIEW_ID"
    body: "$REPLY_TEXT"
  }) {
    comment { id url }
  }
}
EOF

gh api graphql -F query=@/tmp/reply_mutation.graphql | jq '.data.addPullRequestReviewThreadReply.comment'
```

**Key Points for GraphQL Approach:**
- **Documentation First**: Always check [GitHub GraphQL API docs](https://docs.github.com/en/graphql) for official field descriptions and examples
- **Schema Introspection**: Use `gh api graphql` with `__type` and `__schema` queries to explore available fields dynamically
- **Interactive Development**: Use [GraphQL Explorer](https://docs.github.com/en/graphql/overview/explorer) for building and testing queries interactively
- **Required vs Optional Fields**: Use schema introspection to identify required fields (`NON_NULL` type kind)
- **pullRequestReviewId**: **IMPORTANT** - This field is optional and often causes failures when included. Omit it for simple thread replies
- **File-based Mutations**: Create mutation files to avoid shell escaping issues with complex text
- **Validation**: Always validate GraphQL queries against schema before implementing in scripts

**Common Issues and Solutions:**
- **Mutation returns null comment**: Often caused by including optional `pullRequestReviewId` field
- **Permission errors**: Ensure you have write access to the repository
- **Thread not found**: Verify thread ID is correct and thread still exists

**Common GraphQL API Patterns:**
```bash
# Rate limit checking
gh api graphql -f query='{ rateLimit { cost remaining resetAt } }'

# Node ID resolution (useful for debugging)
gh api graphql -f query='{ node(id: "PRRT_kwDONC6gMM5SU-GH") { __typename } }'

# Pagination with cursors (for large result sets)
gh api graphql -f query='{ repository(owner: "apstndb", name: "spanner-mycli") { pullRequests(first: 5, after: "cursor") { pageInfo { hasNextPage endCursor } } } }'
```

### Review Comment Response Template

```markdown
@reviewer Thank you for the code review! I've addressed your suggestion about [specific topic].

## Changes Made

✅ **[Change description]**: [Brief explanation of what was done]

✅ **[Additional improvement]**: [Any additional improvements made]

The improvement provides [benefit description], which is better than the previous approach because [reasoning].

Commit: [commit-hash]
```

### AI Code Review Integration

For automated reviews using Gemini Code Assist:

```bash
# Request Gemini review after addressing comments (recommended method)
go tool gh-helper reviews wait <PR-number> --request-review --timeout 15m
```

**Note**: Use `--request-review` flag instead of manual comment posting for consistent review workflow.

## Knowledge Management

### Benefits of PR Comment Approach

- **Searchable**: GitHub search finds insights across all PRs
- **Contextual**: Insights linked directly to implementation
- **Persistent**: No worktree cleanup affects knowledge retention
- **Collaborative**: Team members can see and build on insights

### Knowledge Management Evolution

- **Improved Workflow**: PR comments provide persistent, searchable, contextual knowledge capture
- **Best Practice**: Add development insights as PR comments after implementation for future reference

## Git Practices

### Important Git Practices

- **CRITICAL**: Never push directly to main branch - always use Pull Requests
- **CRITICAL**: Never commit directly to main branch - always use feature branches
- Always use `git add <specific-files>` instead of `git add .`
- Check `git status` before committing to verify only intended files are staged
- Use `git show --name-only` to review committed files

#### Branch Workflow
- **Feature Development**: Always create branches from `origin/main` using phantom worktrees
- **Main Branch**: Reserved for merged PR content only - no direct commits
- **No Hotfixes**: spanner-mycli is not a service - all changes go through feature branch → PR workflow

### Phantom Worktree Management

#### Worktree Lifecycle
- **Create**: Use `make worktree-setup WORKTREE_NAME=issue-123-feature` (automatically fetches and bases on `origin/main`)
- **Work**: Develop in isolated environment with `phantom shell`
- **Delete**: Use `phantom delete worktree-name` when no longer needed

#### AI Assistant Guidelines

**Decision Matrix for Autonomous vs. User-Permission Operations**:

**Always Request Permission**:
- `phantom delete --force` (uncommitted changes present)
- Any operation with `--force` flag
- Destructive operations affecting work history
- Git operations that could cause data loss
- Repository structural changes (branch deletion, rebasing)

**Autonomous Actions Allowed**:
- Clean phantom worktree deletion (no uncommitted changes, issue resolved)
- Standard git operations on feature branches (e.g., add, commit, push, pull), excluding history-altering operations like rebase which require permission
- Documentation updates following established patterns
- File reading and analysis operations
- Test execution and linting

**User Confirmation Required**:
- Worktree deletion with uncommitted changes (explain what will be lost)
- Branch operations affecting multiple commits
- Operations that bypass standard workflow (emergency fixes)

**Best Practices**:
- **Check Status First**: Always run `git status` before destructive operations
- **Check File Safety**: Refer to `.gitignore` comments for file deletion safety rules
- **Explain Impact**: When requesting permission, clearly state what will be lost or changed
- **Offer Alternatives**: Suggest safer approaches when possible (e.g., commit changes before deletion)
- **Preserve Context**: Prioritize preserving development work over convenience

### Linking Issues and Pull Requests

Use GitHub's issue linking syntax in commit messages and PR descriptions:

- **Supported Keywords**: `Closes`, `Fixes`, `Resolves` (and variations)
- Include links in both commit messages and PR descriptions for automatic closure

### Commit Message Format

```
type(scope): brief description

Detailed explanation of changes if needed.

Fixes #123
```

Examples:
```
feat(timeout): add --timeout flag for gcloud compatibility

Implement STATEMENT_TIMEOUT system variable with duration validation
and pointer type design. Apply timeouts consistently across all query
types while preserving 24h default for partitioned DML operations.

Fixes #276
```

## Related Documentation

- [Development Insights](development-insights.md) - Development patterns and workflows
- [Architecture Guide](architecture-guide.md) - System architecture and components
- [System Variable Patterns](patterns/system-variables.md) - Implementation patterns