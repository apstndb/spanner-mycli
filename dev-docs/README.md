# Developer Documentation

Detailed technical documentation for spanner-mycli developers and coding
agents. AGENTS.md holds only the rules that cause damage if skipped; this
directory holds the details; anything derivable from the code or git history
belongs in neither.

## Contents

```
dev-docs/
├── README.md                    # This file - index
├── architecture-guide.md        # Code map and pointers to authoritative doc comments
├── development-insights.md      # Development workflow notes (manual verification, panic policy)
├── issue-management.md          # gh-helper reference, labels, PR process, worktrees
└── patterns/
    ├── system-variables.md      # Adding and modifying system variables
    └── testing.md               # Test tiers, style, isolation, coverage
```

- [Architecture Guide](architecture-guide.md) - where each subsystem lives and
  which code doc comments are authoritative (system variables state model,
  transactions, streams, timeouts, flag precedence); how to add a client-side
  statement.
- [Development Insights](development-insights.md) - manual verification with
  the embedded emulator, panic vs error policy, review-loop pointers.
- [Issue Management](issue-management.md) - full gh-helper command reference,
  review workflow, label system, release-notes labels, git and phantom
  worktree practices.
- [System Variable Patterns](patterns/system-variables.md) - the
  StartupConfig/ConnectionVars/LastResult decomposition, registration API,
  SET LOCAL contract, testing.
- [Testing Best Practices](patterns/testing.md) - test tiers (`-short`),
  style (std testing + go-cmp), isolation rules, flag/startup testing,
  coverage analysis.

## Maintenance Rules

- Update the relevant file in the same PR as an intentional behavior or
  process change; delete text that no longer matches the code rather than
  hedging it.
- A fact about one specific type or function belongs in its doc comment, not
  here. These docs should carry only cross-cutting knowledge an agent cannot
  learn from the code.
- `make docs-update` regenerates the help sections in the top-level README.md
  (output also written to `./tmp/` for inspection).

## Related Documentation

- [AGENTS.md](../AGENTS.md) - shared repository guidance for coding agents
  (CLAUDE.md is a stub containing `@AGENTS.md`)
- [User Documentation](../docs/) - end-user facing documentation
- [README.md](../README.md) - project overview and quick start
