// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package decoder

import (
	"testing"

	"github.com/apstndb/spanvalue/gcvctor"
	"github.com/google/go-cmp/cmp"
)

// TestJSONFormatConfig_SmokeTest verifies delegation to spanvalue.JSONFormatConfig().
// Comprehensive type coverage is in spanvalue's own tests.
func TestJSONFormatConfig_SmokeTest(t *testing.T) {
	t.Parallel()

	fc := JSONFormatConfig()

	got, err := fc.FormatToplevelColumn(gcvctor.Int64Value(42))
	if err != nil {
		t.Fatalf("FormatToplevelColumn() error = %v", err)
	}

	want := "42"
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("output mismatch (-want +got):\n%s", diff)
	}
}
