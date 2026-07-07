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

package mycli_test

// Registry invariant tests for the merged client-side statement table.
//
// These live in the external test package (issue #778) and iterate
// MergedStatementDefs() so the #728 invariants hold over the same merged table
// (core + features) that every consumer dispatches against. In PR0 the feature
// set is empty, so the table is identical to the core table.
//
// Dispatch over the defs is first-match-wins, so the table order is
// load-bearing (e.g. BEGIN RW must precede BEGIN, SET LOCAL must precede the
// generic SET). Nothing in the production code asserts this, so these tests
// derive canonical example statements from each Description.Syntax and check:
//
//  1. Non-shadowing: every example matches its own def's Pattern and no
//     earlier def's Pattern (TestClientSideStatementDefsNonShadowing).
//  2. Completion consistency: every fuzzyArgCompletion PrefixPattern, when
//     its accepted prefix is completed with a plausible candidate (plus
//     suffix and, where the statement needs more input, a continuation),
//     produces a statement that dispatches to the intended def and parses
//     without error (TestClientSideStatementDefsCompletionConsistency).

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/apstndb/spanner-mycli/internal/mycli"
)

// mergedDefs is the merged (core + feature) client-side statement table the
// invariants run over. With no features it is a copy of the core table.
var mergedDefs = mycli.MergedStatementDefs()

// syntaxPlaceholderValues maps <placeholder> tokens appearing in
// Description.Syntax strings to plausible example values. The expander fails
// the test on an unknown placeholder, so this map must be extended alongside
// new statement definitions.
var syntaxPlaceholderValues = map[string]string{
	"<database>":             "mydb",
	"<role>":                 "myrole",
	"<type>":                 "TABLE",
	"<fqn>":                  "myobject",
	"<table_fqn>":            "mytable",
	"<table1>":               "t1",
	"<table2>":               "t2",
	"<schema>":               "myschema",
	"<sql>":                  "SELECT 1",
	"<seconds>":              "10",
	"<rfc3339_timestamp>":    "'2026-01-01T00:00:00Z'",
	"<timestamp>":            "'2026-01-01T00:00:00Z'",
	"<key>":                  "1",
	"<operation-id-or-name>": "operation123",
	"<node_id>":              "1",
	"<fingerprint>":          "1234567890",
	"<format>":               "CURRENT",
	"<width>":                "80",
	"<preset-or-sections>":   "ALL",
	"<name>":                 "myvar",
	"<value>":                "1",
	"<prompt>":               "prompt text",
}

var placeholderRe = regexp.MustCompile(`<[^<>]+>`)

// expandSyntax derives an example statement from a Description.Syntax string.
// It is deliberately dumb: `[...]` optional segments are dropped
// (keepOptional=false) or kept (keepOptional=true), `{A|B|...}` and bracketed
// alternations pick the first alternative, `<placeholder>` tokens are
// replaced via syntaxPlaceholderValues, and `...` repetition markers become a
// dummy token.
func expandSyntax(t *testing.T, syntax string, keepOptional bool) string {
	t.Helper()

	s := expandSyntaxGroups(t, syntax, keepOptional)
	s = placeholderRe.ReplaceAllStringFunc(s, func(ph string) string {
		v, ok := syntaxPlaceholderValues[ph]
		if !ok {
			t.Fatalf("expandSyntax: unknown placeholder %q in syntax %q; add it to syntaxPlaceholderValues", ph, syntax)
		}
		return v
	})
	// Repetition markers just need some token; the invariant is about
	// dispatch, not semantic validity of the repeated part.
	s = strings.ReplaceAll(s, "...", "1")
	return strings.Join(strings.Fields(s), " ")
}

// expandSyntaxGroups resolves `[...]` and `{...}` groups in a syntax string.
func expandSyntaxGroups(t *testing.T, s string, keepOptional bool) string {
	t.Helper()

	var b strings.Builder
	for i := 0; i < len(s); {
		switch s[i] {
		case '[':
			content, next := matchingGroup(t, s, i, '[', ']')
			if keepOptional {
				b.WriteString(firstAlternative(expandSyntaxGroups(t, content, keepOptional)))
			}
			i = next
		case '{':
			content, next := matchingGroup(t, s, i, '{', '}')
			b.WriteString(firstAlternative(expandSyntaxGroups(t, content, keepOptional)))
			i = next
		default:
			b.WriteByte(s[i])
			i++
		}
	}
	return b.String()
}

// matchingGroup returns the content between the opening delimiter at s[open]
// and its matching closing delimiter, plus the index just past the closer.
func matchingGroup(t *testing.T, s string, open int, openDelim, closeDelim byte) (string, int) {
	t.Helper()

	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case openDelim:
			depth++
		case closeDelim:
			depth--
			if depth == 0 {
				return s[open+1 : i], i + 1
			}
		}
	}
	t.Fatalf("unbalanced %q in syntax fragment %q", string(openDelim), s)
	return "", 0
}

// firstAlternative picks the first `|`-separated alternative. Nested groups
// have already been resolved by the caller, so any remaining `|` belongs to
// this level (e.g. "[ASYNC|SYNC]" or "{READ ONLY|READ WRITE}").
func firstAlternative(s string) string {
	alt, _, _ := strings.Cut(s, "|")
	return alt
}

// firstMatchingDefIndex mirrors BuildCLIStatement's first-match dispatch and
// returns the index of the first def whose Pattern matches, or -1.
func firstMatchingDefIndex(stmt string) int {
	for i, def := range mergedDefs {
		if def.Pattern.MatchString(stmt) {
			return i
		}
	}
	return -1
}

// defIndexBySyntax returns the index of the def that has a description with
// the given Syntax.
func defIndexBySyntax(t *testing.T, syntax string) int {
	t.Helper()
	for i, def := range mergedDefs {
		for _, desc := range def.Descriptions {
			if desc.Syntax == syntax {
				return i
			}
		}
	}
	t.Fatalf("no clientSideStatementDef with syntax %q", syntax)
	return -1
}

// TestClientSideStatementDefsNonShadowing asserts, for the canonical example
// derived from every Description.Syntax, that the example matches its own
// def's Pattern and no earlier def's Pattern. This pins the first-match-wins
// ordering: a new or reordered def that captures an existing statement shape
// fails here instead of silently changing dispatch.
func TestClientSideStatementDefsNonShadowing(t *testing.T) {
	t.Parallel()

	for i, def := range mergedDefs {
		for _, desc := range def.Descriptions {
			for _, keepOptional := range []bool{false, true} {
				t.Run(fmt.Sprintf("def%02d/%s/keepOptional=%v", i, desc.Syntax, keepOptional), func(t *testing.T) {
					example := expandSyntax(t, desc.Syntax, keepOptional)

					if !def.Pattern.MatchString(example) {
						t.Fatalf("example %q derived from syntax %q does not match its own pattern %q", example, desc.Syntax, def.Pattern)
					}
					for j := range i {
						if mergedDefs[j].Pattern.MatchString(example) {
							t.Errorf("example %q derived from syntax %q is shadowed by earlier def %d (syntax %q, pattern %q)",
								example, desc.Syntax, j, firstSyntax(mergedDefs[j]), mergedDefs[j].Pattern)
						}
					}
				})
			}
		}
	}
}

func firstSyntax(def *mycli.StatementDef) string {
	if len(def.Descriptions) > 0 {
		return def.Descriptions[0].Syntax
	}
	return "<no description>"
}

// completionSampleValues provides one plausible candidate per completion type
// (as the fuzzy finder would insert it), keyed by the completion type's
// String() name. The external test package cannot name the unexported
// fuzzyCompletionType, so it keys by the exported String() representation
// instead. The set_target type is handled by an explicit override because its
// candidate list mixes insertion shapes.
var completionSampleValues = map[string]string{
	"database":       "mydb",
	"variable":       "CLI_FORMAT",
	"table":          "mytable",
	"variable_value": "'TABLE'",
	"role":           "myrole",
	"operation":      "operation123",
	"view":           "myview",
	"index":          "myindex",
	"change_stream":  "mystream",
	"sequence":       "mysequence",
	"model":          "mymodel",
	"schema":         "myschema",
	"param":          "myparam",
}

// completionCase describes one candidate insertion to simulate for a
// completion entry.
type completionCase struct {
	// candidate is the value the fuzzy finder would insert.
	candidate string
	// suffixOverride, when non-empty, replaces the entry's default Suffix
	// (mirroring the per-item fzfItem.Suffix mechanism).
	suffixOverride string
	// continuation is extra input the user would type after the completion to
	// finish the statement (empty when the completed text is already a full
	// statement).
	continuation string
	// expectedSyntax identifies the def the finished statement must dispatch
	// to, by one of its Description.Syntax strings. Empty means the def owning
	// the completion entry.
	expectedSyntax string
}

// completionCaseOverrides lists, keyed by PrefixPattern source, the entries
// whose completed text is not already a full statement of the owning def.
// This encodes the intended completion semantics (see #718 for the SET target
// completion): variables get " = <value>", the PARAM keyword re-dispatches to
// the SET PARAM definitions, and LOCAL to SET LOCAL.
var completionCaseOverrides = map[string][]completionCase{
	// SET target completion: one merged candidate list (variables + PARAM +
	// LOCAL keywords with per-item suffixes).
	`(?i)^\s*SET\s+([^\s=]*)$`: {
		{candidate: "CLI_FORMAT", continuation: "1"},
		{candidate: "PARAM", suffixOverride: " ", continuation: "myparam STRING", expectedSyntax: "SET PARAM <name> <type>"},
		{candidate: "LOCAL", suffixOverride: " ", continuation: "CLI_FORMAT = 1", expectedSyntax: "SET LOCAL <name> = <value>"},
	},
	// SET LOCAL name completion inserts "<name> = " and awaits a value.
	`(?i)^\s*SET\s+LOCAL\s+([^\s=]*)$`: {
		{candidate: "CLI_FORMAT", continuation: "1"},
	},
	// SET PARAM name completion inserts "<name> " and awaits a type.
	`(?i)^\s*SET\s+PARAM\s+([^\s=]*)$`: {
		{candidate: "myparam", continuation: "STRING"},
	},
	// MUTATE table completion awaits the operation and body.
	`(?i)^\s*MUTATE\s+(\S*)$`: {
		{candidate: "mytable", continuation: `INSERT STRUCT<PK STRING>("1")`},
	},
}

var (
	optionalNonCapturingGroupRe = regexp.MustCompile(`\(\?:[^()]*\)\?`)
	alternationGroupRe          = regexp.MustCompile(`\(\?:([^()|]*)\|[^()]*\)`)
	captureGroupRe              = regexp.MustCompile(`\([^?][^()]*\)`)
)

// synthesizePrefixInput derives, from a completion PrefixPattern, a canonical
// user input in the "argument not yet typed" state: literal keywords are
// kept, earlier capture groups (already-typed arguments) are filled with a
// dummy value, and the final capture group (the argument being completed) is
// left empty. The vocabulary handled here is exactly what the PrefixPatterns
// use today; anything else fails the test so this helper must be extended
// alongside new patterns.
func synthesizePrefixInput(t *testing.T, re *regexp.Regexp) string {
	t.Helper()

	s := re.String()
	s = strings.TrimPrefix(s, "(?i)")
	// Optional non-capturing groups (e.g. `(?:.*,\s*)?` in DUMP TABLES) are
	// dropped: the canonical prefix is the simplest accepted shape.
	s = optionalNonCapturingGroupRe.ReplaceAllString(s, "")
	// Non-capturing alternations keep their first alternative.
	s = alternationGroupRe.ReplaceAllString(s, "$1")

	// Fill earlier capture groups with a dummy argument; empty the last one.
	groups := captureGroupRe.FindAllStringIndex(s, -1)
	for idx := len(groups) - 1; idx >= 0; idx-- {
		span := groups[idx]
		fill := "ctx1"
		if idx == len(groups)-1 {
			fill = ""
		}
		s = s[:span[0]] + fill + s[span[1]:]
	}

	s = strings.ReplaceAll(s, `\s+`, " ")
	s = strings.ReplaceAll(s, `\s*`, " ")
	s = strings.ReplaceAll(s, "^", "")
	s = strings.ReplaceAll(s, "$", "")
	s = strings.TrimLeft(s, " ")

	if strings.ContainsAny(s, `\^$(){}[]|+*?`) {
		t.Fatalf("synthesizePrefixInput: unhandled regex construct remains in %q (from pattern %q); extend the helper", s, re)
	}
	return s
}

// findFirstCompletionEntry mirrors detectFuzzyContext's first-match iteration
// and returns the identity of the entry that would handle the input.
func findFirstCompletionEntry(input string) (defIdx, entryIdx int, ok bool) {
	for i, def := range mergedDefs {
		for j, comp := range def.Completion {
			if comp.PrefixPattern.MatchString(input) {
				return i, j, true
			}
		}
	}
	return 0, 0, false
}

// TestClientSideStatementDefsCompletionConsistency asserts that every
// completion entry's hand-written PrefixPattern stays consistent with its
// def's main Pattern: a prefix accepted by the PrefixPattern, completed with
// a plausible candidate plus suffix (and a continuation where the statement
// needs more input), must dispatch to the intended def and parse without
// error. This catches drift between the duplicated regex dialects, and also
// completion-entry shadowing across defs (the entry that detectFuzzyContext
// would pick must be the entry under test).
func TestClientSideStatementDefsCompletionConsistency(t *testing.T) {
	t.Parallel()

	for i, def := range mergedDefs {
		for j, comp := range def.Completion {
			t.Run(fmt.Sprintf("def%02d/%s/entry%d(%s)", i, firstSyntax(def), j, comp.CompletionType), func(t *testing.T) {
				input := synthesizePrefixInput(t, comp.PrefixPattern)

				loc := comp.PrefixPattern.FindStringSubmatchIndex(input)
				if loc == nil {
					t.Fatalf("synthesized input %q does not match its own PrefixPattern %q", input, comp.PrefixPattern)
				}

				// The entry that global first-match completion detection picks
				// for this input must be this entry.
				gotDef, gotEntry, ok := findFirstCompletionEntry(input)
				if !ok || gotDef != i || gotEntry != j {
					t.Fatalf("input %q is handled by completion entry def%d/entry%d, not def%d/entry%d: completion shadowing",
						input, gotDef, gotEntry, i, j)
				}

				// Same extraction as detectFuzzyContext: the last capture
				// group is the argument being completed; the completion
				// replaces the buffer from the group start.
				numGroups := len(loc)/2 - 1
				argStart := loc[numGroups*2]

				cases, hasOverride := completionCaseOverrides[comp.PrefixPattern.String()]
				if !hasOverride {
					sample, ok := completionSampleValues[comp.CompletionType.String()]
					if !ok {
						t.Fatalf("no sample candidate for completion type %s; add it to completionSampleValues or completionCaseOverrides", comp.CompletionType)
					}
					cases = []completionCase{{candidate: sample}}
				}

				for _, c := range cases {
					suffix := comp.Suffix
					if c.suffixOverride != "" {
						suffix = c.suffixOverride
					}
					completed := input[:argStart] + c.candidate + suffix

					full := completed
					if c.continuation != "" {
						if !strings.HasSuffix(full, " ") {
							full += " "
						}
						full += c.continuation
					}

					expectedDef := i
					if c.expectedSyntax != "" {
						expectedDef = defIndexBySyntax(t, c.expectedSyntax)
					}

					if got := firstMatchingDefIndex(full); got != expectedDef {
						t.Errorf("completing %q with candidate %q yields %q, which dispatches to def %d (%s), want def %d (%s): completion pattern drifted from statement pattern",
							input, c.candidate, full, got, syntaxOfIndex(got), expectedDef, firstSyntax(mergedDefs[expectedDef]))
						continue
					}
					if _, err := mycli.BuildCLIStatement(full, full); err != nil {
						t.Errorf("completing %q with candidate %q yields %q, which fails to parse: %v", input, c.candidate, full, err)
					}
				}
			})
		}
	}
}

func syntaxOfIndex(i int) string {
	if i < 0 || i >= len(mergedDefs) {
		return "<no match>"
	}
	return firstSyntax(mergedDefs[i])
}
