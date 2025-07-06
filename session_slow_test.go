//go:build !skip_slow_test

package main

import (
	"context"
	"testing"
	"time"

	"github.com/apstndb/spanemuboost"
	"github.com/samber/lo"
	"google.golang.org/grpc/credentials/insecure"

	"cloud.google.com/go/spanner"
	"google.golang.org/api/option"
	"google.golang.org/grpc"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
)

const (
	project  = "project"
	instance = "instance"
	database = "database"
)

func TestRequestPriority(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping emulator test in short mode")
	}

	ctx := t.Context()

	emulator, teardown, err := spanemuboost.NewEmulator(ctx,
		spanemuboost.WithProjectID(project),
		spanemuboost.WithInstanceID(instance),
		spanemuboost.WithDatabaseID(database),
		spanemuboost.WithSetupDDLs(sliceOf("CREATE TABLE t1 (Id INT64) PRIMARY KEY (Id)")),
	)
	if err != nil {
		t.Fatalf("failed to start emulator: %v", err)
	}
	defer teardown()

	var recorder requestRecorder
	unaryInterceptor, streamInterceptor := recordRequestsInterceptors(&recorder)
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(unaryInterceptor),
		grpc.WithStreamInterceptor(streamInterceptor),
	}

	conn, err := grpc.NewClient(emulator.URI(), opts...)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}

	for _, test := range []struct {
		desc                string
		sessionPriority     sppb.RequestOptions_Priority
		transactionPriority sppb.RequestOptions_Priority
		want                sppb.RequestOptions_Priority
	}{
		{
			desc:                "use default priority",
			sessionPriority:     sppb.RequestOptions_PRIORITY_UNSPECIFIED,
			transactionPriority: sppb.RequestOptions_PRIORITY_UNSPECIFIED,
			want:                sppb.RequestOptions_PRIORITY_UNSPECIFIED,
		},
		{
			desc:                "use session priority",
			sessionPriority:     sppb.RequestOptions_PRIORITY_LOW,
			transactionPriority: sppb.RequestOptions_PRIORITY_UNSPECIFIED,
			want:                sppb.RequestOptions_PRIORITY_LOW,
		},
		{
			desc:                "use transaction priority",
			sessionPriority:     sppb.RequestOptions_PRIORITY_UNSPECIFIED,
			transactionPriority: sppb.RequestOptions_PRIORITY_HIGH,
			want:                sppb.RequestOptions_PRIORITY_HIGH,
		},
		{
			desc:                "transaction priority takes over session priority",
			sessionPriority:     sppb.RequestOptions_PRIORITY_HIGH,
			transactionPriority: sppb.RequestOptions_PRIORITY_LOW,
			want:                sppb.RequestOptions_PRIORITY_LOW,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			defer recorder.flush()

			session, err := NewSession(ctx, &systemVariables{
				Project:          project,
				Instance:         instance,
				Database:         database,
				RPCPriority:      test.sessionPriority,
				Role:             "role",
				StatementTimeout: lo.ToPtr(1 * time.Hour), // Long timeout for integration tests
			}, option.WithGRPCConn(conn))
			if err != nil {
				t.Fatalf("failed to create spanner-cli session: %v", err)
			}

			// Read-Write Transaction.
			if err := session.BeginReadWriteTransaction(ctx, 0, test.transactionPriority); err != nil {
				t.Fatalf("failed to begin read write transaction: %v", err)
			}
			iter, _ := session.RunQuery(ctx, spanner.NewStatement("SELECT * FROM t1"))
			if err := iter.Do(func(r *spanner.Row) error {
				return nil
			}); err != nil {
				t.Fatalf("failed to run query: %v", err)
			}
			if _, _, _, _, _, err := session.RunUpdate(ctx, spanner.NewStatement("DELETE FROM t1 WHERE Id = 1"), true); err != nil {
				t.Fatalf("failed to run update: %v", err)
			}
			if _, err := session.CommitReadWriteTransaction(ctx); err != nil {
				t.Fatalf("failed to commit: %v", err)
			}

			// Read-Only Transaction.
			if _, err := session.BeginReadOnlyTransaction(ctx, strong, 0, time.Now(), test.transactionPriority); err != nil {
				t.Fatalf("failed to begin read only transaction: %v", err)
			}
			iter, _ = session.RunQueryWithStats(ctx, spanner.NewStatement("SELECT * FROM t1"), false)
			if err := iter.Do(func(r *spanner.Row) error {
				return nil
			}); err != nil {
				t.Fatalf("failed to run query with stats: %v", err)
			}
			if err := session.CloseReadOnlyTransaction(); err != nil {
				t.Fatalf("failed to close read only transaction: %v", err)
			}

			// Check request priority.
			for _, r := range recorder.requests {
				switch v := r.(type) {
				case *sppb.ExecuteSqlRequest:
					if got := v.GetRequestOptions().GetPriority(); got != test.want {
						t.Errorf("priority mismatch: got = %v, want = %v", got, test.want)
					}
				case *sppb.CommitRequest:
					if got := v.GetRequestOptions().GetPriority(); got != test.want {
						t.Errorf("priority mismatch: got = %v, want = %v", got, test.want)
					}
				}
			}
		})
	}
}

func TestIsolationLevel(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping emulator test in short mode")
	}

	ctx := t.Context()

	emulator, teardown, err := spanemuboost.NewEmulator(ctx,
		spanemuboost.WithProjectID(project),
		spanemuboost.WithInstanceID(instance),
		spanemuboost.WithDatabaseID(database),
		spanemuboost.WithSetupDDLs(sliceOf("CREATE TABLE t1 (Id INT64) PRIMARY KEY (Id)")),
	)
	if err != nil {
		t.Fatalf("failed to start emulator: %v", err)
	}
	defer teardown()

	var recorder requestRecorder
	unaryInterceptor, streamInterceptor := recordRequestsInterceptors(&recorder)
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(unaryInterceptor),
		grpc.WithStreamInterceptor(streamInterceptor),
	}

	conn, err := grpc.NewClient(emulator.URI(), opts...)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}

	for _, test := range []struct {
		desc                      string
		defaultIsolationLevel     sppb.TransactionOptions_IsolationLevel
		transactionIsolationLevel sppb.TransactionOptions_IsolationLevel
		want                      sppb.TransactionOptions_IsolationLevel
	}{
		{
			desc:                      "use default isolation level",
			defaultIsolationLevel:     sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED,
			transactionIsolationLevel: sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED,
			want:                      sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED,
		},
		{
			desc:                      "use default serializable isolation level",
			defaultIsolationLevel:     sppb.TransactionOptions_SERIALIZABLE,
			transactionIsolationLevel: sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED,
			want:                      sppb.TransactionOptions_SERIALIZABLE,
		},
		{
			desc:                      "use default repeatable read isolation level",
			defaultIsolationLevel:     sppb.TransactionOptions_REPEATABLE_READ,
			transactionIsolationLevel: sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED,
			want:                      sppb.TransactionOptions_REPEATABLE_READ,
		},
		{
			desc:                      "use override serializable isolation level",
			defaultIsolationLevel:     sppb.TransactionOptions_REPEATABLE_READ,
			transactionIsolationLevel: sppb.TransactionOptions_SERIALIZABLE,
			want:                      sppb.TransactionOptions_SERIALIZABLE,
		},
		{
			desc:                      "use override repeatable read isolation level",
			defaultIsolationLevel:     sppb.TransactionOptions_SERIALIZABLE,
			transactionIsolationLevel: sppb.TransactionOptions_REPEATABLE_READ,
			want:                      sppb.TransactionOptions_REPEATABLE_READ,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			defer recorder.flush()

			session, err := NewSession(ctx, &systemVariables{
				Project:               project,
				Instance:              instance,
				Database:              database,
				DefaultIsolationLevel: test.defaultIsolationLevel,
				StatementTimeout:      lo.ToPtr(1 * time.Hour), // Long timeout for integration tests
			}, option.WithGRPCConn(conn))
			if err != nil {
				t.Fatalf("failed to create spanner-cli session: %v", err)
			}

			// Read-Write Transaction.
			if err := session.BeginReadWriteTransaction(ctx, test.transactionIsolationLevel, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
				t.Fatalf("failed to begin read write transaction: %v", err)
			}
			iter, _ := session.RunQuery(ctx, spanner.NewStatement("SELECT 1"))
			if err := iter.Do(func(r *spanner.Row) error {
				return nil
			}); err != nil {
				t.Fatalf("failed to run query: %v", err)
			}
			if _, err := session.CommitReadWriteTransaction(ctx); err != nil {
				t.Fatalf("failed to commit: %v", err)
			}

			// Check request priority.
			for _, r := range recorder.requests {
				switch v := r.(type) {
				case *sppb.BeginTransactionRequest:
					if got := v.GetOptions().GetIsolationLevel(); got != test.want {
						t.Errorf("transaction level mismatch: got = %v, want = %v", got, test.want)
					}
				}
			}
		})
	}
}

// requestRecorder is a recorder to retain gRPC requests for spannertest.Server.
type requestRecorder struct {
	requests []interface{}
}

func (r *requestRecorder) flush() {
	r.requests = nil
}

func recordRequestsInterceptors(recorder *requestRecorder) (grpc.UnaryClientInterceptor, grpc.StreamClientInterceptor) {
	unary := func(ctx context.Context, method string, req interface{}, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		recorder.requests = append(recorder.requests, req)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
	stream := func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		s, err := streamer(ctx, desc, cc, method, opts...)
		return &recordRequestsStream{recorder, s}, err
	}
	return unary, stream
}

type recordRequestsStream struct {
	recorder *requestRecorder
	grpc.ClientStream
}

func (s *recordRequestsStream) SendMsg(m interface{}) error {
	s.recorder.requests = append(s.recorder.requests, m)
	return s.ClientStream.SendMsg(m)
}
