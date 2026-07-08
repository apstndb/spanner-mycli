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
package mycli

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
	"github.com/apstndb/protoyaml"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanner-mycli/internal/mycli/decoder"
	"github.com/apstndb/spannerplan"
	"github.com/apstndb/spannerplan/plantree"
	planref "github.com/apstndb/spannerplan/plantree/reference"
	spstats "github.com/apstndb/spannerplan/stats"
	"github.com/olekukonko/tablewriter/tw"
	"github.com/samber/lo"
	loi "github.com/samber/lo/it"
)

type ExplainStatement struct {
	Explain       string
	IsDML         bool // Whether the statement being explained is a DML
	Format        enums.ExplainFormat
	Width         int64
	PrintSections *planref.PrintSections
}

func (s *ExplainStatement) String() string {
	// Reconstruct the full EXPLAIN statement
	return "EXPLAIN " + s.Explain
}

// Execute processes `EXPLAIN` statement for queries and DMLs.
func (s *ExplainStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	return executeExplain(ctx, session, s.Explain, s.IsDML, s.Format, s.Width, s.PrintSections)
}

type ExplainAnalyzeStatement struct {
	Query         string
	Format        enums.ExplainFormat
	Width         int64
	PrintSections *planref.PrintSections
}

func (s *ExplainAnalyzeStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	sql := s.Query

	return executeExplainAnalyze(ctx, session, sql, s.Format, s.Width, s.PrintSections)
}

type ExplainAnalyzeDmlStatement struct {
	Dml           string
	Format        enums.ExplainFormat
	Width         int64
	PrintSections *planref.PrintSections
}

func (ExplainAnalyzeDmlStatement) isMutationStatement() {}

func (s *ExplainAnalyzeDmlStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	return executeExplainAnalyzeDML(ctx, session, s.Dml, s.Format, s.Width, s.PrintSections)
}

type ExplainLastQueryStatement struct {
	Analyze       bool
	Format        enums.ExplainFormat
	Width         int64
	PrintSections *planref.PrintSections
}

func (s *ExplainLastQueryStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if session.systemVariables.LastResult.QueryCache == nil {
		return nil, fmt.Errorf("last query cache missing because query not executed")
	}

	if session.systemVariables.LastResult.QueryCache.QueryPlan == nil || len(session.systemVariables.LastResult.QueryCache.QueryPlan.GetPlanNodes()) == 0 {
		return nil, fmt.Errorf("missing last query plan. This may happen if the Cloud Spanner Emulator is used, as it may not fully support EXPLAIN and EXPLAIN ANALYZE features")
	}

	var err error
	var result *Result
	if s.Analyze {
		result, err = generateExplainAnalyzeResult(session.systemVariables,
			session.systemVariables.LastResult.QueryCache.QueryPlan,
			session.systemVariables.LastResult.QueryCache.QueryStats,
			s.Format, s.Width, s.PrintSections)
	} else {
		result, err = generateExplainResult(session.systemVariables,
			session.systemVariables.LastResult.QueryCache.QueryPlan, s.Format, s.Width, s.PrintSections)
	}

	if err != nil {
		return nil, err
	}

	// Restore the appropriate timestamp from cache
	result.ReadTimestamp = session.systemVariables.LastResult.QueryCache.ReadTimestamp
	result.CommitTimestamp = session.systemVariables.LastResult.QueryCache.CommitTimestamp
	return result, nil
}

type ShowPlanNodeStatement struct {
	NodeID int
}

func (s *ShowPlanNodeStatement) Execute(ctx context.Context, session *Session) (*Result, error) {
	if session.systemVariables.LastResult.QueryCache == nil || session.systemVariables.LastResult.QueryCache.QueryPlan == nil {
		return nil, errors.New("no query plan cached. Run query or EXPLAIN ANALYZE first")
	}

	planNodes := session.systemVariables.LastResult.QueryCache.QueryPlan.GetPlanNodes()
	if s.NodeID >= len(planNodes) {
		return nil, fmt.Errorf("node with ID %d not found in the cached query plan", s.NodeID)
	}

	planNode := planNodes[s.NodeID]
	y, err := protoyaml.Marshal(planNode, protoyaml.WithFlowLeafCollections())
	if err != nil {
		return nil, err
	}

	// Computing incoming parent links requires reconstructing the whole plan,
	// which can fail on a malformed or unexpected cached plan shape. SHOW PLAN
	// NODE is a debugging command whose primary value is dumping the raw node
	// YAML (already marshaled above), so degrade gracefully here: keep showing
	// the node content and replace the parent-links row with a short note
	// instead of failing the whole statement.
	var parentLinksInfo string
	if queryPlan, err := spannerplan.New(planNodes); err != nil {
		parentLinksInfo = fmt.Sprintf("parent links unavailable: %v", err)
	} else {
		parentLinksInfo = formatShowPlanNodeIncomingLinks(queryPlan.ParentLinks(int32(s.NodeID)))
	}

	return &Result{
		TableHeader:  toTableHeader(fmt.Sprintf("Content of Node %v", s.NodeID)),
		Rows:         sliceOf(toRow(string(y)), toRow(parentLinksInfo)),
		AffectedRows: 2,
	}, nil
}

func formatShowPlanNodeIncomingLinks(links []spannerplan.ResolvedParentLink) string {
	if len(links) == 0 {
		return "Incoming Parent Links:\n  - No incoming parent links"
	}

	var lines []string
	lines = append(lines, "Incoming Parent Links:")

	for _, link := range links {
		childType := link.ChildLink.GetType()
		if childType == "" {
			childType = "(not set)"
		}

		parentNodeIndex := -1
		parentTitle := "unknown"
		if link.Parent != nil {
			parentNodeIndex = int(link.Parent.GetIndex())
			parentTitle = spannerplan.NodeTitle(link.Parent)
		}

		lines = append(lines, fmt.Sprintf("  - Parent Node Index: %d", parentNodeIndex))
		lines = append(lines, fmt.Sprintf("    Parent Node Title: %s", parentTitle))
		lines = append(lines, fmt.Sprintf("    Child Link Type: %s", childType))
		if link.ChildLink.GetVariable() != "" {
			lines = append(lines, fmt.Sprintf("    Variable: %s", link.ChildLink.GetVariable()))
		}
	}

	return strings.Join(lines, "\n")
}

type DescribeStatement struct {
	Statement string
	IsDML     bool // Whether the statement being described is a DML
}

func (s *DescribeStatement) String() string {
	// Reconstruct the full DESCRIBE statement
	return "DESCRIBE " + s.Statement
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
		rows = append(rows, toRow(field.GetName(), decoder.FormatTypeVerbose(field.GetType())))
	}

	result := &Result{
		AffectedRows:  len(rows),
		TableHeader:   toTableHeader(describeColumnNames),
		ReadTimestamp: timestamp,
		Rows:          rows,
	}

	return result, nil
}

func executeExplain(ctx context.Context, session *Session, sql string, isDML bool, format enums.ExplainFormat, width int64, printSections *planref.PrintSections) (*Result, error) {
	stmt, err := newStatement(sql, session.systemVariables.Params, true)
	if err != nil {
		return nil, err
	}

	queryPlan, timestamp, _, err := runAnalyzeQuery(ctx, session, stmt, isDML)
	if err != nil {
		return nil, err
	}

	if queryPlan == nil {
		return nil, errors.New("EXPLAIN statement is not supported for Cloud Spanner Emulator")
	}

	result, err := generateExplainResult(session.systemVariables, queryPlan, format, width, printSections)
	if err != nil {
		return nil, err
	}

	result.ReadTimestamp = timestamp
	// EXPLAIN doesn't execute the statement, so it's not actually DML even if explaining DML

	return result, nil
}

func generateExplainResult(sysVars *systemVariables, queryPlan *sppb.QueryPlan, format enums.ExplainFormat, width int64, printSections *planref.PrintSections) (*Result, error) {
	format = lo.Ternary(format != enums.ExplainFormatUnspecified, format, sysVars.Display.ExplainFormat)
	width = lo.Ternary(width != 0, width, sysVars.Display.ExplainWrapWidth)
	rows, predicates, appendices, err := processPlanWithoutStats(queryPlan, format, width, sysVars.Display.ExplainHangingIndent, resolveExplainPrintSections(sysVars, printSections))
	if err != nil {
		return nil, err
	}

	result := &Result{
		TableHeader:  toTableHeader(explainColumnNames),
		ColumnAlign:  explainColumnAlign,
		AffectedRows: len(rows),
		Rows:         rows,
		Predicates:   predicates,
		Appendices:   appendices,
		LintResults:  lo.TernaryF(sysVars.Query.LintPlan, func() []string { return lintPlan(queryPlan) }, lo.Empty[[]string]),
	}
	return result, nil
}

func executeExplainAnalyze(ctx context.Context, session *Session, sql string, format enums.ExplainFormat, width int64, printSections *planref.PrintSections) (*Result, error) {
	stmt, err := newStatement(sql, session.systemVariables.Params, false)
	if err != nil {
		return nil, err
	}

	// EXPLAIN ANALYZE requires the query plan, so PROFILE is forced here
	// regardless of CLI_QUERY_MODE.
	iter, roTxn, err := session.txn.RunQueryWithStats(ctx, stmt, false, sppb.ExecuteSqlRequest_PROFILE)
	if err != nil {
		return nil, err
	}

	// Count the actual data rows while draining the iterator;
	// RowIterator.RowCount is only populated for DML.
	var actualRows int64
	stats, _, _, plan, err := consumeRowIter(iter, func(*spanner.Row) error {
		actualRows++
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Cloud Spanner Emulator doesn't set query plan nodes to the result.
	// See: https://github.com/GoogleCloudPlatform/cloud-spanner-emulator/blob/77188b228e7757cd56ecffb5bc3ee85dce5d6ae1/frontend/handlers/queries.cc#L224-L230
	if plan == nil {
		return nil, errors.New("query plan is not available. EXPLAIN ANALYZE statement is not supported for Cloud Spanner Emulator")
	}

	result, err := generateExplainAnalyzeResult(session.systemVariables, plan, stats, format, width, printSections)
	if err != nil {
		return nil, err
	}

	// Report the rows the query actually returned, not the number of rendered
	// plan-node rows, mirroring executeExplainAnalyzeDML's affected-rows
	// override (#417).
	result.AffectedRows = int(actualRows)

	if roTxn != nil {
		ts, err := roTxn.Timestamp()
		if err != nil {
			slog.Warn("failed to get read-only transaction timestamp", "err", err, "sql", sql)
		} else {
			result.ReadTimestamp = ts
		}
	}

	session.systemVariables.LastResult.QueryCache = &LastQueryCache{
		QueryPlan:     plan,
		QueryStats:    stats,
		ReadTimestamp: result.ReadTimestamp,
	}

	return result, nil
}

func generateExplainAnalyzeResult(sysVars *systemVariables, plan *sppb.QueryPlan, stats map[string]interface{},
	format enums.ExplainFormat, width int64, printSections *planref.PrintSections,
) (*Result, error) {
	queryStats, err := parseQueryStats(stats)
	if err != nil {
		return nil, fmt.Errorf("failed to parse query stats: %w", err)
	}
	return buildExplainAnalyzeResult(sysVars, plan, queryStats, format, width, printSections)
}

// buildExplainAnalyzeResult builds an EXPLAIN ANALYZE result from pre-parsed QueryStats.
func buildExplainAnalyzeResult(sysVars *systemVariables, plan *sppb.QueryPlan, queryStats QueryStats,
	format enums.ExplainFormat, width int64, printSections *planref.PrintSections,
) (*Result, error) {
	def := sysVars.Display.ParsedAnalyzeColumns
	inlines := sysVars.Display.ParsedInlineStats
	format = lo.Ternary(format != enums.ExplainFormatUnspecified, format, sysVars.Display.ExplainFormat)
	width = lo.Ternary(width != 0, width, sysVars.Display.ExplainWrapWidth)

	rows, predicates, appendices, err := processPlan(plan, def, inlines, format, width, sysVars.Display.ExplainHangingIndent, resolveExplainPrintSections(sysVars, printSections))
	if err != nil {
		return nil, fmt.Errorf("failed to process query plan: %w", err)
	}

	columnNames, columnAlign := explainAnalyzeHeader(def, width)

	var lintResults []string
	if sysVars.Query.LintPlan {
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
		Appendices:   appendices,
		LintResults:  lintResults,
		IndexAdvice:  extractIndexAdvice(plan),
	}

	return result, nil
}

func explainAnalyzeHeader(def []columnRenderDef, width int64) ([]string, []tw.Align) {
	// Start with the base columns and alignments for EXPLAIN output.
	baseNames := lo.Ternary(width == 0 || width >= operatorColumnNameLength, explainColumnNames, explainColumnNamesShort)
	baseAlign := explainColumnAlign

	// Extract the names and alignments from the custom column definitions.
	customNames := slices.Collect(loi.Map(slices.Values(def), func(d columnRenderDef) string { return d.Name }))
	customAligns := slices.Collect(loi.Map(slices.Values(def), func(d columnRenderDef) tw.Align { return d.Alignment }))

	// Concatenate the base and custom parts.
	columnNames := slices.Concat(baseNames, customNames)
	columnAlign := slices.Concat(baseAlign, customAligns)

	return columnNames, columnAlign
}

func executeExplainAnalyzeDML(ctx context.Context, session *Session, sql string, format enums.ExplainFormat, width int64, printSections *planref.PrintSections) (*Result, error) {
	stmt, err := newStatement(sql, session.systemVariables.Params, false)
	if err != nil {
		return nil, err
	}

	var queryStats map[string]any
	dmlResult, err := session.txn.RunInNewOrExistRwTx(ctx, func(tx *spanner.ReadWriteStmtBasedTransaction, implicit bool) (int64, *sppb.QueryPlan, *sppb.ResultSetMetadata, error) {
		iter := session.txn.runQueryWithStatsOnTransaction(ctx, tx, stmt, implicit)
		qs, count, metadata, plan, err := consumeRowIterDiscard(iter)
		queryStats = qs
		return count, plan, metadata, err
	})
	if err != nil {
		return nil, err
	}

	result, err := generateExplainAnalyzeResult(session.systemVariables, dmlResult.Plan, queryStats, format, width, printSections)
	if err != nil {
		return nil, err
	}

	result.IsExecutedDML = true
	result.AffectedRows = int(dmlResult.Affected)
	result.AffectedRowsType = rowCountTypeExact
	result.CommitTimestamp = dmlResult.CommitResponse.CommitTs

	// Update LastQueryCache to maintain consistency with other DML execution functions
	session.systemVariables.LastResult.QueryCache = &LastQueryCache{
		QueryPlan:       dmlResult.Plan,
		QueryStats:      queryStats,
		CommitTimestamp: dmlResult.CommitResponse.CommitTs,
	}

	return result, nil
}

func processPlanWithoutStats(plan *sppb.QueryPlan, format enums.ExplainFormat, width int64, hangingIndent bool, printSections planref.PrintSections) (rows []Row, predicates []string, appendices []ResultAppendix, err error) {
	return processPlan(plan, nil, nil, format, width, hangingIndent, printSections)
}

func processPlan(plan *sppb.QueryPlan, columnRenderDefs []columnRenderDef, inlineStatsDefs []inlineStatsDef, format enums.ExplainFormat, width int64, hangingIndent bool, printSections planref.PrintSections) (rows []Row, predicates []string, appendices []ResultAppendix, err error) {
	rowsWithPredicates, err := processPlanNodes(plan.GetPlanNodes(), inlineStatsDefs, format, width, hangingIndent)
	if err != nil {
		return nil, nil, nil, err
	}

	for _, row := range rowsWithPredicates {
		rowStrs := []string{formatPlanRowID(row, printSections), row.Text()}
		for _, colRender := range columnRenderDefs {
			c, err := colRender.MapFunc(row)
			if err != nil {
				return nil, nil, nil, err
			}

			rowStrs = append(rowStrs, c)
		}
		rows = append(rows, toRow(rowStrs...))
	}
	predicates, appendices = buildPlanAppendices(rowsWithPredicates, printSections)

	return rows, predicates, appendices, nil
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
		queryPlan, metadata, err := session.txn.RunAnalyzeQuery(ctx, stmt)
		return queryPlan, time.Time{}, metadata, err
	}

	result, err := session.txn.RunInNewOrExistRwTx(ctx, func(tx *spanner.ReadWriteStmtBasedTransaction, implicit bool) (int64, *sppb.QueryPlan, *sppb.ResultSetMetadata, error) {
		plan, metadata, err := session.txn.runAnalyzeQueryOnTransaction(ctx, tx, stmt)
		return 0, plan, metadata, err
	})
	if err != nil {
		return nil, time.Time{}, nil, err
	}
	return result.Plan, result.CommitResponse.CommitTs, result.Metadata, nil
}

func processPlanNodes(nodes []*sppb.PlanNode, statsDefs []inlineStatsDef, format enums.ExplainFormat, width int64, hangingIndent bool) ([]plantree.RowWithPredicates, error) {
	var options []plantree.Option
	switch format {
	case enums.ExplainFormatCurrent, enums.ExplainFormatUnspecified:
		options = append(options, plantree.WithQueryPlanOptions(
			spannerplan.WithExecutionMethodFormat(spannerplan.ExecutionMethodFormatAngle),
			spannerplan.WithKnownFlagFormat(spannerplan.KnownFlagFormatLabel),
			spannerplan.WithTargetMetadataFormat(spannerplan.TargetMetadataFormatOn),
		))
	case enums.ExplainFormatCompact:
		options = append(options,
			plantree.EnableCompact(),
			plantree.WithQueryPlanOptions(
				spannerplan.WithExecutionMethodFormat(spannerplan.ExecutionMethodFormatAngle),
				spannerplan.WithKnownFlagFormat(spannerplan.KnownFlagFormatLabel),
				spannerplan.WithTargetMetadataFormat(spannerplan.TargetMetadataFormatOn),
			))
	case enums.ExplainFormatTraditional:
		// nop because it is default bformat in plantree.ProcessPlan.
	}

	if width > 0 {
		options = append(options, plantree.WithWrapWidth(int(width)))
		if hangingIndent {
			options = append(options, plantree.WithHangingIndent())
		}
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

func extractIndexAdvice(plan *sppb.QueryPlan) []QueryIndexAdvice {
	var result []QueryIndexAdvice
	for _, advice := range plan.GetQueryAdvice().GetIndexAdvice() {
		if len(advice.GetDdl()) == 0 {
			continue
		}
		result = append(result, QueryIndexAdvice{
			DDL:               advice.GetDdl(),
			ImprovementFactor: advice.GetImprovementFactor(),
		})
	}
	return result
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
