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
	"strings"
	"testing"

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
