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
	"github.com/apstndb/spanner-mycli/enums"
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
	if opts.CommitPriority != sppb.RequestOptions_PRIORITY_HIGH {
		t.Fatalf("inherited CommitPriority = %v, want HIGH", opts.CommitPriority)
	}
	h.tm.sysVars.Transaction.MaxCommitDelay = nil
	h.tm.sysVars.Transaction.CommitPriority = sppb.RequestOptions_PRIORITY_LOW
	h.tm.sysVars.Query.RPCPriority = sppb.RequestOptions_PRIORITY_MEDIUM
	replayed := replayCtor(h.tm)
	if replayed.CommitOptions.MaxCommitDelay == nil {
		t.Fatal("ctor snapshot aliased live MaxCommitDelay")
	}
	if replayed.CommitPriority != sppb.RequestOptions_PRIORITY_HIGH {
		t.Fatal("ctor snapshot followed later COMMIT_PRIORITY or RPC_PRIORITY")
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

func TestDMLQueryModeMatchesCallerEffectiveMode(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		capture bool
	}{
		{name: "capture_enabled", capture: true},
		{name: "capture_disabled", capture: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			h := newHeartbeatHarness(t)
			if tc.capture {
				h.tm.enableSavepointCaptureForTest()
			}
			h.tm.sysVars.Query.QueryMode = sppb.ExecuteSqlRequest_WITH_STATS.Enum()
			session := sessionForTM(t, h.tm)
			if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
				t.Fatal(err)
			}

			const dmlSQL = "UPDATE T SET v = 1 WHERE id = 1"
			if _, err := executeDML(ctx, session, dmlSQL); err != nil {
				t.Fatal(err)
			}
			obs, ok := lastSQLObservation(h.server.sqlObservations(), dmlSQL)
			if !ok {
				t.Fatal("ordinary DML was not observed on the wire")
			}
			if obs.queryMode != sppb.ExecuteSqlRequest_WITH_STATS {
				t.Fatalf("ordinary DML QueryMode = %v, want WITH_STATS", obs.queryMode)
			}
			assertFrozenSQLMode(t, h.tm, tc.capture, dmlSQL, sppb.ExecuteSqlRequest_WITH_STATS)

			const explainSQL = "UPDATE T SET v = 2 WHERE id = 1"
			_, err := executeExplainAnalyzeDML(ctx, session, explainSQL, enums.ExplainFormatUnspecified, 0, nil)
			if err != nil && !errors.Is(err, errExplainAnalyzeUnsupportedOnEmulator) {
				t.Fatalf("EXPLAIN ANALYZE DML: %v", err)
			}
			obs, ok = lastSQLObservation(h.server.sqlObservations(), explainSQL)
			if !ok {
				t.Fatal("EXPLAIN ANALYZE DML was not observed on the wire")
			}
			if obs.queryMode != sppb.ExecuteSqlRequest_PROFILE {
				t.Fatalf("EXPLAIN ANALYZE DML QueryMode = %v, want PROFILE", obs.queryMode)
			}
			assertFrozenSQLMode(t, h.tm, tc.capture, explainSQL, sppb.ExecuteSqlRequest_PROFILE)
		})
	}
}

func assertFrozenSQLMode(t *testing.T, tm *TransactionManager, capture bool, sql string, want sppb.ExecuteSqlRequest_QueryMode) {
	t.Helper()
	entries := replayJournal(tm)
	if !capture {
		if len(entries) != 0 {
			t.Fatalf("capture disabled journaled: %+v", entries)
		}
		return
	}
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if e.kind != replayKindSQL || e.stmt.SQL != sql {
			continue
		}
		if e.stmt.Opts.Mode == nil || *e.stmt.Opts.Mode != want {
			t.Fatalf("frozen %q mode = %v, want %v", sql, e.stmt.Opts.Mode, want)
		}
		return
	}
	t.Fatalf("frozen journal missing %q: %+v", sql, entries)
}

func TestSavepointOwnerAutomaticQueueReservationAccounting(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	h.tm.enableSavepointCaptureForTest()
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}

	h.tm.mu.Lock()
	if err := h.tm.tc.replay.addSavepoint("keep"); err != nil {
		h.tm.mu.Unlock()
		t.Fatal(err)
	}
	h.tm.mu.Unlock()
	assertReplayReservationEqualsPayload(t, h.tm)

	stmt := spanner.NewStatement("UPDATE T SET v=2 WHERE id=1")
	frozen, err := freezeStatement(stmt.SQL, stmt.Params, spanner.QueryOptions{LastStatement: false})
	if err != nil {
		t.Fatal(err)
	}
	full := queuedAccounted([]frozenStatement{frozen})
	marker := savepointMarkerBytes("keep")
	if full <= frozen.payloadBytes() {
		t.Fatal("first queued statement must charge fingerprint and count-vector overhead")
	}
	if got := replayRetainedBytes(h.tm); got != marker {
		t.Fatalf("marker retained=%d, want %d", got, marker)
	}

	ok, err := h.tm.TryEnqueueAutomaticDML(stmt)
	if err != nil || !ok {
		t.Fatalf("first enqueue: ok=%v err=%v", ok, err)
	}
	assertReplayReservationEqualsPayload(t, h.tm)
	if got := replayRetainedBytes(h.tm); got != marker+full {
		t.Fatalf("first enqueue retained=%d, want marker+batch %d", got, marker+full)
	}

	h.tm.DiscardAutomaticDML()
	assertReplayReservationEqualsPayload(t, h.tm)
	if got := replayRetainedBytes(h.tm); got != marker {
		t.Fatalf("discard retained=%d, want marker %d", got, marker)
	}

	for range 3 {
		ok, err := h.tm.TryEnqueueAutomaticDML(stmt)
		if err != nil || !ok {
			t.Fatalf("repeat enqueue: ok=%v err=%v", ok, err)
		}
		assertReplayReservationEqualsPayload(t, h.tm)
		h.tm.DiscardAutomaticDML()
		assertReplayReservationEqualsPayload(t, h.tm)
		if got := replayRetainedBytes(h.tm); got != marker {
			t.Fatalf("repeat discard leaked reservation: %d", got)
		}
	}

	h.tm.mu.Lock()
	h.tm.tc.replay.retainedBytes = savepointJournalLimit - full
	h.tm.mu.Unlock()
	ok, err = h.tm.TryEnqueueAutomaticDML(stmt)
	if err != nil || !ok {
		t.Fatalf("exact-limit first enqueue: ok=%v err=%v", ok, err)
	}
	if got := replayRetainedBytes(h.tm); got != savepointJournalLimit {
		t.Fatalf("exact-limit enqueue retained=%d, want limit", got)
	}
	h.tm.DiscardAutomaticDML()
	if got := replayRetainedBytes(h.tm); got != savepointJournalLimit-full {
		t.Fatalf("exact-limit discard retained=%d, want %d", got, savepointJournalLimit-full)
	}

	h.tm.mu.Lock()
	h.tm.tc.replay.retainedBytes = savepointJournalLimit - full + 1
	h.tm.mu.Unlock()
	ok, err = h.tm.TryEnqueueAutomaticDML(stmt)
	if ok || !errors.Is(err, errSavepointJournalFull) {
		t.Fatalf("over-limit enqueue: ok=%v err=%v", ok, err)
	}
	if got := replayRetainedBytes(h.tm); got != savepointJournalLimit-full+1 {
		t.Fatalf("failed enqueue mutated reservation: %d", got)
	}

	h.tm.mu.Lock()
	h.tm.tc.replay.retainedBytes = marker
	h.tm.tc.replay.queued = nil
	h.tm.mu.Unlock()
	assertReplayReservationEqualsPayload(t, h.tm)

	ok, err = h.tm.TryEnqueueAutomaticDML(stmt)
	if err != nil || !ok {
		t.Fatalf("flush enqueue: ok=%v err=%v", ok, err)
	}
	assertReplayReservationEqualsPayload(t, h.tm)
	h.tm.mu.Lock()
	_, flushErr := h.tm.completeBatchDMLLocked(nil, errors.New("flush failed"))
	h.tm.mu.Unlock()
	if flushErr == nil {
		t.Fatal("failed flush returned nil")
	}
	assertReplayReservationEqualsPayload(t, h.tm)
	if got := replayRetainedBytes(h.tm); got != marker {
		t.Fatalf("failed flush retained=%d, want marker %d", got, marker)
	}
	h.tm.DiscardAutomaticDML()
	assertReplayReservationEqualsPayload(t, h.tm)

	ok, err = h.tm.TryEnqueueAutomaticDML(stmt)
	if err != nil || !ok {
		t.Fatalf("successful flush enqueue: ok=%v err=%v", ok, err)
	}
	assertReplayReservationEqualsPayload(t, h.tm)
	if _, err := h.tm.FlushAutomaticDML(ctx); err != nil {
		t.Fatal(err)
	}
	assertReplayReservationEqualsPayload(t, h.tm)
	entries := replayJournal(h.tm)
	if len(entries) != 1 || entries[0].kind != replayKindBatchDML {
		t.Fatalf("flush journal: %+v", entries)
	}
	if entries[0].payloadBytes != full {
		t.Fatalf("flush payloadBytes=%d, want reserved %d", entries[0].payloadBytes, full)
	}
	if got := replayRetainedBytes(h.tm); got != marker+full {
		t.Fatalf("successful flush retained=%d, want %d", got, marker+full)
	}
	if len(replayQueued(h.tm)) != 0 {
		t.Fatal("queued automatic DML survived flush")
	}
}

func replayRetainedBytes(tm *TransactionManager) int64 {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	if tm.tc == nil || tm.tc.replay == nil {
		return 0
	}
	return tm.tc.replay.retainedBytes
}

func assertReplayReservationEqualsPayload(t *testing.T, tm *TransactionManager) {
	t.Helper()
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	if tm.tc == nil || tm.tc.replay == nil {
		t.Fatal("missing replay state")
	}
	rs := tm.tc.replay
	var payload int64
	for _, e := range rs.entries {
		payload += e.payloadBytes
	}
	for _, sp := range rs.savepoints {
		payload += sp.bytes
	}
	payload += queuedAccounted(rs.queued)
	payload += rs.admittedBytes
	if tm.tc.pending != nil {
		payload += tm.tc.pending.reserved
	}
	if rs.retainedBytes != payload {
		t.Fatalf("retainedBytes=%d, marker/journal/queue payload=%d", rs.retainedBytes, payload)
	}
}
