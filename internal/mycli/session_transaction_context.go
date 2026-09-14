package mycli

import (
	"context"
	"time"

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

// transactionContext is the logical transaction lifetime. It is allocated on
// BEGIN (pending or direct RO/RW), kept as the same pointer through pending
// activation, and retired only on a terminal path. Heartbeat, tag, and SET
// LOCAL undo all live on this object so they cannot be split across replacement
// owners.
type transactionContext struct {
	attrs           transactionAttributes
	txn             transaction
	heartbeatCancel context.CancelFunc
	heartbeatFunc   func(ctx context.Context, startedAttempt uint64) // Function to run heartbeat
	// localVarUndo is this transaction's SET LOCAL undo log. It survives
	// pending activation and is detached into
	// TransactionManager.pendingLocalVarRestore only when the context is
	// retired.
	localVarUndo []savedLocalVar
	// autoDML is this transaction's automatic DML queue. Access it only under
	// tm.mu. Manual START/RUN/ABORT BATCH state stays on Session.batch.
	autoDML []spanner.Statement
	// replay is the optional SAVEPOINT journal. Nil unless capture was
	// enabled at explicit BEGIN via CLI_SAVEPOINT_SUPPORT=ENABLED or the
	// private test hook.
	replay *replayState
	// ctorOpts is the frozen NewReadWriteStmtBasedTransactionWithOptions input.
	ctorOpts spanner.TransactionOptions
	// keepAliveDisabled is the inverted KEEP_TRANSACTION_ALIVE snapshot captured
	// with ctorOpts. The zero value means enabled so ad-hoc test owners keep the
	// existing heartbeat-after-first-SQL behavior. BeginReadWriteTransactionLocked
	// sets it from the session variable before any user SQL.
	keepAliveDisabled bool
	// timeout is the TRANSACTION_TIMEOUT duration captured for this logical
	// owner. Zero means no additional deadline. timeoutCaptured distinguishes
	// an explicit zero from an uninitialized ad-hoc test owner.
	timeout         time.Duration
	timeoutCaptured bool
	deadline        time.Time
	deadlineCtx     context.Context
	deadlineCancel  context.CancelFunc
	attempt         uint64
	inFlight        int
	pending         *captureToken
	// replacing is true while ROLLBACK TO is replacing the physical RW handle.
	replacing bool
}

// EnableHeartbeat enables sending periodic heartbeats for this transaction.
// A frozen KEEP_TRANSACTION_ALIVE=FALSE owner does not schedule a goroutine
// and does not mark sendHeartbeat, so SAVEPOINT reconstruction cannot revive
// keepalive for that owner.
func (tc *transactionContext) EnableHeartbeat() {
	if tc == nil || tc.attrs.mode != transactionModeReadWrite || tc.keepAliveDisabled {
		return
	}
	tc.attrs.sendHeartbeat = true
	// Start heartbeat goroutine if not already started
	if tc.heartbeatCancel == nil && tc.heartbeatFunc != nil {
		startedAttempt := tc.attempt
		ctx, cancel := context.WithCancel(context.Background())
		tc.heartbeatCancel = cancel
		go tc.heartbeatFunc(ctx, startedAttempt)
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

// Close stops the heartbeat goroutine and the transaction-deadline watcher
// if they are running. This should be called when the transaction is
// committed, rolled back, or expired.
func (tc *transactionContext) Close() {
	if tc == nil {
		return
	}
	if tc.heartbeatCancel != nil {
		tc.heartbeatCancel()
		tc.heartbeatCancel = nil
	}
	if tc.deadlineCancel != nil {
		tc.deadlineCancel()
		tc.deadlineCancel = nil
	}
}
