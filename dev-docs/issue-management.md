# Issue and Code Review Management

This document covers GitHub workflow, issue management, code review processes, and development tools usage for spanner-mycli.

## Development Tools Usage

> [!NOTE]
> **This tooling is primarily intended for use by AI assistants.** The commands and workflows described here are designed for programmatic execution to maintain consistency. Human developers should refer to these sections to understand the automation process but are not required to follow it manually for their own contributions.


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
git add <specific-files> && git commit -m "fix: address feedback" && git push
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
- `umbrella` - Parent issue tracking multiple sub-issues. This should be applied to the parent issue in a parent-child relationship.
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

**Implementation Readiness Philosophy**
This project focuses on implementation readiness rather than traditional priority systems:
- **`low hanging fruit`**: Ready to implement with clear scope and minimal complexity
- **No label**: Standard implementation complexity
- **`design-needed`**: Requires design work before implementation can begin
- **`blocked`**: Cannot be worked on due to external dependencies

This approach emphasizes whether an issue can be immediately worked on rather than its business priority.

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

**Label Philosophy Update (2025-07-11)**:
- **Added Label**: `umbrella` - Essential for tracking parent-child issue relationships
- **Rejected**: Priority labels (P0-P3) - Implementation readiness is more relevant than business priority
- **Rejected**: Status labels (in-progress, needs-review) - GitHub UI and PR process provide this visibility
- **Result**: Minimal label set focused on implementation decisions rather than project management overhead

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
- **All sub-issue operations now use gh-helper** - Simple issue numbers work, no node IDs needed
- **GraphQL only needed for**:
  - Custom field selections beyond what gh-helper provides
  - Complex queries with specific field combinations

**Tested and Working gh-helper Commands:**
- ✅ `issues create --parent` - Create new sub-issue
- ✅ `issues edit --parent` - Link existing issue as sub-issue
- ✅ `issues edit --parent --overwrite` - Move sub-issue to different parent
- ✅ `issues edit --unlink-parent` - Remove parent relationship
- ✅ `issues edit --after/--before` - Reorder sub-issues
- ✅ `issues edit --position first/last` - Move to beginning/end
- ✅ `issues show --include-sub` - List sub-issues with stats

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
- **Files to Modify**: `internal/mycli/app.go:90-95`, `internal/mycli/system_variables.go:606`
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

For the full release preparation workflow, use `/release-prep <milestone>`.

### Creating Pull Requests

- Link PRs to issues using "Fixes #issue-number" in commit messages and PR descriptions
- Use descriptive commit messages following conventional format
- Include clear description of changes and test plan
- **Apply appropriate labels** for automatic release notes categorization
- Ensure `make test && make lint` pass before creating PR

For PR creation workflow, use `/create-pr`. For code review response workflow, use `/review-respond` and `/review-cycle`.

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