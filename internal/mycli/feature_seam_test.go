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

// External-package tests for the Feature seam (issue #778): they prove that the
// exported marker embeds grant cross-package satisfaction of the core marker
// interfaces (whose methods stay unexported, #696), and that a feature's
// statement dispatches through the merged table without shadowing.

import (
	"context"
	"regexp"
	"testing"

	"github.com/apstndb/spanner-mycli/internal/mycli"
)

// The marker interfaces' methods are unexported, so a type in this external
// package can only satisfy them by embedding the exported marker types. These
// compile-time assertions fail to build if the embeds stop promoting the
// unexported marker methods across the package boundary.
type embedMutation struct{ mycli.MarksMutation }

type embedDetached struct{ mycli.MarksDetachedCompatible }

type embedClassifier struct{ mycli.MutationClassifier }

var (
	_ mycli.MutationStatement              = embedMutation{}
	_ mycli.DetachedCompatible             = embedDetached{}
	_ mycli.ConditionallyMutatingStatement = embedClassifier{}
	_ mycli.ConditionallyMutatingStatement = (*embedClassifier)(nil)
)

// featureStatement is a minimal Statement contributed by a test Feature.
type featureStatement struct {
	mycli.MarksDetachedCompatible
}

func (*featureStatement) Execute(ctx context.Context, s *mycli.Session) (*mycli.Result, error) {
	return nil, nil
}

func newTestFeature() mycli.Feature {
	return mycli.Feature{
		Name: "TESTFEAT",
		StatementDefs: []*mycli.StatementDef{
			{
				Descriptions: []mycli.StatementDescription{
					{Usage: "test feature statement", Syntax: "TESTFEATURE"},
				},
				Pattern: regexp.MustCompile(`(?is)^TESTFEATURE$`),
				HandleGroups: func(map[string]string) (mycli.Statement, error) {
					return &featureStatement{}, nil
				},
			},
		},
	}
}

// TestFeatureStatementDispatchAfterMerge proves a feature's single StatementDef
// dispatches to that feature (via the same primitive BuildCLIStatement wraps)
// once merged, that it is appended after the core table, and that its keyword
// neither shadows nor is shadowed by any core def.
func TestFeatureStatementDispatchAfterMerge(t *testing.T) {
	t.Parallel()

	feat := newTestFeature()
	core := mycli.MergedStatementDefs()
	defs := mycli.MergedStatementDefs(feat)

	// The feature def is appended after every core def.
	if len(defs) != len(core)+1 {
		t.Fatalf("merged table has %d defs, want core(%d)+1", len(defs), len(core))
	}
	if defs[len(defs)-1] != feat.StatementDefs[0] {
		t.Fatalf("feature def is not appended last")
	}

	const input = "TESTFEATURE"

	// Not shadowed: no core def matches the feature's keyword, so dispatch
	// reaches the feature def.
	for i, def := range core {
		if def.Pattern.MatchString(input) {
			t.Fatalf("feature keyword %q is shadowed by core def %d (%q)", input, i, def.Pattern)
		}
	}

	stmt, err := mycli.BuildStatementWithDefs(defs, input)
	if err != nil {
		t.Fatalf("BuildStatementWithDefs(%q) error: %v", input, err)
	}
	if _, ok := stmt.(*featureStatement); !ok {
		t.Fatalf("dispatch returned %T, want *featureStatement", stmt)
	}

	// Does not shadow: the feature Pattern must not capture a representative
	// core statement shape.
	if feat.StatementDefs[0].Pattern.MatchString("SELECT 1") {
		t.Fatalf("feature pattern shadows a core statement shape")
	}
}
