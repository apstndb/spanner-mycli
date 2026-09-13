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
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestSavepointOwnerJournalRecordsExplicitSQL(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	h.tm.enableSavepointCaptureForTest()
	session := sessionForTM(t, h.tm)
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	entries := replayJournal(h.tm)
	if len(entries) != 1 || entries[0].kind != replayKindSQL || entries[0].stmt.SQL != "SELECT 1" {
		t.Fatalf("journal = %+v", entries)
	}
	if len(entries[0].fingerprint) != 32 {
		t.Fatalf("fingerprint size %d", len(entries[0].fingerprint))
	}
}

func TestSavepointOwnerJournalSkipsImplicitRW(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	h.tm.enableSavepointCaptureForTest()
	_, err := h.tm.RunInNewOrExistRwTx(ctx, func(tx *spanner.ReadWriteStmtBasedTransaction, implicit bool) (int64, *sppb.QueryPlan, *sppb.ResultSetMetadata, error) {
		if !implicit {
			t.Fatal("expected implicit RW")
		}
		if h.tm.capturingLocked() {
			t.Fatal("implicit RW attached a journal")
		}
		return 0, nil, nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSavepointOwnerJournalSkipsPlanAndHeartbeat(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	h.tm.enableSavepointCaptureForTest()
	session := sessionForTM(t, h.tm)
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	if _, _, err := h.tm.RunAnalyzeQuery(ctx, spanner.NewStatement("SELECT 1")); err != nil {
		t.Fatal(err)
	}
	if entries := replayJournal(h.tm); len(entries) != 0 {
		t.Fatalf("PLAN journaled: %+v", entries)
	}
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	before := replayJournal(h.tm)
	if len(before) != 1 {
		t.Fatalf("user query journal: %+v", before)
	}
	select {
	case h.ticks <- time.Now():
	case <-time.After(2 * time.Second):
		t.Fatal("heartbeat tick blocked")
	}
	select {
	case <-h.server.heartbeatStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("heartbeat query was not sent")
	}
	after := replayJournal(h.tm)
	if len(after) != 1 {
		t.Fatalf("heartbeat journaled: %+v", after)
	}
}

func TestSavepointOwnerJournalDMLAndBatchAndMutate(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	h.tm.enableSavepointCaptureForTest()
	session := sessionForTM(t, h.tm)
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	if _, err := executeDML(ctx, session, "INSERT INTO T (id) VALUES (1)"); err != nil {
		t.Fatal(err)
	}
	dml := replayJournal(h.tm)
	if len(dml) != 1 || dml[0].kind != replayKindSQL || dml[0].stmt.SQL != "INSERT INTO T (id) VALUES (1)" {
		t.Fatalf("DML journal: %+v", dml)
	}

	ok, err := h.tm.TryEnqueueAutomaticDML(spanner.NewStatement("INSERT INTO T (id) VALUES (2)"))
	if err != nil || !ok {
		t.Fatalf("enqueue: ok=%v err=%v", ok, err)
	}
	if len(replayJournal(h.tm)) != 1 {
		t.Fatal("queued automatic DML became a journal entry")
	}
	if len(replayQueued(h.tm)) != 1 {
		t.Fatal("queued automatic DML was not reserved")
	}
	if _, err := h.tm.FlushAutomaticDML(ctx); err != nil {
		t.Fatal(err)
	}
	entries := replayJournal(h.tm)
	if len(entries) != 2 || entries[1].kind != replayKindBatchDML {
		t.Fatalf("flush journal: %+v", entries)
	}
	if len(replayQueued(h.tm)) != 0 {
		t.Fatal("queued automatic DML survived flush")
	}

	if _, err := executeBatchDML(ctx, session, []spanner.Statement{spanner.NewStatement("INSERT INTO T (id) VALUES (3)")}); err != nil {
		t.Fatal(err)
	}
	entries = replayJournal(h.tm)
	if len(entries) != 3 || entries[2].kind != replayKindBatchDML {
		t.Fatalf("manual batch journal: %+v", entries)
	}

	if _, err := (&MutateStatement{Table: "T", Operation: "INSERT", Body: "STRUCT(1 AS id)"}).Execute(ctx, session, OperationOutput{}); err != nil {
		t.Fatal(err)
	}
	if _, err := (&MutateStatement{Table: "T", Operation: "DELETE", Body: "ALL"}).Execute(ctx, session, OperationOutput{}); err != nil {
		t.Fatal(err)
	}
	entries = replayJournal(h.tm)
	if len(entries) != 5 || entries[3].kind != replayKindMutate || entries[4].kind != replayKindMutate {
		t.Fatalf("mutate journal: %+v", entries)
	}
	if !entries[4].mutations[0].DeleteAll {
		t.Fatal("DELETE ALL was not frozen")
	}

	if _, err := (&MutateStatement{Table: "T", Operation: "DELETE", Body: "KEY_RANGE(start_closed=>(1), end_open=>(10))"}).Execute(ctx, session, OperationOutput{}); err != nil {
		t.Fatal(err)
	}
	entries = replayJournal(h.tm)
	if len(entries) != 6 || entries[5].kind != replayKindMutate || entries[5].mutations[0].KeyRange == nil {
		t.Fatalf("key range mutate journal: %+v", entries)
	}
	if entries[5].mutations[0].KeyRange.Kind != spanner.ClosedOpen {
		t.Fatalf("key range kind = %v", entries[5].mutations[0].KeyRange.Kind)
	}
}

func TestSavepointOwnerJournalFreezesCtorAndRejectsNonGCV(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	h.tm.enableSavepointCaptureForTest()
	delay := 5 * time.Millisecond
	h.tm.sysVars.Transaction.ExcludeTxnFromChangeStreams = true
	h.tm.sysVars.Transaction.ReadLockMode = sppb.TransactionOptions_ReadWrite_OPTIMISTIC
	h.tm.sysVars.Transaction.MaxCommitDelay = &delay
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_SERIALIZABLE, sppb.RequestOptions_PRIORITY_HIGH); err != nil {
		t.Fatal(err)
	}
	opts := replayCtor(h.tm)
	if !opts.ExcludeTxnFromChangeStreams || opts.ReadLockMode != sppb.TransactionOptions_ReadWrite_OPTIMISTIC || opts.IsolationLevel != sppb.TransactionOptions_SERIALIZABLE {
		t.Fatalf("ctorOpts = %+v", opts)
	}
	if opts.CommitOptions.MaxCommitDelay == nil || *opts.CommitOptions.MaxCommitDelay != delay {
		t.Fatalf("MaxCommitDelay = %v", opts.CommitOptions.MaxCommitDelay)
	}
	h.tm.sysVars.Transaction.MaxCommitDelay = nil
	if replayCtor(h.tm).CommitOptions.MaxCommitDelay == nil {
		t.Fatal("ctor snapshot aliased live MaxCommitDelay")
	}

	_, _, err := h.tm.RunQueryWithStats(ctx, spanner.Statement{SQL: "SELECT @p", Params: map[string]any{"p": int64(1)}}, false, sppb.ExecuteSqlRequest_PROFILE)
	if err == nil {
		t.Fatal("non-GCV params were frozen")
	}
}

func TestSavepointOwnerJournalPendingBeginKeepsReplay(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	h.tm.enableSavepointCaptureForTest()
	session := sessionForTM(t, h.tm)
	if err := h.tm.BeginPendingTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	if _, err := h.tm.DetermineTransaction(ctx); err != nil {
		t.Fatal(err)
	}
	if replayJournal(h.tm) == nil && !func() bool {
		h.tm.mu.RLock()
		defer h.tm.mu.RUnlock()
		return h.tm.tc != nil && h.tm.tc.replay != nil
	}() {
		t.Fatal("pending BEGIN did not attach a journal")
	}
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	entries := replayJournal(h.tm)
	if len(entries) != 1 {
		t.Fatalf("pending activation journal: %+v", entries)
	}
}

func TestSavepointOwnerJournalDMLFingerprintUsesAffectedCount(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	h.tm.enableSavepointCaptureForTest()
	session := sessionForTM(t, h.tm)
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	if _, err := executeDML(ctx, session, "INSERT INTO T (id) VALUES (1)"); err != nil {
		t.Fatal(err)
	}
	entries := replayJournal(h.tm)
	if len(entries) != 2 {
		t.Fatalf("journal: %+v", entries)
	}
	if bytesEqual := len(entries[0].fingerprint) > 0 && len(entries[1].fingerprint) > 0 && string(entries[0].fingerprint) == string(entries[1].fingerprint); bytesEqual {
		t.Fatal("query and DML fingerprints collided")
	}
}

func TestSavepointOwnerCaptureTokenIgnoresUnadmittedAndStaleCompletion(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	h.tm.enableSavepointCaptureForTest()
	session := sessionForTM(t, h.tm)
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	owner := txnContext(h.tm)
	iter, _, tok, err := h.tm.runQueryWithStatsAndCapture(ctx, spanner.NewStatement("SELECT 1"), false, sppb.ExecuteSqlRequest_PROFILE)
	if err != nil {
		t.Fatal(err)
	}
	if tok == nil {
		t.Fatal("query A was not admitted")
	}
	defer iter.Stop()

	_, err = executeSQLImplWithVars(ctx, session, "SELECT 2", session.systemVariables, OperationOutput{w: io.Discard})
	if err == nil || !strings.Contains(err.Error(), "already in flight") {
		t.Fatalf("query B error = %v, want already in flight", err)
	}
	if txnContext(h.tm) != owner {
		t.Fatal("unadmitted query B replaced the owner")
	}
	h.tm.mu.RLock()
	pending, inFlight := h.tm.tc.pending, h.tm.tc.inFlight
	h.tm.mu.RUnlock()
	if pending != tok || inFlight != 1 {
		t.Fatalf("B completed A's capture: pending=%v inFlight=%d", pending != tok, inFlight)
	}

	_, _, _, _, err = consumeRowIterObserving(iter, func(*spanner.Row) error { return nil }, tok.receipt())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tok.receipt().Finish(nil); err != nil {
		t.Fatal(err)
	}
	if err := h.tm.finishQueryCapture(tok, nil); err != nil {
		t.Fatal(err)
	}
	entries := replayJournal(h.tm)
	if len(entries) != 1 || entries[0].stmt.SQL != "SELECT 1" {
		t.Fatalf("A was not journaled: %+v", entries)
	}

	iter2, _, staleTok, err := h.tm.runQueryWithStatsAndCapture(ctx, spanner.NewStatement("SELECT 1"), false, sppb.ExecuteSqlRequest_PROFILE)
	if err != nil {
		t.Fatal(err)
	}
	if staleTok == nil {
		t.Fatal("replacement query was not admitted")
	}
	defer iter2.Stop()
	h.tm.mu.Lock()
	h.tm.tc.attempt++
	replacement := &captureToken{owner: h.tm.tc, attempt: h.tm.tc.attempt, rec: &operationReceipt{}, reserved: 48}
	h.tm.tc.pending = replacement
	h.tm.tc.inFlight = 1
	afterReplace := h.tm.tc.replay.retainedBytes
	h.tm.mu.Unlock()
	if err := h.tm.finishQueryCapture(staleTok, errors.New("stale completion")); err == nil || err.Error() != "stale completion" {
		t.Fatalf("stale finish = %v, want stale completion", err)
	}
	h.tm.mu.Lock()
	defer h.tm.mu.Unlock()
	if h.tm.tc.pending != replacement || h.tm.tc.inFlight != 1 {
		t.Fatal("stale completion finished the replacement operation")
	}
	if h.tm.tc.replay.retainedBytes != afterReplace {
		t.Fatal("stale completion released replacement reservation")
	}
}

func TestSavepointOwnerLocalAdmissionPreservesOwnerWithoutRPC(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	h.tm.enableSavepointCaptureForTest()
	session := sessionForTM(t, h.tm)
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	owner := txnContext(h.tm)

	stmt := spanner.NewStatement("INSERT INTO T (id) VALUES (2)")
	frozen, err := freezeStatement(stmt.SQL, stmt.Params, spanner.QueryOptions{LastStatement: false})
	if err != nil {
		t.Fatal(err)
	}
	full := batchReplayAccounted([]frozenStatement{frozen})
	payload := frozen.payloadBytes()
	if full <= payload {
		t.Fatal("batch accounted bytes should include fingerprint and count-vector overhead")
	}
	h.tm.mu.Lock()
	h.tm.tc.replay.retainedBytes = savepointJournalLimit - payload
	h.tm.mu.Unlock()
	ok, err := h.tm.TryEnqueueAutomaticDML(stmt)
	if ok || !errors.Is(err, errSavepointJournalFull) {
		t.Fatalf("enqueue: ok=%v err=%v", ok, err)
	}
	if txnContext(h.tm) != owner {
		t.Fatal("automatic DML admission retired the owner")
	}
	if len(h.server.batchObservations()) != 0 {
		t.Fatalf("automatic enqueue issued ExecuteBatchDml: %v", h.server.batchObservations())
	}

	h.tm.mu.Lock()
	h.tm.tc.replay.retainedBytes = savepointJournalLimit
	h.tm.mu.Unlock()
	if _, err := executeBatchDML(ctx, session, []spanner.Statement{spanner.NewStatement("INSERT INTO T (id) VALUES (3)")}); !errors.Is(err, errSavepointJournalFull) {
		t.Fatalf("manual batch: %v", err)
	}
	if txnContext(h.tm) != owner {
		t.Fatal("manual Batch DML admission retired the owner")
	}
	if n := len(h.server.batchObservations()); n != 0 {
		t.Fatalf("manual Batch DML issued ExecuteBatchDml (%d): %v", n, h.server.batchObservations())
	}

	if _, err := (&MutateStatement{Table: "T", Operation: "INSERT", Body: "STRUCT(1 AS id)"}).Execute(ctx, session, OperationOutput{}); !errors.Is(err, errSavepointJournalFull) {
		t.Fatalf("mutate: %v", err)
	}
	if txnContext(h.tm) != owner {
		t.Fatal("MUTATE admission retired the owner")
	}

	parsed, err := parseMutation("T", "DELETE", "KEY_RANGE(start_closed=>(1), end_open=>(10))")
	if err != nil {
		t.Fatal(err)
	}
	frozenMut, muts, err := freezeMutate("T", "DELETE", "KEY_RANGE(start_closed=>(1), end_open=>(10))")
	if err != nil {
		t.Fatal(err)
	}
	if frozenMut[0].KeyRange == nil || frozenMut[0].KeyRange.Kind != spanner.ClosedOpen {
		t.Fatalf("frozen key range: %+v", frozenMut)
	}
	if diff := cmp.Diff(parsed, muts, cmp.AllowUnexported(spanner.Mutation{}), protocmp.Transform()); diff != "" {
		t.Fatalf("KEY_RANGE parse/freeze mismatch (-want +got):\n%s", diff)
	}
}
