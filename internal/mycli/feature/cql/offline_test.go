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

package cql

import (
	"encoding/binary"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/gocql/gocql"
	spancql "github.com/googleapis/go-spanner-cassandra/cassandra/gocql"

	"github.com/apstndb/spanner-mycli/internal/mycli"
)

func TestFormatCassandraTypeName(t *testing.T) {
	t.Parallel()

	text := gocql.NewNativeType(4, gocql.TypeText, "")
	intType := gocql.NewNativeType(4, gocql.TypeInt, "")

	tests := []struct {
		name string
		in   gocql.TypeInfo
		want string
	}{
		{name: "native text", in: text, want: "text"},
		{
			name: "list",
			in: gocql.CollectionType{
				NativeType: gocql.NewNativeType(4, gocql.TypeList, ""),
				Elem:       text,
			},
			want: "list<text>",
		},
		{
			name: "set",
			in: gocql.CollectionType{
				NativeType: gocql.NewNativeType(4, gocql.TypeSet, ""),
				Elem:       intType,
			},
			want: "set<int>",
		},
		{
			name: "map",
			in: gocql.CollectionType{
				NativeType: gocql.NewNativeType(4, gocql.TypeMap, ""),
				Key:        text,
				Elem:       intType,
			},
			want: "map<text, int>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := formatCassandraTypeName(tt.in); got != tt.want {
				t.Errorf("formatCassandraTypeName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCQLSessionHolderCloseNil(t *testing.T) {
	t.Parallel()

	h := &cqlSessionHolder{}
	if err := h.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestCQLOpenClusterRecoversNewClusterPanic(t *testing.T) {
	t.Parallel()

	var closes int
	h := &cqlSessionHolder{
		newCluster: func(*spancql.Options) *gocql.ClusterConfig {
			panic("adapter client construction failed")
		},
		closeCluster: func(*gocql.ClusterConfig) { closes++ },
	}

	_, err := h.get(&mycli.Session{})
	if err == nil {
		t.Fatal("get() error = nil, want recovered construction error")
	}
	if !strings.Contains(err.Error(), "adapter client construction failed") {
		t.Fatalf("get() error = %v, want it to mention the panic value", err)
	}
	if closes != 0 {
		t.Fatalf("closeCluster called %d times, want 0 for a panic before the cluster is returned", closes)
	}
	if err := h.Close(); err != nil {
		t.Fatalf("Close() after failed construction: %v", err)
	}
	if closes != 0 {
		t.Fatalf("Close() after failed construction closed %d clusters, want 0", closes)
	}
}

func TestCQLOpenClusterClosesOnCreateSessionFailure(t *testing.T) {
	t.Parallel()

	opened := &gocql.ClusterConfig{}
	createErr := errors.New("create session failed")
	var closed []*gocql.ClusterConfig
	h := &cqlSessionHolder{
		newCluster: func(*spancql.Options) *gocql.ClusterConfig { return opened },
		createSession: func(*gocql.ClusterConfig) (*gocql.Session, error) {
			return nil, createErr
		},
		closeCluster: func(c *gocql.ClusterConfig) { closed = append(closed, c) },
	}

	_, err := h.get(&mycli.Session{})
	if !errors.Is(err, createErr) {
		t.Fatalf("get() error = %v, want %v", err, createErr)
	}
	if len(closed) != 1 || closed[0] != opened {
		t.Fatalf("partial construction closed %d clusters, want the opened cluster once", len(closed))
	}
	if err := h.Close(); err != nil {
		t.Fatalf("Close() after partial construction: %v", err)
	}
	if len(closed) != 1 {
		t.Fatalf("Close() after partial construction closed %d times, want exactly once", len(closed))
	}
}

func TestCQLOpenClusterClosesOnCreateSessionPanic(t *testing.T) {
	t.Parallel()

	opened := &gocql.ClusterConfig{}
	var closes int
	h := &cqlSessionHolder{
		newCluster: func(*spancql.Options) *gocql.ClusterConfig { return opened },
		createSession: func(*gocql.ClusterConfig) (*gocql.Session, error) {
			panic("create session panic")
		},
		closeCluster: func(*gocql.ClusterConfig) { closes++ },
	}

	_, err := h.get(&mycli.Session{})
	if err == nil || !strings.Contains(err.Error(), "create session panic") {
		t.Fatalf("get() error = %v, want recovered CreateSession panic", err)
	}
	if closes != 1 {
		t.Fatalf("closeCluster called %d times, want 1 after CreateSession panic", closes)
	}
}

func TestCQLSessionHolderCloseClosesAdapterOnce(t *testing.T) {
	t.Parallel()

	opened := &gocql.ClusterConfig{}
	session := new(gocql.Session)
	var closes int
	h := &cqlSessionHolder{
		newCluster:    func(*spancql.Options) *gocql.ClusterConfig { return opened },
		createSession: func(*gocql.ClusterConfig) (*gocql.Session, error) { return session, nil },
		closeCluster:  func(c *gocql.ClusterConfig) { closes++ },
	}

	got, err := h.get(&mycli.Session{})
	if err != nil {
		t.Fatalf("get() error = %v", err)
	}
	if got != session {
		t.Fatal("get() did not return the created session")
	}
	again, err := h.get(&mycli.Session{})
	if err != nil {
		t.Fatalf("second get() error = %v", err)
	}
	if again != session {
		t.Fatal("second get() rebuilt the session")
	}
	if closes != 0 {
		t.Fatalf("closeCluster called %d times before Close, want 0", closes)
	}
	if err := h.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if closes != 1 {
		t.Fatalf("Close() closed %d clusters, want 1", closes)
	}
	if err := h.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if closes != 1 {
		t.Fatalf("second Close() closed %d clusters, want still 1", closes)
	}
}

// TestCQLScanPreservesNull exercises the scan destinations and row formatter
// used by CQLStatement.Execute. It decodes with the pinned gocql Unmarshal
// path (the same call Iter.Scan makes) and does not start a Cassandra adapter.
func TestCQLScanPreservesNull(t *testing.T) {
	t.Parallel()

	const proto byte = 4
	text := gocql.NewNativeType(proto, gocql.TypeText, "")
	intType := gocql.NewNativeType(proto, gocql.TypeInt, "")
	boolType := gocql.NewNativeType(proto, gocql.TypeBoolean, "")
	blob := gocql.NewNativeType(proto, gocql.TypeBlob, "")
	big := gocql.NewNativeType(proto, gocql.TypeBigInt, "")
	ascii := gocql.NewNativeType(proto, gocql.TypeAscii, "")

	intZero := make([]byte, 4)
	intOne := make([]byte, 4)
	binary.BigEndian.PutUint32(intOne, 1)
	bigZero := make([]byte, 8)
	bigOne := make([]byte, 8)
	binary.BigEndian.PutUint64(bigOne, 1)

	tests := []struct {
		name     string
		cols     []gocql.TypeInfo
		rows     [][][]byte
		want     [][]string
		wantNull [][]bool
	}{
		{
			name: "empty result",
			cols: []gocql.TypeInfo{text, intType, boolType, blob},
		},
		{
			name: "null zero and nonzero rows",
			cols: []gocql.TypeInfo{text, intType, boolType, blob},
			rows: [][][]byte{
				{nil, nil, nil, nil},
				{[]byte{}, intZero, []byte{0}, []byte{}},
				{[]byte("hello"), intOne, []byte{1}, []byte{1, 2}},
				{nil, intZero, nil, []byte{1, 2}},
			},
			want: [][]string{
				{"NULL", "NULL", "NULL", "NULL"},
				{"", "0", "false", "[]"},
				{"hello", "1", "true", "[1 2]"},
				{"NULL", "0", "NULL", "[1 2]"},
			},
			wantNull: [][]bool{
				{true, true, true, true},
				{false, false, false, false},
				{false, false, false, false},
				{true, false, true, false},
			},
		},
		{
			name: "bigint and ascii",
			cols: []gocql.TypeInfo{big, ascii},
			rows: [][][]byte{
				{nil, nil},
				{bigZero, []byte{}},
				{bigOne, []byte("a")},
			},
			want: [][]string{
				{"NULL", "NULL"},
				{"0", ""},
				{"1", "a"},
			},
			wantNull: [][]bool{
				{true, true},
				{false, false},
				{false, false},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got [][]string
			var gotNull [][]bool
			for _, data := range tt.rows {
				row, nulls := displayScannedCQLRow(t, tt.cols, data)
				got = append(got, row)
				gotNull = append(gotNull, nulls)
			}
			if len(tt.want) == 0 {
				if len(got) != 0 {
					t.Fatalf("empty result produced %#v", got)
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d rows, want %d", len(got), len(tt.want))
			}
			for i := range tt.want {
				if !slices.Equal(got[i], tt.want[i]) {
					t.Errorf("row %d = %#v, want %#v", i, got[i], tt.want[i])
				}
				if !slices.Equal(gotNull[i], tt.wantNull[i]) {
					t.Errorf("row %d null pointers = %v, want %v", i, gotNull[i], tt.wantNull[i])
				}
			}
		})
	}
}

// displayScannedCQLRow builds destinations the way Iter.RowData does, wraps
// them with the Execute helper, and unmarshals each cell.
func displayScannedCQLRow(t *testing.T, cols []gocql.TypeInfo, data [][]byte) ([]string, []bool) {
	t.Helper()
	if len(cols) != len(data) {
		t.Fatalf("column count %d != encoded value count %d", len(cols), len(data))
	}

	values := make([]any, len(cols))
	for i, info := range cols {
		v, err := info.NewWithError()
		if err != nil {
			t.Fatalf("NewWithError(%v) error = %v", info, err)
		}
		values[i] = v
	}
	dests := nullPreservingScanDests(values)
	for i := range cols {
		if err := gocql.Unmarshal(cols[i], data[i], dests[i]); err != nil {
			t.Fatalf("Unmarshal(%v) error = %v", cols[i], err)
		}
	}

	nulls := make([]bool, len(dests))
	for i, dest := range dests {
		nulls[i] = scannedCQLPointerIsNil(dest)
	}
	return formatCQLScannedRow(dests), nulls
}

func TestCQLDecimalAndVarintFormatting(t *testing.T) {
	t.Parallel()

	const proto byte = 4
	decimal := gocql.NewNativeType(proto, gocql.TypeDecimal, "")
	varint := gocql.NewNativeType(proto, gocql.TypeVarint, "")

	tests := []struct {
		name     string
		col      gocql.TypeInfo
		data     []byte
		want     string
		wantNull bool
	}{
		{name: "decimal null", col: decimal, want: "NULL", wantNull: true},
		{name: "decimal zero", col: decimal, data: []byte{0, 0, 0, 0, 0}, want: "0"},
		{name: "decimal negative", col: decimal, data: []byte{0, 0, 0, 2, 0x85}, want: "-1.23"},
		{name: "decimal nonzero", col: decimal, data: []byte{0, 0, 0, 2, 123}, want: "1.23"},
		{name: "varint null", col: varint, want: "NULL", wantNull: true},
		{name: "varint zero", col: varint, data: []byte{0}, want: "0"},
		{name: "varint negative", col: varint, data: []byte{0xd6}, want: "-42"},
		{name: "varint nonzero", col: varint, data: []byte{0x2a}, want: "42"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, nulls := displayScannedCQLRow(t, []gocql.TypeInfo{tt.col}, [][]byte{tt.data})
			if len(got) != 1 || got[0] != tt.want {
				t.Errorf("display = %#v, want %q", got, tt.want)
			}
			if len(nulls) != 1 || nulls[0] != tt.wantNull {
				t.Errorf("null pointer = %v, want %v", nulls, tt.wantNull)
			}
		})
	}
}

// scannedCQLPointerIsNil reports whether decoding stored NULL as a nil
// pointer. A nil slice or other zero value is not NULL.
func scannedCQLPointerIsNil(value any) bool {
	v := reflect.ValueOf(value)
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return true
		}
		v = v.Elem()
	}
	return false
}
