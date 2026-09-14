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
	"github.com/apstndb/spanner-mycli/enums"
)

// CLI_DDL_IN_TRANSACTION_MODE is captured on each logical owner, including
// pending BEGIN. Ordinary SET during an owner changes the next owner only.
// SET LOCAL may change this owner's captured policy only before user work.
//
// Empty means no admitted user database work, buffered mutations, queued DML,
// or SAVEPOINT marker/journal activity. SDK constructor BeginTransaction,
// TRANSACTION_TIMEOUT firstUse, and heartbeat alone do not make an owner
// nonempty. A failed server-visible user operation is still user work. The
// owner-owned hasUserWork bit is the history; queue/journal length is only a
// conservative extra check and is never cleared on physical reconstruction
// or ROLLBACK TO.
//
// CreateDatabase stays on the MutationStatement path and is out of scope.
var (
	errDDLInTransaction           = errors.New("DDL is not allowed while a transaction is active (CLI_DDL_IN_TRANSACTION_MODE=FAIL)")
	errDDLInNonEmptyTransaction   = errors.New("DDL is not allowed in a non-empty transaction (CLI_DDL_IN_TRANSACTION_MODE=ALLOW_IN_EMPTY_TRANSACTION)")
	errDDLInReadOnlyTransaction   = errors.New("DDL is not allowed in a read-only transaction")
	errDDLManualDMLBatch          = errors.New("there is active batch DML")
	errDDLCannotAutoCommit        = errors.New("DDL cannot auto-commit this transaction")
	errDdlInTransactionModeFrozen = errors.New("CLI_DDL_IN_TRANSACTION_MODE cannot be changed with SET LOCAL after this transaction has performed user work")
)

// ddlTxnPrep records a successful pre-DDL Commit so a later Admin failure
// can report both outcomes without inferring Commit from an error.
type ddlTxnPrep struct {
	committed bool
	commitTS  time.Time
	resp      spanner.CommitResponse
}

type ddlAfterCommitError struct {
	commitTS time.Time
	err      error
}

func (e *ddlAfterCommitError) Error() string {
	if e == nil || e.err == nil {
		return "transaction committed, but DDL failed"
	}
	if e.commitTS.IsZero() {
		return fmt.Sprintf("transaction committed, but DDL failed: %v", e.err)
	}
	return fmt.Sprintf("transaction committed at %s, but DDL failed: %v", e.commitTS.Format(time.RFC3339Nano), e.err)
}

func (e *ddlAfterCommitError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func annotateDDLAfterCommit(prep *ddlTxnPrep, err error) error {
	if err == nil || prep == nil || !prep.committed {
		return err
	}
	return &ddlAfterCommitError{commitTS: prep.commitTS, err: err}
}

func snapshotDdlInTransactionModeLocked(tc *transactionContext, vars *systemVariables) {
	if tc == nil || tc.ddlInTransactionCaptured {
		return
	}
	if vars != nil {
		tc.ddlInTransaction = vars.Transaction.DdlInTransactionMode
	}
	tc.ddlInTransactionCaptured = true
}

func (tm *TransactionManager) applyLocalDdlInTransactionMode(mode enums.DdlInTransactionMode) error {
	return tm.withTransactionContextWithLock(func(tcPtr **transactionContext) error {
		if *tcPtr == nil {
			return ErrNoTransaction
		}
		owner := *tcPtr
		if owner.hasUserWork {
			return errDdlInTransactionModeFrozen
		}
		owner.ddlInTransaction = mode
		owner.ddlInTransactionCaptured = true
		return nil
	})
}

func (tm *TransactionManager) markUserWorkLocked() {
	if tm != nil && tm.tc != nil {
		tm.tc.hasUserWork = true
	}
}

func (tm *TransactionManager) HasUserWork() bool {
	if tm == nil {
		return false
	}
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.tc != nil && tm.tc.hasUserWork
}

func (tm *TransactionManager) effectiveDdlInTransactionMode() enums.DdlInTransactionMode {
	if tm == nil {
		return enums.DdlInTransactionModeFail
	}
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	if tm.tc != nil && tm.tc.ddlInTransactionCaptured {
		return tm.tc.ddlInTransaction
	}
	if tm.sysVars != nil {
		return tm.sysVars.Transaction.DdlInTransactionMode
	}
	return enums.DdlInTransactionModeFail
}

func replayHasUserHistory(rs *replayState) bool {
	if rs == nil {
		return false
	}
	return rs.hasMarkers() || rs.needsRecovery() || len(rs.entries) > 0 || len(rs.queued) > 0
}

type ddlOwnerSnapshot struct {
	idle     bool
	pending  bool
	rw       bool
	ro       bool
	recovery bool
	empty    bool
	policy   enums.DdlInTransactionMode
}

func (tm *TransactionManager) ddlOwnerSnapshot() ddlOwnerSnapshot {
	if tm == nil {
		return ddlOwnerSnapshot{idle: true, policy: enums.DdlInTransactionModeFail}
	}
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	policy := enums.DdlInTransactionModeFail
	if tm.sysVars != nil {
		policy = tm.sysVars.Transaction.DdlInTransactionMode
	}
	if tm.tc == nil {
		return ddlOwnerSnapshot{idle: true, policy: policy}
	}
	if tm.tc.ddlInTransactionCaptured {
		policy = tm.tc.ddlInTransaction
	}
	mode := tm.tc.attrs.mode
	empty := !tm.tc.hasUserWork && len(tm.tc.autoDML) == 0 && !replayHasUserHistory(tm.tc.replay)
	return ddlOwnerSnapshot{
		pending:  mode == transactionModePending,
		rw:       mode == transactionModeReadWrite,
		ro:       mode == transactionModeReadOnly,
		recovery: tm.capturingLocked() && tm.tc.replay.needsRecovery(),
		empty:    empty,
		policy:   policy,
	}
}

// prepareDDLInTransaction applies the captured CLI_DDL_IN_TRANSACTION_MODE
// policy. Local validation (manual DML batch, RO, recovery) happens before
// any irreversible Commit or retirement. Empty BulkDdl callers must skip
// this helper so a transaction is not committed.
func prepareDDLInTransaction(ctx context.Context, session *Session) (*ddlTxnPrep, error) {
	if session == nil || session.txn == nil {
		return nil, nil
	}
	session.txn.syncExpiredOwnerRestore()
	if _, ok := session.batch.Current().(*BatchDMLStatement); ok {
		return nil, errDDLManualDMLBatch
	}
	return session.txn.applyDDLInTransactionPolicy(ctx)
}

func (tm *TransactionManager) applyDDLInTransactionPolicy(ctx context.Context) (*ddlTxnPrep, error) {
	state := tm.ddlOwnerSnapshot()
	if state.idle {
		return nil, nil
	}
	if state.ro {
		return nil, errDDLInReadOnlyTransaction
	}
	if state.recovery {
		return nil, fmt.Errorf("%w", errSavepointRecovery)
	}

	switch state.policy {
	case enums.DdlInTransactionModeFail:
		return nil, errDDLInTransaction
	case enums.DdlInTransactionModeAllowInEmptyTransaction:
		if !state.empty {
			return nil, errDDLInNonEmptyTransaction
		}
		if state.pending {
			return nil, tm.retirePendingNoop()
		}
		if state.rw {
			if err := tm.RollbackReadWriteTransaction(ctx); err != nil {
				return nil, err
			}
			tm.restoreLocalVarsIfIdle()
			return nil, nil
		}
		return nil, errDDLInTransaction
	case enums.DdlInTransactionModeAutoCommitTransaction:
		if state.pending {
			if !state.empty {
				return nil, errDDLCannotAutoCommit
			}
			return nil, tm.retirePendingNoop()
		}
		if !state.rw {
			return nil, errDDLCannotAutoCommit
		}
		resp, err := tm.CommitReadWriteTransaction(ctx)
		if err != nil {
			return nil, err
		}
		tm.restoreLocalVarsIfIdle()
		return &ddlTxnPrep{committed: true, commitTS: resp.CommitTs, resp: resp}, nil
	default:
		return nil, fmt.Errorf("unknown CLI_DDL_IN_TRANSACTION_MODE %v", state.policy)
	}
}

func (tm *TransactionManager) retirePendingNoop() error {
	err := tm.withTransactionContextWithLock(func(**transactionContext) error {
		if tm.tc == nil {
			return nil
		}
		if tm.tc.attrs.mode != transactionModePending {
			return fmt.Errorf("internal error: expected pending transaction, got %s", tm.tc.attrs.mode)
		}
		tm.retireTransactionContextLocked()
		return nil
	})
	if err != nil {
		return err
	}
	tm.restoreLocalVarsIfIdle()
	return nil
}
