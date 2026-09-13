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
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"cloud.google.com/go/spanner"
)

const (
	savepointNameLimit      = 128
	savepointCleanupTimeout = 5 * time.Second
)

var (
	errSavepointNotInTransaction     = errors.New("SAVEPOINT requires an explicit transaction")
	errSavepointDuplicate            = errors.New("savepoint already exists")
	errSavepointUnknown              = errors.New("savepoint does not exist")
	errSavepointInFlight             = errors.New("savepoint command while a query is in flight")
	errSavepointEmptyName            = errors.New("savepoint name is empty")
	errSavepointNameTooLong          = errors.New("savepoint name exceeds 128 code points")
	errSavepointRecovery             = errors.New("transaction requires ROLLBACK TO SAVEPOINT")
	errSavepointReconstructionFailed = errors.New("savepoint reconstruction failed; transaction ended")
	errSavepointFingerprintMismatch  = errors.New("savepoint replay fingerprint mismatch")
)

func validateSavepointName(name string) error {
	if name == "" {
		return errSavepointEmptyName
	}
	if utf8.RuneCountInString(name) > savepointNameLimit {
		return errSavepointNameTooLong
	}
	return nil
}

func (tm *TransactionManager) rejectIfRecoveringLocked() error {
	if tm.capturingLocked() && tm.tc.replay.needsRecovery() {
		return fmt.Errorf("%w: %v", errSavepointRecovery, tm.tc.replay.recoveryRequired)
	}
	return nil
}

func savepointCleanupContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), savepointCleanupTimeout)
}

func (tm *TransactionManager) shouldEnterRecoveryLocked() bool {
	return tm.capturingLocked() &&
		tm.tc.attrs.mode == transactionModeReadWrite &&
		tm.tc.replay.hasMarkers() &&
		!tm.tc.replay.needsRecovery()
}

func (tm *TransactionManager) discardPhysicalLocked(context.Context) {
	if tm.tc == nil {
		return
	}
	tm.tc.Close()
	cleanup, cancel := savepointCleanupContext()
	defer cancel()
	switch txn := tm.tc.txn.(type) {
	case *spanner.ReadWriteStmtBasedTransaction:
		txn.Rollback(cleanup)
	case *spanner.ReadOnlyTransaction:
		txn.Close()
	}
	tm.tc.txn = nil
}

func (tm *TransactionManager) enterRecoveryLocked(ctx context.Context, err error) error {
	tm.discardPhysicalLocked(ctx)
	if tm.tc != nil && tm.tc.replay != nil {
		tm.tc.replay.dropQueued()
		if tm.tc.pending != nil {
			tm.tc.replay.release(tm.tc.pending.reserved)
			tm.tc.pending = nil
		}
		tm.tc.inFlight = 0
		tm.tc.replay.recoveryRequired = err
	}
	return err
}

func (tm *TransactionManager) handleOwnerFailureLocked(ctx context.Context, err error) error {
	if isAdmissionError(err) {
		return err
	}
	if tm.shouldEnterRecoveryLocked() {
		return tm.enterRecoveryLocked(ctx, err)
	}
	if rollbackErr := tm.RollbackReadWriteTransactionLocked(ctx); rollbackErr != nil {
		return errors.Join(err, fmt.Errorf("error on rollback: %w", rollbackErr))
	}
	return err
}

func (tm *TransactionManager) HandleOwnerFailure(ctx context.Context, err error) error {
	if tm == nil || err == nil || isAdmissionError(err) {
		return err
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if tm.shouldEnterRecoveryLocked() {
		return tm.enterRecoveryLocked(ctx, err)
	}
	return err
}

func (tm *TransactionManager) NeedsRecovery() bool {
	if tm == nil {
		return false
	}
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.capturingLocked() && tm.tc.replay.needsRecovery()
}

func (tm *TransactionManager) CreateSavepoint(ctx context.Context, name string) error {
	if err := validateSavepointName(name); err != nil {
		return err
	}
	return tm.withTransactionContextWithLock(func(**transactionContext) error {
		if tm.tc == nil {
			return errSavepointNotInTransaction
		}
		if tm.tc.inFlight > 0 {
			return errSavepointInFlight
		}
		if err := tm.rejectIfRecoveringLocked(); err != nil {
			return err
		}
		if !tm.capturingLocked() {
			return errSavepointNotInTransaction
		}
		if _, _, ok := tm.tc.replay.lookup(name); ok {
			return errSavepointDuplicate
		}
		n := savepointMarkerBytes(name)
		if err := tm.tc.replay.reserve(n); err != nil {
			return err
		}
		if _, _, err := tm.flushAutomaticDMLLocked(ctx); err != nil {
			tm.tc.replay.release(n)
			return err
		}
		return tm.tc.replay.commitSavepoint(name, n)
	})
}

func (tm *TransactionManager) ReleaseSavepoint(name string) error {
	if err := validateSavepointName(name); err != nil {
		return err
	}
	return tm.withTransactionContextWithLock(func(**transactionContext) error {
		if tm.tc == nil {
			return errSavepointNotInTransaction
		}
		if tm.tc.inFlight > 0 {
			return errSavepointInFlight
		}
		if err := tm.rejectIfRecoveringLocked(); err != nil {
			return err
		}
		if !tm.capturingLocked() {
			return errSavepointNotInTransaction
		}
		return tm.tc.replay.releaseNamed(name)
	})
}

func (tm *TransactionManager) RollbackToSavepoint(ctx context.Context, name string) error {
	if err := validateSavepointName(name); err != nil {
		return err
	}
	return tm.withTransactionContextWithLock(func(**transactionContext) error {
		return tm.rollbackToSavepointLocked(ctx, name)
	})
}

func (tm *TransactionManager) rollbackToSavepointLocked(ctx context.Context, name string) error {
	if tm.tc == nil {
		return errSavepointNotInTransaction
	}
	if tm.tc.inFlight > 0 {
		return errSavepointInFlight
	}
	if !tm.capturingLocked() {
		return errSavepointNotInTransaction
	}
	idx, _, ok := tm.tc.replay.lookup(name)
	if !ok {
		return errSavepointUnknown
	}
	tm.tc.replay.dropQueued()
	tm.tc.autoDML = nil

	if tm.tc.attrs.mode != transactionModeReadWrite {
		tm.tc.replay.rollbackToMarker(idx)
		tm.tc.replay.recoveryRequired = nil
		return nil
	}

	prefix := append([]replayEntry(nil), tm.tc.replay.entries[:tm.tc.replay.savepoints[idx].position]...)
	ctor := tm.tc.ctorOpts
	tm.tc.replacing = true
	defer func() {
		if tm.tc != nil {
			tm.tc.replacing = false
		}
	}()
	tm.discardPhysicalLocked(ctx)

	candidate, err := spanner.NewReadWriteStmtBasedTransactionWithOptions(ctx, tm.client, ctor)
	if err != nil {
		return tm.failReconstructionLocked(err)
	}
	if err := replayPrefix(ctx, candidate, prefix); err != nil {
		cleanup, cancel := savepointCleanupContext()
		candidate.Rollback(cleanup)
		cancel()
		return tm.failReconstructionLocked(err)
	}
	tm.tc.publishPhysical(candidate)
	tm.tc.replay.rollbackToMarker(idx)
	tm.tc.replay.recoveryRequired = nil
	if tm.tc.attrs.sendHeartbeat {
		tm.tc.EnableHeartbeat()
	}
	return nil
}

func (tm *TransactionManager) failReconstructionLocked(err error) error {
	tm.retireTransactionContextLocked()
	return fmt.Errorf("%w: %w", errSavepointReconstructionFailed, err)
}

func (s frozenStatement) statement() spanner.Statement {
	params := make(map[string]any, len(s.Params))
	for k, v := range s.Params {
		params[k] = v
	}
	return spanner.Statement{SQL: s.SQL, Params: params}
}

func replayPrefix(ctx context.Context, tx *spanner.ReadWriteStmtBasedTransaction, prefix []replayEntry) error {
	for _, e := range prefix {
		if err := replayEntryOn(ctx, tx, e); err != nil {
			return err
		}
	}
	return nil
}

func replayEntryOn(ctx context.Context, tx *spanner.ReadWriteStmtBasedTransaction, e replayEntry) error {
	switch e.kind {
	case replayKindSQL:
		return replaySQL(ctx, tx, e)
	case replayKindBatchDML:
		return replayBatch(ctx, tx, e)
	case replayKindMutate:
		return replayMutations(ctx, tx, e)
	default:
		return fmt.Errorf("savepoint replay: unknown kind %d", e.kind)
	}
}

func replaySQL(ctx context.Context, tx *spanner.ReadWriteStmtBasedTransaction, e replayEntry) error {
	iter := tx.QueryWithOptions(ctx, e.stmt.statement(), e.stmt.Opts.toQueryOptions())
	rec := &operationReceipt{}
	_, count, _, _, err := consumeRowIterObserving(iter, func(*spanner.Row) error { return nil }, rec)
	if err != nil {
		_, _ = rec.Finish(err)
		return err
	}
	var fp []byte
	if e.dml {
		fp, err = rec.FinishDML(count, nil)
	} else {
		fp, err = rec.Finish(nil)
	}
	if err != nil {
		return err
	}
	if !bytes.Equal(fp, e.fingerprint) {
		return errSavepointFingerprintMismatch
	}
	return nil
}

func replayBatch(ctx context.Context, tx *spanner.ReadWriteStmtBasedTransaction, e replayEntry) error {
	dmls := make([]spanner.Statement, 0, len(e.batch))
	for _, stmt := range e.batch {
		dmls = append(dmls, stmt.statement())
	}
	counts, err := tx.BatchUpdateWithOptions(ctx, dmls, spanner.QueryOptions{LastStatement: false})
	if err != nil {
		return err
	}
	rec := &operationReceipt{}
	fp, err := rec.FinishBatch(counts, nil)
	if err != nil {
		return err
	}
	if !bytes.Equal(fp, e.fingerprint) {
		return errSavepointFingerprintMismatch
	}
	return nil
}

func replayMutations(ctx context.Context, tx *spanner.ReadWriteStmtBasedTransaction, e replayEntry) error {
	mutations := make([]*spanner.Mutation, 0, len(e.mutations))
	for _, m := range e.mutations {
		mut, err := m.Mutation()
		if err != nil {
			return err
		}
		mutations = append(mutations, mut)
	}
	if err := tx.BufferWrite(mutations); err != nil {
		return err
	}
	rec := &operationReceipt{}
	fp, err := rec.Finish(nil)
	if err != nil {
		return err
	}
	if !bytes.Equal(fp, e.fingerprint) {
		return errSavepointFingerprintMismatch
	}
	return nil
}
