# Developer Documentation

This directory contains detailed technical documentation for spanner-mycli developers and contributors.

## Documentation Structure

```
dev-docs/
├── README.md                    # This file - overview and navigation
├── architecture-guide.md        # System architecture and code organization
├── development-insights.md      # Development patterns and best practices
├── issue-management.md          # GitHub workflow and PR processes
└── patterns/                    # Specific implementation patterns
    └── system-variables.md      # System variable implementation patterns
```

## Documentation Overview

### [Architecture Guide](architecture-guide.md)
- Core components and their responsibilities
- Client-side statement system details
- Configuration management
- Testing infrastructure
- Dependencies and build system

### [Development Insights](development-insights.md)
- Development workflow patterns
- Error handling best practices
- Resource management strategies
- Code quality improvement techniques
- Context and session management

### [Issue Management](issue-management.md)
- GitHub issue workflow
- Pull request process
- Code review strategies
- Git best practices
- Knowledge management through PR comments

### [Patterns](patterns/)
- [System Variables](patterns/system-variables.md) - Implementation patterns for system variables, timeout management, and testing strategies

## Updating Documentation

### When to Update

1. **New Features**: Document architecture changes and implementation patterns
2. **Bug Fixes**: Update if the fix reveals important patterns or insights
3. **Process Changes**: Update workflow documentation when development processes evolve
4. **Pattern Discovery**: Add new patterns when establishing best practices

### Documentation Standards

- Use clear, technical language appropriate for developers
- Include code examples with proper syntax highlighting
- Cross-reference related documentation
- Keep examples up-to-date with current codebase

### Updating Help Output in README.md

#### Automated Process (Recommended)

```bash
# Generate help output files
scripts/docs/update-help-output.sh

# Files are generated in ./tmp/
# - help_clean.txt: Content for README.md --help section (lines 97-135)
# - statement_help.txt: Content for README.md statement help table (lines 355-406)
```

**Manual Steps Required:**
1. Replace content between code block markers in README.md (lines 97-135) with `./tmp/help_clean.txt`
2. Replace statement help table content (lines 355-406) with `./tmp/statement_help.txt`

#### Manual Process (For Reference)

```bash
mkdir -p ./tmp
script -q ./tmp/help_output.txt sh -c "stty cols 200; go run . --help"
go run . --statement-help > ./tmp/statement_help.txt
sed '1s/^.\{2\}//' ./tmp/help_output.txt > ./tmp/help_clean.txt
# Then manually update README.md with the generated content
```

#### Update Locations in README.md

- **--help output**: Lines 97-135 (within code block at lines 96-136)
- **--statement-help table**: Lines 355-406 (markdown table format)
- **Generation comments**: Document the generation commands in HTML comments for maintainability

## Contributing to Documentation

When adding new documentation:

1. Place it in the appropriate existing file or create a new file if needed
2. Update this README.md with navigation if adding new files
3. Ensure cross-references are accurate
4. Follow existing formatting and structure patterns
5. Remember: Implementation details belong here, not in CLAUDE.md

## Related Documentation

- [CLAUDE.md](../CLAUDE.md) - Essential guidelines only (keep minimal)
- [User Documentation](../docs/) - End-user facing documentation
- [README.md](../README.md) - Project overview and quick start