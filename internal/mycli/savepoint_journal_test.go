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
	"encoding/base64"
	"errors"
	"slices"
	"testing"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanvalue/gcvctor"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestSavepointFingerprintTypedDistinctions(t *testing.T) {
	t.Parallel()

	intField := []*sppb.StructType_Field{{Name: "v", Type: &sppb.Type{Code: sppb.TypeCode_INT64}}}
	strField := []*sppb.StructType_Field{{Name: "v", Type: &sppb.Type{Code: sppb.TypeCode_STRING}}}
	bytesField := []*sppb.StructType_Field{{Name: "v", Type: &sppb.Type{Code: sppb.TypeCode_BYTES}}}
	jsonField := []*sppb.StructType_Field{{Name: "v", Type: &sppb.Type{Code: sppb.TypeCode_JSON}}}
	protoField := []*sppb.StructType_Field{{Name: "v", Type: &sppb.Type{Code: sppb.TypeCode_PROTO, ProtoTypeFqn: "example.Message"}}}
	enumField := []*sppb.StructType_Field{{Name: "v", Type: &sppb.Type{Code: sppb.TypeCode_ENUM, ProtoTypeFqn: "example.Kind"}}}
	arrayField := []*sppb.StructType_Field{{Name: "v", Type: &sppb.Type{Code: sppb.TypeCode_ARRAY, ArrayElementType: &sppb.Type{Code: sppb.TypeCode_INT64}}}}
	structField := []*sppb.StructType_Field{{Name: "v", Type: &sppb.Type{
		Code: sppb.TypeCode_STRUCT,
		StructType: &sppb.StructType{Fields: []*sppb.StructType_Field{
			{Name: "a", Type: &sppb.Type{Code: sppb.TypeCode_INT64}},
			{Name: "b", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
		}},
	}}}

	intOne := gcvctor.Int64Value(1)
	strOne := gcvctor.StringValue("1")
	emptyStr := gcvctor.StringValue("")
	nullStr := gcvctor.NullFromCode(sppb.TypeCode_STRING)
	emptyBytes := spanner.GenericColumnValue{Type: &sppb.Type{Code: sppb.TypeCode_BYTES}, Value: structpb.NewStringValue("")}
	nullBytes := gcvctor.NullFromCode(sppb.TypeCode_BYTES)
	emptyJSON := spanner.GenericColumnValue{Type: &sppb.Type{Code: sppb.TypeCode_JSON}, Value: structpb.NewStringValue("{}")}
	jsonObj := spanner.GenericColumnValue{Type: &sppb.Type{Code: sppb.TypeCode_JSON}, Value: structpb.NewStringValue(`{"a":1}`)}
	protoVal := spanner.GenericColumnValue{Type: proto.Clone(protoField[0].Type).(*sppb.Type), Value: structpb.NewStringValue(base64.StdEncoding.EncodeToString([]byte{0x08, 0x07}))}
	enumVal := spanner.GenericColumnValue{Type: proto.Clone(enumField[0].Type).(*sppb.Type), Value: structpb.NewStringValue("3")}
	emptyArray := spanner.GenericColumnValue{
		Type:  proto.Clone(arrayField[0].Type).(*sppb.Type),
		Value: structpb.NewListValue(&structpb.ListValue{}),
	}
	nullArray := gcvctor.NullOf(arrayField[0].Type)
	array12 := spanner.GenericColumnValue{
		Type: proto.Clone(arrayField[0].Type).(*sppb.Type),
		Value: structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{
			structpb.NewStringValue("1"),
			structpb.NewStringValue("2"),
		}}),
	}
	array21 := spanner.GenericColumnValue{
		Type: proto.Clone(arrayField[0].Type).(*sppb.Type),
		Value: structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{
			structpb.NewStringValue("2"),
			structpb.NewStringValue("1"),
		}}),
	}
	structAB := spanner.GenericColumnValue{
		Type: proto.Clone(structField[0].Type).(*sppb.Type),
		Value: structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{
			structpb.NewStringValue("1"),
			structpb.NewStringValue("x"),
		}}),
	}
	structBA := spanner.GenericColumnValue{
		Type: proto.Clone(structField[0].Type).(*sppb.Type),
		Value: structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{
			structpb.NewStringValue("2"),
			structpb.NewStringValue("x"),
		}}),
	}

	mustHash := func(t *testing.T, fields []*sppb.StructType_Field, rows ...[]spanner.GenericColumnValue) []byte {
		t.Helper()
		fp, err := newResultFingerprinter(fields)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			if err := fp.ObserveGCVs(row); err != nil {
				t.Fatal(err)
			}
		}
		sum, err := fp.FinishQuery()
		if err != nil {
			t.Fatal(err)
		}
		return sum
	}

	if bytes.Equal(mustHash(t, intField, []spanner.GenericColumnValue{intOne}), mustHash(t, strField, []spanner.GenericColumnValue{strOne})) {
		t.Fatal("INT64 1 and STRING 1 hashed equal")
	}
	if bytes.Equal(mustHash(t, strField, []spanner.GenericColumnValue{emptyStr}), mustHash(t, strField, []spanner.GenericColumnValue{nullStr})) {
		t.Fatal("empty STRING and NULL STRING hashed equal")
	}
	if bytes.Equal(mustHash(t, bytesField, []spanner.GenericColumnValue{emptyBytes}), mustHash(t, bytesField, []spanner.GenericColumnValue{nullBytes})) {
		t.Fatal("empty BYTES and NULL BYTES hashed equal")
	}
	if bytes.Equal(mustHash(t, jsonField, []spanner.GenericColumnValue{emptyJSON}), mustHash(t, jsonField, []spanner.GenericColumnValue{jsonObj})) {
		t.Fatal("JSON {} and {a:1} hashed equal")
	}
	if bytes.Equal(mustHash(t, protoField, []spanner.GenericColumnValue{protoVal}), mustHash(t, enumField, []spanner.GenericColumnValue{enumVal})) {
		t.Fatal("PROTO and ENUM hashed equal")
	}
	if bytes.Equal(mustHash(t, arrayField, []spanner.GenericColumnValue{emptyArray}), mustHash(t, arrayField, []spanner.GenericColumnValue{nullArray})) {
		t.Fatal("empty ARRAY and NULL ARRAY hashed equal")
	}
	if bytes.Equal(mustHash(t, arrayField, []spanner.GenericColumnValue{array12}), mustHash(t, arrayField, []spanner.GenericColumnValue{array21})) {
		t.Fatal("ARRAY order was ignored")
	}
	if bytes.Equal(mustHash(t, structField, []spanner.GenericColumnValue{structAB}), mustHash(t, structField, []spanner.GenericColumnValue{structBA})) {
		t.Fatal("STRUCT field values hashed equal")
	}

	empty := mustHash(t, intField)
	one := mustHash(t, intField, []spanner.GenericColumnValue{intOne})
	if bytes.Equal(empty, one) {
		t.Fatal("empty result hashed equal to one row")
	}
	if _, err := newResultFingerprinter(intField); err != nil {
		t.Fatal(err)
	}

	ordered := mustHash(t, intField, []spanner.GenericColumnValue{gcvctor.Int64Value(1)}, []spanner.GenericColumnValue{gcvctor.Int64Value(2)})
	reversed := mustHash(t, intField, []spanner.GenericColumnValue{gcvctor.Int64Value(2)}, []spanner.GenericColumnValue{gcvctor.Int64Value(1)})
	if bytes.Equal(ordered, reversed) {
		t.Fatal("row order was ignored")
	}

	row, err := spanner.NewRow([]string{"v"}, []any{int64(1)})
	if err != nil {
		t.Fatal(err)
	}
	fp, err := newResultFingerprinter(intField)
	if err != nil {
		t.Fatal(err)
	}
	if err := fp.ObserveRow(row); err != nil {
		t.Fatal(err)
	}
	fromRow, err := fp.FinishQuery()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fromRow, one) {
		t.Fatal("ObserveRow and ObserveGCVs hashes diverged")
	}
}

func TestSavepointFingerprintCountVectors(t *testing.T) {
	t.Parallel()
	hashCounts := func(t *testing.T, counts []int64) []byte {
		t.Helper()
		fp, err := newResultFingerprinter(nil)
		if err != nil {
			t.Fatal(err)
		}
		sum, err := fp.FinishBatch(counts)
		if err != nil {
			t.Fatal(err)
		}
		return sum
	}
	a := hashCounts(t, []int64{1, 2})
	b := hashCounts(t, []int64{3})
	c := hashCounts(t, []int64{2, 1})
	if bytes.Equal(a, b) || bytes.Equal(a, c) {
		t.Fatal("batch count vectors hashed equal")
	}

	dmlFields := []*sppb.StructType_Field{{Name: "id", Type: &sppb.Type{Code: sppb.TypeCode_INT64}}}
	fp1, err := newResultFingerprinter(dmlFields)
	if err != nil {
		t.Fatal(err)
	}
	if err := fp1.ObserveGCVs([]spanner.GenericColumnValue{gcvctor.Int64Value(7)}); err != nil {
		t.Fatal(err)
	}
	sum1, err := fp1.FinishDML(1)
	if err != nil {
		t.Fatal(err)
	}
	fp2, err := newResultFingerprinter(dmlFields)
	if err != nil {
		t.Fatal(err)
	}
	if err := fp2.ObserveGCVs([]spanner.GenericColumnValue{gcvctor.Int64Value(7)}); err != nil {
		t.Fatal(err)
	}
	sum2, err := fp2.FinishDML(2)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(sum1, sum2) {
		t.Fatal("DML affected-row counts hashed equal")
	}
}

func TestSavepointFingerprintTruncationIsNotSuccess(t *testing.T) {
	t.Parallel()
	fields := []*sppb.StructType_Field{{Name: "v", Type: &sppb.Type{Code: sppb.TypeCode_INT64}}}
	fp, err := newResultFingerprinter(fields)
	if err != nil {
		t.Fatal(err)
	}
	if err := fp.ObserveGCVs([]spanner.GenericColumnValue{gcvctor.Int64Value(1)}); err != nil {
		t.Fatal(err)
	}
	if _, err := fp.FinishQuery(); err != nil {
		t.Fatal(err)
	}
	if err := fp.ObserveGCVs([]spanner.GenericColumnValue{gcvctor.Int64Value(2)}); err == nil {
		t.Fatal("truncated extra row was accepted after finish")
	}
	if _, err := fp.FinishQuery(); err == nil {
		t.Fatal("second FinishQuery succeeded")
	}
}

func TestFreezeStatementParamsAndOptionsAreImmutable(t *testing.T) {
	t.Parallel()
	mode := sppb.ExecuteSqlRequest_PROFILE
	orig := spanner.GenericColumnValue{
		Type: &sppb.Type{
			Code: sppb.TypeCode_ARRAY,
			ArrayElementType: &sppb.Type{
				Code: sppb.TypeCode_STRUCT,
				StructType: &sppb.StructType{Fields: []*sppb.StructType_Field{
					{Name: "n", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
				}},
			},
		},
		Value: structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{
			structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{
				structpb.NewStringValue("keep"),
			}}),
		}}),
	}
	opts := spanner.QueryOptions{
		Mode:       &mode,
		RequestTag: "user-tag",
		Options: &sppb.ExecuteSqlRequest_QueryOptions{
			OptimizerVersion: "1",
		},
		LastStatement: true,
	}
	frozen, err := freezeStatement("SELECT @p", map[string]any{"p": orig}, opts)
	if err != nil {
		t.Fatal(err)
	}

	orig.Type.ArrayElementType.Code = sppb.TypeCode_JSON
	orig.Type.ArrayElementType.StructType.Fields[0].Name = "mutated"
	orig.Value.GetListValue().Values[0].GetListValue().Values[0].Kind.(*structpb.Value_StringValue).StringValue = "mutated"
	mode = sppb.ExecuteSqlRequest_NORMAL
	opts.RequestTag = "other"
	opts.Options.OptimizerVersion = "9"
	opts.LastStatement = false

	got := frozen.Params["p"]
	if got.Type.GetArrayElementType().GetCode() != sppb.TypeCode_STRUCT {
		t.Fatalf("frozen array element type = %v, want STRUCT", got.Type.GetArrayElementType().GetCode())
	}
	if got.Type.GetArrayElementType().GetStructType().GetFields()[0].GetName() != "n" {
		t.Fatalf("frozen struct field = %q", got.Type.GetArrayElementType().GetStructType().GetFields()[0].GetName())
	}
	if got.Value.GetListValue().GetValues()[0].GetListValue().GetValues()[0].GetStringValue() != "keep" {
		t.Fatalf("frozen nested value = %q, want keep", got.Value.GetListValue().GetValues()[0].GetListValue().GetValues()[0].GetStringValue())
	}
	if frozen.Opts.RequestTag != "user-tag" {
		t.Fatalf("frozen request tag = %q", frozen.Opts.RequestTag)
	}
	if frozen.Opts.Options.GetOptimizerVersion() != "1" {
		t.Fatalf("frozen optimizer = %q", frozen.Opts.Options.GetOptimizerVersion())
	}
	replay := frozen.Opts.toQueryOptions()
	if replay.LastStatement {
		t.Fatal("replay QueryOptions kept LastStatement=true")
	}
	if replay.RequestTag != "user-tag" {
		t.Fatalf("replay tag = %q", replay.RequestTag)
	}
	if *replay.Mode != sppb.ExecuteSqlRequest_PROFILE {
		t.Fatalf("replay mode = %v", *replay.Mode)
	}

	frozen.Opts.Options.OptimizerVersion = "changed"
	if opts.Options.GetOptimizerVersion() != "9" {
		t.Fatal("mutating frozen options leaked into later caller state")
	}
}

func TestFreezeMutationValuesAreImmutable(t *testing.T) {
	t.Parallel()
	src := spanner.GenericColumnValue{
		Type: &sppb.Type{
			Code:             sppb.TypeCode_ARRAY,
			ArrayElementType: &sppb.Type{Code: sppb.TypeCode_STRING},
		},
		Value: structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{
			structpb.NewStringValue("keep"),
		}}),
	}
	cols := []string{"id"}
	frozen := freezeMutationWrite("T", "INSERT", cols, []spanner.GenericColumnValue{src})
	src.Type.ArrayElementType.Code = sppb.TypeCode_BYTES
	src.Value.GetListValue().Values[0].Kind.(*structpb.Value_StringValue).StringValue = "mutated"
	cols[0] = "other"
	if frozen.Columns[0] != "id" {
		t.Fatal("frozen mutation columns aliased caller slice")
	}
	if frozen.Values[0].Type.GetArrayElementType().GetCode() != sppb.TypeCode_STRING {
		t.Fatal("frozen mutation type aliased caller Type graph")
	}
	if frozen.Values[0].Value.GetListValue().GetValues()[0].GetStringValue() != "keep" {
		t.Fatal("frozen mutation values aliased caller Value graph")
	}
	mut, err := frozen.Mutation()
	if err != nil {
		t.Fatal(err)
	}
	want := spanner.Insert("T", []string{"id"}, []any{cloneGenericColumnValue(spanner.GenericColumnValue{
		Type: &sppb.Type{
			Code:             sppb.TypeCode_ARRAY,
			ArrayElementType: &sppb.Type{Code: sppb.TypeCode_STRING},
		},
		Value: structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{
			structpb.NewStringValue("keep"),
		}}),
	})})
	if diff := cmp.Diff(want, mut, cmp.AllowUnexported(spanner.Mutation{}), protocmp.Transform()); diff != "" {
		t.Fatalf("reconstructed mutation mismatch (-want +got):\n%s", diff)
	}
}

func TestFreezeKeyRangeValuesAreImmutable(t *testing.T) {
	t.Parallel()
	start := spanner.GenericColumnValue{
		Type:  &sppb.Type{Code: sppb.TypeCode_INT64},
		Value: structpb.NewStringValue("1"),
	}
	end := spanner.GenericColumnValue{
		Type:  &sppb.Type{Code: sppb.TypeCode_INT64},
		Value: structpb.NewStringValue("10"),
	}
	kr := &frozenKeyRange{
		Start: cloneGCVRow([]spanner.GenericColumnValue{start}),
		End:   cloneGCVRow([]spanner.GenericColumnValue{end}),
		Kind:  spanner.ClosedOpen,
	}
	start.Type.Code = sppb.TypeCode_BYTES
	start.Value.Kind.(*structpb.Value_StringValue).StringValue = "mutated"
	end.Type.Code = sppb.TypeCode_STRING
	if kr.Start[0].Type.GetCode() != sppb.TypeCode_INT64 || kr.Start[0].Value.GetStringValue() != "1" {
		t.Fatal("frozen key-range start aliased caller Type/Value graphs")
	}
	if kr.End[0].Type.GetCode() != sppb.TypeCode_INT64 || kr.End[0].Value.GetStringValue() != "10" {
		t.Fatal("frozen key-range end aliased caller Type/Value graphs")
	}
	fm := frozenMutation{Table: "T", Op: "DELETE", KeyRange: kr}
	got, err := fm.Mutation()
	if err != nil {
		t.Fatal(err)
	}
	want := spanner.Delete("T", spanner.KeyRange{Start: spanner.Key{int64(1)}, End: spanner.Key{int64(10)}, Kind: spanner.ClosedOpen})
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(spanner.Mutation{}), protocmp.Transform()); diff != "" {
		t.Fatalf("key-range mutation mismatch (-want +got):\n%s", diff)
	}

	parsed, err := parseMutation("T", "DELETE", "KEY_RANGE(start_closed=>(1), end_open=>(10))")
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(parsed, []*spanner.Mutation{got}, cmp.AllowUnexported(spanner.Mutation{}), protocmp.Transform()); diff != "" {
		t.Fatalf("parseMutation/freeze KEY_RANGE mismatch (-want +got):\n%s", diff)
	}
}

func TestReplayStateByteAccountingAndTruncate(t *testing.T) {
	t.Parallel()
	rs := &replayState{}
	stmt, err := freezeStatement("SELECT 1", nil, spanner.QueryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	fp, err := newResultFingerprinter(nil)
	if err != nil {
		t.Fatal(err)
	}
	sum, err := fp.FinishQuery()
	if err != nil {
		t.Fatal(err)
	}
	entry := replayEntry{kind: replayKindSQL, stmt: stmt, fingerprint: sum}
	if err := rs.appendEntry(entry); err != nil {
		t.Fatal(err)
	}
	if got, counts := rs.entries[0].dmlObservation(); got != 0 || len(counts) != 0 {
		t.Fatalf("sql entry observation: affected=%d counts=%v", got, counts)
	}
	if rs.needsRecovery() {
		t.Fatal("new journal started in recovery-required")
	}
	if rs.retainedBytes <= 0 {
		t.Fatal("append did not account payload bytes")
	}
	if err := rs.addSavepoint("before"); err != nil {
		t.Fatal(err)
	}
	after, err := freezeStatement("INSERT INTO t (id) VALUES (2)", nil, spanner.QueryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := rs.appendEntry(replayEntry{kind: replayKindSQL, stmt: after, fingerprint: slices.Clone(sum)}); err != nil {
		t.Fatal(err)
	}
	kept := rs.retainedBytes
	rs.truncateAfter(1)
	if len(rs.entries) != 1 || len(rs.savepoints) != 1 {
		t.Fatalf("truncateAfter: entries=%d savepoints=%d", len(rs.entries), len(rs.savepoints))
	}
	if rs.retainedBytes >= kept {
		t.Fatal("truncateAfter did not release later payload")
	}

	param := spanner.GenericColumnValue{Type: &sppb.Type{Code: sppb.TypeCode_STRING}, Value: structpb.NewStringValue("secret")}
	parameterized, err := freezeStatement("SELECT @p", map[string]any{"p": param}, spanner.QueryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := rs.appendEntry(replayEntry{kind: replayKindSQL, stmt: parameterized, fingerprint: slices.Clone(sum)}); err != nil {
		t.Fatal(err)
	}
	if err := rs.addSavepoint("discard-me"); err != nil {
		t.Fatal(err)
	}
	entryBacking := rs.entries
	markerBacking := rs.savepoints
	rs.truncateAfter(0)
	if rs.retainedBytes != 0 {
		t.Fatalf("truncate to zero retainedBytes=%d", rs.retainedBytes)
	}
	for _, e := range entryBacking[:cap(entryBacking)] {
		if e.stmt.SQL != "" || e.stmt.Params != nil || e.fingerprint != nil {
			t.Fatal("truncated journal entry still reachable in backing array")
		}
	}
	for _, sp := range markerBacking[:cap(markerBacking)] {
		if sp.name != "" {
			t.Fatal("truncated savepoint name still reachable in backing array")
		}
	}

	full := &replayState{retainedBytes: savepointJournalLimit}
	if err := full.appendEntry(entry); !errors.Is(err, errSavepointJournalFull) {
		t.Fatalf("full journal: %v", err)
	}
	if len(full.entries) != 0 {
		t.Fatal("exhausted reserve invalidated existing markers/entries")
	}
	if err := full.addSavepoint("x"); !errors.Is(err, errSavepointJournalFull) {
		t.Fatalf("full marker: %v", err)
	}
}

func TestFreezeStatementRejectsNonGenericParams(t *testing.T) {
	t.Parallel()
	_, err := freezeStatement("SELECT @p", map[string]any{"p": int64(1)}, spanner.QueryOptions{})
	if err == nil {
		t.Fatal("expected error for non-GenericColumnValue param")
	}
}
