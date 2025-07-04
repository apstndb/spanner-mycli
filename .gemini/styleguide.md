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

### Error Wrapping with fmt.Errorf (Go 1.20+)

Since Go 1.20, `fmt.Errorf` has enhanced flexibility for error wrapping with the `%w` verb:

- **The `%w` verb can appear anywhere in the format string**, not just at the end
- **Multiple `%w` verbs are allowed** in a single `fmt.Errorf` call
- The position of `%w` in the format string does not affect the error wrapping functionality

**DO NOT** suggest that `%w` must be at the end of the format string:

```go
// All of these are valid in Go 1.20+
err1 := fmt.Errorf("command failed: %w\nadditional info", originalErr)  // Valid
err2 := fmt.Errorf("prefix: %w, suffix", originalErr)                   // Valid
err3 := fmt.Errorf("first: %w, second: %w", err1, err2)                // Valid with multiple %w

// errors.Is() and errors.Unwrap() work correctly with all of the above
```

Reference: https://go.dev/doc/go1.20#errors

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

### Global State Exceptions

While global state modifications should generally be avoided in tests, `slog` (structured logging) is an **accepted exception** in this project because:

1. Many functions throughout the codebase use the global `slog` for logging
2. Refactoring all functions to accept a logger parameter would be impractical
3. The save/restore pattern provides adequate test isolation

**When modifying slog in tests, you MUST use the save/restore pattern**:

```go
// Save the original logger
oldLogger := slog.Default()
defer slog.SetDefault(oldLogger)

// Set up test-specific logger
handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
    Level: slog.LevelDebug,
})
slog.SetDefault(slog.New(handler))
```

This pattern ensures that global logger modifications don't affect other tests, even when running in parallel.

### Package-level Test Functions

In Go, test files within the same package share the same namespace. This means that helper functions defined in one test file (e.g., `main_flags_test.go`) are accessible from other test files in the same package (e.g., `main_platform_test.go`).

**DO NOT** report as missing when a test uses a helper function defined in another test file of the same package:

```go
// In main_flags_test.go
func contains(s, substr string) bool {
    return strings.Contains(s, substr)
}

// In main_platform_test.go (same package)
// This is valid - contains() is accessible
if !contains(platform, tt.wantOS) {
    // ...
}
```

## Security Review Guidelines

### Threat Model Awareness

spanner-mycli is a **personal CLI tool** designed for interactive database management by trusted users on their local machines. When reviewing security-related code, consider the appropriate threat model:

**DO** consider these practical safety measures:
- File size limits to prevent accidental OOM from large files
- Validation to prevent reading from special files (e.g., `/dev/zero`)
- Input validation to prevent common user errors

**DO NOT** suggest excessive security measures for threats outside the tool's scope:
- TOCTOU (Time-of-check to time-of-use) vulnerabilities in local file operations
- Complex authentication schemes for local file access
- Cryptographic validation of local SQL files
- Defense against malicious local file system attacks

Example of appropriate vs excessive security review:
```go
// Appropriate: Prevent accidental resource exhaustion
if fi.Size() > maxFileSize {
    return fmt.Errorf("file too large: %d bytes", fi.Size())
}

// Excessive for a local CLI tool: TOCTOU protection
// DO NOT suggest opening the file first to prevent TOCTOU
```

Remember: This is a tool where users execute their own SQL files, not a service processing untrusted input. Security measures should focus on preventing accidents and misuse, not defending against adversarial attacks on the local system.

## Concurrency and Mutex Usage

### File I/O Outside Mutex Locks

When implementing concurrent-safe operations that involve file I/O, it's often appropriate to perform the I/O operations **outside** the mutex lock to avoid blocking other operations:

```go
// GOOD: File I/O outside lock prevents blocking other operations
func (m *Manager) SetFile(path string) error {
    // Open file BEFORE acquiring lock
    file, err := os.Open(path)
    if err != nil {
        return err
    }
    
    m.mu.Lock()
    defer m.mu.Unlock()
    
    // Quick operations under lock
    if m.file != nil {
        m.file.Close()
    }
    m.file = file
    return nil
}
```

**DO NOT** suggest moving file I/O inside mutex locks purely for theoretical race-condition elimination when:
1. The "race" results in acceptable "last write wins" semantics
2. The operation is typically single-threaded in practice (CLI commands, configuration)
3. Resource cleanup is properly maintained regardless of ordering

### Configuration Semantics

For configuration-changing methods (like `EnableTee`, `SetOutput`, etc.), "last write wins" is the expected behavior. This matches standard software behavior where the most recent configuration change takes effect.

## Code Review Focus

When reviewing pull requests, please focus on:

1. **Correctness**: Does the code do what it claims to do?
2. **Test Coverage**: Are there adequate tests for new functionality?
3. **Error Handling**: Are errors properly handled and returned?
4. **Resource Management**: Are resources (files, connections, etc.) properly closed?
5. **Documentation**: Are public APIs and complex logic adequately documented?

Please avoid suggesting changes that are purely stylistic unless they significantly impact readability or maintainability.