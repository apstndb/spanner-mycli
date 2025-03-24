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

	return &Result{IsMutation: true}, nil
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
			IsMutation: true,
			Timestamp:  ts,
		}, nil
	}

	err := session.BeginPendingTransaction(ctx, s.IsolationLevel, s.Priority)
	if err != nil {
		return nil, err
	}

	return &Result{IsMutation: true}, nil
}

type SetTransactionStatement struct {
	IsReadOnly bool
}

func (s *SetTransactionStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	result := &Result{IsMutation: true}
	if !session.InPendingTransaction() {
		// nop
		return result, nil
	}

	if s.IsReadOnly {
		ts, err := session.BeginReadOnlyTransaction(ctx, timestampBoundUnspecified, 0, time.Time{}, session.tc.priority)
		if err != nil {
			return nil, err
		}
		result.Timestamp = ts
		return result, nil
	} else {
		err := session.BeginReadWriteTransaction(ctx, 0, session.tc.priority)
		if err != nil {
			return nil, err
		}
		return result, nil
	}
}

type CommitStatement struct{}

func (s *CommitStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	result := &Result{IsMutation: true}
	switch {
	case session.InPendingTransaction():
		if err := session.ClosePendingTransaction(); err != nil {
			return nil, err
		}

		return result, nil
	case !session.InReadWriteTransaction() && !session.InReadOnlyTransaction():
		return result, nil
	case session.InReadOnlyTransaction():
		if err := session.CloseReadOnlyTransaction(); err != nil {
			return nil, err
		}

		return result, nil
	case session.InReadWriteTransaction():
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

		result.Timestamp = resp.CommitTs
		result.CommitStats = resp.CommitStats
		return result, nil
	default:
		return nil, errors.New("invalid state")
	}
}

type RollbackStatement struct{}

func (s *RollbackStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	result := &Result{IsMutation: true}
	switch {
	case session.InPendingTransaction():
		if err := session.ClosePendingTransaction(); err != nil {
			return nil, err
		}

		return result, nil
	case !session.InReadWriteTransaction() && !session.InReadOnlyTransaction():
		return result, nil
	case session.InReadOnlyTransaction():
		if err := session.CloseReadOnlyTransaction(); err != nil {
			return nil, err
		}

		return result, nil
	case session.InReadWriteTransaction():
		if err := session.RollbackReadWriteTransaction(ctx); err != nil {
			return nil, err
		}

		return result, nil
	default:
		return nil, errors.New("invalid state")
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
		IsMutation: true,
		Timestamp:  ts,
	}, nil
}
