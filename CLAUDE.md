# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

spanner-mycli is a personal fork of spanner-cli, designed as an interactive command-line tool for Google Cloud Spanner. The project philosophy is "by me, for me" - a continuously evolving tool that prioritizes the author's specific needs over stability. It embraces experimental features and follows a "ZeroVer" approach (will never reach v1.0.0).

## Quick Start for Development

### Essential Commands
```bash
# Building and running
make build          # Build the application
make run PROJECT=myproject INSTANCE=myinstance DATABASE=mydatabase
go run . -p PROJECT -i INSTANCE -d DATABASE  # Run directly with Go

# Testing (CRITICAL before push)
make test           # Run all tests including integration tests (REQUIRED)
make lint           # Run golangci-lint (REQUIRED)

# Fast development cycle
make fasttest       # Run tests excluding slow tests
make clean          # Clean build artifacts and test cache
```

**CRITICAL REQUIREMENTS before push**:
1. **Always run `make test`** (not `make fasttest`) to ensure all integration tests pass
2. **Always run `make lint`** to ensure code quality and style compliance

### Development Workflow with Phantom

We use [phantom](https://github.com/aku11i/phantom) for efficient Git worktree management:

```bash
# Start new work
phantom create issue-<number>-<brief-description>

# Open in tmux (vary layout based on workflow)
phantom shell issue-<number>-<brief-description> --tmux           # new window
phantom shell issue-<number>-<brief-description> --tmux-vertical  # vertical split  
phantom shell issue-<number>-<brief-description> --tmux-horizontal # horizontal split

# Work within the isolated worktree using Claude or preferred tools
# Each worktree maintains independent build artifacts and test state

# Record development knowledge (see Knowledge Management section below)
echo "Pattern discovered..." >> .worktree-knowledge.md

# Before cleanup, extract knowledge to CLAUDE.md
# Cleanup after PR merge
phantom delete issue-<number>-<brief-description>
```

#### Knowledge Management Workflow

To capture and integrate development insights from phantom worktrees:

1. **Record Knowledge During Development** (Broad Scope):
   ```bash
   # In each worktree, document all discoveries broadly
   cat > .worktree-knowledge.md << EOF
   # Issue #XXX Knowledge
   
   ## Development Phase
   [pre-PR / during-review / post-completion]
   
   ## Problem Category
   [Architecture/Development Flow/Testing Strategy/Code Quality/Tool Integration/etc.]
   
   ## Code-Level Insights (Consider for codebase documentation)
   - API design patterns discovered
   - Error handling patterns
   - Performance optimization techniques
   - Testing patterns specific to this component
   - Code organization principles
   
   ## Architecture Insights
   - System design patterns or principles discovered
   - Component interaction patterns
   - System boundary clarifications
   - Scalability considerations
   
   ## Development Flow Insights
   - Workflow pattern discovered or improved
   - Tool integration insights (phantom/tmux/git/CI)
   - Process efficiency improvements
   - Debugging techniques and strategies
   
   ## Testing Strategy Insights
   - Testing approach discoveries
   - Integration test patterns
   - Test organization improvements
   - Coverage strategy insights
   
   ## Code Review & Collaboration Insights
   - Review feedback patterns
   - Communication strategies with AI assistants
   - Conflict resolution approaches
   
   ## Tooling & Environment Insights
   - Editor/IDE optimizations
   - Build system discoveries
   - Local development environment improvements
   
   ## Codebase Integration Recommendations
   - Should this knowledge become code comments?
   - Should this become inline documentation?
   - Should this update existing code documentation?
   - Does this warrant updating README or docs/?
   
   ## CLAUDE.md Integration Suggestions
   - Target Section: [Which CLAUDE.md section should be updated]
   - Focus Area: [Architecture/Workflow/Testing/Process/Tooling]
   - Priority: [High/Medium/Low - for integration decision]
   EOF
   ```

2. **Knowledge Evolution During Development Phases**:
   ```bash
   # Pre-PR Phase: Focus on implementation discoveries
   echo "## Development Phase: pre-PR" >> .worktree-knowledge.md
   echo "- Implementation approach discoveries" >> .worktree-knowledge.md
   echo "- Initial architecture insights" >> .worktree-knowledge.md
   
   # During Review Phase: Capture review feedback and iteration insights
   echo "## Development Phase: during-review" >> .worktree-knowledge.md  
   echo "- Review feedback patterns and resolution strategies" >> .worktree-knowledge.md
   echo "- Code improvement insights from collaboration" >> .worktree-knowledge.md
   
   # Post-Completion Phase: Comprehensive reflection
   echo "## Development Phase: post-completion" >> .worktree-knowledge.md
   echo "- Final architecture insights and lessons learned" >> .worktree-knowledge.md
   echo "- Process optimization opportunities" >> .worktree-knowledge.md
   ```

3. **Knowledge Integration Decision Process**:
   ```bash
   # After successful PR merge, extract and categorize knowledge
   phantom exec issue-XXX-description "cat .worktree-knowledge.md 2>/dev/null || echo 'No knowledge recorded'"
   
   # Claude Code evaluates each insight for appropriate integration:
   # 1. Code-level insights → Consider for code comments, inline docs, or README updates
   # 2. Architecture insights → Potential CLAUDE.md integration 
   # 3. Process insights → CLAUDE.md workflow sections
   # 4. Tool insights → CLAUDE.md development practices
   ```

4. **Selective Integration and Clean Up**:
   ```bash
   # Strategic integration based on knowledge type and timing:
   
   # PRIORITY 1: Code-level insights requiring immediate action
   # - Should this knowledge become code comments before PR merge?
   # - Does this require README or docs/ updates as part of the feature PR?
   # - Are there API documentation updates needed?
   
   # PRIORITY 2: Architecture and process insights for CLAUDE.md
   # - High priority: System design patterns, critical workflow improvements
   # - Medium priority: Development efficiency insights, tool optimizations  
   # - Low priority: Minor process observations, environment tweaks
   
   # Before deletion, check for uncommitted changes
   phantom exec issue-XXX-description "git status --porcelain"
   
   # If .worktree-knowledge.md is the only uncommitted file, safe to delete
   phantom delete issue-XXX-description
   
   # Create separate documentation PR for CLAUDE.md updates only if substantial
   # Minor insights can be integrated directly without separate PR
   ```

**Enhanced Knowledge Template Generation**:
```bash
# Generate comprehensive template for new worktree
generate_knowledge_template() {
    local issue_number=$1
    cat > .worktree-knowledge.md << EOF
# Issue #${issue_number} Knowledge

## Development Phase
pre-PR

## Problem Category
[Architecture/Development Flow/Testing Strategy/Code Quality/Tool Integration/etc.]

## Code-Level Insights (Consider for codebase documentation)
- [API design patterns discovered]
- [Error handling patterns]
- [Performance optimization techniques]
- [Testing patterns specific to this component]

## Architecture Insights
- [System design patterns or principles discovered]
- [Component interaction patterns]
- [System boundary clarifications]

## Development Flow Insights
- [Workflow improvements or discoveries]
- [Tool integration insights (phantom/tmux/git/CI)]
- [Debugging techniques and strategies]

## Testing Strategy Insights
- [Testing approach improvements]
- [Test organization discoveries]
- [Integration test patterns]

## Code Review & Collaboration Insights
- [Review feedback patterns]
- [Communication strategies with AI assistants]

## Tooling & Environment Insights
- [Editor/IDE optimizations]
- [Build system discoveries]

## Codebase Integration Recommendations
- Should this knowledge become code comments? [Yes/No + reasoning]
- Should this become inline documentation? [Yes/No + where]
- Should this update existing code documentation? [Yes/No + which files]
- Does this warrant updating README or docs/? [Yes/No + sections]

## CLAUDE.md Integration Suggestions
- Target Section: [Architecture/Development Practices/Testing/etc.]
- Focus Area: [Architecture/Workflow/Testing/Process/Tooling]
- Priority: [High/Medium/Low]
EOF
}
```

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

### Configuration
- Config file: `.spanner_mycli.cnf` (home directory or current working directory)
- Environment variables: `SPANNER_PROJECT_ID`, `SPANNER_INSTANCE_ID`, `SPANNER_DATABASE_ID`

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