# spanner-mycli-dev

Project-specific development tools for spanner-mycli.

## Quick Start

```bash
# Complete PR workflow (recommended)
spanner-mycli-dev pr-workflow create --wait-checks
spanner-mycli-dev pr-workflow review <PR> --wait-checks

# Worktree management
spanner-mycli-dev worktree setup issue-123-feature
phantom shell issue-123-feature --tmux-horizontal
```

## Design Philosophy

### Project-Specific Focus

Unlike gh-helper (generic GitHub operations), spanner-mycli-dev handles:
- spanner-mycli specific conventions (phantom worktrees)
- Gemini Code Review integration (project-specific bot)
- Documentation generation workflows
- Complete PR lifecycle management

### Integration with gh-helper

spanner-mycli-dev delegates to gh-helper for:
- Review waiting (`--wait-checks` flag)
- Thread listing after workflows complete
- Review request triggering

This separation maintains clean boundaries while providing integrated experience.

## Commands Overview

### pr-workflow

**Purpose**: Complete PR lifecycle management

```bash
# Create new PR and wait for initial review
pr-workflow create [--title "..."] [--body "..."] [--wait-checks]

# Handle review cycle after pushes
pr-workflow review <PR> [--wait-checks]
```

**Default behavior**: Waits for reviews only (Gemini auto-reviews initial PRs)
**With `--wait-checks`**: Also waits for CI checks completion

### worktree

**Purpose**: Phantom worktree management with spanner-mycli conventions

```bash
# Setup worktree with automatic configuration
worktree setup <NAME>

# List existing worktrees
worktree list

# Safe deletion with checks
worktree delete <NAME> [--force]
```

**Automatic setup includes**:
- Fetch latest from origin/main
- Create phantom worktree based on origin/main
- Symlink Claude configuration (`.claude` → `../../../../.claude`)

### docs

**Purpose**: Documentation maintenance workflows

```bash
# Generate help output for README.md
docs update-help
```

**Output files** in `./tmp/`:
- `help_clean.txt`: `--help` output for README.md
- `statement_help.txt`: `--statement-help` output for README.md

### review

**Purpose**: Smart review workflows with auto-detection

```bash
# Smart detection of initial vs subsequent pushes
review gemini <PR> [--force-request] [--wait-checks]
```

**Auto-detection logic**:
- Compares PR creation time vs latest commit time
- Checks commit messages for review-related keywords (`fix:`, `address`, `review`)
- `>5 minute` difference suggests post-creation push

## Gemini Code Review Integration

### spanner-mycli Specific Behavior

spanner-mycli uses `gemini-code-assist` bot with specific patterns:

**Initial PR**: Gemini automatically reviews (no trigger needed)
**Post-push**: Manual `/gemini review` trigger required

### Workflow Commands

```bash
# Auto-detection (recommended)
review gemini 306

# Force request (override detection)
review gemini 306 --force-request

# Include CI checks in wait
review gemini 306 --wait-checks
```

### Complete Gemini Workflow

```bash
# 1. Auto-detect scenario and execute appropriate workflow
spanner-mycli-dev review gemini 306 --wait-checks

# What it does:
# - Detects if review request needed (based on timing/commits)
# - Requests /gemini review if needed
# - Waits for both review feedback AND CI checks
# - Lists any unresolved threads for handling
```

## Phantom Worktree Integration

### spanner-mycli Conventions

**Directory structure**:
```
.git/phantom/worktrees/
├── issue-123-feature/        # Worktree directory
│   ├── .claude -> ../../../../.claude  # Symlinked configuration
│   └── ...                   # Project files
```

**Setup process**:
1. `git fetch origin` (ensure latest)
2. `phantom create <name> --base origin/main`
3. `ln -sf ../../../../.claude .claude` (Claude configuration)
4. Provide next steps with tmux integration

### Workflow Integration

```bash
# Typical development cycle
spanner-mycli-dev worktree setup issue-123-feature

# Work in isolated environment
phantom shell issue-123-feature --tmux-horizontal

# When ready, create PR
spanner-mycli-dev pr-workflow create --wait-checks

# After review feedback, handle responses
gh-helper threads list $(gh pr view --json number -q .number)
gh-helper threads reply-commit <THREAD_ID> <COMMIT_HASH>

# Clean up when done
phantom exec issue-123-feature git status  # Verify clean
phantom delete issue-123-feature
```

## Documentation Workflows

### README.md Help Generation

The `docs update-help` command automates help section updates:

**Process**:
1. Generate `--help` output with proper terminal width
2. Generate `--statement-help` output
3. Clean up formatting (remove script command artifacts)
4. Write to `./tmp/` for manual integration

**Manual integration required**: The tool generates content but doesn't modify README.md directly to avoid conflicts.

### Development Insights Capture

**Best practice**: Record insights in PR comments for searchability:

```bash
gh pr comment <PR> --body "$(cat <<'EOF'
## Development Insights

### Patterns Discovered
- Error handling approach for GraphQL mutations
- State tracking pattern for review monitoring

### Process Improvements
- phantom + tmux horizontal split optimal for this workflow
- 5-minute timeout covers 95% of real cases
EOF
)"
```

## Auto-Detection Algorithm

### PR Creation vs Post-Push Detection

Used by `review gemini` command to determine if `/gemini review` trigger needed:

**Criteria**:
1. **Time diff**: Latest commit >5 minutes after PR creation
2. **Commit messages**: Contain keywords (`fix:`, `address`, `review`, `feedback`)

**GraphQL query**:
```graphql
{
  repository(owner: "apstndb", name: "spanner-mycli") {
    pullRequest(number: 306) {
      createdAt
      headRef {
        target {
          committedDate
          history(first: 3) {
            nodes { committedDate message }
          }
        }
      }
    }
  }
}
```

**Output example**:
```
🔍 Auto-detecting if Gemini review request is needed...
   PR created: 17:08:38
   Latest commit: 17:35:15
   Time difference: 26m37s
   Found review-related commit: fix: address review feedback
✅ Detected: This appears to be after pushes to existing PR
```

## Error Handling

### Worktree Safety Checks

**Before deletion**:
```bash
# Check for uncommitted changes
phantom exec <NAME> git status --porcelain

# If changes found:
❌ Uncommitted changes found:
M  file1.go
?? temp.txt

Please commit or stash changes before deletion, or use --force
```

**Safety mechanisms**:
- Uncommitted changes detection
- Untracked files preservation  
- `--force` flag for override scenarios

### Tool Integration Errors

**gh-helper not found**:
```bash
# Graceful fallback to PATH
ghHelperPath := "./bin/gh-helper"
if !fileExists(ghHelperPath) {
    ghHelperPath = "gh-helper"  # Use PATH
}
```

**Permission errors**: Clear error messages with actionable advice

## Configuration

### File Structure

```
cmd/spanner-mycli-dev/
├── main.go           # Main command structure
├── README.md         # This file
└── go.mod           # Module dependencies
```

### Build Integration

**Makefile targets**:
```bash
make build-tools      # Build spanner-mycli-dev + gh-helper
make worktree-setup WORKTREE_NAME=issue-123  # Legacy wrapper
make docs-update      # Generate documentation
```

## Examples

### Complete Development Cycle

```bash
# 1. Setup development environment
spanner-mycli-dev worktree setup issue-123-feature

# 2. Work in isolated environment  
phantom shell issue-123-feature --tmux-horizontal

# 3. Create PR when ready
spanner-mycli-dev pr-workflow create \
  --title "feat: implement feature X" \
  --body "Description of changes" \
  --wait-checks

# 4. Handle review feedback
gh-helper threads list $(gh pr view --json number -q .number)
gh-helper threads reply-commit <THREAD_ID> <COMMIT_HASH>

# 5. Continue review cycles as needed
spanner-mycli-dev pr-workflow review $(gh pr view --json number -q .number) --wait-checks

# 6. Clean up when merged
phantom delete issue-123-feature
```

### AI Assistant Integration

```bash
# AI can run complete workflows autonomously
spanner-mycli-dev pr-workflow create --wait-checks

# AI can handle smart detection
spanner-mycli-dev review gemini 306 --wait-checks --force-request

# AI can integrate with documentation
spanner-mycli-dev docs update-help
```

## Migration from Shell Scripts

| Old Script | New Command | Improvements |
|------------|-------------|-------------|
| `scripts/dev/setup-phantom-worktree.sh` | `worktree setup` | Structured flags, validation |
| `scripts/docs/update-help-output.sh` | `docs update-help` | Integrated workflow |
| Custom PR scripts | `pr-workflow create/review` | Complete lifecycle |

## Related Tools

- **gh-helper**: Generic GitHub operations (delegated to)
- **phantom**: Worktree management (external dependency)
- **gh CLI**: GitHub authentication and basic operations
- **Claude Code**: AI assistant integration (`.claude` configuration)