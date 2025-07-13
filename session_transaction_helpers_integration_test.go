package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
)

func TestWithReadWriteTransactionIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	_, session, teardown := initialize(t, testTableDDLs, nil)
	defer teardown()

	tests := []struct {
		name        string
		setupTx     func() error
		cleanup     func()
		wantErr     error
		checkCalled bool
	}{
		{
			name:    "no transaction",
			setupTx: func() error { return nil },
			cleanup: func() {},
			wantErr: ErrNotInReadWriteTransaction,
		},
		{
			name: "valid read-write transaction",
			setupTx: func() error {
				return session.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED)
			},
			cleanup: func() {
				_ = session.RollbackReadWriteTransaction(ctx)
			},
			wantErr:     nil,
			checkCalled: true,
		},
		{
			name: "read-only transaction",
			setupTx: func() error {
				_, err := session.BeginReadOnlyTransaction(ctx, timestampBoundUnspecified, 0, time.Time{}, sppb.RequestOptions_PRIORITY_UNSPECIFIED)
				return err
			},
			cleanup: func() {
				_ = session.CloseReadOnlyTransaction()
			},
			wantErr: ErrNotInReadWriteTransaction,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.setupTx(); err != nil {
				t.Fatalf("setup failed: %v", err)
			}
			defer tt.cleanup()

			var called bool
			err := session.withReadWriteTransaction(func(tx *spanner.ReadWriteStmtBasedTransaction) error {
				called = true
				// Verify we can use the transaction
				iter := tx.Query(ctx, spanner.NewStatement("SELECT 1"))
				defer iter.Stop()
				_, err := iter.Next()
				return err
			})

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("withReadWriteTransaction() error = %v, wantErr %v", err, tt.wantErr)
			}

			if called != tt.checkCalled {
				t.Errorf("function called = %v, want %v", called, tt.checkCalled)
			}
		})
	}
}

func TestWithReadWriteTransactionContextIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	_, session, teardown := initialize(t, testTableDDLs, nil)
	defer teardown()

	// Start a read-write transaction
	err := session.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED)
	if err != nil {
		t.Fatalf("failed to begin transaction: %v", err)
	}
	defer func() {
		_ = session.RollbackReadWriteTransaction(ctx)
	}()

	// Test modifying context
	err = session.withReadWriteTransactionContext(func(tx *spanner.ReadWriteStmtBasedTransaction, tc *transactionContext) error {
		// Verify initial state
		if tc.attrs.sendHeartbeat {
			t.Error("expected sendHeartbeat to be false initially")
		}
		// Modify the context
		tc.attrs.sendHeartbeat = true
		tc.attrs.tag = "test-tag"
		return nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify modifications persisted
	attrs := session.TransactionAttrs()
	if !attrs.sendHeartbeat {
		t.Error("expected sendHeartbeat to be true after modification")
	}
	// Note: tag is not exposed through TransactionAttrs, which is correct
}

func TestWithReadOnlyTransactionIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	_, session, teardown := initialize(t, testTableDDLs, nil)
	defer teardown()

	tests := []struct {
		name        string
		setupTx     func() error
		cleanup     func()
		wantErr     error
		checkCalled bool
	}{
		{
			name:    "no transaction",
			setupTx: func() error { return nil },
			cleanup: func() {},
			wantErr: ErrNotInReadOnlyTransaction,
		},
		{
			name: "valid read-only transaction",
			setupTx: func() error {
				_, err := session.BeginReadOnlyTransaction(ctx, timestampBoundUnspecified, 0, time.Time{}, sppb.RequestOptions_PRIORITY_UNSPECIFIED)
				return err
			},
			cleanup: func() {
				_ = session.CloseReadOnlyTransaction()
			},
			wantErr:     nil,
			checkCalled: true,
		},
		{
			name: "read-write transaction",
			setupTx: func() error {
				return session.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED)
			},
			cleanup: func() {
				_ = session.RollbackReadWriteTransaction(ctx)
			},
			wantErr: ErrNotInReadOnlyTransaction,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.setupTx(); err != nil {
				t.Fatalf("setup failed: %v", err)
			}
			defer tt.cleanup()

			var called bool
			err := session.withReadOnlyTransaction(func(tx *spanner.ReadOnlyTransaction) error {
				called = true
				// Verify we can use the transaction
				iter := tx.Query(ctx, spanner.NewStatement("SELECT 1"))
				defer iter.Stop()
				_, err := iter.Next()
				return err
			})

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("withReadOnlyTransaction() error = %v, wantErr %v", err, tt.wantErr)
			}

			if called != tt.checkCalled {
				t.Errorf("function called = %v, want %v", called, tt.checkCalled)
			}
		})
	}
}

func TestTransactionHelpersConcurrencyIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	_, session, teardown := initialize(t, testTableDDLs, nil)
	defer teardown()

	// Start a read-write transaction
	err := session.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED)
	if err != nil {
		t.Fatalf("failed to begin transaction: %v", err)
	}
	defer func() {
		_ = session.RollbackReadWriteTransaction(ctx)
	}()

	// Test concurrent access
	var wg sync.WaitGroup
	errors := make(chan error, 3)

	// Goroutine 1: Read and modify sendHeartbeat
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := session.withReadWriteTransactionContext(func(tx *spanner.ReadWriteStmtBasedTransaction, tc *transactionContext) error {
			// Simulate some work
			time.Sleep(10 * time.Millisecond)
			tc.attrs.sendHeartbeat = true
			return nil
		})
		if err != nil {
			errors <- err
		}
	}()

	// Goroutine 2: Query using the transaction
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := session.withReadWriteTransaction(func(tx *spanner.ReadWriteStmtBasedTransaction) error {
			// Simulate some work
			time.Sleep(5 * time.Millisecond)
			iter := tx.Query(ctx, spanner.NewStatement("SELECT 1"))
			defer iter.Stop()
			_, err := iter.Next()
			return err
		})
		if err != nil {
			errors <- err
		}
	}()

	// Goroutine 3: Get transaction attributes
	wg.Add(1)
	go func() {
		defer wg.Done()
		// This should not block or race
		for i := 0; i < 10; i++ {
			attrs := session.TransactionAttrs()
			if attrs.mode != transactionModeReadWrite {
				errors <- fmt.Errorf("unexpected transaction mode")
				break
			}
			time.Sleep(2 * time.Millisecond)
		}
	}()

	// Wait for all goroutines
	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("concurrent operation error: %v", err)
	}

	// Verify final state
	attrs := session.TransactionAttrs()
	if !attrs.sendHeartbeat {
		t.Error("expected sendHeartbeat to be true after concurrent operations")
	}
}

func TestWithReadWriteTransactionResultIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	_, session, teardown := initialize(t, testTableDDLs, sliceOf("INSERT INTO tbl (id, active) VALUES (1, true)"))
	defer teardown()

	// Start a read-write transaction
	err := session.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED)
	if err != nil {
		t.Fatalf("failed to begin transaction: %v", err)
	}

	// Test successful operation that returns a result
	result, err := withReadWriteTransactionResult(session, func(tx *spanner.ReadWriteStmtBasedTransaction) (spanner.CommitResponse, error) {
		// Do some work in the transaction
		_, err := tx.Update(ctx, spanner.NewStatement("UPDATE tbl SET active = false WHERE id = 1"))
		if err != nil {
			return spanner.CommitResponse{}, err
		}
		// Commit and return the response
		return tx.CommitWithReturnResp(ctx)
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if result.CommitTs.IsZero() {
		t.Error("expected non-zero commit timestamp")
	}

	// Verify the transaction was cleared after commit
	if session.InTransaction() {
		t.Error("expected transaction to be cleared after commit")
	}

	// Test error propagation
	testErr := errors.New("test error")
	_, err = withReadWriteTransactionResult(session, func(tx *spanner.ReadWriteStmtBasedTransaction) (spanner.CommitResponse, error) {
		return spanner.CommitResponse{}, testErr
	})

	if !errors.Is(err, ErrNotInReadWriteTransaction) {
		t.Errorf("expected ErrNotInReadWriteTransaction when no transaction, got %v", err)
	}
}

func TestTransactionStateTransitionsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	_, session, teardown := initialize(t, testTableDDLs, nil)
	defer teardown()

	// Test state transitions with real transactions
	// 1. Initially no transaction
	if session.InTransaction() {
		t.Error("expected no transaction initially")
	}

	// 2. Begin pending transaction
	err := session.BeginPendingTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_HIGH)
	if err != nil {
		t.Fatalf("failed to begin pending transaction: %v", err)
	}

	if !session.InPendingTransaction() {
		t.Error("expected pending transaction")
	}

	// 3. Determine transaction (convert to read-write)
	_, err = session.DetermineTransaction(ctx)
	if err != nil {
		t.Fatalf("failed to determine transaction: %v", err)
	}

	if !session.InReadWriteTransaction() {
		t.Error("expected read-write transaction after determine")
	}

	// 4. Use the transaction through helpers
	var queryExecuted bool
	err = session.withReadWriteTransaction(func(tx *spanner.ReadWriteStmtBasedTransaction) error {
		queryExecuted = true
		iter := tx.Query(ctx, spanner.NewStatement("SELECT 1"))
		defer iter.Stop()
		_, err := iter.Next()
		return err
	})
	if err != nil {
		t.Errorf("unexpected error using transaction: %v", err)
	}

	if !queryExecuted {
		t.Error("expected query to be executed")
	}

	// 5. Rollback
	err = session.RollbackReadWriteTransaction(ctx)
	if err != nil {
		t.Errorf("failed to rollback: %v", err)
	}

	if session.InTransaction() {
		t.Error("expected no transaction after rollback")
	}
}
