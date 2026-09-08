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
	"fmt"
	"math/big"
	"testing"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestMutationTypedDeleteKeys(t *testing.T) {
	t.Parallel()
	u := uuid.MustParse("01234567-89ab-cdef-0123-456789abcdef")
	const uuidExpr = "CAST('01234567-89ab-cdef-0123-456789abcdef' AS UUID)"
	n := *big.NewRat(1500000001, 1000000000)
	const numericExpr = "NUMERIC '1.500000001'"
	for _, tc := range []struct {
		name, body string
		keys       spanner.KeySet
	}{
		{"uuid", uuidExpr, spanner.Key{u}},
		{"numeric", numericExpr, spanner.Key{n}},
		{"uuid set", "[" + uuidExpr + ", " + uuidExpr + "]", spanner.KeySets(spanner.Key{u}, spanner.Key{u})},
		{"numeric set", "[" + numericExpr + ", NUMERIC '-2.5']", spanner.KeySets(spanner.Key{n}, spanner.Key{*big.NewRat(-5, 2)})},
		{"composite", "(7, " + numericExpr + ", " + uuidExpr + ")", spanner.Key{int64(7), n, u}},
		{"composite range", "KEY_RANGE(start_closed=>(7, " + numericExpr + ", " + uuidExpr + "), end_open=>(7, NUMERIC '2.5', " + uuidExpr + "))", spanner.KeyRange{Start: spanner.Key{int64(7), n, u}, End: spanner.Key{int64(7), *big.NewRat(5, 2), u}, Kind: spanner.ClosedOpen}},
		{"range", "KEY_RANGE(start_closed=>" + numericExpr + ", end_open=>NUMERIC '2.5')", spanner.KeyRange{Start: spanner.Key{n}, End: spanner.Key{*big.NewRat(5, 2)}, Kind: spanner.ClosedOpen}},
		{"uuid range", "KEY_RANGE(start_open=>" + uuidExpr + ", end_closed=>" + uuidExpr + ")", spanner.KeyRange{Start: spanner.Key{u}, End: spanner.Key{u}, Kind: spanner.OpenClosed}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stmt, err := BuildStatement("MUTATE TypedKeys DELETE " + tc.body)
			if err != nil {
				t.Fatal(err)
			}
			m, ok := stmt.(*MutateStatement)
			if !ok {
				t.Fatalf("statement type %T", stmt)
			}
			got, err := parseMutation(m.Table, m.Operation, m.Body)
			if err != nil {
				t.Fatal(err)
			}
			want := []*spanner.Mutation{spanner.Delete("TypedKeys", tc.keys)}
			if diff := cmp.Diff(want, got, cmp.AllowUnexported(spanner.Mutation{}), protocmp.Transform(), cmp.Comparer(func(a, b big.Rat) bool { return a.Cmp(&b) == 0 })); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestMutationTypedKeyDecodeErrorsAndNull(t *testing.T) {
	t.Parallel()
	for _, code := range []sppb.TypeCode{sppb.TypeCode_NUMERIC, sppb.TypeCode_UUID} {
		t.Run(code.String(), func(t *testing.T) {
			gcv := spanner.GenericColumnValue{Type: &sppb.Type{Code: code}, Value: structpb.NewStringValue("not-a-valid-key")}
			if _, err := gcvToKeyable(gcv); err == nil {
				t.Fatal("invalid typed key accepted")
			}
			gcv.Value = structpb.NewNullValue()
			got, err := gcvToKeyable(gcv)
			if err != nil || got != nil {
				t.Fatalf("NULL key = %v, %v", got, err)
			}
		})
	}
}

func TestMutationTypedDeleteKeysIntegration(t *testing.T) {
	skipIfShortIntegration(t)
	_, session := initializeWithRandomDB(t, nil, nil)
	exec := func(t *testing.T, sql string) {
		t.Helper()
		stmt, err := BuildStatement(sql)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := stmt.Execute(t.Context(), session); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	for _, typ := range []string{"NUMERIC", "UUID"} {
		t.Run(typ, func(t *testing.T) {
			table := "TypedKey" + typ
			exec(t, "CREATE TABLE "+table+" (K "+typ+" NOT NULL, Id INT64) PRIMARY KEY(K)")
			key := func(i int64) any {
				if typ == "NUMERIC" {
					return *big.NewRat(i*1000000001, 1000000000)
				}
				return uuid.MustParse(fmt.Sprintf("00000000-0000-0000-0000-%012d", i))
			}
			literal := func(i int64) string {
				if typ == "NUMERIC" {
					return fmt.Sprintf("NUMERIC '%d.%09d'", i, i)
				}
				return fmt.Sprintf("CAST('00000000-0000-0000-0000-%012d' AS UUID)", i)
			}
			for _, tc := range []struct {
				name, body string
				want       []int64
			}{
				{"scalar", literal(2), []int64{1, 3, 4}},
				{"set", "[" + literal(2) + ", " + literal(3) + "]", []int64{1, 4}},
				{"closed open", "KEY_RANGE(start_closed=>" + literal(2) + ", end_open=>" + literal(3) + ")", []int64{1, 3, 4}},
				{"open closed", "KEY_RANGE(start_open=>" + literal(2) + ", end_closed=>" + literal(3) + ")", []int64{1, 2, 4}},
				{"closed closed", "KEY_RANGE(start_closed=>" + literal(2) + ", end_closed=>" + literal(3) + ")", []int64{1, 4}},
				{"open open", "KEY_RANGE(start_open=>" + literal(1) + ", end_open=>" + literal(4) + ")", []int64{1, 4}},
			} {
				t.Run(tc.name, func(t *testing.T) {
					var seed []*spanner.Mutation
					for i := int64(1); i <= 4; i++ {
						seed = append(seed, spanner.InsertOrUpdate(table, []string{"K", "Id"}, []any{key(i), i}))
					}
					if _, err := session.client.Apply(t.Context(), seed); err != nil {
						t.Fatal(err)
					}
					exec(t, "MUTATE "+table+" DELETE "+tc.body)
					iter := session.client.Single().Query(t.Context(), spanner.Statement{SQL: "SELECT Id FROM " + table + " ORDER BY Id"})
					defer iter.Stop()
					var got []int64
					if err := iter.Do(func(row *spanner.Row) error {
						var id int64
						if err := row.Column(0, &id); err != nil {
							return err
						}
						got = append(got, id)
						return nil
					}); err != nil {
						t.Fatal(err)
					}
					if diff := cmp.Diff(tc.want, got); diff != "" {
						t.Fatalf("remaining rows (-want +got): %s", diff)
					}
				})
			}
		})
	}
}

func TestMutationTypedCompositeDeleteTransaction(t *testing.T) {
	skipIfShortIntegration(t)
	_, session := initializeWithRandomDB(t, nil, nil)
	exec := func(sql string) {
		t.Helper()
		stmt, err := BuildStatement(sql)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := stmt.Execute(t.Context(), session); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	exec("CREATE TABLE CompositeKeys (Tenant INT64 NOT NULL, N NUMERIC NOT NULL, U UUID NOT NULL, Id INT64) PRIMARY KEY(Tenant, N, U)")
	u := uuid.MustParse("01234567-89ab-cdef-0123-456789abcdef")
	u2 := uuid.MustParse("01234567-89ab-cdef-0123-456789abcdf0")
	var seed []*spanner.Mutation
	for _, row := range []struct {
		tenant, n, id int64
		u             uuid.UUID
	}{{7, 1, 1, u}, {7, 1, 2, u2}, {8, 1, 3, u}, {7, 2, 4, u}} {
		seed = append(seed, spanner.Insert("CompositeKeys", []string{"Tenant", "N", "U", "Id"}, []any{row.tenant, *big.NewRat(row.n, 1), row.u, row.id}))
	}
	if _, err := session.client.Apply(t.Context(), seed); err != nil {
		t.Fatal(err)
	}
	assertRows := func(want []int64) {
		t.Helper()
		iter := session.client.Single().Query(t.Context(), spanner.Statement{SQL: "SELECT Id FROM CompositeKeys ORDER BY Id"})
		defer iter.Stop()
		var got []int64
		if err := iter.Do(func(row *spanner.Row) error {
			var id int64
			if err := row.Column(0, &id); err != nil {
				return err
			}
			got = append(got, id)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("committed rows (-want +got): %s", diff)
		}
	}
	exec("BEGIN RW")
	// The emulator only supports delete ranges differing in the final key part.
	// The unit matrix separately checks a range differing in the NUMERIC part.
	exec("MUTATE CompositeKeys DELETE KEY_RANGE(start_closed=>(7, NUMERIC '1', CAST('01234567-89ab-cdef-0123-456789abcdef' AS UUID)), end_open=>(7, NUMERIC '1', CAST('01234567-89ab-cdef-0123-456789abcdf0' AS UUID)))")
	assertRows([]int64{1, 2, 3, 4}) // Buffered writes are not externally committed yet.
	exec("COMMIT")
	assertRows([]int64{2, 3, 4}) // Preserve the open endpoint and both prefix components.
}
