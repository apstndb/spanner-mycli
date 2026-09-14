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
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/apstndb/spanner-mycli/internal/mycli/streamio"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestDdlCancellationError verifies that a canceled DDL wait produces an error that surfaces
// the still-running operation ID, points the user at SHOW OPERATION, and preserves the
// underlying context cause so errors.Is keeps working for callers.
func TestDdlCancellationError(t *testing.T) {
	tests := []struct {
		name      string
		opName    string
		cause     error
		wantOpID  string
		wantIsErr error
	}{
		{
			name:      "full operation name",
			opName:    "projects/p/instances/i/databases/d/operations/1234567890",
			cause:     context.Canceled,
			wantOpID:  "1234567890",
			wantIsErr: context.Canceled,
		},
		{
			name:      "bare operation id",
			opName:    "op-abc",
			cause:     context.Canceled,
			wantOpID:  "op-abc",
			wantIsErr: context.Canceled,
		},
		{
			name:      "deadline exceeded cause",
			opName:    "projects/p/instances/i/databases/d/operations/op-xyz",
			cause:     context.DeadlineExceeded,
			wantOpID:  "op-xyz",
			wantIsErr: context.DeadlineExceeded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ddlCancellationError(tt.opName, tt.cause)
			if err == nil {
				t.Fatal("expected non-nil error")
			}

			if !errors.Is(err, tt.wantIsErr) {
				t.Errorf("errors.Is(err, %v) = false, want true; err = %v", tt.wantIsErr, err)
			}

			msg := err.Error()
			if !strings.Contains(msg, "SHOW OPERATION '"+tt.wantOpID+"'") {
				t.Errorf("error message does not contain SHOW OPERATION hint for %q\n  got: %s", tt.wantOpID, msg)
			}
		})
	}
}

// TestIsCancellationError verifies that the classification that drives the SHOW OPERATION hint
// recognizes cancellation both from the standard-library context sentinels and from plain gRPC /
// Spanner status errors. The status-error cases matter because a context cancelled while a
// GetOperation poll RPC is in flight surfaces as a bare status error that does NOT wrap
// context.Canceled, so errors.Is alone would miss it.
func TestIsCancellationError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "context canceled sentinel", err: context.Canceled, want: true},
		{name: "context deadline exceeded sentinel", err: context.DeadlineExceeded, want: true},
		{name: "wrapped context canceled", err: fmt.Errorf("poll failed: %w", context.Canceled), want: true},
		{name: "grpc status canceled (in-flight RPC)", err: status.Error(codes.Canceled, "context canceled"), want: true},
		{name: "grpc status deadline exceeded", err: status.Error(codes.DeadlineExceeded, "context deadline exceeded"), want: true},
		{name: "wrapped grpc status canceled", err: fmt.Errorf("poll failed: %w", status.Error(codes.Canceled, "context canceled")), want: true},
		{name: "genuine DDL failure", err: status.Error(codes.InvalidArgument, "bad DDL"), want: false},
		{name: "plain non-status error", err: errors.New("boom"), want: false},
		{name: "nil error", err: nil, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isCancellationError(tt.err); got != tt.want {
				t.Errorf("isCancellationError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestBufferOrExecuteDdlStatements(t *testing.T) {
	t.Parallel()

	t.Run("rejects active batch DML", func(t *testing.T) {
		t.Parallel()
		session := newSessionForLocalVarTest(t)
		session.batch.SetCurrent(&BatchDMLStatement{})
		_, err := bufferOrExecuteDdlStatements(t.Context(), session, []string{"CREATE TABLE t (id INT64) PRIMARY KEY (id)"})
		if err == nil || !strings.Contains(err.Error(), "active batch DML") {
			t.Fatalf("error = %v, want active batch DML", err)
		}
	})

	t.Run("buffers into active bulk DDL", func(t *testing.T) {
		t.Parallel()
		session := newSessionForLocalVarTest(t)
		bulk := &BulkDdlStatement{Ddls: []string{"CREATE TABLE t1 (id INT64) PRIMARY KEY (id)"}}
		session.batch.SetCurrent(bulk)
		got, err := bufferOrExecuteDdlStatements(t.Context(), session, []string{"CREATE TABLE t2 (id INT64) PRIMARY KEY (id)"})
		if err != nil {
			t.Fatalf("bufferOrExecuteDdlStatements() error = %v", err)
		}
		if got == nil || got.KeepVariables {
			t.Fatalf("result = %+v, want empty Result", got)
		}
		want := []string{
			"CREATE TABLE t1 (id INT64) PRIMARY KEY (id)",
			"CREATE TABLE t2 (id INT64) PRIMARY KEY (id)",
		}
		if diff := strings.Join(bulk.Ddls, "\n"); diff != strings.Join(want, "\n") {
			t.Fatalf("buffered DDLs = %v, want %v", bulk.Ddls, want)
		}
		current, ok := session.batch.Current().(*BulkDdlStatement)
		if !ok || current != bulk {
			t.Fatalf("batch.Current() = %T, want original *BulkDdlStatement", session.batch.Current())
		}
	})

	t.Run("rejects queued automatic DML", func(t *testing.T) {
		t.Parallel()
		session := newSessionForLocalVarTest(t)
		session.txn.tc = &transactionContext{
			autoDML: []automaticDMLEntry{{stmt: spanner.Statement{SQL: "INSERT INTO t (id) VALUES (1)"}, expected: 1}},
		}
		_, err := bufferOrExecuteDdlStatements(t.Context(), session, []string{"CREATE TABLE t (id INT64) PRIMARY KEY (id)"})
		if err == nil || !strings.Contains(err.Error(), "active batch DML") {
			t.Fatalf("error = %v, want active batch DML", err)
		}
	})
}

func TestExecuteDdlStatementsEmpty(t *testing.T) {
	t.Parallel()

	t.Run("no echo header", func(t *testing.T) {
		t.Parallel()
		session := newSessionForLocalVarTest(t)
		got, err := executeDdlStatements(t.Context(), session, nil)
		if err != nil {
			t.Fatalf("executeDdlStatements() error = %v", err)
		}
		if got.TableHeader != nil {
			t.Fatalf("TableHeader = %v, want nil", got.TableHeader)
		}
	})

	t.Run("echo header without rows", func(t *testing.T) {
		t.Parallel()
		session := newSessionForLocalVarTest(t)
		session.systemVariables.Feature.EchoExecutedDDL = true
		got, err := executeDdlStatements(t.Context(), session, nil)
		if err != nil {
			t.Fatalf("executeDdlStatements() error = %v", err)
		}
		want := toTableHeader("Executed", "Commit Timestamp")
		if diff := cmp.Diff(want, got.TableHeader); diff != "" {
			t.Fatalf("TableHeader mismatch (-want +got):\n%s", diff)
		}
		if len(got.presentationRows()) != 0 {
			t.Fatalf("Rows = %v, want empty", got.presentationRows())
		}
	})
}

func TestWaitDeadlineReached(t *testing.T) {
	t.Parallel()
	if waitDeadlineReached(time.Time{}) {
		t.Fatal("zero deadline is SYNC (no budget), not reached")
	}
	if !waitDeadlineReached(time.Now().Add(-time.Millisecond)) {
		t.Fatal("past deadline should be reached")
	}
	if waitDeadlineReached(time.Now().Add(time.Hour)) {
		t.Fatal("future deadline should not be reached")
	}
}

func TestAsyncWaitDeadline(t *testing.T) {
	t.Parallel()
	if !waitDeadlineReached(asyncWaitDeadline(0)) {
		t.Fatal("zero timeout should be an already-reached deadline")
	}
	if !waitDeadlineReached(asyncWaitDeadline(-time.Second)) {
		t.Fatal("negative timeout should be an already-reached deadline")
	}
	if waitDeadlineReached(asyncWaitDeadline(time.Hour)) {
		t.Fatal("positive timeout should still have remaining budget")
	}
}

func TestClassifyDdlWaitError(t *testing.T) {
	t.Parallel()

	t.Run("caller deadline wins over expired budget", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		got := classifyDdlWaitError(ctx, time.Now().Add(-time.Millisecond), context.DeadlineExceeded)
		if errors.Is(got, errWaitBudgetExpired) {
			t.Fatal("caller cancellation must take precedence over wait-budget expiry")
		}
		if !errors.Is(got, context.DeadlineExceeded) {
			t.Fatalf("got %v, want caller DeadlineExceeded", got)
		}
	})

	t.Run("budget expiry during in-flight cancel", func(t *testing.T) {
		t.Parallel()
		got := classifyDdlWaitError(t.Context(), time.Now().Add(-time.Millisecond), status.Error(codes.DeadlineExceeded, "context deadline exceeded"))
		if !errors.Is(got, errWaitBudgetExpired) {
			t.Fatalf("got %v, want wait-budget handoff", got)
		}
	})

	t.Run("completed failing LRO stays a failure", func(t *testing.T) {
		t.Parallel()
		err := status.Error(codes.FailedPrecondition, "index already exists")
		got := classifyDdlWaitError(t.Context(), time.Now().Add(-time.Millisecond), err)
		if errors.Is(got, errWaitBudgetExpired) {
			t.Fatal("completed LRO failure must not become a wait-budget handoff")
		}
		if status.Code(got) != codes.FailedPrecondition {
			t.Fatalf("got %v, want FailedPrecondition", got)
		}
	})
}

func TestNewProgressWithTTY(t *testing.T) {
	t.Parallel()

	if p := newProgressWithTTY(t.Context(), nil); p != nil {
		t.Fatal("nil session: got progress, want nil")
	}
	if p := newProgressWithTTY(t.Context(), &Session{}); p != nil {
		t.Fatal("nil systemVariables: got progress, want nil")
	}

	session := newSessionForLocalVarTest(t)
	if p := newProgressWithTTY(t.Context(), session); p != nil {
		t.Fatal("nil StreamManager: got progress, want nil")
	}

	session.systemVariables.StreamManager = streamio.NewStreamManager(io.NopCloser(strings.NewReader("")), io.Discard, io.Discard)
	if p := newProgressWithTTY(t.Context(), session); p != nil {
		t.Fatal("non-TTY output: got progress, want nil")
	}

	tty, err := os.CreateTemp(t.TempDir(), "ddl-progress-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tty.Close() })
	session.systemVariables.StreamManager.SetTtyStream(tty)
	p := newProgressWithTTY(t.Context(), session)
	if p == nil {
		t.Fatal("TTY stream: got nil progress")
	}
	p.Wait()
}
