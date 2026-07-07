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

// Package cql contributes the CQL (Cassandra interface) statement family to
// spanner-mycli through the Feature registration seam (issue #778). It is
// imported only by the full binary (via internal/mycli/feature/all); the core
// internal/mycli package does not import it, so github.com/gocql/gocql and
// github.com/googleapis/go-spanner-cassandra stay off the core import path.
//
// The family is EARLY EXPERIMENTAL: it executes CQL through the Cloud Spanner
// Cassandra adapter (go-spanner-cassandra), which spins up an in-process proxy
// bound to the session's Spanner database.
package cql

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sync"
	"time"

	"github.com/gocql/gocql"
	spancql "github.com/googleapis/go-spanner-cassandra/cassandra/gocql"
	"github.com/samber/lo"
	"go.uber.org/zap/zapcore"

	"github.com/apstndb/spanner-mycli/internal/mycli"
)

// sessionStateKey namespaces the lazy CQL (Cassandra adapter) session in the
// per-Session feature store (mycli.FeatureState).
const sessionStateKey = "cql.session"

// Feature returns the CQL Feature value. The family contributes a single
// statement and no variables or flags. Each call constructs fresh def
// instances, so no state is shared between Feature values (#778 §4.4
// session-isolation commitment); the statement itself carries only its own CQL
// text, so there is no package-level mutable state.
func Feature() mycli.Feature {
	return mycli.Feature{
		Name: "CQL",
		StatementDefs: []*mycli.StatementDef{
			{
				Descriptions: []mycli.StatementDescription{
					{
						Usage:  `Execute CQL`,
						Syntax: `CQL ...`,
						Note:   "EARLY EXPERIMENTAL",
					},
				},
				Pattern: regexp.MustCompile(`(?is)^CQL\s+(?P<cql>.+)$`),
				HandleGroups: func(groups map[string]string) (mycli.Statement, error) {
					return newCQLStatement(groups["cql"]), nil
				},
			},
		},
	}
}

// CQLStatement executes a single CQL statement through the Cloud Spanner
// Cassandra adapter. It embeds mycli.MutationClassifier (Classify wired in the
// constructor to the fail-closed first-keyword classifier over its own CQL text)
// so the READONLY guard can reject mutating CQL statements.
//
// It is deliberately NOT mycli.MarksDetachedCompatible: the Cassandra adapter
// binds to the session's Spanner database, so CQL cannot run in detached
// (admin-only, no database) session mode.
type CQLStatement struct {
	mycli.MutationClassifier
	CQL string
}

// Compile-time assertion that the marker embed promotes the (unexported) core
// marker method across the package boundary (#778 §4.3). A typo that dropped the
// embed would silently bypass the READONLY guard.
var _ mycli.ConditionallyMutatingStatement = (*CQLStatement)(nil)

// newCQLStatement builds a CQLStatement and wires the mutation classifier over
// its own CQL text.
func newCQLStatement(cql string) *CQLStatement {
	s := &CQLStatement{CQL: cql}
	s.Classify = func() bool { return cqlStatementMutates(s.CQL) }
	return s
}

// cqlSessionHolder is the per-Session lazy CQL (Cassandra adapter) session. It
// is stored in the Session feature store (mycli.FeatureState) and closed at
// Session.Close via the io.Closer contract.
//
// Access is serialized by mu so the check-then-act on the cached session is
// atomic (.gemini/styleguide.md §Concurrency). Unlike #781's BigQuery client
// cache the session is built once and never rebuilt: it binds to the Session's
// DatabasePath, which cannot change on a live Session (USE creates a new
// Session), so there is no rebuild-on-reconfigure rule (#778 §4.6).
type cqlSessionHolder struct {
	mu      sync.Mutex
	session *gocql.Session
}

// Close closes the cached gocql session, if any. The cluster is not closed
// separately (matching the pre-extraction in-core lifecycle, which closed only
// cqlSession). Satisfies io.Closer so the feature store closes it at the end of
// Session.Close.
func (h *cqlSessionHolder) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.session == nil {
		return nil
	}
	h.session.Close()
	h.session = nil
	return nil
}

// get returns the CQL session, building the Cassandra adapter cluster and gocql
// session lazily on first use. Deferring the build until the first CQL statement
// keeps the Cassandra adapter (and its in-process proxy) out of ordinary
// Spanner/emulator session creation. The build parameters mirror the
// pre-extraction in-core implementation exactly.
func (h *cqlSessionHolder) get(session *mycli.Session) (*gocql.Session, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.session != nil {
		return h.session, nil
	}

	cluster := spancql.NewCluster(&spancql.Options{
		LogLevel:    zapcore.WarnLevel.String(),
		DatabaseUri: session.DatabasePath(),
	})
	if cluster == nil {
		return nil, fmt.Errorf("failed to create cluster")
	}

	// You can still configure your cluster as usual after connecting to your
	// spanner database
	cluster.Timeout = 5 * time.Second

	s, err := cluster.CreateSession()
	if err != nil {
		return nil, err
	}
	h.session = s
	return s, nil
}

// Execute runs the CQL statement and returns the result rows as display
// strings.
func (cs *CQLStatement) Execute(ctx context.Context, session *mycli.Session) (result *mycli.Result, err error) {
	holder, err := mycli.FeatureState(ctx, session, sessionStateKey,
		func(context.Context, *mycli.Session) (*cqlSessionHolder, error) {
			return &cqlSessionHolder{}, nil
		})
	if err != nil {
		return nil, err
	}

	s, err := holder.get(session)
	if err != nil {
		return nil, err
	}

	q := s.Query(cs.CQL)
	it := q.WithContext(ctx).Iter()
	defer func() {
		closeErr := it.Close()
		if closeErr == nil {
			return
		}
		if err == nil {
			err = closeErr
			return
		}
		if errors.Is(err, closeErr) {
			return
		}
		err = errors.Join(err, closeErr)
	}()

	var headers []string
	for _, col := range it.Columns() {
		headers = append(headers, col.Name+"\n"+formatCassandraTypeName(col.TypeInfo))
	}

	var rows []mycli.Row
	for {
		rd, err := it.RowData()
		if err != nil {
			return nil, err
		}

		if !it.Scan(rd.Values...) {
			break
		}

		var rowStrs []string
		for _, value := range rd.Values {
			rowStrs = append(rowStrs, fmt.Sprint(reflect.Indirect(reflect.ValueOf(value)).Interface()))
		}
		rows = append(rows, mycli.NewRow(rowStrs...))
	}

	result = &mycli.Result{TableHeader: mycli.NewTableHeader(headers...), Rows: rows, AffectedRows: len(rows)}
	return result, nil
}

// formatCassandraTypeName renders a Cassandra column type for the result
// header, expanding collection types (list/set/map) with their element (and
// key) types.
func formatCassandraTypeName(typeInfo gocql.TypeInfo) string {
	if ct, ok := typeInfo.(gocql.CollectionType); ok {
		return fmt.Sprintf("%v<%v%v>",
			ct.Type(),
			lo.Ternary(ct.Key != nil, fmt.Sprint(ct.Key)+", ", ""),
			ct.Elem)
	} else {
		return fmt.Sprint(typeInfo)
	}
}
