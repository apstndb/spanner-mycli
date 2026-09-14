// Copyright 2026 apstndb
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

package mycli

import (
	"context"
	"errors"
	"fmt"

	"cloud.google.com/go/spanner"
)

// automaticDMLEntry is one owner-owned automatic DML queue item. expected and
// verify are frozen at enqueue so a later SET cannot reinterpret this entry.
type automaticDMLEntry struct {
	stmt     spanner.Statement
	expected int64
	verify   bool
}

// errAutomaticDMLCountMismatch is the cause when an enabled automatic DML
// entry's actual BatchUpdate count does not match the count captured at enqueue.
var errAutomaticDMLCountMismatch = errors.New("automatic DML update count mismatch")

func (tm *TransactionManager) discardAutomaticDMLLocked() {
	if tm.tc != nil {
		tm.tc.autoDML = nil
		if tm.tc.replay != nil {
			tm.tc.replay.dropQueued()
		}
	}
}

// DiscardAutomaticDML drops pending automatic DML without executing it.
// ABORT BATCH uses this; it does not roll back already executed statements
// and does not change AUTO_BATCH_DML.
func (tm *TransactionManager) DiscardAutomaticDML() {
	_ = tm.withTransactionContextWithLock(func(**transactionContext) error {
		tm.discardAutomaticDMLLocked()
		return nil
	})
}

// HasAutomaticDML reports whether automatic DML is waiting on the current
// transaction context.
func (tm *TransactionManager) HasAutomaticDML() bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.tc != nil && len(tm.tc.autoDML) > 0
}

// AutomaticBatchInfo returns pending-automatic BatchInfo, or nil if the queue is empty.
func (tm *TransactionManager) AutomaticBatchInfo() *BatchInfo {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	if tm.tc == nil || len(tm.tc.autoDML) == 0 {
		return nil
	}
	return &BatchInfo{Mode: batchModeDML, Size: len(tm.tc.autoDML)}
}

// TryEnqueueAutomaticDML appends stmt to the current explicit RW transaction's
// automatic queue. It returns enqueued=false without error when no RW owner
// exists so implicit DML can execute immediately.
func (tm *TransactionManager) TryEnqueueAutomaticDML(stmt spanner.Statement) (bool, error) {
	var enqueued bool
	err := tm.withTransactionContextWithLock(func(**transactionContext) error {
		if tm.tc == nil || tm.tc.attrs.mode != transactionModeReadWrite {
			return nil
		}
		if err := tm.rejectIfRecoveringLocked(); err != nil {
			return err
		}
		if err := tm.enqueueFrozenAutomaticDMLLocked(stmt); err != nil {
			return err
		}
		tm.tc.autoDML = append(tm.tc.autoDML, tm.newAutomaticDMLEntry(stmt))
		// Queued automatic DML is uncommitted work on an already-started RW
		// owner. Enable the existing keepalive now; waiting until flush is too
		// late to cover the think/paste interval before COMMIT or a read.
		tm.tc.EnableHeartbeat()
		enqueued = true
		tm.noteIdleUserWorkLocked(true)
		return nil
	})
	return enqueued, err
}

// FlushAutomaticDML executes queued automatic DML on the existing RW owner.
// It never starts an implicit transaction. An empty queue is a no-op.
// On BatchUpdate failure the RW transaction is rolled back and the queue is
// not replayable in a later owner. I/O under mu matches executeBatchDML.
func (tm *TransactionManager) FlushAutomaticDML(ctx context.Context) (*Result, error) {
	var dmls []spanner.Statement
	var counts []int64
	err := tm.withTransactionContextWithLock(func(**transactionContext) error {
		var flushErr error
		dmls, counts, flushErr = tm.flushAutomaticDMLLocked(ctx)
		return flushErr
	})
	if err != nil {
		return nil, err
	}
	if len(dmls) == 0 {
		return nil, nil
	}
	return newBatchDMLResult(dmls, counts, &DMLResult{}), nil
}

// flushAutomaticDMLLocked takes the current context's automatic queue then
// BatchUpdates it on that same RW owner. Caller must hold tm.mu. The queue is
// cleared before the RPC so a failure cannot replay work. A missing RW owner
// or owner replacement discards without executing so stale work cannot open a
// new implicit transaction.
func (tm *TransactionManager) flushAutomaticDMLLocked(ctx context.Context) ([]spanner.Statement, []int64, error) {
	owner := tm.tc
	if owner == nil || len(owner.autoDML) == 0 {
		return nil, nil, nil
	}
	queued := owner.autoDML
	owner.autoDML = nil
	dmls := automaticDMLStatements(queued)

	if owner != tm.tc || owner.txn == nil || owner.attrs.mode != transactionModeReadWrite {
		return nil, nil, nil
	}
	rwTxn, ok := owner.txn.(*spanner.ReadWriteStmtBasedTransaction)
	if !ok {
		return nil, nil, ErrNotInReadWriteTransaction
	}

	counts, err := tm.batchUpdateWithRemainingDeadline(ctx, rwTxn, dmls, spanner.QueryOptions{LastStatement: false})
	err = annotateTransactionTimeout(err, owner)
	if err == nil {
		// Compare each enabled entry before a successful journal receipt.
		// RPC or partial BatchUpdate errors keep their original cause.
		err = verifyAutomaticDMLCounts(queued, counts)
	}
	if _, recErr := tm.completeBatchDMLLocked(counts, err); err == nil {
		err = recErr
	}
	if tm.tc != nil {
		tm.tc.EnableHeartbeat()
	}
	if err != nil {
		err = tm.handleOwnerFailureLocked(ctx, err)
		if tm.tc != nil {
			tm.noteIdleUserWorkLocked(true)
		}
		return nil, nil, fmt.Errorf("transaction was aborted: %w", err)
	}
	tm.noteIdleUserWorkLocked(true)
	return dmls, counts, nil
}

func (tm *TransactionManager) heartbeatEnabled() bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.tc.IsHeartbeatEnabled()
}

func (tm *TransactionManager) invokeQueryAfterCollectHook() error {
	tm.mu.RLock()
	hook := tm.queryAfterCollectHook
	tm.mu.RUnlock()
	if hook == nil {
		return nil
	}
	return hook()
}

func (tm *TransactionManager) newAutomaticDMLEntry(stmt spanner.Statement) automaticDMLEntry {
	e := automaticDMLEntry{stmt: stmt, expected: 1}
	if tm.sysVars != nil {
		e.expected = tm.sysVars.Transaction.AutoBatchDMLUpdateCount
		e.verify = tm.sysVars.Transaction.AutoBatchDMLUpdateCountVerification
	}
	return e
}

func automaticDMLStatements(queued []automaticDMLEntry) []spanner.Statement {
	dmls := make([]spanner.Statement, len(queued))
	for i, e := range queued {
		dmls[i] = e.stmt
	}
	return dmls
}

// verifyAutomaticDMLCounts compares each enabled entry's captured expectation
// with the corresponding actual count. Aggregate equality is not enough.
// Disabled entries are skipped. Missing actual counts are not fabricated.
func verifyAutomaticDMLCounts(queued []automaticDMLEntry, counts []int64) error {
	for i, e := range queued {
		if !e.verify {
			continue
		}
		if i >= len(counts) {
			return fmt.Errorf("%w at statement %d: expected %d, actual count missing",
				errAutomaticDMLCountMismatch, i+1, e.expected)
		}
		if counts[i] != e.expected {
			return fmt.Errorf("%w at statement %d: expected %d, actual %d",
				errAutomaticDMLCountMismatch, i+1, e.expected, counts[i])
		}
	}
	return nil
}
