package main

import (
	"context"
	"fmt"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
)

type transactionMode string

const (
	transactionModeUndetermined = ""
	transactionModePending      = "pending"
	transactionModeReadOnly     = "read-only"
	transactionModeReadWrite    = "read-write"
)

// transactionAttributes holds metadata about a transaction
type transactionAttributes struct {
	mode           transactionMode
	tag            string
	priority       sppb.RequestOptions_Priority
	isolationLevel sppb.TransactionOptions_IsolationLevel
	sendHeartbeat  bool
}

// transaction is a common interface for read-write and read-only transactions
type transaction interface {
	QueryWithOptions(ctx context.Context, statement spanner.Statement, opts spanner.QueryOptions) *spanner.RowIterator
	Query(ctx context.Context, statement spanner.Statement) *spanner.RowIterator
}

// transactionContext encapsulates the transaction state and attributes.
// It provides safe access to the underlying transaction while maintaining
// metadata about the transaction's mode and properties.
type transactionContext struct {
	attrs           transactionAttributes
	txn             transaction
	heartbeatCancel context.CancelFunc
	heartbeatFunc   func(ctx context.Context) // Function to run heartbeat
}

// RWTxn returns the transaction as a ReadWriteStmtBasedTransaction.
// Returns an error if the transaction is not in read-write mode.
func (tc *transactionContext) RWTxn() (*spanner.ReadWriteStmtBasedTransaction, error) {
	if tc == nil || tc.txn == nil {
		return nil, ErrTransactionNotAvailable
	}
	if tc.attrs.mode != transactionModeReadWrite {
		return nil, fmt.Errorf("%w: current mode is %v", ErrNotInReadWriteTransaction, tc.attrs.mode)
	}
	return tc.txn.(*spanner.ReadWriteStmtBasedTransaction), nil
}

// ROTxn returns the transaction as a ReadOnlyTransaction.
// Returns an error if the transaction is not in read-only mode.
func (tc *transactionContext) ROTxn() (*spanner.ReadOnlyTransaction, error) {
	if tc == nil || tc.txn == nil {
		return nil, ErrTransactionNotAvailable
	}
	if tc.attrs.mode != transactionModeReadOnly {
		return nil, fmt.Errorf("%w: current mode is %v", ErrNotInReadOnlyTransaction, tc.attrs.mode)
	}
	return tc.txn.(*spanner.ReadOnlyTransaction), nil
}

// Txn returns the transaction interface.
// Returns an error if not in a valid transaction state.
func (tc *transactionContext) Txn() (transaction, error) {
	if tc == nil || tc.txn == nil {
		return nil, ErrTransactionNotAvailable
	}
	if tc.attrs.mode != transactionModeReadOnly && tc.attrs.mode != transactionModeReadWrite {
		return nil, fmt.Errorf("%w: current mode is %v", ErrInvalidTransactionMode, tc.attrs.mode)
	}
	return tc.txn, nil
}

// EnableHeartbeat enables sending periodic heartbeats for this transaction.
// This method provides encapsulation for the sendHeartbeat field.
func (tc *transactionContext) EnableHeartbeat() {
	if tc != nil && tc.attrs.mode == transactionModeReadWrite {
		tc.attrs.sendHeartbeat = true
		// Start heartbeat goroutine if not already started
		if tc.heartbeatCancel == nil && tc.heartbeatFunc != nil {
			ctx, cancel := context.WithCancel(context.Background())
			tc.heartbeatCancel = cancel
			// Debug: Log when heartbeat is started
			// fmt.Println("DEBUG: Starting heartbeat goroutine")
			go tc.heartbeatFunc(ctx)
		}
	}
}

// IsHeartbeatEnabled returns whether heartbeats are enabled for this transaction.
// This method provides encapsulation for the sendHeartbeat field.
func (tc *transactionContext) IsHeartbeatEnabled() bool {
	if tc == nil {
		return false
	}
	return tc.attrs.sendHeartbeat
}

// SetTag sets the transaction tag.
// This method provides encapsulation for the tag field.
func (tc *transactionContext) SetTag(tag string) {
	if tc != nil {
		tc.attrs.tag = tag
	}
}

// Tag returns the transaction tag.
// This method provides encapsulation for the tag field.
func (tc *transactionContext) Tag() string {
	if tc == nil {
		return ""
	}
	return tc.attrs.tag
}

// Close stops the heartbeat goroutine if it's running.
// This should be called when the transaction is committed or rolled back.
func (tc *transactionContext) Close() {
	if tc != nil && tc.heartbeatCancel != nil {
		tc.heartbeatCancel()
		tc.heartbeatCancel = nil
	}
}
