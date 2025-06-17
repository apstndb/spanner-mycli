# Development Tools

This directory contains AI-friendly development tools created as part of issue #301 script reorganization.

## Tools Overview

### gh-helper
Generic GitHub operations optimized for AI assistants:
- **reviews**: PR review state tracking and waiting
- **threads**: Review thread management and replies
- **Purpose**: Reusable across any GitHub repository

### spanner-mycli-dev  
Project-specific development workflows:
- **worktree**: Phantom worktree management
- **docs**: Documentation generation
- **pr-workflow**: Complete PR creation and review cycles
- **review gemini**: Gemini Code Review automation
- **Purpose**: Specific to spanner-mycli project needs

## Architecture

### Module Structure
The dev-tools directory uses a **unified Go module** approach:

```
dev-tools/
├── go.mod                    # Single module for all tools
├── go.sum                    # Shared dependencies
├── shared/                   # Common utilities package
│   ├── utils.go             # File operations, command execution
│   └── github.go            # GitHub API helpers
├── gh-helper/
│   └── main.go              # Generic GitHub tool
└── spanner-mycli-dev/
    └── main.go              # Project-specific tool
```

### Design Rationale

**Why unified module instead of separate modules?**
- Simpler dependency management (single go.mod)
- Natural sharing of common utilities via internal package
- Reduced complexity compared to multiple modules with replace directives
- Easier maintenance and testing

**Code Organization:**
- **shared/**: Common utilities to eliminate duplication
- **Tool separation**: Clear responsibility boundaries
- **Package imports**: `github.com/apstndb/spanner-mycli/dev-tools/shared`

### Evolution History

1. **Initial state**: Separate shell scripts scattered across repository
2. **First refactor**: Individual Go modules per tool (gh-helper, spanner-mycli-dev)
3. **Second refactor**: Attempted shared module with replace directives
4. **Final architecture**: Unified module with shared package (current)

The final approach eliminated ~30 lines of duplicate code while maintaining clean separation of concerns.

## Building

```bash
# Build both tools from project root
make build-tools

# Individual builds (from dev-tools/)
go build -o ../bin/gh-helper ./gh-helper
go build -o ../bin/spanner-mycli-dev ./spanner-mycli-dev
```

## Usage Patterns

### For AI Assistants
Both tools are designed with AI-friendly interfaces:
- Clear subcommand structure
- Comprehensive help text
- Predictable JSON output where applicable
- Timeout handling for long-running operations

### Common Workflows
```bash
# Complete PR workflow
bin/spanner-mycli-dev pr-workflow create --wait-checks

# Review thread management  
bin/gh-helper threads list <PR>
bin/gh-helper threads reply <THREAD_ID> --message "Fixed in commit abc123"

# Worktree development
bin/spanner-mycli-dev worktree setup issue-123-feature
```

## Future Considerations

- **Shared utilities**: Can be extended for additional common operations
- **Tool expansion**: New tools can be added under the unified module
- **API evolution**: GitHub API helpers in shared/ can be enhanced
- **Testing**: Unit tests can be added for shared utilities

The unified module approach provides a solid foundation for continued development tool evolution while maintaining simplicity and avoiding over-engineering.