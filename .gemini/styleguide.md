# Style Guide for Gemini Code Assist

This document provides guidance for Gemini Code Assist when reviewing code in this repository.

## Go Version and Language Features

This project uses **Go 1.24** as specified in `go.mod`. Please consider the following when reviewing Go code:

### Loop Variable Semantics (Go 1.22+)

Since Go 1.22, the loop variable semantics have changed to address the common pitfall of loop variable capture. Each iteration of a loop now creates a new instance of the loop variable, eliminating the need for explicit capturing in most cases.

**DO NOT** suggest capturing loop variables in range loops when reviewing code that targets Go 1.22 or later:

```go
// This is now safe in Go 1.22+ without explicit capture
for _, item := range items {
    t.Run(item.name, func(t *testing.T) {
        // No need for "item := item" capture
        doSomething(item)
    })
}
```

Reference: https://go.dev/blog/loopvar-preview

### Other Go Best Practices

- Follow standard Go idioms and conventions
- Prefer clarity over cleverness
- Use meaningful variable and function names
- Keep functions focused and testable
- Write comprehensive tests for new functionality

## Testing Guidelines

- Table-driven tests are preferred for comprehensive test coverage
- Use `t.Helper()` in test helper functions
- Use `t.Setenv()` instead of `os.Setenv()` for better test isolation
- Avoid global state modifications in tests

## Code Review Focus

When reviewing pull requests, please focus on:

1. **Correctness**: Does the code do what it claims to do?
2. **Test Coverage**: Are there adequate tests for new functionality?
3. **Error Handling**: Are errors properly handled and returned?
4. **Resource Management**: Are resources (files, connections, etc.) properly closed?
5. **Documentation**: Are public APIs and complex logic adequately documented?

Please avoid suggesting changes that are purely stylistic unless they significantly impact readability or maintainability.