// Copyright 2026 apstndb
//
// Licensed under the MIT License.

package mycli

import "testing"

// TestVarDefPredicateConsistency asserts the derived policy predicates stay
// consistent with settable(): a variable that is not settable can be neither
// SET LOCAL-eligible nor RESET-eligible.
func TestVarDefPredicateConsistency(t *testing.T) {
	t.Parallel()
	for i := range varDefs {
		d := &varDefs[i]
		if d.localAllowed() && !d.settable() {
			t.Errorf("%s: localAllowed() true but settable() false", d.name)
		}
		if d.resettable() && !d.settable() {
			t.Errorf("%s: resettable() true but settable() false", d.name)
		}
	}
}

// TestLocalAllowedRoundTrip verifies that for every SET LOCAL-eligible variable,
// the current (default) value round-trips through Registry.Get -> Registry.Set
// without error and without changing the value. SET LOCAL relies on this: the
// saved value is replayed through Registry.Set when the transaction ends, so a
// value that cannot be set back would make restore fail. This replaces the
// pre-flight Set(old) round-trip that SetLocalStatement used to perform.
func TestLocalAllowedRoundTrip(t *testing.T) {
	t.Parallel()
	for i := range varDefs {
		def := &varDefs[i]
		if !def.localAllowed() {
			continue
		}
		t.Run(def.name, func(t *testing.T) {
			t.Parallel()
			sv := newSystemVariablesWithDefaultsForTest()
			sv.ensureRegistry()

			// Unimplemented placeholders (AUTOCOMMIT, RETRY_ABORTS_INTERNALLY)
			// are nominally localAllowed but reject both Get and Set until their
			// runtime behavior lands. SET LOCAL on them fails at the SET itself,
			// so no undo entry is ever recorded and there is no restore hazard.
			if _, ok := sv.Registry.GetVariable(def.name).(*UnimplementedVar); ok {
				t.Skipf("%s is an unimplemented placeholder", def.name)
			}

			before, err := sv.Registry.Get(def.name)
			if err != nil {
				t.Fatalf("Get(%s) error: %v", def.name, err)
			}
			if err := sv.Registry.Set(def.name, before, false); err != nil {
				t.Fatalf("Set(%s, %q) error: %v", def.name, before, err)
			}
			after, err := sv.Registry.Get(def.name)
			if err != nil {
				t.Fatalf("Get(%s) after round-trip error: %v", def.name, err)
			}
			if after != before {
				t.Errorf("%s did not round-trip: before %q, after %q", def.name, before, after)
			}
		})
	}
}
