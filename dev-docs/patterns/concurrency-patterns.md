# Concurrency Patterns

This document describes concurrency patterns used in spanner-mycli, including mutex usage, goroutine management, and thread-safe design principles.

## Table of Contents
- [Overview](#overview)
- [Mutex Usage Patterns](#mutex-usage-patterns)
- [Closure-Based Resource Access](#closure-based-resource-access)
- [Heartbeat Optimization Pattern](#heartbeat-optimization-pattern)
- [Testing Concurrent Code](#testing-concurrent-code)

## Overview

While spanner-mycli is primarily a CLI tool with sequential user interactions, it employs concurrent patterns for:
- Background operations (heartbeats, progress monitoring)
- Thread-safe resource management
- Future extensibility for parallel operations

## Mutex Usage Patterns

### Basic Mutex Pattern

The standard pattern for protecting shared resources with a mutex:

```go
type Session struct {
    tcMutex sync.Mutex
    tc      *transactionContext
}

func (s *Session) SomeOperation() error {
    s.tcMutex.Lock()
    defer s.tcMutex.Unlock()
    
    // Access s.tc safely
    if s.tc == nil {
        return ErrNoTransaction
    }
    return s.tc.doSomething()
}
```

### Lock-Free Read Pattern

For read-only operations that return copies, provide lock-free methods:

```go
// TransactionAttrs returns a copy of transaction attributes.
// This is safe to call without holding the mutex since it returns a copy.
func (s *Session) TransactionAttrs() transactionAttributes {
    s.tcMutex.Lock()
    defer s.tcMutex.Unlock()
    
    if s.tc == nil {
        return transactionAttributes{} // Zero value
    }
    return s.tc.attrs // Return copy, not pointer
}
```

### Check-Before-Lock Pattern

For periodic operations, check state before acquiring expensive locks:

```go
func (s *Session) sendHeartbeat() {
    // First check without lock (using lock-free method)
    attrs := s.TransactionAttrs()
    if !attrs.sendHeartbeat {
        return // No heartbeat needed
    }
    
    // Only acquire lock if actually needed
    s.tcMutex.Lock()
    defer s.tcMutex.Unlock()
    
    // Re-check after acquiring lock
    if s.tc == nil || !s.tc.attrs.sendHeartbeat {
        return
    }
    
    // Send heartbeat
    s.tc.txn.SendHeartbeat()
}
```

## Closure-Based Resource Access

### Problem

Direct field access with mutexes is error-prone:

```go
// BAD: Race condition between check and use
func (s *Session) BadPattern() error {
    if s.InReadWriteTransaction() { // Lock acquired and released
        s.tcMutex.Lock()
        defer s.tcMutex.Unlock()
        return s.tc.txn.Update(...) // tc might be nil now!
    }
}
```

### Solution: Closure-Based Helpers

Use closure-based helpers that guarantee mutex scope:

```go
// withReadWriteTransaction executes fn with the current read-write transaction.
// Returns ErrNotInReadWriteTransaction if not in a read-write transaction.
func (s *Session) withReadWriteTransaction(fn func(*spanner.ReadWriteStmtBasedTransaction) error) error {
    s.tcMutex.Lock()
    defer s.tcMutex.Unlock()
    
    if s.tc == nil || s.tc.attrs.mode != transactionModeReadWrite {
        return ErrNotInReadWriteTransaction
    }
    
    // fn executes with mutex held, ensuring tc cannot change
    return fn(s.tc.txn.(*spanner.ReadWriteStmtBasedTransaction))
}

// Usage
err := s.withReadWriteTransaction(func(tx *spanner.ReadWriteStmtBasedTransaction) error {
    return tx.Update(ctx, stmt)
})
```

### Benefits

1. **Type Safety**: Compile-time verification of transaction type
2. **Atomicity**: Entire operation executes with mutex held
3. **RAII Pattern**: Mutex automatically released via defer
4. **No Nil Checks**: Helper validates state before calling closure

## Heartbeat Optimization Pattern

### Background

Long-running transactions need periodic heartbeats to stay alive, but constant mutex acquisition can cause contention.

### Implementation

```go
func (s *Session) startHeartbeat() {
    ticker := time.NewTicker(5 * time.Second)
    s.heartbeatContext, s.heartbeatCancel = context.WithCancel(context.Background())
    
    go func() {
        defer ticker.Stop()
        for {
            select {
            case <-ticker.C:
                // Check-before-lock optimization
                attrs := s.TransactionAttrs()
                if !attrs.sendHeartbeat {
                    continue
                }
                
                // Only lock when actually sending heartbeat
                s.withReadWriteTransactionContext(func(tx *spanner.ReadWriteStmtBasedTransaction, tc *transactionContext) error {
                    if tc.attrs.sendHeartbeat {
                        // Send with low priority to avoid interfering
                        ctx := context.WithValue(ctx, ctxKeyRequestPriority, sppb.RequestOptions_PRIORITY_LOW)
                        _, _ = tx.Query(ctx, spanner.NewStatement("SELECT 1"))
                    }
                    return nil
                })
                
            case <-s.heartbeatContext.Done():
                return
            }
        }
    }()
}
```

### Key Optimizations

1. **Check-before-lock**: Use lock-free `TransactionAttrs()` to check state
2. **Low Priority**: Heartbeats use `PRIORITY_LOW` to avoid interfering
3. **Request Tagging**: Tag heartbeat requests for easy filtering in logs
4. **Graceful Shutdown**: Use context cancellation for clean goroutine termination

## Testing Concurrent Code

### Race Detection

Always test with race detector enabled:

```bash
go test -race ./...
```

### Concurrent Test Pattern

For testing concurrent access:

```go
func TestConcurrentAccess(t *testing.T) {
    session := newSession()
    
    var wg sync.WaitGroup
    errors := make(chan error, 10)
    
    // Spawn multiple goroutines
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            // Perform concurrent operations
            if err := session.SomeOperation(); err != nil {
                errors <- err
            }
        }()
    }
    
    wg.Wait()
    close(errors)
    
    // Check for errors
    for err := range errors {
        t.Errorf("concurrent operation failed: %v", err)
    }
}
```

### Avoiding Test Deadlocks

When testing mutex-protected code:

1. Use timeouts to detect deadlocks
2. Test both success and error paths
3. Verify mutex is released in all cases

```go
func TestWithTimeout(t *testing.T) {
    done := make(chan bool)
    go func() {
        // Test operation
        session.SomeOperation()
        done <- true
    }()
    
    select {
    case <-done:
        // Success
    case <-time.After(5 * time.Second):
        t.Fatal("operation timed out - possible deadlock")
    }
}
```

## Best Practices

1. **Minimize Lock Scope**: Hold mutexes for the shortest time possible
2. **Defer Unlock**: Always use `defer` for unlock to ensure release
3. **Document Lock Requirements**: Clearly document which functions require locks
4. **Avoid Nested Locks**: Prevent deadlocks by avoiding nested mutex acquisitions
5. **Copy Return Values**: Return copies instead of pointers for lock-free reads
6. **Use sync.RWMutex**: Consider RWMutex for read-heavy workloads
7. **Test with -race**: Always run tests with race detector enabled

## See Also

- [Transaction Patterns](transaction-patterns.md) - Transaction-specific patterns
- [Testing Guidelines](../testing-guidelines.md) - General testing practices
- [Architecture Guide](../architecture-guide.md) - System architecture overview