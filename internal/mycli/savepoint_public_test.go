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
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanner-mycli/internal/mycli/streamio"
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

func TestSavepointPublicStreamingOutputIsolation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		format    enums.DisplayMode
		streaming enums.StreamingMode
		wantQuery func(string) bool
	}{
		{
			name:      "table_buffered",
			format:    enums.DisplayModeTable,
			streaming: enums.StreamingModeFalse,
			wantQuery: func(got string) bool { return strings.Contains(got, "1") },
		},
		{
			name:      "table_streaming",
			format:    enums.DisplayModeTable,
			streaming: enums.StreamingModeTrue,
			wantQuery: func(got string) bool { return strings.Contains(got, "1") },
		},
		{
			name:      "csv",
			format:    enums.DisplayModeCSV,
			streaming: enums.StreamingModeTrue,
			wantQuery: func(got string) bool { return strings.Contains(got, "1") },
		},
		{
			name:      "jsonl",
			format:    enums.DisplayModeJSONL,
			streaming: enums.StreamingModeTrue,
			wantQuery: func(got string) bool { return strings.Contains(got, "1") },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			h := newHeartbeatHarness(t)
			session := sessionForTM(t, h.tm)
			session.systemVariables.Display.CLIFormat = tc.format
			session.systemVariables.Query.StreamingMode = tc.streaming
			session.systemVariables.Display.SuppressResultLines = true
			var defaultOut bytes.Buffer
			session.systemVariables.StreamManager = streamio.NewStreamManager(io.NopCloser(strings.NewReader("")), &defaultOut, io.Discard)
			cli := &Cli{SessionHandler: NewSessionHandler(session), SystemVariables: session.systemVariables}

			mustExec(t, ctx, session, "SET CLI_SAVEPOINT_SUPPORT = 'ENABLED'")
			mustExec(t, ctx, session, "BEGIN RW")
			stmt, err := BuildStatement("SELECT 1")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := cli.executeStatement(ctx, stmt, false, "SELECT 1", nil); err != nil {
				t.Fatalf("prefix SELECT: %v", err)
			}
			if !tc.wantQuery(defaultOut.String()) {
				t.Fatalf("positive control missing query output %q", defaultOut.String())
			}
			mustExec(t, ctx, session, "SAVEPOINT keep")
			prefixBytes := replayRetainedBytes(h.tm)
			cache := session.systemVariables.LastResult.QueryCache
			readTs := session.systemVariables.LastResult.ReadTimestamp
			afterQuery := defaultOut.Len()

			rb, err := BuildStatement("ROLLBACK TO SAVEPOINT keep")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := cli.executeStatement(ctx, rb, false, "ROLLBACK TO SAVEPOINT keep", nil); err != nil {
				t.Fatalf("ROLLBACK TO: %v", err)
			}
			replayed := defaultOut.String()[afterQuery:]
			if tc.wantQuery(replayed) {
				t.Fatalf("replay emitted query bytes %q", replayed)
			}
			if session.systemVariables.LastResult.QueryCache != cache {
				t.Fatal("ROLLBACK TO overwrote LastResult.QueryCache")
			}
			if !session.systemVariables.LastResult.ReadTimestamp.Equal(readTs) {
				t.Fatal("ROLLBACK TO overwrote LastResult.ReadTimestamp")
			}
			if got := replayRetainedBytes(h.tm); got != prefixBytes {
				t.Fatalf("replay retainedBytes=%d, want prefix %d", got, prefixBytes)
			}
		})
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

type barrierFailWriter struct {
	started chan struct{}
	release chan struct{}
	err     error
	once    sync.Once
}

func (w *barrierFailWriter) Write([]byte) (int, error) {
	w.once.Do(func() { close(w.started) })
	<-w.release
	return 0, w.err
}

func enableBufferedMarkdownOutput(session *Session) {
	session.systemVariables.Display.CLIFormat = enums.DisplayModeTable
	session.systemVariables.Query.StreamingMode = enums.StreamingModeFalse
	session.systemVariables.Display.MarkdownCodeblock = true
	session.systemVariables.Display.Verbose = true
}

func TestSavepointPublicBufferedOutputFailureIgnoresStaleCommand(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		sameOwner bool
		next      func(*testing.T, context.Context, *Session)
	}{
		{
			name: "replacement_owner",
			next: func(t *testing.T, ctx context.Context, session *Session) {
				mustExec(t, ctx, session, "ROLLBACK")
				mustExec(t, ctx, session, "BEGIN RW")
				mustExec(t, ctx, session, "SAVEPOINT next")
			},
		},
		{
			name:      "rollback_to_new_attempt",
			sameOwner: true,
			next: func(t *testing.T, ctx context.Context, session *Session) {
				mustExec(t, ctx, session, "ROLLBACK TO SAVEPOINT keep")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			h := newHeartbeatHarness(t)
			session := sessionForTM(t, h.tm)
			enableBufferedMarkdownOutput(session)
			cli := &Cli{SessionHandler: NewSessionHandler(session), SystemVariables: session.systemVariables}

			mustExec(t, ctx, session, "SET CLI_SAVEPOINT_SUPPORT = 'ENABLED'")
			mustExec(t, ctx, session, "BEGIN RW")
			mustExec(t, ctx, session, "SAVEPOINT keep")
			owner := txnContext(h.tm)
			attempt := owner.attempt

			stmt, err := BuildStatement("SELECT 1")
			if err != nil {
				t.Fatal(err)
			}
			cause := errors.New("stale buffered display failed")
			w := &barrierFailWriter{
				started: make(chan struct{}),
				release: make(chan struct{}),
				err:     cause,
			}
			done := make(chan error, 1)
			go func() {
				_, err := cli.executeStatement(ctx, stmt, false, "SELECT 1", w)
				done <- err
			}()
			select {
			case <-w.started:
			case <-time.After(5 * time.Second):
				t.Fatal("buffered writer did not start")
			}

			tc.next(t, ctx, session)
			mustExec(t, ctx, session, "SELECT 2")
			live := txnContext(h.tm)
			afterSelect2 := replayJournal(h.tm)
			if len(afterSelect2) != 1 || afterSelect2[0].stmt.SQL != "SELECT 2" {
				t.Fatalf("journal before stale error: %+v", afterSelect2)
			}

			close(w.release)
			select {
			case err := <-done:
				if !errors.Is(err, cause) {
					t.Fatalf("stale buffered error = %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("stale buffered display did not return")
			}
			if txnContext(h.tm) != live {
				t.Fatal("stale display error replaced the live logical owner")
			}
			if h.tm.NeedsRecovery() {
				t.Fatal("stale display error entered recovery on the live owner")
			}
			after := replayJournal(h.tm)
			if len(after) != 1 || after[0].stmt.SQL != "SELECT 2" {
				t.Fatalf("stale display error retracted the live journal: %+v", after)
			}
			if tc.sameOwner {
				if live != owner {
					t.Fatal("ROLLBACK TO retired the logical owner")
				}
				if live.attempt == attempt {
					t.Fatal("ROLLBACK TO did not replace the physical attempt")
				}
			} else if live == owner {
				t.Fatal("replacement owner reused the original transactionContext")
			}
		})
	}
}

func TestSavepointPublicBufferedOutputFailureInvalidatesSameAttemptSuffix(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	enableBufferedMarkdownOutput(session)
	cli := &Cli{SessionHandler: NewSessionHandler(session), SystemVariables: session.systemVariables}

	mustExec(t, ctx, session, "SET CLI_SAVEPOINT_SUPPORT = 'ENABLED'")
	mustExec(t, ctx, session, "BEGIN RW")
	mustExec(t, ctx, session, "SAVEPOINT keep")
	owner := txnContext(h.tm)
	attempt := owner.attempt

	stmt, err := BuildStatement("SELECT 1")
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("incomplete buffered display failed")
	w := &barrierFailWriter{
		started: make(chan struct{}),
		release: make(chan struct{}),
		err:     cause,
	}
	done := make(chan error, 1)
	go func() {
		_, err := cli.executeStatement(ctx, stmt, false, "SELECT 1", w)
		done <- err
	}()
	select {
	case <-w.started:
	case <-time.After(5 * time.Second):
		t.Fatal("buffered writer did not start")
	}

	mustExec(t, ctx, session, "SELECT 2")
	mustExec(t, ctx, session, "SAVEPOINT incomplete")
	if txnContext(h.tm) != owner || owner.attempt != attempt {
		t.Fatal("same-attempt suffix used a different owner/attempt")
	}
	beforeFail := replayJournal(h.tm)
	if len(beforeFail) != 2 {
		t.Fatalf("journal before stale failure: %+v", beforeFail)
	}

	close(w.release)
	select {
	case err := <-done:
		if !errors.Is(err, cause) {
			t.Fatalf("incomplete buffered error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("incomplete buffered display did not return")
	}
	if txnContext(h.tm) != owner {
		t.Fatal("same-attempt display failure retired the logical owner")
	}
	if !h.tm.NeedsRecovery() {
		t.Fatal("incomplete output did not enter recovery-required")
	}
	after := replayJournal(h.tm)
	if len(after) != 0 {
		t.Fatalf("failed output remained a checkpoint: %+v", after)
	}
	if _, err := execSQL(t, ctx, session, "ROLLBACK TO SAVEPOINT incomplete"); !errors.Is(err, errSavepointUnknown) {
		t.Fatalf("dependent marker after incomplete output: %v", err)
	}

	obsBefore := len(h.server.sqlObservations())
	mustExec(t, ctx, session, "ROLLBACK TO SAVEPOINT keep")
	if h.tm.NeedsRecovery() {
		t.Fatal("ROLLBACK TO keep did not clear recovery-required")
	}
	for _, rec := range h.server.sqlObservations()[obsBefore:] {
		switch rec.sql {
		case "SELECT 1", "SELECT 2":
			t.Fatalf("replayed failed or suffix command %q", rec.sql)
		}
	}
}

func TestSavepointPublicBufferedOutputFailurePreservesRecoveryAcrossOverlappingDisplay(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name             string
		failSelect2First bool
	}{
		{name: "later_then_earlier", failSelect2First: true},
		{name: "earlier_then_later", failSelect2First: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			h := newHeartbeatHarness(t)
			session := sessionForTM(t, h.tm)
			enableBufferedMarkdownOutput(session)
			cli := &Cli{SessionHandler: NewSessionHandler(session), SystemVariables: session.systemVariables}

			mustExec(t, ctx, session, "SET CLI_SAVEPOINT_SUPPORT = 'ENABLED'")
			mustExec(t, ctx, session, "BEGIN RW")
			mustExec(t, ctx, session, "SAVEPOINT keep")
			owner := txnContext(h.tm)

			stmt1, err := BuildStatement("SELECT 1")
			if err != nil {
				t.Fatal(err)
			}
			stmt2, err := BuildStatement("SELECT 2")
			if err != nil {
				t.Fatal(err)
			}
			cause1 := errors.New("select1 markdown fence failed")
			cause2 := errors.New("select2 markdown fence failed")
			w1 := &barrierFailWriter{started: make(chan struct{}), release: make(chan struct{}), err: cause1}
			w2 := &barrierFailWriter{started: make(chan struct{}), release: make(chan struct{}), err: cause2}
			done1 := make(chan error, 1)
			done2 := make(chan error, 1)
			go func() {
				_, err := cli.executeStatement(ctx, stmt1, false, "SELECT 1", w1)
				done1 <- err
			}()
			select {
			case <-w1.started:
			case <-time.After(5 * time.Second):
				t.Fatal("SELECT 1 writer did not start")
			}
			go func() {
				_, err := cli.executeStatement(ctx, stmt2, false, "SELECT 2", w2)
				done2 <- err
			}()
			select {
			case <-w2.started:
			case <-time.After(5 * time.Second):
				t.Fatal("SELECT 2 writer did not start")
			}

			fail := func(release chan struct{}, done chan error, cause error, name string) {
				t.Helper()
				close(release)
				select {
				case err := <-done:
					if !errors.Is(err, cause) {
						t.Fatalf("%s display error = %v", name, err)
					}
				case <-time.After(5 * time.Second):
					t.Fatalf("%s display did not return", name)
				}
			}
			if tc.failSelect2First {
				fail(w2.release, done2, cause2, "SELECT 2")
			} else {
				fail(w1.release, done1, cause1, "SELECT 1")
			}
			if txnContext(h.tm) != owner {
				t.Fatal("first overlapping display failure retired the logical owner")
			}
			if !h.tm.NeedsRecovery() {
				t.Fatal("first overlapping display failure did not enter recovery-required")
			}
			if tc.failSelect2First {
				fail(w1.release, done1, cause1, "SELECT 1")
			} else {
				fail(w2.release, done2, cause2, "SELECT 2")
			}
			if txnContext(h.tm) != owner {
				t.Fatal("second overlapping display failure retired the recovery owner")
			}
			if !h.tm.NeedsRecovery() {
				t.Fatal("second overlapping display failure cleared recovery-required")
			}
			mustExec(t, ctx, session, "ROLLBACK TO SAVEPOINT keep")
			if h.tm.NeedsRecovery() {
				t.Fatal("ROLLBACK TO keep did not clear recovery-required")
			}
			if txnContext(h.tm) != owner {
				t.Fatal("ROLLBACK TO keep retired the logical owner")
			}
		})
	}
}

func TestSavepointPublicBufferedOutputFailureRetractsMutateAndBatch(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	for _, tc := range []struct {
		name string
		sql  string
		kind replayKind
		prep func(*testing.T, context.Context, *Session)
	}{
		{name: "mutate", sql: "MUTATE T INSERT STRUCT(1 AS id)", kind: replayKindMutate},
		{
			name: "run_batch",
			sql:  "RUN BATCH",
			kind: replayKindBatchDML,
			prep: func(t *testing.T, ctx context.Context, session *Session) {
				mustExec(t, ctx, session, "START BATCH DML")
				mustExec(t, ctx, session, "INSERT INTO T (id) VALUES (1)")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHeartbeatHarness(t)
			session := sessionForTM(t, h.tm)
			enableBufferedMarkdownOutput(session)
			cli := &Cli{SessionHandler: NewSessionHandler(session), SystemVariables: session.systemVariables}

			mustExec(t, ctx, session, "SET CLI_SAVEPOINT_SUPPORT = 'ENABLED'")
			mustExec(t, ctx, session, "BEGIN RW")
			mustExec(t, ctx, session, "SAVEPOINT keep")
			owner := txnContext(h.tm)
			prefix := replayJournal(h.tm)

			if tc.prep != nil {
				tc.prep(t, ctx, session)
			}
			stmt, err := BuildStatement(tc.sql)
			if err != nil {
				t.Fatal(err)
			}
			cause := errors.New(tc.name + " markdown fence failed")
			_, err = cli.executeStatement(ctx, stmt, false, tc.sql, &resultFailureWriter{err: cause})
			if !errors.Is(err, cause) {
				t.Fatalf("%s display error = %v", tc.name, err)
			}
			if txnContext(h.tm) != owner {
				t.Fatal("buffered output failure retired the logical owner")
			}
			if !h.tm.NeedsRecovery() {
				t.Fatal("buffered output failure did not enter recovery-required")
			}
			after := replayJournal(h.tm)
			if len(after) != len(prefix) {
				t.Fatalf("failed %s remained journaled: %+v", tc.name, after)
			}
			if _, err := execSQL(t, ctx, session, "SAVEPOINT later"); !errors.Is(err, errSavepointRecovery) {
				t.Fatalf("SAVEPOINT after buffered failure: %v", err)
			}

			mustExec(t, ctx, session, "ROLLBACK TO SAVEPOINT keep")
			if h.tm.NeedsRecovery() {
				t.Fatal("ROLLBACK TO did not clear recovery-required")
			}

			if tc.prep != nil {
				tc.prep(t, ctx, session)
			}
			var ok bytes.Buffer
			if _, err := cli.executeStatement(ctx, stmt, false, tc.sql, &ok); err != nil {
				t.Fatalf("successful %s output: %v", tc.name, err)
			}
			if h.tm.NeedsRecovery() {
				t.Fatal("successful buffered output entered recovery-required")
			}
			journal := replayJournal(h.tm)
			if len(journal) != len(prefix)+1 || journal[len(journal)-1].kind != tc.kind {
				t.Fatalf("successful %s journal: %+v", tc.name, journal)
			}
		})
	}
}

func TestSavepointPublicBufferedOutputFailureWithoutMarkerEndsTransaction(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	for _, tc := range []struct {
		name string
		sql  string
		prep func(*testing.T, context.Context, *Session)
	}{
		{name: "insert", sql: "INSERT INTO T (id) VALUES (1)"},
		{name: "mutate", sql: "MUTATE T INSERT STRUCT(1 AS id)"},
		{
			name: "run_batch",
			sql:  "RUN BATCH",
			prep: func(t *testing.T, ctx context.Context, session *Session) {
				mustExec(t, ctx, session, "START BATCH DML")
				mustExec(t, ctx, session, "INSERT INTO T (id) VALUES (1)")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHeartbeatHarness(t)
			session := sessionForTM(t, h.tm)
			enableBufferedMarkdownOutput(session)
			cli := &Cli{SessionHandler: NewSessionHandler(session), SystemVariables: session.systemVariables}

			mustExec(t, ctx, session, "SET CLI_SAVEPOINT_SUPPORT = 'ENABLED'")
			mustExec(t, ctx, session, "BEGIN RW")
			mustExec(t, ctx, session, "SET LOCAL OPTIMIZER_VERSION = '9'")
			if tc.prep != nil {
				tc.prep(t, ctx, session)
			}
			stmt, err := BuildStatement(tc.sql)
			if err != nil {
				t.Fatal(err)
			}
			cause := errors.New(tc.name + " markdown fence failed")
			_, err = cli.executeStatement(ctx, stmt, false, tc.sql, &resultFailureWriter{err: cause})
			if !errors.Is(err, cause) {
				t.Fatalf("%s display error = %v", tc.name, err)
			}
			if txnContext(h.tm) != nil {
				t.Fatal("no-marker buffered output failure left the logical owner")
			}
			if h.tm.NeedsRecovery() {
				t.Fatal("no-marker buffered output failure entered recovery-required")
			}
			if h.tm.InTransaction() {
				t.Fatal("no-marker buffered output failure left a transaction")
			}
			if got := mustGetVar(t, session, "OPTIMIZER_VERSION"); got == "9" {
				t.Fatal("SET LOCAL survived no-marker output failure")
			}
			if _, err := execSQL(t, ctx, session, "SAVEPOINT keep"); !errors.Is(err, errSavepointNotInTransaction) {
				t.Fatalf("SAVEPOINT after no-marker output failure: %v", err)
			}
		})
	}
}

func TestSavepointPublicBufferedOutputFailureInvalidatesOnlyMarkerEndsTransaction(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	session := sessionForTM(t, h.tm)
	enableBufferedMarkdownOutput(session)
	cli := &Cli{SessionHandler: NewSessionHandler(session), SystemVariables: session.systemVariables}

	mustExec(t, ctx, session, "SET CLI_SAVEPOINT_SUPPORT = 'ENABLED'")
	mustExec(t, ctx, session, "BEGIN RW")
	stmt, err := BuildStatement("INSERT INTO T (id) VALUES (1)")
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("only-marker markdown fence failed")
	w := &barrierFailWriter{
		started: make(chan struct{}),
		release: make(chan struct{}),
		err:     cause,
	}
	done := make(chan error, 1)
	go func() {
		_, err := cli.executeStatement(ctx, stmt, false, "INSERT INTO T (id) VALUES (1)", w)
		done <- err
	}()
	select {
	case <-w.started:
	case <-time.After(5 * time.Second):
		t.Fatal("buffered writer did not start")
	}

	mustExec(t, ctx, session, "SAVEPOINT later")
	close(w.release)
	select {
	case err := <-done:
		if !errors.Is(err, cause) {
			t.Fatalf("only-marker buffered error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("only-marker buffered display did not return")
	}
	if txnContext(h.tm) != nil {
		t.Fatal("only-marker invalidation left the logical owner")
	}
	if h.tm.NeedsRecovery() {
		t.Fatal("only-marker invalidation entered recovery-required")
	}
	if _, err := execSQL(t, ctx, session, "ROLLBACK TO SAVEPOINT later"); !errors.Is(err, errSavepointNotInTransaction) && !errors.Is(err, errSavepointUnknown) {
		t.Fatalf("ROLLBACK TO later after only-marker invalidation: %v", err)
	}
}

func TestSavepointPublicFailedUseCandidatePreservesManualBatchCallback(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	live := ConnectionVars{Project: "p", Instance: "i", Database: "db"}

	t.Run("failed_candidate", func(t *testing.T) {
		t.Parallel()
		sv, session := newBoundSwitchSession(t, live)
		handler := NewSessionHandler(session)
		handler.constructCandidate = func(_ context.Context, identity ConnectionVars) (*Session, error) {
			c := newConstructedSession(DatabaseConnected, nil, nil, spanner.ClientConfig{}, nil, sv, identity)
			c.databaseExistsOverride = func(context.Context) (bool, error) { return false, nil }
			return c, nil
		}
		_, err := handler.ExecuteStatement(ctx, &UseStatement{Database: "missing"})
		if err == nil || !strings.Contains(err.Error(), `unknown database "missing"`) {
			t.Fatalf("USE missing: %v", err)
		}
		if handler.Session != session {
			t.Fatal("failed USE replaced the live session")
		}
		mustExec(t, ctx, session, "START BATCH DML")
		if _, err := execSQL(t, ctx, session, "SET CLI_SAVEPOINT_SUPPORT = 'ENABLED'"); !errors.Is(err, errSetterInManualBatch) {
			t.Fatalf("SET after failed USE candidate: %v", err)
		}
		if !session.batch.IsActive() {
			t.Fatal("rejected SET aborted the batch")
		}
	})

	t.Run("successful_initial", func(t *testing.T) {
		t.Parallel()
		_, session := newBoundSwitchSession(t, live)
		mustExec(t, ctx, session, "START BATCH DML")
		if _, err := execSQL(t, ctx, session, "SET CLI_SAVEPOINT_SUPPORT = 'ENABLED'"); !errors.Is(err, errSetterInManualBatch) {
			t.Fatalf("SET on initial session: %v", err)
		}
	})

	t.Run("successful_adoption", func(t *testing.T) {
		t.Parallel()
		sv, session := newBoundSwitchSession(t, live)
		handler := NewSessionHandler(session)
		handler.constructCandidate = func(_ context.Context, identity ConnectionVars) (*Session, error) {
			c := newConstructedSession(DatabaseConnected, nil, nil, spanner.ClientConfig{}, nil, sv, identity)
			c.databaseExistsOverride = func(context.Context) (bool, error) { return true, nil }
			return c, nil
		}
		if _, err := handler.ExecuteStatement(ctx, &UseStatement{Database: "next"}); err != nil {
			t.Fatal(err)
		}
		if handler.Session == session {
			t.Fatal("successful USE did not adopt the candidate")
		}
		mustExec(t, ctx, handler.Session, "START BATCH DML")
		if _, err := execSQL(t, ctx, handler.Session, "SET CLI_SAVEPOINT_SUPPORT = 'ENABLED'"); !errors.Is(err, errSetterInManualBatch) {
			t.Fatalf("SET after adoption: %v", err)
		}
	})
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
