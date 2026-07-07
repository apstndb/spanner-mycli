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

// This file defines the Feature registration seam (issue #778). A Feature is a
// plain value passed to Main that contributes client-side statements, system
// variables, CLI flags, and per-session state without the core package
// importing the feature's dependencies. In PR0 the seam is introduced with zero
// behavior change: the three optional families (GEMINI/BIGQUERY/CQL) still live
// in core and Main is called with an empty feature list, so the full binary's
// user-visible surface does not move.
//
// New-feature checklist (enforced at registration through the merged tables and
// the #728 invariant tests): every contributed statement must declare its
// READONLY/detached classification via the marker embeds below, every variable
// must declare its SET policy through FeatureVar, and docs/completion must stay
// consistent.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"sync"

	"google.golang.org/api/option"
)

// StatementDef is the exported alias for the client-side statement definition.
// Its fields (Descriptions, Pattern, HandleGroups, Completion) are already
// exported, so feature packages construct these directly.
type StatementDef = clientSideStatementDef

// StatementDescription is the exported alias for a statement's human-readable
// description (Usage/Syntax/Note).
type StatementDescription = clientSideStatementDescription

// Feature is a single optional statement family contributed to Main. Its
// StatementDefs are appended after the core defs in All() order; its Vars are
// converted into the system-variable registry; its Flags/KongVars are wired
// into the kong parser.
type Feature struct {
	// Name identifies the feature (e.g. "GEMINI", "BIGQUERY", "CQL").
	Name string

	// StatementDefs are the client-side statement definitions the feature adds.
	// They are appended after the core table, so feature keywords must be
	// leading unique tokens (the #728 non-shadowing invariant proves this over
	// the merged table).
	StatementDefs []*StatementDef

	// Vars are the system variables the feature registers. They ride the same
	// varDef machinery (SET policy, RESET, generated docs) as core variables.
	Vars []FeatureVar

	// Flags is an optional pointer to a kong-tagged struct appended to the CLI
	// parser via kong.Plugins. Nil when the feature contributes no flags.
	Flags any

	// KongVars supplies optional help-template values (e.g. default model
	// names) merged into the parser's kong.Vars.
	KongVars map[string]string

	// ApplyFlags, when non-nil, routes parsed flag values into system variables
	// through the registry after flag parsing. The set argument forwards to
	// systemVariables.SetFromSimple, keeping feature flags out of the direct
	// assignment path (consistent with the #725 PR4 direction). It is the seam's
	// flag-application mechanism; no feature uses it until GEMINI's flags move
	// (PR3).
	ApplyFlags func(set func(name, value string) error) error
}

// FeatureVar is the exported declaration shape for a feature-contributed system
// variable. Core converts it into an internal varDef at registry construction
// (see VarRegistry.registerAll); the boolean fields mirror the varDef policy
// flags. The variable's live state is owned by the feature package via Var.
type FeatureVar struct {
	Name string
	Desc string
	Var  Variable

	ReadOnly bool // read-only via SET
	InitOnly bool // settable only before session creation
	TxnGuard bool // SET rejected while a transaction is active
	NoLocal  bool // opt-out of SET LOCAL
	NoReset  bool // opt-out of RESET ALL
}

// toVarDef converts a FeatureVar into the internal declarative varDef. Feature
// variables are always session-scoped (the SET-able surface); their handler is
// the feature-owned Variable, so bind ignores the systemVariables pointer.
func (fv FeatureVar) toVarDef() varDef {
	v := fv.Var
	return varDef{
		name:     fv.Name,
		desc:     fv.Desc,
		scope:    scopeSession,
		readOnly: fv.ReadOnly,
		initOnly: fv.InitOnly,
		txnGuard: fv.TxnGuard,
		noLocal:  fv.NoLocal,
		noReset:  fv.NoReset,
		bind:     func(*systemVariables) Variable { return v },
	}
}

// featureVarDefs converts a feature's variable declarations into internal
// varDefs for registry construction.
func featureVarDefs(features []Feature) []varDef {
	var defs []varDef
	for _, f := range features {
		for _, fv := range f.Vars {
			defs = append(defs, fv.toVarDef())
		}
	}
	return defs
}

// activeStatementDefs is the client-side statement table every consumer
// iterates (dispatch, HELP, --statement-help, fuzzy completion). It defaults to
// the core-only table and is replaced by Main with the merged table once
// features are known, before any REPL/exec path starts.
var activeStatementDefs = clientSideStatementDefs

// MergedStatementDefs returns the core statement table followed by each
// feature's StatementDefs in argument order. With no features it returns a copy
// of the core table, so the merged surface is identical to core.
func MergedStatementDefs(features ...Feature) []*StatementDef {
	defs := slices.Clone(clientSideStatementDefs)
	for _, f := range features {
		defs = append(defs, f.StatementDefs...)
	}
	return defs
}

// BuildStatementWithDefs matches input against the given client-side statement
// defs (first-match-wins, same dispatch as BuildCLIStatement) and builds the
// corresponding Statement. Exported so feature tests can exercise their defs
// directly.
func BuildStatementWithDefs(defs []*StatementDef, input string) (Statement, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil, errors.New("empty statement")
	}
	for _, cs := range defs {
		// FindStringSubmatch returns nil on no match, so MatchString is unnecessary.
		if matches := cs.Pattern.FindStringSubmatch(trimmed); matches != nil {
			return cs.HandleGroups(namedGroups(cs.Pattern, matches))
		}
	}
	return nil, errStatementNotMatched
}

// Marker embed types let feature packages opt their statement types into the
// core marker interfaces whose methods stay unexported (#696): embedding the
// zero-size marker promotes the unexported method, satisfying the interface,
// while accidental satisfaction from outside remains impossible. See the
// alternatives in issue #778 §4.3.

// MarksMutation marks an embedding statement as a MutationStatement.
type MarksMutation struct{}

func (MarksMutation) isMutationStatement() {}

// MarksDetachedCompatible marks an embedding statement as DetachedCompatible.
type MarksDetachedCompatible struct{}

func (MarksDetachedCompatible) isDetachedCompatible() {}

// MutationClassifier marks an embedding statement as a
// ConditionallyMutatingStatement whose mutation-ness is decided at runtime by
// Classify (e.g. CQL/BIGQUERY inspecting the statement's leading keyword).
type MutationClassifier struct {
	Classify func() bool
}

func (m MutationClassifier) isConditionallyMutating() bool { return m.Classify() }

// NewRow builds a result Row from string cells. Exported wrapper over toRow for
// feature packages.
func NewRow(values ...string) Row { return toRow(values...) }

// NewTableHeader builds a simple string-column TableHeader. Exported wrapper
// over toTableHeader for feature packages.
func NewTableHeader(names ...string) TableHeader { return toTableHeader(names...) }

// UnquoteString trims surrounding single or double quotes from a def handler
// argument. Exported wrapper over unquoteString for feature packages.
func UnquoteString(s string) string { return unquoteString(s) }

// ProjectID returns the configured Spanner project for the session.
func (s *Session) ProjectID() string { return s.systemVariables.Connection.Project }

// AuthOptions builds client auth options for the given credential using the
// session's system variables. Exported wrapper over createAuthClientOptions for
// feature packages that build their own (non-Spanner) clients.
func (s *Session) AuthOptions(ctx context.Context, credential []byte, allowWithoutAuthentication bool) ([]option.ClientOption, error) {
	return createAuthClientOptions(ctx, credential, s.systemVariables, allowWithoutAuthentication)
}

// featureStore is a keyed per-Session store for feature state with lifecycle.
// Each key is lazily initialized on first use; values implementing io.Closer
// are closed in reverse creation order at the end of Session.Close.
type featureStore struct {
	mu      sync.Mutex
	entries map[string]*featureEntry
	order   []*featureEntry // creation order; closed in reverse
}

type featureEntry struct {
	key   string
	mu    sync.Mutex // serializes init for this key; held across init
	val   any
	ready bool
}

// entry returns the store entry for key, creating it (and recording creation
// order) on first request. The map lock is held only to find or create the
// entry, never across its init, so concurrent requests for different keys do
// not serialize.
func (st *featureStore) entry(key string) *featureEntry {
	st.mu.Lock()
	defer st.mu.Unlock()
	if e, ok := st.entries[key]; ok {
		return e
	}
	if st.entries == nil {
		st.entries = make(map[string]*featureEntry)
	}
	e := &featureEntry{key: key}
	st.entries[key] = e
	st.order = append(st.order, e)
	return e
}

// closeAll closes every stored value implementing io.Closer in reverse creation
// order. Called at the end of Session.Close, after the core clients.
func (st *featureStore) closeAll() {
	st.mu.Lock()
	order := st.order
	st.mu.Unlock()
	for i := len(order) - 1; i >= 0; i-- {
		e := order[i]
		e.mu.Lock()
		val := e.val
		e.mu.Unlock()
		if c, ok := val.(io.Closer); ok {
			if err := c.Close(); err != nil {
				slog.Error("error closing feature state", "key", e.key, "err", err)
			}
		}
	}
}

// FeatureState returns the per-Session value stored under key, initializing it
// via init on first use. A FAILED init is not cached: the error is returned and
// the next call retries, matching the retry semantics of the lazy per-feature
// fields this store replaces (BigQuery client, CQL session, doc cache) — a
// transient build failure must not poison the session. Concurrent calls for the
// same key serialize (init runs at most once at a time and successful init runs
// exactly once); different keys initialize independently. A value implementing
// io.Closer is closed at the end of Session.Close in reverse creation order.
//
// NOTE: the design sketch in issue #778 §4.6 wrote this signature without a
// context parameter, but init requires one (its real callers, e.g. the lazy
// BigQuery client and doc cache builds, all have a ctx in hand). ctx is
// threaded through here so the guarded init receives it.
func FeatureState[T any](ctx context.Context, s *Session, key string, init func(context.Context, *Session) (T, error)) (T, error) {
	e := s.featureState.entry(key)
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.ready {
		v, err := init(ctx, s)
		if err != nil {
			var zero T
			return zero, err
		}
		e.val, e.ready = v, true
	}
	v, ok := e.val.(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("feature state %q: stored value has unexpected type %T", key, e.val)
	}
	return v, nil
}
