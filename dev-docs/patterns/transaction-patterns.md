# Transaction Patterns

This document describes transaction management patterns in spanner-mycli, focusing on safe concurrent access, lifecycle management, and testing strategies.

## Table of Contents
- [Overview](#overview)
- [Transaction Context Management](#transaction-context-management)
- [Closure-Based Transaction Helpers](#closure-based-transaction-helpers)
- [Transaction Lifecycle](#transaction-lifecycle)
- [Testing Unmockable Transaction Types](#testing-unmockable-transaction-types)
- [Heartbeat Management](#heartbeat-management)

## Overview

spanner-mycli manages three types of transactions:
- **Read-Write Transactions**: For DML operations (INSERT, UPDATE, DELETE)
- **Read-Only Transactions**: For consistent multi-read operations
- **Pending Transactions**: Placeholder state before transaction type is determined

All transaction access must be thread-safe due to background operations like heartbeats.

## Transaction Context Management

### Transaction Context Structure

```go
type transactionContext struct {
    txn   interface{} // *spanner.ReadWriteStmtBasedTransaction or *spanner.ReadOnlyTransaction
    attrs transactionAttributes
}

type transactionAttributes struct {
    mode              transactionMode
    sendHeartbeat     bool
    tag               string
    priority          sppb.RequestOptions_Priority
    isolationLevel    sppb.TransactionOptions_ReadWrite_IsolationLevel
    timestampBoundType timestampBoundType
    staleness         time.Duration
    timestamp         time.Time
}
```

### Transaction Modes

```go
type transactionMode string

const (
    transactionModeUndetermined = ""
    transactionModeReadWrite    = "ReadWrite"
    transactionModeReadOnly     = "ReadOnly"
    transactionModePending      = "Pending"
)
```

## Closure-Based Transaction Helpers

### Design Rationale

Direct transaction access is unsafe due to potential race conditions:

```go
// UNSAFE: Race between check and use
if s.InReadWriteTransaction() {
    // Another goroutine might change transaction state here!
    s.tc.txn.Update(...) // Potential nil panic
}
```

### Safe Access Pattern

Use closure-based helpers that hold the mutex for the entire operation:

```go
// Safe helper for read-write transactions
func (s *Session) withReadWriteTransaction(fn func(*spanner.ReadWriteStmtBasedTransaction) error) error {
    s.tcMutex.Lock()
    defer s.tcMutex.Unlock()
    
    if s.tc == nil || s.tc.attrs.mode != transactionModeReadWrite {
        return ErrNotInReadWriteTransaction
    }
    
    return fn(s.tc.txn.(*spanner.ReadWriteStmtBasedTransaction))
}

// Safe helper for read-only transactions
func (s *Session) withReadOnlyTransaction(fn func(*spanner.ReadOnlyTransaction) error) error {
    s.tcMutex.Lock()
    defer s.tcMutex.Unlock()
    
    if s.tc == nil || s.tc.attrs.mode != transactionModeReadOnly {
        return ErrNotInReadOnlyTransaction
    }
    
    return fn(s.tc.txn.(*spanner.ReadOnlyTransaction))
}
```

### Helper with Context Access

For operations that need to modify transaction attributes:

```go
func (s *Session) withReadWriteTransactionContext(
    fn func(*spanner.ReadWriteStmtBasedTransaction, *transactionContext) error,
) error {
    s.tcMutex.Lock()
    defer s.tcMutex.Unlock()
    
    if s.tc == nil || s.tc.attrs.mode != transactionModeReadWrite {
        return ErrNotInReadWriteTransaction
    }
    
    return fn(s.tc.txn.(*spanner.ReadWriteStmtBasedTransaction), s.tc)
}
```

### Usage Examples

```go
// Execute update in transaction
err := s.withReadWriteTransaction(func(tx *spanner.ReadWriteStmtBasedTransaction) error {
    _, err := tx.Update(ctx, spanner.NewStatement("UPDATE users SET active = true"))
    return err
})

// Query with stats
var result QueryResult
err := s.withReadOnlyTransaction(func(tx *spanner.ReadOnlyTransaction) error {
    iter := tx.QueryWithStats(ctx, stmt)
    result.RowIterator = iter
    return nil
})

// Modify transaction attributes
err := s.withReadWriteTransactionContext(func(tx *spanner.ReadWriteStmtBasedTransaction, tc *transactionContext) error {
    tc.attrs.sendHeartbeat = true // Enable heartbeat
    return nil
})
```

## Transaction Lifecycle

### 1. Begin Transaction

```go
func (s *Session) BeginReadWriteTransaction(ctx context.Context, isolation, priority) error {
    s.tcMutex.Lock()
    defer s.tcMutex.Unlock()
    
    // Validate no active transaction
    if err := s.validateNoActiveTransactionLocked(); err != nil {
        return err
    }
    
    // Create new transaction
    txn, err := s.client.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteStmtBasedTransaction) error {
        s.tc = &transactionContext{
            txn: txn,
            attrs: transactionAttributes{
                mode:           transactionModeReadWrite,
                sendHeartbeat:  true,
                isolationLevel: isolation,
                priority:      priority,
            },
        }
        return nil
    })
    
    if err != nil {
        s.clearTransactionContext()
        return err
    }
    
    // Start heartbeat goroutine
    s.startHeartbeat()
    return nil
}
```

### 2. Use Transaction

All operations use the closure-based helpers:

```go
// Query operation
func (s *Session) runQueryWithStatsOnTransaction(ctx context.Context, stmt spanner.Statement) (QueryResult, error) {
    var result QueryResult
    
    err := s.withReadOnlyTransaction(func(tx *spanner.ReadOnlyTransaction) error {
        iter := tx.QueryWithStats(ctx, stmt)
        result.RowIterator = iter
        return nil
    })
    
    if err == nil {
        err = s.withReadWriteTransaction(func(tx *spanner.ReadWriteStmtBasedTransaction) error {
            iter := tx.QueryWithStats(ctx, stmt)
            result.RowIterator = iter
            return nil
        })
    }
    
    return result, err
}
```

### 3. Commit/Rollback

```go
func (s *Session) CommitReadWriteTransaction(ctx context.Context) (spanner.CommitResponse, error) {
    var resp spanner.CommitResponse
    
    err := s.withReadWriteTransaction(func(tx *spanner.ReadWriteStmtBasedTransaction) error {
        var err error
        resp, err = tx.CommitWithReturnResp(ctx)
        return err
    })
    
    if err == nil {
        s.clearTransactionContext() // Clear on success
    }
    
    return resp, err
}
```

## Testing Unmockable Transaction Types

### Challenge

Spanner transaction types have unexported fields and cannot be mocked:

```go
// This doesn't work - unexported fields!
mockTx := &spanner.ReadOnlyTransaction{}
```

### Solution: Two-Tier Testing

#### 1. Unit Tests (Without Real Transactions)

Test error paths and mutex behavior:

```go
func TestTransactionHelpers_ErrorPaths(t *testing.T) {
    tests := []struct {
        name    string
        setupTC func() *transactionContext
        wantErr error
    }{
        {
            name:    "no transaction",
            setupTC: func() *transactionContext { return nil },
            wantErr: ErrNotInReadWriteTransaction,
        },
        {
            name: "wrong transaction type",
            setupTC: func() *transactionContext {
                return &transactionContext{
                    attrs: transactionAttributes{mode: transactionModeReadOnly},
                }
            },
            wantErr: ErrNotInReadWriteTransaction,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            s := &Session{tc: tt.setupTC()}
            
            err := s.withReadWriteTransaction(func(tx *spanner.ReadWriteStmtBasedTransaction) error {
                t.Fatal("should not be called")
                return nil
            })
            
            if err != tt.wantErr {
                t.Errorf("got %v, want %v", err, tt.wantErr)
            }
        })
    }
}
```

#### 2. Integration Tests (With Spanner Emulator)

Test actual transaction behavior:

```go
func TestTransactionHelpers_Integration(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test")
    }
    
    ctx := context.Background()
    _, session, cleanup := setupTestSession(t)
    defer cleanup()
    
    // Begin real transaction
    err := session.BeginReadWriteTransaction(ctx, isolation, priority)
    if err != nil {
        t.Fatal(err)
    }
    defer session.RollbackReadWriteTransaction(ctx)
    
    // Test with real transaction
    err = session.withReadWriteTransaction(func(tx *spanner.ReadWriteStmtBasedTransaction) error {
        // This actually executes!
        iter := tx.Query(ctx, spanner.NewStatement("SELECT 1"))
        defer iter.Stop()
        
        _, err := iter.Next()
        return err
    })
    
    if err != nil {
        t.Errorf("unexpected error: %v", err)
    }
}
```

### Testing Strategy Summary

| Test Type | What to Test | How to Test |
|-----------|--------------|-------------|
| Unit Tests | Error handling, mutex behavior, state transitions | Mock transaction context, test without real txn |
| Integration Tests | Actual queries, commits, rollbacks | Use Spanner emulator with real transactions |
| Race Tests | Concurrent access safety | Run with `-race` flag |

## Heartbeat Management

### Purpose

Keep long-running read-write transactions alive by sending periodic keep-alive queries.

### Implementation Details

```go
func (s *Session) startHeartbeat() {
    ticker := time.NewTicker(5 * time.Second)
    s.heartbeatContext, s.heartbeatCancel = context.WithCancel(context.Background())
    
    go func() {
        defer ticker.Stop()
        
        for {
            select {
            case <-ticker.C:
                s.sendHeartbeat()
            case <-s.heartbeatContext.Done():
                return
            }
        }
    }()
}

func (s *Session) sendHeartbeat() {
    // Check-before-lock optimization
    attrs := s.TransactionAttrs()
    if !attrs.sendHeartbeat {
        return
    }
    
    _ = s.withReadWriteTransaction(func(tx *spanner.ReadWriteStmtBasedTransaction) error {
        // Send low-priority SELECT 1
        ctx := context.WithValue(context.Background(), ctxKeyRequestPriority, sppb.RequestOptions_PRIORITY_LOW)
        ctx = context.WithValue(ctx, ctxKeyRequestTag, "heartbeat")
        
        iter := tx.Query(ctx, spanner.NewStatement("SELECT 1"))
        defer iter.Stop()
        _, _ = iter.Next()
        
        return nil
    })
}
```

### Best Practices

1. **Low Priority**: Use `PRIORITY_LOW` to avoid interfering with real work
2. **Request Tagging**: Tag as "heartbeat" for easy log filtering
3. **Error Tolerance**: Ignore heartbeat errors (transaction might be closing)
4. **Clean Shutdown**: Always cancel heartbeat context when transaction ends

## Best Practices

1. **Always Use Helpers**: Never access `s.tc` directly outside helper functions
2. **Validate State**: Check transaction mode before type assertions
3. **Clear on Completion**: Always clear transaction context after commit/rollback
4. **Test Both Tiers**: Unit tests for logic, integration tests for behavior
5. **Document Mutex Requirements**: Clearly mark functions that require mutex
6. **Use Sentinel Errors**: Define specific errors for each failure mode

## See Also

- [Concurrency Patterns](concurrency-patterns.md) - General concurrency patterns
- [Architecture Guide](../architecture-guide.md) - System architecture
- [Testing Guidelines](../testing-guidelines.md) - Testing best practices