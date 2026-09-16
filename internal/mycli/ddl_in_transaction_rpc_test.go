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
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/apstndb/spanner-mycli/enums"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// uninitializedFileDescriptorSet is a FileDescriptorSet whose nested proto2
// NamePart is missing required fields. Local marshal/CheckInitialized must
// reject it before any Commit or Admin RPC.
func uninitializedFileDescriptorSet() *descriptorpb.FileDescriptorSet {
	return &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{{
			Name: proto.String("bad.proto"),
			Options: &descriptorpb.FileOptions{
				UninterpretedOption: []*descriptorpb.UninterpretedOption{{
					Name: []*descriptorpb.UninterpretedOption_NamePart{{}},
				}},
			},
		}},
	}
}

const ddlInTxnSQL = "CREATE TABLE t (id INT64) PRIMARY KEY (id)"

type ddlTxnHarness struct {
	hb      *heartbeatHarness
	admin   *ddlAdminTestServer
	session *Session
}

func newDDLTxnHarness(t *testing.T) *ddlTxnHarness {
	t.Helper()
	hb := newHeartbeatHarness(t)
	identity := ConnectionVars{Project: "test", Instance: "test", Database: "test"}
	hb.tm.sysVars.Connection = identity
	admin := newCompletedDDLServer(ddlInTxnSQL, time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC))
	session := sessionForTM(t, hb.tm)
	session.connection = identity
	hb.attachSessionClient(session)
	attachDDLAdmin(t, session, admin)
	return &ddlTxnHarness{hb: hb, admin: admin, session: session}
}

func attachDDLAdmin(t *testing.T, session *Session, server *ddlAdminTestServer) {
	t.Helper()
	session.adminClient = newBufconnAdminClient(t, server)
}

func (h *ddlTxnHarness) adminCalled() bool {
	h.admin.mu.Lock()
	defer h.admin.mu.Unlock()
	return h.admin.lastUpdate != nil
}

func TestDDLInTransactionRPCAdmission(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	t.Run("idle executes DDL", func(t *testing.T) {
		t.Parallel()
		h := newDDLTxnHarness(t)
		mustExec(t, ctx, h.session, "CREATE TABLE t (id INT64) PRIMARY KEY (id)")
		if !h.adminCalled() {
			t.Fatal("idle DDL must call UpdateDatabaseDdl")
		}
		if len(h.hb.server.commits) != 0 {
			t.Fatalf("idle DDL committed %d times", len(h.hb.server.commits))
		}
	})

	t.Run("fail pending rejects without begin or admin", func(t *testing.T) {
		t.Parallel()
		h := newDDLTxnHarness(t)
		mustExec(t, ctx, h.session, "BEGIN")
		_, err := execSQL(t, ctx, h.session, ddlInTxnSQL)
		if !errors.Is(err, errDDLInTransaction) {
			t.Fatalf("err=%v", err)
		}
		if h.adminCalled() {
			t.Fatal("FAIL pending must not call Admin")
		}
		if len(h.hb.server.begins) != 0 {
			t.Fatalf("FAIL pending began %v", h.hb.server.begins)
		}
		if !h.session.txn.InPendingTransaction() {
			t.Fatal("pending owner must remain")
		}
	})

	t.Run("allow pending retires without begin or commit", func(t *testing.T) {
		t.Parallel()
		h := newDDLTxnHarness(t)
		mustExec(t, ctx, h.session, "SET CLI_DDL_IN_TRANSACTION_MODE = 'ALLOW_IN_EMPTY_TRANSACTION'")
		mustExec(t, ctx, h.session, "BEGIN")
		mustExec(t, ctx, h.session, ddlInTxnSQL)
		if !h.adminCalled() {
			t.Fatal("ALLOW pending must run DDL")
		}
		if len(h.hb.server.begins) != 0 {
			t.Fatalf("ALLOW pending constructed %v", h.hb.server.begins)
		}
		if len(h.hb.server.commits) != 0 {
			t.Fatalf("ALLOW pending committed %v", h.hb.server.commits)
		}
		if h.session.txn.InTransaction() {
			t.Fatal("owner must be retired")
		}
	})

	t.Run("auto_commit pending is no-op retire", func(t *testing.T) {
		t.Parallel()
		h := newDDLTxnHarness(t)
		mustExec(t, ctx, h.session, "SET CLI_DDL_IN_TRANSACTION_MODE = 'AUTO_COMMIT_TRANSACTION'")
		mustExec(t, ctx, h.session, "BEGIN")
		mustExec(t, ctx, h.session, ddlInTxnSQL)
		if !h.adminCalled() {
			t.Fatal("AUTO_COMMIT pending must run DDL")
		}
		if len(h.hb.server.commits) != 0 {
			t.Fatalf("pending AUTO_COMMIT must not Commit, got %v", h.hb.server.commits)
		}
		if len(h.hb.server.begins) != 0 {
			t.Fatalf("pending AUTO_COMMIT must not BeginTransaction, got %v", h.hb.server.begins)
		}
	})

	t.Run("allow constructor-only rolls back then ddl", func(t *testing.T) {
		t.Parallel()
		h := newDDLTxnHarness(t)
		mustExec(t, ctx, h.session, "SET CLI_DDL_IN_TRANSACTION_MODE = 'ALLOW_IN_EMPTY_TRANSACTION'")
		mustExec(t, ctx, h.session, "BEGIN RW")
		if len(h.hb.server.begins) == 0 {
			t.Fatal("BEGIN RW must construct")
		}
		mustExec(t, ctx, h.session, ddlInTxnSQL)
		if len(h.hb.server.rollbacks) == 0 {
			t.Fatal("ALLOW constructor-only must Rollback")
		}
		if len(h.hb.server.commits) != 0 {
			t.Fatalf("ALLOW constructor-only must not Commit, got %v", h.hb.server.commits)
		}
		if !h.adminCalled() {
			t.Fatal("DDL missing after rollback")
		}
	})

	t.Run("auto_commit constructor-only commits then ddl", func(t *testing.T) {
		t.Parallel()
		h := newDDLTxnHarness(t)
		mustExec(t, ctx, h.session, "SET CLI_DDL_IN_TRANSACTION_MODE = 'AUTO_COMMIT_TRANSACTION'")
		mustExec(t, ctx, h.session, "BEGIN RW")
		mustExec(t, ctx, h.session, ddlInTxnSQL)
		if len(h.hb.server.commits) != 1 {
			t.Fatalf("commits=%v, want 1", h.hb.server.commits)
		}
		if !h.adminCalled() {
			t.Fatal("DDL missing after commit")
		}
		if h.session.txn.InTransaction() {
			t.Fatal("owner must be retired before Admin")
		}
	})

	t.Run("allow rejects user work without admin", func(t *testing.T) {
		t.Parallel()
		h := newDDLTxnHarness(t)
		mustExec(t, ctx, h.session, "SET CLI_DDL_IN_TRANSACTION_MODE = 'ALLOW_IN_EMPTY_TRANSACTION'")
		mustExec(t, ctx, h.session, "BEGIN RW")
		mustExec(t, ctx, h.session, "SELECT 1")
		_, err := execSQL(t, ctx, h.session, ddlInTxnSQL)
		if !errors.Is(err, errDDLInNonEmptyTransaction) {
			t.Fatalf("err=%v", err)
		}
		if h.adminCalled() {
			t.Fatal("nonempty ALLOW must not call Admin")
		}
		if !h.session.txn.InReadWriteTransaction() {
			t.Fatal("rejected DDL must leave the RW owner")
		}
	})

	t.Run("auto_commit user work commits then ddl", func(t *testing.T) {
		t.Parallel()
		h := newDDLTxnHarness(t)
		mustExec(t, ctx, h.session, "SET CLI_DDL_IN_TRANSACTION_MODE = 'AUTO_COMMIT_TRANSACTION'")
		mustExec(t, ctx, h.session, "BEGIN RW")
		mustExec(t, ctx, h.session, "SELECT 1")
		mustExec(t, ctx, h.session, ddlInTxnSQL)
		if len(h.hb.server.commits) != 1 {
			t.Fatalf("commits=%v", h.hb.server.commits)
		}
		if !h.adminCalled() {
			t.Fatal("expected Admin after commit")
		}
	})

	t.Run("commit failure prevents admin", func(t *testing.T) {
		t.Parallel()
		h := newDDLTxnHarness(t)
		mustExec(t, ctx, h.session, "SET CLI_DDL_IN_TRANSACTION_MODE = 'AUTO_COMMIT_TRANSACTION'")
		mustExec(t, ctx, h.session, "BEGIN RW")
		h.hb.tm.commitOverride = func(context.Context, *spanner.ReadWriteStmtBasedTransaction) (spanner.CommitResponse, error) {
			return spanner.CommitResponse{}, status.Error(codes.Aborted, "injected commit failure")
		}
		_, err := execSQL(t, ctx, h.session, ddlInTxnSQL)
		if err == nil || !strings.Contains(err.Error(), "injected commit failure") {
			t.Fatalf("err=%v", err)
		}
		var after *ddlAfterCommitError
		if errors.As(err, &after) {
			t.Fatal("must not report a successful commit after commit error")
		}
		if h.adminCalled() {
			t.Fatal("Admin after commit failure")
		}
	})

	t.Run("successful commit then failed ddl", func(t *testing.T) {
		t.Parallel()
		h := newDDLTxnHarness(t)
		injected := status.Error(codes.InvalidArgument, "bad ddl")
		h.admin.updateErr = injected
		mustExec(t, ctx, h.session, "SET CLI_DDL_IN_TRANSACTION_MODE = 'AUTO_COMMIT_TRANSACTION'")
		mustExec(t, ctx, h.session, "BEGIN RW")
		_, err := execSQL(t, ctx, h.session, ddlInTxnSQL)
		var after *ddlAfterCommitError
		if !errors.As(err, &after) {
			t.Fatalf("err=%v, want ddlAfterCommitError", err)
		}
		if !errors.Is(err, injected) {
			t.Fatalf("must preserve injected Admin cause: %v", err)
		}
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("status=%v, want InvalidArgument: %v", status.Code(err), err)
		}
		if !strings.Contains(err.Error(), "bad ddl") {
			t.Fatalf("must preserve DDL cause: %v", err)
		}
		if len(h.hb.server.commits) != 1 {
			t.Fatalf("commits=%v", h.hb.server.commits)
		}
		if !h.adminCalled() {
			t.Fatal("Admin must have been attempted")
		}
	})

	t.Run("manual dml batch rejects all modes", func(t *testing.T) {
		t.Parallel()
		for _, mode := range []string{"FAIL", "ALLOW_IN_EMPTY_TRANSACTION", "AUTO_COMMIT_TRANSACTION"} {
			t.Run(mode, func(t *testing.T) {
				t.Parallel()
				h := newDDLTxnHarness(t)
				mustExec(t, ctx, h.session, "SET CLI_DDL_IN_TRANSACTION_MODE = '"+mode+"'")
				mustExec(t, ctx, h.session, "BEGIN RW")
				mustExec(t, ctx, h.session, "START BATCH DML")
				_, err := execSQL(t, ctx, h.session, ddlInTxnSQL)
				if !errors.Is(err, errDDLManualDMLBatch) {
					t.Fatalf("err=%v", err)
				}
				if h.adminCalled() {
					t.Fatal("Admin during manual DML batch")
				}
				if len(h.hb.server.commits) != 0 {
					t.Fatal("must not implicit RUN/COMMIT the DML batch")
				}
				if !h.session.batch.IsActive() {
					t.Fatal("manual DML batch must remain")
				}
			})
		}
	})

	t.Run("auto dml count mismatch prevents commit and admin", func(t *testing.T) {
		t.Parallel()
		h := newDDLTxnHarness(t)
		mustExec(t, ctx, h.session, "SET CLI_DDL_IN_TRANSACTION_MODE = 'AUTO_COMMIT_TRANSACTION'")
		mustExec(t, ctx, h.session, "SET AUTO_BATCH_DML = TRUE")
		mustExec(t, ctx, h.session, "SET AUTO_BATCH_DML_UPDATE_COUNT = 1")
		mustExec(t, ctx, h.session, "SET AUTO_BATCH_DML_UPDATE_COUNT_VERIFICATION = TRUE")
		mustExec(t, ctx, h.session, "BEGIN RW")
		const dml = "INSERT INTO t (id) VALUES (1)"
		h.hb.server.setSQLRowCount(dml, 2)
		mustExec(t, ctx, h.session, dml)
		_, err := execSQL(t, ctx, h.session, ddlInTxnSQL)
		if err == nil || !errors.Is(err, errAutomaticDMLCountMismatch) {
			t.Fatalf("err=%v, want count mismatch", err)
		}
		if h.adminCalled() {
			t.Fatal("Admin after count mismatch")
		}
		var after *ddlAfterCommitError
		if errors.As(err, &after) {
			t.Fatal("count mismatch is not a successful commit")
		}
	})

	t.Run("read-only rejects with zero admin", func(t *testing.T) {
		t.Parallel()
		h := newDDLTxnHarness(t)
		mustExec(t, ctx, h.session, "SET CLI_DDL_IN_TRANSACTION_MODE = 'AUTO_COMMIT_TRANSACTION'")
		mustExec(t, ctx, h.session, "BEGIN RO")
		_, err := execSQL(t, ctx, h.session, ddlInTxnSQL)
		if !errors.Is(err, errReadOnly) && !errors.Is(err, errDDLInReadOnlyTransaction) {
			t.Fatalf("err=%v", err)
		}
		if h.adminCalled() {
			t.Fatal("RO must not call Admin")
		}
		if len(h.hb.server.commits) != 0 {
			t.Fatal("RO must not commit")
		}
	})

	t.Run("start batch ddl allow pending then run", func(t *testing.T) {
		t.Parallel()
		h := newDDLTxnHarness(t)
		mustExec(t, ctx, h.session, "SET CLI_DDL_IN_TRANSACTION_MODE = 'ALLOW_IN_EMPTY_TRANSACTION'")
		mustExec(t, ctx, h.session, "BEGIN")
		mustExec(t, ctx, h.session, "START BATCH DDL")
		if h.session.txn.InTransaction() {
			t.Fatal("START BATCH DDL ALLOW must retire pending first")
		}
		mustExec(t, ctx, h.session, ddlInTxnSQL)
		if h.adminCalled() {
			t.Fatal("buffered DDL must not run until RUN BATCH")
		}
		mustExec(t, ctx, h.session, "RUN BATCH")
		if !h.adminCalled() {
			t.Fatal("RUN BATCH must execute DDL")
		}
	})

	t.Run("owner after start batch ddl is rechecked on run", func(t *testing.T) {
		t.Parallel()
		h := newDDLTxnHarness(t)
		mustExec(t, ctx, h.session, "START BATCH DDL")
		mustExec(t, ctx, h.session, ddlInTxnSQL)
		mustExec(t, ctx, h.session, "BEGIN")
		_, err := execSQL(t, ctx, h.session, "RUN BATCH")
		if !errors.Is(err, errDDLInTransaction) {
			t.Fatalf("RUN BATCH: %v, want FAIL on later owner", err)
		}
		if h.adminCalled() {
			t.Fatal("RUN BATCH must not bypass admission")
		}
		if !h.session.batch.IsActive() {
			t.Fatal("rejected RUN BATCH should keep the DDL batch")
		}
	})

	t.Run("run batch after later begin auto_commit admin failure keeps commit receipt", func(t *testing.T) {
		t.Parallel()
		h := newDDLTxnHarness(t)
		injected := status.Error(codes.InvalidArgument, "bad ddl")
		h.admin.updateErr = injected
		mustExec(t, ctx, h.session, "SET CLI_DDL_IN_TRANSACTION_MODE = 'AUTO_COMMIT_TRANSACTION'")
		mustExec(t, ctx, h.session, "START BATCH DDL")
		mustExec(t, ctx, h.session, ddlInTxnSQL)
		mustExec(t, ctx, h.session, "BEGIN RW")
		_, err := execSQL(t, ctx, h.session, "RUN BATCH")
		var after *ddlAfterCommitError
		if !errors.As(err, &after) {
			t.Fatalf("err=%v, want ddlAfterCommitError", err)
		}
		if !errors.Is(err, injected) {
			t.Fatalf("must preserve injected Admin cause: %v", err)
		}
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("status=%v, want InvalidArgument: %v", status.Code(err), err)
		}
		if len(h.hb.server.commits) != 1 {
			t.Fatalf("commits=%v, want 1", h.hb.server.commits)
		}
		if !h.adminCalled() {
			t.Fatal("Admin must have been attempted")
		}
		if h.session.batch.IsActive() {
			t.Fatal("successful admission should consume the batch")
		}
	})

	t.Run("run batch after later begin prevalidation failure keeps batch", func(t *testing.T) {
		t.Parallel()
		h := newDDLTxnHarness(t)
		mustExec(t, ctx, h.session, "SET CLI_DDL_IN_TRANSACTION_MODE = 'AUTO_COMMIT_TRANSACTION'")
		mustExec(t, ctx, h.session, "START BATCH DDL")
		mustExec(t, ctx, h.session, ddlInTxnSQL)
		mustExec(t, ctx, h.session, "BEGIN RW")
		h.session.systemVariables.Internal.ProtoDescriptor = uninitializedFileDescriptorSet()
		_, err := execSQL(t, ctx, h.session, "RUN BATCH")
		if err == nil {
			t.Fatal("expected descriptor validation error")
		}
		var after *ddlAfterCommitError
		if errors.As(err, &after) {
			t.Fatal("prevalidation must not report a successful commit")
		}
		if len(h.hb.server.commits) != 0 {
			t.Fatalf("prevalidation must not Commit, commits=%v", h.hb.server.commits)
		}
		if h.adminCalled() {
			t.Fatal("prevalidation must not call Admin")
		}
		if !h.session.batch.IsActive() {
			t.Fatal("failed admission must keep the DDL batch")
		}
		if !h.session.txn.InReadWriteTransaction() {
			t.Fatal("failed admission must leave the RW owner")
		}
	})
}

func TestDDLInTransactionStartBatchAutoCommitConstructor(t *testing.T) {
	t.Parallel()
	h := newDDLTxnHarness(t)
	ctx := t.Context()
	mustExec(t, ctx, h.session, "SET CLI_DDL_IN_TRANSACTION_MODE = 'AUTO_COMMIT_TRANSACTION'")
	mustExec(t, ctx, h.session, "BEGIN RW")
	mustExec(t, ctx, h.session, "START BATCH DDL")
	if len(h.hb.server.commits) != 1 {
		t.Fatalf("START BATCH DDL AUTO_COMMIT constructor-only commits, got %v", h.hb.server.commits)
	}
	if h.adminCalled() {
		t.Fatal("START must not submit Admin")
	}
	mustExec(t, ctx, h.session, "RUN BATCH")
	if h.adminCalled() {
		t.Fatal("empty RUN BATCH must not call Admin")
	}
	if len(h.hb.server.commits) != 1 {
		t.Fatalf("empty RUN BATCH must not commit again, commits=%v", h.hb.server.commits)
	}
}

func TestDDLInTransactionLocalAsyncRestoredUsesSync(t *testing.T) {
	t.Parallel()
	h := newDDLTxnHarness(t)
	h.admin.done = false
	h.admin.getErr = status.Error(codes.Canceled, "context canceled")
	ctx := t.Context()
	mustExec(t, ctx, h.session, "SET CLI_DDL_IN_TRANSACTION_MODE = 'AUTO_COMMIT_TRANSACTION'")
	mustExec(t, ctx, h.session, "BEGIN RW")
	mustExec(t, ctx, h.session, "SET LOCAL DDL_EXECUTION_MODE = 'ASYNC'")
	_, err := execSQL(t, ctx, h.session, ddlInTxnSQL)
	if err == nil || !strings.Contains(err.Error(), "SHOW OPERATION") {
		t.Fatalf("restored SYNC should poll and hint SHOW OPERATION, err=%v", err)
	}
	if h.admin.getCalls.Load() == 0 {
		t.Fatal("SET LOCAL ASYNC must be restored before Admin; SYNC polls")
	}
	if got := mustGetVar(t, h.session, "DDL_EXECUTION_MODE"); got != "SYNC" {
		t.Fatalf("DDL_EXECUTION_MODE after retire = %q, want SYNC", got)
	}
}

func TestDDLInTransactionSavepointHistoryKeepsPolicy(t *testing.T) {
	t.Parallel()
	h := newDDLTxnHarness(t)
	ctx := t.Context()
	mustExec(t, ctx, h.session, "SET CLI_SAVEPOINT_SUPPORT = 'ENABLED'")
	mustExec(t, ctx, h.session, "SET CLI_DDL_IN_TRANSACTION_MODE = 'ALLOW_IN_EMPTY_TRANSACTION'")
	mustExec(t, ctx, h.session, "BEGIN RW")
	mustExec(t, ctx, h.session, "SELECT 1")
	mustExec(t, ctx, h.session, "SAVEPOINT keep")
	mustExec(t, ctx, h.session, "SELECT 1")
	mustExec(t, ctx, h.session, "ROLLBACK TO SAVEPOINT keep")
	if !h.session.txn.HasUserWork() {
		t.Fatal("ROLLBACK TO must not clear hasUserWork")
	}
	if h.session.txn.effectiveDdlInTransactionMode() != enums.DdlInTransactionModeAllowInEmptyTransaction {
		t.Fatal("reconstruction must keep the captured policy")
	}
	_, err := execSQL(t, ctx, h.session, ddlInTxnSQL)
	if !errors.Is(err, errDDLInNonEmptyTransaction) {
		t.Fatalf("err=%v", err)
	}
	if h.adminCalled() {
		t.Fatal("history bit must keep ALLOW rejecting")
	}
}

func TestDDLInTransactionRecoveryRejectsWithoutAdmin(t *testing.T) {
	t.Parallel()
	h := newDDLTxnHarness(t)
	ctx := t.Context()
	mustExec(t, ctx, h.session, "SET CLI_SAVEPOINT_SUPPORT = 'ENABLED'")
	mustExec(t, ctx, h.session, "SET CLI_DDL_IN_TRANSACTION_MODE = 'AUTO_COMMIT_TRANSACTION'")
	mustExec(t, ctx, h.session, "BEGIN RW")
	mustExec(t, ctx, h.session, "SELECT 1")
	mustExec(t, ctx, h.session, "SAVEPOINT keep")
	h.hb.server.setFailSQL(status.Error(codes.Unknown, "injected user sql failure"))
	if _, err := execSQL(t, ctx, h.session, "SELECT 1"); err == nil {
		t.Fatal("expected injected SQL failure")
	}
	h.hb.server.setFailSQL(nil)
	if !h.session.txn.NeedsRecovery() {
		t.Fatal("owner should require SAVEPOINT recovery")
	}
	_, err := execSQL(t, ctx, h.session, ddlInTxnSQL)
	if !errors.Is(err, errSavepointRecovery) {
		t.Fatalf("err=%v, want recovery", err)
	}
	if h.adminCalled() {
		t.Fatal("recovery must not call Admin")
	}
}

func TestDDLInTransactionDeadlineDoesNotCancelAdminContext(t *testing.T) {
	t.Parallel()
	h := newDDLTxnHarness(t)
	ctx := t.Context()
	mustExec(t, ctx, h.session, "SET CLI_DDL_IN_TRANSACTION_MODE = 'AUTO_COMMIT_TRANSACTION'")
	mustExec(t, ctx, h.session, "SET TRANSACTION_TIMEOUT = '1h'")
	mustExec(t, ctx, h.session, "BEGIN RW")
	mustExec(t, ctx, h.session, ddlInTxnSQL)
	if !h.adminCalled() {
		t.Fatal("Admin must run on the remaining statement context")
	}
	if h.session.txn.InTransaction() {
		t.Fatal("owner retired before Admin")
	}
}

func TestCloseDoesNotAutoCommitForDDLMode(t *testing.T) {
	t.Parallel()
	h := newDDLTxnHarness(t)
	ctx := t.Context()
	mustExec(t, ctx, h.session, "SET CLI_DDL_IN_TRANSACTION_MODE = 'AUTO_COMMIT_TRANSACTION'")
	mustExec(t, ctx, h.session, "BEGIN RW")
	mustExec(t, ctx, h.session, "SELECT 1")
	h.session.Close()
	if len(h.hb.server.commits) != 0 {
		t.Fatalf("Close must not auto-commit, commits=%v", h.hb.server.commits)
	}
	if h.adminCalled() {
		t.Fatal("Close must not run DDL")
	}
}

func TestDDLInTransactionAllExecutionModesAfterAutoCommit(t *testing.T) {
	t.Parallel()
	for _, mode := range []enums.DDLExecutionMode{
		enums.DDLExecutionModeSync,
		enums.DDLExecutionModeAsync,
		enums.DDLExecutionModeAsyncWait,
	} {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()
			h := newDDLTxnHarness(t)
			h.session.systemVariables.Feature.DDLExecutionMode = mode
			if mode == enums.DDLExecutionModeAsyncWait {
				h.session.systemVariables.Feature.DDLAsyncWaitTimeout = time.Hour
			}
			ctx := t.Context()
			mustExec(t, ctx, h.session, "SET CLI_DDL_IN_TRANSACTION_MODE = 'AUTO_COMMIT_TRANSACTION'")
			mustExec(t, ctx, h.session, "BEGIN RW")
			mustExec(t, ctx, h.session, ddlInTxnSQL)
			if !h.adminCalled() {
				t.Fatal("Admin missing")
			}
			if len(h.hb.server.commits) != 1 {
				t.Fatalf("commits=%v", h.hb.server.commits)
			}
		})
	}
}
