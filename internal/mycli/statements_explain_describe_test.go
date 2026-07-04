package mycli

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"strings"
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spannerplan"
	"github.com/apstndb/spannerplan/plantree"
	planref "github.com/apstndb/spannerplan/plantree/reference"
	"github.com/apstndb/spannerplan/protoyaml"
	"github.com/apstndb/spannerplan/stats"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/olekukonko/tablewriter/tw"
	"github.com/samber/lo"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
)

// Test helpers for explain/describe statements

func mustNewStruct(m map[string]interface{}) *structpb.Struct {
	if s, err := structpb.NewStruct(m); err != nil {
		panic(err)
	} else {
		return s
	}
}

func loadTestPlan(t *testing.T, file string) *sppb.QueryPlan {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var plan sppb.QueryPlan
	err = protojson.Unmarshal(b, &plan)
	if err != nil {
		t.Fatal(err)
	}
	return &plan
}

func testPlanProcessing(t *testing.T, file string, want []plantree.RowWithPredicates) {
	t.Helper()
	plan := loadTestPlan(t, file)
	got, err := processPlanNodes(plan.GetPlanNodes(), nil, enums.ExplainFormatTraditional, 0, false)
	if err != nil {
		t.Errorf("error should be nil, but got = %v", err)
	}
	if diff := cmp.Diff(want, got, planRowTextCmpOpts()...); diff != "" {
		t.Errorf("processPlanNodes() differ: %s", diff)
	}
}

func planRowTextCmpOpts() []cmp.Option {
	// These tests cover spanner-mycli's rendered text, predicate, and stats
	// behavior. spannerplan also populates metadata used by appendix rendering.
	return []cmp.Option{
		cmpopts.IgnoreFields(
			plantree.RowWithPredicates{},
			"ChildLinks",
			"DisplayName",
			"Keys",
			"ScalarChildLinks",
		),
	}
}

func TestProcessPlanNodesUsesHangingIndentForWrappedPlans(t *testing.T) {
	t.Parallel()

	rows, err := processPlanNodes(hangingIndentPlanNodes(), nil, enums.ExplainFormatCurrent, 21, true)
	if err != nil {
		t.Fatalf("processPlanNodes(..., hangingIndent=true) error = %v", err)
	}
	got := planRowByID(t, rows, 1)

	want := plantree.RowWithPredicates{
		ID:       1,
		TreePart: "+- \n|          ",
		NodeText: "[Input] Batch Scan\n <Row>",
	}
	if diff := cmp.Diff(want, plantree.RowWithPredicates{
		ID:       got.ID,
		TreePart: got.TreePartString(),
		NodeText: got.NodeText,
	}); diff != "" {
		t.Fatalf("wrapped hanging-indent row mismatch (-want +got):\n%s", diff)
	}

	legacyRows, err := processPlanNodes(hangingIndentPlanNodes(), nil, enums.ExplainFormatCurrent, 21, false)
	if err != nil {
		t.Fatalf("processPlanNodes(..., hangingIndent=false) error = %v", err)
	}
	legacy := planRowByID(t, legacyRows, 1)
	wantLegacy := plantree.RowWithPredicates{
		ID:       1,
		TreePart: "+- \n|  ",
		NodeText: "[Input] Batch Scan\n <Row>",
	}
	if diff := cmp.Diff(wantLegacy, plantree.RowWithPredicates{
		ID:       legacy.ID,
		TreePart: legacy.TreePartString(),
		NodeText: legacy.NodeText,
	}); diff != "" {
		t.Fatalf("wrapped opt-out row mismatch (-want +got):\n%s", diff)
	}
}

func hangingIndentPlanNodes() []*sppb.PlanNode {
	return []*sppb.PlanNode{
		{
			Index:       0,
			DisplayName: "Cross Apply",
			Kind:        sppb.PlanNode_RELATIONAL,
			ChildLinks: []*sppb.PlanNode_ChildLink{
				{ChildIndex: 1},
				{ChildIndex: 2, Type: "Map"},
			},
		},
		{
			Index:       1,
			DisplayName: "Batch Scan",
			Kind:        sppb.PlanNode_RELATIONAL,
			Metadata: &structpb.Struct{Fields: map[string]*structpb.Value{
				"execution_method": structpb.NewStringValue("Row"),
			}},
		},
		{
			Index:       2,
			DisplayName: "Serialize Result",
			Kind:        sppb.PlanNode_RELATIONAL,
		},
	}
}

func planRowByID(t *testing.T, rows []plantree.RowWithPredicates, id int32) plantree.RowWithPredicates {
	t.Helper()
	for _, row := range rows {
		if row.ID == id {
			return row
		}
	}
	t.Fatalf("plan row %d not found in %v", id, rows)
	return plantree.RowWithPredicates{}
}

func TestRenderTreeUsingTestdataPlans(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		title string
		file  string
		want  []plantree.RowWithPredicates
	}{
		{
			// Original Query:
			// SELECT s.LastName FROM (SELECT s.LastName FROM Singers AS s WHERE s.FirstName LIKE 'A%' LIMIT 3) s WHERE s.LastName LIKE 'Rich%';
			title: "With Filter Operator",
			file:  "testdata/plans/filter.input.json",
			want: []plantree.RowWithPredicates{
				{
					ID:       0,
					NodeText: "Serialize Result",
				},
				{
					ID:       1,
					TreePart: "+- ", NodeText: "Filter",
					Predicates: []string{"Condition: STARTS_WITH($LastName, 'Rich')"},
				},
				{
					ID:       2,
					TreePart: "   +- ", NodeText: "Global Limit",
				},
				{
					ID:       3,
					TreePart: "      +- ", NodeText: "Distributed Union",
					Predicates: []string{"Split Range: STARTS_WITH($FirstName, 'A')"},
				},
				{
					ID:       4,
					TreePart: "         +- ", NodeText: "Local Limit",
				},
				{
					ID:       5,
					TreePart: "            +- ", NodeText: "Local Distributed Union",
				},
				{
					ID:       6,
					TreePart: "               +- ", NodeText: "FilterScan",
					Predicates: []string{"Seek Condition: STARTS_WITH($FirstName, 'A')"},
				},
				{
					ID:       7,
					TreePart: "                  +- ", NodeText: "Index Scan (Index: SingersByFirstLastName)",
				},
			},
		},
		{
			/*
				Original Query:
				SELECT a.AlbumTitle, s.SongName
				FROM Albums AS a HASH JOIN Songs AS s
				ON a.SingerId = s.SingerId AND a.AlbumId = s.AlbumId;
			*/
			title: "Hash Join",
			file:  "testdata/plans/hash_join.input.json",
			want: []plantree.RowWithPredicates{
				{
					ID:       0,
					NodeText: "Distributed Union",
				},
				{
					ID:       1,
					TreePart: "+- ", NodeText: "Serialize Result",
				},
				{
					ID:       2,
					TreePart: "   +- ", NodeText: "Hash Join (join_type: INNER)",
					Predicates: []string{"Condition: (($SingerId = $SingerId_1) AND ($AlbumId = $AlbumId_1))"},
				},
				{
					ID:       3,
					TreePart: "      +- ", NodeText: "[Build] Local Distributed Union",
				},
				{
					ID:       4,
					TreePart: "      |  +- ", NodeText: "Table Scan (Full scan: true, Table: Albums)",
				},
				{
					ID:       8,
					TreePart: "      +- ", NodeText: "[Probe] Local Distributed Union",
				},
				{
					ID:       9,
					TreePart: "         +- ", NodeText: "Index Scan (Full scan: true, Index: SongsBySingerAlbumSongNameDesc)",
				},
			},
		},
		{
			/*
				Original Query: https://cloud.google.com/spanner/docs/query-execution-operators?hl=en#array_subqueries
				SELECT a.AlbumId,
				ARRAY(SELECT ConcertDate
				      FROM Concerts
				      WHERE Concerts.SingerId = a.SingerId)
				FROM Albums AS a;
			*/
			title: "Array Subqueries",
			file:  "testdata/plans/array_subqueries.input.json",
			want: []plantree.RowWithPredicates{
				{
					ID:       0,
					NodeText: "Distributed Union",
				},
				{
					ID:       1,
					TreePart: "+- ", NodeText: "Local Distributed Union",
				},
				{
					ID:       2,
					TreePart: "   +- ", NodeText: "Serialize Result",
				},
				{
					ID:       3,
					TreePart: "      +- ", NodeText: "Index Scan (Full scan: true, Index: AlbumsByAlbumTitle)",
				},
				{
					ID:       7,
					TreePart: "      +- ", NodeText: "[Scalar] Array Subquery",
				},
				{
					ID:       8,
					TreePart: "         +- ", NodeText: "Distributed Union",
					Predicates: []string{"Split Range: ($SingerId_1 = $SingerId)"},
				},
				{
					ID:       9,
					TreePart: "            +- ", NodeText: "Local Distributed Union",
				},
				{
					ID:       10,
					TreePart: "               +- ", NodeText: "FilterScan",
					Predicates: []string{"Seek Condition: ($SingerId_1 = $SingerId)"},
				},
				{
					ID:       11,
					TreePart: "                  +- ", NodeText: "Index Scan (Index: ConcertsBySingerId)",
				},
			},
		},
		{
			/*
				Original Query: https://cloud.google.com/spanner/docs/query-execution-operators?hl=en#scalar_subqueries
				SELECT FirstName,
				IF(FirstName='Alice',
				   (SELECT COUNT(*)
				    FROM Songs
				    WHERE Duration > 300),
				   0)
				FROM Singers;
			*/
			title: "Scalar Subqueries",
			file:  "testdata/plans/scalar_subqueries.input.json",
			want: []plantree.RowWithPredicates{
				{
					NodeText: "Distributed Union",
				},
				{
					ID:       1,
					TreePart: "+- ", NodeText: "Local Distributed Union",
				},
				{
					ID:       2,
					TreePart: "   +- ", NodeText: "Serialize Result",
				},
				{
					ID:       3,
					TreePart: "      +- ", NodeText: "Index Scan (Full scan: true, Index: SingersByFirstLastName)",
				},
				{
					ID:       10,
					TreePart: "      +- ", NodeText: "[Scalar] Scalar Subquery",
				},
				{
					ID:       11,
					TreePart: "         +- ", NodeText: "Global Stream Aggregate (scalar_aggregate: true)",
				},
				{
					ID:       12,
					TreePart: "            +- ", NodeText: "Distributed Union",
				},
				{
					ID:       13,
					TreePart: "               +- ", NodeText: "Local Stream Aggregate (scalar_aggregate: true)",
				},
				{
					ID:       14,
					TreePart: "                  +- ", NodeText: "Local Distributed Union",
				},
				{
					ID:       15,
					TreePart: "                     +- ", NodeText: "FilterScan",
					Predicates: []string{
						"Residual Condition: ($Duration > 300)",
					},
				},
				{
					ID:       16,
					TreePart: "                        +- ", NodeText: "Table Scan (Full scan: true, Table: Songs)",
				},
			},
		},
		{
			/*
				Original Query:
				SELECT si.*,
				  ARRAY(SELECT AS STRUCT a.*,
				        ARRAY(SELECT AS STRUCT so.*
				              FROM Songs so
				              WHERE a.SingerId = so.SingerId AND a.AlbumId = so.AlbumId)
				        FROM Albums a
				        WHERE a.SingerId = si.SingerId)
				FROM Singers si;
			*/
			title: "Array Subquery with Compute Struct",
			file:  "testdata/plans/array_subqueries_with_compute_struct.input.json",
			want: []plantree.RowWithPredicates{
				{
					NodeText: "Distributed Union",
				},
				{
					ID:       1,
					TreePart: "+- ", NodeText: "Local Distributed Union",
				},
				{
					ID:       2,
					TreePart: "   +- ", NodeText: "Serialize Result",
				},
				{
					ID:       3,
					TreePart: "      +- ", NodeText: "Table Scan (Full scan: true, Table: Singers)",
				},
				{
					ID:       14,
					TreePart: "      +- ", NodeText: "[Scalar] Array Subquery",
				},
				{
					ID:       15,
					TreePart: "         +- ", NodeText: "Local Distributed Union",
				},
				{
					ID:       16,
					TreePart: "            +- ", NodeText: "Compute Struct",
				},
				{
					ID:       17,
					TreePart: "               +- ", NodeText: "FilterScan",
					Predicates: []string{
						"Seek Condition: ($SingerId_1 = $SingerId)",
					},
				},
				{
					ID:       18,
					TreePart: "               |  +- ", NodeText: "Table Scan (Table: Albums)",
				},
				{
					ID:       31,
					TreePart: "               +- ", NodeText: "[Scalar] Array Subquery",
				},
				{
					ID:       32,
					TreePart: "                  +- ", NodeText: "Local Distributed Union",
				},
				{
					ID:       33,
					TreePart: "                     +- ", NodeText: "Compute Struct",
				},
				{
					ID:       34,
					TreePart: "                        +- ", NodeText: "FilterScan",
					Predicates: []string{
						"Seek Condition: (($SingerId_2 = $SingerId_1) AND ($AlbumId_1 = $AlbumId))",
					},
				},
				{
					ID:       35,
					TreePart: "                           +- ", NodeText: "Table Scan (Table: Songs)",
				},
			},
		},
		{
			/*
				Original Query:
				SELECT so.* FROM Songs so
				WHERE IF(so.SongGenre = "ROCKS", TRUE, EXISTS(SELECT * FROM Concerts c WHERE c.SingerId = so.SingerId))
			*/
			title: "Scalar Subquery with FilterScan",
			file:  "testdata/plans/scalar_subquery_with_filter_scan.input.json",
			want: []plantree.RowWithPredicates{
				{
					NodeText: "Distributed Union",
				},
				{
					ID:       1,
					TreePart: "+- ", NodeText: "Local Distributed Union",
				},
				{
					ID:       2,
					TreePart: "   +- ", NodeText: "Serialize Result",
				},
				{
					ID:       3,
					TreePart: "      +- ", NodeText: "FilterScan",
					Predicates: []string{
						"Residual Condition: IF(($SongGenre = 'ROCKS'), true, $sv_1)",
					},
				},
				{
					ID:       4,
					TreePart: "         +- ", NodeText: "Table Scan (Full scan: true, Table: Songs)",
				},
				{
					ID:       16,
					TreePart: "         +- ", NodeText: "[Scalar] Scalar Subquery",
				},
				{
					ID:       17,
					TreePart: "            +- ", NodeText: "Global Stream Aggregate (scalar_aggregate: true)",
				},
				{
					ID:       18,
					TreePart: "               +- ", NodeText: "Distributed Union",
					Predicates: []string{
						"Split Range: ($SingerId_1 = $SingerId)",
					},
				},
				{
					ID:       19,
					TreePart: "                  +- ", NodeText: "Local Stream Aggregate (scalar_aggregate: true)",
				},
				{
					ID:       20,
					TreePart: "                     +- ", NodeText: "Local Distributed Union",
				},
				{
					ID:       21,
					TreePart: "                        +- ", NodeText: "FilterScan",
					Predicates: []string{
						"Seek Condition: ($SingerId_1 = $SingerId)",
					},
				},
				{
					ID:       22,
					TreePart: "                           +- ", NodeText: "Index Scan (Index: ConcertsBySingerId)",
				},
			},
		},
	} {
		t.Run(test.title, func(t *testing.T) {
			testPlanProcessing(t, test.file, test.want)
		})
	}
}

func TestProcessPlanPredicateMarkersFollowPrintSections(t *testing.T) {
	t.Parallel()
	plan := loadTestPlan(t, "testdata/plans/filter.input.json")

	rows, _, _, err := processPlanWithoutStats(plan, enums.ExplainFormatTraditional, 0, false, planref.PrintSections{planref.PrintPredicates})
	if err != nil {
		t.Fatalf("processPlanWithoutStats() with predicates error = %v", err)
	}
	if !hasStarredPlanID(rows) {
		t.Fatal("processPlanWithoutStats() with predicate appendix should mark predicate rows")
	}

	rows, _, _, err = processPlanWithoutStats(plan, enums.ExplainFormatTraditional, 0, false, planref.PrintSections{})
	if err != nil {
		t.Fatalf("processPlanWithoutStats() without appendices error = %v", err)
	}
	if hasStarredPlanID(rows) {
		t.Fatal("processPlanWithoutStats() without predicate appendix should not mark predicate rows")
	}
}

func TestProcessPlanAppendicesUsingRealPlan(t *testing.T) {
	t.Parallel()
	plan := loadTestPlan(t, "testdata/plans/scalar_subqueries.input.json")

	_, _, appendices, err := processPlanWithoutStats(plan, enums.ExplainFormatTraditional, 0, false, planref.PrintSections{
		planref.PrintAggregate,
		planref.PrintTyped,
	})
	if err != nil {
		t.Fatalf("processPlanWithoutStats() error = %v", err)
	}
	if !appendixContains(appendices, "Aggregates(identified by ID):", "Agg: COUNT()") {
		t.Fatalf("aggregate appendix does not contain COUNT() line: %#v", appendices)
	}
	if !appendixContains(appendices, "Node Parameters(identified by ID):", "Condition:") {
		t.Fatalf("typed appendix does not contain condition line: %#v", appendices)
	}
}

func hasStarredPlanID(rows []Row) bool {
	for _, row := range rows {
		if strings.HasPrefix(row[0].RawText(), "*") {
			return true
		}
	}
	return false
}

func appendixContains(appendices []ResultAppendix, title, fragment string) bool {
	for _, appendix := range appendices {
		if appendix.Title != title {
			continue
		}
		for _, line := range appendix.Lines {
			if strings.Contains(line, fragment) {
				return true
			}
		}
	}
	return false
}

func Total(s string) stats.ExecutionStatsValue {
	return stats.ExecutionStatsValue{Total: s}
}

func TotalWithUnit(s, unit string) stats.ExecutionStatsValue {
	return stats.ExecutionStatsValue{Total: s, Unit: unit}
}

func TestRenderTreeWithStats(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		title           string
		plan            *sppb.QueryPlan
		inlineStatsDefs []inlineStatsDef
		want            []plantree.RowWithPredicates
	}{
		{
			title: "Simple Query",
			plan: &sppb.QueryPlan{
				PlanNodes: []*sppb.PlanNode{
					{
						Index: 0,
						ChildLinks: []*sppb.PlanNode_ChildLink{
							{ChildIndex: 1},
						},
						DisplayName: "Distributed Union",
						Kind:        sppb.PlanNode_RELATIONAL,
						ExecutionStats: mustNewStruct(map[string]interface{}{
							"latency":           map[string]interface{}{"total": "1", "unit": "msec"},
							"rows":              map[string]interface{}{"total": "9"},
							"execution_summary": map[string]interface{}{"num_executions": "1"},
						}),
					},
					{
						Index: 1,
						ChildLinks: []*sppb.PlanNode_ChildLink{
							{ChildIndex: 2},
						},
						DisplayName: "Distributed Union",
						Kind:        sppb.PlanNode_RELATIONAL,
						Metadata:    mustNewStruct(map[string]interface{}{"call_type": "Local"}),
						ExecutionStats: mustNewStruct(map[string]interface{}{
							"latency":           map[string]interface{}{"total": "1", "unit": "msec"},
							"rows":              map[string]interface{}{"total": "9"},
							"execution_summary": map[string]interface{}{"num_executions": "1"},
						}),
					},
					{
						Index: 2,
						ChildLinks: []*sppb.PlanNode_ChildLink{
							{ChildIndex: 3},
						},
						DisplayName: "Serialize Result",
						Kind:        sppb.PlanNode_RELATIONAL,
						ExecutionStats: mustNewStruct(map[string]interface{}{
							"latency":           map[string]interface{}{"total": "1", "unit": "msec"},
							"rows":              map[string]interface{}{"total": "9"},
							"execution_summary": map[string]interface{}{"num_executions": "1"},
						}),
					},
					{
						Index:       3,
						DisplayName: "Scan",
						Kind:        sppb.PlanNode_RELATIONAL,
						Metadata:    mustNewStruct(map[string]interface{}{"scan_type": "IndexScan", "scan_target": "SongsBySingerAlbumSongNameDesc", "Full scan": "true"}),
						ExecutionStats: mustNewStruct(map[string]interface{}{
							"latency":           map[string]interface{}{"total": "1", "unit": "msec"},
							"rows":              map[string]interface{}{"total": "9", "unit": "rows"},
							"scanned_rows":      map[string]interface{}{"total": "9", "unit": "rows"},
							"execution_summary": map[string]interface{}{"num_executions": "1"},
						}),
					},
				},
			},
			inlineStatsDefs: []inlineStatsDef{
				{
					Name: "scanned_rows",
					MapFunc: func(row plantree.RowWithPredicates) (string, error) {
						return row.ExecutionStats.ScannedRows.Total, nil
					},
				},
			},
			want: []plantree.RowWithPredicates{
				{
					ID: 0,
					ExecutionStats: stats.ExecutionStats{
						Rows:             stats.ExecutionStatsValue{Total: "9"},
						ExecutionSummary: stats.ExecutionStatsSummary{NumExecutions: "1"},
						Latency:          stats.ExecutionStatsValue{Total: "1", Unit: "msec"},
					},
					NodeText: "Distributed Union",
				},
				{
					ID:       1,
					TreePart: "+- ", NodeText: "Local Distributed Union",
					ExecutionStats: stats.ExecutionStats{
						Rows:             Total("9"),
						ExecutionSummary: stats.ExecutionStatsSummary{NumExecutions: "1"},
						Latency:          TotalWithUnit("1", "msec"),
					},
				},
				{
					ID:       2,
					TreePart: "   +- ", NodeText: "Serialize Result",
					ExecutionStats: stats.ExecutionStats{
						Rows:             Total("9"),
						ExecutionSummary: stats.ExecutionStatsSummary{NumExecutions: "1"},
						Latency:          TotalWithUnit("1", "msec"),
					},
				},
				{
					ID:       3,
					TreePart: "      +- ", NodeText: "Index Scan (Full scan: true, Index: SongsBySingerAlbumSongNameDesc, scanned_rows=9)",
					ExecutionStats: stats.ExecutionStats{
						Rows:             TotalWithUnit("9", "rows"),
						ExecutionSummary: stats.ExecutionStatsSummary{NumExecutions: "1"},
						Latency:          TotalWithUnit("1", "msec"),
						ScannedRows:      TotalWithUnit("9", "rows"),
					},
				},
			},
		},
	} {
		t.Run(test.title, func(t *testing.T) {
			var opts []plantree.Option
			if len(test.inlineStatsDefs) > 0 {
				opts = append(opts, plantree.WithQueryPlanOptions(
					spannerplan.WithInlineStatsFunc(inlineStatsFunc(test.inlineStatsDefs)),
				))
			}
			got, err := plantree.ProcessPlan(lo.Must(spannerplan.New(test.plan.GetPlanNodes())), opts...)
			if err != nil {
				t.Errorf("error should be nil, but got = %v", err)
			}
			if diff := cmp.Diff(test.want, got, planRowTextCmpOpts()...); diff != "" {
				t.Errorf("node.RenderTreeWithStats() differ: %s", diff)
			}
		})
	}
}

func TestNodeString(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		title string
		node  *sppb.PlanNode
		want  string
	}{
		{
			"Distributed Union with call_type=Local",
			&sppb.PlanNode{
				DisplayName: "Distributed Union",
				Metadata: mustNewStruct(map[string]interface{}{
					"call_type":             "Local",
					"subquery_cluster_node": "4",
				}),
			}, "Local Distributed Union",
		},
		{
			"Scan with scan_type=IndexScan and Full scan=true",
			&sppb.PlanNode{
				DisplayName: "Scan",
				Metadata: mustNewStruct(map[string]interface{}{
					"scan_type":   "IndexScan",
					"scan_target": "SongsBySongName",
					"Full scan":   "true",
				}),
			}, "Index Scan (Full scan: true, Index: SongsBySongName)",
		},
		{
			"Scan with scan_type=TableScan",
			&sppb.PlanNode{
				DisplayName: "Scan",
				Metadata: mustNewStruct(map[string]interface{}{
					"scan_type":   "TableScan",
					"scan_target": "Songs",
				}),
			}, "Table Scan (Table: Songs)",
		},
		{
			"Scan with scan_type=BatchScan",
			&sppb.PlanNode{
				DisplayName: "Scan",
				Metadata: mustNewStruct(map[string]interface{}{
					"scan_type":   "BatchScan",
					"scan_target": "$v2",
				}),
			}, "Batch Scan (Batch: $v2)",
		},
		{
			"Sort Limit with call_type=Local",
			&sppb.PlanNode{
				DisplayName: "Sort Limit",
				Metadata: mustNewStruct(map[string]interface{}{
					"call_type": "Local",
				}),
			}, "Local Sort Limit",
		},
		{
			"Sort Limit with call_type=Global",
			&sppb.PlanNode{
				DisplayName: "Sort Limit",
				Metadata: mustNewStruct(map[string]interface{}{
					"call_type": "Global",
				}),
			}, "Global Sort Limit",
		},
		{
			"Aggregate with iterator_type=Stream",
			&sppb.PlanNode{
				DisplayName: "Aggregate",
				Metadata: mustNewStruct(map[string]interface{}{
					"iterator_type": "Stream",
				}),
			}, "Stream Aggregate",
		},
	} {
		t.Run(test.title, func(t *testing.T) {
			if got := spannerplan.NodeTitle(test.node); got != test.want {
				t.Errorf("NodeTitle() = %q but want %q", got, test.want)
			}
		})
	}
}

//go:embed testdata/stats/select.json
var selectProfileJSON []byte
var selectProfileResultSet = lo.Must(protojsonUnmarshal[sppb.ResultSet](selectProfileJSON))

func TestExplainLastQueryStatement_Execute(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		statement      *ExplainLastQueryStatement
		lastQueryCache *LastQueryCache
		want           *Result
		wantErr        bool
	}{
		{"no cache", &ExplainLastQueryStatement{}, nil, nil, true},
		{"no query plan", &ExplainLastQueryStatement{}, &LastQueryCache{}, nil, true},
		{"EXPLAIN", &ExplainLastQueryStatement{}, &LastQueryCache{
			QueryPlan:  selectProfileResultSet.GetStats().GetQueryPlan(),
			QueryStats: selectProfileResultSet.GetStats().GetQueryStats().AsMap(),
		}, &Result{
			Rows:         sliceOf(toRow("0", "Serialize Result <Row>"), toRow("1", "+- Unit Relation <Row>")),
			AffectedRows: 2,
			ColumnAlign:  sliceOf(tw.AlignRight, tw.AlignLeft),
			TableHeader:  toTableHeader("ID", "Operator <execution_method> (metadata, ...)"),
		}, false},
		{"EXPLAIN ANALYZE", &ExplainLastQueryStatement{Analyze: true}, &LastQueryCache{
			QueryPlan:  selectProfileResultSet.GetStats().GetQueryPlan(),
			QueryStats: selectProfileResultSet.GetStats().GetQueryStats().AsMap(),
		}, &Result{
			Rows:         sliceOf(toRow("0", "Serialize Result <Row>", "1", "1", "0 msecs"), toRow("1", "+- Unit Relation <Row>", "1", "1", "0 msecs")),
			AffectedRows: 2,
			ColumnAlign:  sliceOf(tw.AlignRight, tw.AlignLeft, tw.AlignRight, tw.AlignRight, tw.AlignRight),
			TableHeader:  toTableHeader("ID", "Operator <execution_method> (metadata, ...)", "Rows", "Exec.", "Total Latency"),
			Stats: QueryStats{
				ElapsedTime:                "0.23 msecs",
				CPUTime:                    "0.2 msecs",
				RowsReturned:               "1",
				RowsScanned:                "0",
				DeletedRowsScanned:         "0",
				OptimizerVersion:           "7",
				OptimizerStatisticsPackage: "auto_20250604_03_26_04UTC",
				RemoteServerCalls:          "0/0",
				MemoryPeakUsageBytes:       "4",
				TotalMemoryPeakUsageByte:   "4",
				QueryText:                  "SELECT 1",
				BytesReturned:              "8",
				RuntimeCreationTime:        "",
				StatisticsLoadTime:         "0",
				MemoryUsagePercentage:      "0.000",
				FilesystemDelaySeconds:     "0 msecs",
				LockingDelay:               "0 msecs",
				ServerQueueDelay:           "0.01 msecs",
				IsGraphQuery:               "false",
				RuntimeCached:              "true",
				QueryPlanCached:            "true",
			},
			ForceVerbose: true,
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.statement.Execute(context.Background(), &Session{systemVariables: &systemVariables{
				Display:    DisplayVars{ParsedAnalyzeColumns: DefaultParsedAnalyzeColumns},
				LastResult: LastResult{QueryCache: tt.lastQueryCache},
			}})
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Errorf("Execute() diff = %v", diff)
				return
			}
		})
	}
}

func TestShowPlanNodeStatement_Execute(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		statement      *ShowPlanNodeStatement
		lastQueryCache *LastQueryCache
		wantIncoming   string
		wantErr        bool
	}{
		{
			"node with incoming parent link",
			&ShowPlanNodeStatement{NodeID: 1},
			&LastQueryCache{
				QueryPlan:  selectProfileResultSet.GetStats().GetQueryPlan(),
				QueryStats: selectProfileResultSet.GetStats().GetQueryStats().AsMap(),
			},
			"Incoming Parent Links:\n  - Parent Node Index: 0\n    Parent Node Title: Serialize Result (execution_method: Row)\n    Child Link Type: (not set)",
			false,
		},
		{
			"node with parent link including variable",
			&ShowPlanNodeStatement{NodeID: 1},
			&LastQueryCache{
				QueryPlan: &sppb.QueryPlan{
					PlanNodes: []*sppb.PlanNode{
						{
							Index:       0,
							DisplayName: "Root",
							Kind:        sppb.PlanNode_RELATIONAL,
							ChildLinks: []*sppb.PlanNode_ChildLink{
								{
									ChildIndex: 1,
									Type:       "Map",
									Variable:   "v",
								},
							},
						},
						{
							Index:       1,
							DisplayName: "Child",
							Kind:        sppb.PlanNode_RELATIONAL,
						},
					},
				},
			},
			"Incoming Parent Links:\n  - Parent Node Index: 0\n    Parent Node Title: Root\n    Child Link Type: Map\n    Variable: v",
			false,
		},
		{
			"node without incoming parent links",
			&ShowPlanNodeStatement{NodeID: 0},
			&LastQueryCache{
				QueryPlan: &sppb.QueryPlan{
					PlanNodes: []*sppb.PlanNode{
						{
							Index:       0,
							DisplayName: "Root",
							Kind:        sppb.PlanNode_RELATIONAL,
						},
						{
							Index:       1,
							DisplayName: "Child",
							Kind:        sppb.PlanNode_RELATIONAL,
						},
					},
				},
			},
			"Incoming Parent Links:\n  - No incoming parent links",
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.statement.Execute(context.Background(), &Session{systemVariables: &systemVariables{
				Display:    DisplayVars{ParsedAnalyzeColumns: DefaultParsedAnalyzeColumns},
				LastResult: LastResult{QueryCache: tt.lastQueryCache},
			}})
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if got != nil {
					t.Errorf("Execute() got = %v, want nil", got)
				}
				return
			}

			planNode := tt.lastQueryCache.QueryPlan.GetPlanNodes()[tt.statement.NodeID]
			y, err := protoyaml.Marshal(planNode, getGlobalOpts()...)
			if err != nil {
				t.Fatalf("yaml.Marshal(planNode, getGlobalOpts()) error = %v", err)
			}

			want := &Result{
				Rows:         sliceOf(toRow(string(y)), toRow(tt.wantIncoming)),
				AffectedRows: 2,
				TableHeader:  toTableHeader(fmt.Sprintf("Content of Node %v", tt.statement.NodeID)),
			}
			if diff := cmp.Diff(got, want); diff != "" {
				t.Errorf("Execute() diff = %v", diff)
				return
			}
		})
	}
}
