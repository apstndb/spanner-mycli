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
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/internal/mycli/streamio"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type tagWireStream struct {
	grpc.ClientStream
	observe func(any)
}

func (s *tagWireStream) SendMsg(m any) error {
	s.observe(m)
	return s.ClientStream.SendMsg(m)
}

type tagObservation struct {
	kind, txnTag, reqTag string
}

func newTaggedCLISession(t *testing.T, ctx context.Context, clients interface {
	ClientOptions() []option.ClientOption
}, project, instance, database string, observe func(any), extraDial ...grpc.UnaryClientInterceptor,
) *Session {
	t.Helper()
	unary := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoke grpc.UnaryInvoker, options ...grpc.CallOption) error {
		observe(req)
		if len(extraDial) > 0 {
			return extraDial[0](ctx, method, req, reply, cc, invoke, options...)
		}
		return invoke(ctx, method, req, reply, cc, options...)
	}
	stream := func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, create grpc.Streamer, options ...grpc.CallOption) (grpc.ClientStream, error) {
		s, err := create(ctx, desc, cc, method, options...)
		if err != nil {
			return nil, err
		}
		return &tagWireStream{s, observe}, nil
	}
	flags, err := parseTestFlags(withRequiredFlags())
	if err != nil {
		t.Fatal(err)
	}
	vars, err := initializeSystemVariables(&flags.Spanner)
	if err != nil {
		t.Fatal(err)
	}
	vars.Connection = ConnectionVars{Project: project, Instance: instance, Database: database}
	vars.StreamManager = streamio.NewStreamManager(io.NopCloser(strings.NewReader("")), io.Discard, io.Discard)
	opts := append(clients.ClientOptions(), option.WithGRPCDialOption(grpc.WithChainUnaryInterceptor(unary)), option.WithGRPCDialOption(grpc.WithChainStreamInterceptor(stream)))
	session, err := NewSession(ctx, vars, opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(session.Close)
	return session
}

func TestTransactionTagWireRoutes(t *testing.T) {
	skipIfShortIntegration(t)
	t.Setenv("SPANNER_DISABLE_AUTO_TAGGING", "true")
	clients, _ := initializeWithRandomDB(t, []string{"CREATE TABLE A7TagProbe (Id INT64 NOT NULL) PRIMARY KEY (Id)"}, nil)
	ctx := t.Context()

	observeRW := func(enabled *atomic.Bool, records *[]tagObservation, mu *sync.Mutex) func(any) {
		return func(message any) {
			if !enabled.Load() {
				return
			}
			var kind string
			var options *sppb.RequestOptions
			switch r := message.(type) {
			case *sppb.BeginTransactionRequest:
				if r.Options.GetReadWrite() == nil {
					return
				}
				kind, options = "begin", r.RequestOptions
			case *sppb.ExecuteSqlRequest:
				if !strings.Contains(r.Sql, "A7TagProbe") {
					return
				}
				kind, options = "dml", r.RequestOptions
				if strings.HasPrefix(r.Sql, "SELECT") {
					kind = "query"
				}
			case *sppb.ExecuteBatchDmlRequest:
				kind, options = "batch", r.RequestOptions
			case *sppb.CommitRequest:
				kind, options = "commit", r.RequestOptions
			default:
				return
			}
			mu.Lock()
			*records = append(*records, tagObservation{kind, options.GetTransactionTag(), options.GetRequestTag()})
			mu.Unlock()
		}
	}

	t.Run("sdk_control", func(t *testing.T) {
		var mu sync.Mutex
		var records []tagObservation
		var enabled atomic.Bool
		session := newTaggedCLISession(t, ctx, clients, clients.ProjectID, clients.InstanceID, clients.DatabaseID, observeRW(&enabled, &records, &mu))
		enabled.Store(true)
		_, err := session.client.ReadWriteTransactionWithOptions(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
			if _, err := txn.Update(ctx, spanner.Statement{SQL: "INSERT INTO A7TagProbe (Id) VALUES (1)"}); err != nil {
				return err
			}
			return nil
		}, spanner.TransactionOptions{TransactionTag: "a7-sdk"})
		enabled.Store(false)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range records {
			if r.txnTag != "a7-sdk" {
				t.Errorf("%s tag=%q", r.kind, r.txnTag)
			}
		}
	})

	for i, route := range []string{"cli_pending", "cli_explicit_rw", "cli_implicit", "cli_manual_batch", "cli_automatic_batch", "cli_mutate"} {
		t.Run(route, func(t *testing.T) {
			var mu sync.Mutex
			var records []tagObservation
			var enabled atomic.Bool
			session := newTaggedCLISession(t, ctx, clients, clients.ProjectID, clients.InstanceID, clients.DatabaseID, observeRW(&enabled, &records, &mu))
			wantTag := "a7-" + route
			insert := fmt.Sprintf("INSERT INTO A7TagProbe (Id) VALUES (%d)", i+10)
			query := fmt.Sprintf("SELECT Id FROM A7TagProbe WHERE Id = %d", i+10)
			enabled.Store(true)
			mustExec(t, ctx, session, "SET TRANSACTION_TAG = '"+wantTag+"'")
			if session.systemVariables.ListVariables()["TRANSACTION_TAG"] != wantTag {
				t.Fatal("list disagrees before consume")
			}
			switch route {
			case "cli_pending":
				mustExec(t, ctx, session, "BEGIN")
				mustExec(t, ctx, session, insert)
				mustExec(t, ctx, session, "SET STATEMENT_TAG = 'a7-request'")
				mustExec(t, ctx, session, query)
				mustExec(t, ctx, session, "COMMIT")
			case "cli_explicit_rw":
				mustExec(t, ctx, session, "BEGIN RW")
				if got := mustGetVar(t, session, "TRANSACTION_TAG"); got != wantTag {
					t.Fatalf("SHOW during RW: %q", got)
				}
				mustExec(t, ctx, session, insert)
				mustExec(t, ctx, session, "COMMIT")
			case "cli_implicit":
				mustExec(t, ctx, session, insert)
			case "cli_manual_batch":
				mustExec(t, ctx, session, "BEGIN")
				mustExec(t, ctx, session, "START BATCH DML")
				res := mustExec(t, ctx, session, insert)
				if res.IsExecutedDML || res.BatchInfo == nil || res.BatchInfo.Size != 1 {
					t.Fatalf("not queued: %+v", res)
				}
				mustExec(t, ctx, session, "RUN BATCH")
				mustExec(t, ctx, session, "COMMIT")
			case "cli_automatic_batch":
				mustExec(t, ctx, session, "BEGIN")
				mustExec(t, ctx, session, "SET AUTO_BATCH_DML = TRUE")
				res := mustExec(t, ctx, session, insert)
				if res.IsExecutedDML || res.BatchInfo == nil || res.BatchInfo.Size != 1 {
					t.Fatalf("not queued: %+v", res)
				}
				mustExec(t, ctx, session, "COMMIT")
			case "cli_mutate":
				mustExec(t, ctx, session, fmt.Sprintf("MUTATE A7TagProbe INSERT STRUCT(%d AS Id)", i+10))
			}
			enabled.Store(false)
			if got := mustGetVar(t, session, "TRANSACTION_TAG"); got != "" {
				t.Fatalf("slot after owner: %q", got)
			}
			counts := map[string]int{}
			for _, r := range records {
				counts[r.kind]++
				if r.txnTag != wantTag {
					t.Errorf("%s transaction_tag=%q want=%q", r.kind, r.txnTag, wantTag)
				}
			}
			if counts["commit"] == 0 {
				t.Fatalf("no commit: %v", counts)
			}
			if route == "cli_manual_batch" || route == "cli_automatic_batch" {
				if counts["batch"] != 1 || counts["dml"] != 0 {
					t.Fatalf("batch route counts: %v", counts)
				}
			}
			if route == "cli_mutate" && counts["dml"] != 0 {
				t.Fatalf("MUTATE used SQL: %v", counts)
			}
			mustExec(t, ctx, session, query)
		})
	}
}

func TestTransactionTagLateSetRejected(t *testing.T) {
	skipIfShortIntegration(t)
	t.Setenv("SPANNER_DISABLE_AUTO_TAGGING", "true")
	clients, _ := initializeWithRandomDB(t, []string{"CREATE TABLE A7TagProbe (Id INT64 NOT NULL) PRIMARY KEY (Id)"}, nil)
	ctx := t.Context()
	var mu sync.Mutex
	var records []tagObservation
	var enabled atomic.Bool
	observe := func(message any) {
		if !enabled.Load() {
			return
		}
		if r, ok := message.(*sppb.BeginTransactionRequest); ok && r.Options.GetReadWrite() != nil {
			mu.Lock()
			records = append(records, tagObservation{"begin", r.RequestOptions.GetTransactionTag(), ""})
			mu.Unlock()
		}
		if r, ok := message.(*sppb.CommitRequest); ok {
			mu.Lock()
			records = append(records, tagObservation{"commit", r.RequestOptions.GetTransactionTag(), ""})
			mu.Unlock()
		}
	}
	session := newTaggedCLISession(t, ctx, clients, clients.ProjectID, clients.InstanceID, clients.DatabaseID, observe)
	mustExec(t, ctx, session, "SET TRANSACTION_TAG = 'keep-me'")
	mustExec(t, ctx, session, "BEGIN RW")
	enabled.Store(true)
	_, err := execSQL(t, ctx, session, "SET TRANSACTION_TAG = 'too-late'")
	if err == nil || !errors.Is(err, errTransactionTagInReadWrite) && !strings.Contains(err.Error(), "read-write transaction is active") {
		t.Fatalf("late SET: %v", err)
	}
	_, err = execSQL(t, ctx, session, "SET LOCAL TRANSACTION_TAG = 'too-late-local'")
	if err == nil {
		t.Fatal("late LOCAL succeeded")
	}
	if got := mustGetVar(t, session, "TRANSACTION_TAG"); got != "keep-me" {
		t.Fatalf("applied tag after failed SET: %q", got)
	}
	mustExec(t, ctx, session, "INSERT INTO A7TagProbe (Id) VALUES (50)")
	mustExec(t, ctx, session, "COMMIT")
	for _, r := range records {
		if r.txnTag != "keep-me" {
			t.Errorf("%s tag=%q", r.kind, r.txnTag)
		}
	}
}

func TestTransactionTagFailedBeginPreservesSlot(t *testing.T) {
	skipIfShortIntegration(t)
	clients, _ := initializeWithRandomDB(t, []string{"CREATE TABLE A7TagProbe (Id INT64 NOT NULL) PRIMARY KEY (Id)"}, nil)
	ctx := t.Context()
	const injected = "injected RW begin failure"
	var hookFired atomic.Bool
	failBegin := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoke grpc.UnaryInvoker, options ...grpc.CallOption) error {
		if r, ok := req.(*sppb.BeginTransactionRequest); ok && r.Options.GetReadWrite() != nil {
			hookFired.Store(true)
			return status.Error(codes.PermissionDenied, injected)
		}
		return invoke(ctx, method, req, reply, cc, options...)
	}
	observe := func(any) {}
	session := newTaggedCLISession(t, ctx, clients, clients.ProjectID, clients.InstanceID, clients.DatabaseID, observe, failBegin)
	mustExec(t, ctx, session, "SET TRANSACTION_TAG = 'keep-on-fail'")
	_, err := execSQL(t, ctx, session, "BEGIN RW")
	if err == nil || !strings.Contains(err.Error(), injected) {
		t.Fatalf("BEGIN RW: %v", err)
	}
	if !hookFired.Load() {
		t.Fatal("begin failure hook did not run")
	}
	if got := mustGetVar(t, session, "TRANSACTION_TAG"); got != "keep-on-fail" {
		t.Fatalf("slot after failed begin: %q", got)
	}
}

func TestTransactionTagRODoesNotConsume(t *testing.T) {
	skipIfShortIntegration(t)
	clients, _ := initializeWithRandomDB(t, nil, nil)
	ctx := t.Context()
	session := newTaggedCLISession(t, ctx, clients, clients.ProjectID, clients.InstanceID, clients.DatabaseID, func(any) {})
	mustExec(t, ctx, session, "BEGIN RO")
	mustExec(t, ctx, session, "SET TRANSACTION_TAG = 'after-ro'")
	if got := mustGetVar(t, session, "TRANSACTION_TAG"); got != "after-ro" {
		t.Fatalf("RO SET: %q", got)
	}
	mustExec(t, ctx, session, "COMMIT")
	if got := mustGetVar(t, session, "TRANSACTION_TAG"); got != "after-ro" {
		t.Fatalf("slot after RO: %q", got)
	}
}

func TestTransactionTagLocalThenOrdinary(t *testing.T) {
	skipIfShortIntegration(t)
	t.Setenv("SPANNER_DISABLE_AUTO_TAGGING", "true")
	clients, _ := initializeWithRandomDB(t, []string{"CREATE TABLE A7TagProbe (Id INT64 NOT NULL) PRIMARY KEY (Id)"}, nil)
	ctx := t.Context()
	session := newTaggedCLISession(t, ctx, clients, clients.ProjectID, clients.InstanceID, clients.DatabaseID, func(any) {})
	mustExec(t, ctx, session, "SET TRANSACTION_TAG = 'A'")
	mustExec(t, ctx, session, "BEGIN")
	mustExec(t, ctx, session, "SET LOCAL TRANSACTION_TAG = 'B'")
	mustExec(t, ctx, session, "SET TRANSACTION_TAG = 'C'")
	mustExec(t, ctx, session, "INSERT INTO A7TagProbe (Id) VALUES (70)")
	mustExec(t, ctx, session, "COMMIT")
	if got := mustGetVar(t, session, "TRANSACTION_TAG"); got != "" {
		t.Fatalf("after C consume: %q", got)
	}
}

func TestTransactionTagLocalRestoresBaseline(t *testing.T) {
	skipIfShortIntegration(t)
	t.Setenv("SPANNER_DISABLE_AUTO_TAGGING", "true")
	clients, _ := initializeWithRandomDB(t, []string{"CREATE TABLE A7TagProbe (Id INT64 NOT NULL) PRIMARY KEY (Id)"}, nil)
	ctx := t.Context()
	session := newTaggedCLISession(t, ctx, clients, clients.ProjectID, clients.InstanceID, clients.DatabaseID, func(any) {})
	mustExec(t, ctx, session, "SET TRANSACTION_TAG = 'A'")
	mustExec(t, ctx, session, "BEGIN")
	mustExec(t, ctx, session, "SET LOCAL TRANSACTION_TAG = 'B'")
	mustExec(t, ctx, session, "INSERT INTO A7TagProbe (Id) VALUES (71)")
	mustExec(t, ctx, session, "COMMIT")
	if got := mustGetVar(t, session, "TRANSACTION_TAG"); got != "A" {
		t.Fatalf("restore A: %q", got)
	}
}

func TestTransactionTagPDMLDoesNotConsume(t *testing.T) {
	skipIfShortIntegration(t)
	clients, _ := initializeWithRandomDB(t, []string{"CREATE TABLE A7TagProbe (Id INT64 NOT NULL) PRIMARY KEY (Id)"}, nil)
	ctx := t.Context()
	const injected = "blocked PDML for tag test"
	var hookFired atomic.Bool
	failPDML := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoke grpc.UnaryInvoker, options ...grpc.CallOption) error {
		if r, ok := req.(*sppb.BeginTransactionRequest); ok && r.Options.GetPartitionedDml() != nil {
			hookFired.Store(true)
			if tag := r.RequestOptions.GetTransactionTag(); tag != "" {
				t.Errorf("PDML begin had transaction_tag %q", tag)
			}
			return status.Error(codes.PermissionDenied, injected)
		}
		return invoke(ctx, method, req, reply, cc, options...)
	}
	session := newTaggedCLISession(t, ctx, clients, clients.ProjectID, clients.InstanceID, clients.DatabaseID, func(any) {}, failPDML)
	mustExec(t, ctx, session, "SET TRANSACTION_TAG = 'not-for-pdml'")
	mustExec(t, ctx, session, "SET AUTOCOMMIT_DML_MODE = 'PARTITIONED_NON_ATOMIC'")
	_, err := execSQL(t, ctx, session, "UPDATE A7TagProbe SET Id = Id WHERE TRUE")
	if err == nil || !strings.Contains(err.Error(), injected) {
		t.Fatalf("PDML: %v", err)
	}
	if !hookFired.Load() {
		t.Fatal("PDML hook did not run")
	}
	if got := mustGetVar(t, session, "TRANSACTION_TAG"); got != "not-for-pdml" {
		t.Fatalf("PDML consumed slot: %q", got)
	}
}

func assertWireTagSurfaces(t *testing.T, ctx context.Context, session *Session, want string) {
	t.Helper()
	if got := mustGetVar(t, session, "TRANSACTION_TAG"); got != want {
		t.Fatalf("Get TRANSACTION_TAG=%q want=%q", got, want)
	}
	if listed := session.systemVariables.ListVariables()["TRANSACTION_TAG"]; listed != want {
		t.Fatalf("ListVariables TRANSACTION_TAG=%q want=%q", listed, want)
	}
	res := mustExec(t, ctx, session, "SHOW VARIABLE TRANSACTION_TAG")
	if len(res.Rows) != 1 || len(res.Rows[0]) != 1 || res.Rows[0][0].RawText() != want {
		t.Fatalf("SHOW VARIABLE TRANSACTION_TAG rows=%v want %q", res.Rows, want)
	}
}

func TestTransactionTagPendingSetAfterBegin(t *testing.T) {
	skipIfShortIntegration(t)
	t.Setenv("SPANNER_DISABLE_AUTO_TAGGING", "true")
	clients, _ := initializeWithRandomDB(t, []string{"CREATE TABLE A7TagProbe (Id INT64 NOT NULL) PRIMARY KEY (Id)"}, nil)
	ctx := t.Context()
	var mu sync.Mutex
	var records []tagObservation
	var enabled atomic.Bool
	observe := func(message any) {
		if !enabled.Load() {
			return
		}
		switch r := message.(type) {
		case *sppb.BeginTransactionRequest:
			if r.Options.GetReadWrite() != nil {
				mu.Lock()
				records = append(records, tagObservation{"begin", r.RequestOptions.GetTransactionTag(), ""})
				mu.Unlock()
			}
		case *sppb.CommitRequest:
			mu.Lock()
			records = append(records, tagObservation{"commit", r.RequestOptions.GetTransactionTag(), ""})
			mu.Unlock()
		}
	}
	session := newTaggedCLISession(t, ctx, clients, clients.ProjectID, clients.InstanceID, clients.DatabaseID, observe)
	mustExec(t, ctx, session, "BEGIN")
	mustExec(t, ctx, session, "SET TRANSACTION_TAG = 'after-pending'")
	assertWireTagSurfaces(t, ctx, session, "after-pending")
	enabled.Store(true)
	mustExec(t, ctx, session, "INSERT INTO A7TagProbe (Id) VALUES (80)")
	mustExec(t, ctx, session, "COMMIT")
	enabled.Store(false)
	for _, r := range records {
		if r.txnTag != "after-pending" {
			t.Errorf("%s tag=%q", r.kind, r.txnTag)
		}
	}
	assertWireTagSurfaces(t, ctx, session, "")
}

func TestTransactionTagNoLeakageThenNewTag(t *testing.T) {
	skipIfShortIntegration(t)
	t.Setenv("SPANNER_DISABLE_AUTO_TAGGING", "true")
	clients, _ := initializeWithRandomDB(t, []string{"CREATE TABLE A7TagProbe (Id INT64 NOT NULL) PRIMARY KEY (Id)"}, nil)
	ctx := t.Context()
	var mu sync.Mutex
	var records []tagObservation
	var enabled atomic.Bool
	observe := func(message any) {
		if !enabled.Load() {
			return
		}
		if r, ok := message.(*sppb.CommitRequest); ok {
			mu.Lock()
			records = append(records, tagObservation{"commit", r.RequestOptions.GetTransactionTag(), ""})
			mu.Unlock()
		}
	}
	session := newTaggedCLISession(t, ctx, clients, clients.ProjectID, clients.InstanceID, clients.DatabaseID, observe)
	mustExec(t, ctx, session, "SET TRANSACTION_TAG = 'first-owner'")
	enabled.Store(true)
	mustExec(t, ctx, session, "INSERT INTO A7TagProbe (Id) VALUES (81)")
	enabled.Store(false)
	assertWireTagSurfaces(t, ctx, session, "")
	records = nil
	enabled.Store(true)
	mustExec(t, ctx, session, "INSERT INTO A7TagProbe (Id) VALUES (82)")
	enabled.Store(false)
	if len(records) == 0 {
		t.Fatal("untagged RW produced no commit")
	}
	for _, r := range records {
		if r.txnTag != "" {
			t.Errorf("leaked tag %q on later untagged RW", r.txnTag)
		}
	}
	records = nil
	mustExec(t, ctx, session, "SET TRANSACTION_TAG = 'second-owner'")
	enabled.Store(true)
	mustExec(t, ctx, session, "INSERT INTO A7TagProbe (Id) VALUES (83)")
	enabled.Store(false)
	if len(records) == 0 {
		t.Fatal("second tagged RW produced no commit")
	}
	for _, r := range records {
		if r.txnTag != "second-owner" {
			t.Errorf("second owner tag=%q", r.txnTag)
		}
	}
}

func TestTransactionTagCloseRestoresAfterConsume(t *testing.T) {
	skipIfShortIntegration(t)
	t.Setenv("SPANNER_DISABLE_AUTO_TAGGING", "true")
	_, session := initializeWithRandomDB(t, []string{"CREATE TABLE A7TagProbe (Id INT64 NOT NULL) PRIMARY KEY (Id)"}, nil)
	ctx := t.Context()
	mustExec(t, ctx, session, "SET TRANSACTION_TAG = 'A'")
	mustExec(t, ctx, session, "BEGIN")
	mustExec(t, ctx, session, "SET LOCAL TRANSACTION_TAG = 'B'")
	mustExec(t, ctx, session, "INSERT INTO A7TagProbe (Id) VALUES (84)")
	session.Close()
	if got := mustGetVar(t, session, "TRANSACTION_TAG"); got != "A" {
		t.Fatalf("Close restore A: %q", got)
	}
}

func TestTransactionTagAbortRestoresAfterConsume(t *testing.T) {
	skipIfShortIntegration(t)
	t.Setenv("SPANNER_DISABLE_AUTO_TAGGING", "true")
	_, session := initializeWithRandomDB(t, []string{"CREATE TABLE A7TagProbe (Id INT64 NOT NULL) PRIMARY KEY (Id)"}, nil)
	ctx := t.Context()
	mustExec(t, ctx, session, "SET TRANSACTION_TAG = 'A'")
	mustExec(t, ctx, session, "BEGIN")
	mustExec(t, ctx, session, "SET LOCAL TRANSACTION_TAG = 'B'")
	mustExec(t, ctx, session, "INSERT INTO A7TagProbe (Id) VALUES (85)")
	const injected = "injected collector abort for TRANSACTION_TAG"
	var hookFired bool
	session.txn.queryAfterCollectHook = func() error {
		hookFired = true
		return status.Error(codes.Aborted, injected)
	}
	t.Cleanup(func() { session.txn.queryAfterCollectHook = nil })
	_, err := execSQL(t, ctx, session, "SELECT Id FROM A7TagProbe WHERE Id = 85")
	session.txn.queryAfterCollectHook = nil
	if !hookFired {
		t.Fatal("queryAfterCollectHook did not run")
	}
	if err == nil || !strings.Contains(err.Error(), injected) {
		t.Fatalf("SELECT abort: %v", err)
	}
	if session.txn.InTransaction() {
		t.Fatal("abort left an active transaction")
	}
	if got := mustGetVar(t, session, "TRANSACTION_TAG"); got != "A" {
		t.Fatalf("abort restore A: %q", got)
	}
}

func TestTransactionTagFailedLateSetLeavesUndo(t *testing.T) {
	skipIfShortIntegration(t)
	t.Setenv("SPANNER_DISABLE_AUTO_TAGGING", "true")
	clients, _ := initializeWithRandomDB(t, []string{"CREATE TABLE A7TagProbe (Id INT64 NOT NULL) PRIMARY KEY (Id)"}, nil)
	ctx := t.Context()
	session := newTaggedCLISession(t, ctx, clients, clients.ProjectID, clients.InstanceID, clients.DatabaseID, func(any) {})
	mustExec(t, ctx, session, "SET TRANSACTION_TAG = 'A'")
	mustExec(t, ctx, session, "BEGIN")
	mustExec(t, ctx, session, "SET LOCAL TRANSACTION_TAG = 'B'")
	mustExec(t, ctx, session, "INSERT INTO A7TagProbe (Id) VALUES (86)")
	assertWireTagSurfaces(t, ctx, session, "B")
	_, err := execSQL(t, ctx, session, "SET TRANSACTION_TAG = 'too-late'")
	if err == nil || !errors.Is(err, errTransactionTagInReadWrite) && !strings.Contains(err.Error(), "read-write transaction is active") {
		t.Fatalf("late SET: %v", err)
	}
	assertWireTagSurfaces(t, ctx, session, "B")
	mustExec(t, ctx, session, "COMMIT")
	assertWireTagSurfaces(t, ctx, session, "A")
}

func TestTransactionTagRejectedBeginPreservesOwner(t *testing.T) {
	skipIfShortIntegration(t)
	t.Setenv("SPANNER_DISABLE_AUTO_TAGGING", "true")
	clients, _ := initializeWithRandomDB(t, []string{"CREATE TABLE A7TagProbe (Id INT64 NOT NULL) PRIMARY KEY (Id)"}, nil)
	ctx := t.Context()
	session := newTaggedCLISession(t, ctx, clients, clients.ProjectID, clients.InstanceID, clients.DatabaseID, func(any) {})
	mustExec(t, ctx, session, "SET TRANSACTION_TAG = 'keep-owner'")
	mustExec(t, ctx, session, "BEGIN RW")
	assertWireTagSurfaces(t, ctx, session, "keep-owner")
	_, err := execSQL(t, ctx, session, "BEGIN RW")
	if err == nil {
		t.Fatal("second BEGIN RW succeeded")
	}
	assertWireTagSurfaces(t, ctx, session, "keep-owner")
	mustExec(t, ctx, session, "COMMIT")
	assertWireTagSurfaces(t, ctx, session, "")
}

func TestTransactionTagSessionSwitchAndDetach(t *testing.T) {
	skipIfShortIntegration(t)
	t.Setenv("SPANNER_DISABLE_AUTO_TAGGING", "true")
	_, session := initializeWithRandomDB(t, []string{"CREATE TABLE A7TagProbe (Id INT64 NOT NULL) PRIMARY KEY (Id)"}, nil)
	ctx := t.Context()
	handler := NewSessionHandler(session)
	mustExec(t, ctx, handler.Session, "SET TRANSACTION_TAG = 'across-switch'")
	oldTM := handler.txn

	_, err := handler.ExecuteStatement(ctx, &UseStatement{Database: "missing-a7-tag-db"})
	if err == nil {
		t.Fatal("USE missing database succeeded")
	}
	if handler.txn != oldTM {
		t.Fatal("failed USE replaced TransactionManager")
	}
	assertWireTagSurfaces(t, ctx, handler.Session, "across-switch")
	mustExec(t, ctx, handler.Session, "BEGIN RW")
	assertWireTagSurfaces(t, ctx, handler.Session, "across-switch")
	if _, setErr := execSQL(t, ctx, handler.Session, "SET TRANSACTION_TAG = 'stale'"); setErr == nil {
		t.Fatal("SET succeeded after failed USE; callbacks may be stale")
	}
	mustExec(t, ctx, handler.Session, "COMMIT")
	mustExec(t, ctx, handler.Session, "SET TRANSACTION_TAG = 'across-switch'")

	db := handler.systemVariables.Connection.Database
	if _, err := handler.ExecuteStatement(ctx, &UseStatement{Database: db}); err != nil {
		t.Fatalf("USE same database: %v", err)
	}
	if handler.txn == oldTM {
		t.Fatal("successful USE left the old TransactionManager")
	}
	assertWireTagSurfaces(t, ctx, handler.Session, "across-switch")
	mustExec(t, ctx, handler.Session, "BEGIN RW")
	assertWireTagSurfaces(t, ctx, handler.Session, "across-switch")
	if _, setErr := execSQL(t, ctx, handler.Session, "SET TRANSACTION_TAG = 'stale'"); setErr == nil {
		t.Fatal("SET succeeded after USE; callbacks may be stale")
	}
	mustExec(t, ctx, handler.Session, "COMMIT")
	mustExec(t, ctx, handler.Session, "SET TRANSACTION_TAG = 'across-detach'")

	afterUseTM := handler.txn
	if _, err := handler.ExecuteStatement(ctx, &DetachStatement{}); err != nil {
		t.Fatalf("DETACH: %v", err)
	}
	if handler.txn == afterUseTM {
		t.Fatal("DETACH left the old TransactionManager")
	}
	assertWireTagSurfaces(t, ctx, handler.Session, "across-detach")
	if _, err := handler.ExecuteStatement(ctx, &UseStatement{Database: db}); err != nil {
		t.Fatalf("USE after DETACH: %v", err)
	}
	assertWireTagSurfaces(t, ctx, handler.Session, "across-detach")
	mustExec(t, ctx, handler.Session, "BEGIN RW")
	assertWireTagSurfaces(t, ctx, handler.Session, "across-detach")
	if _, setErr := execSQL(t, ctx, handler.Session, "SET TRANSACTION_TAG = 'stale'"); setErr == nil {
		t.Fatal("SET succeeded after DETACH/USE; callbacks may be stale")
	}
	mustExec(t, ctx, handler.Session, "COMMIT")
}
