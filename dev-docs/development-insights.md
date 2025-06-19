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