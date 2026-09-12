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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanner-mycli/internal/mycli/streamio"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const sqlExportSelectUsers = "SELECT * FROM Users"

var queryCacheFixedReadTS = timestamppb.New(time.Date(2026, 9, 12, 8, 0, 0, 0, time.UTC))

func TestQueryCachePublicationSQLExportNames(t *testing.T) {
	t.Parallel()

	planB := testQueryPlan(t)
	statsB := map[string]any{"elapsed_time": "2 msec", "query": "B"}

	for _, tt := range []struct {
		name         string
		sqlTableName string
	}{
		{name: "auto-detected SQL table name"},
		{name: "explicit SQL table name", sqlTableName: "Users"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			session, live := newQueryCacheRPCSession(t, planB, statsB, nil)
			live.Display.CLIFormat = enums.DisplayModeSQLInsert
			live.Display.SQLTableName = tt.sqlTableName
			seed := seedQueryCacheA()
			live.LastResult.QueryCache = seed

			if _, err := executeSQLImplWithQueryRunner(t.Context(), session, sqlExportSelectUsers, live, session.txn.RunQueryWithStats, true); err != nil {
				t.Fatalf("executeSQLImplWithQueryRunner: %v", err)
			}
			if live.Display.SQLTableName != tt.sqlTableName {
				t.Errorf("live SQLTableName = %q, want %q (auto-detect must not persist)", live.Display.SQLTableName, tt.sqlTableName)
			}
			assertLiveQueryCacheReplaced(t, live, seed, planB, statsB)
		})
	}

	t.Run("detected and explicit publish equal plan/stats", func(t *testing.T) {
		t.Parallel()
		var published [2]*LastQueryCache
		for i, sqlTableName := range []string{"", "Users"} {
			session, live := newQueryCacheRPCSession(t, planB, statsB, nil)
			live.Display.CLIFormat = enums.DisplayModeSQLInsert
			live.Display.SQLTableName = sqlTableName
			live.LastResult.QueryCache = seedQueryCacheA()
			if _, err := executeSQLImplWithQueryRunner(t.Context(), session, sqlExportSelectUsers, live, session.txn.RunQueryWithStats, true); err != nil {
				t.Fatalf("sqlTableName=%q: %v", sqlTableName, err)
			}
			published[i] = live.LastResult.QueryCache
		}
		if diff := cmp.Diff(published[0], published[1], protocmp.Transform(), cmpopts.IgnoreFields(LastQueryCache{}, "ReadTimestamp")); diff != "" {
			t.Errorf("detected vs explicit cache mismatch (-detected +explicit):\n%s", diff)
		}
	})
}

func TestQueryCachePublicationBufferedAndStreamingEmitters(t *testing.T) {
	t.Parallel()

	planB := testQueryPlan(t)
	statsB := map[string]any{"elapsed_time": "3 msec", "query": "B"}

	for _, tt := range []struct {
		name         string
		format       enums.DisplayMode
		sqlTableName string
		streaming    bool
		wantStreamed bool
	}{
		{
			name:         "buffered SQL export auto-detect",
			format:       enums.DisplayModeSQLInsert,
			wantStreamed: false,
		},
		{
			name:         "spanvalue writer SQL export auto-detect",
			format:       enums.DisplayModeSQLInsert,
			streaming:    true,
			wantStreamed: true,
		},
		{
			// TAB streams through the RowProcessor stack. SQL auto-detect
			// does not copy settings here; the dest still comes from the runner.
			name:         "spanvalue processor",
			format:       enums.DisplayModeTab,
			streaming:    true,
			wantStreamed: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			session, live := newQueryCacheRPCSession(t, planB, statsB, nil)
			live.Display.CLIFormat = tt.format
			live.Display.SQLTableName = tt.sqlTableName
			if tt.streaming {
				var buf bytes.Buffer
				live.StreamManager = streamio.NewStreamManager(io.NopCloser(strings.NewReader("")), &buf, io.Discard)
			}
			seed := seedQueryCacheA()
			live.LastResult.QueryCache = seed

			result, err := executeSQLImplWithQueryRunner(t.Context(), session, sqlExportSelectUsers, live, session.txn.RunQueryWithStats, true)
			if err != nil {
				t.Fatalf("executeSQLImplWithQueryRunner: %v", err)
			}
			if result.Streamed != tt.wantStreamed {
				t.Fatalf("Streamed = %v, want %v (wrong emitter route)", result.Streamed, tt.wantStreamed)
			}
			assertLiveQueryCacheReplaced(t, live, seed, planB, statsB)
		})
	}
}

func TestQueryCachePublicationNilDestinationLeavesLiveCache(t *testing.T) {
	t.Parallel()

	planB := testQueryPlan(t)
	session, live := newQueryCacheRPCSession(t, planB, map[string]any{"query": "DUMP"}, nil)
	seed := seedQueryCacheA()
	live.LastResult.QueryCache = seed

	var buf bytes.Buffer
	txn := session.client.ReadOnlyTransaction()
	t.Cleanup(txn.Close)
	if _, err := executeSQLWithFormatAndTxn(t.Context(), session, txn, sqlExportSelectUsers,
		enums.DisplayModeSQLInsert, enums.StreamingModeTrue, "Users", nil, &buf); err != nil {
		t.Fatalf("executeSQLWithFormatAndTxn: %v", err)
	}
	if live.LastResult.QueryCache != seed {
		t.Fatal("DUMP replaced the user's last-query cache")
	}
}

func TestQueryCachePublicationParseFailureLeavesOldCache(t *testing.T) {
	t.Parallel()

	session, live := newQueryCacheRPCSession(t, testQueryPlan(t), map[string]any{"query": "B"}, status.Error(codes.Internal, "iterator failed"))
	live.Display.CLIFormat = enums.DisplayModeSQLInsert
	seed := seedQueryCacheA()
	live.LastResult.QueryCache = seed

	_, err := executeSQLImplWithQueryRunner(t.Context(), session, sqlExportSelectUsers, live, session.txn.RunQueryWithStats, true)
	if err == nil {
		t.Fatal("executeSQLImplWithQueryRunner error = nil, want iterator failure")
	}
	if live.LastResult.QueryCache != seed {
		t.Fatal("iterator failure replaced the live cache")
	}
}

func TestQueryCachePublicationAppendixFailureKeepsPublishedCache(t *testing.T) {
	t.Parallel()

	brokenPlan := &sppb.QueryPlan{
		PlanNodes: []*sppb.PlanNode{{Index: 5, DisplayName: "Scan", Kind: sppb.PlanNode_RELATIONAL}},
	}
	statsB := map[string]any{"elapsed_time": "9 msec", "query": "B"}
	session, live := newQueryCacheRPCSession(t, brokenPlan, statsB, nil)
	live.Display.CLIFormat = enums.DisplayModeSQLInsert
	live.Query.QueryMode = sppb.ExecuteSqlRequest_WITH_PLAN_AND_STATS.Enum()
	seed := seedQueryCacheA()
	live.LastResult.QueryCache = seed

	_, err := executeSQLImplWithQueryRunner(t.Context(), session, sqlExportSelectUsers, live, session.txn.RunQueryWithStats, true)
	if err == nil {
		t.Fatal("executeSQLImplWithQueryRunner error = nil, want appendix rendering failure")
	}
	assertLiveQueryCacheReplaced(t, live, seed, brokenPlan, statsB)
}

func TestQueryCachePublicationExplainLastQueryAndPlanNodes(t *testing.T) {
	t.Parallel()

	planB := testQueryPlan(t)
	statsB := map[string]any{"elapsed_time": "2 msec", "query": "B"}
	session, live := newQueryCacheRPCSession(t, planB, statsB, nil)
	live.Display.CLIFormat = enums.DisplayModeSQLInsert
	seed := &LastQueryCache{
		QueryPlan: &sppb.QueryPlan{PlanNodes: []*sppb.PlanNode{
			{Index: 0, Kind: sppb.PlanNode_RELATIONAL, DisplayName: "OldScan"},
		}},
		QueryStats: map[string]any{"query": "A"},
	}
	live.LastResult.QueryCache = seed

	if _, err := executeSQLImplWithQueryRunner(t.Context(), session, sqlExportSelectUsers, live, session.txn.RunQueryWithStats, true); err != nil {
		t.Fatalf("executeSQLImplWithQueryRunner: %v", err)
	}

	explain, err := (&ExplainLastQueryStatement{}).Execute(t.Context(), session)
	if err != nil {
		t.Fatalf("EXPLAIN LAST QUERY: %v", err)
	}
	if explain == nil || len(explain.Rows) == 0 {
		t.Fatal("EXPLAIN LAST QUERY returned no plan rows")
	}
	var sawNewPlan, sawOldPlan bool
	for _, row := range explain.Rows {
		joined := rowText(row)
		if strings.Contains(joined, "Serialize Result") {
			sawNewPlan = true
		}
		if strings.Contains(joined, "OldScan") {
			sawOldPlan = true
		}
	}
	if !sawNewPlan {
		t.Errorf("EXPLAIN LAST QUERY did not see the newly published plan; rows=%v", explain.Rows)
	}
	if sawOldPlan {
		t.Errorf("EXPLAIN LAST QUERY still showed the seed plan; rows=%v", explain.Rows)
	}

	show, err := (&ShowPlanNodeStatement{NodeID: 1}).Execute(t.Context(), session)
	if err != nil {
		t.Fatalf("SHOW PLAN NODE 1: %v", err)
	}
	if show == nil || len(show.Rows) == 0 || !strings.Contains(show.Rows[0][0].RawText(), "Scan") {
		t.Errorf("SHOW PLAN NODE 1 = %v, want cached Scan node from the new plan", show)
	}

	f := &fuzzyFinderCommand{cli: &Cli{SystemVariables: live}}
	got, err := f.resolveCandidates(t.Context(), fuzzyCompletePlanNode, "")
	if err != nil {
		t.Fatalf("plan-node completion: %v", err)
	}
	if len(got) != 2 || got[0].Value != "0" || got[1].Value != "1" {
		t.Fatalf("plan-node completion = %v, want the newly published two-node plan", got)
	}
}

func assertLiveQueryCacheReplaced(t *testing.T, live *systemVariables, seed *LastQueryCache, plan *sppb.QueryPlan, stats map[string]any) {
	t.Helper()
	got := live.LastResult.QueryCache
	if got == nil || got == seed {
		t.Fatal("live QueryCache was not replaced")
	}
	if diff := cmp.Diff(plan, got.QueryPlan, protocmp.Transform()); diff != "" {
		t.Errorf("QueryPlan mismatch (-want +got):\n%s", diff)
	}
	for k, want := range stats {
		if got.QueryStats[k] != want {
			t.Errorf("QueryStats[%q] = %v, want %v (full=%v)", k, got.QueryStats[k], want, got.QueryStats)
		}
	}
}

func seedQueryCacheA() *LastQueryCache {
	return &LastQueryCache{
		QueryPlan: &sppb.QueryPlan{PlanNodes: []*sppb.PlanNode{
			{Index: 0, Kind: sppb.PlanNode_RELATIONAL, DisplayName: "OldScan"},
		}},
		QueryStats:    map[string]any{"query": "A", "elapsed_time": "1 msec"},
		ReadTimestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func rowText(row Row) string {
	parts := make([]string, len(row))
	for i, cell := range row {
		parts[i] = cell.RawText()
	}
	return strings.Join(parts, " ")
}

type queryCacheRPCServer struct {
	sppb.UnimplementedSpannerServer
	plan    *sppb.QueryPlan
	stats   *structpb.Struct
	execErr error
}

func (s *queryCacheRPCServer) CreateSession(_ context.Context, r *sppb.CreateSessionRequest) (*sppb.Session, error) {
	return &sppb.Session{Name: r.Database + "/sessions/qcache", Multiplexed: true, CreateTime: queryCacheFixedReadTS}, nil
}

func (s *queryCacheRPCServer) BatchCreateSessions(_ context.Context, r *sppb.BatchCreateSessionsRequest) (*sppb.BatchCreateSessionsResponse, error) {
	n := int(r.SessionCount)
	if n <= 0 {
		n = 1
	}
	sessions := make([]*sppb.Session, n)
	for i := range n {
		sessions[i] = &sppb.Session{Name: fmt.Sprintf("%s/sessions/%d", r.Database, i), CreateTime: timestamppb.Now()}
	}
	return &sppb.BatchCreateSessionsResponse{Session: sessions}, nil
}

func (s *queryCacheRPCServer) GetSession(_ context.Context, r *sppb.GetSessionRequest) (*sppb.Session, error) {
	return &sppb.Session{Name: r.Name, Multiplexed: true, CreateTime: timestamppb.Now()}, nil
}

func (s *queryCacheRPCServer) DeleteSession(context.Context, *sppb.DeleteSessionRequest) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (s *queryCacheRPCServer) BeginTransaction(context.Context, *sppb.BeginTransactionRequest) (*sppb.Transaction, error) {
	return &sppb.Transaction{Id: []byte("qcache-ro"), ReadTimestamp: queryCacheFixedReadTS}, nil
}

func (s *queryCacheRPCServer) ExecuteSql(context.Context, *sppb.ExecuteSqlRequest) (*sppb.ResultSet, error) {
	return s.resultSet(), nil
}

func (s *queryCacheRPCServer) ExecuteStreamingSql(_ *sppb.ExecuteSqlRequest, stream sppb.Spanner_ExecuteStreamingSqlServer) error {
	if s.execErr != nil {
		return s.execErr
	}
	return stream.Send(&sppb.PartialResultSet{
		Metadata: s.resultSet().Metadata,
		Values:   []*structpb.Value{structpb.NewStringValue("1")},
		Stats:    s.resultSet().Stats,
	})
}

func (s *queryCacheRPCServer) resultSet() *sppb.ResultSet {
	return &sppb.ResultSet{
		Metadata: &sppb.ResultSetMetadata{
			RowType: &sppb.StructType{Fields: []*sppb.StructType_Field{
				{Name: "id", Type: &sppb.Type{Code: sppb.TypeCode_INT64}},
			}},
			Transaction: &sppb.Transaction{Id: []byte("qcache-ro"), ReadTimestamp: queryCacheFixedReadTS},
		},
		Stats: &sppb.ResultSetStats{
			QueryPlan:  s.plan,
			QueryStats: s.stats,
		},
	}
}

func newQueryCacheRPCSession(t *testing.T, plan *sppb.QueryPlan, stats map[string]any, execErr error) (*Session, *systemVariables) {
	t.Helper()
	listener := bufconn.Listen(1 << 20)
	grpcServer := grpc.NewServer()
	sppb.RegisterSpannerServer(grpcServer, &queryCacheRPCServer{
		plan:    plan,
		stats:   mustNewStruct(stats),
		execErr: execErr,
	})
	go func() {
		if err := grpcServer.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Errorf("serve: %v", err)
		}
	}()
	t.Cleanup(func() {
		grpcServer.Stop()
		_ = listener.Close()
	})
	conn, err := grpc.NewClient("passthrough:///qcache",
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

	live := newSystemVariablesWithDefaultsForTest()
	session := &Session{
		mode:            DatabaseConnected,
		client:          client,
		systemVariables: live,
		txn:             NewTransactionManager(client, live, spanner.ClientConfig{DisableNativeMetrics: true}),
	}
	live.inTransaction = session.txn.InTransaction
	return session, live
}
