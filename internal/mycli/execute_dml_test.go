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

	"cloud.google.com/go/spanner"
	"github.com/apstndb/spancodec"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanner-mycli/internal/mycli/decoder"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// thenReturnRow is a single INT64 column used to exercise THEN RETURN rendering.
type thenReturnRow struct {
	N int64 `spanner:"n"`
}

// TestRenderDMLReturnedRowsJSONLTypedFidelity is the behavior-change test for
// issue #738 PR2. THEN RETURN rows are now rendered from raw typed values at
// display time, so under CLI_FORMAT=JSONL an INT64 column is emitted as a JSON
// number ({"n":1}). Before PR2 the rows were eagerly converted to display-text
// cells inside the transaction with the display FormatConfig, so JSONL replay
// treated them as strings and emitted the type-unfaithful {"n":"1"}.
//
// This test pins the new (correct) output and documents the old (buggy) output
// that the previous eager display-cell path produced.
func TestRenderDMLReturnedRowsJSONLTypedFidelity(t *testing.T) {
	t.Parallel()

	enc := spancodec.MustNewRowEncoder[thenReturnRow]()
	md, err := enc.ResultSetMetadata()
	if err != nil {
		t.Fatalf("ResultSetMetadata: %v", err)
	}
	var rawRows []*spanner.Row
	for row, err := range enc.Rows([]thenReturnRow{{N: 1}}) {
		if err != nil {
			t.Fatalf("encode row: %v", err)
		}
		rawRows = append(rawRows, row)
	}
	tableHeader := toTableHeader(md.GetRowType().GetFields())

	sv := newSystemVariablesWithDefaults()
	sv.Display.CLIFormat = enums.DisplayModeJSONL

	// New typed path: INT64 stays a JSON number.
	got, err := renderDMLReturnedRows(&sv, tableHeader, md, rawRows)
	if err != nil {
		t.Fatalf("renderDMLReturnedRows: %v", err)
	}
	const wantNew = `{"n":1}` + "\n"
	if diff := cmp.Diff(wantNew, string(got)); diff != "" {
		t.Errorf("typed THEN RETURN JSONL output mismatch (-want +got):\n%s", diff)
	}

	// Document the pre-PR2 behavior: eager display cells rendered under JSONL
	// treated the number as a string, emitting {"n":"1"}.
	displayFC, err := decoder.FormatConfigWithProto(nil, false)
	if err != nil {
		t.Fatalf("FormatConfigWithProto: %v", err)
	}
	transform := spannerRowToRow(displayFC, nil, "")
	var displayRows []Row
	for _, row := range rawRows {
		cells, err := transform(row)
		if err != nil {
			t.Fatalf("transform: %v", err)
		}
		displayRows = append(displayRows, cells)
	}
	oldOut, err := runPrintTableData(t, enums.DisplayModeJSONL, false, &Result{
		TableHeader: tableHeader,
		Rows:        displayRows,
	})
	if err != nil {
		t.Fatalf("printTableData(old shape): %v", err)
	}
	if oldOut == wantNew {
		t.Fatalf("expected the pre-PR2 display-cell path to differ from the typed output; both were %q", oldOut)
	}
	if oldOut != `{"n":"1"}`+"\n" {
		t.Errorf("pre-PR2 display-cell JSONL output = %q, want %q", oldOut, `{"n":"1"}`+"\n")
	}
}

// badProtoDescriptor is a FileDescriptorSet with an unresolvable import, so
// decoder.FormatConfigWithProto fails deterministically. It is used to induce a
// render failure independent of CLI_FORMAT.
func badProtoDescriptor() *descriptorpb.FileDescriptorSet {
	return &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{
			{Name: proto.String("bad.proto"), Dependency: []string{"nonexistent.proto"}},
		},
	}
}

// TestThenReturnRenderFailureAbortsImplicitCommit proves invariant 4 (issue #738
// section 6): THEN RETURN rendering happens inside the DML transaction callback,
// so a render failure aborts the implicit commit instead of committing without
// output. It induces a deterministic render failure via an invalid proto
// descriptor and asserts the inserted row was NOT committed. If the render were
// moved after the commit, the row would persist and this test would fail.
func TestThenReturnRenderFailureAbortsImplicitCommit(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
	defer cancel()

	_, session := initializeWithRandomDB(t, testTableDDLs, nil)

	// Force renderDMLReturnedRows to fail without affecting query execution.
	session.systemVariables.Internal.ProtoDescriptor = badProtoDescriptor()

	stmt, err := BuildStatement("INSERT INTO tbl (id, active) VALUES (42, true) THEN RETURN *")
	if err != nil {
		t.Fatalf("invalid statement: %v", err)
	}
	if _, execErr := stmt.Execute(ctx, session); execErr == nil {
		t.Fatalf("expected THEN RETURN render failure, got nil error")
	}

	// The render failed inside the transaction callback, so the implicit commit
	// must have been aborted. Verify the row is absent with a clean descriptor.
	session.systemVariables.Internal.ProtoDescriptor = nil
	sel, err := BuildStatement("SELECT COUNT(*) AS c FROM tbl WHERE id = 42")
	if err != nil {
		t.Fatalf("invalid select: %v", err)
	}
	res, err := sel.Execute(ctx, session)
	if err != nil {
		t.Fatalf("verification select failed: %v", err)
	}
	rows, err := deriveDisplayRows(session.systemVariables, res.Typed)
	if err != nil {
		t.Fatalf("deriveDisplayRows: %v", err)
	}
	if len(rows) != 1 || len(rows[0]) != 1 {
		t.Fatalf("unexpected COUNT(*) shape: %v", rows)
	}
	if got := rows[0][0].RawText(); got != "0" {
		t.Errorf("row was committed despite render failure: COUNT(*) = %s, want 0", got)
	}
}
