package main

import (
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
			s := &Session{
				tc: tt.setupTC(),
			}

			attrs := s.TransactionAttrs()
			if attrs.mode != tt.wantMode {
				t.Errorf("TransactionAttrs().mode = %v, want %v", attrs.mode, tt.wantMode)
			}

			// Verify that modifying the returned attrs doesn't affect the session
			attrs.mode = "modified"
			actualAttrs := s.TransactionAttrs()
			if actualAttrs.mode == "modified" {
				t.Error("modifying returned attrs affected the session state")
			}
		})
	}
}

func TestClearTransactionContext(t *testing.T) {
	s := &Session{
		tc: &transactionContext{
			attrs: transactionAttributes{
				mode: transactionModeReadWrite,
			},
			// txn would be a real transaction in production
		},
	}

	// Verify transaction exists
	if !s.InTransaction() {
		t.Error("expected transaction to exist before clear")
	}

	// Clear the transaction
	s.clearTransactionContext()

	// Verify transaction is cleared
	if s.InTransaction() {
		t.Error("expected transaction to be cleared")
	}

	// Verify tc is nil
	if s.tc != nil {
		t.Error("expected tc to be nil after clear")
	}

	// Verify multiple clears are safe
	s.clearTransactionContext()
	if s.tc != nil {
		t.Error("expected tc to remain nil after second clear")
	}
}

func TestTransactionStateHelpers(t *testing.T) {
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
			s := &Session{
				tc: tt.setupTC(),
			}

			if got := s.InTransaction(); got != tt.wantInTransaction {
				t.Errorf("InTransaction() = %v, want %v", got, tt.wantInTransaction)
			}
			if got := s.InReadWriteTransaction(); got != tt.wantInReadWrite {
				t.Errorf("InReadWriteTransaction() = %v, want %v", got, tt.wantInReadWrite)
			}
			if got := s.InReadOnlyTransaction(); got != tt.wantInReadOnly {
				t.Errorf("InReadOnlyTransaction() = %v, want %v", got, tt.wantInReadOnly)
			}
			if got := s.InPendingTransaction(); got != tt.wantInPending {
				t.Errorf("InPendingTransaction() = %v, want %v", got, tt.wantInPending)
			}
		})
	}
}

func TestTransactionHelperErrorHandling(t *testing.T) {
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
			s := &Session{
				tc: tt.setupTC(),
			}

			var err error
			switch tt.testType {
			case "readwrite":
				err = s.withReadWriteTransaction(func(tx *spanner.ReadWriteStmtBasedTransaction) error {
					t.Fatal("function should not be called")
					return nil
				})
			case "readonly":
				err = s.withReadOnlyTransaction(func(tx *spanner.ReadOnlyTransaction) error {
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

func TestValidateNoActiveTransactionLocked(t *testing.T) {
	tests := []struct {
		name    string
		setupTC func() *transactionContext
		wantErr bool
	}{
		{
			name:    "no transaction",
			setupTC: func() *transactionContext { return nil },
			wantErr: false,
		},
		{
			name: "pending transaction",
			setupTC: func() *transactionContext {
				return &transactionContext{
					attrs: transactionAttributes{mode: transactionModePending},
				}
			},
			wantErr: false, // Pending transactions are allowed
		},
		{
			name: "read-write transaction",
			setupTC: func() *transactionContext {
				return &transactionContext{
					attrs: transactionAttributes{mode: transactionModeReadWrite},
				}
			},
			wantErr: true,
		},
		{
			name: "read-only transaction",
			setupTC: func() *transactionContext {
				return &transactionContext{
					attrs: transactionAttributes{mode: transactionModeReadOnly},
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Session{
				tc: tt.setupTC(),
			}

			// Note: This test doesn't acquire the mutex as the function expects
			// the caller to hold it. In production, this is always called with mutex held.
			err := s.validateNoActiveTransactionLocked()
			if (err != nil) != tt.wantErr {
				t.Errorf("validateNoActiveTransactionLocked() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
