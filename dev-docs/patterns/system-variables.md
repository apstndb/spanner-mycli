# System Variable Implementation Patterns

This document describes how to add and modify system variables in spanner-mycli,
matching the actual code as of the StartupConfig/LastResult decomposition
(PR #692) and SET LOCAL support (PR #400 / PR #691).

## State model

`systemVariables` (internal/mycli/system_variables.go) is layered by ownership
and mutability:

| Group | Type | Mutability | Examples |
|-------|------|------------|----------|
| `Config` | `StartupConfig` | Immutable after startup; every backing variable is registered read-only | `CLI_HOST`, `CLI_INSECURE`, `CLI_SKIP_SYSTEM_COMMAND` |
| `Connection` | `ConnectionVars` | Identity; `Database`/`Role` mutated only by USE/DETACH | `CLI_DATABASE`, `CLI_ROLE` |
| `LastResult` | `LastResult` | Written by statement execution; never user-settable | `READ_TIMESTAMP`, `COMMIT_RESPONSE` |
| `Display`/`Query`/`Transaction`/`Feature`/`Internal` | `*Vars` | The SET-able surface (also scoped by SET LOCAL) | `CLI_FORMAT`, `STATEMENT_TIMEOUT` |

**Single-instance contract**: there is exactly one live `systemVariables` per
process. The registry (`VarRegistry`) captures raw pointers into it, so the
struct must never be copied. USE/DETACH mutate it in place
(`SessionHandler.switchSession`).

## Adding a variable

1. **Naming**: spanner-mycli-specific variables MUST use the `CLI_` prefix.
   Names without the prefix are reserved for java-spanner JDBC-compatible
   properties.
2. **Pick the group by mutability**, not by topic:
   - Fixed at startup -> `StartupConfig` field + read-only registration +
     direct assignment in `createSystemVariablesFromOptions` (config.go).
   - User-settable -> the matching `*Vars` group + writable registration.
   - Produced by statement execution -> `LastResult` field + read-only
     registration (or a getter-only handler).
3. **Add a `varDef` entry** to the `varDefs` table in
   `internal/mycli/var_defs.go`. `registerAll`
   (`internal/mycli/var_registry.go`) iterates the table to build the
   registry; `system_variables_registry.go` holds the set/get plumbing.
4. **Document**: the reference table in docs/system_variables.md is generated
   from the registry (via the hidden `--sysvars-help` flag); run
   `make docs-update` after registering, so the description doubles as user
   documentation. The README system-variables table is hand-maintained; add a
   row there. Add a detailed section to docs/system_variables.md if the
   variable needs more than one line of explanation.
5. **Test**: see Testing below.

## Registration API (var_defs.go + var_handler.go, var_enum_handlers.go, var_custom_handlers.go)

Every variable is one `varDef` entry in the `varDefs` table (var_defs.go).
Metadata (name, `desc`, `scope`/`readOnly`) lives in the def; the `bind`
closure constructs the live handler bound to the process-wide
`systemVariables`. Handlers implement only `Get`/`Set` — read-only enforcement
is driven by the def's scope in `VarRegistry.Set`, not by the handler. `scope`
values: `scopeSession` (SET-able), `scopeStartup` (StartupConfig-backed,
read-only), `scopeConnection` (connection identity, read-only), `scopeResult`
(LastResult output, read-only).

```go
// In the varDefs table (internal/mycli/var_defs.go):

// Writable bool/string/int
{
	name: "CLI_VERBOSE", desc: "Display verbose output.", scope: scopeSession,
	bind: func(sv *systemVariables) Variable { return BoolVar(&sv.Display.Verbose) },
},

// Read-only (StartupConfig-backed): a non-session scope makes it non-settable
{
	name: "CLI_INSECURE", desc: "Skip TLS certificate verification (insecure).",
	scope: scopeStartup,
	bind:  func(sv *systemVariables) Variable { return BoolVar(&sv.Config.Insecure) },
},

// Computed read-only value: getter is func() string (it cannot fail)
{
	name: "CLI_VERSION", desc: "The version of spanner-mycli.", scope: scopeStartup,
	bind: func(sv *systemVariables) Variable { return NewReadOnlyVar(getVersion) },
},

// Enums: enumer-generated types in enums/ with a small typed constructor
{
	name: "CLI_FORMAT", desc: "...", scope: scopeSession,
	bind: func(sv *systemVariables) Variable { return DisplayModeVar(&sv.Display.CLIFormat) },
},

// Validation hook (duration bounds via WithValidator)
{
	name: "STATEMENT_TIMEOUT", desc: "...", scope: scopeSession,
	bind: func(sv *systemVariables) Variable {
		return NullableDurationVar(&sv.Query.StatementTimeout).
			WithValidator(durationValidator(durationPtr(0), nil))
	},
},

// ADD support: set bindAdd to construct the ADD handler (see CLI_PROTO_DESCRIPTOR_FILE)
```

### Session-init-only variables

Variables that control client initialization can be set via `--set` before the
session exists but must reject later writes. The actual pattern is a
`CustomVar` whose setter checks `sv.inTransaction` as a session-existence
proxy (it is nil until the first session is created); see the
`CLI_ENABLE_ADC_PLUS` entry in the `varDefs` table for the canonical example.

### Raw + parsed variables

Variables like `CLI_TYPE_STYLES` and `CLI_ANALYZE_COLUMNS` store the raw
string (shown by SHOW VARIABLE) plus a parsed artifact in a sibling field.
Their custom setter must keep both in sync, and parse errors must reject the
SET before any state changes.

## SET LOCAL compatibility

`SET LOCAL` (statements_system_variable.go) saves the current display value
and restores it through the setter when the transaction ends. This imposes a
contract on every writable variable:

- **Get -> Set must round-trip**: the string returned by the getter must be
  accepted by the setter (nullable handlers already accept `NULL`). SET LOCAL
  verifies this with a pre-flight `Set(Get())` and rejects variables that
  fail, so a non-round-tripping variable degrades gracefully - but fix the
  round-trip if the variable should support SET LOCAL.
- **Setter side effects re-run on restore**: parsing (templates, styles) is
  re-executed when the saved value is set back. Setters must be idempotent
  for the same value.
- Read-only variables and setters that reject writes mid-transaction are
  rejected by the pre-flight check automatically.

## CLI flag mapping (config.go)

- StartupConfig fields are assigned directly in
  `createSystemVariablesFromOptions` (their variables are read-only, so the
  registry path would reject them).
- Settable variables map flags through `applyOptionMappings` /
  `SetFromSimple`, which routes through the setter and its validation.
  Prefer this over direct assignment so `--flag` and `SET` cannot diverge.

## Testing

Match the existing test style (std testing + go-cmp preferred; see
[testing.md](testing.md#test-style) for notes on testify usage):

```go
func TestMyVariable(t *testing.T) {
	t.Parallel()
	sysVars := newSystemVariablesWithDefaultsForTest() // registry-ready defaults
	if err := sysVars.SetFromSimple("CLI_MY_VARIABLE", "value"); err != nil {
		t.Fatal(err)
	}
	got, err := sysVars.Registry.Get("CLI_MY_VARIABLE")
	// assert on got/err
	_ = got
	_ = err
}
```

- Read-only StartupConfig variables: add the name to
  `TestStartupConfigVariablesAreReadOnly` (startup_config_test.go). This is
  load-bearing for security-sensitive variables such as
  `CLI_SKIP_SYSTEM_COMMAND`.
- SET/GET coverage lives in system_variables_test.go; transaction-scoped
  behavior examples are in statements_set_local_test.go (unit; runs with
  `-short` because pending transactions need no RPCs) and
  TestSetLocalStatements in integration_test.go (emulator).

## Related Documentation

- [Development Insights](../development-insights.md) - General development patterns
- [Architecture Guide](../architecture-guide.md) - Overall system architecture
- [docs/system_variables.md](../../docs/system_variables.md) - User-facing reference
