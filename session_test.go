package main

import (
	"context"
	_ "embed"
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

	s.tc = &transactionContext{attrs: transactionAttributes{mode: transactionModeReadWrite}}
	if got := s.TransactionMode(); got != transactionModeReadWrite {
		t.Errorf("Session with read-write transaction should return read-write mode, got %v", got)
	}

	s.tc = &transactionContext{attrs: transactionAttributes{mode: transactionModeReadOnly}}
	if got := s.TransactionMode(); got != transactionModeReadOnly {
		t.Errorf("Session with read-only transaction should return read-only mode, got %v", got)
	}

	s.tc = &transactionContext{attrs: transactionAttributes{mode: transactionModePending}}
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
		name     string
		tc       *transactionContext
		method   string
		wantErr  bool
		errCheck func(error) bool
	}{
		{
			name:    "RWTxn with nil context",
			tc:      nil,
			method:  "RWTxn",
			wantErr: true,
			errCheck: func(err error) bool {
				return errors.Is(err, ErrTransactionNotAvailable)
			},
		},
		{
			name: "RWTxn with nil txn",
			tc: &transactionContext{
				attrs: transactionAttributes{mode: transactionModeReadWrite},
				txn:   nil,
			},
			method:  "RWTxn",
			wantErr: true,
			errCheck: func(err error) bool {
				return errors.Is(err, ErrTransactionNotAvailable)
			},
		},
		{
			name: "RWTxn with wrong mode",
			tc: &transactionContext{
				attrs: transactionAttributes{mode: transactionModeReadOnly},
				txn:   &mockTransaction{},
			},
			method:  "RWTxn",
			wantErr: true,
			errCheck: func(err error) bool {
				return errors.Is(err, ErrNotInReadWriteTransaction)
			},
		},
		{
			name:    "ROTxn with nil context",
			tc:      nil,
			method:  "ROTxn",
			wantErr: true,
			errCheck: func(err error) bool {
				return errors.Is(err, ErrTransactionNotAvailable)
			},
		},
		{
			name: "ROTxn with nil txn",
			tc: &transactionContext{
				attrs: transactionAttributes{mode: transactionModeReadOnly},
				txn:   nil,
			},
			method:  "ROTxn",
			wantErr: true,
			errCheck: func(err error) bool {
				return errors.Is(err, ErrTransactionNotAvailable)
			},
		},
		{
			name: "ROTxn with wrong mode",
			tc: &transactionContext{
				attrs: transactionAttributes{mode: transactionModeReadWrite},
				txn:   &mockTransaction{},
			},
			method:  "ROTxn",
			wantErr: true,
			errCheck: func(err error) bool {
				return errors.Is(err, ErrNotInReadOnlyTransaction)
			},
		},
		{
			name:    "Txn with nil context",
			tc:      nil,
			method:  "Txn",
			wantErr: true,
			errCheck: func(err error) bool {
				return errors.Is(err, ErrTransactionNotAvailable)
			},
		},
		{
			name: "Txn with nil txn",
			tc: &transactionContext{
				attrs: transactionAttributes{mode: transactionModeReadWrite},
				txn:   nil,
			},
			method:  "Txn",
			wantErr: true,
			errCheck: func(err error) bool {
				return errors.Is(err, ErrTransactionNotAvailable)
			},
		},
		{
			name: "Txn with wrong mode",
			tc: &transactionContext{
				attrs: transactionAttributes{mode: transactionModeUndetermined},
				txn:   &mockTransaction{},
			},
			method:  "Txn",
			wantErr: true,
			errCheck: func(err error) bool {
				return errors.Is(err, ErrInvalidTransactionMode)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			switch tt.method {
			case "RWTxn":
				_, err = tt.tc.RWTxn()
			case "ROTxn":
				_, err = tt.tc.ROTxn()
			case "Txn":
				_, err = tt.tc.Txn()
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("Method %s error = %v, wantErr %v", tt.method, err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errCheck != nil && !tt.errCheck(err) {
				t.Errorf("Method %s returned wrong error: %v", tt.method, err)
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
