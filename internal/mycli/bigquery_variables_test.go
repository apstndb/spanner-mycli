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

package mycli_test

// Regression for the #781 review finding: after the BIGQUERY extraction, the
// feature's variables must still appear in the variable enumeration that backs
// SHOW VARIABLES and fuzzy variable-name completion (ListVariables), not only
// in HELP VARIABLES / generated docs (ListVariableInfo). Exercised with the
// real bigquery feature registered.

import (
	"testing"

	"github.com/apstndb/spanner-mycli/internal/mycli"
	"github.com/apstndb/spanner-mycli/internal/mycli/feature/bigquery"
)

func TestVariableListingIncludesBigQueryVars(t *testing.T) {
	t.Parallel()

	session := mycli.NewSessionWithFeaturesForTest(t, bigquery.Feature())

	listed := mycli.ListVariablesForTest(session)
	for _, name := range []string{
		"CLI_BIGQUERY_PROJECT",
		"CLI_BIGQUERY_LOCATION",
		"CLI_BIGQUERY_MAX_BYTES_BILLED",
	} {
		if _, ok := listed[name]; !ok {
			t.Errorf("variable listing (SHOW VARIABLES / completion source) does not include %s", name)
		}
	}
}
