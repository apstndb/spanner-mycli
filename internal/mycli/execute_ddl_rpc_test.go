// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mycli

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/longrunning/autogen/longrunningpb"
	adminapi "cloud.google.com/go/spanner/admin/database/apiv1"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/apstndb/spanner-mycli/internal/mycli/streamio"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const ddlRPCOpName = "projects/test/instances/test/databases/test/operations/op-ddl"

func TestExecuteDdlStatementsRPC(t *testing.T) {
	t.Parallel()

	const ddl = "CREATE TABLE t (id INT64) PRIMARY KEY (id)"
	commitTS := time.Date(2026, 9, 12, 15, 0, 0, 0, time.UTC)

	t.Run("create error", func(t *testing.T) {
		t.Parallel()
		server := &ddlAdminTestServer{updateErr: status.Error(codes.InvalidArgument, "bad DDL")}
		session := newDDLAdminSession(t, server)
		before := session.SchemaGeneration()
		_, err := executeDdlStatements(t.Context(), session, []string{ddl})
		if err == nil || !strings.Contains(err.Error(), "error on create op") {
			t.Fatalf("error = %v, want create-op wrap", err)
		}
		if !strings.Contains(err.Error(), "bad DDL") {
			t.Fatalf("error = %v, want underlying InvalidArgument", err)
		}
		if session.SchemaGeneration() != before {
			t.Fatalf("schema generation = %d, want unchanged %d", session.SchemaGeneration(), before)
		}
		if server.lastUpdate == nil {
			t.Fatal("UpdateDatabaseDdl was not invoked")
		}
		if server.lastUpdate.GetDatabase() != session.DatabasePath() {
			t.Fatalf("Database = %q, want %q", server.lastUpdate.GetDatabase(), session.DatabasePath())
		}
		if diff := strings.Join(server.lastUpdate.GetStatements(), "\n"); diff != ddl {
			t.Fatalf("Statements = %v, want %q", server.lastUpdate.GetStatements(), ddl)
		}
	})

	t.Run("sync echo executed DDL", func(t *testing.T) {
		t.Parallel()
		server := newCompletedDDLServer(ddl, commitTS)
		session := newDDLAdminSession(t, server)
		session.systemVariables.Feature.EchoExecutedDDL = true
		before := session.SchemaGeneration()
		got, err := executeDdlStatements(t.Context(), session, []string{ddl})
		if err != nil {
			t.Fatalf("executeDdlStatements() error = %v", err)
		}
		if session.SchemaGeneration() != before+1 {
			t.Fatalf("schema generation = %d, want %d", session.SchemaGeneration(), before+1)
		}
		if !got.CommitTimestamp.Equal(commitTS) {
			t.Fatalf("CommitTimestamp = %v, want %v", got.CommitTimestamp, commitTS)
		}
		if got.TableHeader == nil {
			t.Fatal("TableHeader = nil, want echo columns")
		}
		if len(got.Rows) != 1 {
			t.Fatalf("len(Rows) = %d, want 1", len(got.Rows))
		}
		if got.Rows[0][0].RawText() != ddl+";" {
			t.Fatalf("executed DDL = %q, want %q", got.Rows[0][0].RawText(), ddl+";")
		}
		if got.Rows[0][1].RawText() != commitTS.Format(time.RFC3339Nano) {
			t.Fatalf("commit timestamp cell = %q, want %q", got.Rows[0][1].RawText(), commitTS.Format(time.RFC3339Nano))
		}
	})

	t.Run("async returns operation rows", func(t *testing.T) {
		t.Parallel()
		server := newCompletedDDLServer(ddl, commitTS)
		server.done = false
		session := newDDLAdminSession(t, server)
		session.systemVariables.Feature.AsyncDDL = true
		before := session.SchemaGeneration()
		got, err := executeDdlStatements(t.Context(), session, []string{ddl})
		if err != nil {
			t.Fatalf("executeDdlStatements() error = %v", err)
		}
		if session.SchemaGeneration() != before+1 {
			t.Fatalf("schema generation = %d, want %d", session.SchemaGeneration(), before+1)
		}
		if got.AffectedRows != 1 || len(got.Rows) != 1 {
			t.Fatalf("async result = %+v", got)
		}
		if got.Rows[0][0].RawText() != "op-ddl" {
			t.Fatalf("OPERATION_ID = %q, want op-ddl", got.Rows[0][0].RawText())
		}
		if got.Rows[0][1].RawText() != ddl+";" {
			t.Fatalf("STATEMENTS = %q, want %q", got.Rows[0][1].RawText(), ddl+";")
		}
		if got.Rows[0][2].RawText() != "false" {
			t.Fatalf("DONE = %q, want false", got.Rows[0][2].RawText())
		}
		if server.getCalls.Load() != 0 {
			t.Fatalf("async path polled GetOperation %d times, want 0", server.getCalls.Load())
		}
	})

	t.Run("async metadata type error", func(t *testing.T) {
		t.Parallel()
		wrong, err := anypb.New(&emptypb.Empty{})
		if err != nil {
			t.Fatal(err)
		}
		server := &ddlAdminTestServer{
			opName:   ddlRPCOpName,
			metadata: wrong,
			done:     false,
		}
		session := newDDLAdminSession(t, server)
		session.systemVariables.Feature.AsyncDDL = true
		_, execErr := executeDdlStatements(t.Context(), session, []string{ddl})
		if execErr == nil || !strings.Contains(execErr.Error(), "failed to get operation metadata") {
			t.Fatalf("error = %v, want metadata unmarshal failure", execErr)
		}
	})

	t.Run("poll cancellation hints SHOW OPERATION", func(t *testing.T) {
		t.Parallel()
		server := newCompletedDDLServer(ddl, commitTS)
		server.done = false
		server.getErr = status.Error(codes.Canceled, "context canceled")
		session := newDDLAdminSession(t, server)
		before := session.SchemaGeneration()
		_, err := executeDdlStatements(t.Context(), session, []string{ddl})
		if err == nil || !strings.Contains(err.Error(), "SHOW OPERATION 'op-ddl'") {
			t.Fatalf("error = %v, want SHOW OPERATION cancellation hint", err)
		}
		if session.SchemaGeneration() != before+1 {
			t.Fatalf("schema generation = %d, want %d after cancel", session.SchemaGeneration(), before+1)
		}
	})

	t.Run("poll invalid argument keeps original error", func(t *testing.T) {
		t.Parallel()
		server := newCompletedDDLServer(ddl, commitTS)
		server.done = false
		server.getErr = status.Error(codes.InvalidArgument, "syntax error")
		session := newDDLAdminSession(t, server)
		before := session.SchemaGeneration()
		_, err := executeDdlStatements(t.Context(), session, []string{ddl})
		if err == nil || strings.Contains(err.Error(), "SHOW OPERATION") {
			t.Fatalf("error = %v, want original DDL failure", err)
		}
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("status.Code = %v, want InvalidArgument; err = %v", status.Code(err), err)
		}
		if !strings.Contains(err.Error(), "syntax error") {
			t.Fatalf("error = %v, want injected message %q", err, "syntax error")
		}
		if session.SchemaGeneration() != before+1 {
			t.Fatalf("schema generation = %d, want %d after accepted op", session.SchemaGeneration(), before+1)
		}
	})

	t.Run("wait loop cancellation", func(t *testing.T) {
		t.Parallel()
		server := newCompletedDDLServer(ddl, commitTS)
		server.stayPending = true
		server.accepted = make(chan struct{})
		server.polled = make(chan struct{})
		session := newDDLAdminSession(t, server)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		// Generous outer timeout is a deadlock guard only; cancellation is
		// synchronized with the fake observing an accepted op and first poll.
		guard, guardCancel := context.WithTimeout(t.Context(), 30*time.Second)
		defer guardCancel()

		errc := make(chan error, 1)
		before := session.SchemaGeneration()
		go func() {
			_, err := executeDdlStatements(ctx, session, []string{ddl})
			errc <- err
		}()

		waitForClosed(t, guard, server.accepted, "accepted UpdateDatabaseDdl")
		waitForClosed(t, guard, server.polled, "first GetOperation poll")
		cancel()

		var err error
		select {
		case err = <-errc:
		case <-guard.Done():
			t.Fatal("timed out waiting for canceled DDL wait")
		}
		if err == nil || !strings.Contains(err.Error(), "SHOW OPERATION 'op-ddl'") {
			t.Fatalf("error = %v, want canceled wait hint", err)
		}
		if session.SchemaGeneration() != before+1 {
			t.Fatalf("schema generation = %d, want %d", session.SchemaGeneration(), before+1)
		}
	})

	t.Run("progress bar forced complete", func(t *testing.T) {
		t.Parallel()
		server := newCompletedDDLServer(ddl, commitTS)
		server.metadata = mustDDLMetadata(ddl, commitTS, 40, 100)
		session := newDDLAdminSession(t, server)
		session.systemVariables.Display.EnableProgressBar = true
		tty, err := os.CreateTemp(t.TempDir(), "ddl-bar-*.txt")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = tty.Close() })
		session.systemVariables.StreamManager = streamio.NewStreamManager(io.NopCloser(strings.NewReader("")), io.Discard, io.Discard)
		session.systemVariables.StreamManager.SetTtyStream(tty)
		got, err := executeDdlStatements(t.Context(), session, []string{ddl})
		if err != nil {
			t.Fatalf("executeDdlStatements() error = %v", err)
		}
		if !got.CommitTimestamp.Equal(commitTS) {
			t.Fatalf("CommitTimestamp = %v, want %v", got.CommitTimestamp, commitTS)
		}
	})
}

type ddlAdminTestServer struct {
	databasepb.UnimplementedDatabaseAdminServer
	longrunningpb.UnimplementedOperationsServer

	mu           sync.Mutex
	updateErr    error
	getErr       error
	stayPending  bool
	done         bool
	opName       string
	metadata     *anypb.Any
	lastUpdate   *databasepb.UpdateDatabaseDdlRequest
	getCalls     atomic.Int32
	accepted     chan struct{}
	acceptedOnce sync.Once
	polled       chan struct{}
	pollOnce     sync.Once
}

func (s *ddlAdminTestServer) notifyAccepted() {
	if s.accepted == nil {
		return
	}
	s.acceptedOnce.Do(func() { close(s.accepted) })
}

func (s *ddlAdminTestServer) notifyPolled() {
	if s.polled == nil {
		return
	}
	s.pollOnce.Do(func() { close(s.polled) })
}

func waitForClosed(t *testing.T, ctx context.Context, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for %s", what)
	}
}

func newCompletedDDLServer(ddl string, commitTS time.Time) *ddlAdminTestServer {
	return &ddlAdminTestServer{
		opName:   ddlRPCOpName,
		done:     true,
		metadata: mustDDLMetadata(ddl, commitTS, 100),
	}
}

func mustDDLMetadata(ddl string, commitTS time.Time, percents ...int32) *anypb.Any {
	progress := make([]*databasepb.OperationProgress, len(percents))
	for i, p := range percents {
		progress[i] = &databasepb.OperationProgress{ProgressPercent: p}
	}
	md, err := anypb.New(&databasepb.UpdateDatabaseDdlMetadata{
		Statements:       []string{ddl},
		CommitTimestamps: []*timestamppb.Timestamp{timestamppb.New(commitTS)},
		Progress:         progress,
	})
	if err != nil {
		panic(err)
	}
	return md
}

func (s *ddlAdminTestServer) operation() *longrunningpb.Operation {
	s.mu.Lock()
	defer s.mu.Unlock()
	op := &longrunningpb.Operation{
		Name:     s.opName,
		Done:     s.done && !s.stayPending,
		Metadata: s.metadata,
	}
	if op.Done {
		resp, err := anypb.New(&emptypb.Empty{})
		if err != nil {
			panic(err)
		}
		op.Result = &longrunningpb.Operation_Response{Response: resp}
	}
	return op
}

func (s *ddlAdminTestServer) UpdateDatabaseDdl(_ context.Context, req *databasepb.UpdateDatabaseDdlRequest) (*longrunningpb.Operation, error) {
	s.mu.Lock()
	s.lastUpdate = proto.Clone(req).(*databasepb.UpdateDatabaseDdlRequest)
	err := s.updateErr
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	op := s.operation()
	s.notifyAccepted()
	return op, nil
}

func (s *ddlAdminTestServer) GetOperation(context.Context, *longrunningpb.GetOperationRequest) (*longrunningpb.Operation, error) {
	s.getCalls.Add(1)
	s.mu.Lock()
	err := s.getErr
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	op := s.operation()
	s.notifyPolled()
	return op, nil
}

func newDDLAdminSession(t *testing.T, server *ddlAdminTestServer) *Session {
	t.Helper()
	listener := bufconn.Listen(1 << 20)
	grpcServer := grpc.NewServer()
	databasepb.RegisterDatabaseAdminServer(grpcServer, server)
	longrunningpb.RegisterOperationsServer(grpcServer, server)
	go func() {
		if err := grpcServer.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Errorf("serve ddl admin: %v", err)
		}
	}()
	t.Cleanup(func() {
		grpcServer.Stop()
		_ = listener.Close()
	})
	conn, err := grpc.NewClient("passthrough:///ddl-admin",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	adminClient, err := adminapi.NewDatabaseAdminClient(t.Context(), option.WithGRPCConn(conn))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = adminClient.Close() })

	sysVars := newSystemVariablesWithDefaultsForTest()
	identity := ConnectionVars{Project: "test", Instance: "test", Database: "test"}
	sysVars.Connection = identity
	session := &Session{
		adminClient:     adminClient,
		systemVariables: sysVars,
		connection:      identity,
	}
	return session
}
