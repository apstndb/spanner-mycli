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
	"net"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type directedReadWireServer struct {
	partitionFanInServer
	mu       sync.Mutex
	requests []*sppb.ExecuteSqlRequest
}

func (s *directedReadWireServer) BeginTransaction(_ context.Context, r *sppb.BeginTransactionRequest) (*sppb.Transaction, error) {
	if r.Options.GetReadOnly() != nil {
		return &sppb.Transaction{Id: []byte("probe-ro"), ReadTimestamp: timestamppb.Now()}, nil
	}
	return &sppb.Transaction{Id: []byte("probe-rw")}, nil
}

func (s *directedReadWireServer) ExecuteStreamingSql(r *sppb.ExecuteSqlRequest, stream sppb.Spanner_ExecuteStreamingSqlServer) error {
	s.mu.Lock()
	s.requests = append(s.requests, proto.Clone(r).(*sppb.ExecuteSqlRequest))
	s.mu.Unlock()
	tx := &sppb.Transaction{Id: []byte("probe-rw")}
	if r.Transaction.GetSingleUse().GetReadOnly() != nil || r.Transaction.GetBegin().GetReadOnly() != nil || string(r.Transaction.GetId()) == "probe-ro" {
		tx = &sppb.Transaction{Id: []byte("probe-ro"), ReadTimestamp: timestamppb.Now()}
	}
	return stream.Send(&sppb.PartialResultSet{
		Metadata: &sppb.ResultSetMetadata{
			RowType: &sppb.StructType{Fields: []*sppb.StructType_Field{
				{Name: "value", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
			}},
			Transaction: tx,
		},
		Values: []*structpb.Value{structpb.NewStringValue("observed")},
	})
}

func startDirectedReadWire(t *testing.T) (*directedReadWireServer, *spanner.Client) {
	t.Helper()
	srv := &directedReadWireServer{partitionFanInServer: partitionFanInServer{nPartitions: 1, rowsPer: 1}}
	listener := bufconn.Listen(1 << 20)
	grpcServer := grpc.NewServer()
	sppb.RegisterSpannerServer(grpcServer, srv)
	go func() {
		if err := grpcServer.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Errorf("serve: %v", err)
		}
	}()
	t.Cleanup(func() {
		grpcServer.Stop()
		_ = listener.Close()
	})
	conn, err := grpc.NewClient("passthrough:///directed-read-wire",
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
	return srv, client
}

func consumeDirectedReadRequest(t *testing.T, srv *directedReadWireServer, it *spanner.RowIterator, want *sppb.DirectedReadOptions) *sppb.ExecuteSqlRequest {
	t.Helper()
	n := 0
	if err := it.Do(func(*spanner.Row) error {
		n++
		return nil
	}); err != nil {
		t.Fatalf("query: %v", err)
	}
	if n != 1 {
		t.Fatalf("rows=%d want 1", n)
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if len(srv.requests) != 1 {
		t.Fatalf("requests=%d want 1", len(srv.requests))
	}
	got := srv.requests[0]
	srv.requests = nil
	if !proto.Equal(got.DirectedReadOptions, want) {
		t.Fatalf("directed=%v want=%v (nil=%t)", got.DirectedReadOptions, want, want == nil)
	}
	return got
}

func TestDirectedReadRuntimeClearWire(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	srv, client := startDirectedReadWire(t)
	b, err := parseDirectedReadOption("us-west1:READ_WRITE")
	if err != nil {
		t.Fatal(err)
	}
	vars := newSystemVariablesWithDefaultsForTest()
	vars.ensureRegistry()
	tm := NewTransactionManager(client, vars, spanner.ClientConfig{DisableNativeMetrics: true})
	query := spanner.Statement{SQL: "SELECT 'observed'"}

	it, txn, err := tm.RunQuery(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	consumeDirectedReadRequest(t, srv, it, nil)
	txn.Close()

	if err := vars.SetFromSimple("DIRECTED_READ", "us-west1:READ_WRITE"); err != nil {
		t.Fatal(err)
	}
	it, txn, err = tm.RunQuery(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	consumeDirectedReadRequest(t, srv, it, b)
	txn.Close()

	if err := vars.SetFromSimple("DIRECTED_READ", ""); err != nil {
		t.Fatal(err)
	}
	it, txn, err = tm.RunQuery(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	consumeDirectedReadRequest(t, srv, it, nil)
	txn.Close()

	if err := vars.SetFromSimple("DIRECTED_READ", "us-west1:READ_WRITE"); err != nil {
		t.Fatal(err)
	}
	srv.mu.Lock()
	srv.requests = nil
	srv.mu.Unlock()
	if _, err := tm.BeginReadOnlyTransaction(ctx, timestampBoundUnspecified, 0, time.Time{}, 0); err != nil {
		t.Fatal(err)
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if len(srv.requests) != 1 {
		t.Fatalf("BEGIN RO SELECT 1 requests=%d want 1", len(srv.requests))
	}
	if !proto.Equal(srv.requests[0].DirectedReadOptions, b) {
		t.Fatalf("BEGIN RO directed=%v want B", srv.requests[0].DirectedReadOptions)
	}
}

func TestDirectedReadROPartitionAndRWOmitWire(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	srv, client := startDirectedReadWire(t)
	b, err := parseDirectedReadOption("us-west1:READ_WRITE")
	if err != nil {
		t.Fatal(err)
	}
	vars := newSystemVariablesWithDefaultsForTest()
	vars.ensureRegistry()
	if err := vars.SetFromSimple("DIRECTED_READ", "us-west1:READ_WRITE"); err != nil {
		t.Fatal(err)
	}
	tm := NewTransactionManager(client, vars, spanner.ClientConfig{DisableNativeMetrics: true})
	query := spanner.Statement{SQL: "SELECT 'observed'"}

	parts, batch, err := tm.RunPartitionQuery(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 1 {
		t.Fatalf("parts=%d", len(parts))
	}
	consumeDirectedReadRequest(t, srv, batch.Execute(ctx, parts[0]), b)
	batch.Close()

	rw, err := spanner.NewReadWriteStmtBasedTransactionWithOptions(ctx, client, spanner.TransactionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rw.Rollback(ctx) })
	tm.tc = &transactionContext{attrs: transactionAttributes{mode: transactionModeReadWrite}, txn: rw}
	it, _, err := tm.RunQuery(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	got := consumeDirectedReadRequest(t, srv, it, nil)
	if got.GetRequestOptions().GetRequestTag() == "spanner_mycli_heartbeat" {
		t.Fatal("user RW query used heartbeat tag")
	}
}
