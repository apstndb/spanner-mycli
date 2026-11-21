// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mycli

import (
	"strings"
	"testing"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/google/go-cmp/cmp"
	"github.com/nyaosorg/go-readline-ny/simplehistory"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestSecureContextConnectionWire(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	server, opts := startDirectedReadDial(t)
	vars := newDirectedReadVars(t)
	vars.Transaction.KeepTransactionAlive = false
	session, _ := newDirectedReadProductSession(t, opts, vars)
	handler := NewSessionHandler(session)
	t.Cleanup(handler.Close)

	run := func(sql string) *Result {
		t.Helper()
		stmt, err := BuildStatement(sql)
		if err != nil {
			t.Fatal(err)
		}
		result, err := handler.ExecuteStatement(ctx, stmt)
		if err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return result
	}
	assertSQLRequest := func(sql string, want map[string]*structpb.Value) {
		t.Helper()
		requests := server.takeRequests()
		if len(requests) != 1 || requests[0].GetSql() != sql {
			t.Fatalf("requests = %v, want one %q", requests, sql)
		}
		got := requests[0].GetRequestOptions().GetClientContext().GetSecureContext()
		if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
			t.Fatalf("Secure Context (-want +got):\n%s", diff)
		}
	}
	assertShow := func(want ...[2]string) {
		t.Helper()
		got := secureContextRows(t, run("SHOW SECURE CONTEXT"))
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("SHOW SECURE CONTEXT rows (-want +got):\n%s", diff)
		}
	}

	assertShow()
	run("SELECT 1")
	assertSQLRequest("SELECT 1", nil)
	run("set secure context tenant_id = 'tenant-a'")
	run("SET SECURE CONTEXT scope = 'app'")
	run(`SET SECURE CONTEXT tenant_id = 'tenant\'b'`)
	want := map[string]*structpb.Value{
		"scope":     structpb.NewStringValue("app"),
		"tenant_id": structpb.NewStringValue("tenant'b"),
	}
	assertShow([2]string{"scope", "app"}, [2]string{"tenant_id", "tenant'b"})
	run("SELECT 1")
	assertSQLRequest("SELECT 1", want)
	run("EXPLAIN SELECT 2")
	assertSQLRequest("SELECT 2", want)
	run("UPDATE T SET x = 1 WHERE id = 1")
	assertSQLRequest("UPDATE T SET x = 1 WHERE id = 1", want)
	run("PARTITIONED UPDATE T SET x = 2 WHERE id = 1")
	assertSQLRequest("UPDATE T SET x = 2 WHERE id = 1", want)

	if err := handler.RecreateClient(ctx); err != nil {
		t.Fatal(err)
	}
	run("SELECT 3")
	assertSQLRequest("SELECT 3", want)
	run("unset secure context Scope") // Case-sensitive missing keys are a no-op.
	assertShow([2]string{"scope", "app"}, [2]string{"tenant_id", "tenant'b"})
	run("unset secure context scope")
	assertShow([2]string{"tenant_id", "tenant'b"})
	run("SELECT 4")
	assertSQLRequest("SELECT 4", map[string]*structpb.Value{"tenant_id": structpb.NewStringValue("tenant'b")})
	run("SET SECURE CONTEXT scope = 'app'")
	run("UNSET SECURE CONTEXT")
	assertShow()
	run("SELECT 5")
	assertSQLRequest("SELECT 5", nil)

	run("SET SECURE CONTEXT tenant_id = 'tenant-c'")
	run("USE db ROLE reader")
	if len(session.secureContext) != 0 {
		t.Fatal("retired session retained Secure Context")
	}
	assertSQLRequest("SELECT 1", nil) // Candidate validation must carry no old context.
	assertShow()
	run("SELECT 6")
	assertSQLRequest("SELECT 6", nil)
	run("SET SECURE CONTEXT tenant_id = 'tenant-d'")
	run("DETACH")
	assertShow()
	run("USE db")
	assertSQLRequest("SELECT 1", nil)
	assertShow()
}

func TestSecureContextRejectsUnsupportedOperations(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	server, opts := startDirectedReadDial(t)
	session, _ := newDirectedReadProductSession(t, opts, newDirectedReadVars(t))
	mustExec(t, ctx, session, "SET SECURE CONTEXT tenant_id = 'tenant-a'")

	requireError := func(sql, want string) {
		t.Helper()
		_, err := execSQL(t, ctx, session, sql)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%s: error = %v, want %q", sql, err, want)
		}
	}
	mustExec(t, ctx, session, "BEGIN")
	requireError("SET SECURE CONTEXT tenant_id = 'tenant-b'", "idle connection")
	requireError("UNSET SECURE CONTEXT tenant_id", "idle connection")
	mustExec(t, ctx, session, "ROLLBACK")
	mustExec(t, ctx, session, "START BATCH DML")
	requireError("UNSET SECURE CONTEXT", "idle connection")
	mustExec(t, ctx, session, "ABORT BATCH")

	requireError("PARTITION SELECT 1", "UNSET SECURE CONTEXT")
	requireError("TRY PARTITIONED QUERY SELECT 1", "UNSET SECURE CONTEXT")
	requireError("RUN PARTITIONED QUERY SELECT 1", "UNSET SECURE CONTEXT")
	requireError("RUN PARTITION 'test'", "UNSET SECURE CONTEXT")
	if requests := server.takePartitionQueries(); len(requests) != 0 {
		t.Fatalf("unexpected partition requests: %v", requests)
	}
	got := secureContextRows(t, mustExec(t, ctx, session, "SHOW SECURE CONTEXT"))
	if diff := cmp.Diff([][2]string{{"tenant_id", "tenant-a"}}, got); diff != "" {
		t.Fatalf("Secure Context after rejection (-want +got):\n%s", diff)
	}

	mustExec(t, ctx, session, "UNSET SECURE CONTEXT tenant_id")
	db := session.DatabasePath()
	sid := db + "/sessions/test"
	tid := []byte("tx")
	tx, err := craftTestTxID(tid, sid, partitionTokenNow())
	if err != nil {
		t.Fatal(err)
	}
	part, err := craftTestQueryPartition([]byte("partition"), &sppb.ExecuteSqlRequest{
		Session:     sid,
		Transaction: &sppb.TransactionSelector{Selector: &sppb.TransactionSelector_Id{Id: tid}},
		Sql:         "SELECT 1",
		RequestOptions: &sppb.RequestOptions{ClientContext: &sppb.RequestOptions_ClientContext{
			SecureContext: map[string]*structpb.Value{"tenant_id": structpb.NewStringValue("another-connection")},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	requireError("RUN PARTITION '"+mustEncode(t, db, tx, part)+"'", "does not accept tokens containing Secure Context")
	if requests := server.takeRequests(); len(requests) != 0 {
		t.Fatalf("unexpected SQL requests: %v", requests)
	}
}

func TestPartitionAdmissionWithEmbeddedSecureContext(t *testing.T) {
	t.Parallel()
	server, opts := startDirectedReadDial(t)
	vars := newDirectedReadVars(t)
	vars.Config.EmbeddedClientConfig = &spanner.ClientConfig{
		ClientContext: &sppb.RequestOptions_ClientContext{
			SecureContext: map[string]*structpb.Value{"tenant": structpb.NewStringValue("embedded")},
		},
	}
	session, _ := newDirectedReadProductSession(t, opts, vars)
	for _, sql := range []string{"PARTITION SELECT 1", "RUN PARTITION 'test'"} {
		_, err := execSQL(t, t.Context(), session, sql)
		if err == nil || !strings.Contains(err.Error(), "embedded Secure Context") {
			t.Fatalf("%s: got %v, want embedded Secure Context error", sql, err)
		}
	}
	if requests := server.takePartitionQueries(); len(requests) != 0 {
		t.Fatalf("unexpected partition requests: %v", requests)
	}
}

func TestSecureContextParseAndHistory(t *testing.T) {
	t.Parallel()
	for _, sql := range []string{
		"SET SECURE CONTEXT tenant_id = @tenant",
		"SET SECURE CONTEXT tenant_id = 1",
		"SET SECURE CONTEXT tenant_id = 'tenant-a' || 'tenant-b'",
		"SET SECURE CONTEXT tenant_id =",
		`SET SECURE CONTEXT tenant_id = '\xff'`,
	} {
		if _, err := BuildStatement(sql); err == nil || !strings.Contains(err.Error(), "requires a GoogleSQL string literal") {
			t.Errorf("BuildStatement(%q) = %v", sql, err)
		}
	}
	history := simplehistory.New()
	for _, sql := range []string{"SET SECURE CONTEXT tenant_id = 'tenant-a'", "SHOW SECURE CONTEXT"} {
		stmt, err := BuildStatement(sql)
		if err != nil {
			t.Fatal(err)
		}
		recordInteractiveHistory(history, stmt, sql)
	}
	if history.Len() != 1 || history.At(0) != "SHOW SECURE CONTEXT;" {
		t.Fatal("interactive history did not omit the SET SECURE CONTEXT statement")
	}
}

func secureContextRows(t *testing.T, result *Result) [][2]string {
	t.Helper()
	payload := result.typedPayload()
	if payload == nil {
		t.Fatal("SHOW SECURE CONTEXT did not return typed rows")
	}
	var values [][2]string
	for _, row := range payload.Rows {
		var name, value string
		if err := row.Columns(&name, &value); err != nil {
			t.Fatal(err)
		}
		values = append(values, [2]string{name, value})
	}
	return values
}
