# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

**IMPORTANT**: This file must be written entirely in English. Do not use Japanese or any other languages in CLAUDE.md.

## CLAUDE.md Update Rules

**CRITICAL RULE**: CLAUDE.md should only contain information that would cause problems if skipped. All other details belong in specialized documentation files.

**Why this rule is critical**: CLAUDE.md is read by AI assistants for every development task. Including verbose content creates cognitive overload and buries essential requirements like `make test && make lint`.

### What belongs in CLAUDE.md
- Critical requirements (e.g., `make test && make lint` before push)
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
1. **Always run `make test`** (not `make fasttest`) - all integration tests must pass
2. **Always run `make lint`** - code quality and style compliance required
3. **Never push directly to main branch** - always use Pull Requests
4. **Never commit directly to main branch** - always use feature branches

## Essential Commands

```bash
# Development cycle
make build                    # Build the application
make test && make lint        # Required before push
make fasttest                 # Quick tests during development

# Running the application
make run PROJECT=myproject INSTANCE=myinstance DATABASE=mydatabase
go run . -p PROJECT -i INSTANCE -d DATABASE

# Documentation updates
scripts/docs/update-help-output.sh    # Update README.md help sections

# Phantom worktree management (recommended)
scripts/dev/setup-phantom-worktree.sh issue-276-timeout-flag

# GitHub review replies (automated)
scripts/dev/list-review-threads.sh 287         # Find threads needing replies
scripts/dev/review-reply.sh THREAD_ID "reply"  # Reply to specific thread
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
scripts/dev/setup-phantom-worktree.sh issue-123-feature  # Auto-fetches and bases on origin/main

# Work in isolated environment
phantom shell issue-123-feature --tmux-horizontal

# Knowledge capture in PR comments (preferred)
gh pr comment <PR-number> --body "Development insights..."
```

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

### Automation Scripts (`scripts/`)
- **[docs/update-help-output.sh](scripts/docs/update-help-output.sh)** - Update README help sections
- **[dev/setup-phantom-worktree.sh](scripts/dev/setup-phantom-worktree.sh)** - Automated worktree setup

## 🎯 Task-Specific Documentation Guide

### When implementing new features:
1. **ALWAYS check**: [dev-docs/architecture-guide.md](dev-docs/architecture-guide.md) - Understand system architecture
2. **For system variables**: [dev-docs/patterns/system-variables.md](dev-docs/patterns/system-variables.md) - Implementation patterns
3. **For client statements**: [dev-docs/architecture-guide.md#client-side-statement-system](dev-docs/architecture-guide.md#client-side-statement-system) - Statement definition patterns

### When working with GitHub issues/PRs:
1. **ALWAYS check**: [dev-docs/issue-management.md](dev-docs/issue-management.md) - Complete GitHub workflow
2. **For insights capture**: [dev-docs/issue-management.md#knowledge-management](dev-docs/issue-management.md#knowledge-management) - PR comment best practices
3. **For review replies**: Use `scripts/dev/list-review-threads.sh` and `scripts/dev/review-reply.sh` - Automated thread replies
4. **GitHub GraphQL API**: [docs.github.com/en/graphql](https://docs.github.com/en/graphql) - Official API documentation

### When encountering development problems:
1. **ALWAYS check**: [dev-docs/development-insights.md](dev-docs/development-insights.md) - Known patterns and solutions
2. **For error handling**: [dev-docs/development-insights.md#error-handling-architecture-evolution](dev-docs/development-insights.md#error-handling-architecture-evolution)
3. **For resource management**: [dev-docs/development-insights.md#resource-management-in-batch-processing](dev-docs/development-insights.md#resource-management-in-batch-processing)

### When setting up development environment:
1. **Start here**: Use `scripts/dev/setup-phantom-worktree.sh` for worktree setup
2. **For workflow details**: [dev-docs/development-insights.md#parallel-issue-development-with-phantom](dev-docs/development-insights.md#parallel-issue-development-with-phantom)

### When updating documentation:
1. **README.md help updates**: Use `scripts/docs/update-help-output.sh`
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
- Always use `git add <specific-files>` (never `git add .`)
- Link PRs to issues: "Fixes #issue-number"
- Check `git status` before committing

## Important Notes

- **Backward Compatibility**: Not required since spanner-mycli is not used as an external library
- **Issue Management**: All fixes must go through Pull Requests - never close issues manually

For any detailed information not covered here, refer to the appropriate documentation in `dev-docs/` or `docs/`.
