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
	"math/rand/v2"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"google.golang.org/grpc/codes"
)

// maxImplicitAbortAttempts is the inclusive cap for one implicit logical
// operation, including its first execution (#994 / #293 phase 1).
const maxImplicitAbortAttempts = 50

type implicitAbortRetryPolicy int

const (
	implicitAbortRetryIfEnabled implicitAbortRetryPolicy = iota
	implicitAbortRetryDisabled
)

var (
	errRetryAbortsExplicitUnsupported = errors.New(
		"RETRY_ABORTS_INTERNALLY=TRUE is not supported for explicit or pending read-write transactions; this property currently retries only implicit autocommit read-write operations")
	errRetryAbortsSetLocalUnsupported = errors.New(
		"SET LOCAL is not supported for RETRY_ABORTS_INTERNALLY; this property currently applies only to implicit autocommit read-write operations. Use ordinary SET to change the session value for later implicit operations. Pending-owner SET LOCAL activation is not implemented yet")
	errImplicitAbortRetryLostOwner = errors.New("implicit abort retry lost its logical owner")
)

func isAbortedErr(err error) bool {
	return err != nil && spanner.ErrCode(err) == codes.Aborted
}

func wrapAbortedIfNeeded(err error) error {
	if err == nil || !isAbortedErr(err) {
		return err
	}
	return fmt.Errorf("transaction was aborted: %w", err)
}

func snapshotRetryAbortsLocked(tc *transactionContext, vars *systemVariables) {
	if tc == nil || tc.retryAbortsCaptured {
		return
	}
	if vars != nil {
		tc.retryAborts = vars.Transaction.RetryAbortsInternally
	}
	tc.retryAbortsCaptured = true
}

func (tm *TransactionManager) rejectExplicitRetryAbortsLocked() error {
	if tm == nil || tm.sysVars == nil || !tm.sysVars.Transaction.RetryAbortsInternally {
		return nil
	}
	// Pending created while READONLY will resolve to an RO owner.
	// Do not reject that READONLY path.
	if tm.sysVars.Transaction.ReadOnly {
		return nil
	}
	return errRetryAbortsExplicitUnsupported
}

func abortRetryDelay(err error) time.Duration {
	// ExtractRetryDelay unwraps *spanner.Error to the original status.
	// GRPCStatus() rebuilds code/description and drops RetryInfo details.
	if d, ok := spanner.ExtractRetryDelay(err); ok && d > 0 {
		return d
	}
	return time.Duration(1+rand.IntN(32)) * time.Millisecond
}

func clampAbortRetryDelay(d, remaining time.Duration, unlimited bool) time.Duration {
	if d < 0 {
		return 0
	}
	if unlimited {
		return d
	}
	if remaining <= 0 {
		return 0
	}
	if d > remaining {
		return remaining
	}
	return d
}

func ownerDeadlineExhausted(owner *transactionContext, now time.Time) bool {
	if owner == nil {
		return false
	}
	if owner.expirePending {
		return true
	}
	return owner.timeoutCaptured && !owner.deadline.IsZero() && !now.Before(owner.deadline)
}

func implicitAbortDeadlineErr(err error) error {
	if err == nil {
		err = context.DeadlineExceeded
	}
	if errors.Is(err, errTransactionTimeout) {
		return err
	}
	return fmt.Errorf("%w: %w", errTransactionTimeout, err)
}

// abortWaitBudget returns how long abort backoff may wait.
// unlimited is true only when neither the caller nor the owner has a deadline.
// unlimited is false with remaining==0 when a deadline exists and is exhausted.
func (tm *TransactionManager) abortWaitBudget(ctx context.Context) (remaining time.Duration, unlimited bool) {
	now := time.Now()
	if tm != nil {
		now = tm.now()
	}
	if tm != nil && ownerDeadlineExhausted(tm.tc, now) {
		return 0, false
	}

	var deadline time.Time
	limited := false
	if dl, ok := ctx.Deadline(); ok {
		deadline = dl
		limited = true
	}
	if tm != nil && tm.tc != nil && tm.tc.timeoutCaptured && !tm.tc.deadline.IsZero() {
		if !limited || tm.tc.deadline.Before(deadline) {
			deadline = tm.tc.deadline
		}
		limited = true
	}
	if !limited {
		return 0, true
	}
	rem := deadline.Sub(now)
	if rem < 0 {
		return 0, false
	}
	return rem, false
}

func (tm *TransactionManager) waitAbortRetry(ctx context.Context, d time.Duration) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if d <= 0 {
		return nil
	}
	if tm != nil && tm.abortRetryWait != nil {
		return tm.abortRetryWait(ctx, d)
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func shouldRetryImplicitAbort(err error, implicit bool, policy implicitAbortRetryPolicy, owner *transactionContext, attempt, maxAttempts int) bool {
	if err == nil || !implicit || policy != implicitAbortRetryIfEnabled || owner == nil || !owner.retryAborts {
		return false
	}
	if attempt >= maxAttempts || isAdmissionError(err) {
		return false
	}
	return isAbortedErr(err)
}

func (tm *TransactionManager) freezeQueryOptionsLocked(mode *sppb.ExecuteSqlRequest_QueryMode) spanner.QueryOptions {
	if tm.frozenQueryOpts != nil {
		return *tm.frozenQueryOpts
	}
	opts := tm.queryOptionsLocked(mode)
	if tm.sysVars != nil {
		tm.sysVars.Transaction.RequestTag = ""
	}
	snap := opts
	tm.frozenQueryOpts = &snap
	return opts
}

// discardPhysicalKeepBudgetLocked rolls back the current physical handle and
// stops heartbeat without cancelling the logical deadline or idle clock.
// Caller must hold tm.mu.
func (tm *TransactionManager) discardPhysicalKeepBudgetLocked(context.Context) {
	if tm.tc == nil {
		return
	}
	if tm.tc.heartbeatCancel != nil {
		tm.tc.heartbeatCancel()
		tm.tc.heartbeatCancel = nil
	}
	cleanup, cancel := savepointCleanupContext()
	defer cancel()
	if rw, ok := tm.tc.txn.(*spanner.ReadWriteStmtBasedTransaction); ok && rw != nil {
		rw.Rollback(cleanup)
	}
	tm.tc.txn = nil
	if tm.tc.pending != nil {
		if tm.tc.replay != nil {
			tm.tc.replay.release(tm.tc.pending.reserved)
		}
		tm.tc.pending = nil
	}
	tm.tc.inFlight = 0
}

// reconstructImplicitPhysicalLocked starts a new physical attempt on the same
// logical owner using frozen constructor options. It is not new user activity
// and does not rearm idle or restart TRANSACTION_TIMEOUT.
func (tm *TransactionManager) reconstructImplicitPhysicalLocked(ctx context.Context) error {
	owner := tm.tc
	if owner == nil || owner.attrs.mode != transactionModeReadWrite {
		return ErrNotInReadWriteTransaction
	}
	tm.discardPhysicalKeepBudgetLocked(ctx)

	ctx, cancelDeadline := tm.bindDeadlineLocked(ctx)
	defer cancelDeadline()

	candidate, err := spanner.NewReadWriteStmtBasedTransactionWithOptions(ctx, tm.client, owner.ctorOpts)
	if err != nil {
		tm.retireTransactionContextLocked()
		return annotateTransactionTimeout(err, owner)
	}
	owner.publishPhysical(candidate)
	return nil
}

func (tm *TransactionManager) stopImplicitAbortForDeadlineLocked(owner *transactionContext, info rwTxAttemptInfo) (*DMLResult, rwTxAttemptInfo, error) {
	if tm.tc == owner {
		tm.retireTransactionContextLocked()
	}
	return nil, info, implicitAbortDeadlineErr(context.DeadlineExceeded)
}

func (tm *TransactionManager) finalizeImplicitAbortLocked(ctx context.Context, info rwTxAttemptInfo, err error) error {
	if tm.tc == nil {
		return wrapAbortedIfNeeded(err)
	}
	if info.phase == dmlAttemptPhaseCommit {
		tm.retireTransactionContextLocked()
		return err
	}
	err = tm.handleOwnerFailureLocked(ctx, err)
	if tm.tc != nil {
		tm.noteIdleUserWorkLocked(true)
	}
	return wrapAbortedIfNeeded(err)
}
