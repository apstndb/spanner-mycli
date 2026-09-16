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
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	runPartitionChildEnv          = "SPANNER_MYCLI_RUN_PARTITION_CHILD"
	runPartitionChildRoleProducer = "producer"
	runPartitionChildRoleConsumer = "consumer"
)

type runPartitionWireServer struct {
	partitionFanInServer
	mu       sync.Mutex
	deletes  atomic.Int32
	execReqs []*sppb.ExecuteSqlRequest
}

func (s *runPartitionWireServer) DeleteSession(context.Context, *sppb.DeleteSessionRequest) (*emptypb.Empty, error) {
	s.deletes.Add(1)
	return &emptypb.Empty{}, nil
}

func (s *runPartitionWireServer) ExecuteStreamingSql(r *sppb.ExecuteSqlRequest, stream sppb.Spanner_ExecuteStreamingSqlServer) error {
	s.mu.Lock()
	s.execReqs = append(s.execReqs, proto.Clone(r).(*sppb.ExecuteSqlRequest))
	s.mu.Unlock()
	return s.partitionFanInServer.ExecuteStreamingSql(r, stream)
}

func (s *runPartitionWireServer) takeExecs() []*sppb.ExecuteSqlRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.execReqs
	s.execReqs = nil
	return out
}

func startRunPartitionWire(t *testing.T, server *runPartitionWireServer) []option.ClientOption {
	t.Helper()
	if server.nPartitions == 0 {
		server.nPartitions = 2
	}
	if server.rowsPer == 0 {
		server.rowsPer = 1
	}
	return bufconnClientOptions(t, func(s *grpc.Server) {
		sppb.RegisterSpannerServer(s, server)
		registerDirectedReadAdmin(s)
	})
}

func newRunPartitionVars(t *testing.T) *systemVariables {
	t.Helper()
	vars := newSystemVariablesWithDefaultsForTest()
	vars.ensureRegistry()
	vars.Connection = ConnectionVars{Project: "p", Instance: "i", Database: "db"}
	vars.StreamManager = streamio.NewStreamManager(io.NopCloser(strings.NewReader("")), io.Discard, io.Discard)
	vars.Transaction.KeepTransactionAlive = false
	return vars
}

func newRunPartitionSession(t *testing.T, opts []option.ClientOption, vars *systemVariables) *Session {
	t.Helper()
	session, _ := newDirectedReadProductSession(t, opts, vars)
	return session
}

func partitionTokensFromResult(t *testing.T, result *Result) []string {
	t.Helper()
	rows, ok := result.Body.PresentationRows()
	if !ok {
		t.Fatal("PARTITION result is not a presentation body")
	}
	tokens := make([]string, 0, len(rows))
	for _, row := range rows {
		if len(row) == 0 {
			t.Fatal("empty PARTITION row")
		}
		tokens = append(tokens, row[0].RawText())
	}
	return tokens
}

func mustPartition(t *testing.T, session *Session, sql string) []string {
	t.Helper()
	result, err := session.ExecuteStatement(t.Context(), &PartitionStatement{SQL: sql})
	if err != nil {
		t.Fatalf("PARTITION: %v", err)
	}
	tokens := partitionTokensFromResult(t, result)
	if len(tokens) == 0 {
		t.Fatal("PARTITION returned no tokens")
	}
	for _, tok := range tokens {
		if !strings.HasPrefix(tok, partitionTokenPrefix) {
			t.Fatalf("token missing experimental prefix: %q", tok[:min(len(tok), 32)])
		}
	}
	return tokens
}

func TestPartitionExportsCompleteEnvelope(t *testing.T) {
	t.Parallel()
	server := &runPartitionWireServer{}
	opts := startRunPartitionWire(t, server)
	session := newRunPartitionSession(t, opts, newRunPartitionVars(t))
	tokens := mustPartition(t, session, "SELECT * FROM Singers")
	decoded, err := decodePartitionToken(tokens[0], partitionTokenNow())
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Database != session.DatabasePath() {
		t.Fatalf("envelope database = %q, want %q", decoded.Database, session.DatabasePath())
	}
	inspected, err := inspectNativePartition(decoded.TxID, decoded.Partition, session.DatabasePath())
	if err != nil {
		t.Fatal(err)
	}
	if inspected.SQL != "SELECT * FROM Singers" {
		t.Fatalf("inspected SQL = %q", inspected.SQL)
	}
	var txID spanner.BatchReadOnlyTransactionID
	if err := txID.UnmarshalBinary(decoded.TxID); err != nil {
		t.Fatal(err)
	}
	var part spanner.Partition
	if err := part.UnmarshalBinary(decoded.Partition); err != nil {
		t.Fatal(err)
	}
}

func TestRunPartitionProducerExitAndSiblings(t *testing.T) {
	t.Parallel()
	server := &runPartitionWireServer{}
	opts := startRunPartitionWire(t, server)
	producerVars := newRunPartitionVars(t)
	producer := newRunPartitionSession(t, opts, producerVars)
	tokens := mustPartition(t, producer, "SELECT * FROM Singers")
	if len(tokens) != 2 {
		t.Fatalf("tokens=%d, want 2", len(tokens))
	}
	producer.Close()
	if server.deletes.Load() != 0 {
		t.Fatalf("DeleteSession after producer close = %d, want 0", server.deletes.Load())
	}

	consumerA := newRunPartitionSession(t, opts, newRunPartitionVars(t))
	consumerB := newRunPartitionSession(t, opts, newRunPartitionVars(t))
	got := make([]string, 0, 2)
	for i, consumer := range []*Session{consumerA, consumerB} {
		result, err := consumer.ExecuteStatement(t.Context(), &RunPartitionStatement{Token: tokens[i]})
		if err != nil {
			t.Fatalf("consumer %d: %v", i, err)
		}
		if result.PartitionCount != 1 {
			t.Fatalf("PartitionCount=%d, want 1", result.PartitionCount)
		}
		if result.AffectedRows != 1 {
			t.Fatalf("AffectedRows=%d, want 1", result.AffectedRows)
		}
		if result.capture != nil {
			t.Fatal("RUN PARTITION must not journal a SAVEPOINT capture token")
		}
		if result.ReadTimestamp.IsZero() {
			t.Fatal("ReadTimestamp missing")
		}
		got = append(got, collectTypedStrings(t, result)...)
	}
	if !hasAllStrings(got, "0-0", "1-0") {
		t.Fatalf("joint rows = %v", got)
	}
	if server.deletes.Load() != 0 {
		t.Fatalf("DeleteSession after consumers = %d, want 0", server.deletes.Load())
	}
}

func TestRunPartitionSiblingCancelIsolation(t *testing.T) {
	t.Parallel()
	server := &runPartitionWireServer{partitionFanInServer: partitionFanInServer{nPartitions: 2, rowsPer: 1, hangExceptFirst: true}}
	opts := startRunPartitionWire(t, server)
	producer := newRunPartitionSession(t, opts, newRunPartitionVars(t))
	tokens := mustPartition(t, producer, "SELECT * FROM Singers")
	producer.Close()

	hanging := newRunPartitionSession(t, opts, newRunPartitionVars(t))
	ctx, cancel := context.WithCancel(t.Context())
	errCh := make(chan error, 1)
	go func() {
		_, err := hanging.ExecuteStatement(ctx, &RunPartitionStatement{Token: tokens[1]})
		errCh <- err
	}()
	deadline := time.Now().Add(3 * time.Second)
	for server.execs.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if err := <-errCh; !isCancelErr(err) {
		t.Fatalf("hanging consumer: %v", err)
	}

	sibling := newRunPartitionSession(t, opts, newRunPartitionVars(t))
	result, err := sibling.ExecuteStatement(t.Context(), &RunPartitionStatement{Token: tokens[0]})
	if err != nil {
		t.Fatalf("sibling: %v", err)
	}
	if !hasAllStrings(collectTypedStrings(t, result), "0-0") {
		t.Fatalf("sibling rows missing")
	}
}

func TestRunPartitionRejectsBeforeExecute(t *testing.T) {
	t.Parallel()
	server := &runPartitionWireServer{}
	opts := startRunPartitionWire(t, server)
	session := newRunPartitionSession(t, opts, newRunPartitionVars(t))
	tokens := mustPartition(t, session, "SELECT * FROM Singers")
	before := server.execs.Load()

	decoded, err := decodePartitionToken(tokens[0], partitionTokenNow())
	if err != nil {
		t.Fatal(err)
	}

	raw, err := json.Marshal(partitionTokenJSON{
		Database:  testPartitionDBB,
		IssuedAt:  decoded.IssuedAt,
		NotAfter:  decoded.NotAfter,
		TxID:      base64.StdEncoding.EncodeToString(decoded.TxID),
		Partition: base64.StdEncoding.EncodeToString(decoded.Partition),
	})
	if err != nil {
		t.Fatal(err)
	}
	outerWrong := partitionTokenPrefix + base64.RawURLEncoding.EncodeToString(raw)

	emptyReq, err := craftTestQueryPartition([]byte("pt"), &sppb.ExecuteSqlRequest{})
	if err != nil {
		t.Fatal(err)
	}
	emptyTok, err := encodePartitionToken(session.DatabasePath(), partitionTokenNow(), decoded.TxID, emptyReq)
	if err != nil {
		t.Fatal(err)
	}

	wrongInner, err := craftTestQueryPartition([]byte("pt"), &sppb.ExecuteSqlRequest{
		Session:     testPartitionDBB + "/sessions/x",
		Transaction: &sppb.TransactionSelector{Selector: &sppb.TransactionSelector_Id{Id: []byte("tid")}},
		Sql:         "SELECT 1",
	})
	if err != nil {
		t.Fatal(err)
	}
	wrongTx, err := craftTestTxID([]byte("tid"), testPartitionDBB+"/sessions/x", time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	wrongInnerTok, err := encodePartitionToken(session.DatabasePath(), partitionTokenNow(), wrongTx, wrongInner)
	if err != nil {
		t.Fatal(err)
	}

	mixedTok, err := encodePartitionToken(session.DatabasePath(), partitionTokenNow(), decoded.TxID, wrongInner)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		token string
		want  string
	}{
		{name: "legacy bare", token: "not-an-envelope", want: "GetPartitionToken-only"},
		{name: "truncated inner", token: mustEncode(t, session.DatabasePath(), decoded.TxID[:3], decoded.Partition), want: "malformed txid blob"},
		{name: "empty request", token: emptyTok, want: "empty"},
		{name: "mixed blobs", token: mixedTok, want: "native session mismatch"},
		{name: "wrong inner database", token: wrongInnerTok, want: "is not under database"},
		{name: "tampered outer database", token: outerWrong, want: "does not match session"},
		{name: "oversized", token: partitionTokenPrefix + strings.Repeat("A", partitionTokenMaxEncodedBytes), want: "exceeds"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := session.ExecuteStatement(t.Context(), &RunPartitionStatement{Token: tt.token})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
		})
	}
	if server.execs.Load() != before {
		t.Fatalf("ExecuteStreamingSql ran on reject path: before=%d after=%d", before, server.execs.Load())
	}
}

func TestRunPartitionAdmissionMatrix(t *testing.T) {
	t.Parallel()
	server := &runPartitionWireServer{}
	opts := startRunPartitionWire(t, server)
	session := newRunPartitionSession(t, opts, newRunPartitionVars(t))
	tokens := mustPartition(t, session, "SELECT * FROM Singers")
	token := tokens[0]

	assertRejectedBeforeExecute := func(t *testing.T, want string, run func() error) {
		t.Helper()
		before := server.execs.Load()
		err := run()
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("err = %v, want %q", err, want)
		}
		if server.execs.Load() != before {
			t.Fatalf("RUN PARTITION executed RPC: before=%d after=%d", before, server.execs.Load())
		}
	}

	t.Run("pending", func(t *testing.T) {
		if err := session.txn.BeginPendingTransaction(t.Context(), sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
			t.Fatal(err)
		}
		assertRejectedBeforeExecute(t, "idle session", func() error {
			_, err := session.ExecuteStatement(t.Context(), &RunPartitionStatement{Token: token})
			return err
		})
		if err := session.txn.ClosePendingTransaction(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("read-write", func(t *testing.T) {
		if err := session.txn.BeginReadWriteTransaction(t.Context(), sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
			t.Fatal(err)
		}
		assertRejectedBeforeExecute(t, "idle session", func() error {
			_, err := session.ExecuteStatement(t.Context(), &RunPartitionStatement{Token: token})
			return err
		})
		if err := session.txn.RollbackReadWriteTransaction(t.Context()); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("read-only", func(t *testing.T) {
		if _, err := session.txn.BeginReadOnlyTransaction(t.Context(), timestampBoundUnspecified, 0, time.Time{}, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
			t.Fatal(err)
		}
		assertRejectedBeforeExecute(t, "idle session", func() error {
			_, err := session.ExecuteStatement(t.Context(), &RunPartitionStatement{Token: token})
			return err
		})
		if err := session.txn.CloseReadOnlyTransaction(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("manual batch", func(t *testing.T) {
		if err := session.batch.Start(batchModeDML); err != nil {
			t.Fatal(err)
		}
		assertRejectedBeforeExecute(t, "manual batch", func() error {
			_, err := session.ExecuteStatement(t.Context(), &RunPartitionStatement{Token: token})
			return err
		})
		session.batch.Abort()
	})
	t.Run("recovery", func(t *testing.T) {
		local := newSessionForLocalVarTest(t)
		plantOwner(t, local, transactionAttributes{mode: transactionModeReadWrite}, errors.New("injected recovery"))
		assertRejectedBeforeExecute(t, errSavepointRecovery.Error(), func() error {
			_, err := local.ExecuteStatement(t.Context(), &RunPartitionStatement{Token: token})
			return err
		})
	})

	// PARTITION keeps its existing admission: it may run while a pending
	// transaction exists because it opens a separate batch RO txn.
	if err := session.txn.BeginPendingTransaction(t.Context(), sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(t.Context(), &PartitionStatement{SQL: "SELECT * FROM Singers"}); err != nil {
		t.Fatalf("PARTITION during pending should keep existing semantics: %v", err)
	}
	if err := session.txn.ClosePendingTransaction(); err != nil {
		t.Fatal(err)
	}
}

func TestRunPartitionPreservesTokenOptions(t *testing.T) {
	t.Parallel()
	server := &runPartitionWireServer{partitionFanInServer: partitionFanInServer{nPartitions: 1, rowsPer: 1}}
	opts := startRunPartitionWire(t, server)
	producerVars := newRunPartitionVars(t)
	producerVars.Query.DataBoostEnabled = true
	producerVars.Query.RPCPriority = sppb.RequestOptions_PRIORITY_HIGH
	dro := includeDirectedRead(false, replicaSel("us-central1", sppb.DirectedReadOptions_ReplicaSelection_READ_ONLY))
	producerVars.Query.DirectedRead = dro
	producer := newRunPartitionSession(t, opts, producerVars)
	mustExec(t, t.Context(), producer, "SET PARAM active = TRUE")
	tokens := mustPartition(t, producer, "SELECT * FROM Singers WHERE active = @active")
	producer.Close()

	consumerVars := newRunPartitionVars(t)
	consumerVars.Query.DataBoostEnabled = false
	consumerVars.Query.RPCPriority = sppb.RequestOptions_PRIORITY_LOW
	consumerVars.Query.DirectedRead = includeDirectedRead(false, replicaSel("us-east1", sppb.DirectedReadOptions_ReplicaSelection_READ_WRITE))
	consumer := newRunPartitionSession(t, opts, consumerVars)
	server.takeExecs()
	if _, err := consumer.ExecuteStatement(t.Context(), &RunPartitionStatement{Token: tokens[0]}); err != nil {
		t.Fatal(err)
	}
	execs := server.takeExecs()
	if len(execs) != 1 {
		t.Fatalf("execs=%d", len(execs))
	}
	got := execs[0]
	if !got.GetDataBoostEnabled() {
		t.Fatal("Data Boost from token was not preserved")
	}
	if got.GetRequestOptions().GetPriority() != sppb.RequestOptions_PRIORITY_HIGH {
		t.Fatalf("priority=%v", got.GetRequestOptions().GetPriority())
	}
	if !proto.Equal(got.GetDirectedReadOptions(), dro) {
		t.Fatalf("directed-read overlay leaked: %v", got.GetDirectedReadOptions())
	}
	active := got.GetParams().GetFields()["active"]
	if active == nil || !active.GetBoolValue() {
		t.Fatalf("bound param active = %v, want BOOL true", active)
	}
	if typ := got.GetParamTypes()["active"]; typ == nil || typ.GetCode() != sppb.TypeCode_BOOL {
		t.Fatalf("param type active = %v, want BOOL", typ)
	}
}

func TestRunPartitionOutputModes(t *testing.T) {
	t.Parallel()
	server := &runPartitionWireServer{partitionFanInServer: partitionFanInServer{nPartitions: 1, rowsPer: 1}}
	opts := startRunPartitionWire(t, server)
	producer := newRunPartitionSession(t, opts, newRunPartitionVars(t))
	tokens := mustPartition(t, producer, "SELECT * FROM Singers")
	producer.Close()

	t.Run("buffered table", func(t *testing.T) {
		vars := newRunPartitionVars(t)
		vars.Display.CLIFormat = enums.DisplayModeTable
		session := newRunPartitionSession(t, opts, vars)
		result, err := session.ExecuteStatement(t.Context(), &RunPartitionStatement{Token: tokens[0]})
		if err != nil {
			t.Fatal(err)
		}
		if result.Body.AlreadyDelivered() {
			t.Fatal("table path should buffer")
		}
		if !hasAllStrings(collectTypedStrings(t, result), "0-0") {
			t.Fatalf("buffered rows missing")
		}
	})
	t.Run("jsonl", func(t *testing.T) {
		var buf bytes.Buffer
		vars := newRunPartitionVars(t)
		vars.Display.CLIFormat = enums.DisplayModeJSONL
		session := newRunPartitionSession(t, opts, vars)
		result, err := session.ExecuteStatementWithOutput(t.Context(), &RunPartitionStatement{Token: tokens[0]}, OperationOutput{w: &buf})
		if err != nil {
			t.Fatal(err)
		}
		if !result.Body.AlreadyDelivered() {
			t.Fatal("JSONL should stream")
		}
		if !strings.Contains(buf.String(), "0-0") {
			t.Fatalf("JSONL output = %q", buf.String())
		}
	})
	t.Run("csv", func(t *testing.T) {
		var buf bytes.Buffer
		vars := newRunPartitionVars(t)
		vars.Display.CLIFormat = enums.DisplayModeCSV
		session := newRunPartitionSession(t, opts, vars)
		result, err := session.ExecuteStatementWithOutput(t.Context(), &RunPartitionStatement{Token: tokens[0]}, OperationOutput{w: &buf})
		if err != nil {
			t.Fatal(err)
		}
		if !result.Body.AlreadyDelivered() {
			t.Fatal("CSV should stream")
		}
		if !strings.Contains(buf.String(), "0-0") {
			t.Fatalf("CSV output = %q", buf.String())
		}
	})
	t.Run("sql export explicit", func(t *testing.T) {
		var buf bytes.Buffer
		vars := newRunPartitionVars(t)
		vars.Display.CLIFormat = enums.DisplayModeSQLInsert
		vars.Display.SQLTableName = "ExplicitSingers"
		session := newRunPartitionSession(t, opts, vars)
		result, err := session.ExecuteStatementWithOutput(t.Context(), &RunPartitionStatement{Token: tokens[0]}, OperationOutput{w: &buf})
		if err != nil {
			t.Fatal(err)
		}
		if result.SQLTableNameForExport != "ExplicitSingers" {
			t.Fatalf("SQLTableNameForExport=%q", result.SQLTableNameForExport)
		}
		if !strings.Contains(buf.String(), "ExplicitSingers") {
			t.Fatalf("SQL export = %q", buf.String())
		}
	})
	t.Run("sql export auto-detect", func(t *testing.T) {
		var buf bytes.Buffer
		vars := newRunPartitionVars(t)
		vars.Display.CLIFormat = enums.DisplayModeSQLInsert
		session := newRunPartitionSession(t, opts, vars)
		result, err := session.ExecuteStatementWithOutput(t.Context(), &RunPartitionStatement{Token: tokens[0]}, OperationOutput{w: &buf})
		if err != nil {
			t.Fatal(err)
		}
		if result.SQLTableNameForExport != "Singers" {
			t.Fatalf("SQLTableNameForExport=%q, want Singers from inspected SQL", result.SQLTableNameForExport)
		}
		if !strings.Contains(buf.String(), "Singers") {
			t.Fatalf("SQL export = %q", buf.String())
		}
	})
}

func TestRunPartitionWriterError(t *testing.T) {
	t.Parallel()
	server := &runPartitionWireServer{partitionFanInServer: partitionFanInServer{nPartitions: 1, rowsPer: 8}}
	opts := startRunPartitionWire(t, server)
	producer := newRunPartitionSession(t, opts, newRunPartitionVars(t))
	tokens := mustPartition(t, producer, "SELECT * FROM Singers")
	producer.Close()

	vars := newRunPartitionVars(t)
	vars.Display.CLIFormat = enums.DisplayModeCSV
	session := newRunPartitionSession(t, opts, vars)
	_, err := session.ExecuteStatementWithOutput(t.Context(), &RunPartitionStatement{Token: tokens[0]}, OperationOutput{w: &failAfterWrites{after: 1}})
	if err == nil || !strings.Contains(err.Error(), "injected write failure") {
		t.Fatalf("got %v, want injected write failure", err)
	}
	if isCancelErr(err) || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("write failure must not be a cancel/deadline substitute: %v", err)
	}
}

func TestRunPartitionSecondProcess(t *testing.T) {
	switch os.Getenv(runPartitionChildEnv) {
	case runPartitionChildRoleProducer:
		runPartitionProducerChild(t)
		return
	case runPartitionChildRoleConsumer:
		runPartitionConsumerChild(t)
		return
	}

	// Local fake-RPC only: this proves a producing process can exit while
	// sibling tokens remain usable. It is not Cloud retention or
	// cross-principal evidence.
	server := &runPartitionWireServer{partitionFanInServer: partitionFanInServer{nPartitions: 2, rowsPer: 1}}
	addr, stop := startRunPartitionTCP(t, server)
	t.Cleanup(stop)

	outFile := filepath.Join(t.TempDir(), "tokens.json")
	producerCtx, producerCancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer producerCancel()
	producerOut := runPartitionTestChild(t, producerCtx, []string{
		runPartitionChildEnv + "=" + runPartitionChildRoleProducer,
		"SPANNER_MYCLI_RUN_PARTITION_ADDR=" + addr,
		"SPANNER_MYCLI_RUN_PARTITION_OUT=" + outFile,
	})
	if !strings.Contains(string(producerOut), "PRODUCER_OK") {
		t.Fatalf("producer output:\n%s", producerOut)
	}
	if server.deletes.Load() != 0 {
		t.Fatalf("DeleteSession after producer exit = %d", server.deletes.Load())
	}

	raw, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	var tokens []string
	if err := json.Unmarshal(raw, &tokens); err != nil {
		t.Fatal(err)
	}
	if len(tokens) != 2 {
		t.Fatalf("exported tokens=%d, want 2", len(tokens))
	}

	wantRows := []string{"0-0", "1-0"}
	for i, tok := range tokens {
		consumerCtx, consumerCancel := context.WithTimeout(t.Context(), 15*time.Second)
		out := runPartitionTestChild(t, consumerCtx, []string{
			runPartitionChildEnv + "=" + runPartitionChildRoleConsumer,
			"SPANNER_MYCLI_RUN_PARTITION_ADDR=" + addr,
			"SPANNER_MYCLI_RUN_PARTITION_TOKEN=" + tok,
		})
		consumerCancel()
		if !strings.Contains(string(out), "CHILD_OK "+wantRows[i]) {
			t.Fatalf("consumer %d output:\n%s", i, out)
		}
	}
	if server.deletes.Load() != 0 {
		t.Fatalf("DeleteSession after separate consumers = %d", server.deletes.Load())
	}
}

func runPartitionTestChild(t *testing.T, ctx context.Context, extraEnv []string) []byte {
	t.Helper()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRunPartitionSecondProcess$", "-test.v", "-test.count=1")
	cmd.Env = append(os.Environ(), extraEnv...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("child: %v\n%s", err, out)
	}
	if cmd.ProcessState == nil || !cmd.ProcessState.Exited() {
		t.Fatal("child process did not exit")
	}
	return out
}

func runPartitionProducerChild(t *testing.T) {
	t.Helper()
	addr := os.Getenv("SPANNER_MYCLI_RUN_PARTITION_ADDR")
	outFile := os.Getenv("SPANNER_MYCLI_RUN_PARTITION_OUT")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	session, err := newRunPartitionTCPSession(ctx, t, addr)
	if err != nil {
		t.Fatal(err)
	}
	tokens := mustPartition(t, session, "SELECT * FROM Singers")
	session.Close()
	raw, err := json.Marshal(tokens)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outFile, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	fmt.Println("PRODUCER_OK")
}

func runPartitionConsumerChild(t *testing.T) {
	t.Helper()
	addr := os.Getenv("SPANNER_MYCLI_RUN_PARTITION_ADDR")
	token := os.Getenv("SPANNER_MYCLI_RUN_PARTITION_TOKEN")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	session, err := newRunPartitionTCPSession(ctx, t, addr)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	result, err := session.ExecuteStatement(ctx, &RunPartitionStatement{Token: token})
	if err != nil {
		t.Fatal(err)
	}
	rows := collectTypedStrings(t, result)
	fmt.Printf("CHILD_OK %s\n", strings.Join(rows, ","))
}

func startRunPartitionTCP(t *testing.T, server *runPartitionWireServer) (addr string, stop func()) {
	t.Helper()
	if server.nPartitions == 0 {
		server.nPartitions = 2
	}
	if server.rowsPer == 0 {
		server.rowsPer = 1
	}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	grpcServer := grpc.NewServer()
	sppb.RegisterSpannerServer(grpcServer, server)
	adminpb.RegisterDatabaseAdminServer(grpcServer, &directedReadAdminServer{})
	go func() {
		if err := grpcServer.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Errorf("tcp serve: %v", err)
		}
	}()
	return lis.Addr().String(), func() {
		grpcServer.Stop()
		_ = lis.Close()
	}
}

func newRunPartitionTCPSession(ctx context.Context, t *testing.T, addr string) (*Session, error) {
	t.Helper()
	vars := newRunPartitionVars(t)
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	t.Cleanup(func() { _ = conn.Close() })
	opts := []option.ClientOption{
		option.WithoutAuthentication(),
		option.WithGRPCConn(conn),
	}
	session, err := newSessionWithFactories(ctx, vars, vars.Connection,
		func(ctx context.Context, db string, cfg spanner.ClientConfig, clientOpts ...option.ClientOption) (*spanner.Client, error) {
			cfg.DisableNativeMetrics = true
			return spanner.NewClientWithConfig(ctx, db, cfg, clientOpts...)
		},
		adminapi.NewDatabaseAdminClient,
		func(c *spanner.Client) { c.Close() },
		opts...,
	)
	if err != nil {
		return nil, err
	}
	bindLiveSessionCallbacks(vars, session)
	return session, nil
}

func collectTypedStrings(t *testing.T, result *Result) []string {
	t.Helper()
	typed, ok := result.Body.Typed()
	if !ok || typed == nil {
		return nil
	}
	var values []string
	for _, row := range typed.Rows {
		var v string
		if err := row.Column(0, &v); err != nil {
			t.Fatal(err)
		}
		values = append(values, v)
	}
	return values
}

func hasAllStrings(got []string, want ...string) bool {
	have := make(map[string]int, len(got))
	for _, g := range got {
		have[g]++
	}
	for _, w := range want {
		if have[w] == 0 {
			return false
		}
	}
	return true
}

func mustEncode(t *testing.T, database string, tx, part []byte) string {
	t.Helper()
	tok, err := encodePartitionToken(database, partitionTokenNow(), tx, part)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}
