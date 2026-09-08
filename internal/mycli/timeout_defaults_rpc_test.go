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
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanemuboost"
	"github.com/apstndb/spanner-mycli/internal/mycli/streamio"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	blockedPDMLBeforeTransport      = "blocked PDML before transport"
	blockedNormalDMLBeforeTransport = "blocked normal DML before transport"
)

type timeoutRPCCounts struct {
	pdmlBegin  atomic.Int32
	executeSQL atomic.Int32
	batchDML   atomic.Int32
	commit     atomic.Int32
	remaining  atomic.Int64
}

func timeoutRPCInterceptor(counts *timeoutRPCCounts) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, request, reply any, cc *grpc.ClientConn, invoke grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		switch req := request.(type) {
		case *sppb.BeginTransactionRequest:
			if req.Options.GetPartitionedDml() != nil {
				counts.pdmlBegin.Add(1)
				if deadline, ok := ctx.Deadline(); ok {
					counts.remaining.Store(int64(time.Until(deadline)))
				}
				return status.Error(codes.PermissionDenied, blockedPDMLBeforeTransport)
			}
		case *sppb.ExecuteSqlRequest:
			counts.executeSQL.Add(1)
			if deadline, ok := ctx.Deadline(); ok {
				counts.remaining.Store(int64(time.Until(deadline)))
			}
			return status.Error(codes.PermissionDenied, blockedNormalDMLBeforeTransport)
		case *sppb.ExecuteBatchDmlRequest:
			counts.batchDML.Add(1)
			return status.Error(codes.PermissionDenied, "blocked unexpected batch DML")
		case *sppb.CommitRequest:
			counts.commit.Add(1)
			return status.Error(codes.PermissionDenied, "blocked unexpected commit")
		}
		return invoke(ctx, method, request, reply, cc, opts...)
	}
}

func timeoutStreamInterceptor(counts *timeoutRPCCounts) grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		if strings.Contains(method, "ExecuteStreamingSql") {
			counts.executeSQL.Add(1)
			if deadline, ok := ctx.Deadline(); ok {
				counts.remaining.Store(int64(time.Until(deadline)))
			}
			return nil, status.Error(codes.PermissionDenied, blockedNormalDMLBeforeTransport)
		}
		return streamer(ctx, desc, cc, method, opts...)
	}
}

func newTimeoutRPCSession(t *testing.T, clients *spanemuboost.Clients, extraArgs []string, counts *timeoutRPCCounts) *Session {
	t.Helper()
	args := append(withRequiredFlags("--enable-partitioned-dml"), extraArgs...)
	flags, err := parseTestFlags(args)
	if err != nil {
		t.Fatal(err)
	}
	vars, err := initializeSystemVariables(&flags.Spanner)
	if err != nil {
		t.Fatal(err)
	}
	vars.Connection = ConnectionVars{Project: clients.ProjectID, Instance: clients.InstanceID, Database: clients.DatabaseID}
	vars.StreamManager = streamio.NewStreamManager(io.NopCloser(strings.NewReader("")), io.Discard, io.Discard)
	opts := append(clients.ClientOptions(),
		option.WithGRPCDialOption(grpc.WithChainUnaryInterceptor(timeoutRPCInterceptor(counts))),
		option.WithGRPCDialOption(grpc.WithChainStreamInterceptor(timeoutStreamInterceptor(counts))),
	)
	session, err := NewSession(t.Context(), vars, opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(session.Close)
	return session
}

func assertApproxDeadline(t *testing.T, got, want time.Duration) {
	t.Helper()
	if got > want || got < want-2*time.Second {
		t.Errorf("deadline remaining=%s want approximately %s", got, want)
	}
}

func TestStatementTimeoutSDKDeadline(t *testing.T) {
	skipIfShortIntegration(t)
	clients, _ := initializeWithRandomDB(t, nil, nil)

	for _, tc := range []struct {
		name   string
		args   []string
		parent time.Duration
		want   time.Duration
	}{
		{name: "omitted", want: 24 * time.Hour},
		{name: "null", args: []string{"--set", "STATEMENT_TIMEOUT=NULL"}, want: 24 * time.Hour},
		{name: "explicit_30s", args: []string{"--timeout", "30s"}, want: 30 * time.Second},
		{name: "explicit_10m", args: []string{"--timeout", "10m"}, want: 10 * time.Minute},
		{name: "short_parent", args: []string{"--set", "STATEMENT_TIMEOUT=NULL"}, parent: 5 * time.Second, want: 5 * time.Second},
	} {
		for _, sql := range []string{
			"PARTITIONED UPDATE T SET V = 1 WHERE TRUE",
			"UPDATE T SET V = 1 WHERE TRUE",
			"DELETE FROM T WHERE TRUE",
		} {
			t.Run(tc.name+"/"+sql, func(t *testing.T) {
				var counts timeoutRPCCounts
				session := newTimeoutRPCSession(t, clients, tc.args, &counts)
				stmt, err := BuildStatement(sql)
				if err != nil {
					t.Fatal(err)
				}
				ctx := t.Context()
				if tc.parent != 0 {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, tc.parent)
					defer cancel()
				}
				_, err = session.ExecuteStatement(ctx, stmt)
				if err == nil || !strings.Contains(err.Error(), blockedPDMLBeforeTransport) || counts.pdmlBegin.Load() != 1 || counts.executeSQL.Load() != 0 || counts.batchDML.Load() != 0 || counts.commit.Load() != 0 {
					t.Fatalf("wrong path: err=%v PDML=%d SQL=%d batch=%d commit=%d", err, counts.pdmlBegin.Load(), counts.executeSQL.Load(), counts.batchDML.Load(), counts.commit.Load())
				}
				assertApproxDeadline(t, time.Duration(counts.remaining.Load()), tc.want)
			})
		}
	}
}

func TestInsertTakesOrdinaryDMLPathWith10mDeadline(t *testing.T) {
	skipIfShortIntegration(t)
	clients, _ := initializeWithRandomDB(t, []string{
		"CREATE TABLE T (Id INT64 NOT NULL, V INT64) PRIMARY KEY (Id)",
	}, nil)
	var counts timeoutRPCCounts
	session := newTimeoutRPCSession(t, clients, nil, &counts)
	_, err := execSQL(t, t.Context(), session, "INSERT INTO T (Id, V) VALUES (1, 1)")
	if err == nil || !strings.Contains(err.Error(), blockedNormalDMLBeforeTransport) {
		t.Fatalf("INSERT did not reach ordinary ExecuteSql: %v", err)
	}
	if counts.pdmlBegin.Load() != 0 || counts.executeSQL.Load() != 1 {
		t.Fatalf("INSERT route PDML=%d SQL=%d", counts.pdmlBegin.Load(), counts.executeSQL.Load())
	}
	assertApproxDeadline(t, time.Duration(counts.remaining.Load()), 10*time.Minute)
}

func TestTimeoutExecutionStateControls(t *testing.T) {
	skipIfShortIntegration(t)
	clients, _ := initializeWithRandomDB(t, []string{
		"CREATE TABLE T (Id INT64 NOT NULL, V INT64) PRIMARY KEY (Id)",
	}, nil)
	ctx := t.Context()

	t.Run("active RW UPDATE uses ordinary 10m path", func(t *testing.T) {
		var counts timeoutRPCCounts
		session := newTimeoutRPCSession(t, clients, nil, &counts)
		mustExec(t, ctx, session, "BEGIN RW")
		assertStmtTimeout(t, session, "UPDATE T SET V = 1 WHERE TRUE", 10*time.Minute)
		_, err := execSQL(t, ctx, session, "UPDATE T SET V = 1 WHERE TRUE")
		if err == nil || !strings.Contains(err.Error(), blockedNormalDMLBeforeTransport) {
			t.Fatalf("active RW UPDATE did not reach ExecuteSql: %v", err)
		}
		if counts.pdmlBegin.Load() != 0 || counts.executeSQL.Load() != 1 {
			t.Fatalf("active RW route PDML=%d SQL=%d", counts.pdmlBegin.Load(), counts.executeSQL.Load())
		}
		assertApproxDeadline(t, time.Duration(counts.remaining.Load()), 10*time.Minute)
	})

	t.Run("automatic queue stays on RW owner", func(t *testing.T) {
		var counts timeoutRPCCounts
		session := newTimeoutRPCSession(t, clients, nil, &counts)
		mustExec(t, ctx, session, "SET AUTO_BATCH_DML = TRUE")
		mustExec(t, ctx, session, "BEGIN RW")
		assertStmtTimeout(t, session, "UPDATE T SET V = 1 WHERE TRUE", 10*time.Minute)
		res := mustExec(t, ctx, session, "UPDATE T SET V = 1 WHERE TRUE")
		if res.IsExecutedDML || !session.txn.HasAutomaticDML() {
			t.Fatalf("expected queued automatic DML: executed=%v queued=%v", res.IsExecutedDML, session.txn.HasAutomaticDML())
		}
		if counts.pdmlBegin.Load() != 0 || counts.executeSQL.Load() != 0 || counts.batchDML.Load() != 0 {
			t.Fatalf("queued UPDATE issued RPCs PDML=%d SQL=%d batch=%d", counts.pdmlBegin.Load(), counts.executeSQL.Load(), counts.batchDML.Load())
		}
	})

	t.Run("manual BatchDML buffers and BulkDDL rejects", func(t *testing.T) {
		var counts timeoutRPCCounts
		session := newTimeoutRPCSession(t, clients, nil, &counts)
		mustExec(t, ctx, session, "START BATCH DML")
		res := mustExec(t, ctx, session, "UPDATE T SET V = 1 WHERE TRUE")
		if res.IsExecutedDML {
			t.Fatal("manual BatchDML executed UPDATE")
		}
		if info := session.batch.Info(); info == nil || info.Size != 1 {
			t.Fatalf("manual BatchDML size: %+v", info)
		}
		session.batch.Abort()

		mustExec(t, ctx, session, "START BATCH DDL")
		_, err := execSQL(t, ctx, session, "UPDATE T SET V = 1 WHERE TRUE")
		if err == nil || !strings.Contains(err.Error(), "there is active batch DDL") {
			t.Fatalf("BulkDDL UPDATE: %v", err)
		}
		if counts.pdmlBegin.Load() != 0 || counts.executeSQL.Load() != 0 {
			t.Fatalf("batch controls issued RPCs PDML=%d SQL=%d", counts.pdmlBegin.Load(), counts.executeSQL.Load())
		}
	})

	t.Run("zero timeout expires at the statement boundary", func(t *testing.T) {
		var counts timeoutRPCCounts
		session := newTimeoutRPCSession(t, clients, []string{"--timeout", "0s"}, &counts)
		start := time.Now()
		_, err := execSQL(t, ctx, session, "UPDATE T SET V = 1 WHERE TRUE")
		if time.Since(start) > time.Second {
			t.Fatalf("zero timeout waited %s", time.Since(start))
		}
		if err == nil || !errors.Is(err, context.DeadlineExceeded) && status.Code(err) != codes.DeadlineExceeded && !strings.Contains(err.Error(), "deadline") {
			t.Fatalf("zero timeout error: %v", err)
		}
		if counts.pdmlBegin.Load() != 0 || counts.executeSQL.Load() != 0 || counts.commit.Load() != 0 {
			t.Fatalf("zero timeout issued writes PDML=%d SQL=%d commit=%d", counts.pdmlBegin.Load(), counts.executeSQL.Load(), counts.commit.Load())
		}
	})

	t.Run("cancelled parent expires at the statement boundary", func(t *testing.T) {
		var counts timeoutRPCCounts
		session := newTimeoutRPCSession(t, clients, nil, &counts)
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		start := time.Now()
		_, err := execSQL(t, canceled, session, "UPDATE T SET V = 1 WHERE TRUE")
		if time.Since(start) > time.Second {
			t.Fatalf("cancelled parent waited %s", time.Since(start))
		}
		if err == nil || !errors.Is(err, context.Canceled) && status.Code(err) != codes.Canceled && !strings.Contains(err.Error(), "cancel") {
			t.Fatalf("cancelled parent error: %v", err)
		}
		if counts.pdmlBegin.Load() != 0 || counts.executeSQL.Load() != 0 || counts.commit.Load() != 0 {
			t.Fatalf("cancelled parent issued writes PDML=%d SQL=%d commit=%d", counts.pdmlBegin.Load(), counts.executeSQL.Load(), counts.commit.Load())
		}
	})
}
