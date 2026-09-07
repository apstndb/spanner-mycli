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
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func tid(name string) tableID          { return tableID{Name: name} }
func tidn(schema, name string) tableID { return tableID{Schema: schema, Name: name} }
func parentPtr(id tableID) *tableID    { p := id; return &p }
func fqnList(ids []tableID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.FQN()
	}
	return out
}

func TestDependencyResolver_TopologicalSort(t *testing.T) {
	t.Parallel()
	dr := NewDependencyResolver()
	dr.tables = map[tableID]*TableDependency{
		tid("Singers"): {ID: tid("Singers")},
		tid("Albums"):  {ID: tid("Albums"), InterleaveParent: parentPtr(tid("Singers"))},
		tid("Songs"):   {ID: tid("Songs"), InterleaveParent: parentPtr(tid("Albums"))},
	}
	got, err := dr.GetOrderForTables([]tableID{tid("Songs"), tid("Albums"), tid("Singers")})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Singers", "Albums", "Songs"}
	if diff := cmp.Diff(want, fqnList(got)); diff != "" {
		t.Fatal(diff)
	}
}

func TestDependencyResolver_ForeignKeysAndPartial(t *testing.T) {
	t.Parallel()
	dr := NewDependencyResolver()
	dr.tables = map[tableID]*TableDependency{
		tid("Venues"):  {ID: tid("Venues")},
		tid("Singers"): {ID: tid("Singers")},
		tid("Concerts"): {
			ID:        tid("Concerts"),
			FKParents: []tableID{tid("Venues"), tid("Singers")},
		},
		tid("Child1"):    {ID: tid("Child1"), InterleaveParent: parentPtr(tid("Parent"))},
		tid("Parent"):    {ID: tid("Parent")},
		tid("Unrelated"): {ID: tid("Unrelated")},
	}
	got, err := dr.GetOrderForTables([]tableID{tid("Concerts"), tid("Venues"), tid("Singers")})
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]string{"Singers", "Venues", "Concerts"}, fqnList(got)); diff != "" {
		t.Fatal(diff)
	}
	partial, err := dr.GetOrderForTables([]tableID{tid("Child1"), tid("Unrelated")})
	if err != nil {
		t.Fatal(err)
	}
	if len(partial) != 2 || !slices.Contains(partial, tid("Child1")) || !slices.Contains(partial, tid("Unrelated")) {
		t.Fatalf("child-only subset: %v", partial)
	}
}

func TestDependencyResolver_NamedSchemaIdentity(t *testing.T) {
	t.Parallel()
	dr := NewDependencyResolver()
	dr.tables = map[tableID]*TableDependency{
		tid("Users"):           {ID: tid("Users")},
		tidn("Alpha", "Users"): {ID: tidn("Alpha", "Users")},
		tidn("Beta", "Child"): {
			ID:               tidn("Beta", "Child"),
			ParentBasename:   "Users",
			InterleaveParent: parentPtr(tidn("Alpha", "Users")),
		},
	}
	got, err := dr.GetOrderForTables([]tableID{tidn("Beta", "Child"), tidn("Alpha", "Users"), tid("Users")})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Users", "Alpha.Users", "Beta.Child"}
	if diff := cmp.Diff(want, fqnList(got)); diff != "" {
		t.Fatal(diff)
	}
}

func TestDependencyResolver_MissingAndNotBase(t *testing.T) {
	t.Parallel()
	dr := NewDependencyResolver()
	dr.tables = map[tableID]*TableDependency{tid("Existing"): {ID: tid("Existing")}}
	dr.objects = map[tableID]catalogObject{
		tid("Existing"): {ID: tid("Existing"), Type: "BASE TABLE"},
		tid("AView"):    {ID: tid("AView"), Type: "VIEW"},
	}
	if _, err := dr.GetOrderForTables([]tableID{tid("Missing")}); err == nil || !containsStr(err.Error(), "not found") {
		t.Fatalf("missing: %v", err)
	}
	if _, err := dr.GetOrderForTables([]tableID{tid("AView")}); err == nil || !containsStr(err.Error(), "not a base table") {
		t.Fatalf("view: %v", err)
	}
}

func TestSelectedNeedsInterleaveDDL(t *testing.T) {
	t.Parallel()
	dr := NewDependencyResolver()
	dr.tables = map[tableID]*TableDependency{
		tidn("Beta", "CrossChild"): {ID: tidn("Beta", "CrossChild"), ParentBasename: "Parent"},
		tidn("Alpha", "Parent"):    {ID: tidn("Alpha", "Parent")},
		tidn("Beta", "Parent"):     {ID: tidn("Beta", "Parent")},
	}
	if !dr.selectedNeedsInterleaveDDL([]tableID{tidn("Beta", "CrossChild"), tidn("Alpha", "Parent")}) {
		t.Fatal("expected GetDdl when another selected table shares parent basename")
	}
	if dr.selectedNeedsInterleaveDDL([]tableID{tidn("Beta", "CrossChild")}) {
		t.Fatal("child-only should not require GetDdl")
	}
}

func containsStr(s, substr string) bool {
	return strings.Contains(s, substr)
}

func TestIsKnownNotEnforced(t *testing.T) {
	t.Parallel()
	no, yes := "NO", "YES"
	if !isKnownNotEnforced(&no) {
		t.Fatal("NO must be known not-enforced")
	}
	if isKnownNotEnforced(&yes) || isKnownNotEnforced(nil) {
		t.Fatal("YES and missing ENFORCED are conservative safety edges")
	}
}

func TestGetOrderForTablesEmptyFKCycleSucceeds(t *testing.T) {
	t.Parallel()
	a, b := tid("A"), tid("B")
	dr := NewDependencyResolver()
	dr.tables = map[tableID]*TableDependency{
		a: {ID: a, FKParents: []tableID{b}, SafetyFKParents: []tableID{b}},
		b: {ID: b, FKParents: []tableID{a}, SafetyFKParents: []tableID{a}},
	}
	got, err := dr.GetOrderForTables([]tableID{b, a})
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]string{"A", "B"}, fqnList(got)); diff != "" {
		t.Fatal(diff)
	}
}

func TestCyclicSafetySCCs(t *testing.T) {
	t.Parallel()
	parent, child, other := tid("Parent"), tid("Child"), tid("Other")
	dr := NewDependencyResolver()
	dr.tables = map[tableID]*TableDependency{
		parent: {ID: parent},
		child: {
			ID:               child,
			InterleaveParent: parentPtr(parent),
			FKParents:        []tableID{other},
			SafetyFKParents:  []tableID{other},
		},
		other: {ID: other, FKParents: []tableID{child}, SafetyFKParents: []tableID{child}},
	}
	sccs := dr.cyclicSafetySCCs([]tableID{parent, child, other})
	if len(sccs) != 1 {
		t.Fatalf("sccs=%v", sccs)
	}
	if diff := cmp.Diff([]string{"Child", "Other"}, fqnList(sccs[0])); diff != "" {
		t.Fatal(diff)
	}
	got, err := dr.GetOrderForTables([]tableID{parent, child, other})
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != parent {
		t.Fatalf("order %v, want Parent first", fqnList(got))
	}

	self := tid("Emp")
	selfDR := NewDependencyResolver()
	selfDR.tables = map[tableID]*TableDependency{
		self: {ID: self, SafetyFKParents: []tableID{self}},
	}
	selfSCCs := selfDR.cyclicSafetySCCs([]tableID{self})
	if len(selfSCCs) != 1 || selfSCCs[0][0] != self {
		t.Fatalf("self scc=%v", selfSCCs)
	}

	infoA, infoB := tid("A"), tid("B")
	info := NewDependencyResolver()
	info.tables = map[tableID]*TableDependency{
		infoA: {ID: infoA, FKParents: []tableID{infoB}},
		infoB: {ID: infoB, FKParents: []tableID{infoA}},
	}
	if sccs := info.cyclicSafetySCCs([]tableID{infoA, infoB}); len(sccs) != 0 {
		t.Fatalf("NOT ENFORCED-equivalent empty SafetyFKParents must not be cyclic: %v", sccs)
	}
}

func TestCyclicSafetySCCsAncestorReverseFK(t *testing.T) {
	t.Parallel()
	parent, child := tid("AParent"), tid("ZChild")
	dr := NewDependencyResolver()
	dr.tables = map[tableID]*TableDependency{
		parent: {ID: parent, FKParents: []tableID{child}, SafetyFKParents: []tableID{child}},
		child:  {ID: child, InterleaveParent: parentPtr(parent)},
	}
	sccs := dr.cyclicSafetySCCs([]tableID{parent, child})
	if len(sccs) != 1 {
		t.Fatalf("sccs=%v", sccs)
	}
	if diff := cmp.Diff([]string{"AParent", "ZChild"}, fqnList(sccs[0])); diff != "" {
		t.Fatal(diff)
	}
}

func TestAncestorFKDoesNotRejectValidInterleave(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		parent tableID
		child  tableID
	}{
		{name: "parent sorts first", parent: tid("AParent"), child: tid("ZChild")},
		{name: "child sorts first", parent: tid("Parent"), child: tid("Child")},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			dr := NewDependencyResolver()
			dr.tables = map[tableID]*TableDependency{
				tt.parent: {ID: tt.parent, FKParents: []tableID{tt.child}},
				tt.child:  {ID: tt.child, InterleaveParent: parentPtr(tt.parent)},
			}
			got, err := dr.GetOrderForTables([]tableID{tt.parent, tt.child})
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff([]string{tt.parent.FQN(), tt.child.FQN()}, fqnList(got)); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func fourParentResolver() *DependencyResolver {
	dr := NewDependencyResolver()
	dr.tables = map[tableID]*TableDependency{
		tid("Parent"):                 {ID: tid("Parent")},
		tidn("Alpha", "Parent"):       {ID: tidn("Alpha", "Parent")},
		tidn("Beta", "Parent"):        {ID: tidn("Beta", "Parent")},
		tidn("Alpha", "SameChild"):    {ID: tidn("Alpha", "SameChild"), ParentBasename: "Parent"},
		tidn("Beta", "CrossChild"):    {ID: tidn("Beta", "CrossChild"), ParentBasename: "Parent"},
		tidn("Alpha", "DefaultChild"): {ID: tidn("Alpha", "DefaultChild"), ParentBasename: "Parent"},
		tid("NamedChild"):             {ID: tid("NamedChild"), ParentBasename: "Parent"},
	}
	return dr
}

func fourParentDDL() []string {
	return []string{
		"CREATE TABLE Parent (Id INT64 NOT NULL) PRIMARY KEY(Id)",
		"CREATE TABLE Alpha.Parent (Id INT64 NOT NULL) PRIMARY KEY(Id)",
		"CREATE TABLE Beta.Parent (Id INT64 NOT NULL) PRIMARY KEY(Id)",
		"CREATE TABLE Alpha.SameChild (Id INT64 NOT NULL, ChildId INT64 NOT NULL) PRIMARY KEY(Id, ChildId), INTERLEAVE IN PARENT Alpha.Parent ON DELETE CASCADE",
		"CREATE TABLE Beta.CrossChild (Id INT64 NOT NULL, ChildId INT64 NOT NULL) PRIMARY KEY(Id, ChildId), INTERLEAVE IN PARENT Alpha.Parent ON DELETE CASCADE",
		"CREATE TABLE Alpha.DefaultChild (Id INT64 NOT NULL, ChildId INT64 NOT NULL) PRIMARY KEY(Id, ChildId), INTERLEAVE IN PARENT Parent ON DELETE CASCADE",
		"CREATE TABLE NamedChild (Id INT64 NOT NULL, ChildId INT64 NOT NULL) PRIMARY KEY(Id, ChildId), INTERLEAVE IN PARENT Alpha.Parent ON DELETE CASCADE",
	}
}

func TestApplyInterleaveParentsFourMatrix(t *testing.T) {
	t.Parallel()
	ddl := fourParentDDL()
	all := []tableID{
		tid("Parent"), tidn("Alpha", "Parent"), tidn("Beta", "Parent"),
		tidn("Alpha", "SameChild"), tidn("Beta", "CrossChild"),
		tidn("Alpha", "DefaultChild"), tid("NamedChild"),
	}

	dr := fourParentResolver()
	if err := dr.applyInterleaveParents(ddl, all); err != nil {
		t.Fatal(err)
	}
	want := map[tableID]tableID{
		tidn("Alpha", "SameChild"):    tidn("Alpha", "Parent"),
		tidn("Beta", "CrossChild"):    tidn("Alpha", "Parent"),
		tidn("Alpha", "DefaultChild"): tid("Parent"),
		tid("NamedChild"):             tidn("Alpha", "Parent"),
	}
	for child, parent := range want {
		got := dr.tables[child].InterleaveParent
		if got == nil || *got != parent {
			t.Fatalf("%s parent = %v, want %s", child.FQN(), got, parent.FQN())
		}
	}

	childOnly := fourParentResolver()
	if err := childOnly.applyInterleaveParents(ddl, []tableID{tidn("Beta", "CrossChild")}); err != nil {
		t.Fatal(err)
	}
	if childOnly.tables[tidn("Beta", "CrossChild")].InterleaveParent != nil {
		t.Fatal("child-only must not add a parent edge")
	}

	unrelated := fourParentResolver()
	if err := unrelated.applyInterleaveParents(ddl, []tableID{tidn("Beta", "CrossChild"), tidn("Beta", "Parent")}); err != nil {
		t.Fatal(err)
	}
	if got := unrelated.tables[tidn("Beta", "CrossChild")].InterleaveParent; got != nil {
		t.Fatalf("unrelated same-basename must not bind %s", got.FQN())
	}

	trueParent := fourParentResolver()
	if err := trueParent.applyInterleaveParents(ddl, []tableID{tidn("Beta", "CrossChild"), tidn("Alpha", "Parent")}); err != nil {
		t.Fatal(err)
	}
	got := trueParent.tables[tidn("Beta", "CrossChild")].InterleaveParent
	if got == nil || *got != tidn("Alpha", "Parent") {
		t.Fatalf("true parent+child = %v", got)
	}
}

func TestGetOrderForTablesNamedFKAndSynonym(t *testing.T) {
	t.Parallel()
	dr := NewDependencyResolver()
	dr.tables = map[tableID]*TableDependency{
		tidn("Alpha", "Parent"): {ID: tidn("Alpha", "Parent")},
		tidn("Beta", "Parent"):  {ID: tidn("Beta", "Parent")},
		tidn("Alpha", "Child"):  {ID: tidn("Alpha", "Child"), FKParents: []tableID{tidn("Beta", "Parent")}},
		tidn("Beta", "Child"):   {ID: tidn("Beta", "Child"), FKParents: []tableID{tidn("Alpha", "Parent")}},
	}
	got, err := dr.GetOrderForTables([]tableID{
		tidn("Alpha", "Child"), tidn("Beta", "Child"),
		tidn("Alpha", "Parent"), tidn("Beta", "Parent"),
	})
	if err != nil {
		t.Fatal(err)
	}
	order := map[tableID]int{}
	for i, id := range got {
		order[id] = i
	}
	if order[tidn("Beta", "Parent")] > order[tidn("Alpha", "Child")] {
		t.Fatalf("Beta.Parent must precede Alpha.Child: %v", fqnList(got))
	}
	if order[tidn("Alpha", "Parent")] > order[tidn("Beta", "Child")] {
		t.Fatalf("Alpha.Parent must precede Beta.Child: %v", fqnList(got))
	}

	childOnly, err := dr.GetOrderForTables([]tableID{tidn("Alpha", "Child")})
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]string{"Alpha.Child"}, fqnList(childOnly)); diff != "" {
		t.Fatal(diff)
	}

	syn := NewDependencyResolver()
	syn.tables = map[tableID]*TableDependency{tid("Original"): {ID: tid("Original")}}
	syn.objects = map[tableID]catalogObject{
		tid("Original"): {ID: tid("Original"), Type: "BASE TABLE"},
		tid("Alias"):    {ID: tid("Alias"), Type: "SYNONYM"},
	}
	if _, err := syn.GetOrderForTables([]tableID{tid("Alias")}); err == nil || !containsStr(err.Error(), "not a base table") {
		t.Fatalf("synonym: %v", err)
	}

	dr = NewDependencyResolver()
	dr.tables = map[tableID]*TableDependency{
		tidn("Beta", "CrossChild"): {ID: tidn("Beta", "CrossChild"), ParentBasename: "Parent"},
	}
	dr.objects = map[tableID]catalogObject{
		tidn("Beta", "CrossChild"): {ID: tidn("Beta", "CrossChild"), Type: "BASE TABLE"},
		tid("Parent"):              {ID: tid("Parent"), Type: "SYNONYM"},
	}
	if err := dr.lookupExplicit(tid("Parent")); err == nil || !containsStr(err.Error(), "not a base table") {
		t.Fatalf("synonym candidate: %v", err)
	}
	if !dr.selectedNeedsInterleaveDDL([]tableID{tidn("Beta", "CrossChild"), tid("Parent")}) {
		t.Fatal("predicate still sees the same-basename synonym; caller must validate first")
	}
}
