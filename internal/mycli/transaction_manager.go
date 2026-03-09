//
// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package mycli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/internal/mycli/decoder"
	"github.com/samber/lo"
)

// Transaction state errors
var (
	ErrNoTransaction             = errors.New("no active transaction")
	ErrNotInReadWriteTransaction = errors.New("not in read-write transaction")
	ErrNotInReadOnlyTransaction  = errors.New("not in read-only transaction")
	ErrNotInPendingTransaction   = errors.New("not in pending transaction")
)

// newStoppedIterator creates a stopped RowIterator.
// When a stopped iterator is used (via Next() or Do()), it immediately returns iterator.Done.
// This is the idiomatic way in the Google Cloud Spanner library to handle cases where
// we need to return a non-nil iterator that indicates no results.
//
// Note: We cannot return custom errors through a stopped iterator. The caller will need
// to check for other indicators (like nil transaction) to understand why the query failed.
func newStoppedIterator() *spanner.RowIterator {
	iter := &spanner.RowIterator{}
	iter.Stop()
	return iter
}

// QueryResult holds the result of a query operation with optional transaction.
type QueryResult struct {
	Iterator    *spanner.RowIterator
	Transaction *spanner.ReadOnlyTransaction
}

// UpdateResult holds the complete result of an update operation.
type UpdateResult struct {
	Rows     []Row                   // Rows returned by the update operation (e.g., from RETURNING clause)
	Stats    map[string]any          // Query statistics from the operation
	Count    int64                   // Number of rows affected by the update
	Metadata *sppb.ResultSetMetadata // Metadata about the result set
	Plan     *sppb.QueryPlan         // Query execution plan (when requested)
}

// DMLResult holds the results of a DML operation execution including commit information.
type DMLResult struct {
	Affected       int64                   // Total number of rows affected by the DML operation
	CommitResponse spanner.CommitResponse  // Commit response including timestamp and stats
	Plan           *sppb.QueryPlan         // Query execution plan (when requested)
	Metadata       *sppb.ResultSetMetadata // Metadata about the result set
}

// TransactionManager manages transaction lifecycle, query execution,
// and mutex-based concurrency for transaction state.
//
// Mutex Protection Rationale:
//
// While spanner-mycli is primarily a CLI tool with sequential user interactions,
// mutex protection is essential for several reasons:
//
// 1. **Goroutine safety**: Even in a CLI, operations may spawn goroutines:
//   - Background heartbeat for long-running transactions
//   - Progress bars and monitoring features
//   - Future parallel query execution features
//   - Signal handlers (Ctrl+C) that may access transaction state
//
// 2. **Correctness over performance**: The mutex ensures:
//   - Atomic check-and-act operations (e.g., checking transaction state before modifying)
//   - Prevention of use-after-free bugs if transaction is closed concurrently
//   - Consistent transaction state across all operations
//
// 3. **Future extensibility**: The mutex-based design allows for:
//   - Safe addition of concurrent features without major refactoring
//   - Integration with tools that may access session state concurrently
//   - Support for multiplexed connections or parallel operations
//
// 4. **Best practices**: Following Go's concurrency principles:
//   - "Don't communicate by sharing memory; share memory by communicating"
//   - When sharing is necessary (as with session state), protect it properly
//   - Make the zero value useful - mutex provides safe default behavior
//
// The performance overhead of mutex operations is negligible for CLI usage patterns,
// while the safety guarantees prevent subtle bugs that could corrupt user data.
type TransactionManager struct {
	tc *transactionContext
	// mu protects the transaction context (tc) from concurrent access.
	// Using RWMutex allows multiple concurrent reads while maintaining exclusive writes.
	// Direct access to tc is managed through withTransactionContextWithLock base method.
	// Helper functions that work with transaction context:
	// - withTransactionContextWithLock: Base method for all tc manipulation (acquires write lock)
	// - TransitTransaction: Atomic state transitions with cleanup
	// - withReadWriteTransaction, withReadWriteTransactionContext, withReadOnlyTransaction: Type-safe transaction access
	// - clearTransactionContext, TransactionAttrsWithLock: Safe context management
	// All transaction context access MUST go through these helpers.
	mu           sync.RWMutex
	client       *spanner.Client      // same pointer as Session.client
	sysVars      *systemVariables     // same pointer as Session.systemVariables
	clientConfig spanner.ClientConfig // for directed read in tryQueryInTransaction
}

// NewTransactionManager creates a new TransactionManager.
func NewTransactionManager(client *spanner.Client, sysVars *systemVariables, clientConfig spanner.ClientConfig) *TransactionManager {
	return &TransactionManager{
		client:       client,
		sysVars:      sysVars,
		clientConfig: clientConfig,
	}
}

// withTransactionContextWithLock is the most generic base method for transaction context manipulation.
// It acquires a write lock and provides direct access to the transaction context pointer,
// allowing atomic read-modify-write operations.
// The function fn receives a pointer to the transaction context pointer, enabling it to replace the entire context.
// NOTE: This is the foundation for all transaction state management operations that modify state.
func (tm *TransactionManager) withTransactionContextWithLock(fn func(tc **transactionContext) error) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return fn(&tm.tc)
}

// TransitTransaction implements a functional state transition pattern for transaction management.
// It atomically transitions from one transaction state to another, handling cleanup of the old state.
// The transition function receives the current context and returns the new context.
// If an error occurs during transition, the original state is preserved.
func (tm *TransactionManager) TransitTransaction(ctx context.Context, fn func(tc *transactionContext) (*transactionContext, error)) error {
	return tm.withTransactionContextWithLock(func(tcPtr **transactionContext) error {
		oldTc := *tcPtr
		newTc, err := fn(oldTc)
		if err != nil {
			return err
		}

		// Cleanup old transaction context if it's being replaced
		if oldTc != nil && oldTc != newTc {
			oldTc.Close()
		}

		*tcPtr = newTc
		return nil
	})
}

// withTransactionLocked is a generic helper that executes fn while holding the transaction mutex.
// It validates the transaction mode and returns an appropriate error if the mode doesn't match.
// NOTE: The mutex must NOT be held by the caller - this function acquires it.
func (tm *TransactionManager) withTransactionLocked(mode transactionMode, fn func() error) error {
	return tm.withTransactionContextWithLock(func(tcPtr **transactionContext) error {
		if *tcPtr == nil || (*tcPtr).txn == nil || (*tcPtr).attrs.mode != mode {
			switch mode {
			case transactionModeReadWrite:
				return ErrNotInReadWriteTransaction
			case transactionModeReadOnly:
				return ErrNotInReadOnlyTransaction
			default:
				return fmt.Errorf("not in %s transaction", mode)
			}
		}
		return fn()
	})
}

func (tm *TransactionManager) withReadWriteTransaction(fn func(*spanner.ReadWriteStmtBasedTransaction) error) error {
	// Reuse withReadWriteTransactionContext, ignoring the context parameter
	return tm.withReadWriteTransactionContext(func(txn *spanner.ReadWriteStmtBasedTransaction, _ *transactionContext) error {
		return fn(txn)
	})
}

// withReadWriteTransactionContext executes fn with both the transaction and context under mutex protection.
// This allows safe access to both the transaction and its context fields.
func (tm *TransactionManager) withReadWriteTransactionContext(fn func(*spanner.ReadWriteStmtBasedTransaction, *transactionContext) error) error {
	return tm.withTransactionLocked(transactionModeReadWrite, func() error {
		// At this point, we know tc is not nil and mode is correct (validated by withTransactionLocked)
		// Type assert the transaction safely
		txn, ok := tm.tc.txn.(*spanner.ReadWriteStmtBasedTransaction)
		if !ok {
			// This should never happen if the mode check is correct, but handle it defensively
			return fmt.Errorf("internal error: transaction has incorrect type for read-write mode (got %T)", tm.tc.txn)
		}
		return fn(txn, tm.tc)
	})
}

// withReadOnlyTransaction executes fn with the current read-only transaction under mutex protection.
// Returns ErrNotInReadOnlyTransaction if not in a read-only transaction.
func (tm *TransactionManager) withReadOnlyTransaction(fn func(*spanner.ReadOnlyTransaction) error) error {
	return tm.withTransactionLocked(transactionModeReadOnly, func() error {
		// At this point, we know tc is not nil and mode is correct (validated by withTransactionLocked)
		// Type assert the transaction safely
		txn, ok := tm.tc.txn.(*spanner.ReadOnlyTransaction)
		if !ok {
			// This should never happen if the mode check is correct, but handle it defensively
			return fmt.Errorf("internal error: transaction has incorrect type for read-only mode (got %T)", tm.tc.txn)
		}
		return fn(txn)
	})
}

// withReadOnlyTransactionOrStart executes fn with a read-only transaction.
// If already in a read-only transaction, uses it.
// If not in any transaction, starts a new read-only transaction and closes it after fn completes.
// If in a read-write transaction, returns an error as INFORMATION_SCHEMA queries cannot be used there.
//
// IMPORTANT: The callback fn must NOT call methods that try to acquire the transaction mutex
// (e.g., RunQueryWithStats). Only use methods designed for transaction contexts
// or direct transaction operations (like txn.Query).
func (tm *TransactionManager) withReadOnlyTransactionOrStart(ctx context.Context, fn func(*spanner.ReadOnlyTransaction) error) error {
	// First try to use existing RO transaction
	err := tm.withReadOnlyTransaction(fn)
	if err != ErrNotInReadOnlyTransaction {
		// Either succeeded with existing transaction or different error
		return err
	}

	// Not in RO transaction, start one
	_, err = tm.BeginReadOnlyTransaction(ctx, timestampBoundUnspecified, 0, time.Time{}, sppb.RequestOptions_PRIORITY_UNSPECIFIED)
	if err != nil {
		return fmt.Errorf("failed to begin read-only transaction: %w", err)
	}
	defer func() {
		_ = tm.CloseReadOnlyTransaction()
	}()

	// Now execute within the transaction we created
	return tm.withReadOnlyTransaction(fn)
}

// clearTransactionContext atomically clears the transaction context.
// This is equivalent to setTransactionContext(nil) but more expressive.
func (tm *TransactionManager) clearTransactionContext() {
	_ = tm.withTransactionContextWithLock(func(tcPtr **transactionContext) error {
		// Close the transaction context to stop any running heartbeat
		if *tcPtr != nil {
			(*tcPtr).Close()
		}
		*tcPtr = nil
		return nil
	})
}

// TransactionState returns the current transaction mode and whether a transaction is active.
// This consolidates multiple transaction state checks into a single method.
func (tm *TransactionManager) TransactionState() (mode transactionMode, isActive bool) {
	attrs := tm.TransactionAttrsWithLock()
	isActive = attrs.mode != transactionModeUndetermined && attrs.mode != ""
	return attrs.mode, isActive
}

// TransactionAttrsWithLock returns a copy of all transaction attributes.
// This allows safe inspection of transaction state without holding the mutex.
// If no transaction is active, returns a zero-value struct with mode=transactionModeUndetermined.
//
// Design decision: This method uses RLock for read-only access, allowing
// concurrent reads of transaction state.
//
// Naming convention:
// - TransactionAttrsWithLock() - Acquires read lock internally (this method)
// - transactionAttrsLocked() - Assumes caller holds lock (read or write)
func (tm *TransactionManager) TransactionAttrsWithLock() transactionAttributes {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	return tm.transactionAttrsLocked()
}

// transactionAttrsLocked returns transaction attributes without locking.
// Caller must hold tm.mu.
//
// IMPORTANT: This method exists to prevent deadlocks in DML execution paths.
// The withTransactionLocked method holds mu and calls callbacks that may
// need to access transaction attributes. Using TransactionAttrsWithLock() in those
// callbacks would attempt to acquire mu again, causing a deadlock since
// Go mutexes are not reentrant.
//
// Example deadlock scenario:
// 1. withTransactionLocked acquires mu
// 2. Calls callback (e.g., runUpdateOnTransaction)
// 3. Callback calls TransactionAttrsWithLock() which tries to acquire mu
// 4. Deadlock occurs
//
// Always use this locked version when already holding mu.
func (tm *TransactionManager) transactionAttrsLocked() transactionAttributes {
	if tm.tc == nil {
		// Return zero-value struct (mode will be transactionModeUndetermined = "")
		return transactionAttributes{}
	}

	// Return a copy of the attributes
	return tm.tc.attrs
}

// TransactionMode returns the current transaction mode.
// Deprecated: Use TransactionState() for new code.
func (tm *TransactionManager) TransactionMode() transactionMode {
	mode, _ := tm.TransactionState()
	return mode
}

// InReadWriteTransaction returns true if the session is running read-write transaction.
func (tm *TransactionManager) InReadWriteTransaction() bool {
	mode, _ := tm.TransactionState()
	return mode == transactionModeReadWrite
}

// InReadOnlyTransaction returns true if the session is running read-only transaction.
func (tm *TransactionManager) InReadOnlyTransaction() bool {
	mode, _ := tm.TransactionState()
	return mode == transactionModeReadOnly
}

// InPendingTransaction returns true if the session is running pending transaction.
func (tm *TransactionManager) InPendingTransaction() bool {
	mode, _ := tm.TransactionState()
	return mode == transactionModePending
}

// InTransaction returns true if the session is running transaction.
func (tm *TransactionManager) InTransaction() bool {
	_, isActive := tm.TransactionState()
	return isActive
}

// GetTransactionFlagsWithLock returns multiple transaction state flags in a single lock acquisition.
// This is useful for avoiding multiple lock acquisitions when checking different transaction states.
// Acquires RLock for concurrent read access.
func (tm *TransactionManager) GetTransactionFlagsWithLock() (inTransaction bool, inReadWriteTransaction bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if tm.tc == nil {
		return false, false
	}

	inTransaction = true
	inReadWriteTransaction = tm.tc.attrs.mode == transactionModeReadWrite
	return
}

// TransactionOptionsBuilder helps build transaction options with proper defaults.
type TransactionOptionsBuilder struct {
	tm             *TransactionManager
	priority       sppb.RequestOptions_Priority
	isolationLevel sppb.TransactionOptions_IsolationLevel
	tag            string
}

// NewTransactionOptionsBuilder creates a new builder with session defaults.
func (tm *TransactionManager) NewTransactionOptionsBuilder() *TransactionOptionsBuilder {
	return &TransactionOptionsBuilder{
		tm:             tm,
		priority:       sppb.RequestOptions_PRIORITY_UNSPECIFIED,
		isolationLevel: sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED,
	}
}

// WithPriority sets the transaction priority.
func (b *TransactionOptionsBuilder) WithPriority(priority sppb.RequestOptions_Priority) *TransactionOptionsBuilder {
	b.priority = priority
	return b
}

// WithIsolationLevel sets the transaction isolation level.
func (b *TransactionOptionsBuilder) WithIsolationLevel(level sppb.TransactionOptions_IsolationLevel) *TransactionOptionsBuilder {
	b.isolationLevel = level
	return b
}

// WithTag sets the transaction tag.
func (b *TransactionOptionsBuilder) WithTag(tag string) *TransactionOptionsBuilder {
	b.tag = tag
	return b
}

// Build creates the final TransactionOptions with resolved defaults.
func (b *TransactionOptionsBuilder) Build() spanner.TransactionOptions {
	// Resolve priority
	priority := b.priority
	if priority == sppb.RequestOptions_PRIORITY_UNSPECIFIED {
		priority = b.tm.sysVars.Query.RPCPriority
	}

	// Resolve isolation level
	isolationLevel := b.isolationLevel
	if isolationLevel == sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED {
		isolationLevel = b.tm.sysVars.Transaction.DefaultIsolationLevel
	}

	return spanner.TransactionOptions{
		CommitOptions:               spanner.CommitOptions{ReturnCommitStats: b.tm.sysVars.Transaction.ReturnCommitStats, MaxCommitDelay: b.tm.sysVars.Transaction.MaxCommitDelay},
		CommitPriority:              priority,
		TransactionTag:              b.tag,
		ExcludeTxnFromChangeStreams: b.tm.sysVars.Transaction.ExcludeTxnFromChangeStreams,
		IsolationLevel:              isolationLevel,
		ReadLockMode:                b.tm.sysVars.Transaction.ReadLockMode,
	}
}

// BuildPriority returns just the resolved priority.
func (b *TransactionOptionsBuilder) BuildPriority() sppb.RequestOptions_Priority {
	if b.priority == sppb.RequestOptions_PRIORITY_UNSPECIFIED {
		return b.tm.sysVars.Query.RPCPriority
	}
	return b.priority
}

// BuildIsolationLevel returns just the resolved isolation level.
func (b *TransactionOptionsBuilder) BuildIsolationLevel() sppb.TransactionOptions_IsolationLevel {
	if b.isolationLevel == sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED {
		return b.tm.sysVars.Transaction.DefaultIsolationLevel
	}
	return b.isolationLevel
}

// BeginPendingTransaction starts pending transaction.
// The actual start of the transaction is delayed until the first operation in the transaction is executed.
func (tm *TransactionManager) BeginPendingTransaction(ctx context.Context, isolationLevel sppb.TransactionOptions_IsolationLevel, priority sppb.RequestOptions_Priority) error {
	opts := tm.NewTransactionOptionsBuilder().
		WithIsolationLevel(isolationLevel).
		WithPriority(priority)
	resolvedIsolationLevel := opts.BuildIsolationLevel()
	resolvedPriority := opts.BuildPriority()

	return tm.TransitTransaction(ctx, func(tc *transactionContext) (*transactionContext, error) {
		// Check for any type of existing transaction (including pending)
		if tc != nil {
			return nil, fmt.Errorf("%s transaction is already running", tc.attrs.mode)
		}

		// Return new pending transaction context
		return &transactionContext{
			attrs: transactionAttributes{
				mode:           transactionModePending,
				priority:       resolvedPriority,
				isolationLevel: resolvedIsolationLevel,
			},
		}, nil
	})
}

// DetermineTransactionLocked determines the type of transaction to start based on the pending transaction
// and system variables. It returns the timestamp for read-only transactions or a zero time for read-write transactions.
// Caller must hold tm.mu.
func (tm *TransactionManager) DetermineTransactionLocked(ctx context.Context) (time.Time, error) {
	var zeroTime time.Time

	// Check if there's a pending transaction
	if tm.tc == nil || tm.tc.attrs.mode != transactionModePending {
		return zeroTime, nil
	}

	priority := tm.tc.attrs.priority
	isolationLevel := tm.tc.attrs.isolationLevel

	// Determine transaction type based on system variables
	if tm.sysVars.Transaction.ReadOnly {
		// Start a read-only transaction with the pending transaction's priority
		return tm.BeginReadOnlyTransactionLocked(ctx, timestampBoundUnspecified, 0, time.Time{}, priority)
	}

	// Start a read-write transaction with the pending transaction's isolation level and priority
	return zeroTime, tm.BeginReadWriteTransactionLocked(ctx, isolationLevel, priority)
}

// DetermineTransaction determines the type of transaction to start based on the pending transaction
// and system variables. It returns the timestamp for read-only transactions or a zero time for read-write transactions.
func (tm *TransactionManager) DetermineTransaction(ctx context.Context) (time.Time, error) {
	var result time.Time
	err := tm.withTransactionContextWithLock(func(tcPtr **transactionContext) error {
		ts, err := tm.DetermineTransactionLocked(ctx)
		result = ts
		return err
	})
	return result, err
}

// DetermineTransactionAndState determines transaction and returns both the timestamp and current transaction state.
// This combines DetermineTransaction and InTransaction checks in a single lock acquisition.
func (tm *TransactionManager) DetermineTransactionAndState(ctx context.Context) (time.Time, bool, error) {
	var result time.Time
	var inTransaction bool
	err := tm.withTransactionContextWithLock(func(tcPtr **transactionContext) error {
		ts, err := tm.DetermineTransactionLocked(ctx)
		result = ts
		if err != nil {
			return err
		}
		// Check if we're in a transaction after determining
		inTransaction = *tcPtr != nil
		return nil
	})
	return result, inTransaction, err
}

// getTransactionTagLocked returns the transaction tag from a pending transaction if it exists.
// Caller must hold tm.mu.
func (tm *TransactionManager) getTransactionTagLocked() string {
	if tm.tc != nil && tm.tc.attrs.mode == transactionModePending {
		return tm.tc.attrs.tag
	}
	return ""
}

// ValidateDatabaseOperation checks whether the TransactionManager has a valid client.
func (tm *TransactionManager) ValidateDatabaseOperation() error {
	if tm.client == nil {
		return errors.New("database operation requires a database connection")
	}
	return nil
}

// BeginReadWriteTransactionLocked starts read-write transaction.
// Caller must hold tm.mu.
func (tm *TransactionManager) BeginReadWriteTransactionLocked(ctx context.Context, isolationLevel sppb.TransactionOptions_IsolationLevel, priority sppb.RequestOptions_Priority) error {
	if err := tm.ValidateDatabaseOperation(); err != nil {
		return err
	}

	// Get transaction tag if there's a pending transaction
	tag := tm.getTransactionTagLocked()

	// Build transaction options using the builder
	builder := tm.NewTransactionOptionsBuilder().
		WithPriority(priority).
		WithIsolationLevel(isolationLevel).
		WithTag(tag)

	opts := builder.Build()
	resolvedPriority := builder.BuildPriority()
	resolvedIsolationLevel := builder.BuildIsolationLevel()

	// Check for existing transaction
	if tm.tc != nil && tm.tc.attrs.mode != transactionModePending {
		return fmt.Errorf("%s transaction is already running", tm.tc.attrs.mode)
	}

	oldTc := tm.tc

	// Create the new transaction
	txn, err := spanner.NewReadWriteStmtBasedTransactionWithOptions(ctx, tm.client, opts)
	if err != nil {
		return err
	}

	// Cleanup old transaction context if it's being replaced
	if oldTc != nil {
		oldTc.Close()
	}

	// Set new transaction context
	tm.tc = &transactionContext{
		attrs: transactionAttributes{
			mode:           transactionModeReadWrite,
			tag:            tag,
			priority:       resolvedPriority,
			isolationLevel: resolvedIsolationLevel,
		},
		txn:           txn,
		heartbeatFunc: tm.startHeartbeat,
	}

	// Heartbeat will be started by EnableHeartbeat() after the first operation.
	// For implicit transactions, they commit immediately so heartbeat isn't needed.
	return nil
}

// BeginReadWriteTransaction starts read-write transaction.
func (tm *TransactionManager) BeginReadWriteTransaction(ctx context.Context, isolationLevel sppb.TransactionOptions_IsolationLevel, priority sppb.RequestOptions_Priority) error {
	return tm.withTransactionContextWithLock(func(tcPtr **transactionContext) error {
		return tm.BeginReadWriteTransactionLocked(ctx, isolationLevel, priority)
	})
}

// CommitReadWriteTransactionLocked commits read-write transaction and returns commit timestamp if successful.
// Caller must hold tm.mu.
func (tm *TransactionManager) CommitReadWriteTransactionLocked(ctx context.Context) (spanner.CommitResponse, error) {
	if tm.tc == nil || tm.tc.txn == nil {
		return spanner.CommitResponse{}, ErrNotInReadWriteTransaction
	}

	rwTxn, ok := tm.tc.txn.(*spanner.ReadWriteStmtBasedTransaction)
	if !ok {
		return spanner.CommitResponse{}, ErrNotInReadWriteTransaction
	}

	resp, err := rwTxn.CommitWithReturnResp(ctx)

	// Always clear transaction context after commit attempt.
	// A failed commit invalidates the transaction on the server,
	// so we must clear the context regardless of the outcome.
	tm.tc.Close()
	tm.tc = nil

	// Return the response and error as-is, preserving any partial commit info
	return resp, err
}

// CommitReadWriteTransaction commits read-write transaction and returns commit timestamp if successful.
func (tm *TransactionManager) CommitReadWriteTransaction(ctx context.Context) (spanner.CommitResponse, error) {
	var resp spanner.CommitResponse
	err := tm.withTransactionContextWithLock(func(tcPtr **transactionContext) error {
		var commitErr error
		resp, commitErr = tm.CommitReadWriteTransactionLocked(ctx)
		return commitErr
	})
	return resp, err
}

// RollbackReadWriteTransactionLocked rollbacks read-write transaction.
// Caller must hold tm.mu.
func (tm *TransactionManager) RollbackReadWriteTransactionLocked(ctx context.Context) error {
	if tm.tc == nil || tm.tc.txn == nil {
		return ErrNotInReadWriteTransaction
	}

	rwTxn, ok := tm.tc.txn.(*spanner.ReadWriteStmtBasedTransaction)
	if !ok {
		return ErrNotInReadWriteTransaction
	}

	rwTxn.Rollback(ctx)

	// Clear transaction context after rollback
	tm.tc.Close()
	tm.tc = nil

	return nil
}

// RollbackReadWriteTransaction rollbacks read-write transaction.
func (tm *TransactionManager) RollbackReadWriteTransaction(ctx context.Context) error {
	return tm.withTransactionContextWithLock(func(tcPtr **transactionContext) error {
		return tm.RollbackReadWriteTransactionLocked(ctx)
	})
}

// resolveTimestampBound returns the effective timestamp bound for a read-only transaction.
func (tm *TransactionManager) resolveTimestampBound(typ timestampBoundType, staleness time.Duration, timestamp time.Time) spanner.TimestampBound {
	tb := spanner.StrongRead()
	switch typ {
	case strong:
		tb = spanner.StrongRead()
	case exactStaleness:
		tb = spanner.ExactStaleness(staleness)
	case readTimestamp:
		tb = spanner.ReadTimestamp(timestamp)
	default:
		if tm.sysVars.Query.ReadOnlyStaleness != nil {
			tb = *tm.sysVars.Query.ReadOnlyStaleness
		}
	}
	return tb
}

// BeginReadOnlyTransactionLocked starts read-only transaction and returns the snapshot timestamp for the transaction if successful.
// Caller must hold tm.mu.
func (tm *TransactionManager) BeginReadOnlyTransactionLocked(ctx context.Context, typ timestampBoundType, staleness time.Duration, timestamp time.Time, priority sppb.RequestOptions_Priority) (time.Time, error) {
	if err := tm.ValidateDatabaseOperation(); err != nil {
		return time.Time{}, err
	}

	tb := tm.resolveTimestampBound(typ, staleness, timestamp)
	resolvedPriority := tm.NewTransactionOptionsBuilder().WithPriority(priority).BuildPriority()

	// Check for existing transaction
	if tm.tc != nil && tm.tc.attrs.mode != transactionModePending {
		return time.Time{}, fmt.Errorf("%s transaction is already running", tm.tc.attrs.mode)
	}

	oldTc := tm.tc
	txn := tm.client.ReadOnlyTransaction().WithTimestampBound(tb)

	// Because google-cloud-go/spanner defers calling BeginTransaction RPC until an actual query is run,
	// we explicitly run a "SELECT 1" query so that we can determine the timestamp of read-only transaction.
	opts := spanner.QueryOptions{Priority: resolvedPriority}
	if _, _, _, _, err := consumeRowIterDiscard(txn.QueryWithOptions(ctx, spanner.NewStatement("SELECT 1"), opts)); err != nil {
		txn.Close()
		return time.Time{}, err
	}

	// Get timestamp first to ensure the transaction is valid
	resultTimestamp, err := txn.Timestamp()
	if err != nil {
		txn.Close()
		return time.Time{}, err
	}

	// Cleanup old transaction context if it's being replaced
	if oldTc != nil {
		oldTc.Close()
	}

	// Set new transaction context
	tm.tc = &transactionContext{
		attrs: transactionAttributes{
			mode:     transactionModeReadOnly,
			priority: resolvedPriority,
		},
		txn: txn,
	}

	return resultTimestamp, nil
}

// BeginReadOnlyTransaction starts read-only transaction and returns the snapshot timestamp for the transaction if successful.
func (tm *TransactionManager) BeginReadOnlyTransaction(ctx context.Context, typ timestampBoundType, staleness time.Duration, timestamp time.Time, priority sppb.RequestOptions_Priority) (time.Time, error) {
	var resultTimestamp time.Time
	err := tm.withTransactionContextWithLock(func(tcPtr **transactionContext) error {
		ts, err := tm.BeginReadOnlyTransactionLocked(ctx, typ, staleness, timestamp, priority)
		resultTimestamp = ts
		return err
	})
	return resultTimestamp, err
}

// closeTransactionWithMode closes a transaction of the specified mode.
// It validates the transaction mode and performs appropriate cleanup.
// The closeFn receives the transaction object to avoid direct tc access.
// closeFn can be nil for transaction types that don't require specific cleanup (e.g., pending transactions).
func (tm *TransactionManager) closeTransactionWithMode(mode transactionMode, closeFn func(txn transaction) error) error {
	return tm.withTransactionContextWithLock(func(tcPtr **transactionContext) error {
		if *tcPtr == nil || (*tcPtr).attrs.mode != mode {
			switch mode {
			case transactionModeReadWrite:
				return ErrNotInReadWriteTransaction
			case transactionModeReadOnly:
				return ErrNotInReadOnlyTransaction
			case transactionModePending:
				return ErrNotInPendingTransaction
			default:
				return fmt.Errorf("not in %s transaction", mode)
			}
		}

		// Execute the close function if provided
		if closeFn != nil && (*tcPtr).txn != nil {
			if err := closeFn((*tcPtr).txn); err != nil {
				return err
			}
		}

		// Close and clear the transaction context
		(*tcPtr).Close()
		*tcPtr = nil
		return nil
	})
}

// CloseReadOnlyTransaction closes a running read-only transaction.
func (tm *TransactionManager) CloseReadOnlyTransaction() error {
	return tm.closeTransactionWithMode(transactionModeReadOnly, func(txn transaction) error {
		// The transaction is passed as a parameter, maintaining encapsulation
		roTxn, ok := txn.(*spanner.ReadOnlyTransaction)
		if !ok {
			// This should never happen given the mode check in closeTransactionWithMode,
			// but we handle it explicitly to prevent resource leaks
			return fmt.Errorf("internal error: transaction object has incorrect type for read-only mode (got %T)", txn)
		}
		roTxn.Close()
		return nil
	})
}

// ClosePendingTransaction closes a pending transaction.
func (tm *TransactionManager) ClosePendingTransaction() error {
	// Pending transactions don't have an underlying transaction to close
	return tm.closeTransactionWithMode(transactionModePending, nil)
}

// runQueryWithStatsOnTransaction executes a query on the given transaction with statistics
// This is a helper function to be used within transaction closures to avoid direct tc access
// NOTE: This method is called from within transaction callbacks where mu is already held.
func (tm *TransactionManager) runQueryWithStatsOnTransaction(ctx context.Context, tx transaction, stmt spanner.Statement, implicit bool) *spanner.RowIterator {
	opts := tm.queryOptionsLocked(sppb.ExecuteSqlRequest_PROFILE.Enum())
	opts.LastStatement = implicit
	return tx.QueryWithOptions(ctx, stmt, opts)
}

// runAnalyzeQueryOnTransaction executes an analyze query on the given transaction
// This is a helper function to be used within transaction closures to avoid direct tc access
// NOTE: This method is called from within transaction callbacks where mu is already held.
func (tm *TransactionManager) runAnalyzeQueryOnTransaction(ctx context.Context, tx transaction, stmt spanner.Statement) (*sppb.QueryPlan, *sppb.ResultSetMetadata, error) {
	mode := sppb.ExecuteSqlRequest_PLAN
	opts := spanner.QueryOptions{
		Mode:     &mode,
		Priority: tm.currentPriorityLocked(),
	}
	if opts.Options == nil {
		opts.Options = &sppb.ExecuteSqlRequest_QueryOptions{}
	}
	iter := tx.QueryWithOptions(ctx, stmt, opts)
	_, _, metadata, plan, err := consumeRowIterDiscard(iter)
	return plan, metadata, err
}

// runUpdateOnTransaction executes an update statement on a transaction.
// NOTE: This method is always called from within withReadWriteTransactionContext,
// so the mu is already held. We must use the locked versions of methods
// to avoid deadlock.
//
// CRITICAL: This method is part of a callback chain where mu is already held:
//
//	RunInNewOrExistRwTx -> withReadWriteTransactionContext -> withTransactionLocked -> callback -> runUpdateOnTransaction
//
// Using non-locked versions of methods like TransactionAttrsWithLock(), currentPriorityWithLock(),
// or queryOptionsWithLock() here will cause a deadlock. Always use the *Locked variants.
func (tm *TransactionManager) runUpdateOnTransaction(ctx context.Context, tx *spanner.ReadWriteStmtBasedTransaction, stmt spanner.Statement, implicit bool) (*UpdateResult, error) {
	fc, err := decoder.FormatConfigWithProto(tm.sysVars.Internal.ProtoDescriptor, tm.sysVars.Display.MultilineProtoText)
	if err != nil {
		return nil, err
	}

	opts := tm.queryOptionsLocked(sppb.ExecuteSqlRequest_PROFILE.Enum())
	opts.LastStatement = implicit

	// Reset STATEMENT_TAG
	tm.sysVars.Transaction.RequestTag = ""

	rows, stats, count, metadata, plan, err := consumeRowIterCollect(
		tx.QueryWithOptions(ctx, stmt, opts),
		spannerRowToRow(fc, tm.sysVars.typeStyles, tm.sysVars.nullStyle),
	)
	if err != nil {
		return nil, err
	}

	return &UpdateResult{
		Rows:     rows,
		Stats:    stats,
		Count:    count,
		Metadata: metadata,
		Plan:     plan,
	}, nil
}

// RunQueryWithStats executes a statement with stats either on the running transaction or on the temporal read-only transaction.
// It returns row iterator and read-only transaction if the statement was executed on the read-only transaction.
func (tm *TransactionManager) RunQueryWithStats(ctx context.Context, stmt spanner.Statement, implicit bool) (*spanner.RowIterator, *spanner.ReadOnlyTransaction) {
	// Validate that we have a database client for query operations
	if err := tm.ValidateDatabaseOperation(); err != nil {
		// This should not happen if DetachedCompatible interface validation is working correctly
		// Log the error for debugging since we can't return it directly
		slog.Error("RunQueryWithStats called without database connection", "error", err, "statement", stmt.SQL)
		// Return nil to indicate error - caller should check for nil
		return nil, nil
	}

	mode := sppb.ExecuteSqlRequest_PROFILE
	opts := tm.buildQueryOptions(&mode)
	opts.LastStatement = implicit
	return tm.runQueryWithOptions(ctx, stmt, opts)
}

// RunQuery executes a statement either on the running transaction or on the temporal read-only transaction.
// It returns row iterator and read-only transaction if the statement was executed on the read-only transaction.
func (tm *TransactionManager) RunQuery(ctx context.Context, stmt spanner.Statement) (*spanner.RowIterator, *spanner.ReadOnlyTransaction) {
	// Validate that we have a database client for query operations
	if err := tm.ValidateDatabaseOperation(); err != nil {
		// This should not happen if DetachedCompatible interface validation is working correctly
		// Log the error for debugging since we can't return it directly
		slog.Error("RunQuery called without database connection", "error", err, "statement", stmt.SQL)
		// Return nil to indicate error - caller should check for nil
		return nil, nil
	}

	opts := tm.buildQueryOptions(nil)
	return tm.runQueryWithOptions(ctx, stmt, opts)
}

// RunAnalyzeQuery analyzes a statement either on the running transaction or on the temporal read-only transaction.
func (tm *TransactionManager) RunAnalyzeQuery(ctx context.Context, stmt spanner.Statement) (*sppb.QueryPlan, *sppb.ResultSetMetadata, error) {
	_, err := tm.DetermineTransaction(ctx)
	if err != nil {
		return nil, nil, err
	}

	mode := sppb.ExecuteSqlRequest_PLAN
	opts := spanner.QueryOptions{
		Mode:     &mode,
		Priority: tm.currentPriorityWithLock(),
	}
	iter, _ := tm.runQueryWithOptions(ctx, stmt, opts)

	_, _, metadata, plan, err := consumeRowIterDiscard(iter)
	return plan, metadata, err
}

func (tm *TransactionManager) runQueryWithOptions(ctx context.Context, stmt spanner.Statement, opts spanner.QueryOptions) (*spanner.RowIterator, *spanner.ReadOnlyTransaction) {
	// Prepare query options
	tm.prepareQueryOptions(&opts)

	// Try to execute in existing transaction first
	if iter, txn := tm.tryQueryInTransaction(ctx, stmt, opts); iter != nil {
		return iter, txn
	}

	// Fall back to single-use transaction
	return tm.runSingleUseQuery(ctx, stmt, opts)
}

// prepareQueryOptions sets up the query options with system variables
func (tm *TransactionManager) prepareQueryOptions(opts *spanner.QueryOptions) {
	if opts.Options == nil {
		opts.Options = &sppb.ExecuteSqlRequest_QueryOptions{}
	}

	opts.Options.OptimizerVersion = tm.sysVars.Query.OptimizerVersion
	opts.Options.OptimizerStatisticsPackage = tm.sysVars.Query.OptimizerStatisticsPackage
	opts.RequestTag = tm.sysVars.Transaction.RequestTag

	// Reset STATEMENT_TAG
	tm.sysVars.Transaction.RequestTag = ""
}

// tryQueryInTransaction attempts to execute a query within an existing transaction
// Returns (nil, nil) if no active transaction
func (tm *TransactionManager) tryQueryInTransaction(ctx context.Context, stmt spanner.Statement, opts spanner.QueryOptions) (*spanner.RowIterator, *spanner.ReadOnlyTransaction) {
	// Hold the lock for the entire operation to ensure atomicity.
	// Performance impact: The single-lock approach adds minimal overhead for a CLI tool
	// where queries are user-initiated and not high-frequency. The correctness guarantee
	// (preventing transaction context changes mid-operation) outweighs the negligible
	// performance cost.
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Check if we're in an active transaction
	if tm.tc == nil || (tm.tc.attrs.mode != transactionModeReadWrite && tm.tc.attrs.mode != transactionModeReadOnly) {
		return nil, nil
	}

	// Validate transaction state
	if tm.tc.txn == nil {
		// This shouldn't happen - log the error and return a stopped iterator
		mode := tm.tc.attrs.mode
		err := fmt.Errorf("internal error: %s transaction context exists but txn is nil", mode)
		slog.Error("INTERNAL ERROR", "error", err)
		return newStoppedIterator(), nil
	}

	// Apply read-write specific settings
	if tm.tc.attrs.mode == transactionModeReadWrite {
		// The current Go Spanner client library does not apply client-level directed read options to read-write transactions.
		// Therefore, we explicitly set query-level options here to fail the query during a read-write transaction.
		opts.DirectedReadOptions = tm.clientConfig.DirectedReadOptions
		tm.tc.EnableHeartbeat()
	}

	// Execute query on the transaction
	iter := tm.tc.txn.QueryWithOptions(ctx, stmt, opts)

	// For read-only transactions, return the transaction for timestamp access
	if tm.tc.attrs.mode == transactionModeReadOnly {
		roTxn, ok := tm.tc.txn.(*spanner.ReadOnlyTransaction)
		if !ok {
			// This shouldn't happen - log the error and return a stopped iterator
			err := fmt.Errorf("internal error: transaction is not a ReadOnlyTransaction (got %T)", tm.tc.txn)
			slog.Error("INTERNAL ERROR", "error", err)
			return newStoppedIterator(), nil
		}
		return iter, roTxn
	}

	// For read-write transactions, return nil as the second value
	return iter, nil
}

// runSingleUseQuery executes a query in a single-use read-only transaction
func (tm *TransactionManager) runSingleUseQuery(ctx context.Context, stmt spanner.Statement, opts spanner.QueryOptions) (*spanner.RowIterator, *spanner.ReadOnlyTransaction) {
	if tm.client == nil {
		// This is a programming error - log the error and return a stopped iterator
		err := fmt.Errorf("internal error: runQueryWithOptions called with nil client despite validations (statement: %v)",
			stmt.SQL)
		slog.Error("INTERNAL ERROR", "error", err)
		return newStoppedIterator(), nil
	}

	txn := tm.client.Single()
	if tm.sysVars.Query.ReadOnlyStaleness != nil {
		txn = txn.WithTimestampBound(*tm.sysVars.Query.ReadOnlyStaleness)
	}
	return txn.QueryWithOptions(ctx, stmt, opts), txn
}

// RunUpdate executes a DML statement on the running read-write transaction.
// It returns error if there is no running read-write transaction.
func (tm *TransactionManager) RunUpdate(ctx context.Context, stmt spanner.Statement, implicit bool) ([]Row, map[string]any, int64,
	*sppb.ResultSetMetadata, *sppb.QueryPlan, error,
) {
	fc, err := decoder.FormatConfigWithProto(tm.sysVars.Internal.ProtoDescriptor, tm.sysVars.Display.MultilineProtoText)
	if err != nil {
		return nil, nil, 0, nil, nil, err
	}

	opts := tm.queryOptionsWithLock(sppb.ExecuteSqlRequest_PROFILE.Enum())
	opts.LastStatement = implicit

	// Reset STATEMENT_TAG
	tm.sysVars.Transaction.RequestTag = ""

	var rows []Row
	var stats map[string]any
	var count int64
	var metadata *sppb.ResultSetMetadata
	var plan *sppb.QueryPlan

	err = tm.withReadWriteTransactionContext(func(txn *spanner.ReadWriteStmtBasedTransaction, tc *transactionContext) error {
		var err error
		rows, stats, count, metadata, plan, err = consumeRowIterCollect(
			txn.QueryWithOptions(ctx, stmt, opts),
			spannerRowToRow(fc, tm.sysVars.typeStyles, tm.sysVars.nullStyle),
		)
		// Enable heartbeat after any operation (success or failure)
		// Even failed operations start the abort countdown
		tc.EnableHeartbeat()
		return err
	})

	if err == ErrNotInReadWriteTransaction {
		return nil, nil, 0, nil, nil, ErrNotInReadWriteTransaction
	}
	if err != nil {
		return nil, nil, 0, nil, nil, err
	}

	return rows, stats, count, metadata, plan, nil
}

func (tm *TransactionManager) queryOptionsWithLock(mode *sppb.ExecuteSqlRequest_QueryMode) spanner.QueryOptions {
	return spanner.QueryOptions{
		Mode:       mode,
		Priority:   tm.currentPriorityWithLock(),
		RequestTag: tm.sysVars.Transaction.RequestTag,
		Options: &sppb.ExecuteSqlRequest_QueryOptions{
			OptimizerVersion:           tm.sysVars.Query.OptimizerVersion,
			OptimizerStatisticsPackage: tm.sysVars.Query.OptimizerStatisticsPackage,
		},
	}
}

// queryOptionsLocked returns query options without locking.
// Caller must hold tm.mu.
//
// This method exists to prevent deadlocks when accessing query options within
// callbacks that are invoked while mu is already held.
// See transactionAttrsLocked for detailed explanation of the deadlock scenario.
func (tm *TransactionManager) queryOptionsLocked(mode *sppb.ExecuteSqlRequest_QueryMode) spanner.QueryOptions {
	return spanner.QueryOptions{
		Mode:       mode,
		Priority:   tm.currentPriorityLocked(),
		RequestTag: tm.sysVars.Transaction.RequestTag,
		Options: &sppb.ExecuteSqlRequest_QueryOptions{
			OptimizerVersion:           tm.sysVars.Query.OptimizerVersion,
			OptimizerStatisticsPackage: tm.sysVars.Query.OptimizerStatisticsPackage,
		},
	}
}

func (tm *TransactionManager) currentPriorityWithLock() sppb.RequestOptions_Priority {
	attrs := tm.TransactionAttrsWithLock()
	if attrs.mode != transactionModeUndetermined && attrs.mode != "" {
		return attrs.priority
	}
	return tm.sysVars.Query.RPCPriority
}

// currentPriorityLocked returns the current priority without locking.
// Caller must hold tm.mu.
//
// This method exists to prevent deadlocks when accessing priority within
// callbacks that are invoked while mu is already held.
// See transactionAttrsLocked for detailed explanation of the deadlock scenario.
func (tm *TransactionManager) currentPriorityLocked() sppb.RequestOptions_Priority {
	attrs := tm.transactionAttrsLocked()
	if attrs.mode != transactionModeUndetermined && attrs.mode != "" {
		return attrs.priority
	}
	return tm.sysVars.Query.RPCPriority
}

func (tm *TransactionManager) buildQueryOptions(mode *sppb.ExecuteSqlRequest_QueryMode) spanner.QueryOptions {
	opts := spanner.QueryOptions{
		Mode:       mode,
		Priority:   tm.currentPriorityWithLock(),
		RequestTag: tm.sysVars.Transaction.RequestTag,
		Options: &sppb.ExecuteSqlRequest_QueryOptions{
			OptimizerVersion:           tm.sysVars.Query.OptimizerVersion,
			OptimizerStatisticsPackage: tm.sysVars.Query.OptimizerStatisticsPackage,
		},
	}
	return opts
}

// startHeartbeat starts heartbeat for read-write transaction.
//
// If no reads or DMLs happen within 10 seconds, the rw-transaction is considered idle at Cloud Spanner server.
// This "SELECT 1" query prevents the transaction from being considered idle.
// cf. https://godoc.org/cloud.google.com/go/spanner#hdr-Idle_transactions
//
// We send an actual heartbeat only if the read-write transaction is active and
// at least one user-initialized SQL query has been executed on the transaction.
// Background: https://github.com/cloudspannerecosystem/spanner-cli/issues/100
func (tm *TransactionManager) startHeartbeat(ctx context.Context) {
	interval := time.NewTicker(5 * time.Second)
	defer interval.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-interval.C:
			// Get transaction attributes safely without holding mutex to avoid contention.
			// This is critical for performance: the heartbeat runs every 5 seconds, and
			// acquiring the mutex each time was causing test timeouts in parallel tests.
			attrs := tm.TransactionAttrsWithLock()

			// Check for invalid states and exit if detected
			if attrs.mode == transactionModeUndetermined || attrs.mode == "" {
				slog.Debug("heartbeat: no active transaction, exiting goroutine")
				return
			}

			// Only send heartbeat if we have an active read-write transaction with heartbeat enabled
			if attrs.mode == transactionModeReadWrite && attrs.sendHeartbeat {
				// Use withReadWriteTransaction to safely access the transaction
				err := tm.withReadWriteTransaction(func(txn *spanner.ReadWriteStmtBasedTransaction) error {
					// Always use LOW priority for heartbeat to avoid interfering with real work
					err := heartbeat(txn, sppb.RequestOptions_PRIORITY_LOW)
					if err != nil {
						slog.Error("heartbeat error", "err", err)
					}
					return nil
				})

				// If we couldn't access the transaction, it might have been cleared
				if err == ErrNotInReadWriteTransaction {
					slog.Debug("heartbeat: transaction no longer active, exiting goroutine")
					return
				}
			}
		}
	}
}

// heartbeat sends a keepalive query on a read-write transaction.
func heartbeat(txn *spanner.ReadWriteStmtBasedTransaction, priority sppb.RequestOptions_Priority) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	iter := txn.QueryWithOptions(ctx, spanner.NewStatement("SELECT 1"), spanner.QueryOptions{
		Priority:   priority,
		RequestTag: "spanner_mycli_heartbeat",
	})
	defer iter.Stop()
	_, err := iter.Next()
	return err
}

// RunInNewOrExistRwTxLocked is a helper function for DML execution.
// It executes a function in the current RW transaction or an implicit RW transaction.
// If there is an error, the transaction will be rolled back.
// Caller must hold tm.mu.
func (tm *TransactionManager) RunInNewOrExistRwTxLocked(ctx context.Context,
	f func(tx *spanner.ReadWriteStmtBasedTransaction, implicit bool) (affected int64, plan *sppb.QueryPlan, metadata *sppb.ResultSetMetadata, err error),
) (*DMLResult, error) {
	// Determine transaction type (this may start a transaction)
	_, err := tm.DetermineTransactionLocked(ctx)
	if err != nil {
		return nil, err
	}

	// Check transaction state (no lock needed, we already hold it)
	attrs := tm.transactionAttrsLocked()
	mode := attrs.mode
	isActive := mode != transactionModeUndetermined && mode != ""
	var implicitRWTx bool

	if !isActive || mode != transactionModeReadWrite {
		// Start implicit transaction
		// Note: isolation level is not session level property so it is left as unspecified
		priority := tm.currentPriorityLocked()
		if err := tm.BeginReadWriteTransactionLocked(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, priority); err != nil {
			return nil, err
		}
		implicitRWTx = true
	}

	// Now we have a read-write transaction
	if tm.tc == nil || tm.tc.txn == nil {
		return nil, ErrNotInReadWriteTransaction
	}

	txn, ok := tm.tc.txn.(*spanner.ReadWriteStmtBasedTransaction)
	if !ok {
		return nil, ErrNotInReadWriteTransaction
	}

	var affected int64
	var plan *sppb.QueryPlan
	var metadata *sppb.ResultSetMetadata

	// Execute the function
	affected, plan, metadata, err = f(txn, implicitRWTx)

	// Enable heartbeat after any operation (success or failure)
	// Even failed operations start the abort countdown
	// BUT: Don't enable heartbeat for implicit transactions since they commit immediately
	if !implicitRWTx && tm.tc != nil {
		tm.tc.EnableHeartbeat()
	}

	if err != nil {
		// Rollback the transaction while holding the lock
		if rollbackErr := tm.RollbackReadWriteTransactionLocked(ctx); rollbackErr != nil {
			err = errors.Join(err, fmt.Errorf("error on rollback: %w", rollbackErr))
		}
		return nil, fmt.Errorf("transaction was aborted: %w", err)
	}

	result := &DMLResult{
		Affected: affected,
		Plan:     plan,
		Metadata: metadata,
	}

	if !implicitRWTx {
		return result, nil
	}

	// For implicit transactions, commit immediately while holding the lock
	resp, err := tm.CommitReadWriteTransactionLocked(ctx)
	if err != nil {
		return nil, err
	}

	result.CommitResponse = resp
	return result, nil
}

// RunInNewOrExistRwTx is a helper function for DML execution.
// It executes a function in the current RW transaction or an implicit RW transaction.
// If there is an error, the transaction will be rolled back.
func (tm *TransactionManager) RunInNewOrExistRwTx(ctx context.Context,
	f func(tx *spanner.ReadWriteStmtBasedTransaction, implicit bool) (affected int64, plan *sppb.QueryPlan, metadata *sppb.ResultSetMetadata, err error),
) (*DMLResult, error) {
	// Use a single lock acquisition for the entire operation
	tm.mu.Lock()
	defer tm.mu.Unlock()

	return tm.RunInNewOrExistRwTxLocked(ctx, f)
}

// RunPartitionQuery runs a partition query.
func (tm *TransactionManager) RunPartitionQuery(ctx context.Context, stmt spanner.Statement) ([]*spanner.Partition, *spanner.BatchReadOnlyTransaction, error) {
	tb := lo.FromPtrOr(tm.sysVars.Query.ReadOnlyStaleness, spanner.StrongRead())

	batchROTx, err := tm.client.BatchReadOnlyTransaction(ctx, tb)
	if err != nil {
		return nil, nil, err
	}

	partitions, err := batchROTx.PartitionQueryWithOptions(ctx, stmt, spanner.PartitionOptions{}, spanner.QueryOptions{
		DataBoostEnabled: tm.sysVars.Query.DataBoostEnabled,
		Priority:         tm.sysVars.Query.RPCPriority,
	})
	if err != nil {
		batchROTx.Cleanup(ctx)
		batchROTx.Close()
		return nil, nil, fmt.Errorf("query can't be a partition query: %w", err)
	}
	return partitions, batchROTx, nil
}
