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
	"strings"
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
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type directedReadWireServer struct {
	partitionFanInServer
	mu       sync.Mutex
	requests []*sppb.ExecuteSqlRequest
	begins   []*sppb.BeginTransactionRequest
	cycleFK  atomic.Bool
}

func (s *directedReadWireServer) BeginTransaction(_ context.Context, r *sppb.BeginTransactionRequest) (*sppb.Transaction, error) {
	s.mu.Lock()
	s.begins = append(s.begins, proto.CloneOf(r))
	s.mu.Unlock()
	if r.Options.GetPartitionedDml() != nil {
		return &sppb.Transaction{Id: []byte("probe-pdml")}, nil
	}
	if r.Options.GetReadOnly() != nil {
		return &sppb.Transaction{Id: []byte("probe-ro"), ReadTimestamp: timestamppb.New(time.Unix(1700000000, 0))}, nil
	}
	return &sppb.Transaction{Id: []byte("probe-rw")}, nil
}

func (s *directedReadWireServer) takeBegins() []*sppb.BeginTransactionRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.begins
	s.begins = nil
	return out
}

func (s *directedReadWireServer) Commit(context.Context, *sppb.CommitRequest) (*sppb.CommitResponse, error) {
	return &sppb.CommitResponse{CommitTimestamp: timestamppb.Now()}, nil
}

func (s *directedReadWireServer) Rollback(context.Context, *sppb.RollbackRequest) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (s *directedReadWireServer) takeRequests() []*sppb.ExecuteSqlRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.requests
	s.requests = nil
	return out
}

func (s *directedReadWireServer) ExecuteSql(ctx context.Context, r *sppb.ExecuteSqlRequest) (*sppb.ResultSet, error) {
	s.mu.Lock()
	s.requests = append(s.requests, proto.Clone(r).(*sppb.ExecuteSqlRequest))
	s.mu.Unlock()
	return &sppb.ResultSet{
		Metadata: &sppb.ResultSetMetadata{
			RowType: &sppb.StructType{Fields: []*sppb.StructType_Field{
				{Name: "value", Type: &sppb.Type{Code: sppb.TypeCode_INT64}},
			}},
		},
		Stats: &sppb.ResultSetStats{RowCount: &sppb.ResultSetStats_RowCountLowerBound{RowCountLowerBound: 1}},
	}, nil
}

func (s *directedReadWireServer) ExecuteStreamingSql(r *sppb.ExecuteSqlRequest, stream sppb.Spanner_ExecuteStreamingSqlServer) error {
	s.mu.Lock()
	s.requests = append(s.requests, proto.Clone(r).(*sppb.ExecuteSqlRequest))
	s.mu.Unlock()
	tx := &sppb.Transaction{Id: []byte("probe-rw")}
	if r.Transaction.GetSingleUse().GetReadOnly() != nil || r.Transaction.GetBegin().GetReadOnly() != nil || string(r.Transaction.GetId()) == "probe-ro" {
		tx = &sppb.Transaction{Id: []byte("probe-ro"), ReadTimestamp: timestamppb.New(time.Unix(1700000000, 0))}
	}
	md, values := directedReadFakeRow(r.Sql, s.cycleFK.Load())
	md.Transaction = tx
	result := &sppb.PartialResultSet{Metadata: md, Values: values}
	if strings.HasPrefix(r.Sql, "UPDATE ") {
		result.Stats = &sppb.ResultSetStats{RowCount: &sppb.ResultSetStats_RowCountExact{RowCountExact: 1}}
		if !strings.Contains(r.Sql, "THEN RETURN") {
			result.Metadata.RowType = &sppb.StructType{}
			result.Values = nil
		}
	}
	if r.QueryMode == sppb.ExecuteSqlRequest_PLAN || r.QueryMode == sppb.ExecuteSqlRequest_PROFILE {
		if result.Stats == nil {
			result.Stats = &sppb.ResultSetStats{}
		}
		result.Stats.QueryPlan = &sppb.QueryPlan{PlanNodes: []*sppb.PlanNode{{DisplayName: "directed-read-fixture"}}}
		result.Stats.QueryStats = &structpb.Struct{Fields: map[string]*structpb.Value{"rows_returned": structpb.NewStringValue("1")}}
		if r.QueryMode == sppb.ExecuteSqlRequest_PLAN {
			result.Values = nil
		}
	}
	return stream.Send(result)
}

func directedReadStringFields(names ...string) []*sppb.StructType_Field {
	fields := make([]*sppb.StructType_Field, len(names))
	for i, name := range names {
		fields[i] = &sppb.StructType_Field{Name: name, Type: &sppb.Type{Code: sppb.TypeCode_STRING}}
	}
	return fields
}

func directedReadFakeRow(sql string, cycleFK bool) (*sppb.ResultSetMetadata, []*structpb.Value) {
	meta := func(names ...string) *sppb.ResultSetMetadata {
		return &sppb.ResultSetMetadata{RowType: &sppb.StructType{Fields: directedReadStringFields(names...)}}
	}
	switch {
	case strings.Contains(sql, "INFORMATION_SCHEMA.TABLES"):
		return meta("TABLE_SCHEMA", "TABLE_NAME", "TABLE_TYPE", "PARENT_TABLE_NAME"), []*structpb.Value{
			structpb.NewStringValue(""), structpb.NewStringValue("T"), structpb.NewStringValue("BASE TABLE"), structpb.NewNullValue(),
		}
	case strings.Contains(sql, "REFERENTIAL_CONSTRAINTS"):
		md := meta("CONSTRAINT_SCHEMA", "CONSTRAINT_NAME", "CHILD_SCHEMA", "CHILD_TABLE",
			"UNIQUE_CONSTRAINT_SCHEMA", "UNIQUE_CONSTRAINT_NAME", "PARENT_SCHEMA", "PARENT_TABLE", "ENFORCED")
		if !cycleFK {
			return md, nil
		}
		return md, []*structpb.Value{
			structpb.NewStringValue(""), structpb.NewStringValue("c"), structpb.NewStringValue(""), structpb.NewStringValue("T"),
			structpb.NewStringValue(""), structpb.NewStringValue("p"), structpb.NewStringValue(""), structpb.NewStringValue("T"),
			structpb.NewStringValue("YES"),
		}
	case strings.Contains(sql, "INFORMATION_SCHEMA.COLUMNS"):
		return meta("COLUMN_NAME"), []*structpb.Value{structpb.NewStringValue("Id")}
	case strings.Contains(sql, "INFORMATION_SCHEMA.SCHEMATA"):
		return meta("SCHEMA_NAME"), []*structpb.Value{structpb.NewStringValue("fixture_schema")}
	case sql == "SELECT `Id` FROM `T`":
		return &sppb.ResultSetMetadata{RowType: &sppb.StructType{Fields: []*sppb.StructType_Field{{Name: "Id", Type: &sppb.Type{Code: sppb.TypeCode_INT64}}}}}, []*structpb.Value{structpb.NewStringValue("42")}
	default:
		return meta("value"), []*structpb.Value{structpb.NewStringValue("observed")}
	}
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
