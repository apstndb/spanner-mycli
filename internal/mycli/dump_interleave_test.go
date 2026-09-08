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
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestExtractInterleaveParent(t *testing.T) {
	t.Parallel()

	simpleChild := `CREATE TABLE Child (
  Id INT64 NOT NULL,
  ChildId INT64 NOT NULL,
) PRIMARY KEY(Id, ChildId),
  INTERLEAVE IN PARENT Parent ON DELETE CASCADE`

	qualifiedChild := `CREATE TABLE Beta.CrossChild (
  Id INT64 NOT NULL,
  ChildId INT64 NOT NULL,
) PRIMARY KEY(Id, ChildId),
  INTERLEAVE IN PARENT Alpha.Parent ON DELETE CASCADE`

	interleaveIn := `CREATE TABLE Child (
  Id INT64 NOT NULL,
  ChildId INT64 NOT NULL,
) PRIMARY KEY(Id, ChildId),
  INTERLEAVE IN Root`

	reservedParent := "CREATE TABLE Child (Id INT64 NOT NULL, ChildId INT64 NOT NULL) PRIMARY KEY(Id, ChildId), INTERLEAVE IN PARENT `Order` ON DELETE CASCADE"

	complex := `CREATE TABLE Child (
  Id INT64 NOT NULL,
  ChildId INT64 NOT NULL,
  Name STRING(MAX),
  NameUpper STRING(MAX) AS (UPPER(Name)) STORED,
  CreatedAt TIMESTAMP OPTIONS (allow_commit_timestamp = true),
  CONSTRAINT FK_Parent FOREIGN KEY (Id) REFERENCES Parent (Id),
  CHECK (ChildId > 0),
) PRIMARY KEY(Id, ChildId),
  INTERLEAVE IN PARENT Alpha.Parent ON DELETE NO ACTION,
  ROW DELETION POLICY (OLDER_THAN(CreatedAt, INTERVAL 7 DAY))`

	noCluster := `CREATE TABLE Child (
  Id INT64 NOT NULL,
  ChildId INT64 NOT NULL,
) PRIMARY KEY(Id, ChildId)`

	tests := []struct {
		name              string
		statements        []string
		child             tableID
		catalogParentName string
		want              tableID
		wantErr           string
	}{
		{
			name:              "default INTERLEAVE IN PARENT",
			statements:        []string{simpleChild},
			child:             tid("Child"),
			catalogParentName: "Parent",
			want:              tid("Parent"),
		},
		{
			name:              "qualified INTERLEAVE IN PARENT",
			statements:        []string{qualifiedChild},
			child:             tidn("Beta", "CrossChild"),
			catalogParentName: "Parent",
			want:              tidn("Alpha", "Parent"),
		},
		{
			name:              "INTERLEAVE IN without PARENT",
			statements:        []string{interleaveIn},
			child:             tid("Child"),
			catalogParentName: "Root",
			want:              tid("Root"),
		},
		{
			name:              "reserved parent name",
			statements:        []string{reservedParent},
			child:             tid("Child"),
			catalogParentName: "Order",
			want:              tid("Order"),
		},
		{
			name:              "complex emitted clauses",
			statements:        []string{complex},
			child:             tid("Child"),
			catalogParentName: "Parent",
			want:              tidn("Alpha", "Parent"),
		},
		{
			name:              "missing DDL",
			statements:        []string{simpleChild},
			child:             tid("Missing"),
			catalogParentName: "Parent",
			wantErr:           "no CREATE TABLE DDL",
		},
		{
			name:              "duplicate DDL",
			statements:        []string{simpleChild, simpleChild},
			child:             tid("Child"),
			catalogParentName: "Parent",
			wantErr:           "duplicate CREATE TABLE DDL",
		},
		{
			name:              "basename mismatch",
			statements:        []string{qualifiedChild},
			child:             tidn("Beta", "CrossChild"),
			catalogParentName: "Other",
			wantErr:           "does not match catalog PARENT_TABLE_NAME",
		},
		{
			name:              "no cluster",
			statements:        []string{noCluster},
			child:             tid("Child"),
			catalogParentName: "Parent",
			wantErr:           "no INTERLEAVE clause",
		},
		{
			name:              "parse failure",
			statements:        []string{"CREATE TABLE Child ( not valid sql"},
			child:             tid("Child"),
			catalogParentName: "Parent",
			wantErr:           "parse CREATE TABLE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractInterleaveParent(tt.statements, tt.child, tt.catalogParentName)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestExtractInterleaveParentCurrentGetDdlCorpus(t *testing.T) {
	t.Parallel()
	// Focused corpus of CREATE TABLE forms GetDatabaseDdl currently emits or
	// that DUMP must still parse: both INTERLEAVE forms, default and named
	// parents, reserved identifiers, and extra table clauses.
	corpus := []struct {
		stmt   string
		child  tableID
		parent tableID
	}{
		{
			stmt:   "CREATE TABLE Child (Id INT64 NOT NULL, ChildId INT64 NOT NULL) PRIMARY KEY(Id, ChildId), INTERLEAVE IN PARENT Parent ON DELETE CASCADE",
			child:  tid("Child"),
			parent: tid("Parent"),
		},
		{
			stmt:   "CREATE TABLE Child (Id INT64 NOT NULL, ChildId INT64 NOT NULL) PRIMARY KEY(Id, ChildId), INTERLEAVE IN Root",
			child:  tid("Child"),
			parent: tid("Root"),
		},
		{
			stmt:   "CREATE TABLE Beta.Child (Id INT64 NOT NULL, ChildId INT64 NOT NULL) PRIMARY KEY(Id, ChildId), INTERLEAVE IN PARENT Alpha.Parent ON DELETE CASCADE",
			child:  tidn("Beta", "Child"),
			parent: tidn("Alpha", "Parent"),
		},
		{
			stmt:   "CREATE TABLE `Order`.Child (Id INT64 NOT NULL, ChildId INT64 NOT NULL) PRIMARY KEY(Id, ChildId), INTERLEAVE IN PARENT `Order`.`Parent` ON DELETE CASCADE",
			child:  tidn("Order", "Child"),
			parent: tidn("Order", "Parent"),
		},
	}
	for _, tt := range corpus {
		got, err := extractInterleaveParent([]string{tt.stmt}, tt.child, tt.parent.Name)
		if err != nil {
			t.Fatalf("stmt %q: %v", tt.stmt, err)
		}
		if diff := cmp.Diff(tt.parent, got); diff != "" {
			t.Fatalf("stmt %q: %s", tt.stmt, diff)
		}
	}
}
