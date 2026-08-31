package mycli

import (
	"context"
	"errors"
	"strings"
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

// Remove the separate TestMain and use the shared emulator from integration_test.go

func TestRequestPriority(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping emulator test in short mode")
	}

	ctx := t.Context()

	// Use shared emulator with a random instance ID for isolation
	clients := spanemuboost.SetupClients(t, lazyRuntime,
		spanemuboost.WithRandomInstanceID(),
		spanemuboost.WithProjectID(project),
		spanemuboost.WithDatabaseID(database),
		spanemuboost.EnableAutoConfig(),
		spanemuboost.WithSetupDDLs(sliceOf("CREATE TABLE t1 (Id INT64) PRIMARY KEY (Id)")),
	)
	var recorder requestRecorder
	unaryInterceptor, streamInterceptor := recordRequestsInterceptors(&recorder)
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(unaryInterceptor),
		grpc.WithStreamInterceptor(streamInterceptor),
	}

	conn, err := grpc.NewClient(clients.URI(), opts...)
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
				Connection: ConnectionVars{
					Project:  clients.ProjectID,
					Instance: clients.InstanceID,
					Database: clients.DatabaseID,
					Role:     "role",
				},
				Query: QueryVars{
					RPCPriority:      test.sessionPriority,
					StatementTimeout: lo.ToPtr(1 * time.Hour), // Long timeout for integration tests
				},
			}, option.WithGRPCConn(conn))
			if err != nil {
				t.Fatalf("failed to create spanner-cli session: %v", err)
			}

			// Read-Write Transaction.
			if err := session.txn.BeginReadWriteTransaction(ctx, 0, test.transactionPriority); err != nil {
				t.Fatalf("failed to begin read write transaction: %v", err)
			}
			iter, _, err := session.txn.RunQuery(ctx, spanner.NewStatement("SELECT * FROM t1"))
			if err != nil {
				t.Fatalf("failed to run query: %v", err)
			}
			if err := iter.Do(func(r *spanner.Row) error {
				return nil
			}); err != nil {
				t.Fatalf("failed to run query: %v", err)
			}
			if _, err := session.txn.RunInNewOrExistRwTx(ctx, func(tx *spanner.ReadWriteStmtBasedTransaction, implicit bool) (int64, *sppb.QueryPlan, *sppb.ResultSetMetadata, error) {
				iter := session.txn.runQueryWithStatsOnTransaction(ctx, tx, spanner.NewStatement("DELETE FROM t1 WHERE Id = 1"), implicit)
				_, count, metadata, plan, err := consumeRowIterDiscard(iter)
				return count, plan, metadata, err
			}); err != nil {
				t.Fatalf("failed to run update: %v", err)
			}
			if _, err := session.txn.CommitReadWriteTransaction(ctx); err != nil {
				t.Fatalf("failed to commit: %v", err)
			}

			// Read-Only Transaction.
			if _, err := session.txn.BeginReadOnlyTransaction(ctx, strong, 0, time.Now(), test.transactionPriority); err != nil {
				t.Fatalf("failed to begin read only transaction: %v", err)
			}
			iter, _, err = session.txn.RunQueryWithStats(ctx, spanner.NewStatement("SELECT * FROM t1"), false, sppb.ExecuteSqlRequest_PROFILE)
			if err != nil {
				t.Fatalf("failed to run query with stats: %v", err)
			}
			if err := iter.Do(func(r *spanner.Row) error {
				return nil
			}); err != nil {
				t.Fatalf("failed to run query with stats: %v", err)
			}
			if err := session.txn.CloseReadOnlyTransaction(); err != nil {
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

func TestExportDataRequestUsesIsolatedSingleUseOptions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping emulator test in short mode")
	}

	ctx := t.Context()
	clients := spanemuboost.SetupClients(t, lazyRuntime,
		spanemuboost.WithRandomInstanceID(),
		spanemuboost.WithProjectID(project),
		spanemuboost.WithDatabaseID(database),
		spanemuboost.EnableAutoConfig(),
	)
	var recorder requestRecorder
	unaryInterceptor, streamInterceptor := recordRequestsInterceptors(&recorder)
	conn, err := grpc.NewClient(clients.URI(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(unaryInterceptor),
		grpc.WithStreamInterceptor(streamInterceptor),
	)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}

	sysVars := newSystemVariablesWithDefaultsForTest()
	sysVars.Connection = ConnectionVars{
		Project:  clients.ProjectID,
		Instance: clients.InstanceID,
		Database: clients.DatabaseID,
	}
	sysVars.Query.StatementTimeout = lo.ToPtr(time.Hour)
	session, err := NewSession(ctx, sysVars, option.WithGRPCConn(conn))
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	if err := session.txn.BeginReadWriteTransaction(ctx, 0, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatalf("failed to begin read-write transaction: %v", err)
	}
	iter, _, err := session.txn.RunQuery(ctx, spanner.NewStatement("SELECT 1"))
	if err != nil {
		t.Fatalf("failed to activate read-write transaction: %v", err)
	}
	if _, _, _, _, err := consumeRowIterDiscard(iter); err != nil {
		t.Fatalf("failed to consume activation query: %v", err)
	}

	const (
		exportSQL  = "EXPORT DATA OPTIONS (format = 'CLOUD_SPANNER', table = 'Account') AS SELECT 1"
		requestTag = "graph-export"
	)
	sysVars.Transaction.RequestTag = requestTag
	sysVars.Query.OptimizerVersion = "7"
	sysVars.Query.OptimizerStatisticsPackage = "auto_test_package"
	sysVars.Query.ReadOnlyStaleness = lo.ToPtr(spanner.ExactStaleness(time.Minute))

	if _, err := session.ExecuteStatement(ctx, &ExportDataStatement{SQL: exportSQL}); err != nil {
		t.Fatalf("ExportDataStatement.Execute() error = %v, want nil", err)
	}
	if !session.txn.InReadWriteTransaction() {
		t.Fatal("EXPORT DATA must not replace or close the explicit transaction")
	}
	if got := sysVars.Transaction.RequestTag; got != "" {
		t.Fatalf("STATEMENT_TAG after EXPORT DATA = %q, want consumed", got)
	}

	var exportRequest *sppb.ExecuteSqlRequest
	for _, request := range recorder.requests {
		if execute, ok := request.(*sppb.ExecuteSqlRequest); ok && strings.HasPrefix(execute.GetSql(), "EXPORT DATA") {
			exportRequest = execute
			break
		}
	}
	if exportRequest == nil {
		t.Fatal("recorded requests contain no EXPORT DATA ExecuteSqlRequest")
	}
	if exportRequest.GetTransaction().GetSingleUse() == nil {
		t.Fatalf("EXPORT DATA transaction selector = %T, want single-use", exportRequest.GetTransaction().GetSelector())
	}
	if got := exportRequest.GetRequestOptions().GetRequestTag(); got != requestTag {
		t.Errorf("EXPORT DATA request tag = %q, want %q", got, requestTag)
	}
	if got := exportRequest.GetQueryOptions().GetOptimizerVersion(); got != "7" {
		t.Errorf("EXPORT DATA optimizer version = %q, want 7", got)
	}
	if got := exportRequest.GetQueryOptions().GetOptimizerStatisticsPackage(); got != "auto_test_package" {
		t.Errorf("EXPORT DATA optimizer statistics package = %q, want auto_test_package", got)
	}
	if got := exportRequest.GetTransaction().GetSingleUse().GetReadOnly().GetExactStaleness().AsDuration(); got != time.Minute {
		t.Errorf("EXPORT DATA exact staleness = %v, want %v", got, time.Minute)
	}

	iter, _, err = session.txn.RunQuery(ctx, spanner.NewStatement("SELECT 1"))
	if err != nil {
		t.Fatalf("failed to run query after EXPORT DATA: %v", err)
	}
	if _, _, _, _, err := consumeRowIterDiscard(iter); err != nil {
		t.Fatalf("failed to consume query after EXPORT DATA: %v", err)
	}
	foundFollowingSelect := false
	for i := len(recorder.requests) - 1; i >= 0; i-- {
		execute, ok := recorder.requests[i].(*sppb.ExecuteSqlRequest)
		if !ok || execute.GetSql() != "SELECT 1" {
			continue
		}
		foundFollowingSelect = true
		if got := execute.GetRequestOptions().GetRequestTag(); got != "" {
			t.Errorf("following SELECT request tag = %q, want empty", got)
		}
		break
	}
	if !foundFollowingSelect {
		t.Fatal("recorded requests contain no following SELECT ExecuteSqlRequest")
	}
}

func TestIsolationLevel(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping emulator test in short mode")
	}

	ctx := t.Context()

	// Use shared emulator with a random instance ID for isolation
	clients := spanemuboost.SetupClients(t, lazyRuntime,
		spanemuboost.WithRandomInstanceID(),
		spanemuboost.WithProjectID(project),
		spanemuboost.WithDatabaseID(database),
		spanemuboost.EnableAutoConfig(),
		spanemuboost.WithSetupDDLs(sliceOf("CREATE TABLE t1 (Id INT64) PRIMARY KEY (Id)")),
	)
	var recorder requestRecorder
	unaryInterceptor, streamInterceptor := recordRequestsInterceptors(&recorder)
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(unaryInterceptor),
		grpc.WithStreamInterceptor(streamInterceptor),
	}

	conn, err := grpc.NewClient(clients.URI(), opts...)
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
				Connection: ConnectionVars{
					Project:  clients.ProjectID,
					Instance: clients.InstanceID,
					Database: clients.DatabaseID,
				},
				Query: QueryVars{
					StatementTimeout: lo.ToPtr(1 * time.Hour), // Long timeout for integration tests
				},
				Transaction: TransactionVars{
					DefaultIsolationLevel: test.defaultIsolationLevel,
				},
			}, option.WithGRPCConn(conn))
			if err != nil {
				t.Fatalf("failed to create spanner-cli session: %v", err)
			}

			// Read-Write Transaction.
			if err := session.txn.BeginReadWriteTransaction(ctx, test.transactionIsolationLevel, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
				t.Fatalf("failed to begin read write transaction: %v", err)
			}
			iter, _, err := session.txn.RunQuery(ctx, spanner.NewStatement("SELECT 1"))
			if err != nil {
				t.Fatalf("failed to run query: %v", err)
			}
			if err := iter.Do(func(r *spanner.Row) error {
				return nil
			}); err != nil {
				t.Fatalf("failed to run query: %v", err)
			}
			if _, err := session.txn.CommitReadWriteTransaction(ctx); err != nil {
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

// TestInstanceExists exercises Session.InstanceExists against the emulator to
// verify (1) the happy path for an existing instance, (2) NotFound handling for
// a bogus instance name, and (3) that the caller's context is honored (a
// pre-cancelled context must fail fast instead of being ignored, as it was when
// the method used context.Background()).
func TestInstanceExists(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping emulator test in short mode")
	}

	ctx := t.Context()
	_, session := initializeAdminSession(t)

	t.Run("existing instance", func(t *testing.T) {
		exists, err := session.InstanceExists(ctx)
		if err != nil {
			t.Fatalf("InstanceExists returned error: %v", err)
		}
		if !exists {
			t.Error("expected InstanceExists to be true for the configured instance")
		}
	})

	t.Run("nonexistent instance", func(t *testing.T) {
		// Temporarily point the session at an instance that does not exist so
		// the databases.list call resolves to NotFound.
		orig := session.systemVariables.Connection.Instance
		session.systemVariables.Connection.Instance = "nonexistent-instance"
		defer func() { session.systemVariables.Connection.Instance = orig }()

		exists, err := session.InstanceExists(ctx)
		if err != nil {
			t.Fatalf("InstanceExists returned error: %v", err)
		}
		if exists {
			t.Error("expected InstanceExists to be false for a nonexistent instance")
		}
	})

	t.Run("cancelled context is honored", func(t *testing.T) {
		cancelledCtx, cancel := context.WithCancel(ctx)
		cancel()

		_, err := session.InstanceExists(cancelledCtx)
		if err == nil {
			t.Fatal("expected InstanceExists to fail with a cancelled context")
		}
		// The cancellation must be surfaced directly (short-circuited), not
		// masked as a generic "checking instance existence failed" error.
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected error to wrap context.Canceled, got: %v", err)
		}
	})
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
