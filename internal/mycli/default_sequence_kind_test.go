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

	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/apstndb/spanner-mycli/enums"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const missingKindSentence = "The sequence kind of an identity column id is not specified. Please specify the sequence kind explicitly or set the database option `default_sequence_kind`."

func TestIsMissingDefaultSequenceKindError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "backtick sentence", err: status.Error(codes.InvalidArgument, missingKindSentence), want: true},
		{
			name: "unquoted sentence",
			err:  status.Error(codes.InvalidArgument, "The sequence kind of an identity column id is not specified. Please specify the sequence kind explicitly or set the database option default_sequence_kind."),
			want: true,
		},
		{name: "syntax InvalidArgument", err: status.Error(codes.InvalidArgument, "syntax error near CREATE"), want: false},
		{name: "option name only", err: status.Error(codes.InvalidArgument, "cannot change default_sequence_kind after tables exist"), want: false},
		{name: "FailedPrecondition same sentence", err: status.Error(codes.FailedPrecondition, missingKindSentence), want: false},
		{name: "Canceled same sentence", err: status.Error(codes.Canceled, missingKindSentence), want: false},
		{name: "DeadlineExceeded", err: status.Error(codes.DeadlineExceeded, missingKindSentence), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isMissingDefaultSequenceKindError(tt.err); got != tt.want {
				t.Fatalf("got %v want %v for %v", got, tt.want, tt.err)
			}
		})
	}
}

func TestShouldAttemptSequenceKindRepair(t *testing.T) {
	t.Parallel()
	err := status.Error(codes.InvalidArgument, missingKindSentence)
	if !shouldAttemptSequenceKindRepair(t.Context(), enums.DDLExecutionModeSync, defaultSequenceKindValue, err) {
		t.Fatal("SYNC + kind + classifier should attempt")
	}
	if shouldAttemptSequenceKindRepair(t.Context(), enums.DDLExecutionModeAsync, defaultSequenceKindValue, err) {
		t.Fatal("ASYNC must not attempt")
	}
	if shouldAttemptSequenceKindRepair(t.Context(), enums.DDLExecutionModeAsyncWait, defaultSequenceKindValue, err) {
		t.Fatal("ASYNC_WAIT must not attempt")
	}
	if shouldAttemptSequenceKindRepair(t.Context(), enums.DDLExecutionModeSync, "", err) {
		t.Fatal("empty kind must not attempt")
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if shouldAttemptSequenceKindRepair(canceled, enums.DDLExecutionModeSync, defaultSequenceKindValue, err) {
		t.Fatal("canceled caller context must not attempt")
	}
}

func TestProvenSuccessfulPrefix(t *testing.T) {
	t.Parallel()
	db := "projects/p/instances/i/databases/db"
	s1, s2, s3 := "CREATE TABLE a", "CREATE TABLE b", "CREATE TABLE c"
	ts := timestamppb.New(time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC))

	tests := []struct {
		name      string
		md        *databasepb.UpdateDatabaseDdlMetadata
		submitted []string
		want      int
		ok        bool
	}{
		{name: "nil metadata", submitted: []string{s1}, ok: false},
		{
			name:      "database mismatch",
			submitted: []string{s1},
			md:        &databasepb.UpdateDatabaseDdlMetadata{Database: "projects/p/instances/i/databases/other", Statements: []string{s1}},
		},
		{
			name:      "statement mismatch",
			submitted: []string{s1},
			md:        &databasepb.UpdateDatabaseDdlMetadata{Database: db, Statements: []string{s2}},
		},
		{
			name:      "empty timestamps is prefix0",
			submitted: []string{s1, s2},
			md:        &databasepb.UpdateDatabaseDdlMetadata{Database: db, Statements: []string{s1, s2}},
			ok:        true,
		},
		{
			name:      "contiguous prefix",
			submitted: []string{s1, s2, s3},
			md:        &databasepb.UpdateDatabaseDdlMetadata{Database: db, Statements: []string{s1, s2, s3}, CommitTimestamps: []*timestamppb.Timestamp{ts}},
			want:      1,
			ok:        true,
		},
		{
			name:      "trailing padded empty tail",
			submitted: []string{s1, s2},
			md:        &databasepb.UpdateDatabaseDdlMetadata{Database: db, Statements: []string{s1, s2}, CommitTimestamps: []*timestamppb.Timestamp{ts, {}}},
			want:      1,
			ok:        true,
		},
		{
			name:      "hole then success",
			submitted: []string{s1, s2, s3},
			md:        &databasepb.UpdateDatabaseDdlMetadata{Database: db, Statements: []string{s1, s2, s3}, CommitTimestamps: []*timestamppb.Timestamp{ts, {}, ts}},
		},
		{
			name:      "too many timestamps",
			submitted: []string{s1},
			md:        &databasepb.UpdateDatabaseDdlMetadata{Database: db, Statements: []string{s1}, CommitTimestamps: []*timestamppb.Timestamp{ts, ts}},
		},
		{
			name:      "covers every statement",
			submitted: []string{s1, s2},
			md:        &databasepb.UpdateDatabaseDdlMetadata{Database: db, Statements: []string{s1, s2}, CommitTimestamps: []*timestamppb.Timestamp{ts, ts}},
		},
		{
			name:      "invalid timestamp",
			submitted: []string{s1, s2},
			md:        &databasepb.UpdateDatabaseDdlMetadata{Database: db, Statements: []string{s1, s2}, CommitTimestamps: []*timestamppb.Timestamp{{Seconds: ts.GetSeconds(), Nanos: 2_000_000_000}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := provenSuccessfulPrefix(db, tt.submitted, tt.md)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("got (%d,%v) want (%d,%v)", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestDefaultSequenceKindAlterSQL(t *testing.T) {
	t.Parallel()
	got, err := defaultSequenceKindAlterSQL(databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL, "my-db", defaultSequenceKindValue)
	if err != nil {
		t.Fatal(err)
	}
	want := "ALTER DATABASE `my-db` SET OPTIONS (default_sequence_kind = 'bit_reversed_positive')"
	if got != want {
		t.Fatalf("GoogleSQL = %q want %q", got, want)
	}
	got, err = defaultSequenceKindAlterSQL(databasepb.DatabaseDialect_POSTGRESQL, "my-db", defaultSequenceKindValue)
	if err != nil {
		t.Fatal(err)
	}
	want = `ALTER DATABASE "my-db" SET spanner.default_sequence_kind = 'bit_reversed_positive'`
	if got != want {
		t.Fatalf("PostgreSQL = %q want %q", got, want)
	}
	if _, err := defaultSequenceKindAlterSQL(databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL, "Bad_ID", defaultSequenceKindValue); err == nil {
		t.Fatal("expected unsafe database id")
	}
	if _, err := defaultSequenceKindAlterSQL(databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL, "my-db", "other"); err == nil {
		t.Fatal("expected unsafe kind")
	}
}

func TestDefaultSequenceKindVar(t *testing.T) {
	t.Parallel()
	sv := newSystemVariablesWithDefaultsForTest()
	sv.ensureRegistry()
	got, err := sv.Registry.Get(defaultSequenceKindVarName)
	if err != nil {
		t.Fatal(err)
	}
	if got != "NULL" {
		t.Fatalf("default Get = %q want NULL", got)
	}
	if err := sv.SetFromGoogleSQL(defaultSequenceKindVarName, "'bit_reversed_positive'"); err != nil {
		t.Fatal(err)
	}
	if sv.Feature.DefaultSequenceKind != defaultSequenceKindValue {
		t.Fatalf("set kind = %q", sv.Feature.DefaultSequenceKind)
	}
	if err := sv.SetFromGoogleSQL(defaultSequenceKindVarName, "NULL"); err != nil {
		t.Fatal(err)
	}
	if sv.Feature.DefaultSequenceKind != "" {
		t.Fatalf("NULL did not clear, got %q", sv.Feature.DefaultSequenceKind)
	}
	if err := sv.SetFromSimple(defaultSequenceKindVarName, "bit_reversed_negative"); err == nil {
		t.Fatal("expected reject of other kind")
	}
	if sv.Feature.DefaultSequenceKind != "" {
		t.Fatal("rejected SET mutated state")
	}
}

func TestDefaultSequenceKindSetLocal(t *testing.T) {
	t.Parallel()
	session := newSessionForLocalVarTest(t)
	ctx := t.Context()
	if _, err := session.ExecuteStatement(ctx, &BeginStatement{}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetStatement{VarName: defaultSequenceKindVarName, Value: "'bit_reversed_positive'"}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecuteStatement(ctx, &SetLocalStatement{VarName: defaultSequenceKindVarName, Value: "NULL"}); err != nil {
		t.Fatal(err)
	}
	if got := mustGetVar(t, session, defaultSequenceKindVarName); got != "NULL" {
		t.Fatalf("SET LOCAL Get = %q want NULL", got)
	}
	if _, err := session.ExecuteStatement(ctx, &CommitStatement{}); err != nil {
		t.Fatal(err)
	}
	if got := mustGetVar(t, session, defaultSequenceKindVarName); got != defaultSequenceKindValue {
		t.Fatalf("after COMMIT Get = %q want %s", got, defaultSequenceKindValue)
	}
}

func TestEchoExecutedDDLRowsSkipsEmpty(t *testing.T) {
	t.Parallel()
	ts := timestamppb.New(time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC))
	rows := echoExecutedDDLRows([]string{"s1", "s2"}, []*timestamppb.Timestamp{ts, {}})
	if len(rows) != 1 || !strings.Contains(rows[0][0].RawText(), "s1") {
		t.Fatalf("rows=%v", rows)
	}
}
