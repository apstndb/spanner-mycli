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
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	queryProfileFilterPlanFile = "testdata/plans/filter.input.json"
	queryProfileFingerprint    = int64(-6422424748333414178)
)

func TestParseQueryProfile(t *testing.T) {
	t.Parallel()

	plan := loadTestPlan(t, queryProfileFilterPlanFile)
	planJSON := marshalQueryPlanJSON(t, plan)

	for _, tt := range []struct {
		name    string
		raw     string
		want    *queryProfiles
		wantErr string
	}{
		{
			name: "decodes plan statistics and fingerprint",
			raw: queryProfileRawJSON(t, planJSON, map[string]any{
				"elapsed_time":                 "11.09 msecs",
				"cpu_time":                     "9.2 msecs",
				"rows_returned":                "7",
				"deleted_rows_scanned":         "0",
				"optimizer_version":            "7",
				"optimizer_statistics_package": "auto_20250527_16_21_42UTC",
				"query_text":                   "SELECT * FROM Singers",
				"future_key":                   "kept",
			}, "123"),
			want: &queryProfiles{
				RawQueryPlan: json.RawMessage(planJSON),
				Fprint:       "123",
				QueryStats: QueryStats{
					ElapsedTime:                "11.09 msecs",
					CPUTime:                    "9.2 msecs",
					RowsReturned:               "7",
					DeletedRowsScanned:         "0",
					OptimizerVersion:           "7",
					OptimizerStatisticsPackage: "auto_20250527_16_21_42UTC",
					QueryText:                  "SELECT * FROM Singers",
					Unknown:                    map[string]any{"future_key": "kept"},
				},
			},
		},
		{
			name:    "malformed json",
			raw:     `{"queryPlan":`,
			wantErr: "unexpected end of JSON input",
		},
		{
			name:    "queryStats is not an object",
			raw:     `{"queryPlan":{},"queryStats":[]}`,
			wantErr: "cannot unmarshal",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseQueryProfile(tt.raw)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("parseQueryProfile() error = %v, want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseQueryProfile() error = %v", err)
			}
			if diff := cmp.Diff(tt.want, got, cmpopts.IgnoreFields(queryProfiles{}, "RawQueryPlan")); diff != "" {
				t.Errorf("parseQueryProfile() mismatch (-want +got):\n%s", diff)
			}

			var decoded sppb.QueryPlan
			if err := protojson.Unmarshal(got.RawQueryPlan, &decoded); err != nil {
				t.Fatalf("RawQueryPlan protojson.Unmarshal: %v", err)
			}
			if diff := cmp.Diff(plan, &decoded, protocmp.Transform()); diff != "" {
				t.Errorf("decoded queryPlan mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestToQpr(t *testing.T) {
	t.Parallel()

	intervalEnd := time.Date(2025, 5, 29, 8, 0, 0, 0, time.UTC)
	plan := loadTestPlan(t, queryProfileFilterPlanFile)
	planJSON := marshalQueryPlanJSON(t, plan)
	validProfile := queryProfileRawJSON(t, planJSON, map[string]any{
		"elapsed_time":  "11.09 msecs",
		"cpu_time":      "9.2 msecs",
		"rows_returned": "7",
		"query_text":    "SELECT * FROM Singers",
	}, fmt.Sprint(queryProfileFingerprint))

	for _, tt := range []struct {
		name    string
		row     *spanner.Row
		wantErr string
		check   func(*testing.T, *queryProfilesRow)
	}{
		{
			name: "decodes profile row and query plan",
			row: mustQueryProfileRow(t, intervalEnd, queryProfileFingerprint, 0.01109, spanner.NullJSON{
				Valid: true,
				Value: unmarshalJSONValue(t, validProfile),
			}),
			check: func(t *testing.T, got *queryProfilesRow) {
				t.Helper()
				if !got.IntervalEnd.Equal(intervalEnd) {
					t.Errorf("IntervalEnd = %v, want %v", got.IntervalEnd, intervalEnd)
				}
				if got.TextFingerprint != queryProfileFingerprint {
					t.Errorf("TextFingerprint = %d, want %d", got.TextFingerprint, queryProfileFingerprint)
				}
				if got.LatencySeconds != 0.01109 {
					t.Errorf("LatencySeconds = %v, want 0.01109", got.LatencySeconds)
				}
				if got.QueryProfile == nil {
					t.Fatal("QueryProfile is nil")
				}
				if got.QueryProfile.QueryStats.QueryText != "SELECT * FROM Singers" {
					t.Errorf("QueryText = %q, want SELECT * FROM Singers", got.QueryProfile.QueryStats.QueryText)
				}
				if got.QueryProfile.QueryStats.ElapsedTime != "11.09 msecs" {
					t.Errorf("ElapsedTime = %q, want 11.09 msecs", got.QueryProfile.QueryStats.ElapsedTime)
				}
				if got.QueryProfile.QueryPlan == nil {
					t.Fatal("QueryPlan is nil")
				}
				if diff := cmp.Diff(plan, got.QueryProfile.QueryPlan, protocmp.Transform()); diff != "" {
					t.Errorf("QueryPlan mismatch (-want +got):\n%s", diff)
				}
			},
		},
		{
			name:    "malformed profile json",
			row:     mustQueryProfileRow(t, intervalEnd, 1, 1, spanner.NullJSON{Valid: false}),
			wantErr: "invalid character",
		},
		{
			name: "malformed query plan",
			row: mustQueryProfileRow(t, intervalEnd, 1, 1, spanner.NullJSON{
				Valid: true,
				Value: map[string]any{"queryPlan": 123, "queryStats": map[string]any{}},
			}),
			wantErr: "syntax error",
		},
		{
			name:    "struct decode failure",
			row:     mustSpannerRow(t, []string{"WRONG"}, []any{int64(1)}),
			wantErr: "WRONG",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := toQpr(tt.row)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("toQpr() error = %v, want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("toQpr() error = %v", err)
			}
			tt.check(t, got)
		})
	}
}

func TestFormatStats(t *testing.T) {
	t.Parallel()

	if got := formatStats(nil); got != "" {
		t.Errorf("formatStats(nil) = %q, want empty", got)
	}

	intervalEnd := time.Date(2025, 5, 29, 8, 0, 0, 0, time.UTC)
	got := formatStats(&queryProfilesRow{
		IntervalEnd:     intervalEnd,
		TextFingerprint: queryProfileFingerprint,
		QueryProfile: &queryProfiles{
			QueryStats: QueryStats{
				ElapsedTime:                "11.09 msecs",
				CPUTime:                    "9.2 msecs",
				RowsReturned:               "7",
				DeletedRowsScanned:         "0",
				OptimizerVersion:           "7",
				OptimizerStatisticsPackage: "auto_20250527_16_21_42UTC",
			},
		},
	})
	for _, want := range []string{
		"interval_end:                 2025-05-29 08:00:00 +0000 UTC",
		"text_fingerprint:             -6422424748333414178",
		"elapsed_time:                 11.09 msecs",
		"cpu_time:                     9.2 msecs",
		"rows_returned:                7",
		"deleted_rows_scanned:         0",
		"optimizer_version:            7",
		"optimizer_statistics_package: auto_20250527_16_21_42UTC",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("formatStats() missing %q\nfull:\n%s", want, got)
		}
	}
}

func TestFormatQueryProfileAppendices(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name       string
		appendices []ResultAppendix
		want       string
	}{
		{name: "empty", want: ""},
		{
			name:       "skips empty lines",
			appendices: []ResultAppendix{{Title: "Predicates(identified by ID):"}},
			want:       "",
		},
		{
			name: "renames predicate title and prefixes newline",
			appendices: []ResultAppendix{{
				Title: "Predicates(identified by ID):",
				Lines: []string{"12: Seek Condition: ($SingerId_1 = $SingerId)"},
			}},
			want: "\nPredicates:\n12: Seek Condition: ($SingerId_1 = $SingerId)",
		},
		{
			name: "joins multiple appendices",
			appendices: []ResultAppendix{
				{Title: "Predicates(identified by ID):", Lines: []string{"1: Residual Condition: true"}},
				{Title: "Ordering(identified by ID):", Lines: []string{"2: Key: SingerId"}},
			},
			want: "\nPredicates:\n1: Residual Condition: true\nOrdering(identified by ID):\n2: Key: SingerId",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := formatQueryProfileAppendices(tt.appendices); got != tt.want {
				t.Errorf("formatQueryProfileAppendices() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestShowQueryProfileStatementsRejectReadWriteTransaction(t *testing.T) {
	t.Parallel()

	want := `"SPANNER_SYS.QUERY_PROFILES_TOP_HOUR" can not be used in a read-write transaction`
	for _, stmt := range []Statement{
		&ShowQueryProfilesStatement{},
		&ShowQueryProfileStatement{Fprint: queryProfileFingerprint},
	} {
		t.Run(fmt.Sprintf("%T", stmt), func(t *testing.T) {
			t.Parallel()
			session := newSessionForLocalVarTest(t)
			session.txn.tc = &transactionContext{attrs: transactionAttributes{mode: transactionModeReadWrite}}
			_, err := stmt.Execute(t.Context(), session, OperationOutput{})
			if err == nil || err.Error() != want {
				t.Fatalf("Execute() error = %v, want %q", err, want)
			}
		})
	}
}

func TestShowQueryProfilesStatementExecute(t *testing.T) {
	t.Parallel()

	intervalEnd := time.Date(2025, 5, 29, 8, 0, 0, 0, time.UTC)
	plan := loadTestPlan(t, queryProfileFilterPlanFile)
	planJSON := marshalQueryPlanJSON(t, plan)
	profile := queryProfileRawJSON(t, planJSON, map[string]any{
		"elapsed_time":                 "11.09 msecs",
		"cpu_time":                     "9.2 msecs",
		"rows_returned":                "7",
		"deleted_rows_scanned":         "0",
		"optimizer_version":            "7",
		"optimizer_statistics_package": "auto_20250527_16_21_42UTC",
		"query_text":                   "SELECT * FROM Singers",
	}, fmt.Sprint(queryProfileFingerprint))

	t.Run("empty result", func(t *testing.T) {
		t.Parallel()
		session, server := newQueryProfileRPCSession(t, nil)
		got, err := (&ShowQueryProfilesStatement{}).Execute(t.Context(), session, OperationOutput{})
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if got.AffectedRows != 0 || len(got.presentationRows()) != 0 {
			t.Fatalf("empty result = %+v, want no rows", got)
		}
		if names := extractTableColumnNames(got.TableHeader); !cmp.Equal(names, []string{"Plan"}) {
			t.Errorf("TableHeader = %v, want [Plan]", names)
		}
		assertQueryProfileSQL(t, server.lastRequest(), "SPANNER_SYS.QUERY_PROFILES_TOP_HOUR", nil)
	})

	t.Run("renders decoded plan stats and renamed predicates", func(t *testing.T) {
		t.Parallel()
		session, server := newQueryProfileRPCSession(t, []queryProfileRPCRow{{
			intervalEnd: intervalEnd,
			fingerprint: queryProfileFingerprint,
			latency:     0.01109,
			profile:     profile,
		}})
		got, err := (&ShowQueryProfilesStatement{}).Execute(t.Context(), session, OperationOutput{})
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if got.AffectedRows != 1 || len(got.presentationRows()) != 1 {
			t.Fatalf("AffectedRows/len(Rows) = %d/%d, want 1", got.AffectedRows, len(got.presentationRows()))
		}
		text := rowText(got.presentationRows()[0])
		for _, want := range []string{
			"SELECT * FROM Singers",
			"Serialize Result",
			"Filter",
			"Predicates:",
			"Seek Condition",
			"STARTS_WITH($FirstName, 'A')",
			"interval_end:                 2025-05-29 08:00:00 +0000 UTC",
			"text_fingerprint:             -6422424748333414178",
			"elapsed_time:                 11.09 msecs",
			"cpu_time:                     9.2 msecs",
			"rows_returned:                7",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("missing %q\nfull:\n%s", want, text)
			}
		}
		if strings.Contains(text, "Predicates(identified by ID):") {
			t.Errorf("appendix title was not rewritten:\n%s", text)
		}
		assertQueryProfileSQL(t, server.lastRequest(), "SPANNER_SYS.QUERY_PROFILES_TOP_HOUR", nil)
	})

	t.Run("malformed profile json", func(t *testing.T) {
		t.Parallel()
		session, _ := newQueryProfileRPCSession(t, []queryProfileRPCRow{{
			intervalEnd: intervalEnd,
			fingerprint: 1,
			profile:     "{",
		}})
		_, err := (&ShowQueryProfilesStatement{}).Execute(t.Context(), session, OperationOutput{})
		if err == nil || (!strings.Contains(err.Error(), "cannot decode") && !strings.Contains(err.Error(), "unexpected EOF")) {
			t.Fatalf("Execute() error = %v, want malformed JSON decode failure", err)
		}
	})

	t.Run("profile json is not an object", func(t *testing.T) {
		t.Parallel()
		session, _ := newQueryProfileRPCSession(t, []queryProfileRPCRow{{
			intervalEnd: intervalEnd,
			fingerprint: 1,
			profile:     `[]`,
		}})
		_, err := (&ShowQueryProfilesStatement{}).Execute(t.Context(), session, OperationOutput{})
		if err == nil || !strings.Contains(err.Error(), "cannot unmarshal") {
			t.Fatalf("Execute() error = %v, want non-object profile JSON", err)
		}
	})

	t.Run("query error", func(t *testing.T) {
		t.Parallel()
		session, server := newQueryProfileRPCSession(t, nil)
		server.execErr = status.Error(codes.NotFound, "QUERY_PROFILES_TOP_HOUR missing")
		_, err := (&ShowQueryProfilesStatement{}).Execute(t.Context(), session, OperationOutput{})
		if err == nil || !strings.Contains(err.Error(), "QUERY_PROFILES_TOP_HOUR missing") {
			t.Fatalf("Execute() error = %v, want injected query error", err)
		}
		if got := status.Code(err); got != codes.NotFound {
			t.Errorf("Execute() error code = %v, want NotFound", got)
		}
	})
}

func TestShowQueryProfileStatementExecute(t *testing.T) {
	t.Parallel()

	intervalEnd := time.Date(2025, 5, 22, 5, 0, 0, 0, time.UTC)
	plan := selectProfileResultSet.GetStats().GetQueryPlan()
	planJSON := marshalQueryPlanJSON(t, plan)
	profile := queryProfileRawJSON(t, planJSON, map[string]any{
		"elapsed_time":  "0.23 msecs",
		"cpu_time":      "0.2 msecs",
		"rows_returned": "1",
		"query_text":    "SELECT 1",
	}, fmt.Sprint(queryProfileFingerprint))

	t.Run("empty result", func(t *testing.T) {
		t.Parallel()
		session, server := newQueryProfileRPCSession(t, nil)
		_, err := (&ShowQueryProfileStatement{Fprint: queryProfileFingerprint}).Execute(t.Context(), session, OperationOutput{})
		if err == nil || err.Error() != "empty result" {
			t.Fatalf("Execute() error = %v, want empty result", err)
		}
		assertQueryProfileSQL(t, server.lastRequest(), "WHERE TEXT_FINGERPRINT = @fprint", map[string]string{
			"fprint": fmt.Sprint(queryProfileFingerprint),
		})
	})

	t.Run("renders analyze result from decoded profile", func(t *testing.T) {
		t.Parallel()
		session, server := newQueryProfileRPCSession(t, []queryProfileRPCRow{{
			intervalEnd: intervalEnd,
			fingerprint: queryProfileFingerprint,
			latency:     0.00023,
			profile:     profile,
		}})
		got, err := (&ShowQueryProfileStatement{Fprint: queryProfileFingerprint}).Execute(t.Context(), session, OperationOutput{})
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if got.Stats.ElapsedTime != "0.23 msecs" || got.Stats.QueryText != "SELECT 1" {
			t.Errorf("Stats = %+v, want elapsed_time 0.23 msecs and query_text SELECT 1", got.Stats)
		}
		if got.AffectedRows == 0 {
			t.Fatal("ANALYZE result has no plan rows")
		}
		var sawSerialize bool
		for _, row := range got.presentationRows() {
			if strings.Contains(rowText(row), "Serialize Result") {
				sawSerialize = true
			}
		}
		if !sawSerialize {
			t.Errorf("plan rows missing Serialize Result: %v", got.presentationRows())
		}
		assertQueryProfileSQL(t, server.lastRequest(), "WHERE TEXT_FINGERPRINT = @fprint", map[string]string{
			"fprint": fmt.Sprint(queryProfileFingerprint),
		})
	})

	t.Run("malformed query plan", func(t *testing.T) {
		t.Parallel()
		session, _ := newQueryProfileRPCSession(t, []queryProfileRPCRow{{
			intervalEnd: intervalEnd,
			fingerprint: queryProfileFingerprint,
			profile:     `{"queryPlan":123,"queryStats":{}}`,
		}})
		_, err := (&ShowQueryProfileStatement{Fprint: queryProfileFingerprint}).Execute(t.Context(), session, OperationOutput{})
		if err == nil || !strings.Contains(err.Error(), "syntax error") {
			t.Fatalf("Execute() error = %v, want malformed plan", err)
		}
	})
}

type queryProfileRPCRow struct {
	intervalEnd time.Time
	fingerprint int64
	latency     float64
	profile     string
}

type queryProfileRPCServer struct {
	sppb.UnimplementedSpannerServer
	mu      sync.Mutex
	rows    []queryProfileRPCRow
	execErr error
	lastReq *sppb.ExecuteSqlRequest
}

func (s *queryProfileRPCServer) lastRequest() *sppb.ExecuteSqlRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneExecuteSqlRequest(s.lastReq)
}

func (s *queryProfileRPCServer) record(req *sppb.ExecuteSqlRequest) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastReq = cloneExecuteSqlRequest(req)
}

func (s *queryProfileRPCServer) CreateSession(_ context.Context, r *sppb.CreateSessionRequest) (*sppb.Session, error) {
	return &sppb.Session{Name: r.Database + "/sessions/qprofile", Multiplexed: true, CreateTime: queryCacheFixedReadTS}, nil
}

func (s *queryProfileRPCServer) BatchCreateSessions(_ context.Context, r *sppb.BatchCreateSessionsRequest) (*sppb.BatchCreateSessionsResponse, error) {
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

func (s *queryProfileRPCServer) GetSession(_ context.Context, r *sppb.GetSessionRequest) (*sppb.Session, error) {
	return &sppb.Session{Name: r.Name, Multiplexed: true, CreateTime: timestamppb.Now()}, nil
}

func (s *queryProfileRPCServer) DeleteSession(context.Context, *sppb.DeleteSessionRequest) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (s *queryProfileRPCServer) BeginTransaction(context.Context, *sppb.BeginTransactionRequest) (*sppb.Transaction, error) {
	return &sppb.Transaction{Id: []byte("qprofile-ro"), ReadTimestamp: queryCacheFixedReadTS}, nil
}

func (s *queryProfileRPCServer) Commit(context.Context, *sppb.CommitRequest) (*sppb.CommitResponse, error) {
	return &sppb.CommitResponse{CommitTimestamp: queryCacheFixedReadTS}, nil
}

func (s *queryProfileRPCServer) Rollback(context.Context, *sppb.RollbackRequest) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (s *queryProfileRPCServer) ExecuteSql(_ context.Context, r *sppb.ExecuteSqlRequest) (*sppb.ResultSet, error) {
	s.record(r)
	if s.execErr != nil {
		return nil, s.execErr
	}
	return s.resultSet(), nil
}

func (s *queryProfileRPCServer) ExecuteStreamingSql(r *sppb.ExecuteSqlRequest, stream sppb.Spanner_ExecuteStreamingSqlServer) error {
	s.record(r)
	if s.execErr != nil {
		return s.execErr
	}
	rs := s.resultSet()
	return stream.Send(&sppb.PartialResultSet{
		Metadata: rs.Metadata,
		Values:   flattenResultSetValues(rs),
		Stats:    rs.Stats,
	})
}

func (s *queryProfileRPCServer) resultSet() *sppb.ResultSet {
	fields := []*sppb.StructType_Field{
		{Name: "INTERVAL_END", Type: &sppb.Type{Code: sppb.TypeCode_TIMESTAMP}},
		{Name: "TEXT_FINGERPRINT", Type: &sppb.Type{Code: sppb.TypeCode_INT64}},
		{Name: "LATENCY_SECONDS", Type: &sppb.Type{Code: sppb.TypeCode_FLOAT64}},
		{Name: "QUERY_PROFILE", Type: &sppb.Type{Code: sppb.TypeCode_JSON}},
	}
	rows := make([]*structpb.ListValue, 0, len(s.rows))
	for _, row := range s.rows {
		interval := row.intervalEnd
		if interval.IsZero() {
			interval = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		}
		rows = append(rows, &structpb.ListValue{Values: []*structpb.Value{
			structpb.NewStringValue(interval.UTC().Format(time.RFC3339Nano)),
			structpb.NewStringValue(fmt.Sprint(row.fingerprint)),
			structpb.NewNumberValue(row.latency),
			structpb.NewStringValue(row.profile),
		}})
	}
	return &sppb.ResultSet{
		Metadata: &sppb.ResultSetMetadata{
			RowType:     &sppb.StructType{Fields: fields},
			Transaction: &sppb.Transaction{Id: []byte("qprofile-ro"), ReadTimestamp: queryCacheFixedReadTS},
		},
		Rows: rows,
	}
}

func flattenResultSetValues(rs *sppb.ResultSet) []*structpb.Value {
	var values []*structpb.Value
	for _, row := range rs.Rows {
		values = append(values, row.GetValues()...)
	}
	return values
}

func newQueryProfileRPCSession(t *testing.T, rows []queryProfileRPCRow) (*Session, *queryProfileRPCServer) {
	t.Helper()
	listener := bufconn.Listen(1 << 20)
	grpcServer := grpc.NewServer()
	server := &queryProfileRPCServer{rows: rows}
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
	conn, err := grpc.NewClient("passthrough:///qprofile",
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
	return session, server
}

func assertQueryProfileSQL(t *testing.T, req *sppb.ExecuteSqlRequest, sqlFragment string, wantParams map[string]string) {
	t.Helper()
	if req == nil {
		t.Fatal("missing ExecuteSql request")
	}
	if !strings.Contains(req.GetSql(), sqlFragment) {
		t.Errorf("SQL = %q, want substring %q", req.GetSql(), sqlFragment)
	}
	got := map[string]string{}
	for k, v := range req.GetParams().GetFields() {
		got[k] = v.GetStringValue()
	}
	if wantParams == nil {
		wantParams = map[string]string{}
	}
	if diff := cmp.Diff(wantParams, got); diff != "" {
		t.Errorf("params mismatch (-want +got):\n%s", diff)
	}
}

func cloneExecuteSqlRequest(req *sppb.ExecuteSqlRequest) *sppb.ExecuteSqlRequest {
	if req == nil {
		return nil
	}
	return proto.Clone(req).(*sppb.ExecuteSqlRequest)
}

func marshalQueryPlanJSON(t *testing.T, plan *sppb.QueryPlan) []byte {
	t.Helper()
	b, err := protojson.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func queryProfileRawJSON(t *testing.T, planJSON []byte, stats map[string]any, fprint string) string {
	t.Helper()
	raw, err := json.Marshal(struct {
		QueryPlan  json.RawMessage `json:"queryPlan"`
		QueryStats map[string]any  `json:"queryStats"`
		Fprint     string          `json:"fprint"`
	}{
		QueryPlan:  planJSON,
		QueryStats: stats,
		Fprint:     fprint,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func unmarshalJSONValue(t *testing.T, raw string) any {
	t.Helper()
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func mustQueryProfileRow(t *testing.T, intervalEnd time.Time, fingerprint int64, latency float64, profile spanner.NullJSON) *spanner.Row {
	t.Helper()
	return mustSpannerRow(t,
		[]string{"INTERVAL_END", "TEXT_FINGERPRINT", "LATENCY_SECONDS", "QUERY_PROFILE"},
		[]any{intervalEnd, fingerprint, latency, profile},
	)
}

func mustSpannerRow(t *testing.T, names []string, values []any) *spanner.Row {
	t.Helper()
	row, err := spanner.NewRow(names, values)
	if err != nil {
		t.Fatal(err)
	}
	return row
}
