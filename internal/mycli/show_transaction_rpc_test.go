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
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
)

type showTransactionRPCCounts struct {
	sqls, begins, commits, rollbacks, batches int
}

func snapshotShowTransactionRPCs(s *heartbeatRPCServer) showTransactionRPCCounts {
	s.mu.Lock()
	defer s.mu.Unlock()
	return showTransactionRPCCounts{
		sqls:      len(s.sqlObs),
		begins:    len(s.begins),
		commits:   len(s.commits),
		rollbacks: len(s.rollbacks),
		batches:   len(s.batchObs),
	}
}

func requireNoShowTransactionRPCs(t *testing.T, s *heartbeatRPCServer, before showTransactionRPCCounts) {
	t.Helper()
	got := snapshotShowTransactionRPCs(s)
	if got != before {
		t.Fatalf("SHOW TRANSACTION issued RPCs: before=%+v after=%+v", before, got)
	}
}

func TestShowTransactionActiveRWNoRPC(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	h.attachSessionClient(session)

	mustExec(t, ctx, session, "SET DEFAULT_ISOLATION_LEVEL = 'SERIALIZABLE'")
	mustExec(t, ctx, session, "BEGIN RW ISOLATION LEVEL REPEATABLE READ")
	mustExec(t, ctx, session, "SET DEFAULT_ISOLATION_LEVEL = 'SERIALIZABLE'")
	before := snapshotShowTransactionRPCs(h.server)
	journalBefore := replayJournal(h.tm)
	autoBefore := autoDMLLen(h.tm)
	owner := txnContext(h.tm)

	if got := mustShowTransaction(t, session, "SHOW TRANSACTION ISOLATION LEVEL"); got != "REPEATABLE_READ" {
		t.Fatalf("active RW isolation = %q", got)
	}
	if got := mustShowTransaction(t, session, "SHOW TRANSACTION READ ONLY"); got != "FALSE" {
		t.Fatalf("active RW read only = %q", got)
	}
	requireNoShowTransactionRPCs(t, h.server, before)
	if txnContext(h.tm) != owner {
		t.Fatal("SHOW replaced the RW owner")
	}
	if len(replayJournal(h.tm)) != len(journalBefore) {
		t.Fatalf("SHOW appended journal: before=%+v after=%+v", journalBefore, replayJournal(h.tm))
	}
	if autoDMLLen(h.tm) != autoBefore {
		t.Fatal("SHOW mutated automatic DML")
	}
}

func TestShowTransactionActiveRONoRPC(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	h.attachSessionClient(session)

	mustExec(t, ctx, session, "BEGIN RO")
	before := snapshotShowTransactionRPCs(h.server)
	owner := txnContext(h.tm)
	if got := mustShowTransaction(t, session, "SHOW TRANSACTION READ ONLY"); got != "TRUE" {
		t.Fatalf("active RO read only = %q", got)
	}
	if got := mustShowTransaction(t, session, "SHOW TRANSACTION ISOLATION LEVEL"); got != "UNSPECIFIED" {
		t.Fatalf("active RO isolation = %q, want stored owner value UNSPECIFIED", got)
	}
	requireNoShowTransactionRPCs(t, h.server, before)
	if txnContext(h.tm) != owner {
		t.Fatal("SHOW replaced the RO owner")
	}
	if mode, _ := session.txn.TransactionState(); mode != transactionModeReadOnly {
		t.Fatalf("mode after SHOW = %q", mode)
	}
}

func TestShowTransactionRecoveryRequiredNoRPC(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	h.attachSessionClient(session)
	owner := enterPublicRecovery(t, ctx, h, session)
	if attrs := h.tm.TransactionAttrsWithLock(); attrs.isolationLevel != sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED {
		t.Fatalf("recovery owner isolation = %v", attrs.isolationLevel)
	}

	before := snapshotShowTransactionRPCs(h.server)
	journalBefore := replayJournal(h.tm)
	if got := mustShowTransaction(t, session, "SHOW TRANSACTION ISOLATION LEVEL"); got != "UNSPECIFIED" {
		t.Fatalf("recovery isolation = %q", got)
	}
	if got := mustShowTransaction(t, session, "SHOW TRANSACTION READ ONLY"); got != "FALSE" {
		t.Fatalf("recovery read only = %q", got)
	}
	requireNoShowTransactionRPCs(t, h.server, before)
	if txnContext(h.tm) != owner {
		t.Fatal("SHOW retired the recovery owner")
	}
	if !h.tm.NeedsRecovery() {
		t.Fatal("SHOW cleared recovery-required")
	}
	if len(replayJournal(h.tm)) != len(journalBefore) {
		t.Fatalf("SHOW appended journal during recovery: before=%+v after=%+v", journalBefore, replayJournal(h.tm))
	}
}

func TestShowTransactionPostCommitActiveRW(t *testing.T) {
	t.Parallel()
	h := newHeartbeatHarness(t)
	ctx := t.Context()
	session := sessionForTM(t, h.tm)
	h.attachSessionClient(session)

	mustExec(t, ctx, session, "SET DEFAULT_ISOLATION_LEVEL = 'SERIALIZABLE'")
	mustExec(t, ctx, session, "BEGIN RW ISOLATION LEVEL REPEATABLE READ")
	mustExec(t, ctx, session, "COMMIT")
	if _, active := session.txn.TransactionState(); active {
		t.Fatal("expected idle after COMMIT")
	}
	before := snapshotShowTransactionRPCs(h.server)
	if got := mustShowTransaction(t, session, "SHOW TRANSACTION ISOLATION LEVEL"); got != "SERIALIZABLE" {
		t.Fatalf("post-COMMIT isolation = %q, want next-transaction default", got)
	}
	if got := mustShowTransaction(t, session, "SHOW TRANSACTION READ ONLY"); got != "FALSE" {
		t.Fatalf("post-COMMIT read only = %q", got)
	}
	requireNoShowTransactionRPCs(t, h.server, before)
}
