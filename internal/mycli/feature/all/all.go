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

import "github.com/apstndb/spanner-mycli/internal/mycli"

// All returns every optional feature in a fixed, documented order that mirrors
// today's client-side statement table order: GEMINI/LLM, then BIGQUERY, then
// CQL. The order is load-bearing: features are appended to the core statement
// table in this order, and the #728 non-shadowing invariant is proven over the
// resulting merged table.
//
// It is empty in PR0 (the seam is introduced with zero behavior change; the
// three families still live in core). Feature packages will be added under
// internal/mycli/feature/{llm,bigquery,cql} as they are extracted.
func All() []mycli.Feature { return nil }
