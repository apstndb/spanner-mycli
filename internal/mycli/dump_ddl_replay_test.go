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

	"github.com/cloudspannerecosystem/memefish/ast"
	"github.com/google/go-cmp/cmp"
)

func TestPrepareDumpDDLForReplayUnchanged(t *testing.T) {
	t.Parallel()
	input := []string{
		"CREATE SCHEMA S",
		"CREATE TABLE S.P (Id INT64 NOT NULL, SYNONYM (Parent)) PRIMARY KEY(Id)",
		"CREATE TABLE C (Id INT64 NOT NULL, Ref INT64, CONSTRAINT fk FOREIGN KEY(Ref) REFERENCES S.p(Id)) PRIMARY KEY(Id)",
		"CREATE TABLE D (Id INT64 NOT NULL, CONSTRAINT fk_self FOREIGN KEY(Id) REFERENCES d(Id)) PRIMARY KEY(Id)",
		"CREATE TABLE E (Id INT64 NOT NULL, CONSTRAINT fk_alias FOREIGN KEY(Id) REFERENCES S.Parent(Id)) PRIMARY KEY(Id)",
		"CREATE SOMETHING FUTURE SYNTAX THAT THIS PARSER DOES NOT KNOW",
		"CREATE TABLE Future (Id INT64 NOT NULL) PRIMARY KEY(Id) NEW_UNPARSED_OPTION ('FOREIGN KEY')",
		"CREATE TABLE F (Id INT64 NOT NULL, FOREIGN KEY(Id) REFERENCES Future(Id)) PRIMARY KEY(Id)",
		"CREATE TABLE IF NOT EXISTS Other (Id INT64 NOT NULL) PRIMARY KEY(Id) NEW_UNPARSED_OPTION (1)",
	}
	want := slices.Clone(input)
	got, err := prepareDumpDDLForReplay(input)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("changed valid DDL (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(want, input); diff != "" {
		t.Fatalf("mutated input: %s", diff)
	}
}

func TestPrepareDumpDDLForReplayConstraintPositions(t *testing.T) {
	t.Parallel()
	const col = "Id INT64 NOT NULL"
	const fk1 = "CONSTRAINT fk1 FOREIGN KEY(Id) REFERENCES B(Id) ON DELETE CASCADE"
	const fk2 = "CONSTRAINT fk2 FOREIGN KEY(Id) REFERENCES B(Id) NOT ENFORCED"
	const check = "CONSTRAINT ck CHECK(Id > 0)"
	for _, decls := range []string{
		col + ", " + fk1,
		col + ", " + fk1 + ",",
		fk1 + ", " + col,
		col + ", " + fk1 + ", " + check,
		col + ", " + fk1 + ", " + fk2,
		col + ", " + fk1 + ", " + fk2 + ",",
		fk1 + ", " + fk2 + ", " + col,
		col + ", " + fk1 + ", " + check + ", " + fk2 + ",",
		col + ", /* keep , comment */ " + fk1 + " /* another , */ , -- keep line\n " + fk2 + ",",
	} {
		t.Run(decls, func(t *testing.T) {
			source := "/* leading */ cReAtE TABLE A (" + decls + ") PRIMARY KEY(Id)"
			input := []string{source, "CREATE TABLE B (Id INT64 NOT NULL) PRIMARY KEY(Id)"}
			got, err := prepareDumpDDLForReplay(input)
			if err != nil {
				t.Fatal(err)
			}
			wantMoved := 1
			if strings.Contains(decls, "fk2") {
				wantMoved++
			}
			if len(got) != 2+wantMoved || got[1] != input[1] {
				t.Fatalf("unexpected result: %v", got)
			}
			if got[2] != "ALTER TABLE A ADD "+fk1 {
				t.Fatalf("lost FK identity/action: %s", got[2])
			}
			if wantMoved == 2 && got[3] != "ALTER TABLE A ADD "+fk2 {
				t.Fatalf("lost enforcement: %s", got[3])
			}
			for _, fragment := range []string{col, "/* leading */", "/* keep , comment */", "/* another , */", "-- keep line", check} {
				if strings.Contains(source, fragment) && !strings.Contains(got[0], fragment) {
					t.Errorf("lost original %q", fragment)
				}
			}
			parsed, err := parseMemefishDDL("", got[0])
			if err != nil {
				t.Fatal(err)
			}
			for _, constraint := range parsed.(*ast.CreateTable).TableConstraints {
				if _, ok := constraint.Constraint.(*ast.ForeignKey); ok {
					t.Fatal("forward FK left inline")
				}
			}
		})
	}
}

func TestPrepareDumpDDLForReplayNamedTables(t *testing.T) {
	t.Parallel()
	input := []string{
		"CREATE SCHEMA Alpha", "CREATE SCHEMA Beta",
		"CREATE TABLE `Alpha`.`Order` (`Id` INT64 NOT NULL, CONSTRAINT `select` FOREIGN KEY(`Id`) REFERENCES `Beta`.`Order`(`Id`) NOT ENFORCED, SYNONYM (OldOrder)) PRIMARY KEY(`Id`)",
		"CREATE TABLE `Beta`.`Order` (`Id` INT64 NOT NULL) PRIMARY KEY(`Id`), INTERLEAVE IN PARENT `Alpha`.`Order` ON DELETE CASCADE",
	}
	got, err := prepareDumpDDLForReplay(input)
	if err != nil {
		t.Fatal(err)
	}
	want := "ALTER TABLE `Alpha`.`Order` ADD CONSTRAINT `select` FOREIGN KEY(`Id`) REFERENCES `Beta`.`Order`(`Id`) NOT ENFORCED"
	if got[len(got)-1] != want {
		t.Fatalf("got %s, want %s", got[len(got)-1], want)
	}
	if got[3] != input[3] || !strings.Contains(got[2], "SYNONYM (OldOrder)") {
		t.Fatal("interleave or synonym changed")
	}
}

func TestPrepareDumpDDLForReplayErrors(t *testing.T) {
	t.Parallel()
	for _, input := range [][]string{
		{"CREATE TABLE T (Id INT64 NOT NULL, FOREIGN KEY(Id) REFERENCES Missing(Id)) PRIMARY KEY(Id)"},
		{"CREATE TABLE T (Id INT64 NOT NULL) PRIMARY KEY(Id)", "CREATE TABLE t (Id INT64 NOT NULL) PRIMARY KEY(Id)"},
		{"CREATE TABLE T (Id INT64 NOT NULL, FOREIGN KEY(Id) REFERENCES B()"},
		{"CREATE TABLE T (Id INT64 NOT NULL, FOREIGN KEY(Id) REFERENCES B(Id)) PRIMARY KEY(Id) NEW_UNPARSED_OPTION (1)"},
	} {
		if got, err := prepareDumpDDLForReplay(input); err == nil || got != nil {
			t.Fatalf("got %v, %v, want pre-output error", got, err)
		}
	}
}
