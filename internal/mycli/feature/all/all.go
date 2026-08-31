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

// Package all assembles the full set of optional features (issue #778) for the
// full spanner-mycli binary. The slim variant does not import this package and
// registers no features.
package all

import (
	"github.com/apstndb/spanner-mycli/internal/mycli"
	"github.com/apstndb/spanner-mycli/internal/mycli/feature/bigquery"
	"github.com/apstndb/spanner-mycli/internal/mycli/feature/cql"
	"github.com/apstndb/spanner-mycli/internal/mycli/feature/llm"
)

// All returns every optional feature in a fixed, documented order: GEMINI/LLM,
// then CQL, then BIGQUERY. The order is load-bearing: features are appended to
// the core statement table in this order, and the #728 non-shadowing invariant
// is proven over the resulting merged table.
//
// Order-contract note (#778 §7): features append AFTER the core table, so
// extracting a family moves its statement-help row to the end of the merged
// table in All() order. The final order is fixed as (llm, cql, bigquery) rather
// than the pre-extraction core order (llm, bigquery, cql). The single
// BIGQUERY/CQL row swap in the generated statement help landed in PR1 (BIGQUERY
// extraction) alone; CQL re-appended at the same relative position (before
// BIGQUERY), and GEMINI/LLM now re-appends at the head, so the generated
// statement help stays byte-identical.
//
// All three families (GEMINI/LLM, CQL, BIGQUERY) are now extracted under
// internal/mycli/feature/{llm,cql,bigquery}.
func All() []mycli.Feature {
	return []mycli.Feature{
		llm.Feature(),
		cql.Feature(),
		bigquery.Feature(),
	}
}
