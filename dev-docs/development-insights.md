# Development Workflow Notes

Durable workflow knowledge that is not tied to one type or function (facts
about a specific type or function belong in its doc comment). Keep this file
short: narrative history and one-shot findings go in issues/PRs, not here.

## Manual Verification with the Embedded Emulator

Any feature can be exercised end-to-end without a Cloud project or Docker
setup ceremony:

```bash
make build
./spanner-mycli --embedded-emulator -t                       # interactive
./spanner-mycli --embedded-emulator -t --file=script.sql     # scripted
./spanner-mycli --embedded-emulator -t --execute="SHOW VARIABLES;"
```

- No `--project` / `--instance` / `--database` flags are needed; the embedded
  emulator supplies dummy values.
- `-t` (table mode) gives stable, readable output suitable for pasting into
  PR descriptions.
- The emulator completes DDL operations instantly (`DONE: true` immediately),
  unlike production. Verify output format and metadata structure, not timing
  behavior.
- Test features both ways when applicable: via the CLI flag and via
  `SET <VARIABLE> = ...`.

## Panic vs Error Return

- Programming errors (impossible states, violated invariants): panic is
  acceptable.
- Runtime conditions (invalid user input, configuration problems, RPC
  failures): return an error.
- Code inherited from spanner-cli: preserve original behavior unless there is
  a concrete reason to change it.

## Review Workflow

The PR review loop (gh-helper commands, Gemini review rules, thread
resolution etiquette) is defined in [AGENTS.md](../AGENTS.md) and
[issue-management.md](issue-management.md). The one rule worth repeating:
push the fix first, then reply to each review thread with the commit hash and
resolve it - a reply without a pushed commit is not verifiable.

## Related Documentation

- [Architecture Guide](architecture-guide.md) - code map and authoritative doc comments
- [patterns/testing.md](patterns/testing.md) - automated testing conventions
- [Issue Management](issue-management.md) - GitHub workflow and processes
