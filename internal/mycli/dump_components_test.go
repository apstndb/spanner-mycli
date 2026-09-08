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
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestOrderedSafetyComponents(t *testing.T) {
	t.Parallel()
	root, a, b, leaf, self := tid("ZRoot"), tidn("Alpha", "Node"), tidn("Beta", "Node"), tid("ALeaf"), tid("Self")
	dr := NewDependencyResolver()
	dr.tables = map[tableID]*TableDependency{
		root: {ID: root},
		a:    {ID: a, InterleaveParent: parentPtr(root), SafetyFKParents: []tableID{b}},
		b:    {ID: b, SafetyFKParents: []tableID{a, root}},
		leaf: {ID: leaf, SafetyFKParents: []tableID{a, b}},
		self: {ID: self, SafetyFKParents: []tableID{self}},
	}
	for _, tc := range []struct {
		name     string
		selected []tableID
		want     []dumpSafetyComponent
	}{
		{"full graph", []tableID{leaf, b, self, root, a}, []dumpSafetyComponent{
			{[]tableID{self}, true}, {[]tableID{root}, false}, {[]tableID{a, b}, true}, {[]tableID{leaf}, false},
		}},
		{"selected cycle", []tableID{b, a}, []dumpSafetyComponent{{[]tableID{a, b}, true}}},
		{"broken selected cycle", []tableID{leaf, a}, []dumpSafetyComponent{{[]tableID{a}, false}, {[]tableID{leaf}, false}}},
		{"duplicate selection", []tableID{b, a, b, a}, []dumpSafetyComponent{{[]tableID{a, b}, true}}},
		{"empty", nil, []dumpSafetyComponent{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := dr.orderedSafetyComponents(tc.selected)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
	if _, err := dr.orderedSafetyComponents([]tableID{tid("Missing")}); err == nil {
		t.Fatal("unknown selected table must be rejected")
	}
	// Reverse-ancestor FK must not receive the physical INSERT-order exception.
	dr.tables[root].SafetyFKParents = []tableID{a}
	got, err := dr.orderedSafetyComponents([]tableID{root, a})
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]dumpSafetyComponent{{[]tableID{root, a}, true}}, got); diff != "" {
		t.Fatal(diff)
	}
}

// Exhaust every directed three-table graph (including self edges) and selected
// subset. Transitive closure is an independent oracle for SCC membership, and
// every cross-component prerequisite must appear before its dependent.
func TestOrderedSafetyComponentsExhaustive(t *testing.T) {
	t.Parallel()
	ids := []tableID{tid("Z"), tidn("A", "Same"), tidn("B", "Same")}
	for edges := range 1 << 9 {
		for subset := 1; subset < 1<<3; subset++ {
			dr := NewDependencyResolver()
			var selected []tableID
			var reach [3][3]bool
			for i, id := range ids {
				dr.tables[id] = &TableDependency{ID: id}
				if subset&(1<<i) != 0 {
					selected = append(selected, id)
				}
				for j, parent := range ids {
					if edges&(1<<(i*3+j)) != 0 {
						dr.tables[id].SafetyFKParents = append(dr.tables[id].SafetyFKParents, parent)
						reach[i][j] = subset&(1<<i) != 0 && subset&(1<<j) != 0
					}
				}
			}
			for k := range 3 {
				for i := range 3 {
					for j := range 3 {
						reach[i][j] = reach[i][j] || reach[i][k] && reach[k][j]
					}
				}
			}
			got, err := dr.orderedSafetyComponents(selected)
			if err != nil {
				t.Fatalf("edges=%d subset=%d: %v", edges, subset, err)
			}
			positions := make(map[tableID]int)
			for i, comp := range got {
				for _, id := range comp.Tables {
					if _, exists := positions[id]; exists {
						t.Fatalf("duplicate %v", id)
					}
					positions[id] = i
					n := slices.Index(ids, id)
					if comp.Cyclic != reach[n][n] {
						t.Fatalf("edges=%d subset=%d cyclic=%v", edges, subset, comp)
					}
				}
			}
			if len(positions) != len(selected) {
				t.Fatalf("lost tables: %v", got)
			}
			for _, a := range selected {
				for _, b := range selected {
					i, j := slices.Index(ids, a), slices.Index(ids, b)
					if (positions[a] == positions[b]) != (a == b || reach[i][j] && reach[j][i]) {
						t.Fatalf("edges=%d subset=%d incorrect SCC: %v", edges, subset, got)
					}
					if reach[i][j] && positions[a] < positions[b] {
						t.Fatalf("dependency after child: %v", got)
					}
				}
			}
			slices.Reverse(selected)
			again, err := dr.orderedSafetyComponents(selected)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(got, again); diff != "" {
				t.Fatalf("input order changed output: %s", diff)
			}
		}
	}
}
