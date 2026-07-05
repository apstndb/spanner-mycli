# Architecture and Code Organization

This document is a map, not a mirror. It tells you where each subsystem lives
and which doc comments are authoritative. When this file and the code disagree,
the code wins - fix this file.

## Package Layout

All application code lives in `internal/mycli` (package `mycli`); the root
`main.go` only calls `mycli.Main(version, installFrom)`.

Key files in `internal/mycli`:

- **app.go**: `Main()`, startup sequence, mode selection (interactive, batch, MCP).
- **config.go**: kong flag definitions (`spannerOptions`), `ValidateSpannerOptions`,
  `initializeSystemVariables` / `createSystemVariablesFromOptions`.
- **cli.go / cli_output.go / cli_readline.go / cli_mcp.go**: interactive loop,
  result rendering, readline integration, MCP server.
- **session.go**: `Session` (Spanner clients, statement execution boundary) and
  `SessionHandler` (USE/DETACH database switching).
- **transaction_manager.go**: `TransactionManager` (transaction state, mutex,
  SET LOCAL undo log).
- **statements.go / statements_*.go**: statement implementations grouped by
  area (schema, mutations, transactions, proto, LLM, dump, ...).
- **client_side_statement_def.go**: **CRITICAL** - all client-side statement
  patterns (see below).
- **system_variables.go, var_registry.go, var_handler.go,
  system_variables_registry.go**: system variables (see
  [patterns/system-variables.md](patterns/system-variables.md)).
- **streamio/**: `StreamManager`, the stdin/stdout/TTY/tee stream separation.

Supporting subpackages under `internal/mycli/` (format, metrics,
decoder, iterutil, filesafety, ...) are small and self-describing; read their
doc comments.

## Authoritative Doc Comments

These subsystems are documented where their invariants are enforced. Read the
code comments before changing anything nearby; do not re-document them here.

- **System variable state model**: `internal/mycli/system_variables.go` -
  the `StartupConfig` / `ConnectionVars` / `LastResult` / SET-able `*Vars`
  decomposition, and the single-instance contract (the struct is never copied;
  the registry holds raw pointers into it).
- **Database switching**: `SessionHandler.switchSession` in
  `internal/mycli/session.go` - USE/DETACH mutate the single live
  `systemVariables` in place.
- **Transactions and locking**: `TransactionManager` in
  `internal/mycli/transaction_manager.go` - mutex rationale, the mandatory
  closure-based access helpers, the `WithLock` / `Locked` method-suffix
  convention, and the SET LOCAL undo log.
- **Statement timeouts**: `Session.getTimeoutForStatement` in
  `internal/mycli/session.go` - timeouts are applied exactly once, at the
  `ExecuteStatement` boundary.
- **Output streams**: `StreamManager` in
  `internal/mycli/streamio/stream_manager.go` - data output (tee-able) vs
  terminal control operations (never tee'd). Use the manager's accessors
  instead of writing to `os.Stdout` directly.
- **Flag precedence**: `parseFlagsArgs` in `internal/mycli/config.go` -
  precedence is CLI > environment > config file > defaults, implemented with
  kong resolvers (the `SPANNER_*` env resolver is registered after the TOML
  resolver on purpose).

## Client-Side Statement System

`client_side_statement_def.go` defines every statement that spanner-mycli
handles itself instead of sending to Spanner. Each entry in
`clientSideStatementDefs` is a `clientSideStatementDef`:

- `Pattern`: a package-level precompiled regexp, matched case-insensitively
  against the whole statement without the trailing semicolon. Use `(?is)` and
  named capture groups.
- `HandleGroups`: converts the named capture groups (`map[string]string`) to a
  `Statement`.
- `Descriptions`: human-readable usage/syntax used for help output and README
  generation.
- `Completion` (optional): fuzzy argument completion for interactive mode.

### Adding a New Client-Side Statement

1. Add a `*clientSideStatementDef` to `clientSideStatementDefs`, following the
   field contract above.
2. Implement the `Statement` (its `Execute` method) in the matching
   `statements_*.go` file.
3. Add tests: statement parsing coverage plus, for behavior, the integration
   table in `integration_test.go` (see [patterns/testing.md](patterns/testing.md)).
4. Run `make docs-update` to refresh the generated statement help in README.md.

## Configuration

- Sources, highest precedence first: command-line flags, `SPANNER_*`
  environment variables, `.spanner_mycli.toml` (home directory, then current
  directory), built-in defaults.
- Startup validation happens in three stages, and different error classes
  surface at different stages: kong parsing (`parseFlags`), business rules
  (`ValidateSpannerOptions`), and value/type validation
  (`initializeSystemVariables`). Tests must mirror the stage they target.

## Testing Infrastructure

See [patterns/testing.md](patterns/testing.md) for test tiers and conventions.
The only test selector in CI and the Makefile is the standard `-short` flag;
do not introduce build tags to gate tests (tag-gated files are not compiled by
default and rot silently).

## Related Documentation

- [patterns/system-variables.md](patterns/system-variables.md) - adding and modifying system variables
- [patterns/testing.md](patterns/testing.md) - testing best practices
- [development-insights.md](development-insights.md) - development workflow notes
- [issue-management.md](issue-management.md) - GitHub workflow and processes
