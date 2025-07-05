# System Variable Implementation Patterns

This document provides detailed patterns and best practices for implementing system variables in spanner-mycli.

## Design Patterns (Issue #243 Insights)

**Discovery**: Consistent patterns for system variable implementation improve maintainability and user experience

### Basic Implementation Pattern

1. **Naming Convention**: CLI-specific variables **MUST** use `CLI_` prefix to distinguish from java-spanner JDBC properties

2. **Access Control Design**:
   - **Presentation layer variables** (display, formatting): Use `boolAccessor()` or `stringAccessor()` for full read/write access
   - **Session behavior variables** (authentication, connection settings): Use explicit `accessor{Getter: ...}` for read-only access to prevent runtime session state changes

3. **Implementation Template**:
   ```go
   // Read/write variable
   "CLI_VARIABLE_NAME": {
       Description: "Clear description of purpose and default value",
       Accessor: boolAccessor(func(variables *systemVariables) *bool {
           return &variables.FieldName
       }),
   },
   
   // Read-only variable (session behavior)
   "CLI_SESSION_VARIABLE": {
       Description: "Clear description with read-only nature explained",
       Accessor: accessor{
           Getter: stringGetter(func(variables *systemVariables) *string {
               return &variables.FieldName
           }),
       },
   },
   ```

4. **Testing Requirements**: Add comprehensive test cases to `system_variables_test.go` covering both successful operations and proper error handling for unimplemented setters

5. **Documentation**: Include clear descriptions explaining purpose, default values, and any access restrictions

### Architecture Guidelines

- **Pattern**: Use `boolAccessor()` and `stringAccessor()` for read/write variables, explicit `accessor{Getter: ...}` for read-only variables
- **Architecture**: Session behavior variables should be read-only to prevent runtime session state changes that could cause inconsistencies
- **Error Handling**: Proper use of `errSetterUnimplemented` and `errGetterUnimplemented` for consistent error reporting
- **Testing Strategy**: System variables test pattern in `system_variables_test.go` covers both SET/GET operations and proper error handling for unimplemented setters
- **Code Organization**: System variable definitions follow clear patterns that make adding new variables straightforward

### Session-Init-Only Variables

Some variables can only be set before session creation because they control client initialization behavior that cannot be changed after the session is established. These are defined in the `sessionInitOnlyVariables` map in `system_variables.go`.

**Implementation Pattern**:
1. Add the variable name to `sessionInitOnlyVariables` slice
2. Use standard accessor (e.g., `boolAccessor`) - the validation is automatic
3. Document in the Description that it must be set before session creation

**Example**:
```go
// In sessionInitOnlyVariables slice
var sessionInitOnlyVariables = []string{
    "CLI_ENABLE_ADC_PLUS",
    // Add more variables here as needed
}

// In systemVariableDefMap
"CLI_ENABLE_ADC_PLUS": {
    Description: "A boolean indicating whether to enable enhanced Application Default Credentials. Must be set before session creation. The default is true.",
    Accessor: boolAccessor(func(variables *systemVariables) *bool {
        return &variables.EnableADCPlus
    }),
},
```

**Behavior**:
- Can be set via `--set` flag before session creation
- After session creation, attempts to change the value will return an error with the current value
- Idempotent sets (setting to the same value) are allowed
- The validation in `systemVariables.Set()` handles case-insensitive variable names correctly

### Development Workflow

- **Workflow**: phantom + tmux horizontal split works well for focused system variable implementation
- **Manual Testing**: Use `--embedded-emulator` only when Go test suite is insufficient - prefer comprehensive test cases in existing test suite

## Timeout Implementation Patterns (Issue #276 Insights)

**Discovery**: Centralized timeout management and pointer type design for proper state handling

### Pointer Type Design

**Key Pattern**: Using `*time.Duration` for system variables enables distinction between unset (nil) vs zero timeout, critical for backward compatibility

```go
type systemVariables struct {
    // Other fields...
    StatementTimeout *time.Duration
}
```

Benefits:
- `nil` = unset (use default behavior)
- `&time.Duration{0}` = explicitly set to zero (immediate timeout)
- `&time.Duration{5*time.Minute}` = explicitly set timeout

### Context Management Pattern

**Centralized timeout application** in `Session.ExecuteStatement` prevents `defer cancel()` timing issues during resource cleanup:

```go
func (s *Session) ExecuteStatement(ctx context.Context, stmt Statement) error {
    // Apply timeout at the session boundary, not in individual operations
    if s.systemVariables.StatementTimeout != nil {
        var cancel context.CancelFunc
        ctx, cancel = context.WithTimeout(ctx, *s.systemVariables.StatementTimeout)
        defer cancel()
    }
    
    // Execute statement with timeout context
    return s.executeStatementWithContext(ctx, stmt)
}
```

### Early Validation Pattern

**Pre-validate CLI flag values** before system variable assignment for better error reporting:

```go
if opts.Timeout != "" {
    // Validate format first
    _, err := time.ParseDuration(opts.Timeout)
    if err != nil {
        return systemVariables{}, fmt.Errorf("invalid value of --timeout: %v: %w", opts.Timeout, err)
    }
    
    // Then set the system variable
    if err := sysVars.Set("STATEMENT_TIMEOUT", opts.Timeout); err != nil {
        return systemVariables{}, fmt.Errorf("invalid value of --timeout: %v: %w", opts.Timeout, err)
    }
}
```

### Differentiated Defaults

**Operation-specific defaults** when timeout is unspecified:

```go
func (s *Session) getEffectiveTimeout() time.Duration {
    if s.systemVariables.StatementTimeout != nil {
        return *s.systemVariables.StatementTimeout
    }
    
    // Apply different defaults based on operation type
    switch s.currentOperation {
    case "partitioned_dml":
        return 24 * time.Hour  // Existing PDML default
    default:
        return 10 * time.Minute  // New general default
    }
}
```

### Context Lifecycle Management

**Context cancellation must align with resource lifecycle** - centralize timeout application at statement execution boundary, not at individual operation level:

- ✅ Apply timeout at `Session.ExecuteStatement` level
- ❌ Apply timeout at individual `client.Single().Query()` calls
- ✅ Ensure `defer cancel()` happens after all resource cleanup
- ❌ Cancel context before resource cleanup completes

## Testing Patterns

### System Variable Test Structure

```go
func TestSystemVariable_TIMEOUT(t *testing.T) {
    tests := []struct {
        name    string
        value   string
        wantErr bool
        want    *time.Duration
    }{
        {"valid duration", "5m", false, durationPtr(5 * time.Minute)},
        {"zero duration", "0", false, durationPtr(0)},
        {"invalid format", "invalid", true, nil},
        {"negative duration", "-5m", true, nil},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            vars := newSystemVariables()
            err := vars.Set("STATEMENT_TIMEOUT", tt.value)
            
            if tt.wantErr {
                assert.Error(t, err)
                return
            }
            
            assert.NoError(t, err)
            assert.Equal(t, tt.want, vars.StatementTimeout)
        })
    }
}
```

### Integration Test Considerations

**Long-running integration tests** need explicit timeout configuration (1h) to prevent false failures during CI:

```go
func TestIntegration_LongRunningQuery(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping long-running integration test")
    }
    
    // Set explicit timeout for integration tests
    session.systemVariables.StatementTimeout = &time.Hour
    
    // Run test...
}
```

## Async DDL System Variable Patterns (Issue #277)

**Discovery**: System variable integration with operation control patterns

### System Variable Integration with Operation Control

**Pattern**: Use system variables to control DDL execution behavior without breaking existing workflows

```go
// In executeDdlStatements function
if session.systemVariables.AsyncDDL {
    return formatAsyncDdlResult(op)
}
// Continue with synchronous execution...
```

**Implementation Template**:
```go
// System variable definition
"CLI_ASYNC_DDL": {
    Description: "A boolean indicating whether DDL statements should be executed asynchronously. The default is false.",
    Accessor: boolAccessor(func(variables *systemVariables) *bool {
        return &variables.AsyncDDL
    }),
},

// CLI flag mapping in createSystemVariablesFromOptions
sysVars.AsyncDDL = opts.Async
```

### CLI Flag Integration Best Practices

**Location**: Process CLI flags in `createSystemVariablesFromOptions()` function

**Pattern for Boolean Flags**:
```go
// Direct mapping approach
sysVars.AsyncDDL = opts.Async
```

**Pattern for Complex Flags** (reference from timeout implementation):
```go
// Validation before system variable assignment
if opts.ComplexFlag != "" {
    if err := sysVars.Set("CLI_COMPLEX_FLAG", opts.ComplexFlag); err != nil {
        return systemVariables{}, fmt.Errorf("invalid value of --complex-flag: %v: %w", opts.ComplexFlag, err)
    }
}
```

## Related Documentation

- [Development Insights](../development-insights.md) - General development patterns
- [Architecture Guide](../architecture-guide.md) - Overall system architecture