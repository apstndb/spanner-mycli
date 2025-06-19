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

## 🚨 CRITICAL REQUIREMENTS

**Before ANY push to the repository**:
1. **Always run `make check`** - runs test && lint (required for quality assurance)
2. **Resolve conflicts with origin/main** - ensure branch can merge cleanly to avoid integration issues
3. **Never push directly to main branch** - always use Pull Requests
4. **Never commit directly to main branch** - always use feature branches
5. **Repository merge policy**: This repository enforces **squash merge only** via Repository Ruleset - AI assistants must use `squash` method for all automated merges

## Essential Commands

```bash
# Development cycle (CRITICAL)
make check                    # REQUIRED before ANY push (runs test && lint)
make build                    # Build the application
make test-quick               # Quick tests during development

# Development tools (Go 1.24 tool management: make build-tools)
go tool gh-helper reviews analyze [PR]      # Comprehensive review analysis (prevents missing feedback)
go tool gh-helper reviews wait [PR]         # Wait for reviews + checks
go tool gh-helper reviews wait [PR] --request-review  # Request Gemini review + wait

# Workflow examples  
gh pr create                                 # Create PR (interactive for title/body)
go tool gh-helper reviews wait              # Wait for automatic Gemini review (initial PR only)

# Review response workflow (for subsequent pushes)
go tool gh-helper reviews analyze <PR> > tmp/review-analysis.yaml  # Analyze all feedback
# Create fix plan in tmp/fix-plan.md, make changes, commit & push
go tool gh-helper reviews wait <PR> --request-review # Request Gemini review
# Reply to threads with commit hash and --resolve flag

# Output format examples (YAML default, JSON with --json)
go tool gh-helper reviews analyze 306 | gojq --yaml-input '.summary.critical'
go tool gh-helper reviews fetch 306 --json | jq '.reviewThreads.needingReply[]'
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
- Presentation variables: Use `boolAccessor()` or `stringAccessor()`
- Session behavior variables: Use read-only `accessor{Getter: ...}`

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
- Worktree lifecycle management

### Knowledge Management
**Best Practice**: Record development insights directly in PR descriptions and comments for searchability and persistence.

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

## 🎯 Task-Specific Documentation Guide

### When implementing new features:
1. **ALWAYS check**: [dev-docs/architecture-guide.md](dev-docs/architecture-guide.md) - Understand system architecture
2. **For system variables**: [dev-docs/patterns/system-variables.md](dev-docs/patterns/system-variables.md) - Implementation patterns
3. **For client statements**: [dev-docs/architecture-guide.md#client-side-statement-system](dev-docs/architecture-guide.md#client-side-statement-system) - Statement definition patterns

### When working with GitHub issues/PRs:
1. **ALWAYS check**: [dev-docs/issue-management.md](dev-docs/issue-management.md) - Complete GitHub workflow
2. **For PR labels**: [dev-docs/issue-management.md#pull-request-labels](dev-docs/issue-management.md#pull-request-labels) - Release notes categorization
3. **For insights capture**: [dev-docs/issue-management.md#knowledge-management](dev-docs/issue-management.md#knowledge-management) - PR comment best practices
4. **For review analysis**: Use `go tool gh-helper reviews analyze` for comprehensive feedback analysis (prevents missing critical issues)
5. **For thread replies**: Use `go tool gh-helper threads reply` - Automated thread replies
6. **Safe Issue/PR content handling**: ALWAYS use stdin or variables for Issue/PR creation/updates as they commonly contain code blocks with backticks, quotes, and other special characters
7. **GitHub GraphQL API**: [docs.github.com/en/graphql](https://docs.github.com/en/graphql) - Official API documentation

**⚠️ CRITICAL: Safe handling of special characters in shell commands**
```bash
# Method 1: Variable + stdin (RECOMMENDED - no temp files)
content='Line 1 with `backticks`
Line 2 with '\''single quotes'\''  
Line 3 with "double quotes"'
echo "$content" | gh issue create --title "Title" --body-file -

# Method 2: Heredoc with stdin (for inline content)
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

**⚠️ CRITICAL: Use `go tool gh-helper reviews analyze` for comprehensive feedback analysis (review bodies + threads)**
**⚠️ WORKFLOW: Plan fixes → commit & push → reply with commit hash and resolve threads immediately**

### When encountering development problems:
1. **ALWAYS check**: [dev-docs/development-insights.md](dev-docs/development-insights.md) - Known patterns and solutions
2. **For error handling**: [dev-docs/development-insights.md#error-handling-architecture-evolution](dev-docs/development-insights.md#error-handling-architecture-evolution)
3. **For resource management**: [dev-docs/development-insights.md#resource-management-in-batch-processing](dev-docs/development-insights.md#resource-management-in-batch-processing)

### When setting up development environment:
1. **Start here**: Use `make worktree-setup WORKTREE_NAME=issue-123-feature` for worktree setup
2. **For workflow details**: [dev-docs/development-insights.md#parallel-issue-development-with-phantom](dev-docs/development-insights.md#parallel-issue-development-with-phantom)

### When updating documentation:
1. **README.md help updates**: Use `make docs-update`
2. **CLAUDE.md updates**: Follow the rules in this file (self-contained)
3. **Other docs**: See [dev-docs/README.md](dev-docs/README.md) for structure guidance

## Quick Reference

### Configuration
- **Config file**: `.spanner_mycli.cnf` (home dir, then current dir)
- **Environment**: `SPANNER_PROJECT_ID`, `SPANNER_INSTANCE_ID`, `SPANNER_DATABASE_ID`

### Testing
```bash
go test -short ./...    # Unit tests only
make test              # Full test suite (required before push)
make lint              # Code quality checks (required before push)
```

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
