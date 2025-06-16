# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

**IMPORTANT**: This file must be written entirely in English. Do not use Japanese or any other languages in CLAUDE.md.

## Project Overview

spanner-mycli is a personal fork of spanner-cli, designed as an interactive command-line tool for Google Cloud Spanner. The project philosophy is "by me, for me" - a continuously evolving tool that prioritizes the author's specific needs over stability. It embraces experimental features and follows a "ZeroVer" approach (will never reach v1.0.0).

## Quick Start for Development

**CRITICAL REQUIREMENTS before push**:
1. **Always run `make test`** (not `make fasttest`) - all integration tests must pass
2. **Always run `make lint`** - code quality and style compliance required

### Essential Commands
```bash
# Development cycle
make build                    # Build the application
make test && make lint        # Required before push
make fasttest                 # Quick tests during development
make clean                    # Clean artifacts when needed

# Running the application
make run PROJECT=myproject INSTANCE=myinstance DATABASE=mydatabase
go run . -p PROJECT -i INSTANCE -d DATABASE
```

### Development Workflow with Phantom

We use [phantom](https://github.com/aku11i/phantom) for efficient Git worktree management:

```bash
# Start new work with Claude settings synchronized
phantom create issue-<number>-<brief-description> --exec 'ln -sf ../../../../.claude .claude'

# Open in tmux (vary layout based on workflow)
phantom shell issue-<number>-<brief-description> --tmux           # new window
phantom shell issue-<number>-<brief-description> --tmux-vertical  # vertical split  
phantom shell issue-<number>-<brief-description> --tmux-horizontal # horizontal split

# Alternative: Create and open in one command (choose one layout)
phantom create issue-<number>-<brief-description> --exec 'ln -sf ../../../../.claude .claude && tmux split-window -h'

# Work within the isolated worktree using Claude or preferred tools
# Each worktree maintains independent build artifacts and test state

# Record development insights
echo "Discovery: Pattern XXX is effective" >> .notes.md

# Before cleanup, extract knowledge to CLAUDE.md
# IMPORTANT: Always check git status before deleting phantom worktree
phantom exec issue-<number>-<brief-description> git status
# Verify no unexpected file changes, then cleanup after PR merge
phantom delete issue-<number>-<brief-description>
```

#### Knowledge Management for Automated Integration

**Recommended Approach**: Record development insights in PR description and comments:

```bash
# Include development insights in PR description during creation
gh pr create --title "Fix: description" --body "$(cat <<'EOF'
## Summary
Brief description of changes...

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

Fixes #XXX
EOF
)"

# Add additional insights discovered during review process as comments
gh pr comment <PR-number> --body "$(cat <<'EOF'
## Review Process Insights

### Code Review Discoveries
- Pattern Y emerged during review discussion
- Alternative approach Z suggested by reviewer

### CI/Testing Insights
- Test failure revealed edge case in component A
EOF
)"
```

**Benefits of PR Comment Approach**:
- **Searchable**: GitHub search finds insights across all PRs
- **Contextual**: Insights linked directly to implementation
- **Persistent**: No worktree cleanup affects knowledge retention
- **Collaborative**: Team members can see and build on insights

**Alternative (.notes.md)**: Still useful for complex multi-session development or when working offline extensively

**Knowledge Management Evolution**:
- **Legacy Issue #243**: `.notes.md` files in worktrees required manual integration and were lost during cleanup
- **Improved Workflow**: PR comments provide persistent, searchable, contextual knowledge capture
- **Best Practice**: Add development insights as PR comments after implementation for future reference

## Architecture and Code Organization

### Core Components
- **main.go**: Entry point, CLI argument parsing, configuration management
- **cli.go**: Main interactive CLI interface and batch processing
- **session.go**: Database session management and Spanner client connections
- **statements.go**: Core SQL statement processing and execution
- **statements_*.go**: Specialized statement handlers (mutations, schema, explain, llm, proto)
- **client_side_statement_def.go**: **CRITICAL** - Defines all client-side statement patterns and handlers

### Client-Side Statement System
The `client_side_statement_def.go` file is the heart of spanner-mycli's extended SQL syntax:

- **clientSideStatementDef**: Structure defining regex patterns and handlers for custom statements
- **clientSideStatementDescription**: Human-readable documentation for each statement
- **Pattern Matching**: Uses compiled regex patterns for case-insensitive statement matching
- **Handler Functions**: Convert regex matches to structured Statement objects

Key statement categories:
- **Database Operations**: `USE`, `DROP DATABASE`, `SHOW DATABASES`, `DETACH`
- **Schema Operations**: `SHOW CREATE`, `SHOW TABLES`, `SHOW COLUMNS`, `SHOW INDEX`, `SHOW DDLS`
- **Query Analysis**: `EXPLAIN`, `EXPLAIN ANALYZE`, `DESCRIBE`, `SHOW PLAN NODE`
- **Transaction Control**: `BEGIN RW/RO`, `COMMIT`, `ROLLBACK`, `SET TRANSACTION`
- **System Variables**: `SET`, `SHOW VARIABLES`, `SHOW VARIABLE`
- **Advanced Features**: Protocol Buffers, GenAI, Partitioned Operations, Batching, Mutations

### Adding New Client-Side Statements
1. Add new entry to `clientSideStatementDefs` slice in `client_side_statement_def.go`
2. Define regex pattern with `(?is)` flags for case-insensitive matching
3. Create corresponding Statement struct and handler
4. Add implementation in appropriate `statements_*.go` file
5. Update tests and documentation

### Adding New System Variables
When exposing new system variables, follow these patterns established in `system_variables.go`:

1. **Naming Convention**: CLI-specific variables **MUST** use `CLI_` prefix to distinguish from java-spanner JDBC properties
2. **Access Control Design**:
   - **Presentation layer variables** (display, formatting): Use `boolAccessor()` or `stringAccessor()` for full read/write access
   - **Session behavior variables** (authentication, connection settings): Use explicit `accessor{Getter: ...}` for read-only access to prevent runtime session state changes
3. **Implementation Pattern**:
   ```go
   // Read/write variable
   "CLI_VARIABLE_NAME": {
       Description: "Clear description of purpose and default value",
       Accessor: boolAccessor(func(variables *systemVariables) *bool {
           return &variables.FieldName
       }),
   },
   
   // Read-only variable (session behavior)
   "CLI_SESSION_VARIABLE": {
       Description: "Clear description with read-only nature explained",
       Accessor: accessor{
           Getter: stringGetter(func(variables *systemVariables) *string {
               return &variables.FieldName
           }),
       },
   },
   ```
4. **Testing Requirements**: Add comprehensive test cases to `system_variables_test.go` covering both successful operations and proper error handling for unimplemented setters
5. **Documentation**: Include clear descriptions explaining purpose, default values, and any access restrictions

#### System Variable Design Patterns (Issue #243 Insights)
**Discovery**: Consistent patterns for system variable implementation improve maintainability and user experience
- **Pattern**: Use `boolAccessor()` and `stringAccessor()` for read/write variables, explicit `accessor{Getter: ...}` for read-only variables
- **Architecture**: Session behavior variables should be read-only to prevent runtime session state changes that could cause inconsistencies
- **Error Handling**: Proper use of `errSetterUnimplemented` and `errGetterUnimplemented` for consistent error reporting
- **Testing Strategy**: System variables test pattern in `system_variables_test.go` covers both SET/GET operations and proper error handling for unimplemented setters
- **Code Organization**: System variable definitions follow clear patterns that make adding new variables straightforward
- **Workflow**: phantom + tmux horizontal split works well for focused system variable implementation
- **Manual Testing**: Use `--embedded-emulator` only when Go test suite is insufficient - prefer comprehensive test cases in existing test suite

## Configuration

**Config file**: `.spanner_mycli.cnf` (searched in home directory, then current directory)
**Environment variables**: `SPANNER_PROJECT_ID`, `SPANNER_INSTANCE_ID`, `SPANNER_DATABASE_ID`

Example config:
```ini
[default]
project = myproject
instance = myinstance
database = mydatabase
```

## Development Practices

### Project Philosophy
- Embraces continuous change and experimentation
- Prioritizes author's specific needs over stability
- Allows testing of experimental features
- Follows "ZeroVer" version policy (will never reach v1.0.0)

### Development Flow Insights

#### Parallel Issue Development with Phantom
**Discovery**: Using phantom worktrees enables efficient parallel development of multiple low-hanging fruit issues
- **Workflow**: Create multiple worktrees simultaneously (`issue-245`, `issue-243`, `issue-241`, etc.)
- **tmux Integration**: Different layout strategies (window/horizontal/vertical) optimize screen space usage
- **Independent Testing**: Each worktree maintains separate build artifacts and test state
- **Context Switching**: No rebuild costs when switching between issues
- **Knowledge Capture**: `.worktree-knowledge.md` enables systematic capture of insights

#### Error Handling Architecture Evolution  
**Discovery**: Systematic replacement of panics with proper error handling improves system robustness
- **Architecture Impact**: Moving from panic-driven failure to graceful error propagation
- **System Boundary**: Clear distinction between "programming errors" (should panic) vs "runtime conditions" (should return errors)
- **Testing Strategy**: Comprehensive nil checks and error condition testing becomes critical
- **Maintenance**: Error handling patterns should be consistent across similar components

#### Systematic Error Detection and Quality Improvement
**Discovery**: Comprehensive error handling review reveals patterns and establishes sustainable practices
- **Detection Strategy**: Use `rg "_, _ ="` and `rg "ignoreError"` for systematic scanning of ignored errors
- **Three-Tier Improvement**: Silent failure → warning logs → contextual logging with relevant data
- **Contextual Information Priority**: Include SQL statements, proto type names, and operation context in error logs
- **Review-Driven Development**: AI assistant collaboration provides iterative improvement from basic fixes to optimization
- **Helper Function Elimination**: Remove error-hiding utilities (`ignoreError`) to enforce explicit handling
- **Log Efficiency**: Balance between debugging information and log volume (proto type names vs full messages)

#### System Robustness Through Panic Elimination
**Discovery**: Strategic panic removal requires systematic categorization and staged approach
- **Decision Criteria for panic vs error return**:
  - Programming errors (type violations, impossible states) → panic or log + graceful degradation
  - Runtime conditions (invalid input, configuration errors) → error return
  - External package code → preserve original behavior
- **Library Code Principles**: Avoid panics in library code, provide callers with choices
- **Systematic Approach**: Use `rg "panic\("` to scan → categorize by file/purpose → prioritize by impact
- **Parallel Development**: Consider PR conflicts when multiple issues address overlapping code areas
- **Consistency Patterns**: Maintain unified structured logging (slog) for better caller control

#### Resource Management in Batch Processing
**Discovery**: Proper resource cleanup requires careful defer placement and lifecycle management
- **defer Timing**: Place defer statements immediately after successful resource creation, not after subsequent operations
- **Batch Processing Complexity**: Partitioned query processing involves multiple goroutines sharing resources, requiring careful lifecycle management
- **Architecture Pattern**: Long-lived resources like `batchROTx` need both `Cleanup()` and `Close()` calls in proper sequence
- **Systematic Review**: Audit defer statement placement by checking if resource creation and cleanup reservation are adjacent
- **Error Path Safety**: Ensure cleanup occurs even when errors happen in subsequent processing steps

#### Systematic Code Quality Improvement (Issue #261 Insights)
**Discovery**: Re-enabling linters after fixing warnings often reveals additional hidden issues not in original scope
- **Linter Re-enablement Workflow**: Fix reported warnings → `make lint` → fix newly revealed warnings → re-enable in `.golangci.yml` → verify
- **Multiple Lint Run Requirement**: Critical practice when re-enabling linters as fixes can expose previously hidden warnings
- **Staticcheck Warning Categories**: QF series (optimization quick fixes), ST series (Go style compliance) - address by category for consistency

### Backward Compatibility
**spanner-mycli does not require traditional backward compatibility** since it's not used as an external library:
- **Clean refactoring over compatibility**: Prefer clear, well-named interfaces
- **Direct removal of old interfaces**: No need to maintain deprecated versions
- **Cleaner codebase**: No accumulation of deprecated interfaces or methods

### Testing Strategy
- Unit tests in `*_test.go` files
- Integration tests in `integration_test.go`
- Slow tests separated with `skip_slow_test` build tag
- Uses testcontainers for Spanner emulator testing
- Test data in `testdata/` directory

## Issue and Code Review Management

### Issue Management
- Issues managed through GitHub Issues with comprehensive labeling
- All fixes must go through Pull Requests - never close issues manually
- Use "claude-code" label for issues identified by automated code analysis
- Additional labels: bug, enhancement, documentation, testing, tech-debt, performance, blocked, concurrency

### Creating and Updating Issues
Use `gh` CLI for better control over formatting:

```bash
# Create issue
gh issue create --title "Issue title" --body "$(cat <<'EOF'
## Problem
Description...

## Expected vs Actual Behavior
What should happen vs what happens...

## Steps to Reproduce
1. Step one
2. Step two
EOF
)" --label "bug,enhancement"

# Update issue with comprehensive implementation plan
gh issue edit 209 --body "$(cat <<'EOF'
## Current Status
Analysis of current state...

## Implementation Plan
### Phase 1: Basic feature (independently mergeable)
- **System Variable**: `CLI_FEATURE_NAME` (default: false)
- **Implementation**: Location and approach
- **Testing**: Test strategy
EOF
)"
```

### Issue Review Workflow

**Implementation Planning Guidelines**:
- **DO NOT include time estimates** - they are meaningless for planning
- **Ensure phases are independently mergeable** - each phase should be a complete PR
- **Focus on spanner-mycli specific functionality** - distinguish from Spanner core features
- **Create system variables for new features** - follow existing patterns
- **Reference specific code locations**: Use `file_path:line_number` format

### Pull Request Process
- Link PRs to issues using "Fixes #issue-number" in commit messages and PR descriptions
- Use descriptive commit messages following conventional format
- Include clear description of changes and test plan
- Ensure `make test` and `make lint` pass before creating PR

### Code Review Response Strategy
1. **Address each comment individually** with focused commits
2. **Use descriptive commit messages** referencing specific issues
3. **Test thoroughly** before pushing changes
4. **For AI reviews (Gemini Code Assist)**: Use `/gemini review` to trigger re-review
5. **For praise comments**: Acknowledge briefly and resolve conversation

## Documentation Management

### Documentation Structure
- **README.md**: High-level feature overview with basic examples
- **docs/query_plan.md**: Comprehensive query plan feature documentation
- **CLAUDE.md**: Development guidelines and workflows (this file)

### Updating Help Output in README.md
```bash
# Generate help outputs with proper formatting
mkdir -p ./tmp
script -q ./tmp/help_output.txt sh -c "stty cols 200; go run . --help"
go run . --statement-help > ./tmp/statement_help.txt
sed '1s/^.\{2\}//' ./tmp/help_output.txt > ./tmp/help_clean.txt

# Update README.md sections with exact output
# Document source with generation commands in HTML comments
```

### Key Documentation Principles
- Ensure documentation matches actual behavior
- Use exact tool output without manual formatting
- Document generation source for maintainability
- Separate overview documentation from detailed technical docs

## Git and Development Practices

### Important Git Practices
- Always use `git add <specific-files>` instead of `git add .`
- Check `git status` before committing to verify only intended files are staged
- Use `git show --name-only` to review committed files

### Linking Issues and Pull Requests
Use GitHub's issue linking syntax in commit messages and PR descriptions:
- **Supported Keywords**: `Closes`, `Fixes`, `Resolves` (and variations)
- Include links in both commit messages and PR descriptions for automatic closure

### Dependencies
- Cloud Spanner SDK: `cloud.google.com/go/spanner`
- SQL Parser: `github.com/cloudspannerecosystem/memefish`
- CLI: `github.com/jessevdk/go-flags`, `github.com/nyaosorg/go-readline-ny`
- Output: `github.com/olekukonko/tablewriter`
- GenAI: `google.golang.org/genai`

#### go-flags Library Behavior (Issue #251 Insights)
**Discovery**: go-flags library uses struct field values as defaults in help text, not just `default` tags
- **Default Display Control**: Use `default-mask:"-"` struct tag to hide config/env values from help text defaults
- **Architecture Pattern**: Avoid creating multiple parser instances with shared structs to prevent config value leakage into help display
- **Testing Requirement**: Help text output verification important when modifying flag parsing logic