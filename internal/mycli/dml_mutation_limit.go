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
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/gsqlutils"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/googleapis/gax-go/v2/apierror"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Pinned Java TransactionMutationLimitExceededException classifier
// (googleapis/google-cloud-java@dd2c41a0). Do not use
// (*spanner.Error).GRPCStatus() / status.Convert(spannerErr) for Help:
// cloud.google.com/go/spanner v1.95.0 reconstructs status.New(code, desc)
// and drops details. Inspect the unwrapped *apierror.APIError instead.
const (
	mutationLimitSentence     = "The transaction contains too many mutations."
	mutationLimitHelpDesc     = "Cloud Spanner limits documentation."
	mutationLimitHelpURL      = "https://cloud.google.com/spanner/docs/limits"
	mutationLimitFallbackNote = "non-atomic mutation-limit fallback"
)

type dmlAttemptPhase int

const (
	dmlAttemptPhaseNone dmlAttemptPhase = iota
	dmlAttemptPhaseUserSQL
	dmlAttemptPhaseCommit
)

type rwTxAttemptInfo struct {
	phase    dmlAttemptPhase
	deadline time.Time
}

// mutationLimitFallbackAttempt is captured before the implicit owner is
// created so later retirement or SET LOCAL restore cannot re-read live
// session SQL, parameters, or request options.
type mutationLimitFallbackAttempt struct {
	stmt spanner.Statement
	opts spanner.QueryOptions
}

func isOrdinaryUpdateOrDelete(sql string) bool {
	token, err := gsqlutils.FirstNonHintToken("", sql)
	if err != nil {
		return false
	}
	return token.IsKeywordLike("UPDATE") || token.IsKeywordLike("DELETE")
}

// captureMutationLimitFallback records eligibility and frozen inputs before
// executeDML creates an implicit owner. A nil result means the statement is
// not a candidate; the original executeDML error is preserved.
func captureMutationLimitFallback(session *Session, sql string) *mutationLimitFallbackAttempt {
	if session == nil || session.systemVariables == nil || session.txn == nil {
		return nil
	}
	if session.systemVariables.Transaction.AutocommitDMLMode != enums.AutocommitDMLModeTransactionalWithFallbackToPartitionedNonAtomic {
		return nil
	}
	if session.batch.IsActive() {
		return nil
	}
	if session.txn.NeedsRecovery() {
		return nil
	}
	inTransaction, _ := session.txn.GetTransactionFlagsWithLock()
	if inTransaction || session.txn.InReadOnlyTransaction() || session.txn.InPendingTransaction() {
		return nil
	}
	if !isOrdinaryUpdateOrDelete(sql) || dmlHasReturningClause(sql) {
		return nil
	}
	stmt, err := newStatement(sql, session.systemVariables.Params, false)
	if err != nil {
		return nil
	}
	return &mutationLimitFallbackAttempt{
		stmt: cloneDMLStatement(stmt),
		opts: snapshotPDMLFallbackOptions(session),
	}
}

func cloneDMLStatement(stmt spanner.Statement) spanner.Statement {
	out := spanner.Statement{SQL: stmt.SQL}
	if len(stmt.Params) == 0 {
		return out
	}
	out.Params = make(map[string]interface{}, len(stmt.Params))
	for name, v := range stmt.Params {
		if gcv, ok := v.(spanner.GenericColumnValue); ok {
			out.Params[name] = cloneGenericColumnValue(gcv)
			continue
		}
		out.Params[name] = v
	}
	return out
}

// snapshotPDMLFallbackOptions freezes the request options PDML can actually
// send. LastStatement is implicit-transaction-only and must not leak into
// partitioned DML. Transaction tags are constructor/Begin options, not
// QueryOptions; executePartitionedUpdate does not attach them. Directed
// read, DataBoost, and query mode are not applied to this PDML helper
// (the existing PARTITIONED_NON_ATOMIC path also passes empty QueryOptions).
func snapshotPDMLFallbackOptions(session *Session) spanner.QueryOptions {
	sv := session.systemVariables
	opts := spanner.QueryOptions{
		Priority:   sv.Query.RPCPriority,
		RequestTag: sv.Transaction.RequestTag,
	}
	if sv.Query.OptimizerVersion != "" || sv.Query.OptimizerStatisticsPackage != "" {
		opts.Options = &sppb.ExecuteSqlRequest_QueryOptions{
			OptimizerVersion:           sv.Query.OptimizerVersion,
			OptimizerStatisticsPackage: sv.Query.OptimizerStatisticsPackage,
		}
	}
	return opts
}

func isMutationLimitExceeded(err error) bool {
	if err == nil {
		return false
	}
	if spanner.ErrCode(err) != codes.InvalidArgument {
		return false
	}
	desc := spanner.ErrDesc(err)
	if !strings.Contains(desc, mutationLimitSentence) {
		return false
	}
	// The weaker Java branch is message-only and is out of scope even when
	// it appears alongside other text without the required Help.
	var apiErr *apierror.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return hasExactMutationLimitHelp(status.Convert(apiErr))
}

func hasExactMutationLimitHelp(st *status.Status) bool {
	if st == nil {
		return false
	}
	var helpCount int
	var links []*errdetails.Help_Link
	for _, detail := range st.Details() {
		help, ok := detail.(*errdetails.Help)
		if !ok {
			continue
		}
		helpCount++
		links = append(links, help.GetLinks()...)
	}
	if helpCount != 1 || len(links) != 1 {
		return false
	}
	return links[0].GetDescription() == mutationLimitHelpDesc && links[0].GetUrl() == mutationLimitHelpURL
}

func finishMutationLimitFallback(ctx context.Context, session *Session, attempt *mutationLimitFallbackAttempt, result *Result, info rwTxAttemptInfo, err error) (*Result, error) {
	if err == nil || attempt == nil {
		return result, err
	}
	if info.phase != dmlAttemptPhaseUserSQL {
		return result, err
	}
	if !isMutationLimitExceeded(err) {
		return result, err
	}
	if session.txn != nil && (session.txn.InTransaction() || session.txn.NeedsRecovery()) {
		return result, err
	}
	if session.txn != nil && session.txn.afterMutationLimitBeforeFallback != nil {
		session.txn.afterMutationLimitBeforeFallback()
	}
	if ctx.Err() != nil {
		return nil, fmt.Errorf("transactional DML failed with mutation limit (%w); partitioned DML fallback skipped: %w", err, ctx.Err())
	}
	fallbackCtx, cancel, budgetErr := applyCapturedTransactionDeadline(ctx, session, info.deadline)
	defer cancel()
	if budgetErr != nil {
		return nil, fmt.Errorf("transactional DML failed with mutation limit (%w); partitioned DML fallback skipped: %w", err, budgetErr)
	}
	pdml, pdmlErr := executePartitionedUpdate(fallbackCtx, session, attempt.stmt, attempt.opts)
	if pdmlErr != nil {
		return nil, wrapMutationLimitFallbackError(err, pdmlErr)
	}
	pdml.MutationLimitFallback = true
	return pdml, nil
}

func applyCapturedTransactionDeadline(ctx context.Context, session *Session, deadline time.Time) (context.Context, context.CancelFunc, error) {
	if deadline.IsZero() {
		return ctx, func() {}, nil
	}
	now := time.Now()
	if session != nil && session.txn != nil {
		now = session.txn.now()
	}
	if !deadline.After(now) {
		return ctx, func() {}, errTransactionTimeout
	}
	ctx, cancel := context.WithDeadline(ctx, deadline)
	return ctx, cancel, nil
}

func wrapMutationLimitFallbackError(orig, fallback error) error {
	return fmt.Errorf("transactional DML failed with mutation limit (%w); partitioned DML fallback also failed (partitioned DML may have partially committed; that phase is not rolled back): %w", orig, fallback)
}
