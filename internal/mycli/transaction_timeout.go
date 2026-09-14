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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TRANSACTION_TIMEOUT is a logical read/write budget, distinct from
// STATEMENT_TIMEOUT and from unimplemented user-idle expiry (#357).
// Zero or NULL means no additional transaction deadline. ABORTED retry
// (#293) is not implemented; a later retry path must reuse this remaining
// budget instead of restarting it.
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
// when SET LOCAL happens before the first database RPC. After the budget is
// armed, only an identical value is accepted.
func (tm *TransactionManager) applyLocalTransactionTimeout(d time.Duration) error {
	return tm.withTransactionContextWithLock(func(tcPtr **transactionContext) error {
		if *tcPtr == nil {
			return ErrNoTransaction
		}
		owner := *tcPtr
		if !owner.deadline.IsZero() && owner.timeout != d {
			return errTransactionTimeoutFrozen
		}
		owner.timeout = d
		owner.timeoutCaptured = true
		return nil
	})
}

// armTransactionDeadlineLocked starts the single total budget at the first
// real read/write transaction RPC. Caller must hold tm.mu. Pending BEGIN,
// SHOW, multiplexed session take, and DML buffering do not call this.
// Read-only owners never arm. Re-arming is a no-op so SAVEPOINT
// reconstruction and later statements keep the original deadline.
func (tm *TransactionManager) armTransactionDeadlineLocked() {
	owner := tm.tc
	if owner == nil || owner.timeout <= 0 || !owner.deadline.IsZero() {
		return
	}
	if owner.attrs.mode == transactionModeReadOnly {
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

// watchTransactionDeadline cancels in-flight RPCs via the already-armed
// deadline context (no mutex) and then retires only the matching logical
// owner under tm.mu. It never calls Registry.Set; SET LOCAL undo is
// detached for the session/CLI safe point.
func (tm *TransactionManager) watchTransactionDeadline(owner *transactionContext, ctx context.Context) {
	<-ctx.Done()
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if tm.tc != owner {
		return
	}
	tm.retireTransactionContextLocked()
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
