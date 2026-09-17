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
	"context"
	"strings"
	"sync"
	"testing"

	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

type showChangeStreamsRPCServer struct {
	queryCacheRPCServer

	mu    sync.Mutex
	sql   []string
	err   error
	empty bool
	rows  []showChangeStreamsRPCRow
}

type showChangeStreamsRPCRow struct {
	schema string
	name   string
	all    bool
}

func (s *showChangeStreamsRPCServer) lastSQL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sql) == 0 {
		return ""
	}
	return s.sql[len(s.sql)-1]
}

func (s *showChangeStreamsRPCServer) sqlCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sql)
}

func (s *showChangeStreamsRPCServer) ExecuteSql(_ context.Context, r *sppb.ExecuteSqlRequest) (*sppb.ResultSet, error) {
	md, values, err := s.observe(r)
	if err != nil {
		return nil, err
	}
	return &sppb.ResultSet{Metadata: md, Rows: valuesToRows(values, 3)}, nil
}

func (s *showChangeStreamsRPCServer) ExecuteStreamingSql(r *sppb.ExecuteSqlRequest, stream sppb.Spanner_ExecuteStreamingSqlServer) error {
	md, values, err := s.observe(r)
	if err != nil {
		return err
	}
	return stream.Send(&sppb.PartialResultSet{Metadata: md, Values: values})
}

func (s *showChangeStreamsRPCServer) observe(r *sppb.ExecuteSqlRequest) (*sppb.ResultSetMetadata, []*structpb.Value, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sql = append(s.sql, r.Sql)
	if s.err != nil {
		return nil, nil, s.err
	}
	md := &sppb.ResultSetMetadata{
		RowType: &sppb.StructType{Fields: []*sppb.StructType_Field{
			{Name: "Schema", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
			{Name: "Name", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
			{Name: "All", Type: &sppb.Type{Code: sppb.TypeCode_BOOL}},
		}},
		Transaction: &sppb.Transaction{Id: []byte("qcache-ro"), ReadTimestamp: queryCacheFixedReadTS},
	}
	if s.empty {
		return md, nil, nil
	}
	var values []*structpb.Value
	for _, row := range s.rows {
		values = append(values,
			structpb.NewStringValue(row.schema),
			structpb.NewStringValue(row.name),
			structpb.NewBoolValue(row.all),
		)
	}
	return md, values, nil
}

func valuesToRows(values []*structpb.Value, width int) []*structpb.ListValue {
	if width == 0 || len(values) == 0 {
		return nil
	}
	var rows []*structpb.ListValue
	for i := 0; i < len(values); i += width {
		end := min(i+width, len(values))
		rows = append(rows, &structpb.ListValue{Values: values[i:end]})
	}
	return rows
}

func newShowChangeStreamsRPCSession(t *testing.T, server *showChangeStreamsRPCServer) (*Session, *systemVariables) {
	t.Helper()
	return newBufconnQuerySession(t, server)
}

func TestShowChangeStreamsRPC(t *testing.T) {
	t.Parallel()

	t.Run("GoogleSQL SQL and typed rows", func(t *testing.T) {
		t.Parallel()
		server := &showChangeStreamsRPCServer{rows: []showChangeStreamsRPCRow{
			{schema: "", name: "Everything", all: true},
			{schema: "", name: "Partial", all: false},
		}}
		session, _ := newShowChangeStreamsRPCSession(t, server)
		got, err := (&ShowChangeStreamsStatement{}).Execute(t.Context(), session, OperationOutput{})
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		sql := server.lastSQL()
		if sql != showChangeStreamsGoogleSQL {
			t.Fatalf("SQL = %q, want exact GoogleSQL literal", sql)
		}
		if strings.Contains(strings.ToUpper(sql), "CATALOG") {
			t.Fatalf("SQL leaked a catalog predicate: %s", sql)
		}
		if got.AffectedRows != 2 {
			t.Fatalf("AffectedRows = %d, want 2", got.AffectedRows)
		}
		if names := extractTableColumnNames(got.TableHeader); strings.Join(names, ",") != "Schema,Name,All" {
			t.Fatalf("headers = %v, want Schema,Name,All", names)
		}
		rows := got.presentationRows()
		if rows[0][1].RawText() != "Everything" || rows[0][2].RawText() != "true" {
			t.Fatalf("row0 = %v, want Everything/true", rowText(rows[0]))
		}
		if rows[1][1].RawText() != "Partial" || rows[1][2].RawText() != "false" {
			t.Fatalf("row1 = %v, want Partial/false", rowText(rows[1]))
		}
	})

	t.Run("PostgreSQL SQL", func(t *testing.T) {
		t.Parallel()
		server := &showChangeStreamsRPCServer{rows: []showChangeStreamsRPCRow{
			{schema: "public", name: "Cs", all: true},
		}}
		session, live := newShowChangeStreamsRPCSession(t, server)
		live.Feature.DatabaseDialect = databasepb.DatabaseDialect_POSTGRESQL
		got, err := (&ShowChangeStreamsStatement{}).Execute(t.Context(), session, OperationOutput{})
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		sql := server.lastSQL()
		if sql != showChangeStreamsPostgreSQL {
			t.Fatalf("SQL = %q, want exact PostgreSQL literal", sql)
		}
		if strings.Contains(strings.ToUpper(sql), "CATALOG") {
			t.Fatalf("PostgreSQL SQL leaked a catalog predicate: %s", sql)
		}
		if got.AffectedRows != 1 || got.presentationRows()[0][2].RawText() != "true" {
			t.Fatalf("PG row = %+v", got)
		}
	})

	t.Run("zero rows success", func(t *testing.T) {
		t.Parallel()
		server := &showChangeStreamsRPCServer{empty: true}
		session, _ := newShowChangeStreamsRPCSession(t, server)
		got, err := (&ShowChangeStreamsStatement{}).Execute(t.Context(), session, OperationOutput{})
		if err != nil {
			t.Fatalf("empty Execute: %v", err)
		}
		if got.AffectedRows != 0 || len(got.presentationRows()) != 0 {
			t.Fatalf("empty result = %+v", got)
		}
		if names := extractTableColumnNames(got.TableHeader); strings.Join(names, ",") != "Schema,Name,All" {
			t.Fatalf("empty headers = %v", names)
		}
	})

	t.Run("server error propagates", func(t *testing.T) {
		t.Parallel()
		want := status.Error(codes.NotFound, "change streams view missing")
		server := &showChangeStreamsRPCServer{err: want}
		session, _ := newShowChangeStreamsRPCSession(t, server)
		_, err := (&ShowChangeStreamsStatement{}).Execute(t.Context(), session, OperationOutput{})
		if err == nil || !strings.Contains(err.Error(), "change streams view missing") {
			t.Fatalf("error = %v, want propagated server error", err)
		}
	})

	t.Run("read-write transaction rejects before query", func(t *testing.T) {
		t.Parallel()
		server := &showChangeStreamsRPCServer{rows: []showChangeStreamsRPCRow{{name: "Cs", all: true}}}
		session, _ := newShowChangeStreamsRPCSession(t, server)
		session.txn.tc = &transactionContext{attrs: transactionAttributes{mode: transactionModeReadWrite}}
		_, err := (&ShowChangeStreamsStatement{}).Execute(t.Context(), session, OperationOutput{})
		if err == nil || err.Error() != `"SHOW CHANGE STREAMS" can not be used in a read-write transaction` {
			t.Fatalf("error = %v, want RW rejection", err)
		}
		if server.sqlCount() != 0 {
			t.Fatalf("dispatched %d queries after RW reject: %q", server.sqlCount(), server.lastSQL())
		}
	})

	t.Run("READONLY allows listing", func(t *testing.T) {
		t.Parallel()
		server := &showChangeStreamsRPCServer{rows: []showChangeStreamsRPCRow{{name: "Cs", all: false}}}
		session, live := newShowChangeStreamsRPCSession(t, server)
		live.Transaction.ReadOnly = true
		got, err := session.ExecuteStatement(t.Context(), &ShowChangeStreamsStatement{})
		if err != nil {
			t.Fatalf("READONLY ExecuteStatement: %v", err)
		}
		if got.AffectedRows != 1 {
			t.Fatalf("READONLY result = %+v", got)
		}
	})
}
