package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
)

type timestampBoundType int

const (
	timestampBoundUnspecified timestampBoundType = iota
	strong
	exactStaleness
	readTimestamp
)

type BeginRoStatement struct {
	TimestampBoundType timestampBoundType
	Staleness          time.Duration
	Timestamp          time.Time
	Priority           sppb.RequestOptions_Priority
}

type BeginRwStatement struct {
	IsolationLevel sppb.TransactionOptions_IsolationLevel
	Priority       sppb.RequestOptions_Priority
}

func (BeginRwStatement) isMutationStatement() {}

func (s *BeginRwStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if session.InReadWriteTransaction() {
		return nil, errors.New("you're in read-write transaction. Please finish the transaction by 'COMMIT;' or 'ROLLBACK;'")
	}

	if session.InReadOnlyTransaction() {
		return nil, errors.New("you're in read-only transaction. Please finish the transaction by 'CLOSE;'")
	}

	if err := session.BeginReadWriteTransaction(ctx, s.IsolationLevel, s.Priority); err != nil {
		return nil, err
	}

	return &Result{}, nil
}

type BeginStatement struct {
	IsolationLevel sppb.TransactionOptions_IsolationLevel
	Priority       sppb.RequestOptions_Priority
}

func (s *BeginStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if session.InTransaction() {
		return nil, errors.New("you're in transaction. Please finish the transaction by 'COMMIT;' or 'ROLLBACK;'")
	}

	if session.systemVariables.ReadOnly {
		ts, err := session.BeginReadOnlyTransaction(ctx, timestampBoundUnspecified, 0, time.Time{}, s.Priority)
		if err != nil {
			return nil, err
		}

		return &Result{
			ReadTimestamp: ts,
		}, nil
	}

	err := session.BeginPendingTransaction(ctx, s.IsolationLevel, s.Priority)
	if err != nil {
		return nil, err
	}

	return &Result{}, nil
}

type SetTransactionStatement struct {
	IsReadOnly bool
}

func (s *SetTransactionStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	result := &Result{}

	// Get transaction attributes atomically to avoid check-then-act race
	attrs := session.TransactionAttrsWithLock()
	if attrs.mode != transactionModePending {
		// nop - not in pending transaction
		return result, nil
	}

	if s.IsReadOnly {
		ts, err := session.BeginReadOnlyTransaction(ctx, timestampBoundUnspecified, 0, time.Time{}, attrs.priority)
		if err != nil {
			return nil, err
		}
		result.ReadTimestamp = ts
		return result, nil
	} else {
		err := session.BeginReadWriteTransaction(ctx, attrs.isolationLevel, attrs.priority)
		if err != nil {
			return nil, err
		}
		return result, nil
	}
}

type CommitStatement struct{}

func (s *CommitStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	result := &Result{}

	// Get transaction state once to avoid multiple mutex acquisitions
	mode, isActive := session.TransactionState()

	// Handle based on transaction mode
	switch mode {
	case transactionModePending:
		if err := session.ClosePendingTransaction(); err != nil {
			return nil, err
		}
		return result, nil

	case transactionModeReadOnly:
		if err := session.CloseReadOnlyTransaction(); err != nil {
			return nil, err
		}
		return result, nil

	case transactionModeReadWrite:
		if session.systemVariables.AutoBatchDML && session.currentBatch != nil {
			var err error
			result, err = runBatch(ctx, session)
			if err != nil {
				return nil, err
			}
		}

		resp, err := session.CommitReadWriteTransaction(ctx)
		if err != nil {
			return nil, err
		}

		result.CommitTimestamp = resp.CommitTs
		result.CommitStats = resp.CommitStats
		return result, nil

	default:
		// No active transaction - this is a no-op
		if !isActive {
			return result, nil
		}
		return nil, fmt.Errorf("invalid transaction state: %v", mode)
	}
}

type RollbackStatement struct{}

func (s *RollbackStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	result := &Result{}

	// Get transaction state once to avoid multiple mutex acquisitions
	mode, isActive := session.TransactionState()

	// Handle based on transaction mode
	switch mode {
	case transactionModePending:
		if err := session.ClosePendingTransaction(); err != nil {
			return nil, err
		}
		return result, nil

	case transactionModeReadOnly:
		if err := session.CloseReadOnlyTransaction(); err != nil {
			return nil, err
		}
		return result, nil

	case transactionModeReadWrite:
		if err := session.RollbackReadWriteTransaction(ctx); err != nil {
			return nil, err
		}
		return result, nil

	default:
		// No active transaction - this is a no-op
		if !isActive {
			return result, nil
		}
		return nil, fmt.Errorf("invalid transaction state: %v", mode)
	}
}

func (s *BeginRoStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if session.InReadWriteTransaction() {
		return nil, errors.New("invalid state: You're in read-write transaction. Please finish the transaction by 'COMMIT;' or 'ROLLBACK;'")
	}

	if session.InReadOnlyTransaction() {
		// close current transaction implicitly
		if _, err := (&RollbackStatement{}).Execute(ctx, session); err != nil {
			return nil, fmt.Errorf("error on close current transaction: %w", err)
		}
	}

	ts, err := session.BeginReadOnlyTransaction(ctx, s.TimestampBoundType, s.Staleness, s.Timestamp, s.Priority)
	if err != nil {
		return nil, err
	}

	return &Result{
		ReadTimestamp: ts,
	}, nil
}
