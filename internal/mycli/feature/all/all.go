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
)

// All returns every optional feature in a fixed, documented order: GEMINI/LLM,
// then CQL, then BIGQUERY. The order is load-bearing: features are appended to
// the core statement table in this order, and the #728 non-shadowing invariant
// is proven over the resulting merged table.
//
// Order-contract note (#778 §7): features append AFTER the core table, so
// extracting a family moves its statement-help row to the end of the merged
// table in All() order. The final order is fixed as (llm, cql, bigquery) rather
// than the pre-extraction core order (llm, bigquery, cql). This makes the single
// BIGQUERY/CQL row swap in the generated statement help land in PR1 (BIGQUERY
// extraction) alone; PR2 (CQL) and PR3 (LLM) then re-append at the same relative
// positions and stay byte-identical.
//
// Only BIGQUERY is extracted so far; LLM and CQL still live in core and will be
// added here (before BIGQUERY, preserving the llm, cql, bigquery order) as they
// are extracted under internal/mycli/feature/{llm,cql}.
func All() []mycli.Feature {
	return []mycli.Feature{
		bigquery.Feature(),
	}
}
