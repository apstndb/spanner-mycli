// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
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
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type queueMutationServer struct {
	heartbeatRPCServer
	requestsMu sync.Mutex
	requests   []*sppb.CommitRequest
}

func (s *queueMutationServer) Commit(ctx context.Context, req *sppb.CommitRequest) (*sppb.CommitResponse, error) {
	s.requestsMu.Lock()
	s.requests = append(s.requests, proto.Clone(req).(*sppb.CommitRequest))
	s.requestsMu.Unlock()
	return s.heartbeatRPCServer.Commit(ctx, req)
}

func (s *queueMutationServer) commitsSnapshot() []*sppb.CommitRequest {
	s.requestsMu.Lock()
	defer s.requestsMu.Unlock()
	out := make([]*sppb.CommitRequest, len(s.requests))
	for i, req := range s.requests {
		out[i] = proto.Clone(req).(*sppb.CommitRequest)
	}
	return out
}

func newQueueMutationSession(t *testing.T) (*Session, *queueMutationServer) {
	t.Helper()
	server := &queueMutationServer{heartbeatRPCServer: heartbeatRPCServer{heartbeatStarted: make(chan struct{})}}
	session, _ := newBufconnQuerySession(t, server)
	session.txn.abortRetryWait = func(context.Context, time.Duration) error { return nil }
	t.Cleanup(func() {
		_ = session.txn.RollbackReadWriteTransaction(context.Background())
		session.txn.clearTransactionContext()
	})
	return session, server
}

func TestQueueMutationWire(t *testing.T) {
	t.Parallel()
	key := &structpb.ListValue{Values: []*structpb.Value{structpb.NewStringValue("9007199254740993"), structpb.NewStringValue("message-1")}}
	delivery := timestamppb.New(time.Date(2030, 1, 2, 3, 4, 5, 123456000, time.UTC))
	for _, tc := range []struct {
		name, sql string
		want      *sppb.Mutation
	}{
		{
			"send default", `MUTATE Tasks SEND (key => (9007199254740993, 'message-1'), payload => b'hello')`,
			&sppb.Mutation{Operation: &sppb.Mutation_Send_{Send: &sppb.Mutation_Send{Queue: "Tasks", Key: key, Payload: structpb.NewStringValue("aGVsbG8=")}}},
		},
		{
			"send time and mixed case", "mutate `Tasks` send (PAYLOAD => b'hello', KEY => (9007199254740993, 'message-1'), DELIVER_TIME => TIMESTAMP '2030-01-02T03:04:05.123456Z')",
			&sppb.Mutation{Operation: &sppb.Mutation_Send_{Send: &sppb.Mutation_Send{Queue: "Tasks", Key: key, Payload: structpb.NewStringValue("aGVsbG8="), DeliverTime: delivery}}},
		},
		{
			"ack default", `MUTATE Tasks ACK (key => (9007199254740993, 'message-1'))`,
			&sppb.Mutation{Operation: &sppb.Mutation_Ack_{Ack: &sppb.Mutation_Ack{Queue: "Tasks", Key: key}}},
		},
		{
			"ack ignore", `MUTATE Tasks ACK (ignore_not_found => TRUE, key => (9007199254740993, 'message-1'))`,
			&sppb.Mutation{Operation: &sppb.Mutation_Ack_{Ack: &sppb.Mutation_Ack{Queue: "Tasks", Key: key, IgnoreNotFound: true}}},
		},
		{
			"scalar key and null payload", `MUTATE Tasks SEND (key => 'message-1', payload => CAST(NULL AS BYTES))`,
			&sppb.Mutation{Operation: &sppb.Mutation_Send_{Send: &sppb.Mutation_Send{Queue: "Tasks", Key: &structpb.ListValue{Values: []*structpb.Value{structpb.NewStringValue("message-1")}}, Payload: structpb.NewNullValue()}}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			session, server := newQueueMutationSession(t)
			// Strict mode still uses the CLI grammar and existing literal ASTs.
			stmt, err := BuildStatementWithCommentsWithMode(tc.sql, tc.sql, enums.ParseModeMemefishOnly)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := session.ExecuteStatement(t.Context(), stmt); err != nil {
				t.Fatal(err)
			}
			requests := server.commitsSnapshot()
			if len(requests) != 1 {
				t.Fatalf("commits = %d", len(requests))
			}
			if diff := cmp.Diff([]*sppb.Mutation{tc.want}, requests[0].Mutations, protocmp.Transform()); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestQueueMutationRejectsInvalidArguments(t *testing.T) {
	t.Parallel()
	for _, body := range []string{
		`SEND ()`, `SEND (key => 1)`, `SEND (payload => b'x')`,
		`SEND (1, b'x')`, `SEND (key => 1, payload => b'x', typo => TRUE)`,
		`SEND (key => 1, KEY => 2, payload => b'x')`,
		`SEND (key => 1, payload => b'x', ignore_not_found => TRUE)`,
		`SEND (key => [], payload => b'x')`, `SEND (key => [1], payload => b'x')`,
		`SEND (key => STRUCT(), payload => b'x')`, `ACK (key => CAST(NULL AS INT64))`,
		`ACK ALL`, `ACK (key => KEY_RANGE(start_closed => 1, end_open => 2))`,
		`ACK (key => 1, payload => b'x')`, `ACK (key => 1, deliver_time => TIMESTAMP '2030-01-01T00:00:00Z')`,
		`ACK (key => 1, ignore_not_found => 'true')`, `ACK (key => 1, ignore_not_found => CAST(NULL AS BOOL))`,
		`SEND (key => 1, payload => @payload)`,
		`SEND (key => 1, payload => b'x', deliver_time => '2030-01-01')`,
		`SEND (key => 1, payload => b'x', deliver_time => CAST(NULL AS TIMESTAMP))`,
		`SEND (key => 1, payload => b'x', deliver_time => TIMESTAMP '0001-01-01T00:00:00Z')`,
		`SEND (key => 1, payload => b'x', deliver_time => PENDING_COMMIT_TIMESTAMP())`,
		`SEND (DISTINCT key => 1, payload => b'x')`,
		`SEND (key => 1, payload => b'x') @{foo=1}`,
		`ACK (key => 1) + 1`,
	} {
		t.Run(body, func(t *testing.T) {
			t.Parallel()
			stmt, err := BuildStatement("MUTATE Tasks " + body)
			if err != nil {
				return
			}
			session, server := newQueueMutationSession(t)
			if _, err := stmt.Execute(t.Context(), session, OperationOutput{}); err == nil {
				t.Fatal("invalid queue mutation accepted")
			}
			if len(server.commitsSnapshot()) != 0 || len(server.beginObservations()) != 0 {
				t.Fatal("invalid mutation reached the backend")
			}
		})
	}
}

func TestQueueMutationSavepointAndAbortReplay(t *testing.T) {
	t.Parallel()
	session, server := newQueueMutationSession(t)
	ctx := t.Context()
	mustExec(t, ctx, session, "SET CLI_SAVEPOINT_SUPPORT = 'ENABLED'")
	beginExplicitRetry(t, ctx, session)
	mustExec(t, ctx, session, `MUTATE Tasks SEND (key => (1, 'kept'), payload => b'hello', deliver_time => TIMESTAMP '2030-01-02T03:04:05Z')`)
	mustExec(t, ctx, session, `MUTATE Tasks ACK (key => (1, 'processed'), ignore_not_found => TRUE)`)
	mustExec(t, ctx, session, "SAVEPOINT keep")
	mustExec(t, ctx, session, `MUTATE Tasks SEND (key => (1, 'removed'), payload => b'discard')`)
	mustExec(t, ctx, session, "ROLLBACK TO SAVEPOINT keep")
	if len(server.commitsSnapshot()) != 0 {
		t.Fatal("buffered mutations committed before COMMIT")
	}
	server.setFailCommitTimes(1, abortedStatus("retry queue commit"))
	mustExec(t, ctx, session, "COMMIT")
	requests := server.commitsSnapshot()
	if len(requests) != 2 {
		t.Fatalf("commit attempts = %d", len(requests))
	}
	first := requests[0].Mutations
	if len(first) != 2 || first[0].GetSend() == nil || first[1].GetAck() == nil {
		t.Fatalf("wrong replayed prefix: %v", first)
	}
	if first[0].GetSend().GetKey().Values[1].GetStringValue() != "kept" || first[0].GetSend().GetPayload().GetStringValue() != "aGVsbG8=" || first[0].GetSend().GetDeliverTime() == nil || !first[1].GetAck().GetIgnoreNotFound() {
		t.Fatalf("lost frozen values/options: %v", first)
	}
	if diff := cmp.Diff(first, requests[1].Mutations, protocmp.Transform()); diff != "" {
		t.Fatal(diff)
	}
	if string(requests[0].GetTransactionId()) == string(requests[1].GetTransactionId()) {
		t.Fatal("ABORTED commit did not use a new transaction")
	}
}

func TestQueueMutationReadOnlyAndServerErrors(t *testing.T) {
	t.Parallel()
	for _, sql := range []string{`MUTATE Tasks SEND (key => 1, payload => b'x')`, `MUTATE Tasks ACK (key => 1)`} {
		t.Run(sql, func(t *testing.T) {
			t.Parallel()
			session, server := newQueueMutationSession(t)
			session.systemVariables.Transaction.ReadOnly = true
			if _, err := execSQL(t, t.Context(), session, sql); !errors.Is(err, errReadOnly) {
				t.Fatalf("READONLY: %v", err)
			}
			if len(server.commitsSnapshot()) != 0 {
				t.Fatal("READONLY committed")
			}
			session.systemVariables.Transaction.ReadOnly = false
			server.setFailCommit(status.Error(codes.NotFound, "queue or message missing"))
			if _, err := execSQL(t, t.Context(), session, sql); spanner.ErrCode(err) != codes.NotFound {
				t.Fatalf("server error: %v", err)
			}
			if len(server.commitsSnapshot()) != 1 {
				t.Fatal("non-ABORTED error was retried")
			}
		})
	}
}

func TestQueueMutationJournalPayloadAccounting(t *testing.T) {
	t.Parallel()
	small, _, err := freezeMutate("Tasks", "SEND", `(key => 1, payload => b'x')`)
	if err != nil {
		t.Fatal(err)
	}
	large, _, err := freezeMutate("Tasks", "SEND", `(key => 1, payload => b'`+strings.Repeat("x", 4096)+`')`)
	if err != nil {
		t.Fatal(err)
	}
	if large[0].payloadBytes()-small[0].payloadBytes() < 4095 {
		t.Fatal("queue payload is not charged to the replay journal")
	}
}
