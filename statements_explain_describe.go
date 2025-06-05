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
	"log/slog"
	"slices"
	"strings"
	"text/template"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/lox"
	"github.com/apstndb/spannerplan"
	"github.com/apstndb/spannerplan/plantree"
	spstats "github.com/apstndb/spannerplan/stats"
	"github.com/ngicks/go-iterator-helper/x/exp/xiter"
	"github.com/olekukonko/tablewriter/tw"
	"github.com/samber/lo"
)

type ExplainStatement struct {
	Explain string
	IsDML   bool
	Format  explainFormat
	Width   int64
}

// Execute processes `EXPLAIN` statement for queries and DMLs.
func (s *ExplainStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	return executeExplain(ctx, session, s.Explain, s.IsDML, s.Format, s.Width)
}

type ExplainAnalyzeStatement struct {
	Query  string
	Format explainFormat
	Width  int64
}

func (s *ExplainAnalyzeStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	sql := s.Query

	return executeExplainAnalyze(ctx, session, sql, s.Format, s.Width)
}

type ExplainAnalyzeDmlStatement struct {
	Dml    string
	Format explainFormat
	Width  int64
}

func (ExplainAnalyzeDmlStatement) isMutationStatement() {}

func (s *ExplainAnalyzeDmlStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	return executeExplainAnalyzeDML(ctx, session, s.Dml, s.Format, s.Width)
}

type ExplainLastQueryStatement struct {
	Analyze bool
	Format  explainFormat
	Width   int64
}

func (s *ExplainLastQueryStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if session.systemVariables.LastQueryCache == nil {
		return nil, fmt.Errorf("last query cache missing. Have you run a query")
	}

	if session.systemVariables.LastQueryCache.QueryPlan == nil || len(session.systemVariables.LastQueryCache.QueryPlan.GetPlanNodes()) == 0 {
		return nil, fmt.Errorf("missing last query plan. If you use cloud-spanner-emulator, it doesn't support EXPLAIN and EXPLAIN ANALYZE")
	}

	var err error
	var result *Result
	if s.Analyze {
		result, err = generateExplainAnalyzeResult(session.systemVariables,
			session.systemVariables.LastQueryCache.QueryPlan,
			session.systemVariables.LastQueryCache.QueryStats,
			s.Format, s.Width)
	} else {
		result, err = generateExplainResult(session.systemVariables,
			session.systemVariables.LastQueryCache.QueryPlan, s.Format, s.Width)
	}

	if err != nil {
		return nil, err
	}

	result.Timestamp = session.systemVariables.LastQueryCache.Timestamp
	return result, nil
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

func executeExplain(ctx context.Context, session *Session, sql string, isDML bool, format explainFormat, width int64) (*Result, error) {
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

	result, err := generateExplainResult(session.systemVariables, queryPlan, format, width)
	if err != nil {
		return nil, err
	}

	result.Timestamp = timestamp

	return result, nil
}

func generateExplainResult(sysVars *systemVariables, queryPlan *sppb.QueryPlan, format explainFormat, width int64) (*Result, error) {
	format = lo.Ternary(format != explainFormatUnspecified, format, sysVars.ExplainFormat)
	width = lo.Ternary(width != 0, width, sysVars.ExplainWrapWidth)
	rows, predicates, err := processPlanWithoutStats(queryPlan, format, width)
	if err != nil {
		return nil, err
	}

	result := &Result{
		TableHeader:  toTableHeader(explainColumnNames),
		ColumnAlign:  explainColumnAlign,
		AffectedRows: len(rows),
		Rows:         rows,
		Predicates:   predicates,
		LintResults:  lox.IfOrEmptyF(sysVars.LintPlan, func() []string { return lintPlan(queryPlan) }),
	}
	return result, nil
}

func executeExplainAnalyze(ctx context.Context, session *Session, sql string, format explainFormat, width int64) (*Result, error) {
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

	result, err := generateExplainAnalyzeResult(session.systemVariables, plan, stats, format, width)
	if err != nil {
		return nil, err
	}

	result.Timestamp = lox.IfOrEmptyF(roTxn != nil, func() time.Time { return ignoreError(roTxn.Timestamp()) })

	session.systemVariables.LastQueryCache = &LastQueryCache{
		QueryPlan:  plan,
		QueryStats: stats,
		Timestamp:  result.Timestamp,
	}

	return result, nil
}

func generateExplainAnalyzeResult(sysVars *systemVariables, plan *sppb.QueryPlan, stats map[string]interface{},
	format explainFormat, width int64) (*Result, error) {
	def := sysVars.ParsedAnalyzeColumns
	inlines := sysVars.ParsedInlineStats
	format = lo.Ternary(format != explainFormatUnspecified, format, sysVars.ExplainFormat)
	width = lo.Ternary(width != 0, width, sysVars.ExplainWrapWidth)

	rows, predicates, err := processPlan(plan, def, inlines, format, width)
	if err != nil {
		return nil, fmt.Errorf("failed to process query plan: %w", err)
	}

	columnNames, columnAlign := explainAnalyzeHeader(def, width)

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

func explainAnalyzeHeader(def []columnRenderDef, width int64) ([]string, []tw.Align) {
	// Start with the base columns and alignments for EXPLAIN output.
	baseNames := lo.Ternary(width == 0 || width >= operatorColumnNameLength, explainColumnNames, explainColumnNamesShort)
	baseAlign := explainColumnAlign

	// Extract the names and alignments from the custom column definitions.
	customNames := slices.Collect(xiter.Map(func(d columnRenderDef) string { return d.Name }, slices.Values(def)))
	customAligns := slices.Collect(xiter.Map(func(d columnRenderDef) tw.Align { return d.Alignment }, slices.Values(def)))

	// Concatenate the base and custom parts.
	columnNames := slices.Concat(baseNames, customNames)
	columnAlign := slices.Concat(baseAlign, customAligns)

	return columnNames, columnAlign
}

func executeExplainAnalyzeDML(ctx context.Context, session *Session, sql string, format explainFormat, width int64) (*Result, error) {
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

	result, err := generateExplainAnalyzeResult(session.systemVariables, queryPlan, queryStats, format, width)
	if err != nil {
		return nil, err
	}

	result.IsMutation = true
	result.AffectedRows = int(affectedRows)
	result.AffectedRowsType = rowCountTypeExact
	result.Timestamp = commitResp.CommitTs

	return result, nil
}

func processPlanWithoutStats(plan *sppb.QueryPlan, format explainFormat, width int64) (rows []Row, predicates []string, err error) {
	return processPlan(plan, nil, nil, format, width)
}

func processPlan(plan *sppb.QueryPlan, columnRenderDefs []columnRenderDef, inlineStatsDefs []inlineStatsDef, format explainFormat, width int64) (rows []Row, predicates []string, err error) {
	rowsWithPredicates, err := processPlanNodes(plan.GetPlanNodes(), inlineStatsDefs, format, width)
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

type inlineStatsDef struct {
	MapFunc func(row plantree.RowWithPredicates) (string, error)
	Name    string
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

func parseInlineStatsDefs(input string) ([]inlineStatsDef, error) {
	var columns []inlineStatsDef
	for part := range strings.SplitSeq(input, ",") {
		name, templateStr, found := strings.Cut(part, ":")
		if !found {
			return nil, fmt.Errorf(`invalid inline stats format: must be "<name>:<template>", but: %v`, part)
		}

		mapFunc, err := templateMapFunc(name, templateStr)
		if err != nil {
			return nil, err
		}

		columns = append(columns, inlineStatsDef{
			MapFunc: mapFunc,
			Name:    name,
		})
	}

	return columns, nil
}

func customListToTableRenderDefs(custom string) ([]columnRenderDef, error) {
	var columns []columnRenderDef
	for part := range strings.SplitSeq(custom, ",") {
		split := strings.SplitN(part, ":", 3)

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
			return nil, fmt.Errorf(`invalid format: must be "<name>:<template>[:<alignment>]", but: %v`, part)
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

func processPlanNodes(nodes []*sppb.PlanNode, statsDefs []inlineStatsDef, format explainFormat, width int64) ([]plantree.RowWithPredicates, error) {
	var options []plantree.Option
	switch format {
	case explainFormatCurrent, explainFormatUnspecified:
		options = append(options, plantree.WithQueryPlanOptions(
			spannerplan.WithExecutionMethodFormat(spannerplan.ExecutionMethodFormatAngle),
			spannerplan.WithKnownFlagFormat(spannerplan.KnownFlagFormatLabel),
			spannerplan.WithTargetMetadataFormat(spannerplan.TargetMetadataFormatOn),
		))
	case explainFormatCompact:
		options = append(options,
			plantree.EnableCompact(),
			plantree.WithQueryPlanOptions(
				spannerplan.WithExecutionMethodFormat(spannerplan.ExecutionMethodFormatAngle),
				spannerplan.WithKnownFlagFormat(spannerplan.KnownFlagFormatLabel),
				spannerplan.WithTargetMetadataFormat(spannerplan.TargetMetadataFormatOn),
			))
	case explainFormatTraditional:
		// nop because it is default bformat in plantree.ProcessPlan.
	}

	if width > 0 {
		options = append(options, plantree.WithWrapWidth(int(width)))
	}

	if len(statsDefs) > 0 {
		options = append(options, plantree.WithQueryPlanOptions(
			spannerplan.WithInlineStatsFunc(inlineStatsFunc(statsDefs)),
		))
	}

	qp, err := spannerplan.New(nodes)
	if err != nil {
		return nil, err
	}

	return plantree.ProcessPlan(qp, options...)
}

func inlineStatsFunc(defs []inlineStatsDef) func(*sppb.PlanNode) []string {
	return func(node *sppb.PlanNode) []string {
		extracted, err := spstats.Extract(node, false)
		if err != nil {
			slog.Warn("failed on extract inline stats", "node_id", node.GetIndex(), "err", err)
			return nil
		}

		if extracted == nil {
			return nil
		}

		row := plantree.RowWithPredicates{ExecutionStats: *extracted}

		var result []string
		for _, def := range defs {
			v, err := def.MapFunc(row)
			if err != nil {
				slog.Warn("failed to execute inline stats template", "name", def.Name, "node_id", node.GetIndex(), "err", err)
				continue
			}

			if v != "" {
				result = append(result, fmt.Sprintf("%s=%s", def.Name, v))
			}
		}
		return result
	}
}
