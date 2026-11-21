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
	"math"
	"strings"
	"sync/atomic"
	"testing"

	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestShowQueryStatsFingerprint(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		suffix string
		want   int64
	}{
		{" -9223372036854775808", math.MinInt64},
		{" +9223372036854775807", math.MaxInt64},
		{" 0", 0},
	} {
		got, err := BuildStatement("show QUERY stats" + tt.suffix)
		if err != nil {
			t.Fatal(err)
		}
		if stmt, ok := got.(*ShowQueryStatsStatement); !ok || stmt.Fprint != tt.want {
			t.Errorf("BuildStatement(%q) = %#v, want fingerprint %d", tt.suffix, got, tt.want)
		}
	}
	for _, suffix := range []string{"", " abc", " 9223372036854775808", " -9223372036854775809", " 1 2"} {
		if stmt, err := BuildStatement("SHOW QUERY STATS" + suffix); err == nil {
			t.Errorf("BuildStatement accepted %q as %#v", suffix, stmt)
		}
	}
}

type queryStatsRPCServer struct {
	queryCacheRPCServer
	query  atomic.Pointer[sppb.ExecuteSqlRequest]
	result *sppb.PartialResultSet
}

func (s *queryStatsRPCServer) ExecuteStreamingSql(r *sppb.ExecuteSqlRequest, stream sppb.Spanner_ExecuteStreamingSqlServer) error {
	s.query.Store(r)
	return stream.Send(s.result)
}

func TestShowQueryStatsRPC(t *testing.T) {
	t.Parallel()

	names := []string{"text_fingerprint", "window_end_utc", "window_minutes", "observed_minutes", "samples", "p50_ms", "p95_ms", "p99_ms"}
	types := []sppb.TypeCode{
		sppb.TypeCode_INT64, sppb.TypeCode_TIMESTAMP, sppb.TypeCode_INT64, sppb.TypeCode_INT64,
		sppb.TypeCode_INT64, sppb.TypeCode_FLOAT64, sppb.TypeCode_FLOAT64, sppb.TypeCode_FLOAT64,
	}
	fields := make([]*sppb.StructType_Field, len(names))
	for i, name := range names {
		fields[i] = &sppb.StructType_Field{Name: name, Type: &sppb.Type{Code: types[i]}}
	}

	for _, tt := range []struct {
		name        string
		dialect     databasepb.DatabaseDialect
		wantSQL     string
		param       string
		observed    string
		samples     string
		percentiles []*structpb.Value
		wantSuffix  string
	}{
		{
			name: "GoogleSQL histograms available", observed: "7", samples: "42",
			dialect: databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL, wantSQL: showQueryStatsGoogleSQL, param: "fprint",
			percentiles: []*structpb.Value{structpb.NewNumberValue(3.5), structpb.NewNumberValue(17), structpb.NewNumberValue(21.125)},
			wantSuffix:  "7 42 3.500000 17 21.125000",
		},
		{
			name: "GoogleSQL no positive histograms", observed: "0", samples: "0",
			dialect: databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL, wantSQL: showQueryStatsGoogleSQL, param: "fprint",
			percentiles: []*structpb.Value{structpb.NewNullValue(), structpb.NewNullValue(), structpb.NewNullValue()},
			wantSuffix:  "0 0 NULL NULL NULL",
		},
		{
			name: "PostgreSQL histograms available", observed: "3", samples: "11",
			dialect: databasepb.DatabaseDialect_POSTGRESQL, wantSQL: showQueryStatsPostgreSQL, param: "p1",
			percentiles: []*structpb.Value{structpb.NewNumberValue(5), structpb.NewNumberValue(9), structpb.NewNumberValue(17)},
			wantSuffix:  "3 11 5 9 17",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			const fingerprint = "-6422424748333414178"
			const windowEnd = "2026-11-18T02:03:04Z"
			server := &queryStatsRPCServer{result: &sppb.PartialResultSet{
				Metadata: &sppb.ResultSetMetadata{
					RowType:     &sppb.StructType{Fields: fields},
					Transaction: &sppb.Transaction{Id: []byte("qcache-ro"), ReadTimestamp: queryCacheFixedReadTS},
				},
				Values: append([]*structpb.Value{
					structpb.NewStringValue(fingerprint),
					structpb.NewStringValue(windowEnd),
					structpb.NewStringValue("60"),
					structpb.NewStringValue(tt.observed),
					structpb.NewStringValue(tt.samples),
				}, tt.percentiles...),
			}}
			session, live := newBufconnQuerySession(t, server)
			live.Transaction.ReadOnly = true
			live.Feature.DatabaseDialect = tt.dialect
			stmt, err := BuildStatement("SHOW QUERY STATS " + fingerprint)
			if err != nil {
				t.Fatal(err)
			}
			result, err := session.ExecuteStatement(t.Context(), stmt)
			if err != nil {
				t.Fatal(err)
			}
			request := server.query.Load()
			if request.GetSql() != tt.wantSQL {
				t.Fatalf("unexpected SQL: %s", request.GetSql())
			}
			if param := request.GetParams().GetFields()[tt.param].GetStringValue(); param != fingerprint {
				t.Fatalf("bound fingerprint = %q, want %q", param, fingerprint)
			}
			if diff := cmp.Diff(names, extractTableColumnNames(result.TableHeader)); diff != "" {
				t.Errorf("headers (-want +got):\n%s", diff)
			}
			rows := result.presentationRows()
			if len(rows) != 1 || result.AffectedRows != 1 {
				t.Fatalf("got %d rows, affected %d, want one row", len(rows), result.AffectedRows)
			}
			if got, want := rowText(rows[0]), fingerprint+" "+windowEnd+" 60 "+tt.wantSuffix; got != want {
				t.Errorf("row = %q, want %q", got, want)
			}
		})
	}
}

func TestShowQueryStatsRejectsReadWriteTransaction(t *testing.T) {
	t.Parallel()
	server := &queryStatsRPCServer{}
	session, _ := newBufconnQuerySession(t, server)
	session.txn.tc = &transactionContext{attrs: transactionAttributes{mode: transactionModeReadWrite}}
	_, err := session.ExecuteStatement(t.Context(), &ShowQueryStatsStatement{})
	if err == nil || !strings.Contains(err.Error(), "read-write transaction") {
		t.Fatalf("error = %v, want read-write rejection", err)
	}
	if server.query.Load() != nil {
		t.Fatal("dispatched query inside read-write transaction")
	}
}
