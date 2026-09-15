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
	"strconv"
	"strings"
	"testing"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/google/go-cmp/cmp"
)

func TestMaxPartitionsRejectsInvalidWithoutMutation(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name  string
		value string
	}{
		{name: "negative", value: "-1"},
		{name: "null", value: "NULL"},
		{name: "overflow", value: "9223372036854775808"},
		{name: "invalid", value: "abc"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			local := newSystemVariablesWithDefaultsForTest()
			local.ensureRegistry()
			if err := local.SetFromSimple("MAX_PARTITIONS", "7"); err != nil {
				t.Fatal(err)
			}
			err := local.SetFromSimple("MAX_PARTITIONS", tt.value)
			if err == nil {
				t.Fatalf("SET MAX_PARTITIONS=%s succeeded, want error", tt.value)
			}
			got, getErr := local.Get("MAX_PARTITIONS")
			if getErr != nil {
				t.Fatal(getErr)
			}
			if diff := cmp.Diff(singletonMap("MAX_PARTITIONS", "7"), got); diff != "" {
				t.Errorf("rejected SET mutated state (-want +got):\n%s", diff)
			}
		})
	}
}

func TestMaxPartitionsDefaultResetAndStartup(t *testing.T) {
	t.Parallel()

	sv := newSystemVariablesWithDefaultsForTest()
	sv.ensureRegistry()
	got, err := sv.Get("MAX_PARTITIONS")
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(singletonMap("MAX_PARTITIONS", "0"), got); diff != "" {
		t.Errorf("default (-want +got):\n%s", diff)
	}

	sv.Query.MaxPartitions = 3
	if err := sv.CaptureStartupSnapshots(); err != nil {
		t.Fatal(err)
	}
	if err := sv.SetFromSimple("MAX_PARTITIONS", "9"); err != nil {
		t.Fatal(err)
	}
	if err := sv.Reset("MAX_PARTITIONS"); err != nil {
		t.Fatal(err)
	}
	got, err = sv.Get("MAX_PARTITIONS")
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(singletonMap("MAX_PARTITIONS", "3"), got); diff != "" {
		t.Errorf("RESET after startup snapshot (-want +got):\n%s", diff)
	}
}

func TestMaxPartitionsAcceptsAboveAdvertisedMaximum(t *testing.T) {
	t.Parallel()
	sv := newSystemVariablesWithDefaultsForTest()
	sv.ensureRegistry()
	if err := sv.SetFromSimple("MAX_PARTITIONS", "200001"); err != nil {
		t.Fatalf("client must not cap at 200000: %v", err)
	}
	got, err := sv.Get("MAX_PARTITIONS")
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(singletonMap("MAX_PARTITIONS", "200001"), got); diff != "" {
		t.Errorf("(-want +got):\n%s", diff)
	}
}

func TestMaxPartitionsWireEntryPaths(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	srv, opts := startDirectedReadDial(t)
	vars := newDirectedReadVars(t)
	session, _ := newDirectedReadProductSession(t, opts, vars)

	for _, max := range []int64{0, 7} {
		if err := vars.SetFromSimple("MAX_PARTITIONS", strconv.FormatInt(max, 10)); err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			name string
			run  func()
		}{
			{
				name: "PARTITION",
				run: func() {
					if _, err := session.ExecuteStatement(ctx, &PartitionStatement{SQL: "SELECT 1"}); err != nil {
						t.Fatal(err)
					}
				},
			},
			{
				name: "TRY PARTITIONED QUERY",
				run: func() {
					if _, err := session.ExecuteStatement(ctx, &TryPartitionedQueryStatement{SQL: "SELECT 1"}); err != nil {
						t.Fatal(err)
					}
				},
			},
			{
				name: "RUN PARTITIONED QUERY",
				run: func() {
					if _, err := session.ExecuteStatement(ctx, &RunPartitionedQueryStatement{SQL: "SELECT 1"}); err != nil {
						t.Fatal(err)
					}
				},
			},
			{
				name: "AUTO_PARTITION_MODE",
				run: func() {
					if err := vars.SetFromSimple("AUTO_PARTITION_MODE", "TRUE"); err != nil {
						t.Fatal(err)
					}
					defer func() {
						if err := vars.SetFromSimple("AUTO_PARTITION_MODE", "FALSE"); err != nil {
							t.Fatal(err)
						}
					}()
					if _, err := execSQL(t, ctx, session, "SELECT 1"); err != nil {
						t.Fatal(err)
					}
				},
			},
		} {
			t.Run(strconv.FormatInt(max, 10)+"/"+tc.name, func(t *testing.T) {
				srv.takePartitionQueries()
				tc.run()
				reqs := srv.takePartitionQueries()
				if len(reqs) != 1 {
					t.Fatalf("PartitionQuery count=%d want 1", len(reqs))
				}
				if reqs[0].GetPartitionOptions() == nil {
					t.Fatal("PartitionOptions omitted; default 0 still includes the object")
				}
				if got := reqs[0].GetPartitionOptions().GetMaxPartitions(); got != max {
					t.Fatalf("wire max_partitions=%d want %d", got, max)
				}
			})
		}
	}
}

func TestMaxPartitionsIndependentOfParallelism(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	srv, opts := startDirectedReadDial(t)
	srv.nPartitions = 3
	vars := newDirectedReadVars(t)
	session, _ := newDirectedReadProductSession(t, opts, vars)

	if err := vars.SetFromSimple("MAX_PARTITIONS", "1"); err != nil {
		t.Fatal(err)
	}
	if err := vars.SetFromSimple("MAX_PARTITIONED_PARALLELISM", "1"); err != nil {
		t.Fatal(err)
	}
	srv.takePartitionQueries()
	srv.takeRequests()
	result, err := session.ExecuteStatement(ctx, &RunPartitionedQueryStatement{SQL: "SELECT 1"})
	if err != nil {
		t.Fatal(err)
	}
	reqs := srv.takePartitionQueries()
	if len(reqs) != 1 {
		t.Fatalf("PartitionQuery count=%d", len(reqs))
	}
	if got := reqs[0].GetPartitionOptions().GetMaxPartitions(); got != 1 {
		t.Fatalf("hint=%d want 1 (parallelism must not rewrite it)", got)
	}
	if result == nil || result.PartitionCount != 3 {
		t.Fatalf("returned partitions=%v, fake must not truncate 3 tokens when hint is 1", result)
	}
	var tokenExecs int
	for _, req := range srv.takeRequests() {
		if len(req.GetPartitionToken()) > 0 {
			tokenExecs++
		}
	}
	if tokenExecs != 3 {
		t.Fatalf("executed partitions=%d want 3 (hint 1 must not drop tokens)", tokenExecs)
	}
	if result.AffectedRows != 3 {
		t.Fatalf("AffectedRows=%d want 3 so all returned partitions are represented", result.AffectedRows)
	}

	if err := vars.SetFromSimple("MAX_PARTITIONS", "0"); err != nil {
		t.Fatal(err)
	}
	if err := vars.SetFromSimple("MAX_PARTITIONED_PARALLELISM", "8"); err != nil {
		t.Fatal(err)
	}
	srv.takePartitionQueries()
	if _, err := session.ExecuteStatement(ctx, &RunPartitionedQueryStatement{SQL: "SELECT 1"}); err != nil {
		t.Fatal(err)
	}
	reqs = srv.takePartitionQueries()
	if len(reqs) != 1 {
		t.Fatalf("PartitionQuery count=%d", len(reqs))
	}
	if got := reqs[0].GetPartitionOptions().GetMaxPartitions(); got != 0 {
		t.Fatalf("hint=%d want 0 after lowering MAX_PARTITIONS", got)
	}
}

func TestMaxPartitionsNormalQueryUnchanged(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	srv, opts := startDirectedReadDial(t)
	vars := newDirectedReadVars(t)
	session, _ := newDirectedReadProductSession(t, opts, vars)
	if err := vars.SetFromSimple("MAX_PARTITIONS", "10"); err != nil {
		t.Fatal(err)
	}
	srv.takePartitionQueries()
	srv.takeRequests()
	if _, err := execSQL(t, ctx, session, "SELECT 1"); err != nil {
		t.Fatal(err)
	}
	if reqs := srv.takePartitionQueries(); len(reqs) != 0 {
		t.Fatalf("ordinary SELECT sent PartitionQuery: %v", reqs)
	}
	execs := srv.takeRequests()
	if len(execs) == 0 {
		t.Fatal("ordinary SELECT sent no ExecuteSql")
	}
	for _, req := range execs {
		if req.GetPartitionToken() != nil {
			t.Fatalf("ordinary SELECT used a partition token: %v", req)
		}
	}
}

func TestMaxPartitionsRejectsWithoutRPC(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	srv, opts := startDirectedReadDial(t)
	vars := newDirectedReadVars(t)
	session, _ := newDirectedReadProductSession(t, opts, vars)
	srv.takePartitionQueries()
	_, err := execSQL(t, ctx, session, "SET MAX_PARTITIONS = -1")
	if err == nil {
		t.Fatal("SET MAX_PARTITIONS=-1 succeeded")
	}
	if !strings.Contains(err.Error(), "non-negative") {
		t.Fatalf("error=%v, want non-negative", err)
	}
	if reqs := srv.takePartitionQueries(); len(reqs) != 0 {
		t.Fatalf("rejected SET sent PartitionQuery: %v", reqs)
	}
	if got := mustGetVar(t, session, "MAX_PARTITIONS"); got != "0" {
		t.Fatalf("rejected SET mutated value=%q", got)
	}
}

func TestMaxPartitionsLocalLifecycle(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	srv, opts := startDirectedReadDial(t)
	vars := newDirectedReadVars(t)
	session, _ := newDirectedReadProductSession(t, opts, vars)
	if err := vars.SetFromSimple("MAX_PARTITIONS", "5"); err != nil {
		t.Fatal(err)
	}

	if _, err := session.ExecuteStatement(ctx, &BeginStatement{Priority: sppb.RequestOptions_PRIORITY_UNSPECIFIED}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: "MAX_PARTITIONS", Value: "10"}); err != nil {
		t.Fatal(err)
	}
	if got := mustGetVar(t, session, "MAX_PARTITIONS"); got != "10" {
		t.Fatalf("LOCAL during txn=%q", got)
	}
	srv.takePartitionQueries()
	if _, err := session.ExecuteStatement(ctx, &PartitionStatement{SQL: "SELECT 1"}); err != nil {
		t.Fatal(err)
	}
	reqs := srv.takePartitionQueries()
	if len(reqs) != 1 || reqs[0].GetPartitionOptions().GetMaxPartitions() != 10 {
		t.Fatalf("LOCAL not frozen at request: %v", reqs)
	}

	if _, err := session.ExecuteStatement(ctx, &CommitStatement{}); err != nil {
		t.Fatal(err)
	}
	if got := mustGetVar(t, session, "MAX_PARTITIONS"); got != "5" {
		t.Fatalf("after COMMIT=%q, want session 5", got)
	}
	srv.takePartitionQueries()
	if _, err := session.ExecuteStatement(ctx, &PartitionStatement{SQL: "SELECT 1"}); err != nil {
		t.Fatal(err)
	}
	reqs = srv.takePartitionQueries()
	if len(reqs) != 1 || reqs[0].GetPartitionOptions().GetMaxPartitions() != 5 {
		t.Fatalf("session value after LOCAL end: %v", reqs)
	}
}

func TestMaxPartitionsRunPartitionDoesNotRepartition(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	srv, opts := startDirectedReadDial(t)
	vars := newDirectedReadVars(t)
	session, _ := newDirectedReadProductSession(t, opts, vars)
	if err := vars.SetFromSimple("MAX_PARTITIONS", "3"); err != nil {
		t.Fatal(err)
	}
	tokens := mustPartition(t, session, "SELECT 1")
	if err := vars.SetFromSimple("MAX_PARTITIONS", "99"); err != nil {
		t.Fatal(err)
	}
	srv.takePartitionQueries()
	if _, err := session.ExecuteStatement(ctx, &RunPartitionStatement{Token: tokens[0]}); err != nil {
		t.Fatal(err)
	}
	if reqs := srv.takePartitionQueries(); len(reqs) != 0 {
		t.Fatalf("RUN PARTITION repartitioned: %v", reqs)
	}
}
