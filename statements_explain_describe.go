// Copyright 2020 Google LLC
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
package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/lox"
	"github.com/apstndb/spannerplanviz/plantree"
	"github.com/apstndb/spannerplanviz/queryplan"
)

type ExplainStatement struct {
	Explain string
	IsDML   bool
}

// Execute processes `EXPLAIN` statement for queries and DMLs.
func (s *ExplainStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	return executeExplain(ctx, session, s.Explain, s.IsDML)
}

type ExplainAnalyzeStatement struct {
	Query string
}

func (s *ExplainAnalyzeStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	sql := s.Query

	return executeExplainAnalyze(ctx, session, sql)
}

type ExplainAnalyzeDmlStatement struct {
	Dml string
}

func (ExplainAnalyzeDmlStatement) isMutationStatement() {}

func (s *ExplainAnalyzeDmlStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	return executeExplainAnalyzeDML(ctx, session, s.Dml)
}

type DescribeStatement struct {
	Statement string
	IsDML     bool
}

// Execute processes `DESCRIBE` statement for queries and DMLs.
func (s *DescribeStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	stmt, err := newStatement(s.Statement, session.systemVariables.Params, true)
	if err != nil {
		return nil, err
	}

	_, timestamp, metadata, err := runAnalyzeQuery(ctx, session, stmt, s.IsDML)
	if err != nil {
		return nil, err
	}

	var rows []Row
	for _, field := range metadata.GetRowType().GetFields() {
		rows = append(rows, toRow(field.GetName(), formatTypeVerbose(field.GetType())))
	}

	result := &Result{
		AffectedRows: len(rows),
		ColumnNames:  describeColumnNames,
		Timestamp:    timestamp,
		Rows:         rows,
	}

	return result, nil
}
func executeExplain(ctx context.Context, session *Session, sql string, isDML bool) (*Result, error) {
	stmt, err := newStatement(sql, session.systemVariables.Params, true)
	if err != nil {
		return nil, err
	}

	queryPlan, timestamp, _, err := runAnalyzeQuery(ctx, session, stmt, isDML)
	if err != nil {
		return nil, err
	}

	if queryPlan == nil {
		return nil, errors.New("EXPLAIN statement is not supported for Cloud Spanner Emulator.")
	}

	rows, predicates, err := processPlanWithoutStats(queryPlan, session.systemVariables.ExecutionMethodFormat)
	if err != nil {
		return nil, err
	}

	result := &Result{
		ColumnNames:  explainColumnNames,
		ColumnAlign:  explainColumnAlign,
		AffectedRows: len(rows),
		Rows:         rows,
		Timestamp:    timestamp,
		Predicates:   predicates,
		LintResults:  lox.IfOrEmptyF(session.systemVariables.LintPlan, func() []string { return lintPlan(queryPlan) }),
	}

	return result, nil
}

func executeExplainAnalyze(ctx context.Context, session *Session, sql string) (*Result, error) {
	stmt, err := newStatement(sql, session.systemVariables.Params, false)
	if err != nil {
		return nil, err
	}

	iter, roTxn := session.RunQueryWithStats(ctx, stmt, false)

	stats, _, _, plan, err := consumeRowIterDiscard(iter)
	if err != nil {
		return nil, err
	}

	queryStats, err := parseQueryStats(stats)
	if err != nil {
		return nil, err
	}

	// Cloud Spanner Emulator doesn't set query plan nodes to the result.
	// See: https://github.com/GoogleCloudPlatform/cloud-spanner-emulator/blob/77188b228e7757cd56ecffb5bc3ee85dce5d6ae1/frontend/handlers/queries.cc#L224-L230
	if plan == nil {
		return nil, errors.New("query plan is not available. EXPLAIN ANALYZE statement is not supported for Cloud Spanner Emulator.")
	}

	rows, predicates, err := processPlanWithStats(plan, session.systemVariables.ExecutionMethodFormat)
	if err != nil {
		return nil, err
	}

	// ReadOnlyTransaction.Timestamp() is invalid until read.
	result := &Result{
		ColumnNames:  explainAnalyzeColumnNames,
		ColumnAlign:  explainAnalyzeColumnAlign,
		ForceVerbose: true,
		AffectedRows: len(rows),
		Stats:        queryStats,
		Timestamp:    lox.IfOrEmptyF(roTxn != nil, func() time.Time { return ignoreError(roTxn.Timestamp()) }),
		Rows:         rows,
		Predicates:   predicates,
		LintResults:  lox.IfOrEmptyF(session.systemVariables.LintPlan, func() []string { return lintPlan(plan) }),
	}
	return result, nil
}

func executeExplainAnalyzeDML(ctx context.Context, session *Session, sql string) (*Result, error) {
	stmt, err := newStatement(sql, session.systemVariables.Params, false)
	if err != nil {
		return nil, err
	}

	affectedRows, commitResp, queryPlan, _, err := session.RunInNewOrExistRwTx(ctx, func(implicit bool) (int64, *sppb.QueryPlan, *sppb.ResultSetMetadata, error) {
		iter, _ := session.RunQueryWithStats(ctx, stmt, implicit)
		_, count, metadata, plan, err := consumeRowIterDiscard(iter)
		return count, plan, metadata, err
	})
	if err != nil {
		return nil, err
	}

	rows, predicates, err := processPlanWithStats(queryPlan, session.systemVariables.ExecutionMethodFormat)
	if err != nil {
		return nil, err
	}

	result := &Result{
		IsMutation:       true,
		ColumnNames:      explainAnalyzeColumnNames,
		ForceVerbose:     true,
		AffectedRows:     int(affectedRows),
		AffectedRowsType: rowCountTypeExact,
		Rows:             rows,
		Predicates:       predicates,
		Timestamp:        commitResp.CommitTs,
		LintResults:      lox.IfOrEmptyF(session.systemVariables.LintPlan, func() []string { return lintPlan(queryPlan) }),
	}

	return result, nil
}

func processPlanWithStats(plan *sppb.QueryPlan, format queryplan.ExecutionMethodFormat) (rows []Row, predicates []string, err error) {
	return processPlanImpl(plan, true, format)
}

func processPlanWithoutStats(plan *sppb.QueryPlan, format queryplan.ExecutionMethodFormat) (rows []Row, predicates []string, err error) {
	return processPlanImpl(plan, false, format)
}

func processPlanImpl(plan *sppb.QueryPlan, withStats bool, format queryplan.ExecutionMethodFormat) (rows []Row, predicates []string, err error) {
	rowsWithPredicates, err := processPlanNodes(plan.GetPlanNodes(), format)
	if err != nil {
		return nil, nil, err
	}

	var maxIDLength int
	for _, row := range rowsWithPredicates {
		if length := len(fmt.Sprint(row.ID)); length > maxIDLength {
			maxIDLength = length
		}
	}

	for _, row := range rowsWithPredicates {
		if withStats {
			rows = append(rows, toRow(row.FormatID(), row.Text(), row.ExecutionStats.Rows.Total, row.ExecutionStats.ExecutionSummary.NumExecutions, row.ExecutionStats.Latency.String()))
		} else {
			rows = append(rows, toRow(row.FormatID(), row.Text()))
		}

		var prefix string
		for i, predicate := range row.Predicates {
			if i == 0 {
				prefix = fmt.Sprintf("%*d:", maxIDLength, row.ID)
			} else {
				prefix = strings.Repeat(" ", maxIDLength+1)
			}
			predicates = append(predicates, fmt.Sprintf("%s %s", prefix, predicate))
		}
	}

	return rows, predicates, nil
}

func runAnalyzeQuery(ctx context.Context, session *Session, stmt spanner.Statement, isDML bool) (queryPlan *sppb.QueryPlan, commitTimestamp time.Time, metadata *sppb.ResultSetMetadata, err error) {
	if !isDML {
		queryPlan, metadata, err := session.RunAnalyzeQuery(ctx, stmt)
		return queryPlan, time.Time{}, metadata, err
	}

	_, commitResp, queryPlan, metadata, err := session.RunInNewOrExistRwTx(ctx, func(implicit bool) (int64, *sppb.QueryPlan, *sppb.ResultSetMetadata, error) {
		plan, metadata, err := session.RunAnalyzeQuery(ctx, stmt)
		return 0, plan, metadata, err
	})
	return queryPlan, commitResp.CommitTs, metadata, err
}

func processPlanNodes(nodes []*sppb.PlanNode, format queryplan.ExecutionMethodFormat) ([]plantree.RowWithPredicates, error) {
	return plantree.ProcessPlan(queryplan.New(nodes),
		plantree.WithQueryPlanOptions(queryplan.WithExecutionMethodFormat(format)))
}
