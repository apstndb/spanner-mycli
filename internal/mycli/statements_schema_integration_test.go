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
	"context"
	"strings"
	"testing"
	"time"
)

func TestShowChangeStreamsEmulator(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
	defer cancel()

	_, session := initializeWithRandomDB(t, []string{
		"CREATE TABLE Singers (SingerId INT64 NOT NULL, FirstName STRING(1024), LastName STRING(1024)) PRIMARY KEY(SingerId)",
		"CREATE CHANGE STREAM NamesAndAlbums FOR Singers OPTIONS (retention_period = '36h', value_capture_type = 'NEW_VALUES')",
		"CREATE CHANGE STREAM Everything FOR ALL",
	}, nil)

	stmt, err := BuildStatement("SHOW CHANGE STREAMS")
	if err != nil {
		t.Fatalf("parse SHOW CHANGE STREAMS: %v", err)
	}
	got, err := stmt.Execute(ctx, session, OperationOutput{})
	if err != nil {
		t.Fatalf("SHOW CHANGE STREAMS: %v", err)
	}
	if names := extractTableColumnNames(got.TableHeader); strings.Join(names, ",") != "Schema,Name,All" {
		t.Fatalf("headers = %v, want Schema,Name,All", names)
	}
	if got.AffectedRows != 2 {
		t.Fatalf("AffectedRows = %d, want 2; body=%v", got.AffectedRows, rowsText(got))
	}
	want := map[string]string{"Everything": "true", "NamesAndAlbums": "false"}
	for _, row := range got.presentationRows() {
		name, all := row[1].RawText(), row[2].RawText()
		if want[name] != all {
			t.Fatalf("stream %q All=%q, want %q (row=%v)", name, all, want[name], rowText(row))
		}
		delete(want, name)
	}
	if len(want) != 0 {
		t.Fatalf("missing streams: %v", want)
	}

	createStmt, err := BuildStatement("SHOW CREATE CHANGE STREAM Everything")
	if err != nil {
		t.Fatalf("parse SHOW CREATE: %v", err)
	}
	created, err := createStmt.Execute(ctx, session, OperationOutput{})
	if err != nil {
		t.Fatalf("SHOW CREATE CHANGE STREAM Everything: %v", err)
	}
	if created.AffectedRows != 1 || !strings.Contains(created.presentationRows()[0][1].RawText(), "FOR ALL") {
		t.Fatalf("SHOW CREATE = %+v", created)
	}
}

func rowsText(got *Result) []string {
	var out []string
	for _, row := range got.presentationRows() {
		out = append(out, rowText(row))
	}
	return out
}
