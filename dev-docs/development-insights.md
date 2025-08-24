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

### Output Function Error Handling Pattern

**Discovery**: Output functions following "log and continue" pattern should be refactored to propagate errors

- **Design Pattern**: Use function types instead of interfaces for formatters to reduce boilerplate
- **Atomic Output**: Implement buffered writing that only outputs on success to prevent partial output on errors
- **Parameter Shadowing**: Use parameter shadowing in helper functions to prevent confusion between writers
- **Function Consolidation**: When converting from structs to functions, merge unnecessary function pairs (e.g., formatXML/writeXML)
- **Error Chain**: Establish clear error propagation: formatter → printTableData → printResult → displayResult → executeStatement

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

## Embedded Emulator Testing Strategies (Issue #277)

**Discovery**: Embedded emulator enables comprehensive feature testing without external dependencies

### Setup and Configuration

**Key Pattern**: No project/instance/database parameters needed for embedded emulator testing

```bash
# Standard embedded emulator testing setup
./spanner-mycli --embedded-emulator --async -t

# No need for:
# --project=test-project --instance=test-instance --database=test-db
```

### Testing Output and Documentation

**Pattern**: Use `-t` (table mode) for clean, readable output suitable for documentation

```bash
# Table mode provides clean output for PR documentation
./spanner-mycli --embedded-emulator -t --file=test-script.sql
```

**Benefits**:
- Clean, formatted output for documentation inclusion
- Consistent formatting across different environments
- Easy to copy-paste results into PR descriptions and documentation

### Feature Testing Patterns

**Multi-approach Testing**: Test both CLI flags and system variables for comprehensive coverage

```sql
-- Test 1: System variable approach
SHOW VARIABLE CLI_ASYNC_DDL;  -- Verify default (FALSE)
SET CLI_ASYNC_DDL = true;      -- Enable via system variable
CREATE TABLE test1 (id INT64) PRIMARY KEY (id);

-- Test 2: CLI flag approach (with --async)
SHOW VARIABLE CLI_ASYNC_DDL;  -- Should show TRUE with --async flag
CREATE TABLE test2 (id INT64) PRIMARY KEY (id);
```

### Environment Behavior Insights

**Discovery**: Embedded emulator operations complete immediately
- DDL operations show `DONE=true` instantly in embedded emulator
- Real production environments will show `DONE=false` initially
- Test format consistency, not timing behavior
- Focus on operation metadata structure and formatting

### Integration with CI/Testing

**Pattern**: Embedded emulator ideal for automated testing scenarios
- No external dependencies or authentication required
- Consistent behavior across different environments
- Fast execution suitable for CI pipelines
- Enables comprehensive feature testing without Spanner project costs

## CLI Flag Testing Insights

### Multi-Stage Validation Architecture
The flag validation happens in three distinct stages:
1. **Parsing Stage** (`parseFlags()`) - Basic syntax validation by go-flags
2. **Business Logic Validation** (`ValidateSpannerOptions()`) - Mutual exclusivity, required fields
3. **System Variable Initialization** (`initializeSystemVariables()`) - Value format validation, type conversions

**Key Learning**: Different error types occur at different stages. Tests must handle this multi-stage validation appropriately.

### Test Environment Best Practices

#### Parallel Test Safety
- Always use `t.Setenv()` instead of `os.Setenv()` for environment variables
- Avoids race conditions in parallel test execution
- Automatic cleanup prevents test pollution

#### Terminal Simulation with PTY
- Use pseudo-terminals (PTY) for accurate interactive mode testing
- Essential for testing features that depend on `term.IsTerminal()`
- Provides realistic terminal behavior in tests

#### Config File Testing Without Global State
- Parse config files directly using `flags.NewIniParser()`
- Avoids `os.Chdir()` which modifies global process state
- Prevents test flakiness and enables parallel execution

### Flag Design Inconsistencies Discovered

During comprehensive flag testing, several design inconsistencies were identified:

1. **Duplicate Flags with Hidden Aliases**
   - `--execute`/`--sql` and `--role`/`--database-role` pairs
   - Hidden flags still appear in error messages
   - Inconsistent handling of alias relationships

2. **Mutually Exclusive Aliases**
   - `--insecure` and `--skip-tls-verify` are aliases but marked as mutually exclusive
   - Creates user confusion since they do the same thing

3. **Silent Value Overrides**
   - `--embedded-emulator` silently overrides user-specified connection parameters
   - No warning given to users about ignored values

4. **Inconsistent Default Handling**
   - Mix of struct tag defaults and programmatic defaults
   - Makes it harder to understand actual default values

**Lesson**: Comprehensive testing reveals API design issues that might otherwise go unnoticed.

### Test Infrastructure Best Practices

During the comprehensive flag testing implementation, several important patterns emerged:

#### 1. Environment Variable Safety
- **Always use `t.Setenv()`** in tests, never `os.Setenv()`
- This prevents race conditions when tests run in parallel
- Automatic cleanup eliminates the need for complex defer statements
- Code review feedback highlighted this as a critical issue for test robustness

#### 2. Avoiding Global State Changes
- **Never use `os.Chdir()`** in tests - it affects all concurrent tests
- Instead, use absolute paths or parser-specific methods
- For config file parsing: use `flags.NewIniParser().ParseFile()` directly

#### 3. Test Helper Patterns
- **PTY helpers** enable accurate terminal detection testing
- **Table-driven tests** with clear test case names improve maintainability
- **Multi-stage validation** tests should mirror the actual validation flow

**Key Insight**: Test code should be written with future parallelization in mind, even if not currently using `t.Parallel()`. This future-proofs the test suite and prevents technical debt.

## Concurrency and Thread Safety Patterns

**Discovery**: CLI tools still need proper concurrency handling for background operations and future extensibility

### Mutex Usage Evolution (Issue #371)

The data race fix in Session.BeginReadWriteTransaction() led to comprehensive concurrency improvements:

1. **Initial Problem**: Simple mutex addition revealed deeper architectural issues
2. **Logical Race Conditions**: Check-and-act operations needed atomic execution
3. **Solution Pattern**: Closure-based helpers guarantee mutex scope

### Key Implementation Patterns

#### Closure-Based Resource Access
```go
// BAD: Race between check and use
if s.InReadWriteTransaction() {
    s.tc.txn.Update(...) // tc might be nil!
}

// GOOD: Atomic check-and-use with closure
err := s.withReadWriteTransaction(func(tx *spanner.ReadWriteStmtBasedTransaction) error {
    return tx.Update(...)
})
```

#### Heartbeat Optimization
- **Problem**: Constant mutex acquisition every 5 seconds caused contention
- **Solution**: Check-before-lock pattern using lock-free TransactionAttrs()
- **Result**: Reduced lock contention from periodic to only-when-needed

#### Testing Strategy for Unmockable Types
- **Challenge**: Spanner types have unexported fields, preventing mocking
- **Solution**: Two-tier testing approach
  - Unit tests: Error paths and mutex behavior
  - Integration tests: Real transaction behavior with emulator

### Lessons Learned

1. **Review-Driven Architecture**: Multiple review cycles led to better design
2. **Performance vs Correctness**: Mutex overhead negligible for CLI usage patterns
3. **Future-Proofing**: Proper concurrency patterns enable safe feature additions
4. **Documentation Importance**: Explaining "why mutex in a CLI" prevents future confusion

For implementation details, see:
- [Architecture Guide](architecture-guide.md) - Transaction management and concurrency patterns
- Session.go source code comments - Detailed implementation notes

## Mutex Deadlock Prevention in Transaction Management

**Discovery**: Go mutexes are not reentrant, requiring careful design to prevent deadlocks in callback-based architectures

### Problem Scenario

The deadlock occurs in DML execution paths through MCP:
1. `withTransactionLocked` acquires `tcMutex`
2. Calls a callback function (e.g., `runUpdateOnTransaction`)
3. Callback attempts to call `TransactionAttrs()` which tries to acquire `tcMutex` again
4. Deadlock occurs because Go mutexes are not reentrant

### Solution Pattern

Create locked variants of methods that don't acquire the mutex, for use within callbacks:
- `transactionAttrsLocked()` - Access transaction attributes without locking
- `currentPriorityLocked()` - Get RPC priority without locking  
- `queryOptionsLocked()` - Build query options without locking

### Implementation Guidelines

1. **Identify callback chains**: Trace execution paths to find where mutex is already held
2. **Create locked variants**: For any method called within a mutex-protected callback
3. **Document clearly**: Add comments explaining the deadlock scenario and why locked variant exists
4. **Naming convention**: 
   - `*WithLock()` suffix - Methods that acquire the lock internally
   - `*Locked()` suffix - Methods that assume caller holds the lock
   - This makes the locking behavior explicit and prevents misuse

### Testing Approach

- Use `TryLock` for deadlock detection during development
- Add debug logging to trace execution paths
- Run integration tests with race detector enabled
- Test DML operations through various execution paths (direct, MCP, batch)

## Related Documentation

- [System Variable Patterns](patterns/system-variables.md) - Implementation patterns for system variables
- [Architecture Guide](architecture-guide.md) - Detailed architecture documentation
- [Issue Management](issue-management.md) - GitHub workflow and processes