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
	"io"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
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
