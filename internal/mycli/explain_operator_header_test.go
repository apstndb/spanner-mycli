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
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/olekukonko/tablewriter/tw"
	"google.golang.org/protobuf/proto"
)

func TestExplainOperatorHeaderDefaultWideAndNarrow(t *testing.T) {
	t.Parallel()
	vars := newSystemVariablesWithDefaultsForTest()
	plan := &sppb.QueryPlan{PlanNodes: hangingIndentPlanNodes()}

	wide, err := generateExplainResult(vars, plan, enums.ExplainFormatCurrent, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := wide.TableHeader.Render(false); !slices.Equal(got, []string{"ID", operatorColumnName}) {
		t.Fatalf("default wide EXPLAIN header = %v", got)
	}

	narrow, err := generateExplainResult(vars, plan, enums.ExplainFormatCurrent, 20, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := narrow.TableHeader.Render(false); !slices.Equal(got, []string{"ID", "Operator"}) {
		t.Fatalf("default narrow EXPLAIN header = %v", got)
	}
}

func TestExplainOperatorHeaderExplicitLabelAtBothWidths(t *testing.T) {
	t.Parallel()
	vars := newSystemVariablesWithDefaultsForTest()
	if err := vars.SetFromSimple("CLI_EXPLAIN_OPERATOR_HEADER", "  Op  "); err != nil {
		t.Fatal(err)
	}
	if got := mustGetVarFromSysVars(t, vars, "CLI_EXPLAIN_OPERATOR_HEADER"); got != "Op" {
		t.Fatalf("normalized assignment = %q, want Op", got)
	}

	plan := &sppb.QueryPlan{PlanNodes: hangingIndentPlanNodes()}
	for _, width := range []int64{0, 20} {
		plain, err := generateExplainResult(vars, plan, enums.ExplainFormatCurrent, width, nil)
		if err != nil {
			t.Fatal(err)
		}
		analyze, err := generateExplainAnalyzeResult(vars, plan, nil, enums.ExplainFormatCurrent, width, nil)
		if err != nil {
			t.Fatal(err)
		}
		profile, err := buildExplainAnalyzeResult(vars, plan, QueryStats{}, enums.ExplainFormatCurrent, width, nil)
		if err != nil {
			t.Fatal(err)
		}
		for name, result := range map[string]*Result{"explain": plain, "analyze": analyze, "profile": profile} {
			got := result.TableHeader.Render(false)
			if len(got) < 2 || got[0] != "ID" || got[1] != "Op" {
				t.Errorf("width %d %s header = %v, want ID Op", width, name, got)
			}
		}
	}
}

func TestExplainOperatorHeaderUnicodeAccepted(t *testing.T) {
	t.Parallel()
	vars := newSystemVariablesWithDefaultsForTest()
	const label = "演算子→Plan"
	if err := vars.SetFromSimple("CLI_EXPLAIN_OPERATOR_HEADER", "  "+label+"  "); err != nil {
		t.Fatal(err)
	}
	plain, err := generateExplainResult(vars, &sppb.QueryPlan{PlanNodes: hangingIndentPlanNodes()}, enums.ExplainFormatCurrent, 20, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := plain.TableHeader.Render(false); !slices.Equal(got, []string{"ID", label}) {
		t.Fatalf("unicode header = %v", got)
	}
}

func TestExplainOperatorHeaderRejectsControlCharacters(t *testing.T) {
	t.Parallel()
	vars := newSystemVariablesWithDefaultsForTest()
	if err := vars.SetFromSimple("CLI_EXPLAIN_OPERATOR_HEADER", "Op"); err != nil {
		t.Fatal(err)
	}

	for _, value := range []string{
		"Op\nX",
		"Op\rX",
		"Op\tX",
		"Op\x00X",
		"\x1b[31mOp",
	} {
		err := vars.SetFromSimple("CLI_EXPLAIN_OPERATOR_HEADER", value)
		if err == nil || !strings.Contains(err.Error(), "control characters") {
			t.Errorf("SetFromSimple(%q) error = %v, want control characters", value, err)
		}
		if got := mustGetVarFromSysVars(t, vars, "CLI_EXPLAIN_OPERATOR_HEADER"); got != "Op" {
			t.Fatalf("rejected %q changed prior value to %q", value, got)
		}
		if vars.Display.ExplainOperatorHeader != "Op" {
			t.Fatalf("rejected %q mutated Display.ExplainOperatorHeader = %q", value, vars.Display.ExplainOperatorHeader)
		}
	}
}

func TestExplainOperatorHeaderWhitespaceOnlyKeepsWidthRule(t *testing.T) {
	t.Parallel()
	vars := newSystemVariablesWithDefaultsForTest()
	if err := vars.SetFromSimple("CLI_EXPLAIN_OPERATOR_HEADER", "Op"); err != nil {
		t.Fatal(err)
	}
	if err := vars.SetFromSimple("CLI_EXPLAIN_OPERATOR_HEADER", " \t\n "); err != nil {
		t.Fatal(err)
	}
	if got := mustGetVarFromSysVars(t, vars, "CLI_EXPLAIN_OPERATOR_HEADER"); got != "" {
		t.Fatalf("whitespace-only SHOW = %q, want empty", got)
	}

	plan := &sppb.QueryPlan{PlanNodes: hangingIndentPlanNodes()}
	wide, err := generateExplainResult(vars, plan, enums.ExplainFormatCurrent, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	narrow, err := generateExplainResult(vars, plan, enums.ExplainFormatCurrent, 20, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := wide.TableHeader.Render(false); !slices.Equal(got, []string{"ID", operatorColumnName}) {
		t.Fatalf("whitespace-only wide header = %v", got)
	}
	if got := narrow.TableHeader.Render(false); !slices.Equal(got, []string{"ID", "Operator"}) {
		t.Fatalf("whitespace-only narrow header = %v", got)
	}
}

func TestExplainOperatorHeaderResetAndLocalRestore(t *testing.T) {
	t.Parallel()

	t.Run("RESET restores startup snapshot", func(t *testing.T) {
		t.Parallel()
		sv, err := initializeSystemVariables(&spannerOptions{
			ProjectId:  "p",
			InstanceId: "i",
			DatabaseId: "d",
			Set:        map[string]string{"CLI_EXPLAIN_OPERATOR_HEADER": "StartupOp"},
		})
		if err != nil {
			t.Fatalf("initializeSystemVariables: %v", err)
		}
		session := &Session{
			mode:            DatabaseConnected,
			systemVariables: sv,
			connection:      sv.Connection,
			txn:             NewTransactionManager(nil, sv, spanner.ClientConfig{}),
		}
		sv.inTransaction = session.txn.InTransaction
		sv.inManualBatch = session.batch.IsActive

		if _, err := session.ExecuteStatement(t.Context(), &SetStatement{VarName: "CLI_EXPLAIN_OPERATOR_HEADER", Value: "'Changed'"}); err != nil {
			t.Fatal(err)
		}
		if got := mustGetVar(t, session, "CLI_EXPLAIN_OPERATOR_HEADER"); got != "Changed" {
			t.Fatalf("after SET = %q", got)
		}
		if _, err := session.ExecuteStatement(t.Context(), &ResetStatement{VarName: "CLI_EXPLAIN_OPERATOR_HEADER"}); err != nil {
			t.Fatal(err)
		}
		if got := mustGetVar(t, session, "CLI_EXPLAIN_OPERATOR_HEADER"); got != "StartupOp" {
			t.Fatalf("after RESET = %q, want StartupOp", got)
		}
	})

	t.Run("SET LOCAL restores on COMMIT and ROLLBACK", func(t *testing.T) {
		t.Parallel()
		for _, endStmt := range []Statement{&CommitStatement{}, &RollbackStatement{}} {
			session := newSessionForLocalVarTest(t)
			if err := session.systemVariables.SetFromSimple("CLI_EXPLAIN_OPERATOR_HEADER", "SessionOp"); err != nil {
				t.Fatal(err)
			}
			if _, err := session.ExecuteStatement(t.Context(), &BeginStatement{}); err != nil {
				t.Fatal(err)
			}
			if _, err := session.ExecuteStatement(t.Context(), &SetLocalStatement{VarName: "CLI_EXPLAIN_OPERATOR_HEADER", Value: "'LocalOp'"}); err != nil {
				t.Fatal(err)
			}
			if got := mustGetVar(t, session, "CLI_EXPLAIN_OPERATOR_HEADER"); got != "LocalOp" {
				t.Fatalf("during txn = %q", got)
			}
			if _, err := session.ExecuteStatement(t.Context(), endStmt); err != nil {
				t.Fatal(err)
			}
			if got := mustGetVar(t, session, "CLI_EXPLAIN_OPERATOR_HEADER"); got != "SessionOp" {
				t.Fatalf("after %T = %q, want SessionOp", endStmt, got)
			}
		}
	})
}

func TestExplainOperatorHeaderIndependentOfAnalyzeAliases(t *testing.T) {
	t.Parallel()
	vars := newSystemVariablesWithDefaultsForTest()
	if err := vars.SetFromSimple("CLI_EXPLAIN_OPERATOR_HEADER", "Op"); err != nil {
		t.Fatal(err)
	}
	if err := vars.SetFromSimple("CLI_ANALYZE_COLUMNS", "Elapsed:{{.Latency}}:CENTER,Hits:{{.Rows.Total}}"); err != nil {
		t.Fatal(err)
	}

	analyze, err := buildExplainAnalyzeResult(vars, &sppb.QueryPlan{PlanNodes: hangingIndentPlanNodes()}, QueryStats{}, enums.ExplainFormatCurrent, 20, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := analyze.TableHeader.Render(false)
	want := []string{"ID", "Op", "Elapsed", "Hits"}
	if !slices.Equal(got, want) {
		t.Fatalf("analyze header = %v, want %v", got, want)
	}
	if !slices.Equal(analyze.ColumnAlign, []tw.Align{tw.AlignRight, tw.AlignLeft, tw.AlignCenter, tw.AlignRight}) {
		t.Fatalf("analyze align = %v", analyze.ColumnAlign)
	}

	plain, err := generateExplainResult(vars, &sppb.QueryPlan{PlanNodes: hangingIndentPlanNodes()}, enums.ExplainFormatCurrent, 20, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := plain.TableHeader.Render(false); !slices.Equal(got, []string{"ID", "Op"}) {
		t.Fatalf("EXPLAIN must not grow analyze aliases: %v", got)
	}
}

func TestExplainOperatorHeaderBuildersAndRenderedOutput(t *testing.T) {
	t.Parallel()
	vars := newSystemVariablesWithDefaultsForTest()
	if err := vars.SetFromSimple("CLI_EXPLAIN_OPERATOR_HEADER", "Op"); err != nil {
		t.Fatal(err)
	}
	plan := &sppb.QueryPlan{PlanNodes: hangingIndentPlanNodes()}

	plain, err := generateExplainResult(vars, plan, enums.ExplainFormatCurrent, 20, nil)
	if err != nil {
		t.Fatal(err)
	}
	analyze, err := generateExplainAnalyzeResult(vars, plan, nil, enums.ExplainFormatCurrent, 20, nil)
	if err != nil {
		t.Fatal(err)
	}

	session := newSessionForLocalVarTest(t)
	session.systemVariables.Display.ExplainOperatorHeader = "Op"
	session.systemVariables.LastResult.QueryCache = &LastQueryCache{QueryPlan: plan, QueryStats: map[string]any{}}
	last, err := (&ExplainLastQueryStatement{Width: 20}).Execute(t.Context(), session, OperationOutput{})
	if err != nil {
		t.Fatal(err)
	}
	lastAnalyze, err := (&ExplainLastQueryStatement{Analyze: true, Width: 20}).Execute(t.Context(), session, OperationOutput{})
	if err != nil {
		t.Fatal(err)
	}

	for name, result := range map[string]*Result{
		"explain":     plain,
		"analyze":     analyze,
		"last":        last,
		"lastAnalyze": lastAnalyze,
	} {
		if got := result.TableHeader.Render(false); got[0] != "ID" || got[1] != "Op" {
			t.Errorf("%s header = %v", name, got)
		}
		if !operatorRowsContain(result, "Cross Apply") {
			t.Errorf("%s lost operator row values", name)
		}
	}

	tableVars := newSystemVariablesWithDefaultsForTest()
	var tableOut bytes.Buffer
	if err := printTableData(tableVars, 0, &tableOut, plain); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tableOut.String(), "Op") || strings.Contains(tableOut.String(), operatorColumnName) {
		t.Fatalf("TABLE missing Op header:\n%s", tableOut.String())
	}
	if !strings.Contains(tableOut.String(), "Cross Apply") {
		t.Fatalf("TABLE lost operator cells:\n%s", tableOut.String())
	}

	csvVars := newSystemVariablesWithDefaultsForTest()
	if err := csvVars.SetFromSimple("CLI_FORMAT", "CSV"); err != nil {
		t.Fatal(err)
	}
	var csvOut bytes.Buffer
	if err := printTableData(csvVars, 0, &csvOut, plain); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(csvOut.String(), "Op") || strings.Contains(csvOut.String(), operatorColumnName) {
		t.Fatalf("CSV missing Op header:\n%s", csvOut.String())
	}
	if !strings.Contains(csvOut.String(), "Cross Apply") {
		t.Fatalf("CSV lost operator cells:\n%s", csvOut.String())
	}
}

func TestExplainOperatorHeaderLongLabelIsNotTruncated(t *testing.T) {
	t.Parallel()
	const label = "DeliberatelyLongOperatorHeader"
	vars := newSystemVariablesWithDefaultsForTest()
	if err := vars.SetFromSimple("CLI_EXPLAIN_OPERATOR_HEADER", label); err != nil {
		t.Fatal(err)
	}
	result, err := generateExplainResult(vars, &sppb.QueryPlan{PlanNodes: hangingIndentPlanNodes()}, enums.ExplainFormatCurrent, 20, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.TableHeader.Render(false); !slices.Equal(got, []string{"ID", label}) {
		t.Fatalf("header = %v", got)
	}
	var out bytes.Buffer
	if err := printTableData(vars, 0, &out, result); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), label) {
		t.Fatalf("WIDTH=20 truncated the explicit header:\n%s", out.String())
	}
}

func TestExplainOperatorHeaderDoesNotChangeQueryResultsOrRawPlans(t *testing.T) {
	t.Parallel()
	vars := newSystemVariablesWithDefaultsForTest()
	query := &Result{
		TableHeader: toTableHeader("SingerId", "Name"),
		Body:        PresentationBody(sliceOf(toRow("1", "Alice"))),
	}
	if err := vars.SetFromSimple("CLI_EXPLAIN_OPERATOR_HEADER", "Op"); err != nil {
		t.Fatal(err)
	}

	var tableOut bytes.Buffer
	if err := printTableData(vars, 0, &tableOut, query); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tableOut.String(), "SingerId") || !strings.Contains(tableOut.String(), "Alice") {
		t.Fatalf("query TABLE lost metadata or rows:\n%s", tableOut.String())
	}
	if strings.Contains(tableOut.String(), "| Op ") || strings.Contains(tableOut.String(), "operator") {
		t.Fatalf("query TABLE picked up the explain header:\n%s", tableOut.String())
	}

	csvVars := newSystemVariablesWithDefaultsForTest()
	csvVars.Display.ExplainOperatorHeader = "Op"
	if err := csvVars.SetFromSimple("CLI_FORMAT", "CSV"); err != nil {
		t.Fatal(err)
	}
	var csvOut bytes.Buffer
	if err := printTableData(csvVars, 0, &csvOut, query); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(csvOut.String(), "SingerId") || !strings.Contains(csvOut.String(), "Alice") {
		t.Fatalf("query CSV lost metadata or rows:\n%s", csvOut.String())
	}

	session := newSessionForLocalVarTest(t)
	plan := proto.Clone(&sppb.QueryPlan{PlanNodes: hangingIndentPlanNodes()}).(*sppb.QueryPlan)
	session.systemVariables.LastResult.QueryCache = &LastQueryCache{QueryPlan: plan}
	beforeJSON, beforeYAML, beforeNode := rawPlanExports(t, session)

	if err := session.systemVariables.SetFromSimple("CLI_EXPLAIN_OPERATOR_HEADER", "Op"); err != nil {
		t.Fatal(err)
	}
	afterJSON, afterYAML, afterNode := rawPlanExports(t, session)
	if beforeJSON != afterJSON {
		t.Fatal("SHOW LAST QUERY PLAN ProtoJSON changed after setting the operator header")
	}
	if beforeYAML != afterYAML {
		t.Fatal("SHOW LAST QUERY PLAN YAML changed after setting the operator header")
	}
	if beforeNode != afterNode {
		t.Fatal("SHOW PLAN NODE YAML changed after setting the operator header")
	}
}

func mustGetVarFromSysVars(t *testing.T, vars *systemVariables, name string) string {
	t.Helper()
	value, err := vars.Registry.Get(name)
	if err != nil {
		t.Fatalf("Registry.Get(%q): %v", name, err)
	}
	return value
}

func operatorRowsContain(result *Result, needle string) bool {
	for _, row := range result.presentationRows() {
		for _, cell := range row {
			if strings.Contains(cell.RawText(), needle) {
				return true
			}
		}
	}
	return false
}

func rawPlanExports(t *testing.T, session *Session) (jsonOut, yamlOut, nodeOut string) {
	t.Helper()
	jsonResult, err := (&ShowLastQueryPlanStatement{}).Execute(t.Context(), session, OperationOutput{})
	if err != nil {
		t.Fatal(err)
	}
	jsonOut = jsonResult.presentationRows()[0][0].RawText()

	path := filepath.Join(t.TempDir(), "plan.yaml")
	if _, err := (&ShowLastQueryPlanStatement{IntoPath: path}).Execute(t.Context(), session, OperationOutput{}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	yamlOut = string(b)

	node, err := (&ShowPlanNodeStatement{NodeID: 0}).Execute(t.Context(), session, OperationOutput{})
	if err != nil {
		t.Fatal(err)
	}
	nodeOut = node.presentationRows()[0][0].RawText()
	return jsonOut, yamlOut, nodeOut
}
