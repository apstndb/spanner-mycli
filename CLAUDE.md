# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

**IMPORTANT**: This file must be written entirely in English. Do not use Japanese or any other languages in CLAUDE.md.

## CLAUDE.md Update Rules

**CRITICAL RULE**: CLAUDE.md should only contain information that would cause problems if skipped. All other details belong in specialized documentation files.

**Why this rule is critical**: CLAUDE.md is read by AI assistants for every development task. Including verbose content creates cognitive overload and buries essential requirements like `make test && make lint`.

### What belongs in CLAUDE.md
- Critical requirements (e.g., `make check` before push)
- Essential commands for daily development
- Brief architecture overview with links to details
- Documentation structure navigation
- These update rules (self-contained)

### What does NOT belong in CLAUDE.md
- Detailed implementation patterns → `dev-docs/patterns/`
- Comprehensive architecture details → `dev-docs/architecture-guide.md`
- Development workflow details → `dev-docs/development-insights.md`
- Issue management procedures → `dev-docs/issue-management.md`

## Project Overview

spanner-mycli is a personal fork of spanner-cli, designed as an interactive command-line tool for Google Cloud Spanner. The project philosophy is "by me, for me" - a continuously evolving tool that prioritizes the author's specific needs over stability. It embraces experimental features and follows a "ZeroVer" approach (will never reach v1.0.0).

### Terminology Clarification

- **OSS spanner-cli**: The open-source `spanner-cli` command (https://github.com/cloudspannerecosystem/spanner-cli)
- **Google Cloud Spanner CLI**: `gcloud alpha spanner cli` - documented as part of gcloud (https://cloud.google.com/spanner/docs/spanner-cli)
- **spannercli**: The undocumented binary (`spannercli sql`) that implements the actual Google Cloud Spanner CLI functionality
- **spanner-mycli**: This project, a fork of OSS spanner-cli

When testing compatibility or referencing behavior, be specific about which implementation you're comparing against.

## 🚨 CRITICAL REQUIREMENTS

**Before ANY push to the repository**:
1. **Always run `make check`** - runs test && lint (required for quality assurance)
2. **Resolve conflicts with origin/main** - ensure branch can merge cleanly to avoid integration issues
3. **Never push directly to main branch** - always use Pull Requests
4. **Never commit directly to main branch** - always use feature branches
5. **Repository merge policy**: This repository enforces **squash merge only** via Repository Ruleset - AI assistants must use `squash` method for all automated merges
6. **PR merge process**: Before merging, if additional commits have been pushed since the initial review, request an updated summary by commenting `/gemini summary`. An initial summary is generated automatically, so this is only for updates. Then, use `go tool gh-helper reviews wait` (**DO NOT** use `--request-review`)
7. **Squash merge commits**: MUST include descriptive summary of PR changes in squash commit message
8. **GitHub comment editing**: NEVER use `gh pr comment --edit-last` - always specify the exact comment ID to avoid editing the wrong comment
9. **GitHub checks must pass**: All GitHub Actions checks MUST pass before merging AND during review cycles. Even if local tests pass, always investigate failing GitHub checks - do NOT assume failures are unrelated to PR changes or transient. Fix all check failures immediately to ensure reviewers can properly assess changes and prevent accumulating technical debt.

## Essential Commands

> [!NOTE]
> **Note for Human Developers**: The `go tool gh-helper` and its associated workflows are primarily designed for automation by AI assistants to ensure consistent and error-free execution of repository management tasks. While human developers can use these tools, they are not a strict requirement for manual contributions.


```bash
# Development cycle (CRITICAL)
make check                    # REQUIRED before ANY push (runs test && lint && fmt-check)
make build                    # Build the application
make test-quick               # Quick tests during development
make fmt                      # Format code with gofmt, goimports, and gofumpt
make help-dev                 # Show all available development commands

# Development tools (Go 1.24 tool management: make build-tools)
go tool gh-helper reviews fetch [PR]        # Fetch review data including threads
go tool gh-helper reviews wait [PR]         # Wait for reviews + checks
go tool gh-helper reviews wait [PR] --request-review  # Request Gemini review + wait

# Workflow examples  
gh pr create                                 # Create PR (interactive for title/body)
go tool gh-helper reviews wait              # Wait for automatic Gemini review (initial PR only)
go tool gh-helper reviews wait --async      # Check reviews once (non-blocking)
go tool gh-helper reviews wait --timeout 15m # Wait with custom timeout

# Issue management with gh-helper
go tool gh-helper issues create --title "Title" --body "Body"  # Create issue
go tool gh-helper issues create --parent 123 --title "Sub-task"  # Create sub-issue
go tool gh-helper issues edit 456 --parent 123  # Link existing issue as sub-issue
go tool gh-helper issues edit 456 --unlink-parent  # Remove parent relationship
go tool gh-helper issues show 248 --include-sub  # Show issue with sub-issues stats

# Review response workflow (for subsequent pushes)
go tool gh-helper reviews fetch <PR> > tmp/review-data.yaml  # Fetch all review data
go tool gh-helper reviews fetch <PR> --threads-only          # Only fetch thread data
go tool gh-helper reviews fetch <PR> --unresolved-only       # Only unresolved threads
go tool gh-helper reviews fetch <PR> --exclude-urls          # Exclude URLs from output
# Create fix plan in tmp/fix-plan.md, make changes, commit & push
go tool gh-helper reviews wait <PR> --request-review # Request Gemini review
# Reply to threads with commit hash and --resolve flag
go tool gh-helper threads reply THREAD_ID --commit-hash abc123 --resolve  # Standard workflow
# Or use new batch resolve: go tool gh-helper threads resolve THREAD_ID1 THREAD_ID2
# Show multiple threads at once: go tool gh-helper threads show THREAD_ID1 THREAD_ID2
# Show thread details (supports multiple threads)
go tool gh-helper threads show THREAD_ID1 THREAD_ID2 THREAD_ID3

# Label management (bulk operations)
go tool gh-helper labels add bug,enhancement --items 254,267,238,245  # Add to multiple items
go tool gh-helper labels remove needs-review --items pull/302,issue/301  # Remove from items
go tool gh-helper labels add-from-issues --pr 254  # Inherit labels from linked issues

# Release notes analysis
go tool gh-helper releases analyze --milestone v0.19.0  # Analyze by milestone
go tool gh-helper releases analyze --since 2024-01-01 --until 2024-01-31  # By date range

# Output format examples (YAML default, JSON with --json)
go tool gh-helper reviews fetch 306 | gojq --yaml-input '.threads[] | select(.needsReply)'
go tool gh-helper reviews fetch 306 --json | jq '.threads[] | select(.needsReply) | .id'

# New: Built-in jq filtering (no external tools needed)
go tool gh-helper reviews fetch 306 --jq '.threads[] | select(.needsReply)'
go tool gh-helper node-id issue/123 pull/456  # Get GraphQL node IDs
```

## Core Architecture Overview

### Critical Components
- **main.go**: Entry point, CLI argument parsing
- **session.go**: Database session management
- **statements.go**: Core SQL statement processing
- **system_variables.go**: System variable management
- **client_side_statement_def.go**: **CRITICAL** - Defines all client-side statement patterns

### System Variable Conventions
- CLI-specific variables **MUST** use `CLI_` prefix
- All variable registrations are in `system_variables_registry.go`
- Use generic VarHandler types for type-safe variable handling

For detailed implementation patterns, see [dev-docs/patterns/system-variables.md](dev-docs/patterns/system-variables.md).

## Development Workflow

### Phantom Worktree Usage
```bash
# Automated setup (recommended)
make worktree-setup WORKTREE_NAME=issue-123-feature  # Auto-fetches and bases on origin/main

# Work in isolated environment
phantom shell issue-123-feature --tmux-horizontal

# Knowledge capture in PR comments (preferred)
gh pr comment <PR-number> --body "Development insights..."
```

**IMPORTANT**: When working with phantom worktrees, **ALWAYS refer to** [dev-docs/issue-management.md#phantom-worktree-management](dev-docs/issue-management.md#phantom-worktree-management) for detailed guidelines including:
- AI assistant decision matrix for autonomous vs. user-permission operations
- Comprehensive deletion safety rules

⚠️ **CRITICAL RULE - WORKTREE DELETION**: 
**NEVER delete a worktree without explicit user permission** - This is non-negotiable. Unauthorized deletion can result in permanent loss of uncommitted work. Always use `git status` to check for changes and ask: "Issue #X worktree exists. Should I delete it? (Note: uncommitted changes will be lost)"
- Worktree lifecycle management

### Knowledge Management
**Best Practice**: Record development insights directly in PR descriptions and comments for searchability and persistence.

## 🎯 Refactoring Guidelines (Avoiding Over-engineering)

When refactoring code, follow these principles to avoid over-engineering:

### 1. **Observe Actual Usage Patterns**
- Analyze how code is actually used before creating abstractions
- Example: Don't create complex metadata handling if functions always pass `nil`

### 2. **Prefer Simple Helper Functions**
- Use straightforward helper functions over complex design patterns
- A 15-line helper function is better than a 200-line factory pattern
- Example: `executeWithFormatter()` instead of `BufferedFormatterAdapter`

### 3. **Minimize Abstraction Layers**
- Only add abstraction when it provides clear, measurable benefits
- Avoid adapter patterns unless converting between incompatible interfaces
- Don't create interfaces for single implementations

### 4. **Prioritize Code Reduction**
- Refactoring should reduce total lines of code, not increase them
- Target: Net reduction of at least 20% in affected areas
- If adding abstraction increases code, reconsider the approach

### 5. **Single Source of Truth**
- Consolidate duplicate logic into one location
- Example: One `createStreamingFormatter()` function instead of multiple switch statements

### 6. **Measure Success by Simplicity**
- Success metrics:
  - Lines of code reduced
  - Duplicate logic eliminated
  - Easier to test
  - Easier to understand
- Failure indicators:
  - More files added than removed
  - Increased complexity
  - Loss of type information

### Example Application
```go
// ❌ Over-engineered: 370 lines for a factory pattern
type FormatterFactory struct { ... }
type BaseFormatter struct { ... }
type BufferedFormatterAdapter struct { ... }

// ✅ Simple: 45 lines for helper functions
func createStreamingFormatter(mode, out, sysVars) { ... }
func executeWithFormatter(formatter, result, columns, sysVars) { ... }
```

## 📚 Documentation Structure

This is a simplified guide. For detailed information, refer to:

### Developer Documentation (`dev-docs/`)
- **[README.md](dev-docs/README.md)** - Overview and navigation for developer docs
- **[architecture-guide.md](dev-docs/architecture-guide.md)** - Detailed architecture and code organization
- **[development-insights.md](dev-docs/development-insights.md)** - Development patterns and best practices
- **[issue-management.md](dev-docs/issue-management.md)** - GitHub workflow and PR processes
- **[patterns/system-variables.md](dev-docs/patterns/system-variables.md)** - System variable implementation patterns

### User Documentation (`docs/`)
- **[query_plan.md](docs/query_plan.md)** - Query plan analysis features
- **[system_variables.md](docs/system_variables.md)** - System variables reference

### Development Tools **Go 1.24 Tool Management: `make build-tools`**
- **gh-helper** - Generic GitHub operations (managed via go.mod tool directive)
- **github-schema** - GitHub GraphQL schema introspection (managed via go.mod tool directive)

## 🎯 Task-Specific Documentation Guide

### When implementing new features:
1. **ALWAYS check**: [dev-docs/architecture-guide.md](dev-docs/architecture-guide.md) - Understand system architecture
2. **For system variables**: [dev-docs/patterns/system-variables.md](dev-docs/patterns/system-variables.md) - Implementation patterns
3. **For client statements**: [dev-docs/architecture-guide.md#client-side-statement-system](dev-docs/architecture-guide.md#client-side-statement-system) - Statement definition patterns
4. **Code documentation**: When code review feedback indicates confusion about design decisions:
   - Add clarifying comments directly in the source code
   - Document the rationale for non-obvious choices
   - Example: If a function returns "unknown" instead of an error, explain why in comments

### When working with GitHub issues/PRs:
1. **ALWAYS check**: [dev-docs/issue-management.md](dev-docs/issue-management.md) - Complete GitHub workflow
2. **Language requirement**: ALL GitHub communications (PRs, issues, comments, commit messages) MUST be in English
3. **For PR labels**: [dev-docs/issue-management.md#pull-request-labels](dev-docs/issue-management.md#pull-request-labels) - Release notes categorization
4. **For insights capture**: [dev-docs/issue-management.md#knowledge-management](dev-docs/issue-management.md#knowledge-management) - PR comment best practices
5. **For review analysis**: Use `go tool gh-helper reviews fetch` for comprehensive feedback analysis (prevents missing critical issues)
6. **For thread replies**: Use `go tool gh-helper threads reply` - Automated thread replies
7. **GitHub operation priority**: Use tools in this order: `gh-helper` → `gh` command → GitHub MCP (API calls)
8. **Sub-issue operations**: Use `gh-helper issues` commands instead of GraphQL:
   - `issues show <parent> --include-sub` - List sub-issues and check completion
   - `issues edit <issue> --parent <parent>` - Link as sub-issue (replaces deprecated `link-parent`)
   - `issues edit <issue> --parent <new> --overwrite` - Move to different parent
   - `issues edit <issue> --unlink-parent` - Remove parent relationship
   - `issues edit <issue> --after <other>` - Reorder sub-issue after another
   - `issues edit <issue> --position first` - Move sub-issue to beginning
9. **Safe Issue/PR content handling**: ALWAYS use stdin or variables for Issue/PR creation/updates as they commonly contain code blocks with special characters (e.g., backticks, quotes, dollar signs, parentheses)
10. **GitHub GraphQL API**: [docs.github.com/en/graphql](https://docs.github.com/en/graphql) - Now only needed for:
    - Complex custom field selections beyond gh-helper's output
    - Advanced queries requiring specific field combinations
11. **Schema introspection**: Use `go tool github-schema` instead of GraphQL:
   - `go tool github-schema type <TypeName>` - Show type fields and descriptions
   - `go tool github-schema mutation <MutationName>` - Show mutation requirements
   - `go tool github-schema search <pattern>` - Search for types/fields

**⚠️ CRITICAL: Safe handling of special characters in shell commands**
```bash
# Method 1: Variable + stdin (RECOMMENDED - no temp files)
content='Line 1 with `backticks`
Line 2 with '\''single quotes'\''  
Line 3 with "double quotes"'
echo "$content" | gh issue create --title "Title" --body-file -

# Method 2: Heredoc with stdin (for inline content)
# WARNING: Claude Code's bash tool may incorrectly handle quoted heredocs (<<'EOF')
# If backslashes are being doubled unexpectedly, use Method 1 or temp files instead
cat <<'EOF' | gh issue create --title "Title" --body-file -
Content with `backticks` and "quotes"
EOF

# Method 3: Variable for command-line args
some_command --message "$content"  # Safe: special chars not evaluated

# If temp files needed, use tmp/ directory
echo "$content" > tmp/issue_body.md
gh issue create --body-file tmp/issue_body.md

# AVOID: Direct strings with special characters
# command --message "Content with `backticks`"  # UNSAFE - backticks execute
```

**⚠️ CRITICAL: ALWAYS use `go tool gh-helper reviews fetch` for comprehensive feedback analysis. Failing to do so may result in missing critical issues!**
**⚠️ WORKFLOW: Plan fixes → commit & push → reply with commit hash and resolve threads immediately**

### When encountering development problems:
1. **ALWAYS check**: [dev-docs/development-insights.md](dev-docs/development-insights.md) - Known patterns and solutions
2. **For error handling**: [dev-docs/development-insights.md#error-handling-architecture-evolution](dev-docs/development-insights.md#error-handling-architecture-evolution)
3. **For resource management**: [dev-docs/development-insights.md#resource-management-in-batch-processing](dev-docs/development-insights.md#resource-management-in-batch-processing)
4. **Use `go doc` commands**: When investigating Go types, interfaces, or package APIs:
   - `go doc <package>` - Show package documentation
   - `go doc <package>.<type>` - Show specific type documentation
   - `go doc -all <package>` - Show ALL exported identifiers (don't miss anything!)
   - Example: `go doc github.com/testcontainers/testcontainers-go.GenericProvider`
   - Example: `go doc -all github.com/testcontainers/testcontainers-go | grep Provider`

### When setting up development environment:
1. **Start here**: Use `make worktree-setup WORKTREE_NAME=issue-123-feature` for worktree setup
2. **For workflow details**: [dev-docs/development-insights.md#parallel-issue-development-with-phantom](dev-docs/development-insights.md#parallel-issue-development-with-phantom)

### When updating documentation:
1. **README.md help updates**: Use `make docs-update`
2. **CLAUDE.md updates**: Follow the rules in this file (self-contained)
3. **Other docs**: See [dev-docs/README.md](dev-docs/README.md) for structure guidance
4. **Documentation labels**: 
   - Use `docs-user` for user-facing docs (README.md, docs/)
   - Use `docs-dev` for internal docs (dev-docs/, CLAUDE.md)
   - Apply `ignore-for-release` to PRs with changes *only* in dev-docs/
5. **Gemini Code Assist style guide**: `.gemini/styleguide.md` - Update when encountering repeated false positives or outdated suggestions from Gemini Code Assist reviews. This helps improve the quality of automated code reviews.
   - **IMPORTANT**: AI assistants MUST obtain user permission before modifying styleguide.md
   - **Process**: 1) Identify repeated false positives, 2) Propose specific changes with rationale, 3) Wait for user approval, 4) Only then implement the changes
   - **Example**: "I noticed Gemini repeatedly suggests X which is outdated since Go 1.22. May I update the style guide to clarify this?"

## Quick Reference

### Configuration
- **Config file**: `.spanner_mycli.cnf` (home dir, then current dir)
- **Environment**: `SPANNER_PROJECT_ID`, `SPANNER_INSTANCE_ID`, `SPANNER_DATABASE_ID`

### Testing
```bash
go test -short ./...    # Unit tests only
make test              # Full test suite (required before push)
make lint              # Code quality checks (required before push)

# Coverage analysis
go test -cover ./...                         # Show coverage percentages
go test -coverprofile=tmp/coverage.out ./... # Generate coverage profile
go tool cover -func=tmp/coverage.out          # Function-level coverage summary (AI-friendly)
go tool cover -html=tmp/coverage.out         # Generate HTML coverage report (detailed line-by-line)
```

**Coverage Analysis for AI Assistants**: 
- Use `go tool cover -func` for a quick, readable summary of function-level coverage percentages
- For detailed line-by-line coverage analysis, generate HTML with `go tool cover -html` and read it as text
- The HTML report shows exactly which lines and branches are covered/uncovered, making it ideal for identifying specific gaps in test coverage

**Test File Organization**: 
- Each production code file (`*.go`) should have a corresponding test file (`*_test.go`)
- Integration tests should be consolidated into existing `integration_*_test.go` files when possible
- Avoid creating new `integration_*_test.go` files unless testing a completely new feature area
- This keeps the test structure manageable and reduces file proliferation

### Git Practices
- **CRITICAL**: Always use `git add <specific-files>` (never `git add .` or `git add -A`)
- **Reason**: Prevents accidental commits of untracked files (.claude, .idea/, tmp/, etc.)
- **CRITICAL**: Never rewrite pushed commits without explicit permission - preserves review history
- Link PRs to issues: "Fixes #issue-number"
- Check `git status` before committing to verify only intended files are staged


## Important Notes

- **Backward Compatibility**: Not required since spanner-mycli is not used as an external library
- **Issue Management**: All fixes must go through Pull Requests - never close issues manually

For any detailed information not covered here, refer to the appropriate documentation in `dev-docs/` or `docs/`.
