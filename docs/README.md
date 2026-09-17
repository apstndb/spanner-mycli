# User documentation

The [README](../README.md) contains installation, basic usage, the generated
[flags](../README.md#usage) and [statement reference](../README.md#client-side-statement-syntax),
and feature examples. Main-branch docs describe main; use the matching release
tag for an older binary.

Detailed guides:

- [System variables](system_variables.md): values, configuration precedence, SET LOCAL and RESET
- [Query plans](query_plan.md): execution profiles, last-query inspection and rendering
- [SAVEPOINT and ABORTED retry](savepoint.md): opt-in recovery and replay limits
- [Meta commands](meta_commands.md): shell, SQL files, prompts and output control
- [Cyclic DUMP restoration](cyclic_dump.md): export modes and restore constraints
- [Slim binary](slim_binary.md): optional-feature exclusions and builds
- [Driver compatibility](spanner-driver-compatibility.md): supported properties and differences

For implementation and validation conventions, see [developer documentation](../dev-docs/README.md).
