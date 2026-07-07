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

// Internal tests for the FeatureState per-session store (issue #778). Internal
// package so a bare Session can be constructed and Close driven directly.

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

// closerRecorder records the order in which Close is called, keyed by name.
type closerRecorder struct {
	name   string
	closed *[]string
}

func (c closerRecorder) Close() error {
	*c.closed = append(*c.closed, c.name)
	return nil
}

func TestFeatureStateLazyOnce(t *testing.T) {
	s := &Session{}
	var calls atomic.Int32

	init := func(ctx context.Context, s *Session) (int, error) {
		calls.Add(1)
		return 42, nil
	}

	for range 3 {
		got, err := FeatureState(context.Background(), s, "k", init)
		if err != nil {
			t.Fatalf("FeatureState returned error: %v", err)
		}
		if got != 42 {
			t.Fatalf("FeatureState = %d, want 42", got)
		}
	}
	if n := calls.Load(); n != 1 {
		t.Fatalf("init called %d times, want exactly 1", n)
	}
}

func TestFeatureStateConcurrentSameKeyInitsOnce(t *testing.T) {
	s := &Session{}
	var calls atomic.Int32

	init := func(ctx context.Context, s *Session) (int, error) {
		calls.Add(1)
		return 7, nil
	}

	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			if _, err := FeatureState(context.Background(), s, "k", init); err != nil {
				t.Errorf("FeatureState returned error: %v", err)
			}
		})
	}
	wg.Wait()
	if n := calls.Load(); n != 1 {
		t.Fatalf("init called %d times under concurrency, want exactly 1", n)
	}
}

func TestFeatureStateRetriesAfterInitError(t *testing.T) {
	// A failed init is not cached: the error is returned and the next call
	// retries. This matches the lazy per-feature fields the store replaces
	// (BigQuery client, CQL session, doc cache) — a transient build failure
	// must not poison the session.
	s := &Session{}
	wantErr := errors.New("boom")
	var calls atomic.Int32
	init := func(ctx context.Context, s *Session) (int, error) {
		if calls.Add(1) == 1 {
			return 0, wantErr
		}
		return 42, nil
	}

	if _, err := FeatureState(context.Background(), s, "k", init); !errors.Is(err, wantErr) {
		t.Fatalf("first FeatureState error = %v, want %v", err, wantErr)
	}
	got, err := FeatureState(context.Background(), s, "k", init)
	if err != nil {
		t.Fatalf("second FeatureState error = %v, want success on retry", err)
	}
	if got != 42 {
		t.Fatalf("second FeatureState = %d, want 42", got)
	}

	// The successful value is cached; init does not run again.
	if _, err := FeatureState(context.Background(), s, "k", init); err != nil {
		t.Fatalf("third FeatureState error = %v", err)
	}
	if n := calls.Load(); n != 2 {
		t.Fatalf("init called %d times, want 2 (error retried, success cached)", n)
	}
}

func TestFeatureStateClosesInReverseOrderOnSessionClose(t *testing.T) {
	s := &Session{}
	var closed []string

	// Create three entries in order a, b, c.
	for _, name := range []string{"a", "b", "c"} {
		if _, err := FeatureState(context.Background(), s, name, func(ctx context.Context, s *Session) (closerRecorder, error) {
			return closerRecorder{name: name, closed: &closed}, nil
		}); err != nil {
			t.Fatalf("FeatureState(%q) error: %v", name, err)
		}
	}

	s.Close()

	want := []string{"c", "b", "a"}
	if len(closed) != len(want) {
		t.Fatalf("closed = %v, want %v", closed, want)
	}
	for i := range want {
		if closed[i] != want[i] {
			t.Fatalf("closed = %v, want %v (reverse creation order)", closed, want)
		}
	}
}

// TestMutationClassifierDelegates verifies the MutationClassifier embed's
// isConditionallyMutating method forwards to Classify. The method is unexported
// so this delegation check must live in the internal package (the external
// feature_seam_test only asserts interface satisfaction at compile time).
func TestMutationClassifierDelegates(t *testing.T) {
	t.Parallel()

	called := false
	m := MutationClassifier{Classify: func() bool { called = true; return true }}
	if !m.isConditionallyMutating() {
		t.Fatal("isConditionallyMutating() = false, want true")
	}
	if !called {
		t.Fatal("Classify was not called")
	}

	m = MutationClassifier{Classify: func() bool { return false }}
	if m.isConditionallyMutating() {
		t.Fatal("isConditionallyMutating() = true, want false")
	}
}

func TestFeatureStateNonCloserValuesSkippedOnClose(t *testing.T) {
	// A value that does not implement io.Closer must not break Close.
	s := &Session{}
	if _, err := FeatureState(context.Background(), s, "plain", func(ctx context.Context, s *Session) (int, error) {
		return 1, nil
	}); err != nil {
		t.Fatalf("FeatureState error: %v", err)
	}
	s.Close() // must not panic
}
