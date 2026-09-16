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
	"errors"
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
