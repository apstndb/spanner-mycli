# Migration Analysis: Existing Code to Struct Tag Approach

## Summary

After analyzing the existing system variable registrations, I've identified several challenges that would need to be addressed before the struct tag approach can fully replace the current implementation.

## Migration Categories

### 1. ✅ Easily Migratable (70% of variables)

Simple boolean, string, and integer variables with basic getters/setters:
```go
// Current
registerSimpleVariables(registry, []simpleVar[bool]{
    {"AUTO_PARTITION_MODE", "Description...", &sv.AutoPartitionMode},
}, sysvar.DualModeBoolParser, sysvar.FormatBool)

// With tags
AutoPartitionMode bool `sysvar:"name=AUTO_PARTITION_MODE,desc='Description...'"`
```

### 2. ⚠️ Requires Extended Tag Syntax (20% of variables)

#### Custom Setters
```go
// Current - READONLY has validation logic
func(v bool) error {
    if sv.CurrentSession != nil && (sv.CurrentSession.InReadOnlyTransaction() || sv.CurrentSession.InReadWriteTransaction()) {
        return errors.New("can't change READONLY when there is a active transaction")
    }
    sv.ReadOnly = v
    return nil
}

// Proposed tag
ReadOnly bool `sysvar:"name=READONLY,desc='...',setter=setReadOnly"`
```

#### Nullable Types with Range Validation
```go
// Current
minDelay := time.Duration(0)
maxDelay := 500 * time.Millisecond
sysvar.NewNullableDurationVariableParser("MAX_COMMIT_DELAY", ..., &minDelay, &maxDelay)

// Proposed tag
MaxCommitDelay *time.Duration `sysvar:"name=MAX_COMMIT_DELAY,desc='...',type=nullable_duration,min=0,max=500ms"`
```

#### Proto Enums with Aliases
```go
// Current
sysvar.RegisterProtobufEnumWithAliases(registry,
    "DEFAULT_ISOLATION_LEVEL", ...,
    sppb.TransactionOptions_IsolationLevel_value,
    "ISOLATION_LEVEL_",
    getter, setter,
    map[sppb.TransactionOptions_IsolationLevel][]string{
        sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED: {"UNSPECIFIED"},
    },
)

// Proposed tag
DefaultIsolationLevel sppb.TransactionOptions_IsolationLevel `sysvar:"name=DEFAULT_ISOLATION_LEVEL,desc='...',type=proto_enum,prefix=ISOLATION_LEVEL_,alias_unspecified=UNSPECIFIED"`
```

### 3. ❌ Requires Custom Parser Implementation (10% of variables)

#### Complex Multi-Operation Variables
```go
// CLI_PROTO_DESCRIPTOR_FILE has both setter (replace all) and adder (append) operations
sysvar.NewProtoDescriptorFileParser(
    "CLI_PROTO_DESCRIPTOR_FILE",
    getter,
    func(files []string) error { /* replace all */ },
    func(file string) error { /* append one */ },
    validator,
)
```

#### Variables with Side Effects
```go
// CLI_ANALYZE_COLUMNS parses template and stores in separate field
func(v string) error {
    def, err := customListToTableRenderDefs(v)
    if err != nil {
        return err
    }
    sv.AnalyzeColumns = v
    sv.ParsedAnalyzeColumns = def  // Side effect
    return nil
}
```

#### Session-Init-Only Variables
```go
// CLI_ENABLE_ADC_PLUS can only be set before session creation
sysvar.SetSessionInitOnly(&sv.EnableADCPlus, "CLI_ENABLE_ADC_PLUS", &sv.CurrentSession)
```

## Technical Challenges

### 1. Method References in Tags
Go struct tags are strings, so we can't directly reference methods. Solutions:
- Use method naming conventions (e.g., `setter=setReadOnly` implies `func (sv *systemVariables) setReadOnly(bool) error`)
- Generate method stubs that call the actual implementations

### 2. Complex Type Information
Some variables need rich type information that's awkward in tags:
- Proto type paths (e.g., `sppb.TransactionOptions_IsolationLevel`)
- Multiple validation parameters
- Enum value maps

### 3. Parser Selection Logic
The generator needs to be smart about choosing the right parser based on:
- Base type (bool, string, int64, etc.)
- Nullability (pointer types)
- Special types (time.Duration, proto enums)
- Custom type flags in tags

### 4. Backward Compatibility
During migration, we need to ensure:
- Generated code produces identical behavior
- Custom logic is preserved
- Error messages remain consistent

## Migration Strategy

### Phase 1: Simple Variables (Immediate)
1. Add tags to simple bool/string/int variables
2. Generate registration code for these
3. Compare with manual registration to ensure correctness
4. Replace manual registration with generated code

### Phase 2: Extended Syntax (Week 1-2)
1. Implement custom setter/getter support
2. Add nullable type handling
3. Support min/max validation in tags
4. Handle proto enums with prefix stripping

### Phase 3: Complex Variables (Week 3-4)
1. Keep complex variables with manual registration
2. Document which variables can't be migrated
3. Consider hybrid approach where generator handles simple cases

## Recommendation

The struct tag approach can successfully migrate approximately 70-80% of system variables, which would still provide significant value by:
- Reducing boilerplate for common cases
- Standardizing variable definitions
- Making it easier to add new simple variables

For the remaining 20-30% complex cases, keeping manual registration is acceptable and maintains flexibility for special requirements.

## Next Steps

1. Implement Phase 1 for simple variables
2. Test generated code against existing tests
3. Gradually extend tag syntax for Phase 2
4. Document clear guidelines on when to use tags vs manual registration