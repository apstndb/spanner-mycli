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
	"testing"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestSavepointRollbackToReplaysPrefix(t *testing.T) {
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
	if err := h.tm.CreateSavepoint(ctx, "keep"); err != nil {
		t.Fatal(err)
	}
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
	if _, err := executeDML(ctx, session, "INSERT INTO T (id) VALUES (1)"); err != nil {
		t.Fatal(err)
	}
	before := replayJournal(h.tm)
	if len(before) != 3 {
		t.Fatalf("journal before rollback: %+v", before)
	}
	if err := h.tm.RollbackToSavepoint(ctx, "keep"); err != nil {
		t.Fatal(err)
	}
	after := replayJournal(h.tm)
	if len(after) != 1 || after[0].stmt.SQL != "SELECT 1" {
		t.Fatalf("journal after rollback: %+v", after)
	}
	h.tm.mu.RLock()
	_, _, ok := h.tm.tc.replay.lookup("keep")
	h.tm.mu.RUnlock()
	if !ok {
		t.Fatal("ROLLBACK TO dropped the target marker")
	}
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
}

func TestSavepointRecoveryRequiredUntilRollbackTo(t *testing.T) {
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
	if err := h.tm.CreateSavepoint(ctx, "keep"); err != nil {
		t.Fatal(err)
	}
	ok, err := h.tm.TryEnqueueAutomaticDML(spanner.NewStatement("INSERT INTO T (id) VALUES (2)"))
	if err != nil || !ok {
		t.Fatalf("enqueue: ok=%v err=%v", ok, err)
	}
	h.server.setFailBatchDML(status.Error(codes.AlreadyExists, "injected batch failure"))
	if _, err := h.tm.FlushAutomaticDML(ctx); err == nil {
		t.Fatal("flush succeeded")
	}
	if !h.tm.NeedsRecovery() {
		t.Fatal("failed DML did not enter recovery-required")
	}
	if _, err := h.tm.CommitReadWriteTransaction(ctx); !errors.Is(err, errSavepointRecovery) {
		t.Fatalf("COMMIT during recovery: %v", err)
	}
	if err := h.tm.CreateSavepoint(ctx, "later"); !errors.Is(err, errSavepointRecovery) {
		t.Fatalf("SAVEPOINT during recovery: %v", err)
	}
	h.server.setFailBatchDML(nil)
	if err := h.tm.RollbackToSavepoint(ctx, "keep"); err != nil {
		t.Fatal(err)
	}
	if h.tm.NeedsRecovery() {
		t.Fatal("ROLLBACK TO did not clear recovery-required")
	}
	after := replayJournal(h.tm)
	if len(after) != 1 {
		t.Fatalf("recovered journal: %+v", after)
	}
	if _, err := executeSQLImplWithVars(ctx, session, "SELECT 1", session.systemVariables, OperationOutput{w: io.Discard}); err != nil {
		t.Fatal(err)
	}
}

func TestSavepointReleaseAndDuplicateAndUnknown(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	h.tm.enableSavepointCaptureForTest()
	if err := h.tm.BeginReadWriteTransaction(ctx, sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED, sppb.RequestOptions_PRIORITY_UNSPECIFIED); err != nil {
		t.Fatal(err)
	}
	if err := h.tm.CreateSavepoint(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if err := h.tm.CreateSavepoint(ctx, "a"); !errors.Is(err, errSavepointDuplicate) {
		t.Fatalf("duplicate: %v", err)
	}
	if err := h.tm.CreateSavepoint(ctx, "b"); err != nil {
		t.Fatal(err)
	}
	if err := h.tm.ReleaseSavepoint("a"); err != nil {
		t.Fatal(err)
	}
	if err := h.tm.RollbackToSavepoint(ctx, "a"); !errors.Is(err, errSavepointUnknown) {
		t.Fatalf("unknown after RELEASE: %v", err)
	}
	if err := h.tm.RollbackToSavepoint(ctx, "missing"); !errors.Is(err, errSavepointUnknown) {
		t.Fatalf("unknown: %v", err)
	}
}

func TestSavepointCommandsRequireExplicitJournal(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h := newHeartbeatHarness(t)
	if err := h.tm.CreateSavepoint(ctx, "s"); !errors.Is(err, errSavepointNotInTransaction) {
		t.Fatalf("idle SAVEPOINT: %v", err)
	}
	h.tm.enableSavepointCaptureForTest()
	_, err := h.tm.RunInNewOrExistRwTx(ctx, func(*spanner.ReadWriteStmtBasedTransaction, bool) (int64, *sppb.QueryPlan, *sppb.ResultSetMetadata, error) {
		return 0, nil, nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
