# Style Guide for Gemini Code Assist

This document provides guidance for Gemini Code Assist when reviewing code in this repository.

## Go Version and Language Features

This project uses **Go 1.25** as specified in `go.mod`. Please consider the following when reviewing Go code:

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

## Concurrency and Race Conditions

### Logical vs Memory Race Conditions

When reviewing concurrent code, distinguish between:
1. **Memory race conditions**: Unsynchronized concurrent access to shared memory (undefined behavior)
2. **Logical race conditions**: Operations that appear atomic to callers but aren't (incorrect semantics)

Example of a logical race condition:
```go
// BAD: Logical race - operation appears atomic but isn't
func (m *Manager) SetFile(path string) error {
    file, err := os.Open(path)  // Outside lock
    if err != nil {
        return err
    }
    
    m.mu.Lock()
    defer m.mu.Unlock()
    
    // Another thread could have changed m.file between Open and Lock
    if m.file != nil {
        m.file.Close()  // Might close a different file than expected
    }
    m.file = file
    return nil
}
```

While there's no memory race (all access to `m.file` is synchronized), concurrent calls violate the expected semantics - a successful call might have its file immediately closed by another thread.

### When File I/O Inside Locks is Appropriate

For operations that must be atomic (like configuration changes), it's often necessary to hold the lock during file I/O:

```go
// GOOD: Entire operation is atomic
func (m *Manager) SetFile(path string) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    file, err := os.Open(path)  // Inside lock for atomicity
    if err != nil {
        return err
    }
    
    if m.file != nil {
        m.file.Close()
    }
    m.file = file
    return nil
}
```

The performance impact is usually acceptable for infrequent operations like configuration changes in CLI tools.

### Mutex Protection in CLI Tools

While CLI tools typically have sequential user interactions, proper mutex usage is still important:

1. **Background operations**: CLI tools may spawn goroutines for:
   - Progress bars and monitoring
   - Background heartbeats for long-running operations
   - Signal handlers (e.g., Ctrl+C)
   - Future parallel features

2. **Check-then-act patterns**: Always look for logical races in patterns like:
   ```go
   // BAD: Check-then-act race
   if !s.InTransaction() {
       s.BeginTransaction()  // Another goroutine might begin between check and act
   }
   
   // GOOD: Atomic operation
   attrs := s.TransactionAttrs()  // Single atomic call
   if attrs.mode == transactionModeNone {
       // ...
   }
   ```

3. **Closure-based patterns**: When reviewing transaction or session code, prefer closure-based patterns that encapsulate critical sections:
   ```go
   // GOOD: Mutex held throughout operation
   func (s *Session) withTransaction(fn func(tx *Transaction) error) error {
       s.mu.Lock()
       defer s.mu.Unlock()
       
       if s.tx == nil {
           return ErrNoTransaction
       }
       return fn(s.tx)
   }
   ```

### Helper Functions for Concurrent Access

When reviewing code that accesses shared state, look for opportunities to suggest helper functions:

1. **Encapsulation**: Helper functions should encapsulate the entire critical section
2. **Type safety**: Use specific types rather than generic interfaces where possible
3. **Clear ownership**: Document which functions are allowed direct access to shared state

Example review feedback:
```go
// Instead of direct access scattered throughout:
s.mu.Lock()
tx := s.tc.txn
s.mu.Unlock()
result := tx.Query(...)  // BAD: tx might be invalid

// Suggest helper functions:
result, err := s.runQueryOnTransaction(ctx, stmt)  // GOOD: Encapsulated
```

### Periodic Operations and Mutex Contention

When reviewing code with periodic operations (timers, heartbeats, monitoring), pay special attention to mutex usage:

1. **Check-before-lock pattern**: For frequently executed operations, check state using lock-free methods before acquiring mutex
2. **Document performance rationale**: When optimizing mutex usage, explain why in comments

Example of good periodic operation pattern:
```go
// GOOD: Minimize mutex contention in periodic operations
ticker := time.NewTicker(5 * time.Second)
for range ticker.C {
    // Check state without mutex first (lock-free read)
    if !s.shouldProcess() {
        continue
    }
    
    // Only acquire mutex when actually needed
    s.mu.Lock()
    s.doWork()
    s.mu.Unlock()
}

// BAD: Acquires mutex every tick regardless of need
for range ticker.C {
    s.mu.Lock()
    if s.needsWork {
        s.doWork()
    }
    s.mu.Unlock()
}
```

This pattern is especially important in:
- Heartbeat mechanisms
- Health checks
- Metric collection
- Background cleanup tasks

## CLI Tool Error Notification

For CLI tools, user-facing error notifications should use standard streams:
- **stderr**: For warnings and non-fatal errors that users need to see
- **slog**: For internal debugging and structured logging

Example:
```go
// GOOD: User-facing warning via stderr
fmt.Fprintf(errStream, "WARNING: Operation failed: %v\n", err)

// AVOID: Using slog for user notifications
slog.Error("Operation failed", "error", err)  // User might not see this
```

## Code Review Context

When reviewing code, ensure you have the full context:
- Check if suggested fixes are already implemented nearby
- Read comments that explain design decisions
- Verify line numbers match the actual issue location

Common false positives to avoid:
1. Suggesting to add code that already exists (e.g., TOCTOU double-checks)
2. Missing extensive comments that explain the current design
3. Suggesting changes that violate principles already documented in the code

## Output Format Memory Patterns

### Memory Buffering vs Streaming in Output Formats

Different output formats in `cli_output.go` use different memory patterns by design:

**Buffering formats** (build complete output in memory):
- TABLE format - Uses `strings.Builder` to calculate column widths
- XML format - Builds complete structure for well-formed output

**Streaming formats** (output row-by-row):
- VERTICAL format - Simple key-value pairs
- TAB format - Simple delimited values
- HTML format - Simple tag structure allows streaming

**DO NOT** suggest converting buffering formats to streaming without considering:
1. **Complexity**: XML with proper headers and structure is complex to stream correctly
2. **Consistency**: TABLE and XML formats intentionally follow the same pattern
3. **Context**: This is an interactive CLI tool, not a bulk export utility
4. **Documentation**: If the code explicitly documents the trade-off, respect it

Example of documented trade-off:
```go
// Note: This implementation builds the entire result set in memory before
// encoding. For very large result sets, consider using TAB format for
// streaming output. This design prioritizes simplicity and consistency
// with the TABLE format over memory efficiency.
```

For users needing to process very large datasets, the TAB format already provides a streaming option.

## Code Review Focus

When reviewing pull requests, please focus on:

1. **Correctness**: Does the code do what it claims to do?
2. **Test Coverage**: Are there adequate tests for new functionality?
3. **Error Handling**: Are errors properly handled and returned?
4. **Resource Management**: Are resources (files, connections, etc.) properly closed?
5. **Documentation**: Are public APIs and complex logic adequately documented?

Please avoid suggesting changes that are purely stylistic unless they significantly impact readability or maintainability.