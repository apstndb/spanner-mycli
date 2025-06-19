# Development Tools

AI-optimized development tools (part of issue #301 script reorganization)

## Essence

**Problem**: Shell scripts are AI-unfriendly (temp files, unstructured output, poor error handling)  
**Solution**: Go tools with AI-friendly interfaces

### Design Principles
- **Structured output**: YAML/JSON (`gojq --yaml-input` compatible)
- **Batch operations**: Multiple actions in one command (e.g., `threads resolve ID1 ID2 ID3`)
- **GraphQL integration**: Unified data fetching instead of multiple `gh` CLI calls
- **Clear separation**: Generic (gh-helper) vs project-specific (spanner-mycli-dev)

## Tools

### gh-helper (Generic GitHub operations)
```bash
# Review monitoring & thread management
gh-helper reviews wait [PR] --request-review
gh-helper threads resolve <ID1> [<ID2>...]
```

### spanner-mycli-dev (Project-specific)
```bash
# Worktree & Gemini review automation
spanner-mycli-dev worktree setup issue-123
spanner-mycli-dev review gemini <PR>
```

## Usage

```bash
# Build
make build-tools

# Basic usage
./bin/gh-helper reviews wait [PR] --request-review
./bin/gh-helper threads resolve <thread-id> [<thread-id>...]
./bin/gh-helper threads show <thread-id> [<thread-id>...]
./bin/spanner-mycli-dev worktree setup issue-123

# Structured output processing
./bin/gh-helper reviews analyze 306 | gojq --yaml-input '.summary.critical'
```

See each tool's `--help` for details.