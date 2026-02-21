# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

**IMPORTANT**: This file must be written entirely in English. Do not use Japanese or any other languages in CLAUDE.md.

## CLAUDE.md Update Rules

**CRITICAL RULE**: CLAUDE.md should only contain information that would cause problems if skipped. All other details belong in specialized documentation files.

### What belongs in CLAUDE.md
- Critical requirements (e.g., `make check` before push)
- Essential commands for daily development
- Brief architecture overview with links to details
- Rules that, if skipped, cause real damage (data loss, broken builds, security issues)

### What does NOT belong in CLAUDE.md
- Detailed implementation patterns → `dev-docs/patterns/`
- Comprehensive architecture details → `dev-docs/architecture-guide.md`
- Development workflow details → `dev-docs/development-insights.md`
- Issue management procedures → `dev-docs/issue-management.md`
- Testing best practices → `dev-docs/patterns/testing.md`

## Project Overview

spanner-mycli is a personal fork of spanner-cli, an interactive CLI for Google Cloud Spanner. Philosophy: "by me, for me", ZeroVer (never reaching v1.0.0), experimental features welcome.

**Terminology**: OSS spanner-cli, Google Cloud Spanner CLI (`gcloud alpha spanner cli`), spannercli (`spannercli sql`), and spanner-mycli (this project) are all distinct. Be specific when comparing.

## CRITICAL REQUIREMENTS

**Before ANY push to the repository**:
1. **Always run `make check`** - runs test && lint && fmt-check
2. **Resolve conflicts with origin/main** - ensure branch can merge cleanly
3. **Never push/commit directly to main branch** - always use feature branches + PRs
4. **Squash merge only** - enforced via Repository Ruleset
5. **PR merge process**: Use `go tool gh-helper reviews wait` before merging (**DO NOT** use `--request-review`). Request `/gemini summary` only if additional commits were pushed after initial review.
6. **Squash merge commits**: MUST include descriptive summary of PR changes
7. **GitHub comment editing**: NEVER use `gh pr comment --edit-last` - always specify exact comment ID
8. **GitHub checks must pass**: All CI checks MUST pass before merging. Always investigate failures - never assume they are transient.

## Essential Commands

```bash
# Development cycle (CRITICAL)
make check                    # REQUIRED before ANY push (test + lint + fmt-check)
make build                    # Build the application
make test-quick               # Quick tests during development
make fmt                      # Format code

# Development tools (Go tool directive, managed via go.mod)
go tool gh-helper reviews fetch [PR]                    # Fetch review data
go tool gh-helper reviews fetch [PR] --unresolved-only  # Only unresolved threads
go tool gh-helper reviews wait [PR]                     # Wait for reviews + checks
go tool gh-helper threads reply THREAD_ID --commit-hash abc123 --resolve
go tool gh-helper issues show <N> --include-sub         # Show issue with sub-issues
go tool gh-helper issues edit <N> --parent <P>          # Link as sub-issue
go tool gh-helper labels add bug,enhancement --items 254,267
go tool gh-helper releases analyze --milestone v0.19.0
go tool github-schema type <TypeName>                   # GraphQL schema introspection
```

For full gh-helper command reference, see [dev-docs/issue-management.md](dev-docs/issue-management.md).

## Core Architecture Overview

### Critical Components
- **main.go**: Entry point, CLI argument parsing
- **session.go**: Database session management
- **statements.go**: Core SQL statement processing
- **system_variables.go**: System variable management
- **client_side_statement_def.go**: **CRITICAL** - Defines all client-side statement patterns

### System Variable Conventions
- CLI-specific variables **MUST** use `CLI_` prefix
- All registrations in `system_variables_registry.go`
- Details: [dev-docs/patterns/system-variables.md](dev-docs/patterns/system-variables.md)

### Regex Pattern Guidelines
- **Static patterns**: Precompile at package level (`var patternRe = regexp.MustCompile(...)`)
- **Dynamic patterns**: Compile at runtime, avoid caching unless profiling shows need

## Development Workflow

### Worktree Usage

Choose the right tool for the job:

| Use case | Tool |
|----------|------|
| Claude Code session (auto-cleanup) | `claude --worktree feature-auth` |
| tmux parallel work, manual editing | `make worktree-setup WORKTREE_NAME=issue-123-feature` |
| Start from GitHub Issue/PR | `phantom github checkout <number>` |
| Resume PR-linked session | `claude --from-pr <number>` |

```bash
# Phantom workflow
make worktree-setup WORKTREE_NAME=issue-123-feature  # Auto-fetches, bases on origin/main
phantom shell issue-123-feature --tmux-horizontal
phantom github checkout 248                           # Checkout issue/PR directly
```

**CRITICAL**: **NEVER delete a worktree without explicit user permission.** Unauthorized deletion can permanently lose uncommitted work. Always check `git status` first.

Details: [dev-docs/issue-management.md#phantom-worktree-management](dev-docs/issue-management.md#phantom-worktree-management)

## Refactoring Guidelines

- **Observe actual usage** before creating abstractions
- **Prefer simple helpers** over complex design patterns (15-line helper > 200-line factory)
- **No interfaces for single implementations**, no adapter patterns unless truly needed
- **Refactoring should reduce code**, not increase it (target: -20% in affected areas)
- **Single source of truth**: consolidate duplicate logic
- **Failure indicators**: more files added than removed, increased complexity

## GitHub Issues/PRs

- **Language**: ALL GitHub communications MUST be in English
- **Tool priority**: `gh-helper` → `gh` CLI → GitHub MCP
- **Review workflow**: `go tool gh-helper reviews fetch` for feedback analysis (CRITICAL - prevents missing issues). Plan fixes → commit & push → reply with commit hash and resolve threads.
- **Safe content handling**: ALWAYS use stdin, variables, or `--body-file` for content with special characters. NEVER pass backtick-containing strings directly in shell commands.
- **Documentation labels**: `docs-user` (README, docs/), `docs-dev` (dev-docs/, CLAUDE.md), `ignore-for-release` (dev-docs only PRs)
- **Gemini style guide** (`.gemini/styleguide.md`): MUST obtain user permission before modifying

## Quick Reference

### Configuration
- **Config file**: `.spanner_mycli.cnf` (home dir, then current dir)
- **Environment**: `SPANNER_PROJECT_ID`, `SPANNER_INSTANCE_ID`, `SPANNER_DATABASE_ID`

### Testing
```bash
go test -short ./...                         # Unit tests only
make test                                    # Full test suite (required before push)
go test -coverprofile=tmp/coverage.out ./... # Coverage profile
go tool cover -func=tmp/coverage.out         # Function-level summary
```

For testing best practices (table-driven tests, coverage comparison, test organization), see [dev-docs/patterns/testing.md](dev-docs/patterns/testing.md).

### Git Practices
- **CRITICAL**: Always use `git add <specific-files>` (never `git add .` or `git add -A`)
- **CRITICAL**: Never rewrite pushed commits without explicit permission
- Link PRs to issues: "Fixes #issue-number"
- Check `git status` before committing

## Important Notes

- **No backward compatibility required** - not used as an external library
- **No future-proofing** - only implement what's needed now
- **Issue management**: All fixes go through PRs - never close issues manually
- **License headers**: New files use "Copyright [year] apstndb"; files from spanner-cli keep "Copyright [year] Google LLC"

## Documentation Navigation

- [dev-docs/README.md](dev-docs/README.md) - Developer docs overview
- [dev-docs/architecture-guide.md](dev-docs/architecture-guide.md) - Architecture details
- [dev-docs/development-insights.md](dev-docs/development-insights.md) - Patterns and solutions
- [dev-docs/issue-management.md](dev-docs/issue-management.md) - GitHub workflow, PR processes
- [dev-docs/patterns/system-variables.md](dev-docs/patterns/system-variables.md) - System variable patterns
- [dev-docs/patterns/testing.md](dev-docs/patterns/testing.md) - Testing best practices
- [docs/query_plan.md](docs/query_plan.md) - Query plan features
- [docs/system_variables.md](docs/system_variables.md) - System variables reference