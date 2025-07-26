# Parser Framework Design Documentation

## Overview

This parser framework provides a type-safe, generic approach to parsing and validating system variables in spanner-mycli. It addresses the complexity of handling values from different sources (REPL SET statements vs CLI flags/config files) with appropriate parsing rules for each context.

## Key Design Decisions

### 1. No Default Whitespace Trimming

After careful analysis of go-flags behavior:

- **INI files**: go-flags already handles whitespace appropriately
  - Unquoted values: whitespace is trimmed
  - Double-quoted values: whitespace is preserved
- **CLI flags**: Values after `=` are preserved as-is
- **Conclusion**: Additional `strings.TrimSpace()` is unnecessary and potentially harmful

### 2. Dual-Mode Parsing

System variables can receive values from two distinct sources:

#### GoogleSQL Mode (REPL SET statements)
- Must follow GoogleSQL lexical structure
- String literals: `'single quotes'`, `"double quotes"`, `r'raw'`, `b'bytes'`
- Proper escape sequence handling (`\'`, `\"`, `\n`, `\u0041`, etc.)
- Uses memefish parser for accuracy

#### Simple Mode (CLI flags and config files)
- Direct value usage without SQL parsing rules
- No quote removal (values are used as provided by go-flags)
- More intuitive for command-line usage

### 3. Type Safety Through Generics

```go
// Generic parser interface
type Parser[T any] interface {
    Parse(value string) (T, error)
    Validate(value T) error
    ParseAndValidate(value string) (T, error)
}
```

Benefits:
- Compile-time type checking
- Reusable validation logic
- Consistent error handling

### 4. Composable Validation

```go
// Chain multiple validators
parser := parser.WithValidation(
    parser.NewIntParser(),
    validators.Range(1, 100),
    validators.Even(),
)
```

## Usage Examples

### Simple String Variable
```go
// Simple mode: no processing
--set OPTIMIZER_VERSION=LATEST

// GoogleSQL mode: requires quotes
SET OPTIMIZER_VERSION = 'LATEST';
```

### Integer with Range
```go
// Both modes handle the same
--set TAB_WIDTH=4
SET TAB_WIDTH = 4;
```

### Duration Values
```go
// Simple mode
--set STATEMENT_TIMEOUT=10s

// GoogleSQL mode
SET STATEMENT_TIMEOUT = '10s';
```

## Implementation Structure

```
internal/parser/
├── parser.go          # Core interfaces and base types
├── types.go           # Basic type parsers (bool, int, string, duration)
├── enum.go            # Enum parser with case-insensitive matching
├── memefish.go        # GoogleSQL compatibility layer
├── dualmode.go        # Dual-mode parsing support
└── sysvar/
    ├── sysvar.go      # System variable integration
    ├── predefined.go  # Common parser instances
    └── dualmode.go    # Dual-mode registry for system variables
```

## Migration Guide

### Before (Old Style)
```go
// Manual parsing with inconsistent error handling
func oldSetter(sysVars *systemVariables, name, value string) error {
    timeout, err := time.ParseDuration(unquoteString(value))
    if err != nil {
        return fmt.Errorf("invalid timeout format: %v", err)
    }
    if timeout < 0 {
        return fmt.Errorf("timeout cannot be negative")
    }
    sysVars.StatementTimeout = &timeout
    return nil
}
```

### After (New Style)
```go
// Declarative with automatic validation
registry.Register(CreateDualModeVariableParser(
    "STATEMENT_TIMEOUT",
    "Timeout value for statements",
    parser.NewDualModeParser(
        googleSQLParser,  // For SET statements
        parser.NewDurationParser().WithRange(0, time.Hour),  // For CLI/config
    ),
    setter,
    getter,
))
```

## Best Practices

1. **Use simple parsers by default** - Don't add unnecessary processing
2. **Let go-flags handle INI parsing** - It already does the right thing
3. **Use dual-mode for all system variables** - Consistent behavior across contexts
4. **Add validation at parser level** - Not in setters
5. **Provide clear error messages** - Include valid ranges/values

## Future Enhancements

1. **Custom parser for complex types** (e.g., endpoint parsing)
2. **Automatic help generation** from parser metadata
3. **Parser composition** for nested structures
4. **Async validation** for expensive checks