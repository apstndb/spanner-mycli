// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mycli

import (
	"context"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/enums"
)

// ensureLazyAutocommitOwner installs a pending logical owner when AUTOCOMMIT
// is false, the session is idle, and stmt is eligible. Local admission runs
// first so a disabled or invalid SAVEPOINT cannot acquire an owner.
//
// After COMMIT/ROLLBACK/DDL teardown the session stays idle; the next eligible
// statement repeats this. #357 idle expiry, when it lands, must attach to this
// same owner identity — this function does not start an idle timer.
func (s *Session) ensureLazyAutocommitOwner(ctx context.Context, stmt Statement) error {
	if s == nil || s.systemVariables == nil || s.systemVariables.Transaction.Autocommit {
		return nil
	}
	if s.txn == nil || s.txn.InTransaction() {
		return nil
	}

	if err := admitLazyAutocommitStatement(s, stmt); err != nil {
		return err
	}
	if !lazyAutocommitEligible(s, stmt) {
		return nil
	}
	if _, ok := stmt.(MutationStatement); ok {
		if err := s.failStatementIfReadOnly(); err != nil {
			return err
		}
	}
	return s.txn.BeginPendingTransaction(ctx,
		sppb.TransactionOptions_ISOLATION_LEVEL_UNSPECIFIED,
		sppb.RequestOptions_PRIORITY_UNSPECIFIED)
}

func admitLazyAutocommitStatement(s *Session, stmt Statement) error {
	sp, ok := stmt.(*SavepointStatement)
	if !ok {
		return nil
	}
	if err := rejectSavepointCommand(s); err != nil {
		return err
	}
	if err := validateSavepointName(sp.Name); err != nil {
		return err
	}
	if s.systemVariables.Transaction.SavepointSupport != enums.SavepointSupportEnabled {
		return errSavepointDisabled
	}
	return nil
}

// lazyAutocommitEligible is a real-dispatch type switch, not MutationStatement.
// EXPLAIN PLAN (including DML), PDML/TRUNCATE, DDL, admin, partition, and
// inspection commands stay non-eligible so existing guards keep working.
func lazyAutocommitEligible(session *Session, stmt Statement) bool {
	switch s := stmt.(type) {
	case *SelectStatement:
		return !sessionQueryModeIsPlan(session) && !tryPartitionQuery(session)
	case *DmlStatement:
		if session.batch.IsActive() || tryPartitionQuery(session) || sessionQueryModeIsPlan(session) {
			return false
		}
		return true
	case *MutateStatement:
		return true
	case *BatchDMLStatement:
		return len(s.DMLs) > 0
	case *RunBatchStatement:
		return runBatchNeedsLazyAutocommitOwner(session)
	case *ExplainAnalyzeStatement, *ExplainAnalyzeDmlStatement:
		return true
	case *SavepointStatement:
		return true
	default:
		return false
	}
}

func sessionQueryModeIsPlan(session *Session) bool {
	if session == nil || session.systemVariables == nil {
		return false
	}
	qm := session.systemVariables.Query.QueryMode
	return qm != nil && *qm == sppb.ExecuteSqlRequest_PLAN
}

func tryPartitionQuery(session *Session) bool {
	return session != nil && session.systemVariables != nil && session.systemVariables.Query.TryPartitionQuery
}

func runBatchNeedsLazyAutocommitOwner(session *Session) bool {
	if session == nil || !session.batch.IsActive() {
		return false
	}
	b, ok := session.batch.Current().(*BatchDMLStatement)
	return ok && len(b.DMLs) > 0
}
