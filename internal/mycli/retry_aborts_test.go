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
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestRetryAbortsInternallyDefaultAndReset(t *testing.T) {
	t.Parallel()
	sv := newSystemVariablesWithDefaultsForTest()
	if err := sv.CaptureStartupSnapshots(); err != nil {
		t.Fatal(err)
	}
	got, err := sv.Get("RETRY_ABORTS_INTERNALLY")
	if err != nil {
		t.Fatal(err)
	}
	if got["RETRY_ABORTS_INTERNALLY"] != "FALSE" {
		t.Fatalf("default SHOW = %v, want FALSE", got)
	}
	if err := sv.SetFromSimple("RETRY_ABORTS_INTERNALLY", "TRUE"); err != nil {
		t.Fatal(err)
	}
	if !sv.Transaction.RetryAbortsInternally {
		t.Fatal("SET TRUE did not update session value")
	}
	if err := sv.Reset("RETRY_ABORTS_INTERNALLY"); err != nil {
		t.Fatal(err)
	}
	if sv.Transaction.RetryAbortsInternally {
		t.Fatal("RESET did not restore FALSE")
	}
}

func TestWrapAbortedIfNeeded(t *testing.T) {
	t.Parallel()
	aborted := status.Error(codes.Aborted, "aborted")
	wrapped := wrapAbortedIfNeeded(aborted)
	if wrapped == nil || !strings.Contains(wrapped.Error(), "transaction was aborted") {
		t.Fatalf("ABORTED wrap = %v", wrapped)
	}
	other := status.Error(codes.InvalidArgument, "bad")
	if got := wrapAbortedIfNeeded(other); got != other {
		t.Fatalf("non-ABORTED wrap = %v", got)
	}
}

func TestAbortRetryDelayUsesRetryInfo(t *testing.T) {
	t.Parallel()
	st, err := status.New(codes.Aborted, "aborted").WithDetails(&errdetails.RetryInfo{
		RetryDelay: durationpb.New(7 * time.Millisecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := abortRetryDelay(st.Err()); got != 7*time.Millisecond {
		t.Fatalf("delay = %s, want 7ms", got)
	}
}

func TestAbortRetryDelayDefaultBounded(t *testing.T) {
	t.Parallel()
	d := abortRetryDelay(status.Error(codes.Aborted, "aborted"))
	if d < time.Millisecond || d > 32*time.Millisecond {
		t.Fatalf("default delay %s outside 1-32ms", d)
	}
}

func TestClampAbortRetryDelay(t *testing.T) {
	t.Parallel()
	if got := clampAbortRetryDelay(20*time.Millisecond, 5*time.Millisecond); got != 5*time.Millisecond {
		t.Fatalf("clamp = %s", got)
	}
	if got := clampAbortRetryDelay(2*time.Millisecond, 5*time.Millisecond); got != 2*time.Millisecond {
		t.Fatalf("unchanged = %s", got)
	}
}

func TestWaitAbortRetryHonorsCancel(t *testing.T) {
	t.Parallel()
	tm := &TransactionManager{}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := tm.waitAbortRetry(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled wait = %v", err)
	}
}

func TestWaitAbortRetrySeam(t *testing.T) {
	t.Parallel()
	var seen time.Duration
	tm := &TransactionManager{
		abortRetryWait: func(ctx context.Context, d time.Duration) error {
			seen = d
			return nil
		},
	}
	if err := tm.waitAbortRetry(t.Context(), 3*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if seen != 3*time.Millisecond {
		t.Fatalf("seam delay = %s", seen)
	}
}

func TestShouldRetryImplicitAbort(t *testing.T) {
	t.Parallel()
	owner := &transactionContext{retryAborts: true}
	aborted := status.Error(codes.Aborted, "aborted")
	if !shouldRetryImplicitAbort(aborted, true, implicitAbortRetryIfEnabled, owner, 1, 50) {
		t.Fatal("expected retry")
	}
	if shouldRetryImplicitAbort(aborted, true, implicitAbortRetryIfEnabled, owner, 50, 50) {
		t.Fatal("50th attempt must not retry")
	}
	if shouldRetryImplicitAbort(aborted, false, implicitAbortRetryIfEnabled, owner, 1, 50) {
		t.Fatal("explicit owner must not retry")
	}
	if shouldRetryImplicitAbort(aborted, true, implicitAbortRetryDisabled, owner, 1, 50) {
		t.Fatal("PLAN/disabled policy must not retry")
	}
	if shouldRetryImplicitAbort(status.Error(codes.Unavailable, "u"), true, implicitAbortRetryIfEnabled, owner, 1, 50) {
		t.Fatal("non-ABORTED must not retry")
	}
}

func TestSnapshotRetryAbortsIgnoresLaterSessionSet(t *testing.T) {
	t.Parallel()
	sv := newSystemVariablesWithDefaultsForTest()
	sv.Transaction.RetryAbortsInternally = true
	tc := &transactionContext{}
	snapshotRetryAbortsLocked(tc, sv)
	sv.Transaction.RetryAbortsInternally = false
	snapshotRetryAbortsLocked(tc, sv)
	if !tc.retryAborts || !tc.retryAbortsCaptured {
		t.Fatalf("snapshot mutated: %+v", tc)
	}
}
