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
	"slices"
	"strings"
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"

	"github.com/apstndb/spanner-mycli/enums"
)

func TestCLISavepointSupportDefaultDisabled(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	if got := mustGetVar(t, session, "CLI_SAVEPOINT_SUPPORT"); got != "DISABLED" {
		t.Fatalf("default CLI_SAVEPOINT_SUPPORT = %q", got)
	}
	if _, err := execSQL(t, ctx, session, "SET CLI_SAVEPOINT_SUPPORT = 'ENABLED'"); err != nil {
		t.Fatal(err)
	}
	if h.tm.sysVars.Transaction.SavepointSupport != enums.SavepointSupportEnabled {
		t.Fatal("SET CLI_SAVEPOINT_SUPPORT did not enable capture")
	}
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	if _, err := execSQL(t, ctx, session, "SET CLI_SAVEPOINT_SUPPORT = 'DISABLED'"); err == nil || !strings.Contains(err.Error(), "can't change variable when there is an active transaction") {
		t.Fatalf("SET during transaction: %v", err)
	}
}

func TestSavepointPublicStatementsRequireEnabledAndExplicitTxn(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	if _, err := execSQL(t, ctx, session, "SAVEPOINT keep"); !errors.Is(err, errSavepointNotInTransaction) {
		t.Fatalf("idle SAVEPOINT: %v", err)
	}
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	if _, err := execSQL(t, ctx, session, "SAVEPOINT keep"); !errors.Is(err, errSavepointDisabled) {
		t.Fatalf("disabled SAVEPOINT: %v", err)
	}
}

func TestSavepointPublicEnabledJournalAndNestedMarkers(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	mustExec(t, ctx, session, "SET CLI_SAVEPOINT_SUPPORT = 'ENABLED'")
	mustExec(t, ctx, session, "BEGIN RW")
	h.tm.mu.RLock()
	hasJournal := h.tm.tc != nil && h.tm.tc.replay != nil
	h.tm.mu.RUnlock()
	if !hasJournal {
		t.Fatal("ENABLED BEGIN did not attach a journal")
	}
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	mustExec(t, ctx, session, "SAVEPOINT a")
	if _, err := executeDML(ctx, session, "INSERT INTO T (id) VALUES (1)"); err != nil {
		t.Fatal(err)
	}
	mustExec(t, ctx, session, "SAVEPOINT b")
	if _, err := executeDML(ctx, session, "INSERT INTO T (id) VALUES (2)"); err != nil {
		t.Fatal(err)
	}
	before := replayJournal(h.tm)
	if len(before) != 3 {
		t.Fatalf("journal before rollback: %+v", before)
	}
	mustExec(t, ctx, session, "ROLLBACK TO SAVEPOINT a")
	if got := replayMarkerNames(h.tm); !slices.Equal(got, []string{"a"}) {
		t.Fatalf("markers after ROLLBACK TO a: %v", got)
	}
	after := replayJournal(h.tm)
	if len(after) != 1 || after[0].stmt.SQL != "SELECT 1" {
		t.Fatalf("journal after ROLLBACK TO a: %+v", after)
	}
	mustExec(t, ctx, session, "RELEASE a")
	if got := replayMarkerNames(h.tm); len(got) != 0 {
		t.Fatalf("RELEASE left markers: %v", got)
	}
}

func TestSavepointPublicRejectsManualBatch(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	mustExec(t, ctx, session, "SET CLI_SAVEPOINT_SUPPORT = 'ENABLED'")
	mustExec(t, ctx, session, "BEGIN RW")
	if err := session.batch.Start(batchModeDML); err != nil {
		t.Fatal(err)
	}
	if _, err := execSQL(t, ctx, session, "SAVEPOINT keep"); !errors.Is(err, errSavepointInManualBatch) {
		t.Fatalf("SAVEPOINT in batch: %v", err)
	}
	if _, err := execSQL(t, ctx, session, "ROLLBACK TO keep"); !errors.Is(err, errSavepointInManualBatch) {
		t.Fatalf("ROLLBACK TO in batch: %v", err)
	}
	if _, err := execSQL(t, ctx, session, "RELEASE keep"); !errors.Is(err, errSavepointInManualBatch) {
		t.Fatalf("RELEASE in batch: %v", err)
	}
}

func TestSavepointPublicSetLocalAndOutputIsolation(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	mustExec(t, ctx, session, "SET CLI_SAVEPOINT_SUPPORT = 'ENABLED'")
	mustExec(t, ctx, session, "BEGIN RW")
	if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "CLI_VERBOSE", Value: "TRUE"}); err != nil {
		t.Fatal(err)
	}
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	mustExec(t, ctx, session, "SAVEPOINT keep")
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 2", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	cache := session.systemVariables.LastResult.QueryCache
	mustExec(t, ctx, session, "ROLLBACK TO keep")
	if session.systemVariables.LastResult.QueryCache != cache {
		t.Fatal("ROLLBACK TO overwrote LastResult.QueryCache")
	}
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "TRUE" {
		t.Fatalf("SET LOCAL did not survive ROLLBACK TO: %s", got)
	}
}

func TestSavepointPublicPendingMarkers(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	mustExec(t, ctx, session, "SET CLI_SAVEPOINT_SUPPORT = 'ENABLED'")
	mustExec(t, ctx, session, "BEGIN")
	mustExec(t, ctx, session, "SAVEPOINT a")
	mustExec(t, ctx, session, "SAVEPOINT b")
	if got := replayMarkerNames(h.tm); !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("pending markers: %v", got)
	}
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	if got := replayMarkerNames(h.tm); !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("activation dropped pending markers: %v", got)
	}
}
