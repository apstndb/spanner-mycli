# Design: Format Package Registry & ValueFormatMode Refactoring

## Plan

### Goal
Decouple the `format` package from `enums.DisplayMode` and specific format implementations (e.g., `formatsql`), enabling:
1. Format implementations to be registered as plugins without circular dependencies
2. Value formatting requirements to be declared by formatters themselves (self-describing)
3. `execute_sql.go` to be format-agnostic — no mode-name switching for value format decisions

### Phases
1. **Extract `format.Mode` as string type** — Replace `enums.DisplayMode` dependency in `format` package with a string-based `Mode` type
2. **Registry pattern** — Allow external packages to register `FormatFunc` and `StreamingFormatter` factories
3. **`formatsql` as plugin** — Move SQL export formatter to its own package, register via `init()`
4. **`ValueFormatMode` declaration** — Formatters declare their value formatting requirements (display vs SQL literal), removing mode-name checks from callers

## Design

### Architecture Overview

```
┌────────────────────┐     registers      ┌─────────────────┐
│   formatsql/       │ ─────────────────►  │  format/         │
│   (SQL export)     │   init():           │  registry.go     │
│                    │   - FormatFunc      │  mode.go         │
│   Declares:        │   - StreamingFmt    │  config.go       │
│   SQLLiteralValues │   - ValueFmtMode   │                  │
└────────────────────┘                     └────────┬────────┘
                                                    │
                                           queries  │ ValueFormatModeFor()
                                                    ▼
                                          ┌─────────────────┐
                                          │  execute_sql.go  │
                                          │  cli_output.go   │
                                          │                  │
                                          │  switch vfm {    │
                                          │  case SQLLiteral │
                                          │  default:Display │
                                          │  }               │
                                          └─────────────────┘
```

### Key Types

```go
// format/mode.go

type Mode string              // "TABLE", "CSV", "SQL_INSERT", etc.
type ValueFormatMode int      // DisplayValues | SQLLiteralValues

func ValueFormatModeFor(mode Mode) ValueFormatMode  // Query registry
```

```go
// format/registry.go

func RegisterFormatFunc(factory FormatFuncFactory, modes ...Mode)
func RegisterStreamingFormatter(factory StreamingFormatterFactory, modes ...Mode)
func RegisterValueFormatMode(vfm ValueFormatMode, modes ...Mode)
```

### Value Format Pipeline (Before → After)

**Before:**
```go
// execute_sql.go — knows specific mode names
switch sysVars.Display.CLIFormat {
case enums.DisplayModeSQLInsert, enums.DisplayModeSQLInsertOrIgnore, enums.DisplayModeSQLInsertOrUpdate:
    fc = spanvalue.LiteralFormatConfig
    usingSQLLiterals = true
default:
    fc = decoder.FormatConfigWithProto(...)
    usingSQLLiterals = false
}
```

**After:**
```go
// execute_sql.go — queries registry, no mode-name knowledge needed
fmtMode := format.Mode(sysVars.Display.CLIFormat.String())
vfm := format.ValueFormatModeFor(fmtMode)

switch vfm {
case format.SQLLiteralValues:
    fc = spanvalue.LiteralFormatConfig
default:
    fc = decoder.FormatConfigWithProto(...)
}
```

### Registration (formatsql)

```go
func init() {
    format.RegisterFormatFunc(newFormatSQL, ModeSQLInsert, ModeSQLInsertOrIgnore, ModeSQLInsertOrUpdate)
    format.RegisterStreamingFormatter(newStreamingFormatterSQL, ModeSQLInsert, ModeSQLInsertOrIgnore, ModeSQLInsertOrUpdate)
    format.RegisterValueFormatMode(format.SQLLiteralValues, ModeSQLInsert, ModeSQLInsertOrIgnore, ModeSQLInsertOrUpdate)
}
```

### Activation via blank import

```go
// app.go
import _ "github.com/apstndb/spanner-mycli/internal/mycli/formatsql"
```

## Evaluation

### What was accomplished

| Item | Status | Notes |
|------|--------|-------|
| `format.Mode` string type | Done | Replaces `enums.DisplayMode` in format package |
| Mode constants | Done | `ModeTable`, `ModeCSV`, etc. |
| `IsTableMode()` method | Done | Replaces multi-condition checks |
| Registry for FormatFunc | Done | `RegisterFormatFunc` / `lookupFormatFunc` |
| Registry for StreamingFormatter | Done | `RegisterStreamingFormatter` / `lookupStreamingFormatter` |
| Registry for ValueFormatMode | Done | `RegisterValueFormatMode` / `lookupValueFormatMode` |
| `ValueFormatModeFor()` query API | Done | Returns `DisplayValues` for unknown modes |
| `formatsql` package extraction | Done | Renamed from `format/sql.go` to `formatsql/formatsql.go` |
| `formatsql` self-registration via `init()` | Done | Registers all three: FormatFunc, StreamingFormatter, ValueFormatMode |
| `execute_sql.go` uses `ValueFormatModeFor` | Done | No more `enums.DisplayModeSQLInsert` switch |
| `cli_output.go` uses `ValueFormatModeFor` | Done | Replaced `IsSQLExport()` calls |
| `HasSQLFormattedValues` derived from `ValueFmtMode` | Done | `qe.ValueFmtMode == format.SQLLiteralValues` |
| `IsSQLExportMode` in formatsql removed | Done | Superseded by `ValueFormatModeFor` |
| Tests for registry | Done | `TestNewFormatter_RegisteredMode`, `TestNewStreamingFormatter_RegisteredMode` |
| Tests for `ValueFormatModeFor` | Done | `TestValueFormatModeFor` |

### Dependency direction

```
Before:  format ──depends on──► enums (DisplayMode enum)
         format ──contains──► sql.go (memefish dependency)

After:   format ──no dependency──► enums
         formatsql ──depends on──► format (Mode, FormatFunc, etc.)
         formatsql ──depends on──► memefish
         execute_sql ──depends on──► format (ValueFormatModeFor)
         execute_sql ──depends on──► formatsql (ExtractTableNameFromQuery only)
```

The `format` package is now a leaf dependency with no knowledge of specific format implementations.

### Diff statistics

- 14 files changed, ~280 insertions, ~120 deletions (net +160 lines)
- 2 new files: `format/mode.go`, `format/registry.go`
- 1 renamed: `format/sql.go` → `formatsql/formatsql.go`

### Remaining coupling

| Coupling | Location | Reason | Mitigation path |
|----------|----------|--------|-----------------|
| `formatsql.ExtractTableNameFromQuery` | `execute_sql.go:71` | Table name auto-detection for SQL export | Could be moved behind a registry callback if more formats need similar pre-processing |
| `enums.DisplayMode` ↔ `format.Mode` conversion | `cli_output.go`, `formatter_utils.go`, `streaming.go` | Bridge between enum world and string world | Acceptable — conversion is mechanical (`format.Mode(dm.String())`) |
| `enums.IsSQLExport()` | `enums/enums.go` | Still defined but no longer called from core | Can be removed when no external callers remain |

### Benefits

1. **New format modes**: Adding a new format (e.g., JSON, Protobuf text) requires only:
   - A new package with `init()` registering its factories and ValueFormatMode
   - A blank import in `app.go`
   - No changes to `execute_sql.go`, `cli_output.go`, `format/format.go`

2. **Value format extensibility**: If a future format needs a third ValueFormatMode (e.g., `JSONLiteralValues`), it's an enum addition + registration — no switch cascades to update.

3. **Testability**: Modes can be registered/tested independently without the full application context.

### Risks

1. **Runtime registration** — Format availability depends on `init()` ordering and blank imports. A missing import silently falls through to "unsupported mode" error. This is a standard Go pattern (e.g., `database/sql` drivers) and the error message is clear.

2. **Global mutable state** — The registry uses package-level maps with mutex protection. Concurrent test registration could interfere. Mitigated by using unique mode names in tests.

3. **`make check` not verified** — Due to Go 1.25 toolchain being unavailable in the current environment (network-restricted), compilation and tests could not be run. CI will validate.
