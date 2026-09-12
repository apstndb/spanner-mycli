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
	"fmt"
	"net"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const heartbeatSQL = "SELECT 1"

type heartbeatRecord struct {
	txnID    string
	sql      string
	reqTag   string
	priority sppb.RequestOptions_Priority
}

type heartbeatRPCServer struct {
	sppb.UnimplementedSpannerServer

	mu         sync.Mutex
	next       atomic.Uint64
	sqlTxn     map[string]string
	heartbeats []heartbeatRecord

	heartbeatStarted     chan struct{}
	heartbeatStartedOnce sync.Once
	blockHeartbeat       <-chan struct{}
}

func (s *heartbeatRPCServer) newTxnID() []byte {
	n := s.next.Add(1)
	return fmt.Appendf(nil, "hb-txn-%d", n)
}

func (s *heartbeatRPCServer) txnIDFor(r *sppb.ExecuteSqlRequest) []byte {
	if id := r.GetTransaction().GetId(); len(id) > 0 {
		return slices.Clone(id)
	}
	return s.newTxnID()
}

func (s *heartbeatRPCServer) noteSQL(r *sppb.ExecuteSqlRequest, txnID []byte) {
	rec := heartbeatRecord{
		txnID:    string(txnID),
		sql:      r.GetSql(),
		reqTag:   r.GetRequestOptions().GetRequestTag(),
		priority: r.GetRequestOptions().GetPriority(),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sqlTxn == nil {
		s.sqlTxn = make(map[string]string)
	}
	s.sqlTxn[r.GetSql()] = rec.txnID
	if rec.reqTag == "spanner_mycli_heartbeat" {
		s.heartbeats = append(s.heartbeats, rec)
		s.heartbeatStartedOnce.Do(func() {
			if s.heartbeatStarted != nil {
				close(s.heartbeatStarted)
			}
		})
	}
}

func (s *heartbeatRPCServer) txnIDForSQL(sql string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sqlTxn[sql]
}

func (s *heartbeatRPCServer) heartbeatIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, len(s.heartbeats))
	for i, rec := range s.heartbeats {
		ids[i] = rec.txnID
	}
	return ids
}

func (s *heartbeatRPCServer) heartbeatRecords() []heartbeatRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.heartbeats)
}

func (s *heartbeatRPCServer) CreateSession(_ context.Context, r *sppb.CreateSessionRequest) (*sppb.Session, error) {
	return &sppb.Session{Name: r.Database + "/sessions/heartbeat", Multiplexed: true, CreateTime: timestamppb.Now()}, nil
}

func (s *heartbeatRPCServer) BatchCreateSessions(_ context.Context, r *sppb.BatchCreateSessionsRequest) (*sppb.BatchCreateSessionsResponse, error) {
	n := int(r.SessionCount)
	if n <= 0 {
		n = 1
	}
	sessions := make([]*sppb.Session, n)
	for i := range n {
		sessions[i] = &sppb.Session{
			Name:       fmt.Sprintf("%s/sessions/heartbeat-%d", r.Database, i),
			CreateTime: timestamppb.Now(),
		}
	}
	return &sppb.BatchCreateSessionsResponse{Session: sessions}, nil
}

func (s *heartbeatRPCServer) GetSession(_ context.Context, r *sppb.GetSessionRequest) (*sppb.Session, error) {
	return &sppb.Session{Name: r.Name, Multiplexed: true, CreateTime: timestamppb.Now()}, nil
}

func (s *heartbeatRPCServer) DeleteSession(context.Context, *sppb.DeleteSessionRequest) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (s *heartbeatRPCServer) BeginTransaction(context.Context, *sppb.BeginTransactionRequest) (*sppb.Transaction, error) {
	return &sppb.Transaction{Id: s.newTxnID()}, nil
}

func (s *heartbeatRPCServer) Rollback(context.Context, *sppb.RollbackRequest) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (s *heartbeatRPCServer) Commit(context.Context, *sppb.CommitRequest) (*sppb.CommitResponse, error) {
	return &sppb.CommitResponse{CommitTimestamp: timestamppb.Now()}, nil
}

func (s *heartbeatRPCServer) ExecuteSql(ctx context.Context, r *sppb.ExecuteSqlRequest) (*sppb.ResultSet, error) {
	txnID := s.txnIDFor(r)
	s.noteSQL(r, txnID)
	if err := s.waitHeartbeatIfNeeded(ctx, r); err != nil {
		return nil, err
	}
	return s.resultSet(txnID), nil
}

func (s *heartbeatRPCServer) ExecuteStreamingSql(r *sppb.ExecuteSqlRequest, stream sppb.Spanner_ExecuteStreamingSqlServer) error {
	txnID := s.txnIDFor(r)
	s.noteSQL(r, txnID)
	if err := s.waitHeartbeatIfNeeded(stream.Context(), r); err != nil {
		return err
	}
	return stream.Send(&sppb.PartialResultSet{
		Metadata: s.resultSet(txnID).Metadata,
		Values:   []*structpb.Value{structpb.NewStringValue("1")},
	})
}

func (s *heartbeatRPCServer) waitHeartbeatIfNeeded(ctx context.Context, r *sppb.ExecuteSqlRequest) error {
	if r.GetRequestOptions().GetRequestTag() != "spanner_mycli_heartbeat" || s.blockHeartbeat == nil {
		return nil
	}
	select {
	case <-s.blockHeartbeat:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *heartbeatRPCServer) resultSet(txnID []byte) *sppb.ResultSet {
	return &sppb.ResultSet{
		Metadata: &sppb.ResultSetMetadata{
			RowType: &sppb.StructType{Fields: []*sppb.StructType_Field{
				{Name: "", Type: &sppb.Type{Code: sppb.TypeCode_INT64}},
			}},
			Transaction: &sppb.Transaction{Id: txnID},
		},
	}
}

type heartbeatHarness struct {
	tm     *TransactionManager
	server *heartbeatRPCServer
	ticks  chan time.Time

	arrived chan struct{}
	release chan struct{}
	attempt chan struct{}
	unblock chan struct{}
}

func newHeartbeatHarness(t *testing.T) *heartbeatHarness {
	t.Helper()
	server := &heartbeatRPCServer{
		heartbeatStarted: make(chan struct{}),
	}
	listener := bufconn.Listen(1 << 20)
	grpcServer := grpc.NewServer()
	sppb.RegisterSpannerServer(grpcServer, server)
	go func() {
		if err := grpcServer.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Errorf("serve: %v", err)
		}
	}()
	t.Cleanup(func() {
		grpcServer.Stop()
		_ = listener.Close()
	})
	conn, err := grpc.NewClient("passthrough:///heartbeat",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	client, err := spanner.NewClientWithConfig(t.Context(), "projects/test/instances/test/databases/test",
		spanner.ClientConfig{DisableNativeMetrics: true}, option.WithGRPCConn(conn))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)

	sysVars := newSystemVariablesWithDefaultsForTest()
	tm := NewTransactionManager(client, sysVars, spanner.ClientConfig{DisableNativeMetrics: true})
	ticks := make(chan time.Time)
	h := &heartbeatHarness{
		tm:      tm,
		server:  server,
		ticks:   ticks,
		arrived: make(chan struct{}),
		release: make(chan struct{}),
		attempt: make(chan struct{}, 8),
		unblock: make(chan struct{}),
	}
	tm.heartbeatTicks = ticks
	t.Cleanup(func() {
		h.releaseBarriers()
		_ = tm.RollbackReadWriteTransaction(context.Background())
		tm.clearTransactionContext()
	})
	return h
}

func (h *heartbeatHarness) releaseBarriers() {
	select {
	case <-h.release:
	default:
		close(h.release)
	}
	select {
	case <-h.unblock:
	default:
		close(h.unblock)
	}
}

func (h *heartbeatHarness) installBeforeAcquireBarrier() {
	var once sync.Once
	h.tm.heartbeatBeforeAcquire = func() {
		once.Do(func() { close(h.arrived) })
		<-h.release
	}
	h.tm.heartbeatAfterAttempt = func() {
		select {
		case h.attempt <- struct{}{}:
		default:
		}
	}
}

func waitChan(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case <-ch:
	case <-timer.C:
		t.Fatalf("timeout waiting for %s", what)
	case <-t.Context().Done():
		t.Fatalf("test cancelled waiting for %s: %v", what, t.Context().Err())
	}
}

func sendTick(t *testing.T, ticks chan<- time.Time) {
	t.Helper()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case ticks <- time.Time{}:
	case <-timer.C:
		t.Fatal("timeout sending heartbeat tick")
	case <-t.Context().Done():
		t.Fatalf("test cancelled sending heartbeat tick: %v", t.Context().Err())
	}
}

func beginRWAndProbe(t *testing.T, ctx context.Context, h *heartbeatHarness, sql string) string {
	t.Helper()
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatalf("BeginReadWriteTransaction: %v", err)
	}
	iter, _, err := h.tm.RunQuery(ctx, spanner.NewStatement(sql))
	if err != nil {
		t.Fatalf("RunQuery(%s): %v", sql, err)
	}
	if _, _, _, _, err := consumeRowIterDiscard(iter); err != nil {
		t.Fatalf("drain %s: %v", sql, err)
	}
	if !h.tm.heartbeatEnabled() {
		t.Fatal("first user operation did not enable heartbeat")
	}
	id := h.server.txnIDForSQL(sql)
	if id == "" {
		t.Fatalf("no transaction id captured for %q; records=%v", sql, h.server.heartbeatRecords())
	}
	return id
}

func assertHeartbeatMeta(t *testing.T, recs []heartbeatRecord) {
	t.Helper()
	for _, rec := range recs {
		if rec.sql != heartbeatSQL {
			t.Errorf("heartbeat SQL = %q, want %q", rec.sql, heartbeatSQL)
		}
		if rec.reqTag != "spanner_mycli_heartbeat" {
			t.Errorf("heartbeat request tag = %q, want spanner_mycli_heartbeat", rec.reqTag)
		}
		if rec.priority != sppb.RequestOptions_PRIORITY_LOW {
			t.Errorf("heartbeat priority = %v, want PRIORITY_LOW", rec.priority)
		}
	}
}

func TestHeartbeatDelayedTickDoesNotBorrowReplacementOwner(t *testing.T) {
	t.Parallel()
	// #922: a tick that snapshots owner A, then A is rolled back and B is
	// begun, must not issue SELECT 1 on B. This schedule reproduced that
	// borrow before the owner-pointer check (red: heartbeat IDs included B).
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	h.installBeforeAcquireBarrier()

	idA := beginRWAndProbe(t, ctx, h, "SELECT 1 AS owner_a")
	sendTick(t, h.ticks)
	waitChan(t, h.arrived, "owner A eligibility snapshot")

	if err := h.tm.RollbackReadWriteTransaction(ctx); err != nil {
		t.Fatalf("rollback A: %v", err)
	}
	idB := beginRWAndProbe(t, ctx, h, "SELECT 1 AS owner_b")
	if idA == idB {
		t.Fatal("replacement owner reused transaction id A")
	}

	close(h.release)
	waitChan(t, h.attempt, "owner A delayed acquire attempt")

	if got := h.server.heartbeatIDs(); slices.Contains(got, idB) {
		t.Fatalf("delayed A tick issued SELECT 1 on replacement owner B (%s); heartbeats=%v A=%s B=%s", got, got, idA, idB)
	}
	if got := h.server.heartbeatIDs(); slices.Contains(got, idA) {
		t.Fatalf("delayed A tick issued SELECT 1 after A ended; heartbeats=%v A=%s", got, idA)
	}

	sendTick(t, h.ticks)
	waitChan(t, h.attempt, "owner B heartbeat attempt")
	got := h.server.heartbeatIDs()
	if !slices.Contains(got, idB) {
		t.Fatalf("B heartbeat missing on B; heartbeats=%v B=%s", got, idB)
	}
	if slices.Contains(got, idA) {
		t.Fatalf("B heartbeat also hit A; heartbeats=%v A=%s B=%s", got, idA, idB)
	}
	assertHeartbeatMeta(t, h.server.heartbeatRecords())
}

func TestHeartbeatCancelledTickWithNoReplacementIssuesNoRequest(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	h.installBeforeAcquireBarrier()

	idA := beginRWAndProbe(t, ctx, h, "SELECT 1 AS owner_a")
	sendTick(t, h.ticks)
	waitChan(t, h.arrived, "owner A eligibility snapshot")
	if err := h.tm.RollbackReadWriteTransaction(ctx); err != nil {
		t.Fatalf("rollback A: %v", err)
	}
	close(h.release)
	waitChan(t, h.attempt, "cancelled acquire attempt")
	if got := h.server.heartbeatIDs(); len(got) != 0 {
		t.Fatalf("cancelled A tick issued heartbeat %v on owner %s", got, idA)
	}
}

func TestHeartbeatInFlightAcquireStaysOnOriginalOwner(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	h.server.blockHeartbeat = h.unblock
	h.tm.heartbeatAfterAttempt = func() {
		select {
		case h.attempt <- struct{}{}:
		default:
		}
	}

	idA := beginRWAndProbe(t, ctx, h, "SELECT 1 AS owner_a")
	sendTick(t, h.ticks)
	waitChan(t, h.server.heartbeatStarted, "in-flight heartbeat RPC")

	// The server barrier proves the RPC is in flight. Probe its lock directly;
	// an immediate receive from a rollback goroutine only checks scheduling.
	if h.tm.mu.TryLock() {
		h.tm.mu.Unlock()
		t.Fatal("heartbeat released tm.mu while its RPC was in flight")
	}

	rolled := make(chan error, 1)
	go func() {
		rolled <- h.tm.RollbackReadWriteTransaction(ctx)
	}()

	close(h.unblock)
	waitChan(t, h.attempt, "in-flight heartbeat completion")
	select {
	case err := <-rolled:
		if err != nil {
			t.Fatalf("rollback A: %v", err)
		}
	case <-t.Context().Done():
		t.Fatal("timeout waiting for rollback after in-flight heartbeat")
	}

	got := h.server.heartbeatIDs()
	if !slices.Equal(got, []string{idA}) {
		t.Fatalf("in-flight heartbeat IDs = %v, want only A=%s", got, idA)
	}

	idB := beginRWAndProbe(t, ctx, h, "SELECT 1 AS owner_b")
	sendTick(t, h.ticks)
	waitChan(t, h.attempt, "owner B heartbeat attempt")
	got = h.server.heartbeatIDs()
	if !slices.Equal(got, []string{idA, idB}) {
		t.Fatalf("heartbeats=%v, want A then B (%s, %s)", got, idA, idB)
	}
	assertHeartbeatMeta(t, h.server.heartbeatRecords())
}
