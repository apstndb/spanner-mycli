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
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	adminapi "cloud.google.com/go/spanner/admin/database/apiv1"
	adminpb "cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanner-mycli/internal/mycli/streamio"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
)

func startDirectedReadDial(t *testing.T) (*directedReadWireServer, *grpc.ClientConn) {
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
	conn, err := grpc.NewClient("passthrough:///directed-read-matrix",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return srv, conn
}

func mustParseDirectedRead(t *testing.T, s string) *sppb.DirectedReadOptions {
	t.Helper()
	got, err := parseDirectedReadOption(s)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func newDirectedReadVars(t *testing.T) *systemVariables {
	t.Helper()
	vars := newSystemVariablesWithDefaultsForTest()
	vars.ensureRegistry()
	vars.Connection = ConnectionVars{Project: "p", Instance: "i", Database: "db"}
	vars.StreamManager = streamio.NewStreamManager(io.NopCloser(strings.NewReader("")), io.Discard, io.Discard)
	return vars
}

func newDirectedReadProductSession(t *testing.T, conn *grpc.ClientConn, vars *systemVariables) (*Session, spanner.ClientConfig) {
	t.Helper()
	var gotCfg spanner.ClientConfig
	session, err := newSessionWithFactories(t.Context(), vars,
		func(ctx context.Context, db string, cfg spanner.ClientConfig, _ ...option.ClientOption) (*spanner.Client, error) {
			gotCfg = cfg
			return spanner.NewClientWithConfig(ctx, db, cfg, option.WithGRPCConn(conn))
		},
		adminapi.NewDatabaseAdminClient,
		func(c *spanner.Client) { c.Close() },
		option.WithGRPCConn(conn),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(session.Close)
	return session, gotCfg
}

func requireDirected(t *testing.T, got, want *sppb.DirectedReadOptions) {
	t.Helper()
	if !proto.Equal(got, want) {
		t.Fatalf("directed=%v want=%v", got, want)
	}
}

func TestDirectedReadStartupEmbeddedAndProductWire(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	srv, conn := startDirectedReadDial(t)
	a := mustParseDirectedRead(t, "us-east1:READ_ONLY")
	b := mustParseDirectedRead(t, "us-west1:READ_WRITE")
	embed := mustParseDirectedRead(t, "europe-west1:READ_ONLY")
	embedCopy := proto.CloneOf(embed)

	vars, err := initializeSystemVariables(&spannerOptions{
		ProjectId:    "p",
		InstanceId:   "i",
		DatabaseId:   "db",
		DirectedRead: "us-east1:READ_ONLY",
	})
	if err != nil {
		t.Fatal(err)
	}
	vars.StreamManager = streamio.NewStreamManager(io.NopCloser(strings.NewReader("")), io.Discard, io.Discard)
	vars.Config.EmbeddedClientConfig = &spanner.ClientConfig{
		DisableNativeMetrics: true,
		Type:                 spanner.OMNI,
		DisableRouteToLeader: true,
		UserAgent:            "embedded-directed-test",
		DirectedReadOptions:  embed,
	}
	session, cfg := newDirectedReadProductSession(t, conn, vars)
	if cfg.DirectedReadOptions != nil {
		t.Fatalf("copied client DRO=%v want nil", cfg.DirectedReadOptions)
	}
	if cfg.UserAgent != "embedded-directed-test" || cfg.Type != spanner.OMNI || !cfg.DisableRouteToLeader {
		t.Fatalf("unrelated embedded fields changed: %+v", cfg)
	}
	if vars.Config.EmbeddedClientConfig.DirectedReadOptions != embed || !proto.Equal(embed, embedCopy) {
		t.Fatal("embedded original DRO mutated")
	}
	requireDirected(t, vars.Query.DirectedRead, a)

	it, txn, err := session.txn.RunQuery(ctx, spanner.Statement{SQL: "SELECT 'observed'"})
	if err != nil {
		t.Fatal(err)
	}
	consumeDirectedReadRequest(t, srv, it, a)
	txn.Close()

	if err := vars.SetFromSimple("DIRECTED_READ", "us-west1:READ_WRITE"); err != nil {
		t.Fatal(err)
	}
	it, txn, err = session.txn.RunQuery(ctx, spanner.Statement{SQL: "SELECT 'observed'"})
	if err != nil {
		t.Fatal(err)
	}
	consumeDirectedReadRequest(t, srv, it, b)
	txn.Close()

	if err := vars.SetFromSimple("DIRECTED_READ", ""); err != nil {
		t.Fatal(err)
	}
	it, txn, err = session.txn.RunQuery(ctx, spanner.Statement{SQL: "SELECT 'observed'"})
	if err != nil {
		t.Fatal(err)
	}
	consumeDirectedReadRequest(t, srv, it, nil)
	txn.Close()
}

func TestDirectedReadRecreateClientKeepsLiveOption(t *testing.T) {
	skipIfShortIntegration(t)
	ctx := t.Context()
	clients, _ := initializeWithRandomDB(t, []string{"CREATE TABLE T (Id INT64 NOT NULL) PRIMARY KEY (Id)"}, nil)
	var mu sync.Mutex
	var reqs []*sppb.ExecuteSqlRequest
	observe := func(m any) {
		if r, ok := m.(*sppb.ExecuteSqlRequest); ok {
			mu.Lock()
			reqs = append(reqs, proto.Clone(r).(*sppb.ExecuteSqlRequest))
			mu.Unlock()
		}
	}
	session := newTaggedCLISession(t, ctx, clients, clients.ProjectID, clients.InstanceID, clients.DatabaseID, observe)
	if err := session.systemVariables.SetFromSimple("DIRECTED_READ", "us-west1:READ_WRITE"); err != nil {
		t.Fatal(err)
	}
	b := mustParseDirectedRead(t, "us-west1:READ_WRITE")
	old := session.client
	if err := session.RecreateClient(ctx); err != nil {
		t.Fatal(err)
	}
	if session.clientConfig.DirectedReadOptions != nil {
		t.Fatal("recreated clientConfig DRO not nil")
	}
	if session.client == old {
		t.Fatal("client not replaced")
	}
	mu.Lock()
	reqs = nil
	mu.Unlock()
	res := mustExec(t, ctx, session, "SELECT 1")
	if res.AffectedRows != 1 {
		t.Fatalf("rows=%d", res.AffectedRows)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(reqs) == 0 {
		t.Fatal("no ExecuteSql after RecreateClient")
	}
	requireDirected(t, reqs[len(reqs)-1].DirectedReadOptions, b)
}

func TestDirectedReadSessionHandlerUseDetach(t *testing.T) {
	skipIfShortIntegration(t)
	ctx := t.Context()
	clients, session := initializeWithRandomDB(t, []string{"CREATE TABLE T (Id INT64 NOT NULL) PRIMARY KEY (Id)"}, nil)
	var mu sync.Mutex
	var reqs []*sppb.ExecuteSqlRequest
	observe := func(m any) {
		if r, ok := m.(*sppb.ExecuteSqlRequest); ok {
			mu.Lock()
			reqs = append(reqs, proto.Clone(r).(*sppb.ExecuteSqlRequest))
			mu.Unlock()
		}
	}
	wired := newTaggedCLISession(t, ctx, clients, clients.ProjectID, clients.InstanceID, clients.DatabaseID, observe)
	_ = session
	handler := NewSessionHandler(wired)
	if err := wired.systemVariables.SetFromSimple("DIRECTED_READ", "us-west1:READ_WRITE"); err != nil {
		t.Fatal(err)
	}
	b := mustParseDirectedRead(t, "us-west1:READ_WRITE")
	db := wired.systemVariables.Connection.Database
	if _, err := handler.ExecuteStatement(ctx, &UseStatement{Database: db}); err != nil {
		t.Fatalf("USE: %v", err)
	}
	if handler.systemVariables != wired.systemVariables {
		t.Fatal("USE forked systemVariables")
	}
	mu.Lock()
	if len(reqs) == 0 {
		mu.Unlock()
		t.Fatal("USE sent no ExecuteSql")
	}
	requireDirected(t, reqs[len(reqs)-1].DirectedReadOptions, b)
	reqs = nil
	mu.Unlock()

	if _, err := handler.ExecuteStatement(ctx, &DetachStatement{}); err != nil {
		t.Fatalf("DETACH: %v", err)
	}
	if err := handler.systemVariables.SetFromSimple("DIRECTED_READ", "asia-northeast1:READ_ONLY"); err != nil {
		t.Fatal(err)
	}
	c := mustParseDirectedRead(t, "asia-northeast1:READ_ONLY")
	if _, err := handler.ExecuteStatement(ctx, &UseStatement{Database: db}); err != nil {
		t.Fatalf("attach: %v", err)
	}
	got, err := handler.systemVariables.Get("DIRECTED_READ")
	if err != nil {
		t.Fatal(err)
	}
	if got["DIRECTED_READ"] != "asia-northeast1:READ_ONLY" {
		t.Fatalf("registry after attach = %q", got["DIRECTED_READ"])
	}
	mu.Lock()
	reqs = nil
	mu.Unlock()
	mustExec(t, ctx, handler.Session, "SELECT 1")
	mu.Lock()
	defer mu.Unlock()
	if len(reqs) == 0 {
		t.Fatal("SELECT after attach sent no ExecuteSql")
	}
	requireDirected(t, reqs[len(reqs)-1].DirectedReadOptions, c)
}

func TestDirectedReadROVariantsGuardsAndOmits(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	srv, conn := startDirectedReadDial(t)
	b := mustParseDirectedRead(t, "us-west1:READ_WRITE")
	vars := newDirectedReadVars(t)
	session, _ := newDirectedReadProductSession(t, conn, vars)
	if err := vars.SetFromSimple("DIRECTED_READ", "us-west1:READ_WRITE"); err != nil {
		t.Fatal(err)
	}

	if _, err := session.ExecuteStatement(ctx, &BeginRoStatement{}); err != nil {
		t.Fatalf("BEGIN RO: %v", err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "DIRECTED_READ", Value: "'us-east1'"}); err == nil || !strings.Contains(err.Error(), "active transaction") {
		t.Fatalf("SET during RO: %v", err)
	}
	srv.takeRequests()
	it, _, err := session.txn.RunQuery(ctx, spanner.Statement{SQL: "SELECT 'observed'"})
	if err != nil {
		t.Fatal(err)
	}
	got := consumeDirectedReadRequest(t, srv, it, b)
	if string(got.GetTransaction().GetId()) != "probe-ro" {
		t.Fatalf("subsequent RO txn=%v", got.Transaction)
	}

	srv.takeRequests()
	singleIt, singleTxn, err := session.txn.RunSingleUseQueryWithStats(ctx, spanner.Statement{SQL: "SELECT 'observed'"}, sppb.ExecuteSqlRequest_PROFILE)
	if err != nil {
		t.Fatal(err)
	}
	if singleIt == nil {
		t.Fatal("independent single-use iterator is nil")
	}
	singleReq := consumeDirectedReadRequest(t, srv, singleIt, b)
	if singleTxn != nil {
		singleTxn.Close()
	}
	if singleReq.GetTransaction().GetSingleUse() == nil {
		t.Fatalf("independent single-use missing txn selector: %v", singleReq.Transaction)
	}
	if !session.txn.InReadOnlyTransaction() {
		t.Fatal("independent single consumed user RO txn")
	}
	if _, err := session.ExecuteStatement(ctx, &CommitStatement{}); err != nil {
		t.Fatalf("CLOSE/COMMIT RO: %v", err)
	}

	if _, err := session.ExecuteStatement(ctx, &BeginRwStatement{}); err != nil {
		t.Fatalf("BEGIN RW: %v", err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: "DIRECTED_READ", Value: "'us-east1'"}); err == nil || !strings.Contains(err.Error(), "active transaction") {
		t.Fatalf("SET during RW: %v", err)
	}
	srv.takeRequests()
	it, _, err = session.txn.RunQuery(ctx, spanner.Statement{SQL: "SELECT 'observed'"})
	if err != nil {
		t.Fatal(err)
	}
	rwSel := consumeDirectedReadRequest(t, srv, it, nil)
	if rwSel.GetRequestOptions().GetRequestTag() == "spanner_mycli_heartbeat" {
		t.Fatal("RW SELECT used heartbeat tag")
	}
	srv.takeRequests()
	if _, _, err := session.txn.RunAnalyzeQuery(ctx, spanner.Statement{SQL: "SELECT 'observed'"}); err != nil && !strings.Contains(err.Error(), "query plan") {
		// Plan may be absent from the fake; the request still has to omit DRO.
		_ = err
	}
	for _, req := range srv.takeRequests() {
		requireDirected(t, req.DirectedReadOptions, nil)
	}

	err = session.txn.withReadWriteTransaction(func(txn *spanner.ReadWriteStmtBasedTransaction) error {
		return heartbeat(txn, sppb.RequestOptions_PRIORITY_LOW)
	})
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	var sawHB bool
	for _, req := range srv.takeRequests() {
		if req.GetRequestOptions().GetRequestTag() == "spanner_mycli_heartbeat" {
			sawHB = true
			requireDirected(t, req.DirectedReadOptions, nil)
		}
	}
	if !sawHB {
		t.Fatal("heartbeat ExecuteSql not observed")
	}

	if _, err := session.ExecuteStatement(ctx, &CommitStatement{}); err != nil {
		t.Fatalf("COMMIT: %v", err)
	}
	if err := vars.SetFromSimple("DIRECTED_READ", "us-west1:READ_WRITE"); err != nil {
		t.Fatalf("SET after COMMIT: %v", err)
	}

	exists, err := session.DatabaseExists(ctx)
	if err != nil || !exists {
		t.Fatalf("DatabaseExists: exists=%v err=%v", exists, err)
	}
	existsReqs := srv.takeRequests()
	if len(existsReqs) == 0 {
		t.Fatal("DatabaseExists sent no request")
	}
	requireDirected(t, existsReqs[0].DirectedReadOptions, b)

	vars.Query.DataBoostEnabled = true
	vars.Query.RPCPriority = sppb.RequestOptions_PRIORITY_HIGH
	parts, batch, err := session.txn.RunPartitionQuery(ctx, spanner.Statement{SQL: "SELECT 'observed'"})
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 1 {
		t.Fatalf("parts=%d", len(parts))
	}
	partReq := consumeDirectedReadRequest(t, srv, batch.Execute(ctx, parts[0]), b)
	if partReq.GetPartitionToken() == nil {
		t.Fatal("partition token missing")
	}
	if !partReq.GetDataBoostEnabled() {
		t.Fatal("partition DataBoostEnabled not forwarded")
	}
	if partReq.GetRequestOptions().GetPriority() != sppb.RequestOptions_PRIORITY_HIGH {
		t.Fatalf("partition priority=%v", partReq.GetRequestOptions().GetPriority())
	}
	batch.Close()

	srv.takeRequests()
	if _, err := executePDML(ctx, session, "UPDATE T SET V = 1 WHERE TRUE"); err != nil {
		t.Logf("PDML execution: %v", err)
	}
	for _, req := range srv.takeRequests() {
		requireDirected(t, req.DirectedReadOptions, nil)
	}

	f := &fuzzyFinderCommand{cli: &Cli{SessionHandler: NewSessionHandler(session)}}
	vars.Transaction.RequestTag = "keep-me"
	srv.takeRequests()
	items, err := f.fetchSchemaCandidates(ctx)
	if err != nil {
		t.Fatalf("completion: %v", err)
	}
	if vars.Transaction.RequestTag != "keep-me" {
		t.Fatal("completion consumed STATEMENT_TAG")
	}
	if session.txn.InTransaction() {
		t.Fatal("completion started a user transaction")
	}
	comp := srv.takeRequests()
	if len(comp) == 0 {
		t.Fatal("completion sent no request")
	}
	requireDirected(t, comp[0].DirectedReadOptions, b)
	_ = items
}

func TestDirectedReadDumpSnapshotWire(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	srv, conn := startDirectedReadDial(t)
	b := mustParseDirectedRead(t, "us-west1:READ_WRITE")
	vars := newDirectedReadVars(t)
	session, _ := newDirectedReadProductSession(t, conn, vars)
	if err := vars.SetFromSimple("DIRECTED_READ", "us-west1:READ_WRITE"); err != nil {
		t.Fatal(err)
	}
	session.dumpDDLOverride = func(context.Context) (*adminpb.GetDatabaseDdlResponse, error) {
		return &adminpb.GetDatabaseDdlResponse{Statements: []string{"CREATE TABLE T (Id INT64) PRIMARY KEY (Id)"}}, nil
	}

	srv.takeRequests()
	if _, err := executeDump(ctx, session, dumpModeTables, []tableID{{Name: "T"}}); err != nil {
		t.Fatalf("buffered DUMP TABLES: %v", err)
	}
	buffered := srv.takeRequests()
	if len(buffered) == 0 {
		t.Fatal("DUMP sent no ExecuteSql")
	}
	var sawCatalog, sawColumns, sawData bool
	var txnID []byte
	for _, req := range buffered {
		requireDirected(t, req.DirectedReadOptions, b)
		if txnID == nil {
			txnID = req.GetTransaction().GetId()
		}
		if len(txnID) > 0 && len(req.GetTransaction().GetId()) > 0 && string(req.GetTransaction().GetId()) != string(txnID) {
			t.Fatalf("DUMP used multiple txn ids %q vs %q", txnID, req.GetTransaction().GetId())
		}
		switch {
		case strings.Contains(req.Sql, "INFORMATION_SCHEMA.TABLES"):
			sawCatalog = true
		case strings.Contains(req.Sql, "INFORMATION_SCHEMA.COLUMNS"):
			sawColumns = true
		case strings.Contains(req.Sql, "SELECT Id FROM") || strings.Contains(req.Sql, "SELECT `Id` FROM"):
			sawData = true
		}
	}
	if !sawCatalog || !sawColumns {
		t.Fatalf("DUMP paths catalog=%v columns=%v data=%v sqls=%v", sawCatalog, sawColumns, sawData, dumpSQLs(buffered))
	}

	var stream bytes.Buffer
	vars.StreamManager = streamio.NewStreamManager(io.NopCloser(strings.NewReader("")), &stream, io.Discard)
	srv.takeRequests()
	if _, err := executeDump(ctx, session, dumpModeTables, []tableID{{Name: "T"}}); err != nil {
		t.Fatalf("streaming DUMP TABLES: %v", err)
	}
	streamed := srv.takeRequests()
	if len(streamed) == 0 {
		t.Fatal("streaming DUMP sent no ExecuteSql")
	}
	for _, req := range streamed {
		requireDirected(t, req.DirectedReadOptions, b)
	}

	vars.Display.DumpCyclicMode = enums.DumpCyclicModeMutate
	srv.cycleFK = true
	srv.takeRequests()
	if _, err := executeDump(ctx, session, dumpModeTables, []tableID{{Name: "T"}}); err != nil {
		t.Fatalf("cyclic MUTATE DUMP: %v", err)
	}
	cyclic := srv.takeRequests()
	if len(cyclic) == 0 {
		t.Fatal("cyclic DUMP sent no ExecuteSql")
	}
	for _, req := range cyclic {
		requireDirected(t, req.DirectedReadOptions, b)
	}
}

func dumpSQLs(reqs []*sppb.ExecuteSqlRequest) []string {
	out := make([]string, 0, len(reqs))
	for _, req := range reqs {
		out = append(out, req.Sql)
	}
	return out
}
