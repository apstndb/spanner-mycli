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

func (tm *TransactionManager) discardAutomaticDMLLocked() {
	tm.autoDML = nil
	tm.autoDMLOwner = 0
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

// HasAutomaticDML reports whether automatic DML is waiting for this manager.
func (tm *TransactionManager) HasAutomaticDML() bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return len(tm.autoDML) > 0
}

// AutomaticBatchInfo returns pending-automatic BatchInfo, or nil if the queue is empty.
func (tm *TransactionManager) AutomaticBatchInfo() *BatchInfo {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	if len(tm.autoDML) == 0 {
		return nil
	}
	return &BatchInfo{Mode: batchModeDML, Size: len(tm.autoDML)}
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
		if tm.autoDMLOwner != 0 && tm.autoDMLOwner != tm.autoDMLGeneration {
			tm.discardAutomaticDMLLocked()
		}
		tm.autoDML = append(tm.autoDML, stmt)
		tm.autoDMLOwner = tm.autoDMLGeneration
		// Queued automatic DML is uncommitted work on an already-started RW
		// owner. Enable the existing keepalive now; waiting until flush is too
		// late to cover the think/paste interval before COMMIT or a read.
		tm.tc.EnableHeartbeat()
		enqueued = true
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

// flushAutomaticDMLLocked takes the automatic queue then BatchUpdates it on the
// current RW transaction. Caller must hold tm.mu. A generation mismatch or
// missing RW owner discards without executing so stale work cannot open a new
// implicit transaction.
func (tm *TransactionManager) flushAutomaticDMLLocked(ctx context.Context) ([]spanner.Statement, []int64, error) {
	if len(tm.autoDML) == 0 {
		return nil, nil, nil
	}
	dmls := tm.autoDML
	owner := tm.autoDMLOwner
	tm.discardAutomaticDMLLocked()

	if owner != tm.autoDMLGeneration || tm.tc == nil || tm.tc.txn == nil || tm.tc.attrs.mode != transactionModeReadWrite {
		return nil, nil, nil
	}
	rwTxn, ok := tm.tc.txn.(*spanner.ReadWriteStmtBasedTransaction)
	if !ok {
		return nil, nil, ErrNotInReadWriteTransaction
	}

	counts, err := rwTxn.BatchUpdateWithOptions(ctx, dmls, spanner.QueryOptions{LastStatement: false})
	if tm.tc != nil {
		tm.tc.EnableHeartbeat()
	}
	if err != nil {
		if rollbackErr := tm.RollbackReadWriteTransactionLocked(ctx); rollbackErr != nil {
			err = errors.Join(err, fmt.Errorf("error on rollback: %w", rollbackErr))
		}
		return nil, nil, fmt.Errorf("transaction was aborted: %w", err)
	}
	return dmls, counts, nil
}

// HeartbeatEnabled reports whether the current RW context has heartbeats armed.
func (tm *TransactionManager) HeartbeatEnabled() bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.tc.IsHeartbeatEnabled()
}

func (tm *TransactionManager) invokeQueryAfterFlushHook() error {
	tm.mu.RLock()
	hook := tm.queryAfterFlushHook
	tm.mu.RUnlock()
	if hook == nil {
		return nil
	}
	return hook()
}
