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

// Test-only bridge exposing a few unexported internals to the external
// mycli_test package (issue #778). This keeps the production API surface small
// while letting external tests — e.g. the feature/bigquery dispatch-level
// READONLY guard test, which must live outside package mycli to avoid an import
// cycle with feature packages — drive a real Session through the guard.

import (
	"errors"
	"testing"
)

// NewReadOnlySessionForTest builds a READONLY, database-connected Session wired
// for the pending-transaction lifecycle (no RPCs), for external-package dispatch
// tests.
func NewReadOnlySessionForTest(t *testing.T) *Session {
	t.Helper()
	s := newSessionForLocalVarTest(t)
	s.systemVariables.Transaction.ReadOnly = true
	return s
}

// IsReadOnlyError reports whether err is the READONLY guard sentinel. The
// sentinel stays unexported in production; this bridge lets external tests
// assert on it.
func IsReadOnlyError(err error) bool { return errors.Is(err, errReadOnly) }
