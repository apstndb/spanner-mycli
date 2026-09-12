package mycli

import (
	"sync"
	"sync/atomic"
	"testing"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
)

// Unit tests for transaction helper functions.
// These tests focus on error handling and mutex behavior without real transactions.
// For tests with real transactions, see session_transaction_helpers_integration_test.go
//
// Note: We cannot create empty spanner.ReadOnlyTransaction or spanner.ReadWriteStmtBasedTransaction
// instances because they have unexported fields, so we test only the paths where txn is nil.

func TestTransactionAttrs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		setupTC  func() *transactionContext
		wantMode transactionMode
	}{
		{
			name:     "no transaction",
			setupTC:  func() *transactionContext { return nil },
			wantMode: transactionModeUndetermined,
		},
		{
			name: "read-write transaction",
			setupTC: func() *transactionContext {
				return &transactionContext{
					attrs: transactionAttributes{
						mode:     transactionModeReadWrite,
						priority: sppb.RequestOptions_PRIORITY_HIGH,
						tag:      "test-tag",
					},
				}
			},
			wantMode: transactionModeReadWrite,
		},
		{
			name: "read-only transaction",
			setupTC: func() *transactionContext {
				return &transactionContext{
					attrs: transactionAttributes{
						mode:     transactionModeReadOnly,
						priority: sppb.RequestOptions_PRIORITY_MEDIUM,
					},
				}
			},
			wantMode: transactionModeReadOnly,
		},
		{
			name: "pending transaction",
			setupTC: func() *transactionContext {
				return &transactionContext{
					attrs: transactionAttributes{
						mode: transactionModePending,
					},
				}
			},
			wantMode: transactionModePending,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := &TransactionManager{
				tc: tt.setupTC(),
			}

			attrs := tm.TransactionAttrsWithLock()
			if attrs.mode != tt.wantMode {
				t.Errorf("TransactionAttrs().mode = %v, want %v", attrs.mode, tt.wantMode)
			}

			// Verify that modifying the returned attrs doesn't affect the manager
			attrs.mode = "modified"
			actualAttrs := tm.TransactionAttrsWithLock()
			if actualAttrs.mode == "modified" {
				t.Error("modifying returned attrs affected the manager state")
			}
		})
	}
}

func TestBeginPendingTransactionResolvesOptions(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name          string
		sysVars       *systemVariables
		priority      sppb.RequestOptions_Priority
		isolation     sppb.TransactionOptions_IsolationLevel
		wantPriority  sppb.RequestOptions_Priority
		wantIsolation sppb.TransactionOptions_IsolationLevel
	}{
		{
			name:          "explicit values do not require settings",
			priority:      sppb.RequestOptions_PRIORITY_LOW,
			isolation:     sppb.TransactionOptions_REPEATABLE_READ,
			wantPriority:  sppb.RequestOptions_PRIORITY_LOW,
			wantIsolation: sppb.TransactionOptions_REPEATABLE_READ,
		},
		{
			name: "unspecified values use settings",
			sysVars: &systemVariables{
				Query:       QueryVars{RPCPriority: sppb.RequestOptions_PRIORITY_HIGH},
				Transaction: TransactionVars{DefaultIsolationLevel: sppb.TransactionOptions_SERIALIZABLE},
			},
			priority:      sppb.RequestOptions_PRIORITY_UNSPECIFIED,
			isolation:     sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED,
			wantPriority:  sppb.RequestOptions_PRIORITY_HIGH,
			wantIsolation: sppb.TransactionOptions_SERIALIZABLE,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tm := &TransactionManager{sysVars: tt.sysVars}
			if err := tm.BeginPendingTransaction(t.Context(), tt.isolation, tt.priority); err != nil {
				t.Fatalf("BeginPendingTransaction() error = %v", err)
			}

			attrs := tm.TransactionAttrsWithLock()
			if attrs.mode != transactionModePending {
				t.Errorf("TransactionAttrsWithLock().mode = %q, want %q", attrs.mode, transactionModePending)
			}
			if attrs.priority != tt.wantPriority {
				t.Errorf("TransactionAttrsWithLock().priority = %v, want %v", attrs.priority, tt.wantPriority)
			}
			if attrs.isolationLevel != tt.wantIsolation {
				t.Errorf("TransactionAttrsWithLock().isolationLevel = %v, want %v", attrs.isolationLevel, tt.wantIsolation)
			}
		})
	}
}

func TestClearTransactionContext(t *testing.T) {
	t.Parallel()
	tm := &TransactionManager{
		tc: &transactionContext{
			attrs: transactionAttributes{
				mode: transactionModeReadWrite,
			},
			// txn would be a real transaction in production
		},
	}

	// Verify transaction exists
	if !tm.InTransaction() {
		t.Error("expected transaction to exist before clear")
	}

	// Clear the transaction
	tm.clearTransactionContext()

	// Verify transaction is cleared
	if tm.InTransaction() {
		t.Error("expected transaction to be cleared")
	}

	// Verify tc is nil
	if tm.tc != nil {
		t.Error("expected tc to be nil after clear")
	}

	// Verify multiple clears are safe
	tm.clearTransactionContext()
	if tm.tc != nil {
		t.Error("expected tc to remain nil after second clear")
	}
}

func TestTransactionStateHelpers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name              string
		setupTC           func() *transactionContext
		wantInTransaction bool
		wantInReadWrite   bool
		wantInReadOnly    bool
		wantInPending     bool
	}{
		{
			name:              "no transaction",
			setupTC:           func() *transactionContext { return nil },
			wantInTransaction: false,
			wantInReadWrite:   false,
			wantInReadOnly:    false,
			wantInPending:     false,
		},
		{
			name: "read-write transaction",
			setupTC: func() *transactionContext {
				return &transactionContext{
					attrs: transactionAttributes{mode: transactionModeReadWrite},
				}
			},
			wantInTransaction: true,
			wantInReadWrite:   true,
			wantInReadOnly:    false,
			wantInPending:     false,
		},
		{
			name: "read-only transaction",
			setupTC: func() *transactionContext {
				return &transactionContext{
					attrs: transactionAttributes{mode: transactionModeReadOnly},
				}
			},
			wantInTransaction: true,
			wantInReadWrite:   false,
			wantInReadOnly:    true,
			wantInPending:     false,
		},
		{
			name: "pending transaction",
			setupTC: func() *transactionContext {
				return &transactionContext{
					attrs: transactionAttributes{mode: transactionModePending},
				}
			},
			wantInTransaction: true,
			wantInReadWrite:   false,
			wantInReadOnly:    false,
			wantInPending:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := &TransactionManager{
				tc: tt.setupTC(),
			}

			if got := tm.InTransaction(); got != tt.wantInTransaction {
				t.Errorf("InTransaction() = %v, want %v", got, tt.wantInTransaction)
			}
			if got := tm.InReadWriteTransaction(); got != tt.wantInReadWrite {
				t.Errorf("InReadWriteTransaction() = %v, want %v", got, tt.wantInReadWrite)
			}
			if got := tm.InReadOnlyTransaction(); got != tt.wantInReadOnly {
				t.Errorf("InReadOnlyTransaction() = %v, want %v", got, tt.wantInReadOnly)
			}
			if got := tm.InPendingTransaction(); got != tt.wantInPending {
				t.Errorf("InPendingTransaction() = %v, want %v", got, tt.wantInPending)
			}
		})
	}
}

func TestTransactionHelperErrorHandling(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		testType string // "readwrite" or "readonly"
		setupTC  func() *transactionContext
		wantErr  error
	}{
		// Read-write transaction tests
		{
			name:     "readwrite/no transaction context",
			testType: "readwrite",
			setupTC:  func() *transactionContext { return nil },
			wantErr:  ErrNotInReadWriteTransaction,
		},
		{
			name:     "readwrite/wrong mode - read-only",
			testType: "readwrite",
			setupTC: func() *transactionContext {
				return &transactionContext{
					attrs: transactionAttributes{mode: transactionModeReadOnly},
				}
			},
			wantErr: ErrNotInReadWriteTransaction,
		},
		{
			name:     "readwrite/wrong mode - pending",
			testType: "readwrite",
			setupTC: func() *transactionContext {
				return &transactionContext{
					attrs: transactionAttributes{mode: transactionModePending},
				}
			},
			wantErr: ErrNotInReadWriteTransaction,
		},
		// Read-only transaction tests
		{
			name:     "readonly/no transaction context",
			testType: "readonly",
			setupTC:  func() *transactionContext { return nil },
			wantErr:  ErrNotInReadOnlyTransaction,
		},
		{
			name:     "readonly/wrong mode - read-write",
			testType: "readonly",
			setupTC: func() *transactionContext {
				return &transactionContext{
					attrs: transactionAttributes{mode: transactionModeReadWrite},
				}
			},
			wantErr: ErrNotInReadOnlyTransaction,
		},
		{
			name:     "readonly/wrong mode - pending",
			testType: "readonly",
			setupTC: func() *transactionContext {
				return &transactionContext{
					attrs: transactionAttributes{mode: transactionModePending},
				}
			},
			wantErr: ErrNotInReadOnlyTransaction,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := &TransactionManager{
				tc: tt.setupTC(),
			}

			var err error
			switch tt.testType {
			case "readwrite":
				err = tm.withReadWriteTransaction(func(tx *spanner.ReadWriteStmtBasedTransaction) error {
					t.Fatal("function should not be called")
					return nil
				})
			case "readonly":
				err = tm.withReadOnlyTransaction(func(tx *spanner.ReadOnlyTransaction) error {
					t.Fatal("function should not be called")
					return nil
				})
			default:
				t.Fatalf("unknown test type: %s", tt.testType)
			}

			if err != tt.wantErr {
				t.Errorf("%s helper error = %v, wantErr %v", tt.testType, err, tt.wantErr)
			}
		})
	}
}

func TestBeginPendingTransactionRejectsExistingContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setupTC func(closed *bool) *transactionContext
		wantErr string
	}{
		{
			name:    "idle to pending succeeds",
			setupTC: func(*bool) *transactionContext { return nil },
		},
		{
			name: "pending to pending fails without replacing",
			setupTC: func(closed *bool) *transactionContext {
				return &transactionContext{
					attrs: transactionAttributes{
						mode:           transactionModePending,
						tag:            "pending-tag",
						priority:       sppb.RequestOptions_PRIORITY_LOW,
						isolationLevel: sppb.TransactionOptions_REPEATABLE_READ,
					},
					heartbeatCancel: func() { *closed = true },
				}
			},
			wantErr: "pending transaction is already running",
		},
		{
			name: "read-write to pending fails without replacing or closing",
			setupTC: func(closed *bool) *transactionContext {
				return &transactionContext{
					attrs: transactionAttributes{
						mode:           transactionModeReadWrite,
						tag:            "rw-tag",
						priority:       sppb.RequestOptions_PRIORITY_HIGH,
						isolationLevel: sppb.TransactionOptions_SERIALIZABLE,
					},
					heartbeatCancel: func() { *closed = true },
				}
			},
			wantErr: "read-write transaction is already running",
		},
		{
			name: "read-only to pending fails without replacing or closing",
			setupTC: func(closed *bool) *transactionContext {
				return &transactionContext{
					attrs: transactionAttributes{
						mode:           transactionModeReadOnly,
						tag:            "ro-tag",
						priority:       sppb.RequestOptions_PRIORITY_MEDIUM,
						isolationLevel: sppb.TransactionOptions_SERIALIZABLE,
					},
					heartbeatCancel: func() { *closed = true },
				}
			},
			wantErr: "read-only transaction is already running",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var closed bool
			existing := tt.setupTC(&closed)
			tm := &TransactionManager{tc: existing}
			err := tm.BeginPendingTransaction(t.Context(), sppb.TransactionOptions_SERIALIZABLE, sppb.RequestOptions_PRIORITY_HIGH)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("BeginPendingTransaction() error = %v", err)
				}
				attrs := tm.TransactionAttrsWithLock()
				if attrs.mode != transactionModePending {
					t.Errorf("mode = %q, want pending", attrs.mode)
				}
				if attrs.priority != sppb.RequestOptions_PRIORITY_HIGH {
					t.Errorf("priority = %v, want HIGH", attrs.priority)
				}
				if attrs.isolationLevel != sppb.TransactionOptions_SERIALIZABLE {
					t.Errorf("isolation = %v, want SERIALIZABLE", attrs.isolationLevel)
				}
				return
			}
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("error = %v, want %q", err, tt.wantErr)
			}
			if tm.tc != existing {
				t.Fatal("existing transaction context was replaced")
			}
			if closed {
				t.Error("existing transaction context was closed")
			}
			if existing != nil {
				if tm.tc.attrs.tag != existing.attrs.tag ||
					tm.tc.attrs.priority != existing.attrs.priority ||
					tm.tc.attrs.isolationLevel != existing.attrs.isolationLevel ||
					tm.tc.attrs.mode != existing.attrs.mode {
					t.Errorf("existing options/tags changed: %+v", tm.tc.attrs)
				}
			}
		})
	}
}

func TestBeginPendingTransactionConcurrent(t *testing.T) {
	t.Parallel()

	tm := &TransactionManager{}
	var success atomic.Int32
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			if err := tm.BeginPendingTransaction(t.Context(), sppb.TransactionOptions_SERIALIZABLE, sppb.RequestOptions_PRIORITY_HIGH); err == nil {
				success.Add(1)
			}
		})
	}
	wg.Wait()
	if got := success.Load(); got != 1 {
		t.Fatalf("concurrent BEGIN published %d contexts, want 1", got)
	}
	if attrs := tm.TransactionAttrsWithLock(); attrs.mode != transactionModePending {
		t.Errorf("mode = %q, want pending", attrs.mode)
	}
}

func TestSetClient(t *testing.T) {
	t.Parallel()

	oldClient := &spanner.Client{}
	newClient := &spanner.Client{}

	tests := []struct {
		name    string
		tc      *transactionContext
		wantErr bool
	}{
		{
			name:    "idle swaps successfully",
			tc:      nil,
			wantErr: false,
		},
		{
			name:    "refuses during active read-write transaction",
			tc:      &transactionContext{attrs: transactionAttributes{mode: transactionModeReadWrite}},
			wantErr: true,
		},
		{
			name:    "refuses during active read-only transaction",
			tc:      &transactionContext{attrs: transactionAttributes{mode: transactionModeReadOnly}},
			wantErr: true,
		},
		{
			name:    "refuses during pending transaction",
			tc:      &transactionContext{attrs: transactionAttributes{mode: transactionModePending}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := &TransactionManager{
				tc:     tt.tc,
				client: oldClient,
			}

			err := tm.SetClient(newClient)
			if (err != nil) != tt.wantErr {
				t.Fatalf("SetClient() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr {
				// On refusal the client must be left untouched so the caller can
				// close the new client without orphaning the live transaction.
				if tm.client != oldClient {
					t.Errorf("SetClient() replaced client on refusal: got %p, want %p", tm.client, oldClient)
				}
			} else {
				if tm.client != newClient {
					t.Errorf("SetClient() did not swap client: got %p, want %p", tm.client, newClient)
				}
			}
		})
	}
}
