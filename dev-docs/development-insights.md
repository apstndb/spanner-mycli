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
bin/spanner-mycli-dev worktree setup issue-276-timeout-flag

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

**Discovery: Hard 2-Minute Timeout with No Grace Period**

Claude Code enforces a hard 2-minute timeout on bash commands, but this creates challenges for long-running operations like CI waiting.

**Technical Implementation**:
```go
// Use 90 seconds to provide 30-second safety margin
claudeCodeSafeTimeout := 1.5 // 90 seconds
if float64(effectiveTimeout) > claudeCodeSafeTimeout {
    fmt.Printf("⚠️  Claude Code has 2-minute timeout. Adjusting from %d to %.1f minutes for safety.\n", 
        timeout, claudeCodeSafeTimeout)
    effectiveTimeout = int(claudeCodeSafeTimeout * 60)
}
```

**Key Lessons**:
- Environment detection enables adaptive behavior
- Safety margins prevent mid-execution termination
- Clear user communication about constraint reasoning builds trust

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
💡 Then push and run: bin/gh-helper reviews wait 306
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
bin/spanner-mycli-dev docs update-help    # Generate help output for README.md
```

## AI-Friendly Tool Development (Issue #301 Insights)

### Key Design Principles Discovered

**1. stdin/heredoc vs Temporary Files**
AI assistants strongly prefer stdin input over temporary file creation:
```bash
# Preferred pattern (AI-friendly)
echo "Multi-line content" | bin/gh-helper threads reply THREAD_ID

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

## Related Documentation

- [System Variable Patterns](patterns/system-variables.md) - Implementation patterns for system variables
- [Architecture Guide](architecture-guide.md) - Detailed architecture documentation
- [Issue Management](issue-management.md) - GitHub workflow and processes