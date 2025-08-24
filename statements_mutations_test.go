package main

import (
	"testing"
	"time"

	"github.com/apstndb/spantype/typector"

	"cloud.google.com/go/civil"
	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestParseMutation(t *testing.T) {
	t.Parallel()
	int64GCV := spanner.GenericColumnValue{
		Type:  typector.CodeToSimpleType(sppb.TypeCode_INT64),
		Value: structpb.NewStringValue("1"),
	}

	commitTimestampGCV := spanner.GenericColumnValue{
		Type:  typector.CodeToSimpleType(sppb.TypeCode_TIMESTAMP),
		Value: structpb.NewStringValue("spanner.commit_timestamp()"),
	}

	tests := []struct {
		desc  string
		op    string
		table string
		input string
		want  []*spanner.Mutation
	}{
		{
			"DELETE single", "DELETE", "t", `("foo", 1, 1.1, TRUE, b"foo", TIMESTAMP "2000-01-01T00:00:00Z", DATE "2000-01-01")`,
			sliceOf(spanner.Delete("t", spanner.Key{"foo", int64(1), 1.1, true, []byte("foo"), time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), civil.Date{Year: 2000, Month: time.January, Day: 1}})),
		},
		{
			"DELETE multi", "DELETE", "t", "[(1), (2), (3)]",
			sliceOf(spanner.Delete("t", spanner.KeySets(spanner.Key{int64(1)}, spanner.Key{int64(2)}, spanner.Key{int64(3)}))),
		},
		{
			"DELETE ALL", "DELETE", "t", "ALL",
			sliceOf(spanner.Delete("t", spanner.AllKeys())),
		},
		{
			"DELETE ClosedClosed", "DELETE", "t", "KEY_RANGE(start_closed=>(1), end_closed=>(10))",
			sliceOf(spanner.Delete("t", spanner.KeyRange{Start: spanner.Key{int64(1)}, End: spanner.Key{int64(10)}, Kind: spanner.ClosedClosed})),
		},
		{
			"DELETE ClosedOpen", "DELETE", "t", "KEY_RANGE(start_closed=>(1), end_open=>(10))",
			sliceOf(spanner.Delete("t", spanner.KeyRange{Start: spanner.Key{int64(1)}, End: spanner.Key{int64(10)}, Kind: spanner.ClosedOpen})),
		},
		{
			"DELETE OpenClosed", "DELETE", "t", "KEY_RANGE(start_open=>(1), end_closed=>(10))",
			sliceOf(spanner.Delete("t", spanner.KeyRange{Start: spanner.Key{int64(1)}, End: spanner.Key{int64(10)}, Kind: spanner.OpenClosed})),
		},
		{
			"DELETE OpenOpen", "DELETE", "t", "KEY_RANGE(start_open=>(1), end_open=>(10))",
			sliceOf(spanner.Delete("t", spanner.KeyRange{Start: spanner.Key{int64(1)}, End: spanner.Key{int64(10)}, Kind: spanner.OpenOpen})),
		},
		{
			"INSERT typeless struct", "INSERT", "t", "STRUCT(1 AS n)",
			sliceOf(spanner.Insert("t", []string{"n"}, []any{int64GCV})),
		},
		{
			"INSERT typeless struct with PENDING_COMMIT_TIMESTAMP()", "INSERT", "t", "STRUCT(PENDING_COMMIT_TIMESTAMP() AS CreatedAt)",
			sliceOf(spanner.Insert("t", []string{"CreatedAt"}, []any{commitTimestampGCV})),
		},
		{
			"UPDATE typed struct", "UPDATE", "t", "STRUCT<n INT64>(1)",
			sliceOf(spanner.Update("t", []string{"n"}, []any{int64GCV})),
		},
		{
			"INSERT_OR_UPDATE typed struct", "INSERT_OR_UPDATE", "t", "STRUCT<n INT64>(1)",
			sliceOf(spanner.InsertOrUpdate("t", []string{"n"}, []any{int64GCV})),
		},
		{
			"REPLACE typed struct", "REPLACE", "t", "STRUCT<n INT64>(1)",
			sliceOf(spanner.Replace("t", []string{"n"}, []any{int64GCV})),
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got, err := parseMutation(tt.table, tt.op, tt.input)
			if err != nil {
				t.Errorf("should suceed, but fail, err: %v", err)
			}
			if diff := cmp.Diff(tt.want, got, cmp.AllowUnexported(spanner.Mutation{}), protocmp.Transform()); diff != "" {
				t.Errorf("parseMutation() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
