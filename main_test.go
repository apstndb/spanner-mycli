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

package main

import (
	"testing"

	"github.com/apstndb/spanner-mycli/internal/mycli"
)

func TestFullBinaryRegistersOptionalStatements(t *testing.T) {
	t.Parallel()

	features := registeredFeatures()
	if got, want := len(features), 3; got != want {
		t.Fatalf("len(registeredFeatures()) = %d, want %d", got, want)
	}

	defs := mycli.MergedStatementDefs(features...)
	for _, input := range []string{
		`GEMINI "show all tables"`,
		"CQL SELECT * FROM users",
		"BIGQUERY SELECT 1",
	} {
		if _, err := mycli.BuildStatementWithDefs(defs, input); err != nil {
			t.Errorf("BuildStatementWithDefs(%q) error = %v", input, err)
		}
	}
}
