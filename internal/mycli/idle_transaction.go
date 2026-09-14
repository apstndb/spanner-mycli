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
)

const idleTransactionTimeoutVarName = "CLI_IDLE_TRANSACTION_TIMEOUT"

// CLI_IDLE_TRANSACTION_TIMEOUT is a sliding user-idle quiet interval on the
// logical owner. It is distinct from TRANSACTION_TIMEOUT (#482) and from
// KEEP_TRANSACTION_ALIVE. NULL or 0 disables idle expiry. idleUserWork is
// admitted user/database work on this owner. It is not constructor firstUse
// and not #402 hasUserWork (DDL emptiness).
var (
	errIdleTransactionTimeout = errors.New("CLI_IDLE_TRANSACTION_TIMEOUT exceeded; the transaction was rolled back")
	errIdleTransactionFrozen  = errors.New("CLI_IDLE_TRANSACTION_TIMEOUT cannot be changed after admitted user work")
)

func idleTransactionTimeoutDuration(vars *systemVariables) time.Duration {
	if vars == nil || vars.Transaction.IdleTransactionTimeout == nil {
		return 0
	}
	return *vars.Transaction.IdleTransactionTimeout
}

func snapshotIdleTimeoutLocked(tc *transactionContext, vars *systemVariables) {
	if tc == nil || tc.idleCaptured {
		return
	}
	tc.idle = idleTransactionTimeoutDuration(vars)
	tc.idleCaptured = true
}

func (tm *TransactionManager) applyLocalIdleTimeout(d time.Duration) error {
	return tm.withTransactionContextWithLock(func(tcPtr **transactionContext) error {
		if *tcPtr == nil {
			return ErrNoTransaction
		}
		owner := *tcPtr
		if owner.idleUserWork && owner.idle != d {
			return errIdleTransactionFrozen
		}
		owner.idle = d
		owner.idleCaptured = true
		if !owner.idleUserWork {
			tm.cancelIdleTimerLocked(owner)
		}
		return nil
	})
}

func acksIdleNotice(stmt Statement) bool {
	switch stmt.(type) {
	case *RollbackStatement, *BeginStatement, *BeginRwStatement, *BeginRoStatement,
		*UseStatement, *UseDatabaseMetaCommand, *DetachStatement:
		return true
	default:
		return false
	}
}

func (tm *TransactionManager) consumeIdleNotice(ack bool) error {
	if tm == nil {
		return nil
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if tm.idleNotice == nil {
		return nil
	}
	err := tm.idleNotice
	tm.idleNotice = nil
	if ack {
		return nil
	}
	return err
}

func (tm *TransactionManager) beginIdleResultHold() {
	if tm == nil {
		return
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()
	// Manager-level hold so a newly installed owner (BEGIN RW) is covered
	// through CLI output even when this ran before the owner existed.
	// Do not cancel the quiet-interval timer: client-only SHOW/SET must
	// keep the existing deadline.
	tm.idleCLIHold++
}

func (tm *TransactionManager) endIdleResultHold() {
	if tm == nil {
		return
	}
	tm.mu.Lock()
	if tm.idleCLIHold > 0 {
		tm.idleCLIHold--
	}
	if tm.tc != nil {
		tm.maybeRearmOrExpireIdleLocked(tm.tc)
	}
	tm.mu.Unlock()
	// Serialized CLI completion barrier after pager/error cleanup.
	// Timer goroutines never call Registry.Set; restore detached SET LOCAL
	// undo here once no owner remains.
	tm.restoreLocalVarsIfIdle()
}

// completeAdmittedQuery finishes optional SAVEPOINT capture and counts
// completed admitted query work even when no replay token exists. Call this
// after the query RPC/iterator has run. Rejected local admission should
// return before this hook. A surviving owner still counts a server/result
// error as activity.
func (tm *TransactionManager) completeAdmittedQuery(tok *captureToken, consumeErr error) error {
	if tm == nil {
		return consumeErr
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()
	err := tm.finishQueryCaptureLocked(tok, consumeErr)
	if tm.tc != nil {
		tm.noteIdleUserWorkLocked(true)
	}
	if consumeErr != nil {
		return consumeErr
	}
	return err
}

func (tm *TransactionManager) noteIdleUserWorkLocked(survived bool) {
	if tm == nil || tm.tc == nil || !survived {
		return
	}
	owner := tm.tc
	owner.idleUserWork = true
	owner.idleLastUser = tm.now()
	owner.idleElapsedHeld = false
	if tm.idleBlockedLocked(owner) {
		tm.cancelIdleTimerLocked(owner)
		owner.idleNeedsRearm = owner.idle > 0
		return
	}
	tm.rearmIdleLocked(owner)
}

func (tm *TransactionManager) noteIdleUserWork(survived bool) {
	if tm == nil {
		return
	}
	tm.mu.Lock()
	tm.noteIdleUserWorkLocked(survived)
	tm.mu.Unlock()
}

func (tm *TransactionManager) idleBlockedLocked(owner *transactionContext) bool {
	if owner == nil {
		return true
	}
	return tm.statementDepth > 0 || tm.idleCLIHold > 0 || owner.idleHold > 0
}

func (tm *TransactionManager) cancelIdleTimerLocked(owner *transactionContext) {
	if owner == nil || owner.idleCancel == nil {
		return
	}
	owner.idleCancel()
	owner.idleCancel = nil
}

func (tm *TransactionManager) maybeRearmOrExpireIdleLocked(owner *transactionContext) {
	if owner == nil || owner != tm.tc {
		return
	}
	if tm.idleBlockedLocked(owner) {
		return
	}
	if owner.idleNeedsRearm {
		if owner.idle <= 0 || !owner.idleUserWork {
			owner.idleNeedsRearm = false
			owner.idleElapsedHeld = false
			tm.cancelIdleTimerLocked(owner)
			return
		}
		tm.rearmIdleLocked(owner)
		return
	}
	if owner.idleElapsedHeld {
		tm.rollbackAndRetireIdleLocked(owner)
		if hook := tm.idleAfterExpire; hook != nil {
			hook(owner)
		}
	}
}

func (tm *TransactionManager) rearmIdleLocked(owner *transactionContext) {
	if owner == nil || owner.idle <= 0 || !owner.idleUserWork {
		tm.cancelIdleTimerLocked(owner)
		return
	}
	tm.cancelIdleTimerLocked(owner)
	owner.idleNeedsRearm = false
	owner.idleElapsedHeld = false
	owner.idleGen++
	gen := owner.idleGen
	deadline := tm.now().Add(owner.idle)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	owner.idleCancel = cancel
	go tm.watchIdleDeadline(owner, gen, ctx)
}

func (tm *TransactionManager) watchIdleDeadline(owner *transactionContext, gen uint64, ctx context.Context) {
	<-ctx.Done()
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return
	}
	tm.tryIdleExpire(owner, gen)
}

func (tm *TransactionManager) tryIdleExpire(owner *transactionContext, gen uint64) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if tm.tc != owner || owner.idleGen != gen {
		return
	}
	if owner.idle <= 0 || !owner.idleUserWork {
		return
	}
	if tm.idleBlockedLocked(owner) {
		// Preserve the elapsed deadline. Non-activity must not restart
		// idle; retire at the next safe barrier unless admitted work
		// completes and requests a fresh interval.
		owner.idleElapsedHeld = true
		return
	}
	tm.rollbackAndRetireIdleLocked(owner)
	if hook := tm.idleAfterExpire; hook != nil {
		hook(owner)
	}
}

func (tm *TransactionManager) rollbackAndRetireIdleLocked(owner *transactionContext) {
	if tm.tc != owner {
		return
	}
	if rw, ok := owner.txn.(*spanner.ReadWriteStmtBasedTransaction); ok && rw != nil {
		ctx, cancel := savepointCleanupContext()
		rw.Rollback(ctx)
		cancel()
	} else if ro, ok := owner.txn.(*spanner.ReadOnlyTransaction); ok && ro != nil {
		ro.Close()
	}
	owner.idleExpired = true
	tm.idleNotice = fmt.Errorf("%w", errIdleTransactionTimeout)
	tm.retireTransactionContextLocked()
}

func (tm *TransactionManager) retireIdleIfPendingLocked() bool {
	if tm.tc == nil || !tm.tc.expirePending || !tm.tc.idleExpired {
		return false
	}
	tm.rollbackAndRetireIdleLocked(tm.tc)
	return true
}
