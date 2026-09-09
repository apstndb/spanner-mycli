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
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanner-mycli/enums"
	"google.golang.org/protobuf/proto"
)

func TestDirectedReadROPlanAndProfileABClear(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	srv, opts := startDirectedReadDial(t)
	vars := newDirectedReadVars(t)
	session, _ := newDirectedReadProductSession(t, opts, vars)
	stmt := spanner.Statement{SQL: "SELECT 'observed'"}

	for _, sel := range directedReadABClear(t) {
		t.Run(sel.name, func(t *testing.T) {
			if err := vars.SetFromSimple("DIRECTED_READ", sel.set); err != nil {
				t.Fatal(err)
			}

			srv.takeRequests()
			plan, _, err := session.txn.RunAnalyzeQuery(ctx, stmt)
			if err != nil {
				t.Fatalf("RO PLAN: %v", err)
			}
			requireDirectedPlan(t, plan)
			planReqs := srv.takeRequests()
			if len(planReqs) != 1 || planReqs[0].QueryMode != sppb.ExecuteSqlRequest_PLAN || planReqs[0].Sql != stmt.SQL {
				t.Fatalf("RO PLAN requests=%v", planReqs)
			}
			requireDirected(t, planReqs[0].DirectedReadOptions, sel.want)
			if planReqs[0].GetTransaction().GetId() != nil && string(planReqs[0].GetTransaction().GetId()) == "probe-rw" {
				t.Fatal("RO PLAN used RW transaction identity")
			}

			result, err := executeExplain(ctx, session, stmt.SQL, false, enums.ExplainFormatUnspecified, 0, nil)
			if err != nil || result == nil || result.AffectedRows < 1 {
				t.Fatalf("EXPLAIN result=%v error=%v", result, err)
			}

			srv.takeRequests()
			it, txn, err := session.txn.RunQueryWithStats(ctx, stmt, false, sppb.ExecuteSqlRequest_PROFILE)
			if err != nil {
				t.Fatalf("RO PROFILE: %v", err)
			}
			var n int64
			stats, _, _, profilePlan, err := consumeRowIter(it, func(*spanner.Row) error {
				n++
				return nil
			})
			if txn != nil {
				txn.Close()
			}
			if err != nil || n != 1 {
				t.Fatalf("RO PROFILE consume rows=%d error=%v", n, err)
			}
			requireDirectedPlan(t, profilePlan)
			requireDirectedProfileStats(t, stats)
			profileReqs := srv.takeRequests()
			if len(profileReqs) != 1 || profileReqs[0].QueryMode != sppb.ExecuteSqlRequest_PROFILE {
				t.Fatalf("RO PROFILE requests=%v", profileReqs)
			}
			requireDirected(t, profileReqs[0].DirectedReadOptions, sel.want)
		})
	}
}

func TestDirectedReadSelectedROPathsABClear(t *testing.T) {
	t.Parallel()

	t.Run("begin-ro-init", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
		defer cancel()
		srv, opts := startDirectedReadDial(t)
		vars := newDirectedReadVars(t)
		session, _ := newDirectedReadProductSession(t, opts, vars)
		for _, sel := range directedReadABClear(t) {
			if err := vars.SetFromSimple("DIRECTED_READ", sel.set); err != nil {
				t.Fatal(err)
			}
			srv.takeRequests()
			if _, err := session.ExecuteStatement(ctx, &BeginRoStatement{}); err != nil {
				t.Fatalf("%s BEGIN RO: %v", sel.name, err)
			}
			reqs := srv.takeRequests()
			if len(reqs) != 1 || reqs[0].Sql != "SELECT 1" {
				t.Fatalf("%s BEGIN RO SELECT 1 requests=%v", sel.name, reqs)
			}
			requireDirected(t, reqs[0].DirectedReadOptions, sel.want)
			if _, err := session.ExecuteStatement(ctx, &CommitStatement{}); err != nil {
				t.Fatalf("%s CLOSE RO: %v", sel.name, err)
			}
		}
	})

	t.Run("ro-subsequent", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
		defer cancel()
		srv, opts := startDirectedReadDial(t)
		vars := newDirectedReadVars(t)
		session, _ := newDirectedReadProductSession(t, opts, vars)
		for _, sel := range directedReadABClear(t) {
			if err := vars.SetFromSimple("DIRECTED_READ", sel.set); err != nil {
				t.Fatal(err)
			}
			if _, err := session.ExecuteStatement(ctx, &BeginRoStatement{}); err != nil {
				t.Fatal(err)
			}
			srv.takeRequests()
			it, _, err := session.txn.RunQuery(ctx, spanner.Statement{SQL: "SELECT 'observed'"})
			if err != nil {
				t.Fatal(err)
			}
			got := consumeDirectedReadRequest(t, srv, it, sel.want)
			if string(got.GetTransaction().GetId()) != "probe-ro" {
				t.Fatalf("%s subsequent RO txn=%v", sel.name, got.Transaction)
			}
			if _, err := session.ExecuteStatement(ctx, &CommitStatement{}); err != nil {
				t.Fatal(err)
			}
		}
	})

	t.Run("independent-single-use", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
		defer cancel()
		srv, opts := startDirectedReadDial(t)
		vars := newDirectedReadVars(t)
		session, _ := newDirectedReadProductSession(t, opts, vars)
		for _, sel := range directedReadABClear(t) {
			if err := vars.SetFromSimple("DIRECTED_READ", sel.set); err != nil {
				t.Fatal(err)
			}
			if _, err := session.ExecuteStatement(ctx, &BeginRoStatement{}); err != nil {
				t.Fatal(err)
			}
			srv.takeRequests()
			it, txn, err := session.txn.RunSingleUseQueryWithStats(ctx, spanner.Statement{SQL: "SELECT 'observed'"}, sppb.ExecuteSqlRequest_PROFILE)
			if err != nil {
				t.Fatal(err)
			}
			var n int64
			stats, _, _, plan, err := consumeRowIter(it, func(*spanner.Row) error {
				n++
				return nil
			})
			if txn != nil {
				txn.Close()
			}
			if err != nil || n != 1 {
				t.Fatalf("%s independent PROFILE rows=%d error=%v", sel.name, n, err)
			}
			requireDirectedPlan(t, plan)
			requireDirectedProfileStats(t, stats)
			reqs := srv.takeRequests()
			if len(reqs) != 1 || reqs[0].QueryMode != sppb.ExecuteSqlRequest_PROFILE || reqs[0].GetTransaction().GetSingleUse() == nil {
				t.Fatalf("%s independent requests=%v", sel.name, reqs)
			}
			requireDirected(t, reqs[0].DirectedReadOptions, sel.want)
			if !session.txn.InReadOnlyTransaction() {
				t.Fatal("independent single-use consumed the user RO transaction")
			}
			if _, err := session.ExecuteStatement(ctx, &CommitStatement{}); err != nil {
				t.Fatal(err)
			}
		}
	})

	t.Run("database-exists", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
		defer cancel()
		srv, opts := startDirectedReadDial(t)
		vars := newDirectedReadVars(t)
		session, _ := newDirectedReadProductSession(t, opts, vars)
		for _, sel := range directedReadABClear(t) {
			if err := vars.SetFromSimple("DIRECTED_READ", sel.set); err != nil {
				t.Fatal(err)
			}
			srv.takeRequests()
			exists, err := session.DatabaseExists(ctx)
			if err != nil || !exists {
				t.Fatalf("%s DatabaseExists exists=%v err=%v", sel.name, exists, err)
			}
			reqs := srv.takeRequests()
			if len(reqs) == 0 || reqs[0].Sql != "SELECT 1" {
				t.Fatalf("%s DatabaseExists requests=%v", sel.name, reqs)
			}
			requireDirected(t, reqs[0].DirectedReadOptions, sel.want)
		}
	})

	t.Run("partition", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
		defer cancel()
		srv, opts := startDirectedReadDial(t)
		vars := newDirectedReadVars(t)
		session, _ := newDirectedReadProductSession(t, opts, vars)
		vars.Query.DataBoostEnabled = true
		vars.Query.RPCPriority = sppb.RequestOptions_PRIORITY_HIGH
		vars.Query.MaxPartitionedParallelism = 1
		for _, sel := range directedReadABClear(t) {
			if err := vars.SetFromSimple("DIRECTED_READ", sel.set); err != nil {
				t.Fatal(err)
			}
			srv.takeRequests()
			srv.takePartitionQueries()
			result, err := runPartitionedQuery(ctx, session, "SELECT 'observed'")
			if err != nil || result == nil || result.AffectedRows != 1 || result.PartitionCount != 1 {
				t.Fatalf("%s partition result=%v error=%v", sel.name, result, err)
			}
			reqs := srv.takeRequests()
			if len(reqs) == 0 {
				t.Fatalf("%s partition sent no ExecuteSql", sel.name)
			}
			var exec *sppb.ExecuteSqlRequest
			for _, req := range reqs {
				if req.GetPartitionToken() != nil {
					exec = req
					break
				}
			}
			if exec == nil {
				t.Fatalf("%s no ExecuteSql with partition token: %v", sel.name, dumpSQLs(reqs))
			}
			requireDirected(t, exec.DirectedReadOptions, sel.want)
			if !exec.GetDataBoostEnabled() {
				t.Fatal("partition DataBoostEnabled not forwarded")
			}
			if exec.GetRequestOptions().GetPriority() != sppb.RequestOptions_PRIORITY_HIGH {
				t.Fatalf("partition priority=%v", exec.GetRequestOptions().GetPriority())
			}
			// MaxPartitionedParallelism is local fan-in, not a PartitionQuery field.
			// Inspect ExecuteSqlRequest tokens above; do not treat PartitionQuery as routing evidence.
			if len(srv.takePartitionQueries()) == 0 {
				t.Fatalf("%s PartitionQuery was not sent", sel.name)
			}
		}
		srv.takeRequests()
		it, txn, err := session.txn.RunQuery(ctx, spanner.Statement{SQL: "SELECT 'observed'"})
		if err != nil {
			t.Fatal(err)
		}
		consumeDirectedReadRequest(t, srv, it, nil)
		if txn != nil {
			txn.Close()
		}
		// runPartitionedQuery already ran local Cleanup+Close. A later SELECT
		// succeeding is the client-lifecycle postcondition. DeleteSession
		// counts are not remote partition-token invalidation.
	})
}

func TestDirectedReadCompletionPreservesTxnCacheAndTag(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	srv, opts := startDirectedReadDial(t)
	vars := newDirectedReadVars(t)
	session, _ := newDirectedReadProductSession(t, opts, vars)
	handler := NewSessionHandler(session)
	f := &fuzzyFinderCommand{cli: &Cli{SessionHandler: handler, SystemVariables: vars}}

	for _, sel := range directedReadABClear(t) {
		if err := vars.SetFromSimple("DIRECTED_READ", sel.set); err != nil {
			t.Fatal(err)
		}
		vars.Transaction.RequestTag = "keep-me"
		srv.takeRequests()
		items, err := f.fetchSchemaCandidates(ctx)
		if err != nil {
			t.Fatalf("%s completion: %v", sel.name, err)
		}
		if len(items) != 1 || items[0].Value != "fixture_schema" {
			t.Fatalf("%s candidates=%v, want fixture_schema", sel.name, items)
		}
		if vars.Transaction.RequestTag != "keep-me" {
			t.Fatal("completion consumed STATEMENT_TAG")
		}
		if session.txn.InTransaction() {
			t.Fatal("autocommit completion started a user transaction")
		}
		reqs := srv.takeRequests()
		if len(reqs) == 0 || !strings.Contains(reqs[0].Sql, "INFORMATION_SCHEMA.SCHEMATA") {
			t.Fatalf("%s completion SQL=%v", sel.name, dumpSQLs(reqs))
		}
		requireDirected(t, reqs[0].DirectedReadOptions, sel.want)
	}

	b := mustParseDirectedRead(t, "us-west1:READ_WRITE")
	if err := vars.SetFromSimple("DIRECTED_READ", "us-west1:READ_WRITE"); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &BeginRoStatement{}); err != nil {
		t.Fatal(err)
	}
	srv.takeRequests()
	it, _, err := session.txn.RunQuery(ctx, spanner.Statement{SQL: "SELECT 'observed'"})
	if err != nil {
		t.Fatal(err)
	}
	owner := consumeDirectedReadRequest(t, srv, it, b)
	if string(owner.GetTransaction().GetId()) != "probe-ro" {
		t.Fatalf("owner txn=%v", owner.Transaction)
	}

	sentinel := []fzfItem{{Value: "cached-sentinel"}}
	f.setCachedCandidates(fuzzyCompleteSchema, sentinel)
	cacheGen := f.schemaCache.schemaGeneration
	cacheSession := f.schemaCache.session
	vars.Transaction.RequestTag = "keep-me"
	srv.takeRequests()
	items, err := f.fetchSchemaCandidates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Value != "fixture_schema" {
		t.Fatalf("in-txn candidates=%v", items)
	}
	if vars.Transaction.RequestTag != "keep-me" {
		t.Fatal("in-txn completion consumed STATEMENT_TAG")
	}
	if f.schemaCache == nil || len(f.schemaCache.candidates) != 1 || f.schemaCache.candidates[0].Value != "cached-sentinel" {
		t.Fatal("fetchSchemaCandidates mutated the existing schema cache")
	}
	if f.schemaCache.schemaGeneration != cacheGen || f.schemaCache.session != cacheSession {
		t.Fatal("completion changed cache session identity or schema generation")
	}
	if !session.txn.InReadOnlyTransaction() {
		t.Fatal("completion ended the user RO transaction")
	}
	reqs := srv.takeRequests()
	if len(reqs) == 0 {
		t.Fatal("in-txn completion sent no request")
	}
	requireDirected(t, reqs[0].DirectedReadOptions, b)
	if string(reqs[0].GetTransaction().GetId()) == "probe-ro" {
		t.Fatal("completion reused the user RO transaction instead of Single()")
	}

	cached, err := f.resolveCandidates(ctx, fuzzyCompleteSchema, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cached) != 1 || cached[0].Value != "cached-sentinel" {
		t.Fatalf("cache hit=%v, want cached-sentinel", cached)
	}

	srv.takeRequests()
	it, _, err = session.txn.RunQuery(ctx, spanner.Statement{SQL: "SELECT 'observed'"})
	if err != nil {
		t.Fatal(err)
	}
	again := consumeDirectedReadRequest(t, srv, it, b)
	if string(again.GetTransaction().GetId()) != "probe-ro" {
		t.Fatalf("user RO identity after completion=%v", again.Transaction)
	}
}

func TestDirectedReadSessionHandlerLifecycleWire(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	srv, opts := startDirectedReadDial(t)
	a := mustParseDirectedRead(t, "us-east1:READ_ONLY")
	b := mustParseDirectedRead(t, "us-west1:READ_WRITE")
	c := mustParseDirectedRead(t, "asia-northeast1:READ_ONLY")
	embed := mustParseDirectedRead(t, "europe-west1:READ_ONLY")
	embedCopy := proto.CloneOf(embed)

	vars := newDirectedReadVars(t)
	vars.Query.DirectedRead = a
	vars.Config.EmbeddedClientConfig = &spanner.ClientConfig{
		DisableNativeMetrics: true,
		DisableRouteToLeader: true,
		UserAgent:            "embedded-directed-lifecycle",
		DirectedReadOptions:  embed,
	}
	session, cfg := newDirectedReadProductSession(t, opts, vars)
	if cfg.DirectedReadOptions != nil {
		t.Fatalf("copied client DRO=%v want nil", cfg.DirectedReadOptions)
	}
	handler := NewSessionHandler(session)
	if handler.systemVariables != vars {
		t.Fatal("handler forked systemVariables")
	}

	if _, err := handler.ExecuteStatement(ctx, &UseStatement{Database: vars.Connection.Database}); err != nil {
		t.Fatalf("USE: %v", err)
	}
	if handler.systemVariables != vars {
		t.Fatal("USE forked systemVariables")
	}
	if handler.clientConfig.UserAgent != "embedded-directed-lifecycle" || !handler.clientConfig.DisableRouteToLeader {
		t.Fatalf("USE dropped unrelated config: %+v", handler.clientConfig)
	}
	if handler.clientConfig.DirectedReadOptions != nil {
		t.Fatal("USE restored a client DRO default")
	}
	if vars.Config.EmbeddedClientConfig.DirectedReadOptions != embed || !proto.Equal(embed, embedCopy) {
		t.Fatal("USE mutated embedded DRO")
	}
	srv.takeRequests()
	it, txn, err := handler.txn.RunQuery(ctx, spanner.Statement{SQL: "SELECT 'observed'"})
	if err != nil {
		t.Fatal(err)
	}
	consumeDirectedReadRequest(t, srv, it, a)
	if txn != nil {
		txn.Close()
	}

	if _, err := handler.ExecuteStatement(ctx, &DetachStatement{}); err != nil {
		t.Fatalf("DETACH: %v", err)
	}
	if !handler.IsDetached() || handler.client != nil {
		t.Fatal("DETACH did not switch to an admin-only session")
	}
	if handler.systemVariables != vars {
		t.Fatal("DETACH forked systemVariables")
	}
	if handler.clientConfig.DirectedReadOptions != nil {
		t.Fatal("DETACH copied a client DRO default")
	}
	got, err := handler.systemVariables.Get("DIRECTED_READ")
	if err != nil || got["DIRECTED_READ"] != "us-east1:READ_ONLY" {
		t.Fatalf("registry after DETACH=%v err=%v", got, err)
	}

	if err := handler.systemVariables.SetFromSimple("DIRECTED_READ", "us-west1:READ_WRITE"); err != nil {
		t.Fatal(err)
	}
	if _, err := handler.ExecuteStatement(ctx, &UseStatement{Database: "db"}); err != nil {
		t.Fatalf("attach after detached SET: %v", err)
	}
	got, err = handler.systemVariables.Get("DIRECTED_READ")
	if err != nil || got["DIRECTED_READ"] != "us-west1:READ_WRITE" {
		t.Fatalf("registry after attach=%v err=%v", got, err)
	}
	if handler.clientConfig.DirectedReadOptions != nil {
		t.Fatal("reattach restored a client DRO default")
	}
	srv.takeRequests()
	it, txn, err = handler.txn.RunQuery(ctx, spanner.Statement{SQL: "SELECT 'observed'"})
	if err != nil {
		t.Fatal(err)
	}
	consumeDirectedReadRequest(t, srv, it, b)
	if txn != nil {
		txn.Close()
	}

	if _, err := handler.ExecuteStatement(ctx, &DetachStatement{}); err != nil {
		t.Fatal(err)
	}
	if err := handler.systemVariables.SetFromSimple("DIRECTED_READ", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := handler.ExecuteStatement(ctx, &UseStatement{Database: "db"}); err != nil {
		t.Fatalf("clear-then-attach: %v", err)
	}
	if handler.systemVariables.Query.DirectedRead != nil || handler.clientConfig.DirectedReadOptions != nil {
		t.Fatal("clear-then-attach resurrected a DRO default")
	}
	srv.takeRequests()
	it, txn, err = handler.txn.RunQuery(ctx, spanner.Statement{SQL: "SELECT 'observed'"})
	if err != nil {
		t.Fatal(err)
	}
	consumeDirectedReadRequest(t, srv, it, nil)
	if txn != nil {
		txn.Close()
	}

	if err := handler.systemVariables.SetFromSimple("DIRECTED_READ", "asia-northeast1:READ_ONLY"); err != nil {
		t.Fatal(err)
	}
	srv.takeRequests()
	it, txn, err = handler.txn.RunQuery(ctx, spanner.Statement{SQL: "SELECT 'observed'"})
	if err != nil {
		t.Fatal(err)
	}
	consumeDirectedReadRequest(t, srv, it, c)
	if txn != nil {
		txn.Close()
	}
}

func TestDirectedReadAdminConstructorEmbeddedDefaults(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	srv, opts := startDirectedReadDial(t)
	a := mustParseDirectedRead(t, "us-east1:READ_ONLY")
	embed := mustParseDirectedRead(t, "europe-west1:READ_ONLY")
	embedCopy := proto.CloneOf(embed)

	vars := newDirectedReadVars(t)
	vars.Connection.Database = ""
	vars.Query.DirectedRead = a
	vars.Config.EmbeddedClientConfig = &spanner.ClientConfig{
		DisableNativeMetrics: true,
		DisableRouteToLeader: true,
		UserAgent:            "embedded-admin-directed",
		DirectedReadOptions:  embed,
	}
	session, err := NewAdminSession(ctx, vars, opts...)
	if err != nil {
		t.Fatalf("NewAdminSession: %v", err)
	}
	t.Cleanup(session.Close)
	if !session.IsDetached() || session.client != nil {
		t.Fatal("admin constructor was not detached")
	}
	if session.clientConfig.DirectedReadOptions != nil {
		t.Fatalf("admin copied DRO=%v want nil", session.clientConfig.DirectedReadOptions)
	}
	if session.clientConfig.UserAgent != "embedded-admin-directed" || !session.clientConfig.DisableRouteToLeader || !session.clientConfig.DisableNativeMetrics {
		t.Fatalf("admin unrelated embedded fields changed: %+v", session.clientConfig)
	}
	if vars.Config.EmbeddedClientConfig.DirectedReadOptions != embed || !proto.Equal(embed, embedCopy) {
		t.Fatal("admin constructor mutated embedded DRO")
	}
	got, err := vars.Get("DIRECTED_READ")
	if err != nil || got["DIRECTED_READ"] != "us-east1:READ_ONLY" {
		t.Fatalf("admin registry=%v err=%v", got, err)
	}

	handler := NewSessionHandler(session)
	if _, err := handler.ExecuteStatement(ctx, &UseStatement{Database: "db"}); err != nil {
		t.Fatalf("attach from admin: %v", err)
	}
	if handler.systemVariables != vars {
		t.Fatal("admin attach forked systemVariables")
	}
	if handler.clientConfig.DirectedReadOptions != nil {
		t.Fatal("admin attach restored embedded DRO as a client default")
	}
	srv.takeRequests()
	it, txn, err := handler.txn.RunQuery(ctx, spanner.Statement{SQL: "SELECT 'observed'"})
	if err != nil {
		t.Fatal(err)
	}
	consumeDirectedReadRequest(t, srv, it, a)
	if txn != nil {
		txn.Close()
	}
}

func TestDirectedReadRecreateClientFailureRecoveryWire(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	srv, opts := startDirectedReadDial(t)
	a := mustParseDirectedRead(t, "us-east1:READ_ONLY")
	b := mustParseDirectedRead(t, "us-west1:READ_WRITE")
	embed := mustParseDirectedRead(t, "europe-west1:READ_ONLY")

	vars := newDirectedReadVars(t)
	vars.Query.DirectedRead = a
	vars.Config.EmbeddedClientConfig = &spanner.ClientConfig{
		DisableNativeMetrics: true,
		UserAgent:            "recreate-directed",
		DirectedReadOptions:  embed,
	}
	session, _ := newDirectedReadProductSession(t, opts, vars)
	if err := vars.SetFromSimple("DIRECTED_READ", "us-west1:READ_WRITE"); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &BeginRoStatement{}); err != nil {
		t.Fatal(err)
	}
	oldClient := session.client
	if err := session.RecreateClient(ctx); err == nil {
		t.Fatal("RecreateClient during RO succeeded")
	}
	if session.client != oldClient {
		t.Fatal("failed RecreateClient replaced the live client")
	}
	if session.clientConfig.DirectedReadOptions != nil {
		t.Fatal("failed RecreateClient restored a client DRO default")
	}
	if !proto.Equal(vars.Query.DirectedRead, b) {
		t.Fatal("failed RecreateClient lost the live DIRECTED_READ value")
	}

	srv.takeRequests()
	it, _, err := session.txn.RunQuery(ctx, spanner.Statement{SQL: "SELECT 'observed'"})
	if err != nil {
		t.Fatal(err)
	}
	consumeDirectedReadRequest(t, srv, it, b)
	if _, err := session.ExecuteStatement(ctx, &CommitStatement{}); err != nil {
		t.Fatal(err)
	}

	if err := session.RecreateClient(ctx); err != nil {
		t.Fatalf("RecreateClient after CLOSE: %v", err)
	}
	if session.client == oldClient {
		t.Fatal("successful RecreateClient kept the old client")
	}
	if session.clientConfig.DirectedReadOptions != nil {
		t.Fatal("recreated clientConfig DRO not nil")
	}
	srv.takeRequests()
	it, txn, err := session.txn.RunQuery(ctx, spanner.Statement{SQL: "SELECT 'observed'"})
	if err != nil {
		t.Fatal(err)
	}
	consumeDirectedReadRequest(t, srv, it, b)
	if txn != nil {
		txn.Close()
	}

	if err := vars.SetFromSimple("DIRECTED_READ", ""); err != nil {
		t.Fatal(err)
	}
	if err := session.RecreateClient(ctx); err != nil {
		t.Fatal(err)
	}
	if session.clientConfig.DirectedReadOptions != nil || vars.Query.DirectedRead != nil {
		t.Fatal("clear then RecreateClient resurrected startup or embedded DRO")
	}
	srv.takeRequests()
	it, txn, err = session.txn.RunQuery(ctx, spanner.Statement{SQL: "SELECT 'observed'"})
	if err != nil {
		t.Fatal(err)
	}
	consumeDirectedReadRequest(t, srv, it, nil)
	if txn != nil {
		txn.Close()
	}
}

func TestDirectedReadRollbackAndCloseFailurePostconditions(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	srv, opts := startDirectedReadDial(t)
	vars := newDirectedReadVars(t)
	session, _ := newDirectedReadProductSession(t, opts, vars)
	if err := vars.SetFromSimple("DIRECTED_READ", "us-west1:READ_WRITE"); err != nil {
		t.Fatal(err)
	}

	if _, err := session.ExecuteStatement(ctx, &BeginRwStatement{}); err != nil {
		t.Fatal(err)
	}
	srv.failRollback.Store(true)
	if _, err := session.ExecuteStatement(ctx, &RollbackStatement{}); err != nil {
		t.Fatalf("ROLLBACK with failing RPC: %v", err)
	}
	if session.txn.InTransaction() {
		t.Fatal("local rollback left a user transaction")
	}
	if srv.rollbacks == 0 {
		t.Fatal("ROLLBACK did not send a Rollback RPC")
	}
	// Local transaction context is cleared even when the Rollback RPC fails.
	// This is not evidence that the server aborted the transaction.
	if err := vars.SetFromSimple("DIRECTED_READ", "us-east1:READ_ONLY"); err != nil {
		t.Fatalf("SET after local rollback cleanup: %v", err)
	}
	srv.failRollback.Store(false)

	if _, err := session.ExecuteStatement(ctx, &BeginRoStatement{}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &CommitStatement{}); err != nil {
		t.Fatalf("CLOSE RO: %v", err)
	}
	if session.txn.InTransaction() {
		t.Fatal("CLOSE left a user transaction")
	}
	if err := vars.SetFromSimple("DIRECTED_READ", ""); err != nil {
		t.Fatalf("SET after CLOSE: %v", err)
	}
	srv.takeRequests()
	it, txn, err := session.txn.RunQuery(ctx, spanner.Statement{SQL: "SELECT 'observed'"})
	if err != nil {
		t.Fatal(err)
	}
	consumeDirectedReadRequest(t, srv, it, nil)
	if txn != nil {
		txn.Close()
	}
}
