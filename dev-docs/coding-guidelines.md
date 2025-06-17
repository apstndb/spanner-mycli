# Coding Guidelines

This document defines the coding guidelines for the spanner-mycli project, based on Go language standards and project-specific conventions.

## Summary of Findings

### 1. Capitalized Error Messages
- **Issue**: Some error messages start with uppercase letters
- **Go Convention**: Error strings should not be capitalized unless they begin with proper nouns or acronyms
- **Example Pattern**: `"EXPLAIN statement is not supported"` should be `"explain statement is not supported"` or better: `"unsupported by emulator: EXPLAIN"`

### 2. Missing Doc Comments on Exported Functions
- **Issue**: Multiple exported functions lack doc comments
- **Go Convention**: All exported functions, types, constants, and variables should have doc comments starting with the name of the item
- **Affected Areas**: Session management, CLI functions, statement processing, utility functions
- **Note**: Since this project is not used as an external library, some internal-only exports may have minimal documentation by design

### 3. Exported Variables Without Doc Comments
- **Issue**: Some exported variables lack documentation
- **Go Convention**: Exported variables should have doc comments explaining their purpose

## Positive Findings ✅

### 1. Error Handling
- No patterns of silently ignored errors found
- Errors appear to be properly propagated

### 2. Naming Conventions
- Function and variable names follow Go conventions (camelCase, exported items start with uppercase)
- No Hungarian notation or underscore-separated names found

### 3. Package Organization
- Single package (main) structure is appropriate for a CLI tool
- No circular dependencies

## Recommendations

1. **Error Messages**: Ensure new error messages follow the lowercase convention
2. **Documentation**: Add doc comments to exported items in new code
3. **Future Improvements**: Consider restructuring to minimize unnecessary exports

## Status

As documented in `dev-docs/development-insights.md`, these existing deviations are temporarily permitted but will be addressed in future refactoring. New code should follow standard Go conventions.