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
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"net"
	"strings"
	"testing"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/enums"
	"github.com/apstndb/spanner-mycli/internal/mycli/format"
	"github.com/apstndb/spanner-mycli/internal/mycli/streamio"
	"github.com/apstndb/spanvalue"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestSQLLiteralFormatConfigFloat32(t *testing.T) {
	t.Parallel()
	negativeZero := float32(math.Copysign(0, -1))
	structValue := spanner.GenericColumnValue{
		Type: &sppb.Type{Code: sppb.TypeCode_STRUCT, StructType: &sppb.StructType{
			Fields: []*sppb.StructType_Field{{Name: "F", Type: &sppb.Type{Code: sppb.TypeCode_FLOAT32}}},
		}},
		Value: structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{structpb.NewNumberValue(math.Copysign(0, -1))}}),
	}
	for _, tc := range []struct {
		name  string
		value any
		want  string // Empty means preserve the upstream preset's existing bytes.
	}{
		{"negative zero", negativeZero, "CAST(-0.0 AS FLOAT32)"},
		{"nullable negative zero", spanner.NullFloat32{Float32: negativeZero, Valid: true}, "CAST(-0.0 AS FLOAT32)"},
		{"positive zero", float32(0), ""},
		{"integer", float32(3), ""},
		{"finite", float32(1.5), ""},
		{"NaN", float32(math.NaN()), ""},
		{"positive infinity", float32(math.Inf(1)), ""},
		{"negative infinity", float32(math.Inf(-1)), ""},
		{"NULL", spanner.NullFloat32{}, ""},
		{"FLOAT64 negative zero", math.Copysign(0, -1), ""},
		{"STRING with cast text", "CAST(-0 AS FLOAT32)", ""},
		{"JSON with cast text", spanner.NullJSON{Value: map[string]any{"text": "CAST(-0 AS FLOAT32)"}, Valid: true}, ""},
		{"array", []spanner.NullFloat32{{Float32: negativeZero, Valid: true}, {Valid: true}, {}}, "[CAST(-0.0 AS FLOAT32), CAST(0 AS FLOAT32), NULL]"},
		{"empty array", []spanner.NullFloat32{}, ""},
		{"NULL array", []spanner.NullFloat32(nil), ""},
		{"struct field", structValue, "STRUCT<F FLOAT32>(CAST(-0.0 AS FLOAT32))"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row, err := spanner.NewRow([]string{"V"}, []any{tc.value})
			if err != nil {
				t.Fatal(err)
			}
			var value spanner.GenericColumnValue
			if err := row.Column(0, &value); err != nil {
				t.Fatal(err)
			}
			want := tc.want
			if want == "" {
				want, err = spanvalue.LiteralFormatConfig().FormatToplevelColumn(value)
				if err != nil {
					t.Fatal(err)
				}
			}
			for _, mode := range []enums.DisplayMode{enums.DisplayModeSQLInsert, enums.DisplayModeSQLInsertOrIgnore, enums.DisplayModeSQLInsertOrUpdate} {
				sv := newSystemVariablesWithDefaults()
				sv.Display.CLIFormat = mode
				sv.Display.SQLTableName = "Values"
				render, err := prepareFormatConfig("SELECT V FROM Values", &sv, queryRenderingFrom(&sv))
				if err != nil {
					t.Fatal(err)
				}
				streaming := render.Spanvalue
				buffered, _, err := typedReplayFormatConfig(&sv)
				if err != nil {
					t.Fatal(err)
				}
				for name, config := range map[string]*spanvalue.FormatConfig{"streaming": streaming, "buffered": buffered} {
					got, err := config.FormatToplevelColumn(value)
					if err != nil {
						t.Fatal(err)
					}
					if got != want {
						t.Errorf("%s/%s got %s, want %s", mode, name, got, want)
					}
				}
			}
		})
	}
}

func TestSQLLiteralFormatConfigInvalidFloat32(t *testing.T) {
	t.Parallel()
	value := spanner.GenericColumnValue{
		Type: &sppb.Type{Code: sppb.TypeCode_FLOAT32}, Value: structpb.NewStringValue("not a float"),
	}
	sv := newSystemVariablesWithDefaults()
	sv.Display.CLIFormat = enums.DisplayModeSQLInsert
	config, _, err := typedReplayFormatConfig(&sv)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := config.FormatToplevelColumn(value); err == nil {
		t.Fatal("expected invalid FLOAT32 error")
	}
}

func TestPrepareFormatConfigSQLExportTableName(t *testing.T) {
	t.Parallel()

	t.Run("auto-detect fills render only", func(t *testing.T) {
		t.Parallel()
		sv := newSystemVariablesWithDefaults()
		sv.Display.CLIFormat = enums.DisplayModeSQLInsert
		sv.Display.SQLTableName = ""
		sv.LastResult.QueryCache = seedQueryCacheA()
		origRegistry := sv.Registry

		render, err := prepareFormatConfig("SELECT * FROM Users", &sv, queryRenderingFrom(&sv))
		if err != nil {
			t.Fatal(err)
		}
		if render.Export.SQLTableName != "Users" {
			t.Errorf("render SQLTableName = %q, want Users", render.Export.SQLTableName)
		}
		if sv.Display.SQLTableName != "" {
			t.Errorf("live SQLTableName = %q, want empty", sv.Display.SQLTableName)
		}
		if sv.LastResult.QueryCache == nil || sv.LastResult.QueryCache.QueryStats["query"] != "A" {
			t.Error("live QueryCache mutated")
		}
		if sv.Registry != origRegistry {
			t.Error("live Registry pointer changed")
		}
	})

	t.Run("explicit name wins over auto-detect", func(t *testing.T) {
		t.Parallel()
		sv := newSystemVariablesWithDefaults()
		sv.Display.CLIFormat = enums.DisplayModeSQLInsert
		sv.Display.SQLTableName = "Explicit"
		render, err := prepareFormatConfig("SELECT * FROM Users", &sv, queryRenderingFrom(&sv))
		if err != nil {
			t.Fatal(err)
		}
		if render.Export.SQLTableName != "Explicit" {
			t.Errorf("render SQLTableName = %q, want Explicit", render.Export.SQLTableName)
		}
		if sv.Display.SQLTableName != "Explicit" {
			t.Errorf("live SQLTableName = %q, want Explicit", sv.Display.SQLTableName)
		}
	})

	t.Run("failed auto-detect does not fail the query", func(t *testing.T) {
		t.Parallel()
		sv := newSystemVariablesWithDefaults()
		sv.Display.CLIFormat = enums.DisplayModeSQLInsert
		render, err := prepareFormatConfig("SELECT 1", &sv, queryRenderingFrom(&sv))
		if err != nil {
			t.Fatalf("prepareFormatConfig: %v", err)
		}
		if render.Export.SQLTableName != "" {
			t.Errorf("render SQLTableName = %q, want empty", render.Export.SQLTableName)
		}
		if sv.Display.SQLTableName != "" {
			t.Errorf("live SQLTableName = %q, want empty", sv.Display.SQLTableName)
		}
	})
}

func TestSQLLiteralFormatConfigInvalidFloat32Streaming(t *testing.T) {
	t.Parallel()
	value := spanner.GenericColumnValue{
		Type: &sppb.Type{Code: sppb.TypeCode_FLOAT32}, Value: structpb.NewStringValue("not a float"),
	}
	sv := newSystemVariablesWithDefaults()
	sv.Display.CLIFormat = enums.DisplayModeSQLInsert
	render, err := prepareFormatConfig("SELECT V FROM Values", &sv, queryRenderingFrom(&sv))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := render.Spanvalue.FormatToplevelColumn(value); err == nil {
		t.Fatal("expected invalid FLOAT32 error from sqlLiteralFormatConfig")
	}
}

func TestQueryRenderingFromNilAndExecuteOverrides(t *testing.T) {
	t.Parallel()

	if diff := cmp.Diff(queryRendering{}, queryRenderingFrom(nil)); diff != "" {
		t.Errorf("queryRenderingFrom(nil) mismatch (-want +got):\n%s", diff)
	}

	sv := newSystemVariablesWithDefaultsForTest()
	sv.Display.CLIFormat = enums.DisplayModeTable
	sv.Query.StreamingMode = enums.StreamingModeAuto
	sv.Display.SQLTableName = "KeepMe"
	sv.Display.SkipColumnNames = false
	orig := queryRenderingFrom(sv)

	got := orig.withExecuteOverrides(enums.DisplayModeSQLInsert, enums.StreamingModeTrue, "Users")
	if got.CLIFormat != enums.DisplayModeSQLInsert {
		t.Errorf("CLIFormat = %v, want SQL_INSERT", got.CLIFormat)
	}
	if got.StreamingMode != enums.StreamingModeTrue {
		t.Errorf("StreamingMode = %v, want TRUE", got.StreamingMode)
	}
	if got.Export.CLIFormat != enums.DisplayModeSQLInsert {
		t.Errorf("Export.CLIFormat = %v, want SQL_INSERT", got.Export.CLIFormat)
	}
	if got.Export.SQLTableName != "Users" {
		t.Errorf("Export.SQLTableName = %q, want Users", got.Export.SQLTableName)
	}
	if !got.Export.SkipColumnNames || !got.Formatter.SkipColumnNames {
		t.Error("DUMP overrides must skip column names")
	}
	if orig.CLIFormat != enums.DisplayModeTable || orig.StreamingMode != enums.StreamingModeAuto {
		t.Error("withExecuteOverrides mutated the original rendering value")
	}
	if sv.Display.CLIFormat != enums.DisplayModeTable || sv.Query.StreamingMode != enums.StreamingModeAuto {
		t.Error("withExecuteOverrides mutated live settings")
	}
	if sv.Display.SQLTableName != "KeepMe" {
		t.Errorf("live SQLTableName = %q, want KeepMe", sv.Display.SQLTableName)
	}

	keepName := orig.withExecuteOverrides(enums.DisplayModeCSV, enums.StreamingModeFalse, "")
	if keepName.Export.SQLTableName != "KeepMe" {
		t.Errorf("empty sqlTableName replaced Export.SQLTableName = %q, want KeepMe", keepName.Export.SQLTableName)
	}
}

func TestPrepareFormatConfigValueModes(t *testing.T) {
	t.Parallel()

	t.Run("JSONL uses JSON values", func(t *testing.T) {
		t.Parallel()
		sv := newSystemVariablesWithDefaultsForTest()
		sv.Display.CLIFormat = enums.DisplayModeJSONL
		render, err := prepareFormatConfig("SELECT 1", sv, queryRenderingFrom(sv))
		if err != nil {
			t.Fatalf("prepareFormatConfig: %v", err)
		}
		if render.ValueFmtMode != format.JSONValues {
			t.Errorf("ValueFmtMode = %v, want JSONValues", render.ValueFmtMode)
		}
		if render.Spanvalue == nil {
			t.Fatal("Spanvalue = nil")
		}
	})

	t.Run("TABLE with nil sysVars still builds a formatter", func(t *testing.T) {
		t.Parallel()
		render, err := prepareFormatConfig("SELECT 1", nil, queryRendering{})
		if err != nil {
			t.Fatalf("prepareFormatConfig: %v", err)
		}
		if render.ValueFmtMode != format.DisplayValues {
			t.Errorf("ValueFmtMode = %v, want DisplayValues", render.ValueFmtMode)
		}
		if render.Spanvalue == nil {
			t.Fatal("Spanvalue = nil")
		}
	})

	t.Run("TABLE with live settings", func(t *testing.T) {
		t.Parallel()
		sv := newSystemVariablesWithDefaultsForTest()
		sv.Display.CLIFormat = enums.DisplayModeTable
		render, err := prepareFormatConfig("SELECT 1", sv, queryRenderingFrom(sv))
		if err != nil {
			t.Fatalf("prepareFormatConfig: %v", err)
		}
		if render.ValueFmtMode != format.DisplayValues {
			t.Errorf("ValueFmtMode = %v, want DisplayValues", render.ValueFmtMode)
		}
		if render.Spanvalue == nil {
			t.Fatal("Spanvalue = nil")
		}
	})

	t.Run("invalid proto descriptor fails TABLE formatting", func(t *testing.T) {
		t.Parallel()
		sv := newSystemVariablesWithDefaultsForTest()
		sv.Display.CLIFormat = enums.DisplayModeTable
		sv.Internal.ProtoDescriptor = badProtoDescriptor()
		_, err := prepareFormatConfig("SELECT 1", sv, queryRenderingFrom(sv))
		if err == nil {
			t.Fatal("prepareFormatConfig error = nil, want proto descriptor failure")
		}
	})
}

func TestDisplayScreenWidth(t *testing.T) {
	t.Parallel()

	t.Run("AutoWrap false ignores width sources", func(t *testing.T) {
		t.Parallel()
		sv := newSystemVariablesWithDefaultsForTest()
		sv.Display.AutoWrap = false
		sv.Display.FixedWidth = int64Ptr(40)
		if got := displayScreenWidth(sv); got != math.MaxInt {
			t.Errorf("displayScreenWidth = %d, want MaxInt", got)
		}
	})

	t.Run("AutoWrap uses FixedWidth", func(t *testing.T) {
		t.Parallel()
		sv := newSystemVariablesWithDefaultsForTest()
		sv.Display.AutoWrap = true
		sv.Display.FixedWidth = int64Ptr(72)
		if got := displayScreenWidth(sv); got != 72 {
			t.Errorf("displayScreenWidth = %d, want 72", got)
		}
	})

	t.Run("AutoWrap without TTY does not wrap", func(t *testing.T) {
		t.Parallel()
		sv := newSystemVariablesWithDefaultsForTest()
		sv.Display.AutoWrap = true
		sv.StreamManager = streamio.NewStreamManager(io.NopCloser(bytes.NewReader(nil)), &bytes.Buffer{}, io.Discard)
		if got := displayScreenWidth(sv); got != math.MaxInt {
			t.Errorf("displayScreenWidth = %d, want MaxInt when terminal width is unknown", got)
		}
	})
}

func TestStreamingProcessorForAndDecideExecutionMode(t *testing.T) {
	t.Parallel()

	sv := newSystemVariablesWithDefaultsForTest()
	var out bytes.Buffer

	t.Run("table buffers unless StreamingModeTrue", func(t *testing.T) {
		render := queryRenderingFrom(sv)
		render.CLIFormat = enums.DisplayModeTable
		render.StreamingMode = enums.StreamingModeAuto
		proc, err := streamingProcessorFor(render, &out, 80)
		if err != nil {
			t.Fatalf("streamingProcessorFor: %v", err)
		}
		if proc != nil {
			t.Fatal("TABLE AUTO should buffer")
		}
		render.StreamingMode = enums.StreamingModeTrue
		proc, err = streamingProcessorFor(render, &out, 80)
		if err != nil {
			t.Fatalf("streamingProcessorFor TABLE TRUE: %v", err)
		}
		if _, ok := proc.(*TablePreviewProcessor); !ok {
			t.Fatalf("TABLE TRUE processor type = %T, want *TablePreviewProcessor", proc)
		}
	})

	t.Run("TAB streams for AUTO FALSE and TRUE", func(t *testing.T) {
		render := queryRenderingFrom(sv)
		render.CLIFormat = enums.DisplayModeTab
		for _, mode := range []enums.StreamingMode{enums.StreamingModeAuto, enums.StreamingModeFalse, enums.StreamingModeTrue} {
			render.StreamingMode = mode
			proc, err := streamingProcessorFor(render, &out, 80)
			if err != nil {
				t.Fatalf("mode %v: %v", mode, err)
			}
			if proc == nil {
				t.Fatalf("TAB mode %v returned nil processor", mode)
			}
		}
		render.StreamingMode = enums.StreamingMode(99)
		proc, err := streamingProcessorFor(render, &out, 80)
		if err != nil {
			t.Fatalf("unknown mode: %v", err)
		}
		if proc != nil {
			t.Fatal("unknown streaming mode should not stream TAB")
		}
	})

	t.Run("decideExecutionMode nil writer buffers", func(t *testing.T) {
		useStreaming, proc, err := decideExecutionMode(&queryExecution{
			Session: &Session{systemVariables: sv},
			Render:  queryRendering{CLIFormat: enums.DisplayModeTab},
		})
		if err != nil {
			t.Fatalf("decideExecutionMode: %v", err)
		}
		if useStreaming || proc != nil {
			t.Fatalf("nil writer useStreaming=%v proc=%T, want buffered", useStreaming, proc)
		}
	})

	t.Run("CSV streams without a RowProcessor", func(t *testing.T) {
		useStreaming, proc, err := decideExecutionMode(&queryExecution{
			Output: &out,
			Render: queryRendering{CLIFormat: enums.DisplayModeCSV},
		})
		if err != nil {
			t.Fatalf("decideExecutionMode: %v", err)
		}
		if !useStreaming || proc != nil {
			t.Fatalf("CSV useStreaming=%v proc=%T, want streaming with nil processor", useStreaming, proc)
		}
	})

	t.Run("TAB with destination streams through RowProcessor", func(t *testing.T) {
		session := &Session{systemVariables: sv}
		useStreaming, proc, err := decideExecutionMode(&queryExecution{
			Session: session,
			Output:  &out,
			Render:  queryRenderingFrom(sv).withExecuteOverrides(enums.DisplayModeTab, enums.StreamingModeTrue, ""),
		})
		if err != nil {
			t.Fatalf("decideExecutionMode: %v", err)
		}
		if !useStreaming || proc == nil {
			t.Fatalf("TAB useStreaming=%v proc=%T, want streaming processor", useStreaming, proc)
		}
	})

	t.Run("unsupported format fails decideExecutionMode", func(t *testing.T) {
		useStreaming, proc, err := decideExecutionMode(&queryExecution{
			Session: &Session{systemVariables: sv},
			Output:  &out,
			Render:  queryRendering{CLIFormat: enums.DisplayMode(999)},
		})
		if err == nil || !strings.Contains(err.Error(), "unsupported streaming mode") {
			t.Fatalf("decideExecutionMode error = %v, want unsupported streaming mode", err)
		}
		if useStreaming || proc != nil {
			t.Fatalf("error path useStreaming=%v proc=%T", useStreaming, proc)
		}
	})
}

func TestQueryExecutionOutputWriter(t *testing.T) {
	t.Parallel()

	explicit := &bytes.Buffer{}
	qe := &queryExecution{Output: explicit}
	if qe.outputWriter() != explicit {
		t.Fatal("outputWriter did not return the explicit destination")
	}

	sv := newSystemVariablesWithDefaultsForTest()
	sessionOut := &bytes.Buffer{}
	sv.StreamManager = streamio.NewStreamManager(io.NopCloser(bytes.NewReader(nil)), sessionOut, io.Discard)
	qe = &queryExecution{Session: &Session{systemVariables: sv}}
	if qe.outputWriter() != sessionOut {
		t.Fatal("outputWriter did not fall back to the session destination")
	}
}

func TestNewAndFinalizeMetrics(t *testing.T) {
	t.Parallel()

	sv := newSystemVariablesWithDefaultsForTest()
	sv.Query.Profile = false
	m := newMetrics(sv)
	if m.Profile || m.MemoryBefore != nil {
		t.Fatal("unprofiled metrics captured memory")
	}
	finalizeMetrics(m, sv)
	if m.CompletionTime.IsZero() || m.MemoryAfter != nil {
		t.Fatal("unprofiled finalize captured memory or skipped completion time")
	}

	sv.Query.Profile = true
	m = newMetrics(sv)
	if !m.Profile || m.MemoryBefore == nil {
		t.Fatal("profiled metrics skipped MemoryBefore")
	}
	finalizeMetrics(m, sv)
	if m.MemoryAfter == nil {
		t.Fatal("profiled finalize skipped MemoryAfter")
	}
}

func TestRollbackReadWriteIfAborted(t *testing.T) {
	t.Parallel()

	aborted := status.Error(codes.Aborted, "injected abort")
	other := errors.New("not aborted")

	if got := rollbackReadWriteIfAborted(t.Context(), nil, nil); got != nil {
		t.Errorf("nil err = %v, want nil", got)
	}
	if got := rollbackReadWriteIfAborted(t.Context(), nil, aborted); !errors.Is(got, aborted) {
		t.Errorf("nil session = %v, want original abort", got)
	}

	sv := newSystemVariablesWithDefaultsForTest()
	session := &Session{systemVariables: sv}
	if got := rollbackReadWriteIfAborted(t.Context(), session, aborted); !errors.Is(got, aborted) {
		t.Errorf("nil txn = %v, want original abort", got)
	}

	session.txn = NewTransactionManager(nil, sv, spanner.ClientConfig{})
	if got := rollbackReadWriteIfAborted(t.Context(), session, aborted); !errors.Is(got, aborted) {
		t.Errorf("not in RW = %v, want original abort", got)
	}
	if got := rollbackReadWriteIfAborted(t.Context(), session, other); !errors.Is(got, other) {
		t.Errorf("non-aborted = %v, want original error", got)
	}

	session.txn.tc = &transactionContext{attrs: transactionAttributes{mode: transactionModeReadWrite}}
	got := rollbackReadWriteIfAborted(t.Context(), session, aborted)
	if !errors.Is(got, aborted) {
		t.Fatalf("rollback-join missing abort: %v", got)
	}
	if !strings.Contains(got.Error(), "error on rollback") {
		t.Errorf("rollback-join = %v, want rollback wrapping", got)
	}
}

func mustNotInvokeQueryRunner(t *testing.T) queryWithStatsRunner {
	t.Helper()
	return func(context.Context, spanner.Statement, bool, sppb.ExecuteSqlRequest_QueryMode) (*spanner.RowIterator, *spanner.ReadOnlyTransaction, error) {
		t.Helper()
		t.Fatal("query runner invoked; validation should have failed first")
		return nil, nil, errors.New("unreachable")
	}
}

func TestExecuteSQLImplWithQueryRunnerErrorContracts(t *testing.T) {
	t.Parallel()

	runFail := func(context.Context, spanner.Statement, bool, sppb.ExecuteSqlRequest_QueryMode) (*spanner.RowIterator, *spanner.ReadOnlyTransaction, error) {
		return nil, nil, errors.New("injected runner failure")
	}

	t.Run("prepareFormatConfig error", func(t *testing.T) {
		t.Parallel()
		session := newSessionForLocalVarTest(t)
		session.systemVariables.Internal.ProtoDescriptor = badProtoDescriptor()
		_, want := prepareFormatConfig("SELECT 1", session.systemVariables, queryRenderingFrom(session.systemVariables))
		if want == nil {
			t.Fatal("setup: prepareFormatConfig error = nil, want proto descriptor failure")
		}
		_, err := executeSQLImplWithQueryRunner(t.Context(), session, "SELECT 1", session.systemVariables, mustNotInvokeQueryRunner(t), true)
		if err == nil || err.Error() != want.Error() {
			t.Fatalf("error = %v, want %v", err, want)
		}
	})

	t.Run("newStatement error", func(t *testing.T) {
		t.Parallel()
		session := newSessionForLocalVarTest(t)
		const sql = "SELECT '"
		_, want := newStatement(sql, session.systemVariables.Params, false)
		if want == nil {
			t.Fatal("setup: newStatement error = nil, want statement parse failure")
		}
		_, err := executeSQLImplWithQueryRunner(t.Context(), session, sql, session.systemVariables, mustNotInvokeQueryRunner(t), true)
		if err == nil || err.Error() != want.Error() {
			t.Fatalf("error = %v, want %v", err, want)
		}
	})

	t.Run("runner error", func(t *testing.T) {
		t.Parallel()
		session := newSessionForLocalVarTest(t)
		_, err := executeSQLImplWithQueryRunner(t.Context(), session, "SELECT 1", session.systemVariables, runFail, true)
		if err == nil || !strings.Contains(err.Error(), "injected runner failure") {
			t.Fatalf("error = %v, want injected runner failure", err)
		}
	})

	t.Run("unsupported format fails decideExecutionMode and stops the iterator", func(t *testing.T) {
		t.Parallel()
		session, live := newQueryCacheRPCSession(t, testQueryPlan(t), map[string]any{"query": "B"}, nil)
		live.Display.CLIFormat = enums.DisplayMode(999)
		var buf bytes.Buffer
		live.StreamManager = streamio.NewStreamManager(io.NopCloser(strings.NewReader("")), &buf, io.Discard)
		var iter *spanner.RowIterator
		run := func(ctx context.Context, stmt spanner.Statement, implicit bool, mode sppb.ExecuteSqlRequest_QueryMode) (*spanner.RowIterator, *spanner.ReadOnlyTransaction, error) {
			it, roTxn, err := session.txn.RunQueryWithStats(ctx, stmt, implicit, mode)
			iter = it
			return it, roTxn, err
		}
		_, err := executeSQLImplWithQueryRunner(t.Context(), session, sqlExportSelectUsers, live, run, true)
		if err == nil || !strings.Contains(err.Error(), "unsupported streaming mode") {
			t.Fatalf("error = %v, want unsupported streaming mode", err)
		}
		if iter == nil {
			t.Fatal("runner did not return an iterator")
		}
		_, nextErr := iter.Next()
		if nextErr == nil || spanner.ErrCode(nextErr) != codes.FailedPrecondition || !strings.Contains(nextErr.Error(), "Next called after Stop") {
			t.Fatalf("iterator Next() = %v, want Stop (FailedPrecondition Next called after Stop)", nextErr)
		}
	})

	t.Run("after-collect hook error without rollback", func(t *testing.T) {
		t.Parallel()
		session, live := newQueryCacheRPCSession(t, testQueryPlan(t), map[string]any{"elapsed_time": "1 msec", "query": "B"}, nil)
		injected := errors.New("after collect failed")
		session.txn.queryAfterCollectHook = func() error { return injected }
		t.Cleanup(func() { session.txn.queryAfterCollectHook = nil })
		_, err := executeSQLImplWithQueryRunner(t.Context(), session, sqlExportSelectUsers, live, session.txn.RunQueryWithStats, false)
		if !errors.Is(err, injected) {
			t.Fatalf("error = %v, want after collect failed", err)
		}
		if live.LastResult.QueryCache == nil {
			t.Fatal("hook failure cleared the already published cache")
		}
	})
}

func TestExecuteSQLBufferedAndStreamingEmptyResults(t *testing.T) {
	t.Parallel()

	plan := testQueryPlan(t)
	stats := map[string]any{"elapsed_time": "1 msec", "query": "empty"}

	t.Run("buffered empty SELECT", func(t *testing.T) {
		t.Parallel()
		session, live := newEmptySQLRPCSession(t, plan, stats)
		live.Display.CLIFormat = enums.DisplayModeTable
		result, err := executeSQL(t.Context(), session, sqlExportSelectUsers)
		if err != nil {
			t.Fatalf("executeSQL: %v", err)
		}
		if result.alreadyDelivered() {
			t.Fatal("TABLE without a writer should buffer")
		}
		if result.AffectedRows != 0 {
			t.Errorf("AffectedRows = %d, want 0", result.AffectedRows)
		}
		if result.TableHeader == nil {
			t.Fatal("TableHeader = nil for empty result")
		}
		if live.LastResult.QueryCache == nil {
			t.Fatal("empty buffered result did not publish QueryCache")
		}
		if result.Metrics == nil || result.Metrics.IsStreaming {
			t.Fatal("buffered empty result missing metrics or marked streaming")
		}
	})

	t.Run("streaming empty TAB", func(t *testing.T) {
		t.Parallel()
		session, live := newEmptySQLRPCSession(t, plan, stats)
		var buf bytes.Buffer
		live.Display.CLIFormat = enums.DisplayModeTab
		live.StreamManager = streamio.NewStreamManager(io.NopCloser(strings.NewReader("")), &buf, io.Discard)
		result, err := executeSQL(t.Context(), session, sqlExportSelectUsers)
		if err != nil {
			t.Fatalf("executeSQL: %v", err)
		}
		if !result.alreadyDelivered() {
			t.Fatal("TAB with a writer should stream")
		}
		if result.AffectedRows != 0 {
			t.Errorf("AffectedRows = %d, want 0", result.AffectedRows)
		}
		if !strings.Contains(buf.String(), "id") {
			t.Errorf("streamed output %q missing header", buf.String())
		}
	})

	t.Run("SQL export copies detected table name onto the result", func(t *testing.T) {
		t.Parallel()
		session, live := newEmptySQLRPCSession(t, plan, stats)
		live.Display.CLIFormat = enums.DisplayModeSQLInsert
		result, err := executeSQLImplWithVars(t.Context(), session, sqlExportSelectUsers, live)
		if err != nil {
			t.Fatalf("executeSQLImplWithVars: %v", err)
		}
		if result.SQLTableNameForExport != "Users" {
			t.Errorf("SQLTableNameForExport = %q, want Users", result.SQLTableNameForExport)
		}
	})

	t.Run("single-use empty SELECT", func(t *testing.T) {
		t.Parallel()
		session, live := newEmptySQLRPCSession(t, plan, stats)
		result, err := executeSQLImplSingleUse(t.Context(), session, sqlExportSelectUsers, live)
		if err != nil {
			t.Fatalf("executeSQLImplSingleUse: %v", err)
		}
		if result.AffectedRows != 0 {
			t.Errorf("AffectedRows = %d, want 0", result.AffectedRows)
		}
		if live.LastResult.QueryCache == nil {
			t.Fatal("single-use empty result did not publish QueryCache")
		}
	})

	t.Run("single-use iterator failure", func(t *testing.T) {
		t.Parallel()
		session, live := newQueryCacheRPCSession(t, plan, stats, status.Error(codes.Internal, "iterator failed"))
		seed := seedQueryCacheA()
		live.LastResult.QueryCache = seed
		_, err := executeSQLImplSingleUse(t.Context(), session, sqlExportSelectUsers, live)
		if err == nil {
			t.Fatal("executeSQLImplSingleUse error = nil, want iterator failure")
		}
		if live.LastResult.QueryCache != seed {
			t.Fatal("iterator failure replaced the live cache")
		}
	})
}

type emptySQLRPCServer struct {
	queryCacheRPCServer
}

func (s *emptySQLRPCServer) ExecuteStreamingSql(_ *sppb.ExecuteSqlRequest, stream sppb.Spanner_ExecuteStreamingSqlServer) error {
	if s.execErr != nil {
		return s.execErr
	}
	return stream.Send(&sppb.PartialResultSet{
		Metadata: s.resultSet().Metadata,
		Stats:    s.resultSet().Stats,
	})
}

func newEmptySQLRPCSession(t *testing.T, plan *sppb.QueryPlan, stats map[string]any) (*Session, *systemVariables) {
	t.Helper()
	listener := bufconn.Listen(1 << 20)
	grpcServer := grpc.NewServer()
	sppb.RegisterSpannerServer(grpcServer, &emptySQLRPCServer{queryCacheRPCServer: queryCacheRPCServer{
		plan:  plan,
		stats: mustNewStruct(stats),
	}})
	go func() {
		if err := grpcServer.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Errorf("serve: %v", err)
		}
	}()
	t.Cleanup(func() {
		grpcServer.Stop()
		_ = listener.Close()
	})
	conn, err := grpc.NewClient("passthrough:///empty-sql",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	client, err := spanner.NewClientWithConfig(t.Context(), "projects/test/instances/test/databases/test",
		spanner.ClientConfig{DisableNativeMetrics: true}, option.WithGRPCConn(conn))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)

	live := newSystemVariablesWithDefaultsForTest()
	session := &Session{
		mode:            DatabaseConnected,
		client:          client,
		systemVariables: live,
		txn:             NewTransactionManager(client, live, spanner.ClientConfig{DisableNativeMetrics: true}),
	}
	live.inTransaction = session.txn.InTransaction
	return session, live
}
