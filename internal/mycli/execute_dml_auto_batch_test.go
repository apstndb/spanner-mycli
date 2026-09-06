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
	"strconv"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestAutoBatchDMLLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping emulator integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
	defer cancel()

	t.Run("direct-control insert commits", func(t *testing.T) {
		session := newAutoBatchSession(t)
		mustExec(t, ctx, session, "BEGIN")
		res := mustExec(t, ctx, session, "INSERT INTO AuditBatch (Id) VALUES (1)")
		if !res.IsExecutedDML || res.AffectedRows != 1 {
			t.Fatalf("direct insert: executed=%v affected=%d", res.IsExecutedDML, res.AffectedRows)
		}
		mustExec(t, ctx, session, "COMMIT")
		if got := countID(t, ctx, session, 1); got != 1 {
			t.Fatalf("direct-control persisted Id=1: got %d, want 1", got)
		}
	})

	t.Run("rollback does not resurrect queued DML", func(t *testing.T) {
		session := newAutoBatchSession(t)
		mustExec(t, ctx, session, "SET AUTO_BATCH_DML = TRUE")
		mustExec(t, ctx, session, "BEGIN")
		queued := mustExec(t, ctx, session, "INSERT INTO AuditBatch (Id) VALUES (1)")
		if queued.IsExecutedDML || queued.BatchInfo == nil || queued.BatchInfo.Size != 1 {
			t.Fatalf("enqueue: %+v", queued)
		}
		if got := countID(t, ctx, session, 1); got != 1 {
			t.Fatalf("read-your-writes before rollback: got %d, want 1", got)
		}
		mustExec(t, ctx, session, "ROLLBACK")
		if session.txn.HasAutomaticDML() || session.batch.IsActive() {
			t.Fatal("automatic queue survived ROLLBACK")
		}
		mustExec(t, ctx, session, "BEGIN")
		mustExec(t, ctx, session, "INSERT INTO AuditBatch (Id) VALUES (2)")
		mustExec(t, ctx, session, "COMMIT")
		if got := countID(t, ctx, session, 1); got != 0 {
			t.Fatalf("rolled-back write resurrected: got %d rows for Id=1", got)
		}
		if got := countID(t, ctx, session, 2); got != 1 {
			t.Fatalf("new transaction control: got %d rows for Id=2, want 1", got)
		}
	})

	t.Run("disable before COMMIT still flushes", func(t *testing.T) {
		session := newAutoBatchSession(t)
		mustExec(t, ctx, session, "SET AUTO_BATCH_DML = TRUE")
		mustExec(t, ctx, session, "BEGIN")
		mustExec(t, ctx, session, "INSERT INTO AuditBatch (Id) VALUES (1)")
		mustExec(t, ctx, session, "SET AUTO_BATCH_DML = FALSE")
		if !session.txn.HasAutomaticDML() {
			t.Fatal("SET FALSE flushed or discarded the queue")
		}
		mustExec(t, ctx, session, "COMMIT")
		if session.txn.HasAutomaticDML() {
			t.Fatal("automatic queue remained after COMMIT")
		}
		if got := countID(t, ctx, session, 1); got != 1 {
			t.Fatalf("COMMIT with option disabled lost queued write: got %d, want 1", got)
		}
	})

	t.Run("disable then dependent UPDATE observes queued insert", func(t *testing.T) {
		session := newAutoBatchSession(t)
		mustExec(t, ctx, session, "SET AUTO_BATCH_DML = TRUE")
		mustExec(t, ctx, session, "BEGIN")
		mustExec(t, ctx, session, "INSERT INTO AuditBatch (Id) VALUES (1)")
		mustExec(t, ctx, session, "SET AUTO_BATCH_DML = FALSE")
		upd := mustExec(t, ctx, session, "UPDATE AuditBatch SET Flag = TRUE WHERE Id = 1")
		if !upd.IsExecutedDML || upd.AffectedRows != 1 {
			t.Fatalf("dependent UPDATE: executed=%v affected=%d", upd.IsExecutedDML, upd.AffectedRows)
		}
		mustExec(t, ctx, session, "ROLLBACK")
		if got := countID(t, ctx, session, 1); got != 0 {
			t.Fatalf("rollback after flush+UPDATE persisted rows: got %d", got)
		}
	})

	t.Run("THEN RETURN flushes then returns rows", func(t *testing.T) {
		session := newAutoBatchSession(t)
		mustExec(t, ctx, session, "SET AUTO_BATCH_DML = TRUE")
		mustExec(t, ctx, session, "BEGIN")
		mustExec(t, ctx, session, "INSERT INTO AuditBatch (Id) VALUES (1)")
		res := mustExec(t, ctx, session, "INSERT INTO AuditBatch (Id) VALUES (2) THEN RETURN Id")
		if !res.IsExecutedDML || res.AffectedRows != 1 || res.TableHeader == nil {
			t.Fatalf("THEN RETURN: executed=%v affected=%d header=%v batch=%+v",
				res.IsExecutedDML, res.AffectedRows, res.TableHeader, res.BatchInfo)
		}
		if !strings.Contains(string(res.RenderedOutput), "2") {
			t.Fatalf("THEN RETURN rendered output %q does not contain returned Id 2", res.RenderedOutput)
		}
		if session.txn.HasAutomaticDML() {
			t.Fatal("THEN RETURN left automatic work queued")
		}
		mustExec(t, ctx, session, "ROLLBACK")
		if got := countID(t, ctx, session, 2); got != 0 {
			t.Fatalf("THEN RETURN rollback control: got %d persisted rows", got)
		}
	})

	t.Run("PLAN leaves queue, SELECT flushes", func(t *testing.T) {
		session := newAutoBatchSession(t)
		mustExec(t, ctx, session, "SET AUTO_BATCH_DML = TRUE")
		mustExec(t, ctx, session, "BEGIN")
		mustExec(t, ctx, session, "INSERT INTO AuditBatch (Id) VALUES (1)")
		_, err := execSQL(t, ctx, session, "EXPLAIN INSERT INTO AuditBatch (Id) VALUES (2)")
		if err != nil && !strings.Contains(err.Error(), "EXPLAIN") && !strings.Contains(err.Error(), "query plan") {
			t.Fatalf("EXPLAIN: %v", err)
		}
		if !session.txn.HasAutomaticDML() {
			t.Fatal("EXPLAIN/PLAN flushed the automatic queue")
		}
		if got := countID(t, ctx, session, 1); got != 1 {
			t.Fatalf("SELECT after PLAN should flush queued insert: got %d", got)
		}
		mustExec(t, ctx, session, "ROLLBACK")
	})

	t.Run("EXPLAIN ANALYZE DML UPDATE depends on queued insert", func(t *testing.T) {
		session := newAutoBatchSession(t)
		mustExec(t, ctx, session, "SET AUTO_BATCH_DML = TRUE")
		mustExec(t, ctx, session, "BEGIN")
		mustExec(t, ctx, session, "INSERT INTO AuditBatch (Id) VALUES (1)")
		_, err := execSQL(t, ctx, session, "EXPLAIN ANALYZE UPDATE AuditBatch SET Flag = TRUE WHERE Id = 1")
		if err != nil && !errors.Is(err, errExplainAnalyzeUnsupportedOnEmulator) {
			t.Fatalf("EXPLAIN ANALYZE DML: %v", err)
		}
		if session.txn.HasAutomaticDML() {
			t.Fatal("EXPLAIN ANALYZE DML left automatic work queued")
		}
		if !flagTrue(t, ctx, session, 1) {
			t.Fatal("PROFILE DML UPDATE did not observe the queued insert")
		}
		mustExec(t, ctx, session, "ROLLBACK")
	})

	t.Run("PROFILE SELECT flushes queued DML", func(t *testing.T) {
		session := newAutoBatchSession(t)
		mustExec(t, ctx, session, "SET AUTO_BATCH_DML = TRUE")
		mustExec(t, ctx, session, "BEGIN")
		mustExec(t, ctx, session, "INSERT INTO AuditBatch (Id) VALUES (1)")
		_, err := execSQL(t, ctx, session, "EXPLAIN ANALYZE SELECT Id FROM AuditBatch WHERE Id = 1")
		if err != nil && !strings.Contains(err.Error(), "query plan") && !strings.Contains(err.Error(), "EXPLAIN ANALYZE") {
			t.Fatalf("EXPLAIN ANALYZE SELECT: %v", err)
		}
		if session.txn.HasAutomaticDML() {
			t.Fatal("PROFILE/EXPLAIN ANALYZE SELECT left automatic work queued")
		}
		if got := countID(t, ctx, session, 1); got != 1 {
			t.Fatalf("queued insert was not flushed before PROFILE SELECT: got %d", got)
		}
		mustExec(t, ctx, session, "ROLLBACK")
	})

	t.Run("parameter snapshot at enqueue", func(t *testing.T) {
		session := newAutoBatchSession(t)
		mustExec(t, ctx, session, "SET AUTO_BATCH_DML = TRUE")
		mustExec(t, ctx, session, "SET PARAM n = 1")
		mustExec(t, ctx, session, "BEGIN")
		mustExec(t, ctx, session, "INSERT INTO AuditBatch (Id) VALUES (@n)")
		mustExec(t, ctx, session, "SET PARAM n = 2")
		mustExec(t, ctx, session, "COMMIT")
		if got := countID(t, ctx, session, 1); got != 1 {
			t.Fatalf("captured parameter not used: Id=1 got %d", got)
		}
		if got := countID(t, ctx, session, 2); got != 0 {
			t.Fatalf("later parameter leaked into queued DML: Id=2 got %d", got)
		}
	})

	t.Run("SET LOCAL restore does not RPC and does not flush", func(t *testing.T) {
		session := newAutoBatchSession(t)
		mustExec(t, ctx, session, "BEGIN")
		mustExec(t, ctx, session, "SET LOCAL AUTO_BATCH_DML = TRUE")
		if got := mustGetVar(t, session, "AUTO_BATCH_DML"); got != "TRUE" {
			t.Fatalf("SET LOCAL AUTO_BATCH_DML: got %q", got)
		}
		mustExec(t, ctx, session, "INSERT INTO AuditBatch (Id) VALUES (1)")
		if !session.txn.HasAutomaticDML() {
			t.Fatal("SET LOCAL TRUE did not enqueue")
		}
		mustExec(t, ctx, session, "ROLLBACK")
		if got := mustGetVar(t, session, "AUTO_BATCH_DML"); got != "FALSE" {
			t.Fatalf("SET LOCAL restore: got %q, want FALSE", got)
		}
		if session.txn.HasAutomaticDML() {
			t.Fatal("ROLLBACK left automatic work")
		}
		if got := countID(t, ctx, session, 1); got != 0 {
			t.Fatalf("SET LOCAL enqueue persisted after ROLLBACK: got %d", got)
		}
	})

	t.Run("failed flush is not replayable", func(t *testing.T) {
		session := newAutoBatchSession(t)
		mustExec(t, ctx, session, "SET AUTO_BATCH_DML = TRUE")
		mustExec(t, ctx, session, "BEGIN")
		mustExec(t, ctx, session, "INSERT INTO MissingTable (Id) VALUES (1)")
		_, err := execSQL(t, ctx, session, "COMMIT")
		if err == nil {
			t.Fatal("COMMIT of missing-table automatic DML succeeded")
		}
		if session.txn.HasAutomaticDML() {
			t.Fatal("failed flush left replayable automatic work")
		}
		mustExec(t, ctx, session, "BEGIN")
		mustExec(t, ctx, session, "COMMIT")
		if got := countID(t, ctx, session, 1); got != 0 {
			t.Fatalf("failed flush replayed into later transaction: got %d", got)
		}
	})

	t.Run("session close discards automatic work", func(t *testing.T) {
		session := newAutoBatchSession(t)
		mustExec(t, ctx, session, "SET AUTO_BATCH_DML = TRUE")
		mustExec(t, ctx, session, "BEGIN")
		mustExec(t, ctx, session, "INSERT INTO AuditBatch (Id) VALUES (1)")
		session.Close()
		if session.txn.HasAutomaticDML() {
			t.Fatal("Close left automatic work")
		}
		// initializeWithRandomDB also registers Close; make the second call a no-op.
		session.client = nil
		session.adminClient = nil
	})

	t.Run("direct manager rollback discards automatic work", func(t *testing.T) {
		session := newAutoBatchSession(t)
		mustExec(t, ctx, session, "SET AUTO_BATCH_DML = TRUE")
		mustExec(t, ctx, session, "BEGIN")
		mustExec(t, ctx, session, "INSERT INTO AuditBatch (Id) VALUES (1)")
		if err := session.txn.RollbackReadWriteTransaction(ctx); err != nil {
			t.Fatalf("RollbackReadWriteTransaction: %v", err)
		}
		if session.txn.HasAutomaticDML() {
			t.Fatal("direct rollback left automatic work")
		}
		mustExec(t, ctx, session, "BEGIN")
		mustExec(t, ctx, session, "INSERT INTO AuditBatch (Id) VALUES (2)")
		mustExec(t, ctx, session, "COMMIT")
		if got := countID(t, ctx, session, 1); got != 0 {
			t.Fatalf("direct-rollback write resurrected: got %d", got)
		}
	})

	t.Run("manual START conflicts with automatic queue", func(t *testing.T) {
		session := newAutoBatchSession(t)
		mustExec(t, ctx, session, "SET AUTO_BATCH_DML = TRUE")
		mustExec(t, ctx, session, "BEGIN")
		mustExec(t, ctx, session, "INSERT INTO AuditBatch (Id) VALUES (1)")
		_, err := execSQL(t, ctx, session, "START BATCH DML")
		if err == nil || !strings.Contains(err.Error(), "already in batch") {
			t.Fatalf("START BATCH with automatic queue: %v", err)
		}
		mustExec(t, ctx, session, "ABORT BATCH")
		if session.txn.HasAutomaticDML() || session.batch.IsActive() {
			t.Fatal("ABORT BATCH did not discard automatic work")
		}
		mustExec(t, ctx, session, "ROLLBACK")
	})

	t.Run("RUN BATCH flushes automatic queue", func(t *testing.T) {
		session := newAutoBatchSession(t)
		mustExec(t, ctx, session, "SET AUTO_BATCH_DML = TRUE")
		mustExec(t, ctx, session, "BEGIN")
		mustExec(t, ctx, session, "INSERT INTO AuditBatch (Id) VALUES (1)")
		res := mustExec(t, ctx, session, "RUN BATCH")
		if !res.IsExecutedDML || res.AffectedRows != 1 {
			t.Fatalf("RUN BATCH automatic: %+v", res)
		}
		if session.txn.HasAutomaticDML() {
			t.Fatal("RUN BATCH left automatic work")
		}
		if got := countID(t, ctx, session, 1); got != 1 {
			t.Fatalf("RUN BATCH did not execute automatic DML: got %d", got)
		}
		mustExec(t, ctx, session, "ROLLBACK")
	})

	t.Run("COMMIT does not run leftover manual batch", func(t *testing.T) {
		session := newAutoBatchSession(t)
		mustExec(t, ctx, session, "SET AUTO_BATCH_DML = TRUE")
		mustExec(t, ctx, session, "BEGIN")
		mustExec(t, ctx, session, "START BATCH DML")
		mustExec(t, ctx, session, "INSERT INTO AuditBatch (Id) VALUES (1)")
		mustExec(t, ctx, session, "COMMIT")
		if !session.batch.IsActive() {
			t.Fatal("COMMIT consumed the manual batch")
		}
		if got := countID(t, ctx, session, 1); got != 0 {
			t.Fatalf("COMMIT executed leftover manual batch: got %d", got)
		}
		mustExec(t, ctx, session, "ABORT BATCH")
	})

	t.Run("DDL rejected while automatic DML is pending", func(t *testing.T) {
		session := newAutoBatchSession(t)
		mustExec(t, ctx, session, "SET AUTO_BATCH_DML = TRUE")
		mustExec(t, ctx, session, "BEGIN")
		mustExec(t, ctx, session, "INSERT INTO AuditBatch (Id) VALUES (1)")
		_, err := execSQL(t, ctx, session, "CREATE TABLE Extra (Id INT64) PRIMARY KEY (Id)")
		if err == nil || !strings.Contains(err.Error(), "active batch DML") {
			t.Fatalf("DDL with automatic queue: %v", err)
		}
		if !session.txn.HasAutomaticDML() {
			t.Fatal("DDL guard consumed the automatic queue")
		}
		mustExec(t, ctx, session, "ROLLBACK")
	})

	t.Run("manual THEN RETURN is rejected not silently batched", func(t *testing.T) {
		session := newAutoBatchSession(t)
		mustExec(t, ctx, session, "START BATCH DML")
		_, err := execSQL(t, ctx, session, "INSERT INTO AuditBatch (Id) VALUES (1) THEN RETURN Id")
		if !errors.Is(err, errReturningDMLNotSupportedInBatch) {
			t.Fatalf("manual returning DML: %v", err)
		}
		mustExec(t, ctx, session, "ABORT BATCH")
	})

	t.Run("enqueue enables heartbeat while queue remains pending", func(t *testing.T) {
		session := newAutoBatchSession(t)
		mustExec(t, ctx, session, "SET AUTO_BATCH_DML = TRUE")
		mustExec(t, ctx, session, "BEGIN")
		mustExec(t, ctx, session, "INSERT INTO AuditBatch (Id) VALUES (1)")
		if !session.txn.HasAutomaticDML() {
			t.Fatal("expected pending automatic DML")
		}
		if !session.txn.HeartbeatEnabled() {
			t.Fatal("enqueue did not enable the RW heartbeat")
		}
		mustExec(t, ctx, session, "ROLLBACK")
		if session.txn.HeartbeatEnabled() {
			t.Fatal("heartbeat remained after rollback")
		}
	})

	t.Run("DESCRIBE leaves automatic queue", func(t *testing.T) {
		session := newAutoBatchSession(t)
		mustExec(t, ctx, session, "SET AUTO_BATCH_DML = TRUE")
		mustExec(t, ctx, session, "BEGIN")
		mustExec(t, ctx, session, "INSERT INTO AuditBatch (Id) VALUES (1)")
		if _, err := execSQL(t, ctx, session, "DESCRIBE SELECT Id FROM AuditBatch"); err != nil {
			t.Fatalf("DESCRIBE: %v", err)
		}
		if !session.txn.HasAutomaticDML() {
			t.Fatal("DESCRIBE flushed the automatic queue")
		}
		mustExec(t, ctx, session, "ROLLBACK")
	})

	t.Run("direct manager commit flushes nonempty automatic queue", func(t *testing.T) {
		session := newAutoBatchSession(t)
		mustExec(t, ctx, session, "SET AUTO_BATCH_DML = TRUE")
		mustExec(t, ctx, session, "BEGIN")
		mustExec(t, ctx, session, "INSERT INTO AuditBatch (Id) VALUES (1)")
		if !session.txn.HasAutomaticDML() {
			t.Fatal("need nonempty automatic queue before direct commit")
		}
		if _, err := session.txn.CommitReadWriteTransaction(ctx); err != nil {
			t.Fatalf("CommitReadWriteTransaction: %v", err)
		}
		if session.txn.HasAutomaticDML() || session.txn.InReadWriteTransaction() {
			t.Fatal("direct commit left queue or RW context")
		}
		if got := countID(t, ctx, session, 1); got != 1 {
			t.Fatalf("direct commit did not persist flushed insert: got %d", got)
		}
	})

	t.Run("commit RPC failure after flush is not replayable", func(t *testing.T) {
		session := newAutoBatchSession(t)
		mustExec(t, ctx, session, "SET AUTO_BATCH_DML = TRUE")
		mustExec(t, ctx, session, "BEGIN")
		mustExec(t, ctx, session, "INSERT INTO AuditBatch (Id) VALUES (1)")
		if !session.txn.HasAutomaticDML() {
			t.Fatal("need nonempty automatic queue before failing commit")
		}
		var hookCalled bool
		session.txn.commitAfterFlushHook = func() error {
			hookCalled = true
			return errors.New("injected commit failure")
		}
		t.Cleanup(func() { session.txn.commitAfterFlushHook = nil })
		_, err := session.txn.CommitReadWriteTransaction(ctx)
		if err == nil || !strings.Contains(err.Error(), "injected commit failure") {
			t.Fatalf("direct commit failure: %v", err)
		}
		if !hookCalled {
			t.Fatal("commit hook did not run; Locked flush path was not reached")
		}
		if session.txn.HasAutomaticDML() || session.txn.InReadWriteTransaction() {
			t.Fatal("failed commit left queue or RW context")
		}
		mustExec(t, ctx, session, "BEGIN")
		mustExec(t, ctx, session, "COMMIT")
		if got := countID(t, ctx, session, 1); got != 0 {
			t.Fatalf("flushed insert survived failed commit into a later transaction: got %d", got)
		}
	})

	t.Run("ordinary query abort after flush clears owner", func(t *testing.T) {
		assertQueryAbortClearsOwner(t, ctx, "SELECT Id FROM AuditBatch WHERE Id = 1")
	})

	t.Run("PROFILE query abort after flush clears owner", func(t *testing.T) {
		assertQueryAbortClearsOwner(t, ctx, "EXPLAIN ANALYZE SELECT Id FROM AuditBatch WHERE Id = 1")
	})

	t.Run("implicit DML stays immediate with option true", func(t *testing.T) {
		session := newAutoBatchSession(t)
		mustExec(t, ctx, session, "SET AUTO_BATCH_DML = TRUE")
		res := mustExec(t, ctx, session, "INSERT INTO AuditBatch (Id) VALUES (1)")
		if !res.IsExecutedDML || res.AffectedRows != 1 {
			t.Fatalf("implicit insert: executed=%v affected=%d", res.IsExecutedDML, res.AffectedRows)
		}
		if session.txn.HasAutomaticDML() {
			t.Fatal("implicit DML was queued")
		}
		if got := countID(t, ctx, session, 1); got != 1 {
			t.Fatalf("implicit insert not persisted: got %d", got)
		}
	})
}

func TestSetLocalAutoBatchDMLRestoreWithoutClient(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	ctx := t.Context()

	if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
		t.Fatalf("BEGIN: %v", err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "AUTO_BATCH_DML", Value: "TRUE"}); err != nil {
		t.Fatalf("SET LOCAL: %v", err)
	}
	if got := mustGetVar(t, session, "AUTO_BATCH_DML"); got != "TRUE" {
		t.Fatalf("in-transaction value: got %q", got)
	}
	if _, err := session.ExecuteStatement(ctx, &RollbackStatement{}); err != nil {
		t.Fatalf("ROLLBACK: %v", err)
	}
	if got := mustGetVar(t, session, "AUTO_BATCH_DML"); got != "FALSE" {
		t.Fatalf("restored value: got %q, want FALSE", got)
	}
}

func TestEnqueueAutomaticDMLEnablesHeartbeat(t *testing.T) {
	t.Parallel()
	sysVars := newSystemVariablesWithDefaultsForTest()
	tm := NewTransactionManager(nil, sysVars, spanner.ClientConfig{})
	started := make(chan struct{})
	tm.tc = &transactionContext{
		attrs: transactionAttributes{mode: transactionModeReadWrite},
		heartbeatFunc: func(ctx context.Context) {
			close(started)
			<-ctx.Done()
		},
	}
	tm.autoDMLGeneration = 1
	ok, err := tm.TryEnqueueAutomaticDML(spanner.NewStatement("INSERT INTO t (id) VALUES (1)"))
	if err != nil || !ok {
		t.Fatalf("TryEnqueueAutomaticDML: ok=%v err=%v", ok, err)
	}
	if !tm.HeartbeatEnabled() {
		t.Fatal("enqueue did not mark heartbeat enabled")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("heartbeat goroutine did not start")
	}
	tm.tc.Close()
	if !tm.HasAutomaticDML() {
		t.Fatal("heartbeat enablement flushed or discarded the queue")
	}
}

func TestAutomaticDMLDiscardedOnClear(t *testing.T) {
	t.Parallel()
	sysVars := newSystemVariablesWithDefaultsForTest()
	tm := NewTransactionManager(nil, sysVars, spanner.ClientConfig{})
	tm.autoDML = []spanner.Statement{{SQL: "INSERT INTO t (id) VALUES (1)"}}
	tm.autoDMLOwner = 1
	tm.clearTransactionContext()
	if tm.HasAutomaticDML() {
		t.Fatal("clearTransactionContext left automatic DML")
	}
}

func newAutoBatchSession(t *testing.T) *Session {
	t.Helper()
	_, session := initializeWithRandomDB(t, []string{
		"CREATE TABLE AuditBatch (Id INT64 NOT NULL, Flag BOOL) PRIMARY KEY (Id)",
	}, nil)
	return session
}

func execSQL(t *testing.T, ctx context.Context, session *Session, sql string) (*Result, error) {
	t.Helper()
	stmt, err := BuildStatement(sql)
	if err != nil {
		t.Fatalf("BuildStatement(%q): %v", sql, err)
	}
	return session.ExecuteStatement(ctx, stmt)
}

func mustExec(t *testing.T, ctx context.Context, session *Session, sql string) *Result {
	t.Helper()
	res, err := execSQL(t, ctx, session, sql)
	if err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return res
}

func countID(t *testing.T, ctx context.Context, session *Session, id int) int {
	t.Helper()
	res := mustExec(t, ctx, session, "SELECT Id FROM AuditBatch WHERE Id = "+strconv.Itoa(id))
	return res.AffectedRows
}

func flagTrue(t *testing.T, ctx context.Context, session *Session, id int) bool {
	t.Helper()
	res := mustExec(t, ctx, session, "SELECT Id FROM AuditBatch WHERE Id = "+strconv.Itoa(id)+" AND Flag = TRUE")
	return res.AffectedRows == 1
}

func assertQueryAbortClearsOwner(t *testing.T, ctx context.Context, sql string) {
	t.Helper()
	session := newAutoBatchSession(t)
	mustExec(t, ctx, session, "SET AUTO_BATCH_DML = TRUE")
	mustExec(t, ctx, session, "BEGIN")
	mustExec(t, ctx, session, "INSERT INTO AuditBatch (Id) VALUES (1)")
	session.txn.queryAfterFlushHook = func() error {
		return status.Error(codes.Aborted, "injected after flush")
	}
	_, err := execSQL(t, ctx, session, sql)
	session.txn.queryAfterFlushHook = nil
	if spanner.ErrCode(err) != codes.Aborted {
		t.Fatalf("%s abort: %v", sql, err)
	}
	if session.txn.HasAutomaticDML() || session.txn.InReadWriteTransaction() {
		t.Fatalf("%s abort left queue or RW context", sql)
	}
	if err := session.RecreateClient(ctx); err != nil {
		t.Fatalf("RecreateClient after %s abort: %v", sql, err)
	}
	mustExec(t, ctx, session, "BEGIN")
	mustExec(t, ctx, session, "INSERT INTO AuditBatch (Id) VALUES (2)")
	mustExec(t, ctx, session, "COMMIT")
	if got := countID(t, ctx, session, 1); got != 0 {
		t.Fatalf("%s abort replayed old write: got %d", sql, got)
	}
	if got := countID(t, ctx, session, 2); got != 1 {
		t.Fatalf("%s abort blocked a later transaction: Id=2 got %d", sql, got)
	}
}
