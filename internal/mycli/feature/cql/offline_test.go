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
	"testing"

	"github.com/gocql/gocql"
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
