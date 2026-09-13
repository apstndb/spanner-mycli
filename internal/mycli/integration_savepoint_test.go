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
	"context"
	"testing"
	"time"
)

func TestSavepointEmulatorCommitContents(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
	defer cancel()

	_, session := initializeWithRandomDB(t, testTableDDLs, nil)

	for _, sql := range []string{
		"SET CLI_SAVEPOINT_SUPPORT = 'ENABLED'",
		"BEGIN",
		"INSERT INTO tbl (id, active) VALUES (1, true)",
		"SAVEPOINT before_second_row",
		"INSERT INTO tbl (id, active) VALUES (2, false)",
		"ROLLBACK TO SAVEPOINT before_second_row",
		"COMMIT",
	} {
		stmt, err := BuildStatement(sql)
		if err != nil {
			t.Fatalf("BuildStatement(%q): %v", sql, err)
		}
		if _, err := stmt.Execute(ctx, session, OperationOutput{}); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}

	stmt, err := BuildStatement("SELECT id, active FROM tbl ORDER BY id ASC")
	if err != nil {
		t.Fatal(err)
	}
	result, err := stmt.Execute(ctx, session, OperationOutput{})
	if err != nil {
		t.Fatal(err)
	}
	compareResult(t, result, &Result{
		AffectedRows: 1,
		TableHeader:  toTableHeader(testTableRowType),
		Body: PresentationBody(sliceOf(
			toRow("1", "true"),
		)),
	})
}
