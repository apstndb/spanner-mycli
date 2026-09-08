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

// spanner-mycli-slim excludes the optional GEMINI/LLM, BIGQUERY, and CQL
// statement families. It is a separate main package rather than a build-tagged
// variant, so normal Go tooling always compiles both release graphs.
package main

import "github.com/apstndb/spanner-mycli/internal/mycli"

var (
	// version and installFrom are set via ldflags at build time.
	version     = ""
	installFrom = "built from source"
)

func main() { mycli.Main(version, installFrom, registeredFeatures()...) }

func registeredFeatures() []mycli.Feature { return nil }
