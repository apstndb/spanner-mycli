# sysvargen - System Variable Code Generator Proof of Concept

This is a proof-of-concept for generating system variable registration code from struct tags.

## Concept

Instead of manually writing registration code like:

```go
mustRegister(registry, sysvar.NewBooleanParser(
    "READONLY",
    "A boolean indicating whether or not the connection is in read-only mode.",
    sysvar.GetValue(&sv.ReadOnly),
    func(v bool) error {
        if sv.CurrentSession != nil && (sv.CurrentSession.InReadOnlyTransaction() || sv.CurrentSession.InReadWriteTransaction()) {
            return errors.New("can't change READONLY when there is a active transaction")
        }
        sv.ReadOnly = v
        return nil
    },
))
```

You can annotate fields with struct tags:

```go
type systemVariables struct {
    ReadOnly bool `sysvar:"name=READONLY,desc='A boolean indicating whether or not the connection is in read-only mode',setter=setReadOnly"`
}
```

## Usage

```bash
cd tools/sysvargen
go run cmd/sysvargen/main.go -input example/system_variables_example.go
```

## Features Demonstrated

- ✅ Boolean variables with simple getters/setters
- ✅ String variables (read-write and read-only)
- ✅ Integer variables
- ✅ Custom setter methods
- ✅ Read-only variables
- ⚠️ Nullable types (recognized but not fully implemented)
- ⚠️ Enum types (recognized but not fully implemented)
- ⚠️ Proto enum types (recognized but not fully implemented)

## Benefits

1. **Reduced Boilerplate**: Single source of truth for variable definitions
2. **Type Safety**: Go compiler validates struct fields
3. **Consistency**: Generated code follows same patterns
4. **Documentation**: Variable descriptions are part of the definition
5. **Maintainability**: Adding new variables is simpler

## Limitations of Current POC

1. Type resolution is simplified (doesn't resolve custom types fully)
2. Enum handling needs more work
3. No support for complex validation rules in tags
4. No support for generating tests

## Next Steps

To make this production-ready:

1. Improve type resolution using go/types package
2. Add support for enum definitions in tags
3. Generate test cases alongside registration code
4. Add validation for tag syntax
5. Support more complex types (nullable, proto enums)
6. Add go:generate directives to main codebase

## Alternative Approaches Considered

1. **Protocol Buffer Custom Options**: Would work well for proto enums but not for local types
2. **Reflection at Runtime**: Would lose compile-time safety
3. **YAML/JSON Configuration**: Less Go-idiomatic, requires separate schema
4. **Template-based Generation**: More flexible but requires maintaining templates