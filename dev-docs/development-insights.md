# Development Flow Insights

This document captures development patterns, best practices, and lessons learned from implementing features in spanner-mycli.

## Parallel Issue Development with Phantom

**Discovery**: Using phantom worktrees enables efficient parallel development of multiple low-hanging fruit issues

- **Workflow**: Create multiple worktrees simultaneously (`issue-245`, `issue-243`, `issue-241`, etc.)
- **tmux Integration**: Different layout strategies (window/horizontal/vertical) optimize screen space usage
- **Independent Testing**: Each worktree maintains separate build artifacts and test state
- **Context Switching**: No rebuild costs when switching between issues
- **Knowledge Capture**: `.worktree-knowledge.md` enables systematic capture of insights

### Automated Setup

Use the provided script for consistent worktree setup:

```bash
# Automated setup (recommended)
make worktree-setup WORKTREE_NAME=issue-276-timeout-flag

# Manual setup (for reference)
phantom create issue-276-timeout-flag --exec 'ln -sf ../../../../.claude .claude'
```

### Path Structure Explanation

**Phantom worktree directory structure:**
```
/home/user/spanner-mycli/                    # Repository root
├── .claude/                                 # Claude settings directory
└── .git/phantom/worktrees/<worktree-name>/  # Phantom worktree location
```

**Symlink path breakdown (`../../../../.claude`):**
- `../` → `/home/user/spanner-mycli/.git/phantom/worktrees/`
- `../../` → `/home/user/spanner-mycli/.git/phantom/`
- `../../../` → `/home/user/spanner-mycli/.git/`
- `../../../../` → `/home/user/spanner-mycli/` (repository root)
- `../../../../.claude` → `/home/user/spanner-mycli/.claude/`
phantom shell issue-276-timeout-flag --tmux-horizontal
```

## AI-Friendly Tool Development Insights (Issue #301)

### GitHub GraphQL API Edge Cases

**Critical Discovery: statusCheckRollup Behavior with Merge Conflicts**

During development of `gh-helper`, we discovered that GitHub's GraphQL API returns `statusCheckRollup: null` when a Pull Request has merge conflicts, rather than providing check status information.

**Technical Details**:
- `mergeable: "CONFLICTING"` + `mergeStateStatus: "DIRTY"` indicates merge conflicts
- `statusCheckRollup: null` in this state prevents CI workflows from starting
- This is GitHub's intentional behavior, not a bug or API limitation

**Implementation Lessons**:
1. Always check `mergeable` field before interpreting `statusCheckRollup`
2. Immediate termination with actionable guidance beats infinite waiting
3. User experience benefits from clear problem diagnosis + solution steps

**References**: 
- [GitHub Community Discussion](https://github.community/t/what-does-it-mean-when-statuscheckrollup-is-null/252822)
- Real-world testing showed this pattern consistently across repositories

### Claude Code Environment Constraints

**Discovery: Configurable Timeout via Environment Variables**

While Claude Code defaults to a 2-minute timeout, it supports configuration through environment variables in `~/.claude/settings.json` or project `.claude/settings.json`.

**Research Findings from GitHub Issues**:
- anthropics/claude-code#1039: Feature request for configurable timeouts was implemented
- anthropics/claude-code#1216: Default behavior shows "Command timed out after 2m 0.0s"
- anthropics/claude-code#1717: User successfully configured 15-minute timeouts via settings.json

**Environment Variable Configuration**:
```json
{
  "env": {
    "BASH_MAX_TIMEOUT_MS": "900000",    // 15 minutes - upper limit for explicit timeouts
    "BASH_DEFAULT_TIMEOUT_MS": "900000"  // 15 minutes - default when no timeout specified
  }
}
```

**Implementation Strategy**:
```go
// Check for Claude Code environment variables
claudeCodeEnvTimeout, hasClaudeCodeEnv := checkClaudeCodeEnvironment()

if hasClaudeCodeEnv {
    // Respect configured timeout limit
    if timeoutDuration > claudeCodeEnvTimeout {
        fmt.Printf("⚠️  Requested timeout (%v) exceeds Claude Code limit (%v). Using %v.\n", 
            timeoutDuration, claudeCodeEnvTimeout, claudeCodeEnvTimeout)
        effectiveTimeout = claudeCodeEnvTimeout
    }
} else {
    // Default: Use 90-second safety margin
    claudeCodeLimit := 90 * time.Second
    if timeoutDuration > claudeCodeLimit {
        fmt.Printf("⚠️  Claude Code has 2-minute timeout (no env config detected). Using %v for safety.\n", claudeCodeLimit)
        fmt.Printf("💡 To extend timeout, set BASH_MAX_TIMEOUT_MS in ~/.claude/settings.json\n")
    }
}
```

**Key Lessons**:
- Environment detection enables adaptive behavior (fallback to safe defaults vs respecting user config)
- Project settings (.claude/settings.json) should be committed for team consistency
- Local settings (.claude/settings.local.json) should NOT be committed (personal preferences)
- Clear guidance about configuration options builds user confidence
- Claude Code does auto-retry on timeout, but relying on this behavior is not recommended

### Cobra CLI Framework Assumptions vs Reality

**Critical Discovery**: Cobra's error handling makes assumptions that break down for operational tools.

**Cobra's Assumptions**:
- All `RunE` errors = user syntax mistakes → Show usage help
- All errors should be printed by framework → Duplicate messages
- CLI tools are primarily for syntax-driven operations

**Reality for Development Tools**:
- Most errors are operational (merge conflicts, API failures, timeouts)
- Users need specific solutions, not generic command syntax
- Rich error messaging happens in business logic, not error handling

**Evolution of Understanding**:
1. **Initial**: "Why does my tool show usage help for merge conflicts?"
2. **Investigation**: Cobra assumes all errors are usage errors
3. **Solution**: Separate operational guidance from error propagation
4. **Insight**: Framework assumptions don't match all use cases

**Architectural Lesson**: When framework assumptions don't match your use case, work with the framework's design rather than against it. Use error codes for control flow, user messaging for guidance.

### AI Assistant Integration Patterns

**Discovery: State Tracking Reduces API Load**

Implementing incremental review state tracking in `~/.cache/spanner-mycli-reviews/` reduced GitHub API calls by ~80% during development cycles.

**Technical Implementation**:
- Store last known review ID and timestamp
- Compare against current state to detect "new" reviews
- Survives tool restarts and provides consistent behavior

**Benefits**:
- Faster response times (no redundant API calls)
- Better rate limiting compliance
- More reliable detection of incremental changes

**File Format**:
```json
{
  "id": "PRR_kwDONC6gMM6vB1Fv",
  "createdAt": "2025-06-17T17:20:47Z"
}
```

### Error Message Design for AI Workflows

**Key Insight**: AI assistants need structured, actionable error messages with specific next steps.

**Effective Pattern**:
```
❌ [timestamp] Clear problem statement
⚠️  Impact explanation (why this matters)
💡 Specific solution command
💡 Follow-up action with exact syntax
```

**Example**:
```
❌ [04:07:27] PR has merge conflicts (status: DIRTY)
⚠️  CI checks will not run until conflicts are resolved
💡 Resolve conflicts with: git rebase origin/main
💡 Then push and run: go tool gh-helper reviews wait 306
```

**Benefits**:
- AI can parse structured information
- Users get clear action items
- Reduces back-and-forth troubleshooting

## Error Handling Architecture Evolution

**Discovery**: Systematic replacement of panics with proper error handling improves system robustness

- **Architecture Impact**: Moving from panic-driven failure to graceful error propagation
- **System Boundary**: Clear distinction between "programming errors" (should panic) vs "runtime conditions" (should return errors)
- **Testing Strategy**: Comprehensive nil checks and error condition testing becomes critical
- **Maintenance**: Error handling patterns should be consistent across similar components

## Systematic Error Detection and Quality Improvement

**Discovery**: Comprehensive error handling review reveals patterns and establishes sustainable practices

- **Detection Strategy**: Use `rg "_, _ ="` and `rg "ignoreError"` for systematic scanning of ignored errors
- **Three-Tier Improvement**: Silent failure → warning logs → contextual logging with relevant data
- **Contextual Information Priority**: Include SQL statements, proto type names, and operation context in error logs
- **Review-Driven Development**: AI assistant collaboration provides iterative improvement from basic fixes to optimization
- **Helper Function Elimination**: Remove error-hiding utilities (`ignoreError`) to enforce explicit handling
- **Log Efficiency**: Balance between debugging information and log volume (proto type names vs full messages)

## System Robustness Through Panic Elimination

**Discovery**: Strategic panic removal requires systematic categorization and staged approach

### Decision Criteria for panic vs error return

- Programming errors (type violations, impossible states) → panic or log + graceful degradation
- Runtime conditions (invalid input, configuration errors) → error return
- External package code → preserve original behavior

### Implementation Strategy

- **Library Code Principles**: Avoid panics in library code, provide callers with choices
- **Systematic Approach**: Use `rg "panic\("` to scan → categorize by file/purpose → prioritize by impact
- **Parallel Development**: Consider PR conflicts when multiple issues address overlapping code areas
- **Consistency Patterns**: Maintain unified structured logging (slog) for better caller control

## Resource Management in Batch Processing

**Discovery**: Proper resource cleanup requires careful defer placement and lifecycle management

- **defer Timing**: Place defer statements immediately after successful resource creation, not after subsequent operations
- **Batch Processing Complexity**: Partitioned query processing involves multiple goroutines sharing resources, requiring careful lifecycle management
- **Architecture Pattern**: Long-lived resources like `batchROTx` need both `Cleanup()` and `Close()` calls in proper sequence
- **Systematic Review**: Audit defer statement placement by checking if resource creation and cleanup reservation are adjacent
- **Error Path Safety**: Ensure cleanup occurs even when errors happen in subsequent processing steps

## Systematic Code Quality Improvement

**Discovery**: Re-enabling linters after fixing warnings often reveals additional hidden issues not in original scope

### Linter Re-enablement Workflow

1. Fix reported warnings
2. Run `make lint`
3. Fix newly revealed warnings
4. Re-enable in `.golangci.yml`
5. Verify with final `make lint` run

### Key Practices

- **Multiple Lint Run Requirement**: Critical practice when re-enabling linters as fixes can expose previously hidden warnings
- **Staticcheck Warning Categories**: QF series (optimization quick fixes), ST series (Go style compliance) - address by category for consistency

## Context and Session Management Architecture

**Discovery**: Centralized context management prevents race conditions and improves resource lifecycle control

### Design Patterns

- **Statement Type Assertion**: Using type assertion on `Statement` interface allows differentiated timeout behavior based on operation type
- **Integration Test Considerations**: Long-running integration tests need explicit timeout configuration (1h) to prevent false failures during CI
- **System Variable Sharing**: MCP server tests require careful system variable instance sharing between CLI and Session to maintain state consistency
- **Backward Compatibility Strategy**: Preserve existing defaults for specific operations (PDML 24h) while introducing new general defaults (10m)

### Development Process

- **Review-Driven Improvement**: AI assistant collaboration (Gemini Code Assist) provides valuable feedback for validation patterns and code clarity
- **Context Lifecycle Management**: Context cancellation must align with resource lifecycle - centralize timeout application at statement execution boundary, not at individual operation level

## Quick Command Reference

Common commands used during development:

```bash
# Quick tests during development
go test -short ./...        # Unit tests only
make lint                   # Linter only  
make test && make lint      # Full validation

# Development cycle
make build                  # Build the application
make fasttest              # Quick tests during development
make clean                 # Clean artifacts when needed

# Help documentation updates
make docs-update    # Generate help output for README.md
```

## AI-Friendly Tool Development (Issue #301 Insights)

### Key Design Principles Discovered

**1. stdin/heredoc vs Temporary Files**
AI assistants strongly prefer stdin input over temporary file creation:
```bash
# Preferred pattern (AI-friendly)
echo "Multi-line content" | go tool gh-helper threads reply THREAD_ID

# Avoid this pattern (requires file system operations)
cat > /tmp/content.txt << EOF
Content here
EOF
tool --file /tmp/content.txt
rm /tmp/content.txt
```

**2. Self-Documenting Tool Design**
Comprehensive `--help` output is critical for AI tool discovery and usage:
- Include usage examples in help text
- Document all input methods (stdin, flags, etc.)
- Provide context about when to use each command

**3. Module Organization Strategy**
Unified Go module for development tools (`github.com/apstndb/spanner-mycli/dev-tools`) proved superior to separate modules:
- Simplifies dependency management
- Reduces build complexity
- Enables easier cross-tool integration

**4. Generic vs Project-Specific Separation**
Clear separation between generic GitHub operations (`gh-helper`) and project-specific tools (`spanner-mycli-dev`) improves:
- Code reusability across projects
- Maintenance clarity  
- AI assistant understanding of tool scope

**5. Build System Integration**
Makefile targets that wrap new tools maintain familiar workflows while leveraging modern tooling underneath.

### Implementation Patterns

**GraphQL Error Handling**
When working with GitHub GraphQL API, avoid including optional fields that cause null responses:
```go
// CRITICAL: Do NOT include pullRequestReviewId - causes failures
mutation {
  addPullRequestReviewThreadReply(input: {
    pullRequestReviewThreadId: "$THREAD_ID"
    body: "$REPLY_TEXT"
  }) { ... }
}
```

**State Management for Review Monitoring**
Review state tracking in `~/.cache/spanner-mycli-reviews/` enables incremental monitoring without API rate limit issues.

**Critical Review Analysis Pattern**

**Key principle**: Different review mechanisms capture different types of feedback:
- **Inline threads**: Specific implementation details, syntax issues
- **General review comments**: Architecture concerns, design patterns, critical system behavior

**AI Assistant workflow for comprehensive feedback analysis**:

## Review Thread Resolution Workflow (Issue #306 Enhancement)

**Critical Pattern**: Proper thread resolution workflow prevents confusion about feedback status:

1. **Make changes**: Address reviewer feedback with code modifications
2. **Commit changes**: Create commit with proper message
3. **Push changes**: Ensure commit is available on GitHub
4. **Reply with reference**: Reply to thread with commit hash reference
5. **Resolve thread**: Mark as addressed (can be combined with reply)

**Why this order matters**:
- Commit hash is available only after committing
- GitHub can display commit references only after push
- Shows reviewer that feedback was implemented, not just acknowledged
- Provides verifiable evidence of changes

```bash
# Complete workflow example
# 1-2. Make changes and commit
git add . && git commit -m "fix: address review feedback"
COMMIT_HASH=$(git rev-parse HEAD)  # Capture the fixing commit hash
# 3. Push to make commit available on GitHub  
git push
# 4-5. Reply with commit reference and resolve (can be combined)
go tool gh-helper threads reply PRRT_xyz --commit-hash $COMMIT_HASH --message "Fixed as suggested" --resolve

# Alternative: Find commit by message or content
# git log --oneline --grep="review feedback" -1 --format="%H"
# git log --oneline -S "specific code change" -1 --format="%H"
```

**Thread Resolution Detection Logic** (Fixed in Issue #306):
- **Previous (incorrect)**: Thread needs reply if ANY user comment exists
- **Current (correct)**: Thread needs reply if LAST comment is from external user
- **Implication**: More accurate "needs reply" detection prevents missed feedback

**AI Assistant workflow for comprehensive feedback analysis**:
1. Use `go tool gh-helper reviews analyze <PR>` for complete review analysis (not just threads)
2. Look for severity indicators: "critical", "high-severity", "panic", "error" in review bodies
3. Don't assume all important feedback appears in threaded comments
4. Always analyze review summaries after responding to individual threads

**Technical implementation**: Unified GraphQL query (`shared/unified_review.go`) fetches both review bodies and threads simultaneously, with automatic severity detection and actionable item extraction.

## Related Documentation

- [System Variable Patterns](patterns/system-variables.md) - Implementation patterns for system variables
- [Architecture Guide](architecture-guide.md) - Detailed architecture documentation
- [Issue Management](issue-management.md) - GitHub workflow and processes