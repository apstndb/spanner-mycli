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
	"slices"
	"strings"
	"text/template"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/lox"
	"github.com/apstndb/spannerplanviz/plantree"
	"github.com/apstndb/spannerplanviz/queryplan"
	"github.com/ngicks/go-iterator-helper/x/exp/xiter"
	"github.com/olekukonko/tablewriter/tw"
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
		TableHeader:  toTableHeader(describeColumnNames),
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

	rows, predicates, err := processPlanWithoutStats(queryPlan, session.systemVariables.SpannerCLICompatiblePlan)
	if err != nil {
		return nil, err
	}

	result := &Result{
		TableHeader:  toTableHeader(explainColumnNames),
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

	// Cloud Spanner Emulator doesn't set query plan nodes to the result.
	// See: https://github.com/GoogleCloudPlatform/cloud-spanner-emulator/blob/77188b228e7757cd56ecffb5bc3ee85dce5d6ae1/frontend/handlers/queries.cc#L224-L230
	if plan == nil {
		return nil, errors.New("query plan is not available. EXPLAIN ANALYZE statement is not supported for Cloud Spanner Emulator.")
	}

	result, err := generateExplainAnalyzeResult(session.systemVariables, plan, stats)
	if err != nil {
		return nil, err
	}

	result.Timestamp = lox.IfOrEmptyF(roTxn != nil, func() time.Time { return ignoreError(roTxn.Timestamp()) })
	return result, nil
}

func generateExplainAnalyzeResult(sysVars *systemVariables, plan *sppb.QueryPlan, stats map[string]interface{}) (*Result, error) {
	def := sysVars.ParsedAnalyzeColumns

	rows, predicates, err := processPlan(plan, def, sysVars.SpannerCLICompatiblePlan)
	if err != nil {
		return nil, fmt.Errorf("failed to process query plan: %w", err)
	}

	columnNames, columnAlign := explainAnalyzeHeader(def)

	queryStats, err := parseQueryStats(stats)
	if err != nil {
		return nil, fmt.Errorf("failed to parse query stats: %w", err)
	}

	var lintResults []string
	if sysVars.LintPlan {
		lintResults = lintPlan(plan)
	}

	// ReadOnlyTransaction.Timestamp() is invalid until read.
	result := &Result{
		TableHeader:  toTableHeader(columnNames),
		ColumnAlign:  columnAlign,
		ForceVerbose: true,
		AffectedRows: len(rows),
		Stats:        queryStats,
		Rows:         rows,
		Predicates:   predicates,
		LintResults:  lintResults,
	}
	return result, nil
}

func explainAnalyzeHeader(def []columnRenderDef) ([]string, []tw.Align) {
	// Start with the base columns and alignments for EXPLAIN output.
	baseNames := explainColumnNames
	baseAlign := explainColumnAlign

	// Extract the names and alignments from the custom column definitions.
	customNames := slices.Collect(xiter.Map(func(d columnRenderDef) string { return d.Name }, slices.Values(def)))
	customAligns := slices.Collect(xiter.Map(func(d columnRenderDef) tw.Align { return d.Alignment }, slices.Values(def)))

	// Concatenate the base and custom parts.
	columnNames := slices.Concat(baseNames, customNames)
	columnAlign := slices.Concat(baseAlign, customAligns)

	return columnNames, columnAlign
}

func executeExplainAnalyzeDML(ctx context.Context, session *Session, sql string) (*Result, error) {
	stmt, err := newStatement(sql, session.systemVariables.Params, false)
	if err != nil {
		return nil, err
	}

	var queryStats map[string]any
	affectedRows, commitResp, queryPlan, _, err := session.RunInNewOrExistRwTx(ctx, func(implicit bool) (int64, *sppb.QueryPlan, *sppb.ResultSetMetadata, error) {
		iter, _ := session.RunQueryWithStats(ctx, stmt, implicit)
		qs, count, metadata, plan, err := consumeRowIterDiscard(iter)
		queryStats = qs
		return count, plan, metadata, err
	})
	if err != nil {
		return nil, err
	}

	result, err := generateExplainAnalyzeResult(session.systemVariables, queryPlan, queryStats)
	if err != nil {
		return nil, err
	}

	result.IsMutation = true
	result.AffectedRows = int(affectedRows)
	result.AffectedRowsType = rowCountTypeExact
	result.Timestamp = commitResp.CommitTs

	return result, nil
}

func processPlanWithoutStats(plan *sppb.QueryPlan, spannerCLICompatible bool) (rows []Row, predicates []string, err error) {
	return processPlan(plan, nil, spannerCLICompatible)
}

func processPlan(plan *sppb.QueryPlan, columnRenderDefs []columnRenderDef, spannerCLICompatible bool) (rows []Row, predicates []string, err error) {
	rowsWithPredicates, err := processPlanNodes(plan.GetPlanNodes(), spannerCLICompatible)
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
		rowStrs := []string{row.FormatID(), row.Text()}
		for _, colRender := range columnRenderDefs {
			c, err := colRender.MapFunc(row)
			if err != nil {
				return nil, nil, err
			}

			rowStrs = append(rowStrs, c)
		}
		rows = append(rows, rowStrs)

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

type columnRenderDef struct {
	MapFunc   func(row plantree.RowWithPredicates) (string, error)
	Name      string
	Alignment tw.Align
}

func templateMapFunc(tmplName, tmplText string) (func(row plantree.RowWithPredicates) (string, error), error) {
	tmpl, err := template.New(tmplName).Parse(tmplText)
	if err != nil {
		return nil, err
	}

	return func(row plantree.RowWithPredicates) (string, error) {
		var sb strings.Builder
		if err = tmpl.Execute(&sb, row.ExecutionStats); err != nil {
			return "", err
		}

		return sb.String(), nil
	}, nil
}

func parseAlignment(s string) (tw.Align, error) {
	switch strings.TrimPrefix(s, "ALIGN_") {
	case "RIGHT":
		return tw.AlignRight, nil
	case "LEFT":
		return tw.AlignLeft, nil
	case "CENTER":
		return tw.AlignCenter, nil
	case "NONE":
		return tw.AlignNone, nil
	case "DEFAULT":
		return tw.AlignDefault, nil
	default:
		return "", fmt.Errorf("unknown Alignment: %s", s)
	}
}

func customListToTableRenderDef(custom []string) ([]columnRenderDef, error) {
	var columns []columnRenderDef
	for _, s := range custom {
		split := strings.SplitN(s, ":", 3)

		var align tw.Align
		switch len(split) {
		case 2:
			align = tw.AlignRight
		case 3:
			alignStr := split[2]
			var err error
			align, err = parseAlignment(alignStr)
			if err != nil {
				return nil, fmt.Errorf("failed to parseAlignment(): %w", err)
			}
		default:
			return nil, fmt.Errorf(`invalid format: must be "<name>:<template>[:<alignment>]", but: %v`, s)
		}

		name, templateStr := split[0], split[1]
		mapFunc, err := templateMapFunc(name, templateStr)
		if err != nil {
			return nil, err
		}

		columns = append(columns, columnRenderDef{
			MapFunc:   mapFunc,
			Name:      name,
			Alignment: align,
		})
	}

	return columns, nil
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

func processPlanNodes(nodes []*sppb.PlanNode, spannerCLICompatible bool) ([]plantree.RowWithPredicates, error) {
	var options []plantree.Option
	if !spannerCLICompatible {
		options = append(options, plantree.WithQueryPlanOptions(
			queryplan.WithExecutionMethodFormat(queryplan.ExecutionMethodFormatAngle),
			queryplan.WithFullScanFormat(queryplan.FullScanFormatLabel),
			queryplan.WithTargetMetadataFormat(queryplan.TargetMetadataFormatOn),
		))
	}

	return plantree.ProcessPlan(queryplan.New(nodes), options...)
}
