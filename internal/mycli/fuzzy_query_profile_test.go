// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mycli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/go-cmp/cmp"
	readline "github.com/nyaosorg/go-readline-ny"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestQueryProfileCompletionContext(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ input, prefix string }{
		{"SHOW QUERY PROFILE ", ""},
		{"show query profile 12", "12"},
		{"  SHOW\tQUERY\tPROFILE\t-3", "-3"},
		{"SHOW QUERY PROFILE -", "-"},
		{"SHOW QUERY PROFILE -6422424748333414178", "-6422424748333414178"},
		{"\nSHOW QUERY PROFILE 0", "0"},
	} {
		t.Run(tt.input, func(t *testing.T) {
			got := detectFuzzyContext(tt.input)
			wantStart := utf8.RuneCountInString(strings.TrimSuffix(tt.input, tt.prefix))
			if got.completionType != fuzzyCompleteQueryProfile || got.argPrefix != tt.prefix || got.suffix != "" || got.argStartPos != wantStart {
				t.Fatalf("context = %+v, want type query_profile prefix %q start %d", got, tt.prefix, wantStart)
			}
		})
	}
	for _, input := range []string{
		"SHOW QUERY PROFILES",
		"SHOW QUERY PROFILES ",
		"show query profiles 1",
		"SHOW QUERY PROFILE 1;",
		"SHOW QUERY PROFILE abc",
		"SELECT 'SHOW QUERY PROFILE 1'",
		"SHOW QUERY PROFILE",
	} {
		if got := detectFuzzyContext(input); got.completionType == fuzzyCompleteQueryProfile {
			t.Fatalf("unexpected query-profile completion for %q: %+v", input, got)
		}
	}
	if !requiresNetwork(fuzzyCompleteQueryProfile) || fuzzyCompleteQueryProfile.String() != "query_profile" || completionHeader(fuzzyCompleteQueryProfile) != "Query Profiles" {
		t.Fatal("query-profile completion registration mismatch")
	}
}

func testQueryProfileCompletionRow(fp int64, interval time.Time, queryText string) *queryProfilesRow {
	return &queryProfilesRow{
		IntervalEnd:     interval,
		TextFingerprint: fp,
		QueryProfile:    &queryProfiles{QueryStats: QueryStats{QueryText: queryText}},
	}
}

func TestQueryProfileCompletionCandidates(t *testing.T) {
	t.Parallel()
	older := time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2025, 5, 29, 8, 0, 0, 0, time.UTC)
	same := time.Date(2025, 5, 15, 0, 0, 0, 0, time.UTC)
	long := strings.Repeat("界", 121)
	got := queryProfileCompletionItems([]*queryProfilesRow{
		testQueryProfileCompletionRow(2, same, "SELECT two"),
		testQueryProfileCompletionRow(queryProfileFingerprint, older, "SELECT old preview"),
		testQueryProfileCompletionRow(queryProfileFingerprint, newer, "SELECT  DISTINCTIVE_FROM_SINGERS  WHERE\nid = 1"),
		testQueryProfileCompletionRow(3, same, ""),
		testQueryProfileCompletionRow(4, newer, long),
		{TextFingerprint: 5, IntervalEnd: newer},
		nil,
	})
	wantValues := []string{
		fmt.Sprintf("%d", queryProfileFingerprint),
		"4",
		"5",
		"2",
		"3",
	}
	var gotValues []string
	for _, item := range got {
		gotValues = append(gotValues, item.Value)
	}
	if diff := cmp.Diff(wantValues, gotValues); diff != "" {
		t.Fatalf("order/dedup mismatch (-want +got):\n%s", diff)
	}
	if got[0].Value != fmt.Sprintf("%d", queryProfileFingerprint) || !strings.Contains(got[0].Label, "SELECT DISTINCTIVE_FROM_SINGERS WHERE id = 1") {
		t.Fatalf("newest preview not used: %+v", got[0])
	}
	if strings.Contains(got[0].Label, "old preview") {
		t.Fatalf("stale interval preview leaked: %+v", got[0])
	}
	if got[0].Value != fmt.Sprintf("%d", queryProfileFingerprint) {
		t.Fatalf("Value must be the signed decimal fingerprint: %+v", got[0])
	}
	if got[4].Value != "3" || got[4].Label != "3" {
		t.Fatalf("empty preview must fall back to fingerprint: %+v", got[4])
	}
	if got[2].Value != "5" || got[2].Label != "5" {
		t.Fatalf("nil profile must fall back to fingerprint: %+v", got[2])
	}
	if got[1].Label != "4 "+strings.Repeat("界", 120)+"…" {
		t.Fatalf("unicode truncation = %q", got[1].Label)
	}
	for _, item := range got {
		if item.Value != fmt.Sprintf("%d", mustParseQueryProfileFingerprint(t, item.Value)) {
			t.Fatalf("Value is not an int64 decimal: %q", item.Value)
		}
		if strings.Contains(item.Value, "SELECT") || strings.Contains(item.Value, " ") {
			t.Fatalf("Value must be fingerprint-only: %q", item.Value)
		}
		if !utf8.ValidString(item.Label) || strings.ContainsFunc(item.Label, unicode.IsControl) {
			t.Fatalf("unsafe label: %q", item.Label)
		}
	}

	selected := runFzfFilter(got, "DISTINCTIVE_FROM_SINGERS", "Query Profiles", "--no-sort")
	wantSelected := []string{fmt.Sprintf("%d", queryProfileFingerprint)}
	if !reflect.DeepEqual(selected, wantSelected) {
		t.Fatalf("selected = %v, want %v", selected, wantSelected)
	}
	if len(selected) != 1 || selected[0] != fmt.Sprintf("%d", queryProfileFingerprint) {
		t.Fatalf("filter must return only the fingerprint: %v", selected)
	}
	stmt, err := BuildStatement("SHOW QUERY PROFILE " + selected[0])
	if err != nil {
		t.Fatal(err)
	}
	gotStmt, ok := stmt.(*ShowQueryProfileStatement)
	if !ok || gotStmt.Fprint != queryProfileFingerprint {
		t.Fatalf("inserted statement = %#v, want SHOW QUERY PROFILE %d", stmt, queryProfileFingerprint)
	}
}

func mustParseQueryProfileFingerprint(t *testing.T, s string) int64 {
	t.Helper()
	var fp int64
	if _, err := fmt.Sscan(s, &fp); err != nil {
		t.Fatalf("Sscan(%q): %v", s, err)
	}
	return fp
}

func TestQueryProfileCompletionFetchAndAdmission(t *testing.T) {
	t.Parallel()
	intervalEnd := time.Date(2025, 5, 29, 8, 0, 0, 0, time.UTC)
	planJSON := marshalQueryPlanJSON(t, loadTestPlan(t, queryProfileFilterPlanFile))
	profile := queryProfileRawJSON(t, planJSON, map[string]any{
		"query_text": "SELECT * FROM Singers",
	}, fmt.Sprint(queryProfileFingerprint))

	t.Run("current database route", func(t *testing.T) {
		t.Parallel()
		session, server := newQueryProfileRPCSession(t, []queryProfileRPCRow{{
			intervalEnd: intervalEnd,
			fingerprint: queryProfileFingerprint,
			profile:     profile,
		}})
		var buf bytes.Buffer
		f := &fuzzyFinderCommand{
			cli:    &Cli{SessionHandler: NewSessionHandler(session), SystemVariables: session.systemVariables},
			editor: dummyFuzzyEditor(&buf),
		}
		got, err := f.resolveCandidates(t.Context(), fuzzyCompleteQueryProfile, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].Value != fmt.Sprintf("%d", queryProfileFingerprint) {
			t.Fatalf("candidates = %+v", got)
		}
		if !strings.Contains(got[0].Label, "SELECT * FROM Singers") {
			t.Fatalf("label missing query text: %q", got[0].Label)
		}
		assertQueryProfileSQL(t, server.lastRequest(), "SPANNER_SYS.QUERY_PROFILES_TOP_HOUR", nil)
		if !strings.Contains(buf.String(), "Loading...") {
			t.Fatalf("loading indicator missing: %q", buf.String())
		}
		if cached := f.getCachedCandidates(fuzzyCompleteQueryProfile); cached != nil {
			t.Fatalf("query profiles must stay uncached: %+v", cached)
		}
	})

	t.Run("no rows", func(t *testing.T) {
		t.Parallel()
		session, server := newQueryProfileRPCSession(t, nil)
		f := fuzzyFinderForSession(session)
		got, err := f.fetchQueryProfileCandidates(t.Context())
		if err != nil || len(got) != 0 {
			t.Fatalf("got %v err=%v, want empty", got, err)
		}
		assertQueryProfileSQL(t, server.lastRequest(), "SPANNER_SYS.QUERY_PROFILES_TOP_HOUR", nil)
	})

	t.Run("query error", func(t *testing.T) {
		t.Parallel()
		session, server := newQueryProfileRPCSession(t, nil)
		server.execErr = status.Error(codes.NotFound, "QUERY_PROFILES_TOP_HOUR missing")
		f := fuzzyFinderForSession(session)
		_, err := f.fetchQueryProfileCandidates(t.Context())
		if err == nil || !strings.Contains(err.Error(), "QUERY_PROFILES_TOP_HOUR missing") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("cancelled fetch", func(t *testing.T) {
		t.Parallel()
		session, server := newQueryProfileRPCSession(t, []queryProfileRPCRow{{
			intervalEnd: intervalEnd,
			fingerprint: queryProfileFingerprint,
			profile:     profile,
		}})
		f := fuzzyFinderForSession(session)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err := f.fetchQueryProfileCandidates(ctx)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled fetch: %v", err)
		}
		if server.lastRequest() != nil {
			t.Fatal("cancelled fetch must not query")
		}
	})

	t.Run("active read-write rejects without query", func(t *testing.T) {
		t.Parallel()
		session, server := newQueryProfileRPCSession(t, []queryProfileRPCRow{{
			intervalEnd: intervalEnd,
			fingerprint: queryProfileFingerprint,
			profile:     profile,
		}})
		session.txn.tc = &transactionContext{attrs: transactionAttributes{mode: transactionModeReadWrite}}
		f := fuzzyFinderForSession(session)
		_, err := f.fetchQueryProfileCandidates(t.Context())
		if err == nil || !strings.Contains(err.Error(), "SPANNER_SYS.QUERY_PROFILES_TOP_HOUR") {
			t.Fatalf("RW err=%v", err)
		}
		if server.lastRequest() != nil {
			t.Fatal("active RW must not query QUERY_PROFILES_TOP_HOUR")
		}
	})

	t.Run("missing session", func(t *testing.T) {
		t.Parallel()
		sv := newSystemVariablesWithDefaults()
		f := &fuzzyFinderCommand{
			cli:    &Cli{SystemVariables: &sv, SessionHandler: NewSessionHandler(nil)},
			editor: dummyFuzzyEditor(io.Discard),
		}
		got, err := f.fetchQueryProfileCandidates(t.Context())
		if err != nil || got != nil {
			t.Fatalf("got %v err=%v, want empty", got, err)
		}
	})
}

func TestQueryProfileCompletionEmptyOrCancelledEditor(t *testing.T) {
	t.Parallel()
	for _, cancelled := range []bool{false, true} {
		sv := newSystemVariablesWithDefaults()
		f := &fuzzyFinderCommand{
			cli:    &Cli{SystemVariables: &sv, SessionHandler: NewSessionHandler(nil)},
			editor: dummyFuzzyEditor(io.Discard),
		}
		ctx, cancel := context.WithCancel(t.Context())
		if cancelled {
			cancel()
		}
		b := &readline.Buffer{Editor: &readline.Editor{}}
		input := "SHOW QUERY PROFILE "
		b.Cursor = b.InsertString(0, input)
		result := f.Call(ctx, b)
		cancel()
		if result != readline.CONTINUE || b.String() != input || b.Cursor != len(input) {
			t.Fatalf("cancelled=%v: result=%v buffer=%q cursor=%d", cancelled, result, b.String(), b.Cursor)
		}
	}
}

func TestShowQueryProfilesSharedHelperUnchanged(t *testing.T) {
	t.Parallel()
	intervalEnd := time.Date(2025, 5, 29, 8, 0, 0, 0, time.UTC)
	planJSON := marshalQueryPlanJSON(t, loadTestPlan(t, queryProfileFilterPlanFile))
	profile := queryProfileRawJSON(t, planJSON, map[string]any{
		"elapsed_time": "11.09 msecs",
		"query_text":   "SELECT * FROM Singers",
	}, fmt.Sprint(queryProfileFingerprint))
	session, server := newQueryProfileRPCSession(t, []queryProfileRPCRow{{
		intervalEnd: intervalEnd,
		fingerprint: queryProfileFingerprint,
		latency:     0.01109,
		profile:     profile,
	}})
	got, err := (&ShowQueryProfilesStatement{}).Execute(t.Context(), session, OperationOutput{})
	if err != nil {
		t.Fatal(err)
	}
	if got.AffectedRows != 1 {
		t.Fatalf("AffectedRows = %d", got.AffectedRows)
	}
	text := rowText(got.presentationRows()[0])
	if !strings.Contains(text, "SELECT * FROM Singers") || !strings.Contains(text, "text_fingerprint:             -6422424748333414178") {
		t.Fatalf("SHOW QUERY PROFILES rendering changed:\n%s", text)
	}
	assertQueryProfileSQL(t, server.lastRequest(), queryProfilesTopHourSQL, nil)
}
