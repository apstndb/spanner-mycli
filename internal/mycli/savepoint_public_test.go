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
	"slices"
	"strings"
	"testing"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

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

func enterPublicRecovery(t *testing.T, ctx context.Context, h *heartbeatHarness, session *Session) *transactionContext {
	t.Helper()
	mustExec(t, ctx, session, "SET CLI_SAVEPOINT_SUPPORT = 'ENABLED'")
	mustExec(t, ctx, session, "BEGIN RW")
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	mustExec(t, ctx, session, "SAVEPOINT keep")
	owner := txnContext(h.tm)
	h.server.setFailSQL(status.Error(codes.PermissionDenied, "injected public recovery"))
	_, err := execSQL(t, ctx, session, "SELECT 2")
	if err == nil || spanner.ErrCode(err) != codes.PermissionDenied {
		t.Fatalf("injected SELECT: %v", err)
	}
	h.server.setFailSQL(nil)
	if txnContext(h.tm) != owner {
		t.Fatal("recovery retired the logical owner")
	}
	if !h.tm.NeedsRecovery() {
		t.Fatal("expected recovery-required")
	}
	return owner
}

func TestSavepointPublicRecoveryAllowlistPreservesState(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	owner := enterPublicRecovery(t, ctx, h, session)
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "FALSE" {
		t.Fatalf("CLI_VERBOSE before = %q", got)
	}

	for _, sql := range []string{
		"SET CLI_VERBOSE = TRUE",
		"SET LOCAL CLI_VERBOSE = TRUE",
		"START BATCH DML",
		"START BATCH DDL",
		"ABORT BATCH",
		"SET PARAM p = 1",
		"SELECT 3",
		"SAVEPOINT after_error",
		"RELEASE keep",
		"COMMIT",
	} {
		if _, err := execSQL(t, ctx, session, sql); !errors.Is(err, errSavepointRecovery) {
			t.Fatalf("%s during recovery: %v", sql, err)
		}
		if txnContext(h.tm) != owner {
			t.Fatalf("%s retired the logical owner", sql)
		}
		if !h.tm.NeedsRecovery() {
			t.Fatalf("%s cleared recovery-required", sql)
		}
	}
	if got := mustGetVar(t, session, "CLI_VERBOSE"); got != "FALSE" {
		t.Fatalf("rejected SET changed CLI_VERBOSE: %s", got)
	}
	if session.batch.IsActive() {
		t.Fatal("rejected START BATCH left a manual batch")
	}
	if len(session.systemVariables.Params) != 0 {
		t.Fatalf("rejected SET PARAM mutated params: %v", session.systemVariables.Params)
	}

	if _, err := execSQL(t, ctx, session, "HELP"); err != nil {
		t.Fatalf("HELP during recovery: %v", err)
	}
	if _, err := execSQL(t, ctx, session, "SHOW VARIABLE CLI_VERBOSE"); err != nil {
		t.Fatalf("SHOW VARIABLE during recovery: %v", err)
	}
	if _, err := execSQL(t, ctx, session, "SHOW VARIABLES"); err != nil {
		t.Fatalf("SHOW VARIABLES during recovery: %v", err)
	}
	if _, err := execSQL(t, ctx, session, "HELP VARIABLES"); err != nil {
		t.Fatalf("HELP VARIABLES during recovery: %v", err)
	}
	if _, err := execSQL(t, ctx, session, "SHOW PARAMS"); err != nil {
		t.Fatalf("SHOW PARAMS during recovery: %v", err)
	}

	mustExec(t, ctx, session, "ROLLBACK TO SAVEPOINT keep")
	if h.tm.NeedsRecovery() {
		t.Fatal("ROLLBACK TO did not clear recovery-required")
	}
	if txnContext(h.tm) != owner {
		t.Fatal("ROLLBACK TO replaced the logical owner")
	}
}

func TestSavepointPublicSetRejectedInManualBatch(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	for _, mode := range []struct {
		name string
		sql  string
	}{
		{name: "dml", sql: "START BATCH DML"},
		{name: "ddl", sql: "START BATCH DDL"},
	} {
		t.Run(mode.name+"_enable", func(t *testing.T) {
			t.Parallel()
			h := newHeartbeatHarness(t)
			session := sessionForTM(t, h.tm)
			if got := mustGetVar(t, session, "CLI_SAVEPOINT_SUPPORT"); got != "DISABLED" {
				t.Fatalf("default = %q", got)
			}
			mustExec(t, ctx, session, mode.sql)
			if _, err := execSQL(t, ctx, session, "SET CLI_SAVEPOINT_SUPPORT = 'ENABLED'"); !errors.Is(err, errSetterInManualBatch) {
				t.Fatalf("SET ENABLED in %s: %v", mode.name, err)
			}
			if got := mustGetVar(t, session, "CLI_SAVEPOINT_SUPPORT"); got != "DISABLED" {
				t.Fatalf("rejected SET changed setting: %s", got)
			}
			if !session.batch.IsActive() {
				t.Fatal("rejected SET aborted the batch")
			}
		})
		t.Run(mode.name+"_disable", func(t *testing.T) {
			t.Parallel()
			h := newHeartbeatHarness(t)
			session := sessionForTM(t, h.tm)
			mustExec(t, ctx, session, "SET CLI_SAVEPOINT_SUPPORT = 'ENABLED'")
			mustExec(t, ctx, session, mode.sql)
			if _, err := execSQL(t, ctx, session, "SET CLI_SAVEPOINT_SUPPORT = 'DISABLED'"); !errors.Is(err, errSetterInManualBatch) {
				t.Fatalf("SET DISABLED in %s: %v", mode.name, err)
			}
			if got := mustGetVar(t, session, "CLI_SAVEPOINT_SUPPORT"); got != "ENABLED" {
				t.Fatalf("rejected SET changed setting: %s", got)
			}
			if !session.batch.IsActive() {
				t.Fatal("rejected SET aborted the batch")
			}
		})
	}
}

func TestSavepointPublicBufferedOutputFailureEntersRecovery(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	session.systemVariables.Display.CLIFormat = enums.DisplayModeTable
	session.systemVariables.Query.StreamingMode = enums.StreamingModeFalse
	session.systemVariables.Display.MarkdownCodeblock = true
	cli := &Cli{SessionHandler: NewSessionHandler(session), SystemVariables: session.systemVariables}

	mustExec(t, ctx, session, "SET CLI_SAVEPOINT_SUPPORT = 'ENABLED'")
	mustExec(t, ctx, session, "BEGIN RW")
	mustExec(t, ctx, session, "SAVEPOINT keep")
	owner := txnContext(h.tm)
	prefix := replayJournal(h.tm)

	stmt, err := BuildStatement("SELECT 1")
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("opening markdown fence failed")
	w := &resultFailureWriter{err: cause}
	_, err = cli.executeStatement(ctx, stmt, false, "SELECT 1", w)
	if !errors.Is(err, cause) {
		t.Fatalf("buffered display error = %v", err)
	}
	if txnContext(h.tm) != owner {
		t.Fatal("buffered output failure retired the logical owner")
	}
	if !h.tm.NeedsRecovery() {
		t.Fatal("buffered output failure did not enter recovery-required")
	}
	after := replayJournal(h.tm)
	if len(after) != len(prefix) {
		t.Fatalf("failed buffered query remained journaled: %+v", after)
	}
	if _, err := execSQL(t, ctx, session, "SAVEPOINT later"); !errors.Is(err, errSavepointRecovery) {
		t.Fatalf("SAVEPOINT after buffered failure: %v", err)
	}
	if _, err := execSQL(t, ctx, session, "COMMIT"); !errors.Is(err, errSavepointRecovery) {
		t.Fatalf("COMMIT after buffered failure: %v", err)
	}

	mustExec(t, ctx, session, "ROLLBACK TO SAVEPOINT keep")
	if h.tm.NeedsRecovery() {
		t.Fatal("ROLLBACK TO did not clear recovery-required")
	}

	var ok bytes.Buffer
	if _, err := cli.executeStatement(ctx, stmt, false, "SELECT 1", &ok); err != nil {
		t.Fatalf("successful buffered output: %v", err)
	}
	if h.tm.NeedsRecovery() {
		t.Fatal("successful buffered output entered recovery-required")
	}
	if !strings.Contains(ok.String(), "```sql") {
		t.Fatalf("successful buffered output missing fence: %q", ok.String())
	}
	journal := replayJournal(h.tm)
	if len(journal) != len(prefix)+1 {
		t.Fatalf("successful buffered query journal: %+v", journal)
	}
}

func TestCli_executeStatement_abortedSkipsRecreateWhenSavepointRecoverable(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	h.attachSessionClient(session)
	cli := &Cli{SessionHandler: NewSessionHandler(session), SystemVariables: session.systemVariables}

	mustExec(t, ctx, session, "SET CLI_SAVEPOINT_SUPPORT = 'ENABLED'")
	mustExec(t, ctx, session, "BEGIN RW")
	mustExec(t, ctx, session, "SAVEPOINT keep")
	owner := txnContext(h.tm)

	stmt, err := BuildStatement("SELECT 1")
	if err != nil {
		t.Fatal(err)
	}
	h.server.setFailSQL(status.Error(codes.Aborted, "injected abort"))
	_, err = cli.executeStatement(ctx, stmt, false, "SELECT 1", io.Discard)
	if err == nil || spanner.ErrCode(err) != codes.Aborted {
		t.Fatalf("aborted SELECT: %v", err)
	}
	if strings.Contains(err.Error(), "cannot replace client") {
		t.Fatalf("RecreateClient extra error: %v", err)
	}
	if strings.Contains(err.Error(), "database operation requires a database connection") {
		t.Fatalf("RecreateClient extra error: %v", err)
	}
	if txnContext(h.tm) != owner {
		t.Fatal("aborted SELECT retired the logical owner")
	}
	if !h.tm.NeedsRecovery() {
		t.Fatal("aborted SELECT did not enter recovery-required")
	}

	h.server.setFailSQL(nil)
	mustExec(t, ctx, session, "ROLLBACK TO SAVEPOINT keep")
	if h.tm.NeedsRecovery() {
		t.Fatal("ROLLBACK TO did not clear recovery-required")
	}
	if txnContext(h.tm) != owner {
		t.Fatal("ROLLBACK TO replaced the logical owner")
	}
}

func TestCli_executeStatement_abortedRecreatesClientWithoutCheckpoint(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	cli := &Cli{SessionHandler: NewSessionHandler(session), SystemVariables: session.systemVariables}

	mustExec(t, ctx, session, "SET CLI_SAVEPOINT_SUPPORT = 'ENABLED'")
	mustExec(t, ctx, session, "BEGIN RW")
	if txnContext(h.tm) == nil {
		t.Fatal("BEGIN RW did not attach an owner")
	}

	stmt, err := BuildStatement("SELECT 1")
	if err != nil {
		t.Fatal(err)
	}
	h.server.setFailSQL(status.Error(codes.Aborted, "injected abort"))
	_, err = cli.executeStatement(ctx, stmt, false, "SELECT 1", io.Discard)
	if err == nil || spanner.ErrCode(err) != codes.Aborted {
		t.Fatalf("aborted SELECT: %v", err)
	}
	if !strings.Contains(err.Error(), "database operation requires a database connection") {
		t.Fatalf("error = %v, want RecreateClient after terminal abort", err)
	}
	if h.tm.NeedsRecovery() {
		t.Fatal("no-checkpoint abort entered recovery-required")
	}
	if txnContext(h.tm) != nil {
		t.Fatal("no-checkpoint abort left a live owner")
	}
}
