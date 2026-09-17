// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
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
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/longrunning/autogen/longrunningpb"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/apstndb/spanner-mycli/enums"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type scriptedDDLStep struct {
	createErr           error
	done                bool
	stay                bool
	errCode             codes.Code
	errMsg              string
	meta                *databasepb.UpdateDatabaseDdlMetadata
	getErr              error
	blockGetUntilCancel bool
	requireLiveCtx      bool
	accepted            chan struct{}
	acceptedOnce        *sync.Once
	polled              chan struct{}
	polledOnce          *sync.Once
}

type scriptedDDLServer struct {
	databasepb.UnimplementedDatabaseAdminServer
	longrunningpb.UnimplementedOperationsServer

	mu           sync.Mutex
	n            int
	reqs         []*databasepb.UpdateDatabaseDdlRequest
	createCtxErr []error
	ops          map[string]*longrunningpb.Operation
	getErr       map[string]error
	hangGet      map[string]bool
	pollCh       map[string]chan struct{}
	pollOnce     map[string]*sync.Once
	step         func(req *databasepb.UpdateDatabaseDdlRequest, n int) scriptedDDLStep
}

func notifyOnce(ch chan struct{}, once *sync.Once) {
	if ch == nil {
		return
	}
	if once == nil {
		close(ch)
		return
	}
	once.Do(func() { close(ch) })
}

func (s *scriptedDDLServer) UpdateDatabaseDdl(ctx context.Context, req *databasepb.UpdateDatabaseDdlRequest) (*longrunningpb.Operation, error) {
	s.mu.Lock()
	s.reqs = append(s.reqs, proto.Clone(req).(*databasepb.UpdateDatabaseDdlRequest))
	s.createCtxErr = append(s.createCtxErr, ctx.Err())
	n := s.n
	s.n++
	step := s.step(req, n)
	s.mu.Unlock()
	notifyOnce(step.accepted, step.acceptedOnce)
	if step.requireLiveCtx && ctx.Err() != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "repair RPC %d saw canceled context: %v", n, ctx.Err())
	}
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
	if s.hangGet == nil {
		s.hangGet = map[string]bool{}
	}
	if s.pollCh == nil {
		s.pollCh = map[string]chan struct{}{}
	}
	if s.pollOnce == nil {
		s.pollOnce = map[string]*sync.Once{}
	}
	s.ops[name] = proto.Clone(op).(*longrunningpb.Operation)
	if step.getErr != nil {
		s.getErr[name] = step.getErr
	}
	if step.blockGetUntilCancel {
		s.hangGet[name] = true
	}
	if step.polled != nil {
		s.pollCh[name] = step.polled
		s.pollOnce[name] = step.polledOnce
	}
	s.mu.Unlock()
	return op, nil
}

func (s *scriptedDDLServer) GetOperation(ctx context.Context, req *longrunningpb.GetOperationRequest) (*longrunningpb.Operation, error) {
	s.mu.Lock()
	if ch := s.pollCh[req.Name]; ch != nil {
		notifyOnce(ch, s.pollOnce[req.Name])
	}
	hang := s.hangGet[req.Name]
	if err := s.getErr[req.Name]; err != nil {
		s.mu.Unlock()
		return nil, err
	}
	var op *longrunningpb.Operation
	if stored, ok := s.ops[req.Name]; ok && stored != nil {
		op = proto.Clone(stored).(*longrunningpb.Operation)
	}
	s.mu.Unlock()
	if hang {
		<-ctx.Done()
		return nil, status.FromContextError(ctx.Err()).Err()
	}
	if op != nil {
		return op, nil
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
	return newBufconnAdminSession(t, server), server
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

	t.Run("alter create rejection no suffix", func(t *testing.T) {
		t.Parallel()
		later := status.Error(codes.FailedPrecondition, "alter create rejected")
		session, server := newScriptedDDLSession(t, func(req *databasepb.UpdateDatabaseDdlRequest, n int) scriptedDDLStep {
			if n == 0 {
				return missingKindStep()
			}
			return scriptedDDLStep{createErr: later, requireLiveCtx: true}
		})
		enableKind(session)
		before := session.SchemaGeneration()
		_, err := executeDdlStatements(t.Context(), session, []string{userDDL})
		requireRepairPhaseError(t, err, "ALTER DATABASE", "alter create rejected")
		requireExactDDLRequests(t, server.reqs, [][]string{
			{userDDL},
			{wantGoogleSQLAlter("test")},
		})
		if len(server.reqs[1].GetProtoDescriptors()) != 0 {
			t.Fatal("ALTER carried descriptors")
		}
		requireCreateCtxLive(t, server, 1)
		if session.SchemaGeneration() != before+1 {
			t.Fatalf("gen=%d want %d", session.SchemaGeneration(), before+1)
		}
	})

	t.Run("suffix create rejection does not loop", func(t *testing.T) {
		t.Parallel()
		later := status.Error(codes.InvalidArgument, missingKindSentence)
		session, server := newScriptedDDLSession(t, func(req *databasepb.UpdateDatabaseDdlRequest, n int) scriptedDDLStep {
			switch n {
			case 0:
				return missingKindStep()
			case 1:
				return successStep(commit)
			default:
				return scriptedDDLStep{createErr: later, requireLiveCtx: true}
			}
		})
		enableKind(session)
		before := session.SchemaGeneration()
		_, err := executeDdlStatements(t.Context(), session, []string{userDDL})
		requireRepairPhaseError(t, err, "suffix retry", missingKindSentence)
		requireExactDDLRequests(t, server.reqs, [][]string{
			{userDDL},
			{wantGoogleSQLAlter("test")},
			{userDDL},
		})
		if len(server.reqs) != 3 {
			t.Fatalf("rpc count=%d want 3 (no recursive repair)", len(server.reqs))
		}
		requireCreateCtxLive(t, server, 1, 2)
		if session.SchemaGeneration() != before+2 {
			t.Fatalf("gen=%d want %d", session.SchemaGeneration(), before+2)
		}
	})

	t.Run("in-flight cancel during alter wait", func(t *testing.T) {
		t.Parallel()
		accepted := make(chan struct{})
		polled := make(chan struct{})
		var acceptedOnce, polledOnce sync.Once
		session, server := newScriptedDDLSession(t, func(req *databasepb.UpdateDatabaseDdlRequest, n int) scriptedDDLStep {
			if n == 0 {
				return missingKindStep()
			}
			return scriptedDDLStep{
				stay:                true,
				requireLiveCtx:      true,
				blockGetUntilCancel: true,
				accepted:            accepted,
				acceptedOnce:        &acceptedOnce,
				polled:              polled,
				polledOnce:          &polledOnce,
				meta:                &databasepb.UpdateDatabaseDdlMetadata{},
			}
		})
		enableKind(session)
		before := session.SchemaGeneration()
		caller := newRepairPhaseCtx(t.Context())
		guard, guardCancel := context.WithTimeout(t.Context(), 30*time.Second)
		defer guardCancel()
		errc := make(chan error, 1)
		go func() {
			_, err := executeDdlStatements(caller, session, []string{userDDL})
			errc <- err
		}()
		waitForClosed(t, guard, accepted, "accepted ALTER UpdateDatabaseDdl")
		requireCreateCtxLive(t, server, 1)
		waitForClosed(t, guard, polled, "first ALTER GetOperation poll")
		caller.cancel()
		var err error
		select {
		case err = <-errc:
		case <-guard.Done():
			t.Fatal("timed out waiting for canceled ALTER wait")
		}
		requireRepairPhaseError(t, err, "ALTER DATABASE", "")
		if !isCancellationError(err) && !errors.Is(err, context.Canceled) {
			t.Fatalf("want canceled later cause: %v", err)
		}
		if !strings.Contains(err.Error(), "SHOW OPERATION") {
			t.Fatalf("want SHOW OPERATION hint: %v", err)
		}
		requireExactDDLRequests(t, server.reqs, [][]string{
			{userDDL},
			{wantGoogleSQLAlter("test")},
		})
		if session.SchemaGeneration() != before+2 {
			t.Fatalf("gen=%d want %d", session.SchemaGeneration(), before+2)
		}
	})

	t.Run("in-flight deadline during suffix wait", func(t *testing.T) {
		t.Parallel()
		accepted := make(chan struct{})
		polled := make(chan struct{})
		var acceptedOnce, polledOnce sync.Once
		session, server := newScriptedDDLSession(t, func(req *databasepb.UpdateDatabaseDdlRequest, n int) scriptedDDLStep {
			switch n {
			case 0:
				return missingKindStep()
			case 1:
				return successStep(commit)
			default:
				return scriptedDDLStep{
					stay:                true,
					requireLiveCtx:      true,
					blockGetUntilCancel: true,
					accepted:            accepted,
					acceptedOnce:        &acceptedOnce,
					polled:              polled,
					polledOnce:          &polledOnce,
					meta:                &databasepb.UpdateDatabaseDdlMetadata{},
				}
			}
		})
		enableKind(session)
		before := session.SchemaGeneration()
		caller := newRepairPhaseCtx(t.Context())
		guard, guardCancel := context.WithTimeout(t.Context(), 30*time.Second)
		defer guardCancel()
		errc := make(chan error, 1)
		go func() {
			_, err := executeDdlStatements(caller, session, []string{userDDL})
			errc <- err
		}()
		waitForClosed(t, guard, accepted, "accepted suffix UpdateDatabaseDdl")
		requireCreateCtxLive(t, server, 1, 2)
		waitForClosed(t, guard, polled, "first suffix GetOperation poll")
		caller.expireDeadline()
		var err error
		select {
		case err = <-errc:
		case <-guard.Done():
			t.Fatal("timed out waiting for suffix deadline")
		}
		requireRepairPhaseError(t, err, "suffix retry", "")
		if !isCancellationError(err) && !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("want deadline later cause: %v", err)
		}
		if !strings.Contains(err.Error(), "SHOW OPERATION") {
			t.Fatalf("want SHOW OPERATION hint: %v", err)
		}
		requireExactDDLRequests(t, server.reqs, [][]string{
			{userDDL},
			{wantGoogleSQLAlter("test")},
			{userDDL},
		})
		if session.SchemaGeneration() != before+3 {
			t.Fatalf("gen=%d want %d", session.SchemaGeneration(), before+3)
		}
	})
}

func wantGoogleSQLAlter(databaseID string) string {
	return "ALTER DATABASE `" + databaseID + "` SET OPTIONS (default_sequence_kind = 'bit_reversed_positive')"
}

func requireExactDDLRequests(t *testing.T, reqs []*databasepb.UpdateDatabaseDdlRequest, want [][]string) {
	t.Helper()
	if len(reqs) != len(want) {
		t.Fatalf("rpc count=%d want %d", len(reqs), len(want))
	}
	for i, stmts := range want {
		if got := reqs[i].GetStatements(); !slices.Equal(got, stmts) {
			t.Fatalf("req[%d]=%v want %v", i, got, stmts)
		}
	}
}

func requireCreateCtxLive(t *testing.T, server *scriptedDDLServer, indexes ...int) {
	t.Helper()
	server.mu.Lock()
	defer server.mu.Unlock()
	for _, i := range indexes {
		if i >= len(server.createCtxErr) {
			t.Fatalf("create ctx[%d] missing; recorded %d", i, len(server.createCtxErr))
		}
		if err := server.createCtxErr[i]; err != nil {
			t.Fatalf("create RPC %d saw %v, want live context", i, err)
		}
	}
}

func requireRepairPhaseError(t *testing.T, err error, phase, laterSubstr string) {
	t.Helper()
	if err == nil {
		t.Fatal("want repair-phase error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "DEFAULT_SEQUENCE_KIND "+phase+" failed") {
		t.Fatalf("phase %q missing: %v", phase, err)
	}
	if !strings.Contains(msg, "original DDL error") {
		t.Fatalf("original cause missing: %v", err)
	}
	if !strings.Contains(msg, missingKindSentence) {
		t.Fatalf("original missing-kind sentence missing: %v", err)
	}
	if laterSubstr != "" && !strings.Contains(msg, laterSubstr) {
		t.Fatalf("later %q missing: %v", laterSubstr, err)
	}
}

// repairPhaseCtx starts live so the initial missing-kind DDL and the first
// repair RPC share the original budget. Tests expire it only after the fake
// has accepted the in-flight repair operation.
type repairPhaseCtx struct {
	context.Context
	done     chan struct{}
	once     sync.Once
	mu       sync.Mutex
	err      error
	deadline time.Time
}

func newRepairPhaseCtx(parent context.Context) *repairPhaseCtx {
	return &repairPhaseCtx{Context: parent, done: make(chan struct{})}
}

func (c *repairPhaseCtx) Done() <-chan struct{} { return c.done }

func (c *repairPhaseCtx) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

func (c *repairPhaseCtx) Deadline() (time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.deadline.IsZero() {
		return time.Time{}, false
	}
	return c.deadline, true
}

func (c *repairPhaseCtx) cancel() {
	c.finish(context.Canceled, time.Time{})
}

func (c *repairPhaseCtx) expireDeadline() {
	c.finish(context.DeadlineExceeded, time.Now())
}

func (c *repairPhaseCtx) finish(err error, deadline time.Time) {
	c.once.Do(func() {
		c.mu.Lock()
		c.err = err
		c.deadline = deadline
		c.mu.Unlock()
		close(c.done)
	})
}
