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
	"io"

	"cloud.google.com/go/spanner"
)

// maxExplicitAbortRetries is the number of automatic physical reconstructions
// allowed per logical owner (49 reconstructions + the original attempt = 50).
const maxExplicitAbortRetries = maxImplicitAbortAttempts - 1

var errRetryAbortsSetLocalNotPending = errors.New(
	"SET LOCAL RETRY_ABORTS_INTERNALLY is only allowed on an unused pending read-write transaction before the first database operation, queued work, or SAVEPOINT marker")

type deliveredByteCounter struct {
	w         io.Writer
	n         int64
	presented bool
}

func (c *deliveredByteCounter) Write(p []byte) (int, error) {
	if c == nil || c.w == nil {
		return len(p), nil
	}
	n, err := c.w.Write(p)
	if n > 0 {
		c.n += int64(n)
	}
	return n, err
}

func (c *deliveredByteCounter) notePresented() {
	if c != nil {
		c.presented = true
	}
}

func (c *deliveredByteCounter) delivered() bool {
	return c != nil && (c.n > 0 || c.presented)
}

func wrapDeliveredWriter(out OperationOutput) (OperationOutput, *deliveredByteCounter) {
	w := out.Writer()
	if w == nil {
		return out, &deliveredByteCounter{}
	}
	c := &deliveredByteCounter{w: w}
	return out.withWriter(c), c
}

func (tm *TransactionManager) explicitAbortRetryEligibleLocked(err error) bool {
	if tm == nil || tm.tc == nil || !tm.tc.retryAborts {
		return false
	}
	if tm.tc.attrs.mode != transactionModeReadWrite {
		return false
	}
	if err == nil || isAdmissionError(err) || !isAbortedErr(err) {
		return false
	}
	return true
}

// explicitQueryAbortRetryEligibleLocked requires a still-current admitted
// owner/attempt token. Nil (single-use/unadmitted), stale, PLAN-only, and
// RO tokens must not reconstruct the live owner.
func (tm *TransactionManager) explicitQueryAbortRetryEligibleLocked(tok *captureToken, err error) bool {
	if !tm.explicitAbortRetryEligibleLocked(err) {
		return false
	}
	if tok == nil || tok.planOnly || !tok.belongsToLocked(tm) {
		return false
	}
	return true
}

func (tm *TransactionManager) pendingRetryLocalAllowedLocked() error {
	if tm == nil || tm.tc == nil {
		return errors.New("SET LOCAL requires an active transaction; start one with BEGIN")
	}
	owner := tm.tc
	if owner.attrs.mode != transactionModePending {
		return errRetryAbortsSetLocalNotPending
	}
	if owner.firstUse || owner.idleUserWork || owner.inFlight > 0 || owner.pending != nil {
		return errRetryAbortsSetLocalNotPending
	}
	if len(owner.autoDML) > 0 {
		return errRetryAbortsSetLocalNotPending
	}
	if owner.replay != nil && (len(owner.replay.entries) > 0 || owner.replay.hasMarkers() || len(owner.replay.queued) > 0 || owner.replay.admittedBytes > 0) {
		return errRetryAbortsSetLocalNotPending
	}
	return nil
}

func (tm *TransactionManager) checkLocalRetryAborts() error {
	return tm.withTransactionContextWithLock(func(**transactionContext) error {
		return tm.pendingRetryLocalAllowedLocked()
	})
}

func (tm *TransactionManager) applyLocalRetryAborts(enabled bool) error {
	return tm.withTransactionContextWithLock(func(**transactionContext) error {
		if err := tm.pendingRetryLocalAllowedLocked(); err != nil {
			return err
		}
		tm.tc.retryAborts = enabled
		tm.tc.retryAbortsCaptured = true
		if enabled {
			tm.ensureReplayLocked()
			return nil
		}
		if !tm.savepointCaptureEnabledLocked() {
			tm.tc.replay = nil
		}
		return nil
	})
}

// recoverExplicitAbortLocked reconstructs the physical RW attempt and silently
// replays the journaled prefix after a real Spanner ABORTED. Caller holds
// tm.mu and may be unlocked across the interruptible backoff wait.
// recovered is true when the caller should retry the current frozen operation.
func (tm *TransactionManager) recoverExplicitAbortLocked(ctx context.Context, err error, currentOutputDelivered bool) (bool, error) {
	if !tm.explicitAbortRetryEligibleLocked(err) {
		return false, err
	}
	owner := tm.tc
	if currentOutputDelivered || owner.abortRetries >= maxExplicitAbortRetries {
		return false, tm.handleOwnerFailureLocked(ctx, err)
	}

	for {
		if ctx.Err() != nil {
			if tm.tc == owner {
				tm.retireTransactionContextLocked()
			}
			return false, ctx.Err()
		}
		if ownerDeadlineExhausted(owner, tm.now()) {
			if tm.tc == owner {
				tm.retireTransactionContextLocked()
			}
			return false, abortDeadlineCause(ctx, owner, tm.now(), nil)
		}
		remaining, unlimited := tm.abortWaitBudget(ctx)
		if !unlimited && remaining <= 0 {
			if tm.tc == owner {
				tm.retireTransactionContextLocked()
			}
			return false, abortDeadlineCause(ctx, owner, tm.now(), nil)
		}

		if recErr := tm.reconstructExplicitPhysicalLocked(ctx); recErr != nil {
			return false, recErr
		}
		owner.abortRetries++

		remaining, unlimited = tm.abortWaitBudget(ctx)
		if !unlimited && remaining <= 0 {
			if tm.tc == owner {
				tm.retireTransactionContextLocked()
			}
			return false, abortDeadlineCause(ctx, owner, tm.now(), nil)
		}

		delay := clampAbortRetryDelay(abortRetryDelay(err), remaining, unlimited)
		waitCtx, waitCancel := tm.bindDeadlineLocked(ctx)
		tm.mu.Unlock()
		waitErr := tm.waitAbortRetry(waitCtx, delay)
		waitCancel()
		tm.mu.Lock()
		if waitErr != nil {
			if tm.tc == owner {
				tm.retireTransactionContextLocked()
			}
			return false, abortDeadlineCause(ctx, owner, tm.now(), waitErr)
		}
		if ownerDeadlineExhausted(owner, tm.now()) {
			if tm.tc == owner {
				tm.retireTransactionContextLocked()
			}
			return false, abortDeadlineCause(ctx, owner, tm.now(), nil)
		}
		if tm.tc != owner || owner.txn == nil {
			if tm.tc == owner {
				tm.retireTransactionContextLocked()
			}
			return false, errImplicitAbortRetryLostOwner
		}

		replayErr := tm.replayJournalLocked(ctx)
		if replayErr == nil {
			// Caller cancel is terminal even if the mock/RPC raced past the
			// canceled context and reported a successful prefix replay.
			if ctx.Err() != nil {
				if tm.tc == owner {
					tm.retireTransactionContextLocked()
				}
				return false, ctx.Err()
			}
			if owner.attrs.sendHeartbeat {
				owner.EnableHeartbeat()
			}
			return true, nil
		}
		if isAbortedErr(replayErr) {
			err = replayErr
			if owner.abortRetries >= maxExplicitAbortRetries {
				return false, tm.handleOwnerFailureLocked(ctx, err)
			}
			continue
		}
		return false, tm.failReconstructionLocked(replayErr)
	}
}

func (tm *TransactionManager) reconstructExplicitPhysicalLocked(ctx context.Context) error {
	owner := tm.tc
	if owner == nil || owner.attrs.mode != transactionModeReadWrite {
		return ErrNotInReadWriteTransaction
	}
	tm.discardPhysicalKeepBudgetLocked(ctx)

	ctx, cancelDeadline := tm.bindDeadlineLocked(ctx)
	defer cancelDeadline()

	owner.replacing = true
	defer func() {
		if tm.tc == owner {
			owner.replacing = false
		}
	}()

	candidate, err := spanner.NewReadWriteStmtBasedTransactionWithOptions(ctx, tm.client, owner.ctorOpts)
	if err != nil {
		tm.retireTransactionContextLocked()
		return annotateTransactionTimeout(err, owner)
	}
	owner.publishPhysical(candidate)
	return nil
}

func (tm *TransactionManager) replayJournalLocked(ctx context.Context) error {
	owner := tm.tc
	if owner == nil || owner.replay == nil || len(owner.replay.entries) == 0 {
		return nil
	}
	tx, ok := owner.txn.(*spanner.ReadWriteStmtBasedTransaction)
	if !ok || tx == nil {
		return ErrNotInReadWriteTransaction
	}
	ctx, cancelDeadline := tm.bindDeadlineLocked(ctx)
	defer cancelDeadline()
	prefix := append([]replayEntry(nil), owner.replay.entries...)
	return replayPrefix(ctx, tm, tx, prefix)
}

func (tm *TransactionManager) restoreAutomaticDMLForRetryLocked(queued []automaticDMLEntry) error {
	if tm.tc == nil {
		return errImplicitAbortRetryLostOwner
	}
	tm.tc.autoDML = queued
	if tm.tc.replay == nil || len(queued) == 0 {
		return nil
	}
	tm.tc.replay.queued = nil
	for _, e := range queued {
		if err := tm.enqueueFrozenAutomaticDMLLocked(e.stmt); err != nil {
			return err
		}
	}
	return nil
}

func (tm *TransactionManager) tryExplicitAbortRetry(ctx context.Context, tok *captureToken, err error, currentOutputDelivered bool) (recovered, handled bool, outErr error) {
	if tm == nil || err == nil {
		return false, false, err
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if !tm.explicitQueryAbortRetryEligibleLocked(tok, err) {
		return false, false, err
	}
	recovered, outErr = tm.recoverExplicitAbortLocked(ctx, err, currentOutputDelivered)
	return recovered, true, outErr
}

func wrapAbortedKeepCause(err error) error {
	if err == nil || !isAbortedErr(err) {
		return err
	}
	if errors.Is(err, errSavepointReconstructionFailed) || errors.Is(err, errTransactionTimeout) {
		return err
	}
	return fmt.Errorf("transaction was aborted: %w", err)
}
