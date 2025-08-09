# Code Generation Approach Evaluation

## Overview

After implementing proof-of-concepts and analyzing the codebase, here's an evaluation of different code generation approaches for the sysvar framework.

## Approaches Evaluated

### 1. Stringer Tool (Implemented)
- **Pros**: 
  - Standard Go tool, well-maintained
  - Perfect for int-based enums
  - Zero configuration needed
- **Cons**: 
  - Generated strings (e.g., "DisplayModeTable") don't match user-facing format ("TABLE")
  - Only works for int-based enums, not string-based
- **Verdict**: ✅ Use for debugging/logging, not for user-facing strings

### 2. Struct Tag-based Generator (POC Implemented)
- **Pros**:
  - Go-idiomatic approach
  - Single source of truth
  - Type-safe with compiler validation
  - Flexible for all variable types
- **Cons**:
  - Requires custom tooling
  - Complex types need more work
- **Verdict**: ✅ Best overall approach for general system variables

### 3. Protocol Buffer Enum Unification (User's Question)

The user asked: "ところで enum の定義を Protocol Buffers に統一することで単純化できる実装があるのではないですか？"
(By the way, isn't there an implementation that could be simplified by unifying enum definitions to Protocol Buffers?)

**Analysis**:

#### Current Local Enums
```go
// String-based enums
type explainFormat string     // "CURRENT", "TRADITIONAL", "COMPACT"
type parseMode string         // "FALLBACK", "NO_MEMEFISH", "MEMEFISH_ONLY"
type AutocommitDMLMode bool  // Actually a boolean, not enum

// Int-based enum
type DisplayMode int          // iota-based constants
```

#### Protobuf Unification Approach
```proto
// hypothetical sysvar_enums.proto
enum DisplayMode {
  DISPLAY_MODE_UNSPECIFIED = 0;
  DISPLAY_MODE_TABLE = 1;
  DISPLAY_MODE_TABLE_COMMENT = 2;
  // ...
}

enum ExplainFormat {
  EXPLAIN_FORMAT_UNSPECIFIED = 0;
  EXPLAIN_FORMAT_CURRENT = 1;
  EXPLAIN_FORMAT_TRADITIONAL = 2;
  EXPLAIN_FORMAT_COMPACT = 3;
}
```

**Pros**:
- Unified enum handling with `BuildProtobufEnumMap`
- Automatic String() methods
- Consistent validation
- Could generate both Go and proto from single source

**Cons**:
- Adds protobuf compilation step
- Overkill for CLI-only enums (never sent over network)
- Forces int32 type (not always appropriate)
- String-based enums more natural for some cases
- Would need to maintain .proto files

**Verdict**: ❌ Not recommended because:
1. These enums are internal to CLI, never serialized
2. String-based enums are more readable for parsing mode
3. Adds unnecessary complexity to build process

### 4. Alternative: Type Definition Generator

Instead of Protocol Buffers, generate the enum types themselves:

```go
//go:generate go run ./tools/enumgen -type DisplayMode -values TABLE,TABLE_COMMENT,VERTICAL...
```

This would generate:
- The type definition
- Constants
- String() method with correct user-facing strings
- Parse function
- Validation

**Verdict**: ✅ Better than protobuf for local enums

## Recommended Approach

**Hybrid Strategy**:

1. **For DisplayMode and similar int enums**: 
   - Use `stringer` for debugging String() method
   - Keep manual `FormatEnumFromMap` for user-facing strings
   
2. **For system variable registration**:
   - Implement struct tag-based generator
   - Start with simple types, gradually add complex ones
   
3. **For local enums**:
   - Keep as-is (string-based enums are fine)
   - Don't convert to protobuf unless actually needed for serialization
   
4. **For protobuf enums**:
   - Continue using existing `RegisterProtobufEnum` helpers

## Implementation Priority

1. **Phase 1**: Polish struct tag generator for basic types (bool, string, int)
2. **Phase 2**: Add nullable type support
3. **Phase 3**: Add enum support with value definitions in tags
4. **Phase 4**: Generate tests alongside registration code

## Conclusion

The struct tag-based approach provides the best balance of:
- Go idiomaticity
- Maintainability  
- Flexibility
- Type safety

Protocol Buffer unification would add complexity without significant benefits for CLI-internal enums.