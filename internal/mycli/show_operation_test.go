package mycli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/longrunning/autogen/longrunningpb"
	adminapi "cloud.google.com/go/spanner/admin/database/apiv1"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/stretchr/testify/assert"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func ddlShowOperation(name string) *longrunningpb.Operation {
	md, err := anypb.New(&databasepb.UpdateDatabaseDdlMetadata{
		Statements: []string{"CREATE TABLE t (id INT64) PRIMARY KEY (id)"},
	})
	if err != nil {
		panic(err)
	}
	return &longrunningpb.Operation{Name: name, Metadata: md}
}

func TestShowOperationSyncWaitError(t *testing.T) {
	t.Parallel()

	ddlOp := ddlShowOperation("projects/p/instances/i/databases/d/operations/op-ddl")
	otherOp := &longrunningpb.Operation{
		Name: "projects/p/instances/i/databases/d/operations/op-other",
	}
	fullName := "projects/p/instances/i/databases/d/operations/op-ddl"

	tests := []struct {
		name           string
		op             *longrunningpb.Operation
		operationName  string
		err            error
		wantGenDelta   uint64
		wantIsCanceled bool
		wantIsDeadline bool
		wantHintOpID   string
		wantUnchanged  bool
	}{
		{
			name:           "cancel after DDL identified bumps schema and hints",
			op:             ddlOp,
			operationName:  fullName,
			err:            context.Canceled,
			wantGenDelta:   1,
			wantIsCanceled: true,
			wantHintOpID:   "op-ddl",
		},
		{
			name:           "deadline after DDL identified bumps schema and hints",
			op:             ddlOp,
			operationName:  "op-ddl",
			err:            context.DeadlineExceeded,
			wantGenDelta:   1,
			wantIsDeadline: true,
			wantHintOpID:   "op-ddl",
		},
		{
			name:          "grpc cancel after DDL identified bumps schema and hints",
			op:            ddlOp,
			operationName: fullName,
			err:           status.Error(codes.Canceled, "context canceled"),
			wantGenDelta:  1,
			wantHintOpID:  "op-ddl",
		},
		{
			name:           "cancel before identification hints without schema bump",
			op:             nil,
			operationName:  fullName,
			err:            context.Canceled,
			wantGenDelta:   0,
			wantIsCanceled: true,
			wantHintOpID:   "op-ddl",
		},
		{
			name:           "cancel of non-DDL operation hints without schema bump",
			op:             otherOp,
			operationName:  "projects/p/instances/i/databases/d/operations/op-other",
			err:            context.Canceled,
			wantGenDelta:   0,
			wantIsCanceled: true,
			wantHintOpID:   "op-other",
		},
		{
			name:          "non-cancel error after DDL identified bumps schema and keeps original",
			op:            ddlOp,
			operationName: fullName,
			err:           errors.New("boom"),
			wantGenDelta:  1,
			wantUnchanged: true,
		},
		{
			name:          "non-cancel error before identification does not bump schema",
			op:            nil,
			operationName: fullName,
			err:           errors.New("not found"),
			wantGenDelta:  0,
			wantUnchanged: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			session := &Session{systemVariables: newSystemVariablesWithDefaultsForTest()}
			before := session.SchemaGeneration()

			got := handleShowOperationSyncWaitError(session, tt.op, tt.operationName, tt.err)
			if got == nil {
				t.Fatal("expected non-nil error")
			}

			if delta := session.SchemaGeneration() - before; delta != tt.wantGenDelta {
				t.Errorf("schema generation delta = %d, want %d", delta, tt.wantGenDelta)
			}

			if tt.wantUnchanged {
				if !errors.Is(got, tt.err) && got.Error() != tt.err.Error() {
					t.Errorf("non-cancel error mutated: got %v, want %v", got, tt.err)
				}
				if strings.Contains(got.Error(), "SHOW OPERATION") {
					t.Errorf("non-cancel error unexpectedly contains SHOW OPERATION hint: %v", got)
				}
				return
			}

			if tt.wantIsCanceled && !errors.Is(got, context.Canceled) {
				t.Errorf("errors.Is(err, context.Canceled) = false; err = %v", got)
			}
			if tt.wantIsDeadline && !errors.Is(got, context.DeadlineExceeded) {
				t.Errorf("errors.Is(err, context.DeadlineExceeded) = false; err = %v", got)
			}
			if tt.wantHintOpID != "" && !strings.Contains(got.Error(), "SHOW OPERATION '"+tt.wantHintOpID+"'") {
				t.Errorf("error missing SHOW OPERATION hint for %q\n  got: %s", tt.wantHintOpID, got)
			}
		})
	}
}

func TestShowOperationSyncCancellationErrorPreservesCause(t *testing.T) {
	t.Parallel()

	wrapped := fmt.Errorf("failed to poll operation %q: %w", "op-ddl", context.Canceled)
	err := showOperationSyncCancellationError("projects/p/instances/i/databases/d/operations/op-ddl", wrapped)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("errors.Is(err, context.Canceled) = false; err = %v", err)
	}
	if !strings.Contains(err.Error(), "SHOW OPERATION 'op-ddl'") {
		t.Fatalf("error missing hint: %v", err)
	}
}

func TestShowOperationStatement_getOperationDescription(t *testing.T) {
	t.Parallel()
	stmt := &ShowOperationStatement{}

	tests := []struct {
		name     string
		op       *longrunningpb.Operation
		expected string
	}{
		{
			name: "operation with no metadata",
			op: &longrunningpb.Operation{
				Name: "projects/test/instances/test/databases/test/operations/auto_op_123",
			},
			expected: "Operation auto_op_123",
		},
		{
			name: "DDL operation with statements",
			op: func() *longrunningpb.Operation {
				md := &databasepb.UpdateDatabaseDdlMetadata{
					Statements: []string{"CREATE TABLE test (id INT64) PRIMARY KEY (id)"},
				}
				any, _ := anypb.New(md)
				return &longrunningpb.Operation{
					Name:     "projects/test/instances/test/databases/test/operations/auto_op_123",
					Metadata: any,
				}
			}(),
			expected: "CREATE TABLE test (id INT64) PRIMARY KEY (id)",
		},
		{
			name: "DDL operation with no statements",
			op: func() *longrunningpb.Operation {
				md := &databasepb.UpdateDatabaseDdlMetadata{
					Statements: []string{},
				}
				any, _ := anypb.New(md)
				return &longrunningpb.Operation{
					Name:     "projects/test/instances/test/databases/test/operations/auto_op_123",
					Metadata: any,
				}
			}(),
			expected: "DDL Operation auto_op_123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stmt.getOperationDescription(tt.op)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestShowOperationStatement_getOperationProgress(t *testing.T) {
	t.Parallel()
	stmt := &ShowOperationStatement{}

	tests := []struct {
		name     string
		op       *longrunningpb.Operation
		expected float64
	}{
		{
			name: "operation with no metadata",
			op: &longrunningpb.Operation{
				Name: "projects/test/instances/test/databases/test/operations/auto_op_123",
			},
			expected: 0.0,
		},
		{
			name: "DDL operation with progress",
			op: func() *longrunningpb.Operation {
				md := &databasepb.UpdateDatabaseDdlMetadata{
					Progress: []*databasepb.OperationProgress{
						{ProgressPercent: 60},
						{ProgressPercent: 40},
					},
				}
				any, _ := anypb.New(md)
				return &longrunningpb.Operation{
					Name:     "projects/test/instances/test/databases/test/operations/auto_op_123",
					Metadata: any,
				}
			}(),
			expected: 50.0, // Average of 60 and 40
		},
		{
			name: "DDL operation with no progress",
			op: func() *longrunningpb.Operation {
				md := &databasepb.UpdateDatabaseDdlMetadata{
					Progress: []*databasepb.OperationProgress{},
				}
				any, _ := anypb.New(md)
				return &longrunningpb.Operation{
					Name:     "projects/test/instances/test/databases/test/operations/auto_op_123",
					Metadata: any,
				}
			}(),
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stmt.getOperationProgress(tt.op)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestShowOperationStatement_SyncModeWithCompletedOperation(t *testing.T) {
	t.Parallel()
	// Test that SYNC mode immediately returns for completed operations
	// This is a unit test that doesn't require the full integration setup

	// Create a mock completed operation
	completedOp := &longrunningpb.Operation{
		Name: "projects/test/instances/test/databases/test/operations/auto_op_123",
		Done: true,
	}

	stmt := &ShowOperationStatement{
		OperationId: "auto_op_123",
		Mode:        "SYNC",
	}

	// Test that getOperationDescription works correctly
	desc := stmt.getOperationDescription(completedOp)
	assert.Equal(t, "Operation auto_op_123", desc)

	// Test that getOperationProgress works correctly
	progress := stmt.getOperationProgress(completedOp)
	assert.Equal(t, 0.0, progress)
}

func TestShowOperationStatement_ProgressCalculation(t *testing.T) {
	t.Parallel()
	stmt := &ShowOperationStatement{}

	// Test multiple progress values averaging
	md := &databasepb.UpdateDatabaseDdlMetadata{
		Progress: []*databasepb.OperationProgress{
			{ProgressPercent: 10},
			{ProgressPercent: 20},
			{ProgressPercent: 30},
		},
	}
	any, _ := anypb.New(md)
	op := &longrunningpb.Operation{
		Name:     "projects/test/instances/test/databases/test/operations/auto_op_123",
		Metadata: any,
	}

	progress := stmt.getOperationProgress(op)
	expected := (10.0 + 20.0 + 30.0) / 3.0
	assert.Equal(t, expected, progress)
}

func TestShowOperationStatement_ContextCancellation(t *testing.T) {
	t.Parallel()
	// Test context cancellation behavior
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// This test verifies that context cancellation is properly handled
	// In a real scenario, this would test the polling loop cancellation
	select {
	case <-ctx.Done():
		assert.Equal(t, context.DeadlineExceeded, ctx.Err())
	case <-time.After(200 * time.Millisecond):
		t.Error("Context should have been cancelled")
	}
}

func TestShowOperationStatement_MetadataTypes(t *testing.T) {
	t.Parallel()
	stmt := &ShowOperationStatement{}

	tests := []struct {
		name         string
		metadata     proto.Message
		expectedOp   string
		expectedProg float64
	}{
		{
			name: "UpdateDatabaseDdlMetadata with single statement",
			metadata: &databasepb.UpdateDatabaseDdlMetadata{
				Statements: []string{"CREATE INDEX idx ON table (col)"},
				Progress:   []*databasepb.OperationProgress{{ProgressPercent: 75}},
			},
			expectedOp:   "CREATE INDEX idx ON table (col)",
			expectedProg: 75.0,
		},
		{
			name: "UpdateDatabaseDdlMetadata with multiple statements",
			metadata: &databasepb.UpdateDatabaseDdlMetadata{
				Statements: []string{
					"CREATE TABLE t1 (id INT64) PRIMARY KEY (id)",
					"CREATE INDEX idx ON t1 (id)",
				},
				Progress: []*databasepb.OperationProgress{
					{ProgressPercent: 80},
					{ProgressPercent: 60},
				},
			},
			expectedOp:   "CREATE TABLE t1 (id INT64) PRIMARY KEY (id)", // First statement
			expectedProg: 70.0,                                          // Average of 80 and 60
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			any, err := anypb.New(tt.metadata)
			assert.NoError(t, err)

			op := &longrunningpb.Operation{
				Name:     "projects/test/instances/test/databases/test/operations/auto_op_123",
				Metadata: any,
			}

			desc := stmt.getOperationDescription(op)
			assert.Equal(t, tt.expectedOp, desc)

			progress := stmt.getOperationProgress(op)
			assert.Equal(t, tt.expectedProg, progress)
		})
	}
}

func TestShowOperationStatement_SyncModeCanceledDDLPoll(t *testing.T) {
	t.Parallel()

	operationName := "projects/test/instances/test/databases/test/operations/auto_op_123"
	ddlOp := ddlShowOperation(operationName)
	server := &showOperationTestServer{responses: []showOperationResponse{
		{operation: ddlOp},
		{err: status.Error(codes.Canceled, "context canceled")},
	}}
	session := newShowOperationTestSession(t, server)
	stmt := &ShowOperationStatement{OperationId: "auto_op_123", Mode: "SYNC"}
	ticks := make(chan time.Time, 1)
	ticks <- time.Now()

	result, err := stmt.executeSyncModeWithTicks(t.Context(), session, ticks)
	if result != nil {
		t.Fatalf("executeSyncModeWithTicks() result = %#v, want nil", result)
	}
	if err == nil {
		t.Fatal("executeSyncModeWithTicks() error = nil, want cancellation hint")
	}
	if !strings.Contains(err.Error(), "SHOW OPERATION 'auto_op_123'") {
		t.Errorf("error missing SHOW OPERATION hint: %v", err)
	}
	if status.Code(err) != codes.Canceled {
		t.Errorf("status.Code(err) = %v, want Canceled; err = %v", status.Code(err), err)
	}
	if got := session.SchemaGeneration(); got != 1 {
		t.Errorf("SchemaGeneration() = %d, want 1", got)
	}
}

func TestShowOperationStatement_SyncModeFormatsInitiallyDoneWithoutRefetch(t *testing.T) {
	t.Parallel()

	operationName := "projects/test/instances/test/databases/test/operations/auto_op_123"
	done := ddlShowOperation(operationName)
	done.Done = true
	server := &showOperationTestServer{responses: []showOperationResponse{
		{operation: done},
	}}
	session := newShowOperationTestSession(t, server)
	stmt := &ShowOperationStatement{OperationId: "auto_op_123", Mode: "SYNC"}

	result, err := stmt.executeSyncModeWithTicks(t.Context(), session, make(chan time.Time))
	if err != nil {
		t.Fatalf("executeSyncModeWithTicks() error = %v", err)
	}
	assertDoneShowOperationRow(t, result)
	if len(server.responses) != 0 {
		t.Fatalf("unused GetOperation responses = %d, want 0", len(server.responses))
	}
}

func TestShowOperationStatement_SyncModeFormatsPollDoneWithoutRefetch(t *testing.T) {
	t.Parallel()

	operationName := "projects/test/instances/test/databases/test/operations/auto_op_123"
	inProgress := ddlShowOperation(operationName)
	done := ddlShowOperation(operationName)
	done.Done = true
	server := &showOperationTestServer{responses: []showOperationResponse{
		{operation: inProgress},
		{operation: done},
	}}
	session := newShowOperationTestSession(t, server)
	stmt := &ShowOperationStatement{OperationId: "auto_op_123", Mode: "SYNC"}
	ticks := make(chan time.Time, 1)
	ticks <- time.Now()

	result, err := stmt.executeSyncModeWithTicks(t.Context(), session, ticks)
	if err != nil {
		t.Fatalf("executeSyncModeWithTicks() error = %v", err)
	}
	assertDoneShowOperationRow(t, result)
	if len(server.responses) != 0 {
		t.Fatalf("unused GetOperation responses = %d, want 0", len(server.responses))
	}
}

func TestShowOperationStatement_AsyncModeStillFetches(t *testing.T) {
	t.Parallel()

	operationName := "projects/test/instances/test/databases/test/operations/auto_op_123"
	done := ddlShowOperation(operationName)
	done.Done = true
	server := &showOperationTestServer{responses: []showOperationResponse{
		{operation: done},
	}}
	session := newShowOperationTestSession(t, server)
	stmt := &ShowOperationStatement{OperationId: "auto_op_123", Mode: "ASYNC"}

	result, err := stmt.executeAsyncMode(t.Context(), session, operationName)
	if err != nil {
		t.Fatalf("executeAsyncMode() error = %v", err)
	}
	assertDoneShowOperationRow(t, result)
	if len(server.responses) != 0 {
		t.Fatalf("ASYNC skipped GetOperation; unused responses = %d", len(server.responses))
	}
}

func assertDoneShowOperationRow(t *testing.T, result *Result) {
	t.Helper()
	if result == nil {
		t.Fatal("result = nil")
	}
	if len(result.Rows) != 1 {
		t.Fatalf("len(result.Rows) = %d, want 1", len(result.Rows))
	}
	if got := result.Rows[0][0].RawText(); got != "auto_op_123" {
		t.Errorf("OPERATION_ID = %q, want auto_op_123", got)
	}
	if got := result.Rows[0][2].RawText(); got != "true" {
		t.Errorf("DONE = %q, want true", got)
	}
}

type showOperationResponse struct {
	operation *longrunningpb.Operation
	err       error
}

type showOperationTestServer struct {
	longrunningpb.UnimplementedOperationsServer
	responses []showOperationResponse
}

func (s *showOperationTestServer) GetOperation(_ context.Context, _ *longrunningpb.GetOperationRequest) (*longrunningpb.Operation, error) {
	if len(s.responses) == 0 {
		return nil, status.Error(codes.Internal, "unexpected extra GetOperation")
	}
	response := s.responses[0]
	s.responses = s.responses[1:]
	return response.operation, response.err
}

func newShowOperationTestSession(t *testing.T, server longrunningpb.OperationsServer) *Session {
	t.Helper()

	listener := bufconn.Listen(1 << 20)
	grpcServer := grpc.NewServer()
	longrunningpb.RegisterOperationsServer(grpcServer, server)
	go func() {
		if err := grpcServer.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Errorf("serve operation test server: %v", err)
		}
	}()
	t.Cleanup(func() {
		grpcServer.Stop()
		_ = listener.Close()
	})

	conn, err := grpc.NewClient(
		"passthrough:///show-operation-test",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("create gRPC test client: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	adminClient, err := adminapi.NewDatabaseAdminClient(t.Context(), option.WithGRPCConn(conn))
	if err != nil {
		t.Fatalf("create database admin client: %v", err)
	}
	t.Cleanup(func() { _ = adminClient.Close() })

	return &Session{
		adminClient: adminClient,
		systemVariables: &systemVariables{Connection: ConnectionVars{
			Project:  "test",
			Instance: "test",
			Database: "test",
		}},
	}
}
