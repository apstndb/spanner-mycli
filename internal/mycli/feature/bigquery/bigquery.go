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

// Package bigquery contributes the BIGQUERY statement family to spanner-mycli
// through the Feature registration seam (issue #778). It is imported only by the
// full binary (via internal/mycli/feature/all); the core internal/mycli package
// does not import it, so cloud.google.com/go/bigquery stays off the core import
// path. The cloud dependency is aliased as bq throughout this package.
//
// Tracked debt: BigQuery values are exported as display strings, not typed
// values (see formatBigQueryValue). A BIGQUERY query result therefore cannot be
// consumed downstream with column types; this limitation is inherited from the
// original in-core implementation and tracked as the feature's debt in #778 §1.
package bigquery

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"sync"

	bq "cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"

	"github.com/apstndb/spanner-mycli/internal/mycli"
)

// clientStateKey namespaces the lazy BigQuery client in the per-Session feature
// store (mycli.FeatureState).
const clientStateKey = "bigquery.client"

// config holds the BIGQUERY feature's live variable state. Exactly one instance
// is allocated per Feature() call and captured by that Feature's Variable
// handlers, def handler closure, and constructed statements, so there is no
// package-level mutable state (session-isolation commitment, #778 §4.4).
type config struct {
	Project        string // CLI_BIGQUERY_PROJECT (defaults to CLI_PROJECT when empty)
	Location       string // CLI_BIGQUERY_LOCATION
	MaxBytesBilled *int64 // CLI_BIGQUERY_MAX_BYTES_BILLED
}

// Feature returns the BIGQUERY Feature value. Each call allocates a FRESH config
// and wires fresh Variable/def/statement instances around it, so no state is
// shared between Feature values (#778 §4.4 session-isolation commitment).
func Feature() mycli.Feature {
	cfg := &config{}
	return mycli.Feature{
		Name: "BIGQUERY",
		StatementDefs: []*mycli.StatementDef{
			{
				Descriptions: []mycli.StatementDescription{
					{
						Usage:  `Execute BigQuery SQL`,
						Syntax: `BIGQUERY <sql>`,
					},
				},
				Pattern: regexp.MustCompile(`(?is)^BIGQUERY\s+(?P<sql>\S.*)$`),
				HandleGroups: func(groups map[string]string) (mycli.Statement, error) {
					return newBigQueryStatement(groups["sql"], cfg), nil
				},
			},
		},
		Vars: []mycli.FeatureVar{
			{
				Name: "CLI_BIGQUERY_PROJECT",
				Desc: "GCP project for BigQuery queries. Defaults to CLI_PROJECT when empty.",
				Var:  mycli.StringVar(&cfg.Project),
			},
			{
				Name: "CLI_BIGQUERY_LOCATION",
				Desc: "BigQuery location for queries (e.g. US, EU).",
				Var:  mycli.StringVar(&cfg.Location),
			},
			{
				Name: "CLI_BIGQUERY_MAX_BYTES_BILLED",
				Desc: "Maximum bytes billed per BigQuery query.",
				Var:  mycli.NullableIntVar(&cfg.MaxBytesBilled),
			},
		},
	}
}

// BigQueryStatement executes a single BigQuery SQL statement. It embeds
// mycli.MutationClassifier (Classify wired in the constructor to the fail-closed
// first-keyword classifier over its own SQL) so the READONLY guard can reject
// mutating BigQuery statements, and mycli.MarksDetachedCompatible so it runs in
// detached session mode (admin-only, no Spanner database).
type BigQueryStatement struct {
	mycli.MutationClassifier
	mycli.MarksDetachedCompatible
	SQL string
	cfg *config
}

// Compile-time assertions that the marker embeds promote the (unexported) core
// marker methods across the package boundary (#778 §4.3). A typo that dropped an
// embed would silently bypass the READONLY / detached guards.
var (
	_ mycli.ConditionallyMutatingStatement = (*BigQueryStatement)(nil)
	_ mycli.DetachedCompatible             = (*BigQueryStatement)(nil)
)

// newBigQueryStatement builds a BigQueryStatement carrying a reference to the
// feature-owned config and wires the mutation classifier over its own SQL.
func newBigQueryStatement(sql string, cfg *config) *BigQueryStatement {
	s := &BigQueryStatement{SQL: sql, cfg: cfg}
	s.Classify = func() bool { return bigQueryStatementMutates(s.SQL) }
	return s
}

// effectiveProject resolves the BigQuery project: the explicit
// CLI_BIGQUERY_PROJECT if set, otherwise the session's Spanner project
// (CLI_PROJECT). Kept as client-build logic (not a variable getter) per #778
// §4.4.
func effectiveProject(cfgProject, sessionProject string) string {
	return cmp.Or(cfgProject, sessionProject)
}

// clientKey identifies the effective BigQuery client configuration. A cached
// client is reused only while these values are unchanged; a SET of
// CLI_BIGQUERY_PROJECT/CLI_BIGQUERY_LOCATION produces a new key and forces a
// rebuild.
type clientKey struct {
	project  string
	location string
}

// clientCache is the per-Session lazy BigQuery client with rebuild-on-reconfig.
// It is stored in the Session feature store (mycli.FeatureState) and closed at
// Session.Close via the io.Closer contract.
//
// Unlike the original in-core field cache, access is serialized by mu rather
// than relying on the single-threaded statement path, so the check-then-act on
// the cached client is atomic (.gemini/styleguide.md §Concurrency).
type clientCache struct {
	cfg *config

	mu     sync.Mutex
	client *bq.Client
	key    clientKey
}

// Close closes the cached client, if any. Satisfies io.Closer so the feature
// store closes it at the end of Session.Close.
func (c *clientCache) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client == nil {
		return nil
	}
	err := c.client.Close()
	c.client = nil
	return err
}

// get returns a BigQuery client for the current CLI_BIGQUERY_PROJECT/LOCATION,
// building auth options lazily on first use. Deferring option construction until
// the first BIGQUERY statement keeps BigQuery credential resolution
// (ADC/ADCPlus/impersonation) out of ordinary Spanner/emulator session creation,
// which must succeed without any BigQuery credentials. The cached client is
// rebuilt when the effective (project, location) changes.
func (c *clientCache) get(ctx context.Context, session *mycli.Session) (*bq.Client, error) {
	project := effectiveProject(c.cfg.Project, session.ProjectID())
	if project == "" {
		return nil, fmt.Errorf("BigQuery project not configured: set CLI_BIGQUERY_PROJECT or CLI_PROJECT")
	}
	location := c.cfg.Location
	key := clientKey{project: project, location: location}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client != nil {
		if c.key == key {
			return c.client, nil
		}
		// CLI_BIGQUERY_PROJECT/LOCATION changed since the client was built;
		// drop the stale client and rebuild for the new configuration.
		if err := c.client.Close(); err != nil {
			slog.Warn("error on bigquery.Client.Close() during reconfigure", "err", err)
		}
		c.client = nil
		c.key = clientKey{}
	}

	// allowWithoutAuthentication is false: BigQuery uses real credentials even
	// when the Spanner endpoint is an emulator with WithoutAuthentication.
	opts, err := session.AuthOptions(ctx, session.CredentialBytes(), false)
	if err != nil {
		return nil, err
	}
	client, err := bq.NewClient(ctx, project, opts...)
	if err != nil {
		return nil, err
	}
	if location != "" {
		client.Location = location
	}
	c.client = client
	c.key = key
	return client, nil
}

// Execute runs the BigQuery SQL and returns the result rows as display strings.
func (s *BigQueryStatement) Execute(ctx context.Context, session *mycli.Session) (*mycli.Result, error) {
	cache, err := mycli.FeatureState(ctx, session, clientStateKey,
		func(context.Context, *mycli.Session) (*clientCache, error) {
			return &clientCache{cfg: s.cfg}, nil
		})
	if err != nil {
		return nil, err
	}

	client, err := cache.get(ctx, session)
	if err != nil {
		return nil, err
	}

	q := client.Query(s.SQL)
	if loc := s.cfg.Location; loc != "" {
		q.Location = loc
	}
	if maxBytes := s.cfg.MaxBytesBilled; maxBytes != nil {
		q.MaxBytesBilled = *maxBytes
	}

	it, err := q.Read(ctx)
	if err != nil {
		return nil, err
	}

	var rows []mycli.Row
	var values []bq.Value
	var headers []string
	for {
		err := it.Next(&values)
		if len(headers) == 0 {
			for _, field := range it.Schema {
				headers = append(headers, field.Name)
			}
		}
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		rowValues := make([]string, len(values))
		for i, v := range values {
			// Pass the schema field type so NUMERIC vs BIGNUMERIC scale can be
			// distinguished (both arrive as *big.Rat).
			var fieldType bq.FieldType
			if i < len(it.Schema) {
				fieldType = it.Schema[i].Type
			}
			rowValues[i] = formatBigQueryValue(v, fieldType)
		}
		rows = append(rows, mycli.NewRow(rowValues...))
	}

	return &mycli.Result{
		TableHeader:  mycli.NewTableHeader(headers...),
		Rows:         rows,
		AffectedRows: len(rows),
	}, nil
}
