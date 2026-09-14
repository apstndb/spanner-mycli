// Copyright 2026 apstndb
//
// Licensed under the MIT License.

package mycli

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/longrunning/autogen/longrunningpb"
	adminapi "cloud.google.com/go/spanner/admin/database/apiv1"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/apstndb/spanner-mycli/enums"
	"google.golang.org/api/option"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type scriptedDDLStep struct {
	createErr error
	done      bool
	stay      bool
	errCode   codes.Code
	errMsg    string
	meta      *databasepb.UpdateDatabaseDdlMetadata
	getErr    error
}

type scriptedDDLServer struct {
	databasepb.UnimplementedDatabaseAdminServer
	longrunningpb.UnimplementedOperationsServer

	mu     sync.Mutex
	n      int
	reqs   []*databasepb.UpdateDatabaseDdlRequest
	ops    map[string]*longrunningpb.Operation
	getErr map[string]error
	step   func(req *databasepb.UpdateDatabaseDdlRequest, n int) scriptedDDLStep
}

func (s *scriptedDDLServer) UpdateDatabaseDdl(_ context.Context, req *databasepb.UpdateDatabaseDdlRequest) (*longrunningpb.Operation, error) {
	s.mu.Lock()
	s.reqs = append(s.reqs, proto.Clone(req).(*databasepb.UpdateDatabaseDdlRequest))
	n := s.n
	s.n++
	step := s.step(req, n)
	s.mu.Unlock()
	if step.createErr != nil {
		return nil, step.createErr
	}
	name := ddlRPCOpName + "-" + strings.ReplaceAll(strings.Join(req.GetStatements(), "/"), " ", "_")
	op := &longrunningpb.Operation{Name: name, Done: step.done && !step.stay}
	if step.meta != nil {
		md := proto.Clone(step.meta).(*databasepb.UpdateDatabaseDdlMetadata)
		if md.Database == "" {
			md.Database = req.GetDatabase()
		}
		if len(md.Statements) == 0 {
			md.Statements = append([]string(nil), req.GetStatements()...)
		}
		anyMD, err := anypb.New(md)
		if err != nil {
			return nil, err
		}
		op.Metadata = anyMD
	}
	if op.Done && step.errMsg != "" {
		op.Result = &longrunningpb.Operation_Error{Error: &statuspb.Status{Code: int32(step.errCode), Message: step.errMsg}}
	} else if op.Done {
		op.Result = &longrunningpb.Operation_Response{Response: mustEmptyAny()}
	}
	s.mu.Lock()
	if s.ops == nil {
		s.ops = map[string]*longrunningpb.Operation{}
	}
	if s.getErr == nil {
		s.getErr = map[string]error{}
	}
	s.ops[name] = proto.Clone(op).(*longrunningpb.Operation)
	if step.getErr != nil {
		s.getErr[name] = step.getErr
	}
	s.mu.Unlock()
	return op, nil
}

func (s *scriptedDDLServer) GetOperation(_ context.Context, req *longrunningpb.GetOperationRequest) (*longrunningpb.Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.getErr[req.Name]; err != nil {
		return nil, err
	}
	if op, ok := s.ops[req.Name]; ok && op != nil {
		return proto.Clone(op).(*longrunningpb.Operation), nil
	}
	return nil, status.Errorf(codes.NotFound, "unknown op %s", req.Name)
}

func mustEmptyAny() *anypb.Any {
	a, err := anypb.New(&emptypb.Empty{})
	if err != nil {
		panic(err)
	}
	return a
}

func newScriptedDDLSession(t *testing.T, step func(*databasepb.UpdateDatabaseDdlRequest, int) scriptedDDLStep) (*Session, *scriptedDDLServer) {
	t.Helper()
	server := &scriptedDDLServer{step: step}
	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer()
	databasepb.RegisterDatabaseAdminServer(gs, server)
	longrunningpb.RegisterOperationsServer(gs, server)
	go func() {
		if err := gs.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Errorf("serve: %v", err)
		}
	}()
	t.Cleanup(func() {
		gs.Stop()
		_ = lis.Close()
	})
	conn, err := grpc.NewClient("passthrough:///seq-kind",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	admin, err := adminapi.NewDatabaseAdminClient(t.Context(), option.WithGRPCConn(conn))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	sysVars := newSystemVariablesWithDefaultsForTest()
	identity := ConnectionVars{Project: "test", Instance: "test", Database: "test"}
	sysVars.Connection = identity
	return &Session{adminClient: admin, systemVariables: sysVars, connection: identity}, server
}

func enableKind(session *Session) {
	session.systemVariables.Feature.DefaultSequenceKind = defaultSequenceKindValue
}

func successStep(ts time.Time) scriptedDDLStep {
	return scriptedDDLStep{
		done: true,
		meta: &databasepb.UpdateDatabaseDdlMetadata{
			CommitTimestamps: []*timestamppb.Timestamp{timestamppb.New(ts)},
		},
	}
}

func missingKindStep(ts ...*timestamppb.Timestamp) scriptedDDLStep {
	return scriptedDDLStep{
		done:    true,
		errCode: codes.InvalidArgument,
		errMsg:  missingKindSentence,
		meta:    &databasepb.UpdateDatabaseDdlMetadata{CommitTimestamps: ts},
	}
}

func TestSequenceKindRepairRPC(t *testing.T) {
	t.Parallel()
	commit := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	userDDL := "CREATE TABLE t (id INT64 AUTO_INCREMENT) PRIMARY KEY (id)"
	s1 := "CREATE TABLE a (id INT64) PRIMARY KEY (id)"
	s2 := "CREATE TABLE b (id INT64 AUTO_INCREMENT) PRIMARY KEY (id)"

	t.Run("opt-out no extra ddl", func(t *testing.T) {
		t.Parallel()
		session, server := newScriptedDDLSession(t, func(req *databasepb.UpdateDatabaseDdlRequest, n int) scriptedDDLStep {
			return missingKindStep()
		})
		before := session.SchemaGeneration()
		_, err := executeDdlStatements(t.Context(), session, []string{userDDL})
		if err == nil || !isMissingDefaultSequenceKindError(err) {
			t.Fatalf("err=%v", err)
		}
		if len(server.reqs) != 1 {
			t.Fatalf("rpc count=%d want 1", len(server.reqs))
		}
		if session.SchemaGeneration() != before+1 {
			t.Fatalf("gen=%d want %d", session.SchemaGeneration(), before+1)
		}
	})

	t.Run("unrelated invalid argument", func(t *testing.T) {
		t.Parallel()
		session, server := newScriptedDDLSession(t, func(req *databasepb.UpdateDatabaseDdlRequest, n int) scriptedDDLStep {
			return scriptedDDLStep{done: true, errCode: codes.InvalidArgument, errMsg: "syntax error", meta: &databasepb.UpdateDatabaseDdlMetadata{}}
		})
		enableKind(session)
		_, err := executeDdlStatements(t.Context(), session, []string{"CREATE TABEL typo"})
		if err == nil || isMissingDefaultSequenceKindError(err) {
			t.Fatalf("err=%v", err)
		}
		if len(server.reqs) != 1 {
			t.Fatalf("rpc count=%d", len(server.reqs))
		}
	})

	t.Run("create rejection prefix0 then alter and retry", func(t *testing.T) {
		t.Parallel()
		session, server := newScriptedDDLSession(t, func(req *databasepb.UpdateDatabaseDdlRequest, n int) scriptedDDLStep {
			if n == 0 {
				return scriptedDDLStep{createErr: status.Error(codes.InvalidArgument, missingKindSentence)}
			}
			return successStep(commit)
		})
		enableKind(session)
		session.systemVariables.Internal.ProtoDescriptor = nil
		got, err := executeDdlStatements(t.Context(), session, []string{userDDL})
		if err != nil {
			t.Fatal(err)
		}
		if len(server.reqs) != 3 {
			t.Fatalf("rpc count=%d want 3", len(server.reqs))
		}
		if !strings.Contains(server.reqs[1].GetStatements()[0], "ALTER DATABASE `test`") {
			t.Fatalf("alter=%v", server.reqs[1].GetStatements())
		}
		if len(server.reqs[1].GetProtoDescriptors()) != 0 {
			t.Fatalf("ALTER carried descriptors")
		}
		if got := strings.Join(server.reqs[2].GetStatements(), "|"); got != userDDL {
			t.Fatalf("suffix=%q", got)
		}
		if got.CommitTimestamp.IsZero() {
			t.Fatal("missing commit timestamp")
		}
	})

	t.Run("partial success suffix only", func(t *testing.T) {
		t.Parallel()
		session, server := newScriptedDDLSession(t, func(req *databasepb.UpdateDatabaseDdlRequest, n int) scriptedDDLStep {
			if n == 0 {
				return missingKindStep(timestamppb.New(commit))
			}
			return successStep(commit.Add(time.Minute))
		})
		enableKind(session)
		session.systemVariables.Internal.ProtoDescriptor = &descriptorpb.FileDescriptorSet{
			File: []*descriptorpb.FileDescriptorProto{{Name: proto.String("x.proto")}},
		}
		_, err := executeDdlStatements(t.Context(), session, []string{s1, s2})
		if err != nil {
			t.Fatal(err)
		}
		if len(server.reqs) != 3 {
			t.Fatalf("rpc count=%d", len(server.reqs))
		}
		if got := strings.Join(server.reqs[2].GetStatements(), "|"); got != s2 {
			t.Fatalf("replayed %q", got)
		}
		if len(server.reqs[0].GetProtoDescriptors()) == 0 || len(server.reqs[2].GetProtoDescriptors()) == 0 {
			t.Fatal("descriptors dropped")
		}
		if string(server.reqs[0].GetProtoDescriptors()) != string(server.reqs[2].GetProtoDescriptors()) {
			t.Fatal("descriptor bytes changed")
		}
		if len(server.reqs[1].GetProtoDescriptors()) != 0 {
			t.Fatal("ALTER had descriptors")
		}
	})

	t.Run("empty timestamps prefix0", func(t *testing.T) {
		t.Parallel()
		session, server := newScriptedDDLSession(t, func(req *databasepb.UpdateDatabaseDdlRequest, n int) scriptedDDLStep {
			if n == 0 {
				return missingKindStep()
			}
			return successStep(commit)
		})
		enableKind(session)
		if _, err := executeDdlStatements(t.Context(), session, []string{userDDL}); err != nil {
			t.Fatal(err)
		}
		if len(server.reqs) != 3 {
			t.Fatalf("rpc count=%d", len(server.reqs))
		}
		if got := strings.Join(server.reqs[2].GetStatements(), "|"); got != userDDL {
			t.Fatalf("suffix=%q", got)
		}
	})

	t.Run("tail padded metadata", func(t *testing.T) {
		t.Parallel()
		session, server := newScriptedDDLSession(t, func(req *databasepb.UpdateDatabaseDdlRequest, n int) scriptedDDLStep {
			if n == 0 {
				return missingKindStep(timestamppb.New(commit), &timestamppb.Timestamp{})
			}
			return successStep(commit)
		})
		enableKind(session)
		if _, err := executeDdlStatements(t.Context(), session, []string{s1, s2}); err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(server.reqs[2].GetStatements(), "|"); got != s2 {
			t.Fatalf("suffix=%q", got)
		}
	})

	t.Run("missing metadata no repair", func(t *testing.T) {
		t.Parallel()
		session, server := newScriptedDDLSession(t, func(req *databasepb.UpdateDatabaseDdlRequest, n int) scriptedDDLStep {
			return scriptedDDLStep{done: true, errCode: codes.InvalidArgument, errMsg: missingKindSentence}
		})
		enableKind(session)
		_, err := executeDdlStatements(t.Context(), session, []string{userDDL})
		if err == nil {
			t.Fatal("want original error")
		}
		if len(server.reqs) != 1 {
			t.Fatalf("rpc count=%d want 1", len(server.reqs))
		}
	})

	t.Run("holey metadata no repair", func(t *testing.T) {
		t.Parallel()
		session, server := newScriptedDDLSession(t, func(req *databasepb.UpdateDatabaseDdlRequest, n int) scriptedDDLStep {
			return missingKindStep(timestamppb.New(commit), nil, timestamppb.New(commit))
		})
		enableKind(session)
		_, err := executeDdlStatements(t.Context(), session, []string{s1, s2, userDDL})
		if err == nil {
			t.Fatal("want original")
		}
		if len(server.reqs) != 1 {
			t.Fatalf("rpc count=%d", len(server.reqs))
		}
	})

	t.Run("covers all statements no repair", func(t *testing.T) {
		t.Parallel()
		session, server := newScriptedDDLSession(t, func(req *databasepb.UpdateDatabaseDdlRequest, n int) scriptedDDLStep {
			return missingKindStep(timestamppb.New(commit), timestamppb.New(commit))
		})
		enableKind(session)
		_, err := executeDdlStatements(t.Context(), session, []string{s1, s2})
		if err == nil {
			t.Fatal("want original")
		}
		if len(server.reqs) != 1 {
			t.Fatalf("rpc count=%d", len(server.reqs))
		}
	})

	t.Run("nonterminal poll failure no repair", func(t *testing.T) {
		t.Parallel()
		session, server := newScriptedDDLSession(t, func(req *databasepb.UpdateDatabaseDdlRequest, n int) scriptedDDLStep {
			return scriptedDDLStep{done: false, stay: true, meta: &databasepb.UpdateDatabaseDdlMetadata{}, getErr: status.Error(codes.Unavailable, "poll failed")}
		})
		enableKind(session)
		_, err := executeDdlStatements(t.Context(), session, []string{userDDL})
		if err == nil {
			t.Fatal("want poll error")
		}
		if len(server.reqs) != 1 {
			t.Fatalf("rpc count=%d", len(server.reqs))
		}
	})

	t.Run("alter failure stops without suffix", func(t *testing.T) {
		t.Parallel()
		session, server := newScriptedDDLSession(t, func(req *databasepb.UpdateDatabaseDdlRequest, n int) scriptedDDLStep {
			if n == 0 {
				return missingKindStep()
			}
			return scriptedDDLStep{done: true, errCode: codes.FailedPrecondition, errMsg: "option already set", meta: &databasepb.UpdateDatabaseDdlMetadata{}}
		})
		enableKind(session)
		_, err := executeDdlStatements(t.Context(), session, []string{userDDL})
		if err == nil || !strings.Contains(err.Error(), "ALTER DATABASE") {
			t.Fatalf("err=%v", err)
		}
		if !strings.Contains(err.Error(), "original DDL error") {
			t.Fatalf("missing original cause: %v", err)
		}
		if len(server.reqs) != 2 {
			t.Fatalf("rpc count=%d want 2 (no suffix)", len(server.reqs))
		}
	})

	t.Run("suffix failure does not loop", func(t *testing.T) {
		t.Parallel()
		session, server := newScriptedDDLSession(t, func(req *databasepb.UpdateDatabaseDdlRequest, n int) scriptedDDLStep {
			if n == 1 {
				return successStep(commit)
			}
			return missingKindStep()
		})
		enableKind(session)
		_, err := executeDdlStatements(t.Context(), session, []string{userDDL})
		if err == nil || !strings.Contains(err.Error(), "suffix retry") {
			t.Fatalf("err=%v", err)
		}
		if len(server.reqs) != 3 {
			t.Fatalf("rpc count=%d want 3", len(server.reqs))
		}
	})

	t.Run("async no repair", func(t *testing.T) {
		t.Parallel()
		session, server := newScriptedDDLSession(t, func(req *databasepb.UpdateDatabaseDdlRequest, n int) scriptedDDLStep {
			return scriptedDDLStep{done: false, stay: true, meta: &databasepb.UpdateDatabaseDdlMetadata{}}
		})
		enableKind(session)
		session.systemVariables.Feature.DDLExecutionMode = enums.DDLExecutionModeAsync
		if _, err := executeDdlStatements(t.Context(), session, []string{userDDL}); err != nil {
			t.Fatal(err)
		}
		if len(server.reqs) != 1 {
			t.Fatalf("ASYNC rpc count=%d", len(server.reqs))
		}
	})

	t.Run("async wait completed error no repair", func(t *testing.T) {
		t.Parallel()
		session, server := newScriptedDDLSession(t, func(req *databasepb.UpdateDatabaseDdlRequest, n int) scriptedDDLStep {
			return missingKindStep()
		})
		enableKind(session)
		session.systemVariables.Feature.DDLExecutionMode = enums.DDLExecutionModeAsyncWait
		_, err := executeDdlStatements(t.Context(), session, []string{userDDL})
		if err == nil {
			t.Fatal("want completed error")
		}
		if len(server.reqs) != 1 {
			t.Fatalf("ASYNC_WAIT rpc count=%d", len(server.reqs))
		}
	})

	t.Run("googlesql hyphenated identifier", func(t *testing.T) {
		t.Parallel()
		session, server := newScriptedDDLSession(t, func(req *databasepb.UpdateDatabaseDdlRequest, n int) scriptedDDLStep {
			if n == 0 {
				return missingKindStep()
			}
			return successStep(commit)
		})
		enableKind(session)
		session.connection.Database = "my-db"
		if _, err := executeDdlStatements(t.Context(), session, []string{userDDL}); err != nil {
			t.Fatal(err)
		}
		want := "ALTER DATABASE `my-db` SET OPTIONS (default_sequence_kind = 'bit_reversed_positive')"
		if got := server.reqs[1].GetStatements()[0]; got != want {
			t.Fatalf("alter=%q want %q", got, want)
		}
	})

	t.Run("postgresql hyphenated identifier", func(t *testing.T) {
		t.Parallel()
		session, server := newScriptedDDLSession(t, func(req *databasepb.UpdateDatabaseDdlRequest, n int) scriptedDDLStep {
			if n == 0 {
				return missingKindStep()
			}
			return successStep(commit)
		})
		enableKind(session)
		session.connection.Database = "my-db"
		session.systemVariables.Feature.DatabaseDialect = databasepb.DatabaseDialect_POSTGRESQL
		if _, err := executeDdlStatements(t.Context(), session, []string{userDDL}); err != nil {
			t.Fatal(err)
		}
		want := `ALTER DATABASE "my-db" SET spanner.default_sequence_kind = 'bit_reversed_positive'`
		if got := server.reqs[1].GetStatements()[0]; got != want {
			t.Fatalf("alter=%q want %q", got, want)
		}
	})

	t.Run("echo successful phases in order", func(t *testing.T) {
		t.Parallel()
		session, _ := newScriptedDDLSession(t, func(req *databasepb.UpdateDatabaseDdlRequest, n int) scriptedDDLStep {
			if n == 0 {
				return missingKindStep(timestamppb.New(commit))
			}
			return successStep(commit.Add(time.Duration(n) * time.Minute))
		})
		enableKind(session)
		session.systemVariables.Feature.EchoExecutedDDL = true
		got, err := executeDdlStatements(t.Context(), session, []string{s1, s2})
		if err != nil {
			t.Fatal(err)
		}
		rows := got.presentationRows()
		if len(rows) != 3 {
			t.Fatalf("echo rows=%d want 3: %v", len(rows), rows)
		}
		if !strings.HasPrefix(rows[0][0].RawText(), s1) {
			t.Fatalf("row0=%q", rows[0][0].RawText())
		}
		if !strings.Contains(rows[1][0].RawText(), "ALTER DATABASE") {
			t.Fatalf("row1=%q", rows[1][0].RawText())
		}
		if !strings.HasPrefix(rows[2][0].RawText(), s2) {
			t.Fatalf("row2=%q", rows[2][0].RawText())
		}
	})

	t.Run("caller cancel before repair", func(t *testing.T) {
		t.Parallel()
		session, server := newScriptedDDLSession(t, func(req *databasepb.UpdateDatabaseDdlRequest, n int) scriptedDDLStep {
			return missingKindStep()
		})
		enableKind(session)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err := executeDdlStatements(ctx, session, []string{userDDL})
		if err == nil {
			t.Fatal("want error")
		}
		if len(server.reqs) != 0 {
			t.Fatalf("canceled caller issued %d RPCs, want 0", len(server.reqs))
		}
	})
}
