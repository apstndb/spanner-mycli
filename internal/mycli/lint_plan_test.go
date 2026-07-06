package mycli

import (
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/google/go-cmp/cmp"
)

// TestLintPlan exercises each heuristic implemented by lintPlan against a query
// plan that triggers it. Expected messages are hardcoded (not produced by the
// same formatting code under test) so the assertions verify the documented
// linter behavior described in docs/query_plan.md. The output slice is a flat
// list where each finding is a node-title header line followed by one or more
// indented ("    ") message lines.
func TestLintPlan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// file is a plan fixture under testdata/plans. Existing fixtures are
		// reused where they already trigger a rule; sort/minor_sort/
		// hash_aggregate/no_findings are minimal fixtures added specifically
		// for rules that no existing fixture exercised.
		file string
		// want is the complete expected lint output. nil means "no findings".
		want []string
	}{
		{
			// "Filter" display name -> Filter operator heuristic.
			name: "filter_operator",
			file: "testdata/plans/filter.input.json",
			want: []string{
				"1: Filter",
				"    Potentially expensive operator Filter can't utilize index: Maybe better to modify to use Filter Scan with Seek Condition?",
			},
		},
		{
			// "Hash Join" display name -> Hash Join heuristic; the same plan
			// also has "Full scan": "true" metadata on two scan nodes.
			name: "hash_join_and_full_scan",
			file: "testdata/plans/hash_join.input.json",
			want: []string{
				"2: Hash Join (join_type: INNER)",
				"    Potentially expensive operator Hash Join: Maybe better to modify to use Cross Apply or Merge Join?",
				"4: Table Scan (Full scan: true, Table: Albums)",
				"    Full scan=true: Potentially expensive execution full scan: Do you really want full scan?",
				"9: Index Scan (Full scan: true, Index: SongsBySingerAlbumSongNameDesc)",
				"    Full scan=true: Potentially expensive execution full scan: Do you really want full scan?",
			},
		},
		{
			// "Residual Condition" child link type -> Residual Condition
			// heuristic; the same plan also has a full-scan Table Scan.
			name: "residual_condition_and_full_scan",
			file: "testdata/plans/scalar_subquery_with_filter_scan.input.json",
			want: []string{
				"3: FilterScan",
				"    Residual Condition: Potentially expensive Residual Condition: Maybe better to modify it to Scan Condition",
				"4: Table Scan (Full scan: true, Table: Songs)",
				"    Full scan=true: Potentially expensive execution full scan: Do you really want full scan?",
			},
		},
		{
			// Full scan heuristic in isolation (single triggering node).
			name: "full_scan_only",
			file: "testdata/plans/array_subqueries.input.json",
			want: []string{
				"3: Index Scan (Full scan: true, Index: AlbumsByAlbumTitle)",
				"    Full scan=true: Potentially expensive execution full scan: Do you really want full scan?",
			},
		},
		{
			// "Sort" display name -> Sort heuristic, including the formatted
			// key order pulled from the "Key" child link. The key description
			// "$SingerId DESC" is resolved through the Scan node's "SingerId"
			// variable definition, so it renders as "SingerId DESC".
			name: "sort_operator",
			file: "testdata/plans/sort.input.json",
			want: []string{
				"0: Sort",
				"    Potentially expensive operator Sort: Maybe better to modify to use the same order with the index?, order: SingerId DESC",
			},
		},
		{
			// "Minor Sort" display name -> Minor Sort heuristic. This is
			// matched before the plain "Sort" branch even though the name
			// also contains "Sort"; the message reports major/minor keys.
			name: "minor_sort_operator",
			file: "testdata/plans/minor_sort.input.json",
			want: []string{
				"0: Minor Sort",
				"    Potentially expensive operator Minor Sort is cheaper than Sort but it may be not optimal: Maybe better to modify to use the same order with the index?: major: $SingerId ASC, minor: $AlbumId DESC",
			},
		},
		{
			// iterator_type=Hash metadata -> Hash iterator heuristic. The node
			// title is rendered as "Hash Aggregate" by spannerplan.
			name: "hash_iterator",
			file: "testdata/plans/hash_aggregate.input.json",
			want: []string{
				"0: Hash Aggregate",
				"    iterator_type=Hash: Potentially expensive execution Hash Aggregate: Maybe better to modify to use Stream Aggregate?, keys: $SingerId",
			},
		},
		{
			// A plan with a plain (non-full) Table Scan under Serialize Result
			// triggers no heuristic.
			name: "no_findings",
			file: "testdata/plans/no_findings.input.json",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			plan := loadTestPlan(t, tt.file)
			got := lintPlan(plan)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("lintPlan(%s) mismatch (-want +got):\n%s", tt.file, diff)
			}
		})
	}
}

// TestLintPlan_InvalidPlan verifies that lintPlan degrades gracefully: when
// spannerplan.New rejects the plan (here, a PlanNode whose Index does not match
// its slice position), the linter logs and returns no findings rather than
// panicking or surfacing an error, because linting is only informative.
func TestLintPlan_InvalidPlan(t *testing.T) {
	t.Parallel()

	// Index 5 at slice position 0 violates the Index-matches-position contract
	// enforced by spannerplan.New.
	plan := &sppb.QueryPlan{
		PlanNodes: []*sppb.PlanNode{
			{Index: 5, DisplayName: "Scan"},
		},
	}

	if got := lintPlan(plan); got != nil {
		t.Errorf("lintPlan(invalid plan) = %v, want nil", got)
	}
}
