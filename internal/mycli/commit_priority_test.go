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
	"strings"
	"testing"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/enums"
)

func requireCommitPriority(t *testing.T, obs []commitObservation, want sppb.RequestOptions_Priority) {
	t.Helper()
	if len(obs) == 0 {
		t.Fatal("no Commit RPCs observed")
	}
	for i, c := range obs {
		if c.priority != want {
			t.Fatalf("Commit[%d] priority = %v, want %v", i, c.priority, want)
		}
	}
}

func requireUserSQLPriority(t *testing.T, obs []sqlObservation, want sppb.RequestOptions_Priority) {
	t.Helper()
	var saw bool
	for _, o := range obs {
		if o.reqTag == "spanner_mycli_heartbeat" {
			continue
		}
		saw = true
		if o.priority != want {
			t.Fatalf("ExecuteSql %q priority = %v, want %v", o.sql, o.priority, want)
		}
	}
	if !saw {
		t.Fatal("no user ExecuteSql RPCs observed")
	}
}

func TestCommitPriorityRegistryAndNoLocal(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	ctx := t.Context()
	if got := mustGetVar(t, session, "COMMIT_PRIORITY"); got != "UNSPECIFIED" {
		t.Fatalf("default = %q, want UNSPECIFIED", got)
	}
	mustExec(t, ctx, session, "SET COMMIT_PRIORITY = 'HIGH'")
	if got := mustGetVar(t, session, "COMMIT_PRIORITY"); got != "HIGH" {
		t.Fatalf("SET HIGH = %q", got)
	}
	mustExec(t, ctx, session, "SET COMMIT_PRIORITY = 'UNSPECIFIED'")
	if got := mustGetVar(t, session, "COMMIT_PRIORITY"); got != "UNSPECIFIED" {
		t.Fatalf("SET UNSPECIFIED = %q", got)
	}
	mustExec(t, ctx, session, "BEGIN")
	_, err := execSQL(t, ctx, session, "SET LOCAL COMMIT_PRIORITY = 'LOW'")
	if err == nil || !strings.Contains(err.Error(), "does not support SET LOCAL") {
		t.Fatalf("SET LOCAL: %v", err)
	}
	if got := mustGetVar(t, session, "COMMIT_PRIORITY"); got != "UNSPECIFIED" {
		t.Fatalf("rejected SET LOCAL changed value: %s", got)
	}
}

func TestCommitPriorityFakeRPCExplicitRW(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	mustExec(t, ctx, session, "SET RPC_PRIORITY = 'HIGH'")
	mustExec(t, ctx, session, "SET COMMIT_PRIORITY = 'LOW'")
	mustExec(t, ctx, session, "BEGIN RW")
	if _, err := executeDML(ctx, session, "INSERT INTO T (id) VALUES (1)"); err != nil {
		t.Fatal(err)
	}
	mustExec(t, ctx, session, "SET COMMIT_PRIORITY = 'MEDIUM'")
	mustExec(t, ctx, session, "SET RPC_PRIORITY = 'MEDIUM'")
	if replayCtor(h.tm).CommitPriority != sppb.RequestOptions_PRIORITY_LOW {
		t.Fatalf("active ctor CommitPriority = %v", replayCtor(h.tm).CommitPriority)
	}
	mustExec(t, ctx, session, "COMMIT")
	requireUserSQLPriority(t, h.server.sqlObservations(), sppb.RequestOptions_PRIORITY_HIGH)
	requireCommitPriority(t, h.server.commitObservations(), sppb.RequestOptions_PRIORITY_LOW)
}

func TestCommitPriorityFakeRPCImplicitDML(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	mustExec(t, ctx, session, "SET RPC_PRIORITY = 'HIGH'")
	mustExec(t, ctx, session, "SET COMMIT_PRIORITY = 'LOW'")
	if _, err := executeDML(ctx, session, "INSERT INTO T (id) VALUES (1)"); err != nil {
		t.Fatal(err)
	}
	requireUserSQLPriority(t, h.server.sqlObservations(), sppb.RequestOptions_PRIORITY_HIGH)
	requireCommitPriority(t, h.server.commitObservations(), sppb.RequestOptions_PRIORITY_LOW)
}

func TestCommitPriorityFakeRPCPendingActivation(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	mustExec(t, ctx, session, "SET RPC_PRIORITY = 'HIGH'")
	mustExec(t, ctx, session, "SET COMMIT_PRIORITY = 'LOW'")
	mustExec(t, ctx, session, "BEGIN")
	mustExec(t, ctx, session, "SET COMMIT_PRIORITY = 'MEDIUM'")
	if _, err := executeDML(ctx, session, "INSERT INTO T (id) VALUES (1)"); err != nil {
		t.Fatal(err)
	}
	if replayCtor(h.tm).CommitPriority != sppb.RequestOptions_PRIORITY_MEDIUM {
		t.Fatalf("pending activation ctor = %v, want MEDIUM", replayCtor(h.tm).CommitPriority)
	}
	mustExec(t, ctx, session, "SET COMMIT_PRIORITY = 'HIGH'")
	mustExec(t, ctx, session, "COMMIT")
	requireUserSQLPriority(t, h.server.sqlObservations(), sppb.RequestOptions_PRIORITY_HIGH)
	requireCommitPriority(t, h.server.commitObservations(), sppb.RequestOptions_PRIORITY_MEDIUM)
}

func TestCommitPriorityFakeRPCClearRestoresInheritance(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	mustExec(t, ctx, session, "SET RPC_PRIORITY = 'HIGH'")
	mustExec(t, ctx, session, "SET COMMIT_PRIORITY = 'LOW'")
	if _, err := executeDML(ctx, session, "INSERT INTO T (id) VALUES (1)"); err != nil {
		t.Fatal(err)
	}
	requireCommitPriority(t, h.server.commitObservations(), sppb.RequestOptions_PRIORITY_LOW)

	mustExec(t, ctx, session, "SET COMMIT_PRIORITY = 'UNSPECIFIED'")
	if _, err := executeDML(ctx, session, "INSERT INTO T (id) VALUES (2)"); err != nil {
		t.Fatal(err)
	}
	obs := h.server.commitObservations()
	if len(obs) != 2 {
		t.Fatalf("commits = %d, want 2", len(obs))
	}
	if obs[0].priority != sppb.RequestOptions_PRIORITY_LOW {
		t.Fatalf("first commit = %v, want LOW", obs[0].priority)
	}
	if obs[1].priority != sppb.RequestOptions_PRIORITY_HIGH {
		t.Fatalf("cleared inherit commit = %v, want HIGH", obs[1].priority)
	}
	requireUserSQLPriority(t, h.server.sqlObservations(), sppb.RequestOptions_PRIORITY_HIGH)
}

func TestCommitPriorityFakeRPCSavepointReplacement(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	h.tm.enableSavepointCaptureForTest()
	session := sessionForTM(t, h.tm)
	mustExec(t, ctx, session, "SET RPC_PRIORITY = 'HIGH'")
	mustExec(t, ctx, session, "SET COMMIT_PRIORITY = 'LOW'")
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	if _, err := executeDML(ctx, session, "INSERT INTO T (id) VALUES (1)"); err != nil {
		t.Fatal(err)
	}
	if err := h.tm.CreateSavepoint(ctx, "keep"); err != nil {
		t.Fatal(err)
	}
	if _, err := executeDML(ctx, session, "INSERT INTO T (id) VALUES (2)"); err != nil {
		t.Fatal(err)
	}
	beginsBefore := len(h.server.beginObservations())
	mustExec(t, ctx, session, "SET COMMIT_PRIORITY = 'MEDIUM'")
	mustExec(t, ctx, session, "SET RPC_PRIORITY = 'MEDIUM'")
	if err := h.tm.RollbackToSavepoint(ctx, "keep"); err != nil {
		t.Fatal(err)
	}
	if replayCtor(h.tm).CommitPriority != sppb.RequestOptions_PRIORITY_LOW {
		t.Fatalf("reconstructed ctor = %v, want LOW", replayCtor(h.tm).CommitPriority)
	}
	if len(h.server.beginObservations()) <= beginsBefore {
		t.Fatal("ROLLBACK TO did not start a replacement physical attempt")
	}
	mustExec(t, ctx, session, "COMMIT")
	requireUserSQLPriority(t, h.server.sqlObservations(), sppb.RequestOptions_PRIORITY_HIGH)
	requireCommitPriority(t, h.server.commitObservations(), sppb.RequestOptions_PRIORITY_LOW)
}

func TestCommitPriorityHeartbeatStaysLow(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	mustExec(t, ctx, session, "SET RPC_PRIORITY = 'HIGH'")
	mustExec(t, ctx, session, "SET COMMIT_PRIORITY = 'HIGH'")
	beginRWAndProbe(t, ctx, h, "SELECT 1")
	sendTick(t, h.ticks)
	waitChan(t, h.server.heartbeatStarted, "heartbeat")
	assertHeartbeatMeta(t, h.server.heartbeatRecords())
	mustExec(t, ctx, session, "COMMIT")
	requireCommitPriority(t, h.server.commitObservations(), sppb.RequestOptions_PRIORITY_HIGH)
}

func TestCommitPriorityDoesNotAffectReadOnlyOrAdmin(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	h.attachSessionClient(session)
	mustExec(t, ctx, session, "SET RPC_PRIORITY = 'HIGH'")
	mustExec(t, ctx, session, "SET COMMIT_PRIORITY = 'LOW'")

	if _, err := h.tm.BeginReadOnlyTransaction(ctx, timestampBoundUnspecified, 0, time.Time{}, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	if err := h.tm.CloseReadOnlyTransaction(); err != nil {
		t.Fatal(err)
	}
	if len(h.server.commitObservations()) != 0 {
		t.Fatalf("RO issued Commit: %v", h.server.commitObservations())
	}
	requireUserSQLPriority(t, h.server.sqlObservations(), sppb.RequestOptions_PRIORITY_HIGH)

	exists, err := session.DatabaseExists(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("DatabaseExists = false")
	}
	var sawAdmin bool
	for _, o := range h.server.sqlObservations() {
		if o.sql == "SELECT 1" && o.readOnly && o.priority == sppb.RequestOptions_PRIORITY_HIGH {
			sawAdmin = true
		}
		if o.priority == sppb.RequestOptions_PRIORITY_LOW {
			t.Fatalf("RO/admin path used COMMIT_PRIORITY: %+v", o)
		}
	}
	if !sawAdmin {
		t.Fatal("DatabaseExists did not issue HIGH-priority SELECT 1")
	}
}

func TestCommitPriorityExplicitBeginPriorityNotReplacedBySessionRPC(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	mustExec(t, ctx, session, "SET RPC_PRIORITY = 'MEDIUM'")
	mustExec(t, ctx, session, "SET COMMIT_PRIORITY = 'UNSPECIFIED'")
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_LOW); err != nil {
		t.Fatal(err)
	}
	mustExec(t, ctx, session, "SET RPC_PRIORITY = 'HIGH'")
	if _, err := executeDML(ctx, session, "INSERT INTO T (id) VALUES (1)"); err != nil {
		t.Fatal(err)
	}
	if replayCtor(h.tm).CommitPriority != sppb.RequestOptions_PRIORITY_LOW {
		t.Fatalf("inherited commit followed later RPC_PRIORITY: %v", replayCtor(h.tm).CommitPriority)
	}
	mustExec(t, ctx, session, "COMMIT")
	requireUserSQLPriority(t, h.server.sqlObservations(), sppb.RequestOptions_PRIORITY_LOW)
	requireCommitPriority(t, h.server.commitObservations(), sppb.RequestOptions_PRIORITY_LOW)
}

func TestCommitPriorityAutocommitSelectUsesRPCNotCommit(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	mustExec(t, ctx, session, "SET RPC_PRIORITY = 'HIGH'")
	mustExec(t, ctx, session, "SET COMMIT_PRIORITY = 'LOW'")
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	if len(h.server.commitObservations()) != 0 {
		t.Fatalf("autocommit SELECT issued Commit: %v", h.server.commitObservations())
	}
	requireUserSQLPriority(t, h.server.sqlObservations(), sppb.RequestOptions_PRIORITY_HIGH)
}

func TestCommitPriorityDoesNotAffectPDML(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	h.attachSessionClient(session)
	mustExec(t, ctx, session, "SET RPC_PRIORITY = 'HIGH'")
	mustExec(t, ctx, session, "SET COMMIT_PRIORITY = 'LOW'")
	session.systemVariables.Transaction.AutocommitDMLMode = enums.AutocommitDMLModePartitionedNonAtomic
	if _, err := executePDML(ctx, session, "UPDATE T SET id = 1 WHERE TRUE"); err != nil {
		t.Fatal(err)
	}
	if len(h.server.commitObservations()) != 0 {
		t.Fatalf("PDML issued Commit: %v", h.server.commitObservations())
	}
	var saw bool
	for _, o := range h.server.sqlObservations() {
		if !strings.Contains(o.sql, "UPDATE T") {
			continue
		}
		saw = true
		if o.priority == sppb.RequestOptions_PRIORITY_LOW {
			t.Fatal("PDML used COMMIT_PRIORITY")
		}
	}
	if !saw {
		t.Fatal("no PDML ExecuteSql observed")
	}
}
