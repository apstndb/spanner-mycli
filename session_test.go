package main

import (
	_ "embed"
	"context"
	"errors"
	"testing"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestParseDirectedReadOption(t *testing.T) {
	for _, tt := range []struct {
		desc   string
		option string
		want   *sppb.DirectedReadOptions
	}{
		{
			desc:   "use directed read location option only",
			option: "us-central1",
			want: &sppb.DirectedReadOptions{
				Replicas: &sppb.DirectedReadOptions_IncludeReplicas_{
					IncludeReplicas: &sppb.DirectedReadOptions_IncludeReplicas{
						ReplicaSelections: []*sppb.DirectedReadOptions_ReplicaSelection{
							{
								Location: "us-central1",
							},
						},
						AutoFailoverDisabled: true,
					},
				},
			},
		},
		{
			desc:   "use directed read location and type option (READ_ONLY)",
			option: "us-central1:READ_ONLY",
			want: &sppb.DirectedReadOptions{
				Replicas: &sppb.DirectedReadOptions_IncludeReplicas_{
					IncludeReplicas: &sppb.DirectedReadOptions_IncludeReplicas{
						ReplicaSelections: []*sppb.DirectedReadOptions_ReplicaSelection{
							{
								Location: "us-central1",
								Type:     sppb.DirectedReadOptions_ReplicaSelection_READ_ONLY,
							},
						},
						AutoFailoverDisabled: true,
					},
				},
			},
		},
		{
			desc:   "use directed read location and type option (READ_WRITE)",
			option: "us-central1:READ_WRITE",
			want: &sppb.DirectedReadOptions{
				Replicas: &sppb.DirectedReadOptions_IncludeReplicas_{
					IncludeReplicas: &sppb.DirectedReadOptions_IncludeReplicas{
						ReplicaSelections: []*sppb.DirectedReadOptions_ReplicaSelection{
							{
								Location: "us-central1",
								Type:     sppb.DirectedReadOptions_ReplicaSelection_READ_WRITE,
							},
						},
						AutoFailoverDisabled: true,
					},
				},
			},
		},
		{
			desc:   "use invalid type option",
			option: "us-central1:READONLY",
			want:   nil,
		},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			got, _ := parseDirectedReadOption(tt.option)

			if !cmp.Equal(got, tt.want, protocmp.Transform()) {
				t.Errorf("Got = %v, but want = %v", got, tt.want)
			}
		})
	}
}

func TestSession_TransactionMode(t *testing.T) {
	s := &Session{}

	if got := s.TransactionMode(); got != transactionModeUndetermined {
		t.Errorf("New session should have undetermined transaction mode, got %v", got)
	}

	s.tc = &transactionContext{mode: transactionModeReadWrite}
	if got := s.TransactionMode(); got != transactionModeReadWrite {
		t.Errorf("Session with read-write transaction should return read-write mode, got %v", got)
	}

	s.tc = &transactionContext{mode: transactionModeReadOnly}
	if got := s.TransactionMode(); got != transactionModeReadOnly {
		t.Errorf("Session with read-only transaction should return read-only mode, got %v", got)
	}

	s.tc = &transactionContext{mode: transactionModePending}
	if got := s.TransactionMode(); got != transactionModePending {
		t.Errorf("Session with pending transaction should return pending mode, got %v", got)
	}
}

func TestSession_FailStatementIfReadOnly(t *testing.T) {
	s := &Session{systemVariables: &systemVariables{ReadOnly: true}}
	err := s.failStatementIfReadOnly()
	if err == nil {
		t.Errorf("failStatementIfReadOnly should return an error when ReadOnly is true")
	}
	if !errors.Is(err, errReadOnly) {
		t.Errorf("failStatementIfReadOnly should return specific error, got %v", err)
	}

	s = &Session{systemVariables: &systemVariables{ReadOnly: false}}
	err = s.failStatementIfReadOnly()
	if err != nil {
		t.Errorf("failStatementIfReadOnly should not return an error when ReadOnly is false")
	}
}

func TestTransactionContext_NilChecks(t *testing.T) {
	tests := []struct {
		name         string
		tc           *transactionContext
		method       string
		shouldPanic  bool
		panicMessage string
	}{
		{
			name:         "RWTxn with nil context",
			tc:           nil,
			method:       "RWTxn",
			shouldPanic:  true,
			panicMessage: "read-write transaction is not available",
		},
		{
			name: "RWTxn with nil txn",
			tc: &transactionContext{
				mode: transactionModeReadWrite,
				txn:  nil,
			},
			method:       "RWTxn",
			shouldPanic:  true,
			panicMessage: "read-write transaction is not available",
		},
		{
			name: "RWTxn with wrong mode",
			tc: &transactionContext{
				mode: transactionModeReadOnly,
				txn:  &mockTransaction{},
			},
			method:       "RWTxn",
			shouldPanic:  true,
			panicMessage: "must be in read-write transaction, but: read-only",
		},
		{
			name:         "ROTxn with nil context",
			tc:           nil,
			method:       "ROTxn",
			shouldPanic:  true,
			panicMessage: "read-only transaction is not available",
		},
		{
			name: "ROTxn with nil txn",
			tc: &transactionContext{
				mode: transactionModeReadOnly,
				txn:  nil,
			},
			method:       "ROTxn",
			shouldPanic:  true,
			panicMessage: "read-only transaction is not available",
		},
		{
			name: "ROTxn with wrong mode",
			tc: &transactionContext{
				mode: transactionModeReadWrite,
				txn:  &mockTransaction{},
			},
			method:       "ROTxn",
			shouldPanic:  true,
			panicMessage: "must be in read-only transaction, but: read-write",
		},
		{
			name:         "Txn with nil context",
			tc:           nil,
			method:       "Txn",
			shouldPanic:  true,
			panicMessage: "transaction is not available",
		},
		{
			name: "Txn with nil txn",
			tc: &transactionContext{
				mode: transactionModeReadWrite,
				txn:  nil,
			},
			method:       "Txn",
			shouldPanic:  true,
			panicMessage: "transaction is not available",
		},
		{
			name: "Txn with wrong mode",
			tc: &transactionContext{
				mode: transactionModeUndetermined,
				txn:  &mockTransaction{},
			},
			method:       "Txn",
			shouldPanic:  true,
			panicMessage: "must be in transaction, but: ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					if !tt.shouldPanic {
						t.Errorf("Method %s panicked unexpectedly: %v", tt.method, r)
					} else if tt.panicMessage != "" && r != tt.panicMessage {
						t.Errorf("Method %s panicked with wrong message. Expected: %q, got: %v", tt.method, tt.panicMessage, r)
					}
				} else if tt.shouldPanic {
					t.Errorf("Method %s should have panicked but didn't", tt.method)
				}
			}()

			switch tt.method {
			case "RWTxn":
				tt.tc.RWTxn()
			case "ROTxn":
				tt.tc.ROTxn()
			case "Txn":
				tt.tc.Txn()
			}
		})
	}
}

// mockTransaction implements the transaction interface for testing
type mockTransaction struct{}

func (m *mockTransaction) QueryWithOptions(ctx context.Context, statement spanner.Statement, opts spanner.QueryOptions) *spanner.RowIterator {
	return nil
}

func (m *mockTransaction) Query(ctx context.Context, statement spanner.Statement) *spanner.RowIterator {
	return nil
}
