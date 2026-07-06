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
	"strings"
	"testing"
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
