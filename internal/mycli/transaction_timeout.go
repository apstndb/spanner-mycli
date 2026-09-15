// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mycli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TRANSACTION_TIMEOUT is a logical read/write budget, distinct from
// STATEMENT_TIMEOUT and from CLI_IDLE_TRANSACTION_TIMEOUT (#357).
// Zero or NULL means no additional transaction deadline. Implicit and
// explicit ABORTED retry (#994 / #1006) reuse this remaining budget and
// never restart it.
var (
	errTransactionTimeout       = errors.New("TRANSACTION_TIMEOUT exceeded")
	errTransactionTimeoutFrozen = errors.New("TRANSACTION_TIMEOUT cannot be changed after the transaction deadline has started")
)

func transactionTimeoutDuration(vars *systemVariables) time.Duration {
	if vars == nil || vars.Transaction.TransactionTimeout == nil {
		return 0
	}
	return *vars.Transaction.TransactionTimeout
}

func (tm *TransactionManager) now() time.Time {
	if tm != nil && tm.nowFunc != nil {
		return tm.nowFunc()
	}
	return time.Now()
}

// snapshotTransactionTimeoutLocked stores the session duration on a newly
// created logical owner. Pending BEGIN captures it before any RPC; later
// session SET does not overwrite this field.
func snapshotTransactionTimeoutLocked(tc *transactionContext, vars *systemVariables) {
	if tc == nil || tc.timeoutCaptured {
		return
	}
	tc.timeout = transactionTimeoutDuration(vars)
	tc.timeoutCaptured = true
}

// applyLocalTransactionTimeout updates the logical owner's captured duration
// when SET LOCAL happens before the first database RPC. After first real
// database use, only an identical value is accepted, including when the
// selected duration is NULL/0 and no timer exists.
func (tm *TransactionManager) applyLocalTransactionTimeout(d time.Duration) error {
	return tm.withTransactionContextWithLock(func(tcPtr **transactionContext) error {
		if *tcPtr == nil {
			return ErrNoTransaction
		}
		owner := *tcPtr
		if owner.firstUse && owner.timeout != d {
			return errTransactionTimeoutFrozen
		}
		owner.timeout = d
		owner.timeoutCaptured = true
		return nil
	})
}

// armTransactionDeadlineLocked records first real read/write database use
// and starts the single total budget when the captured duration is
// positive. Caller must hold tm.mu. Pending BEGIN, SHOW, multiplexed
// session take, and DML buffering do not call this. Read-only owners
// never mark first use. Re-entry is a no-op so SAVEPOINT reconstruction
// and later statements keep the original first-use / deadline.
func (tm *TransactionManager) armTransactionDeadlineLocked() {
	owner := tm.tc
	if owner == nil {
		return
	}
	if owner.attrs.mode == transactionModeReadOnly {
		return
	}
	owner.firstUse = true
	if owner.timeout <= 0 || !owner.deadline.IsZero() {
		return
	}
	deadline := tm.now().Add(owner.timeout)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	owner.deadline = deadline
	owner.deadlineCtx = ctx
	owner.deadlineCancel = cancel
	go tm.watchTransactionDeadline(owner, ctx)
}

// bindDeadlineLocked returns ctx shortened to the remaining owner budget.
// Caller must hold tm.mu (read or write). The cancel must be called.
func (tm *TransactionManager) bindDeadlineLocked(ctx context.Context) (context.Context, context.CancelFunc) {
	if tm.tc == nil || tm.tc.deadline.IsZero() {
		return ctx, func() {}
	}
	return context.WithDeadline(ctx, tm.tc.deadline)
}

// armAndBindDeadlineLocked starts the budget if needed and applies the
// remaining deadline to ctx. Caller must hold tm.mu.
func (tm *TransactionManager) armAndBindDeadlineLocked(ctx context.Context) (context.Context, context.CancelFunc) {
	tm.armTransactionDeadlineLocked()
	return tm.bindDeadlineLocked(ctx)
}

// bindTransactionDeadline applies the remaining owner budget without
// starting it. Used by callers that cannot hold tm.mu across an RPC that
// already went through an arming path (constructor or prior statement).
func (tm *TransactionManager) bindTransactionDeadline(ctx context.Context) (context.Context, context.CancelFunc) {
	if tm == nil {
		return ctx, func() {}
	}
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.bindDeadlineLocked(ctx)
}

// batchUpdateWithRemainingDeadline binds the remaining TRANSACTION_TIMEOUT
// at the actual Batch DML RPC. Caller must hold tm.mu. Every BatchUpdate
// route (manual batch, RUN BATCH, FlushAutomaticDML, flush-before-read,
// SAVEPOINT replay) must go through this helper so a captured caller
// context cannot bypass the owner budget. Mutex-held Batch DML observes
// the bound deadline and can cancel without waiting for tm.mu.
func (tm *TransactionManager) batchUpdateWithRemainingDeadline(ctx context.Context, tx *spanner.ReadWriteStmtBasedTransaction, dmls []spanner.Statement, opts spanner.QueryOptions) ([]int64, error) {
	ctx, cancel := tm.armAndBindDeadlineLocked(ctx)
	defer cancel()
	return tx.BatchUpdateWithOptions(ctx, dmls, opts)
}

// watchTransactionDeadline cancels in-flight RPCs via the already-armed
// deadline context (no lifecycle lock, no tm.mu) and then expires only
// the matching logical owner under tm.mu. It never calls Registry.Set.
func (tm *TransactionManager) watchTransactionDeadline(owner *transactionContext, ctx context.Context) {
	<-ctx.Done()
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return
	}
	tm.retireMatchingOwner(owner)
}

// retireMatchingOwner expires only the matching logical owner under tm.mu.
// Outside ExecuteStatement it detaches SET LOCAL undo immediately. During
// a statement it marks expirePending and stops heartbeat/deadline watchers
// without detaching undo, so syncExpiredOwnerRestore can restore before
// ordinary SET, default reads, or a replacement owner. It does not call
// Registry.Set.
func (tm *TransactionManager) retireMatchingOwner(owner *transactionContext) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if tm.tc != owner {
		return
	}
	if tm.statementDepth > 0 {
		owner.expirePending = true
		owner.Close()
	} else {
		tm.retireTransactionContextLocked()
	}
	if hook := tm.timeoutAfterExpire; hook != nil {
		hook(owner)
	}
}

func annotateTransactionTimeout(err error, owner *transactionContext) error {
	if err == nil || owner == nil || owner.deadline.IsZero() {
		return err
	}
	if !isDeadlineExceeded(err) {
		return err
	}
	if owner.deadlineCtx != nil && !errors.Is(owner.deadlineCtx.Err(), context.DeadlineExceeded) && time.Now().Before(owner.deadline) {
		return err
	}
	return fmt.Errorf("%w: %w", errTransactionTimeout, err)
}

func isDeadlineExceeded(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || status.Code(err) == codes.DeadlineExceeded
}
