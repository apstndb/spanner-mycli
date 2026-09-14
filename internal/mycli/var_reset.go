// Copyright 2026 apstndb
//
// Licensed under the MIT License.

package mycli

import (
	"fmt"
	"strings"
)

// preparedReset is the validated RESET plan. Prepare fills it without mutating
// the live registry or resource graph; commit applies assignments and reports
// which canonical LOCAL undo entries to retire after the whole operation succeeds.
type preparedReset struct {
	assignments []resetAssignment
	retireUndo  []string
}

type resetAssignment struct {
	name  string
	value string
	v     Variable
}

// CaptureStartupSnapshots records explicit supported startup values after
// defaults, config, flags, and --set. It must run before --init-command and
// --init-command-add: those statements are ordinary SQL and remain resettable.
func (sv *systemVariables) CaptureStartupSnapshots() error {
	sv.ensureRegistry()
	return sv.Registry.captureStartupSnapshots()
}

// Reset restores one canonical name or alias to its captured startup snapshot.
// It does not retire SET LOCAL undo; persistent RESET statements do that after
// the whole operation succeeds. Single-variable parser/help belong to #960.
func (sv *systemVariables) Reset(name string) error {
	sv.ensureRegistry()
	return sv.Registry.Reset(name)
}

// ResetAll restores every captured resettable variable to its startup snapshot.
func (sv *systemVariables) ResetAll() error {
	sv.ensureRegistry()
	return sv.Registry.ResetAll()
}

func (r *VarRegistry) Reset(name string) error {
	prep, err := r.prepareReset([]string{name})
	if err != nil {
		return err
	}
	return r.commitReset(prep)
}

func (r *VarRegistry) ResetAll() error {
	prep, err := r.prepareResetAll()
	if err != nil {
		return err
	}
	return r.commitReset(prep)
}

func (r *VarRegistry) captureStartupSnapshots() error {
	snaps := make(map[string]string)
	var captureErr error
	r.forEachDef(func(def *varDef) {
		if captureErr != nil || !def.resettable() {
			return
		}
		rv := r.vars[strings.ToUpper(def.name)]
		if _, ok := rv.v.(resetPreparer); !ok {
			captureErr = fmt.Errorf("RESET ALL: %s is resettable but has no prepare support", def.name)
			return
		}
		value, err := variableResetValue(rv.v)
		if err != nil {
			captureErr = fmt.Errorf("RESET ALL: capture %s: %w", def.name, err)
			return
		}
		snaps[def.name] = value
	})
	if captureErr != nil {
		return captureErr
	}
	r.sv.startupSnapshots = snaps
	return nil
}

func (r *VarRegistry) prepareResetAll() (*preparedReset, error) {
	if r.sv.startupSnapshots == nil {
		return nil, errResetSnapshotsMissing
	}
	var names []string
	r.forEachDef(func(def *varDef) {
		if !def.resettable() {
			return
		}
		if _, ok := r.sv.startupSnapshots[def.name]; ok {
			names = append(names, def.name)
		}
	})
	return r.prepareReset(names)
}

func (r *VarRegistry) prepareReset(names []string) (*preparedReset, error) {
	if r.sv.startupSnapshots == nil {
		return nil, errResetSnapshotsMissing
	}
	prep := &preparedReset{}
	seen := make(map[string]struct{})
	for _, name := range names {
		def := r.lookupDef(name)
		if def == nil {
			return nil, &ErrUnknownVariable{Name: name}
		}
		if _, dup := seen[def.name]; dup {
			continue
		}
		seen[def.name] = struct{}{}
		if !def.resettable() {
			return nil, fmt.Errorf("%s does not support RESET", def.name)
		}
		snap, ok := r.sv.startupSnapshots[def.name]
		if !ok {
			return nil, fmt.Errorf("%s does not support RESET", def.name)
		}
		rv := r.vars[strings.ToUpper(def.name)]
		current, err := variableResetValue(rv.v)
		if err != nil {
			return nil, fmt.Errorf("RESET %s: %w", def.name, err)
		}
		if current == snap {
			// Equal-value RESET skips assignment and txn/batch guards, but still
			// retires the targeted LOCAL undo after the whole operation succeeds.
			prep.retireUndo = append(prep.retireUndo, def.name)
			continue
		}
		if err := r.checkSetPolicy(def); err != nil {
			return nil, fmt.Errorf("RESET %s: %w", def.name, err)
		}
		preparer, ok := rv.v.(resetPreparer)
		if !ok {
			return nil, fmt.Errorf("RESET %s: %w", def.name, errResetUnsupported)
		}
		if err := preparer.PrepareReset(snap); err != nil {
			return nil, fmt.Errorf("RESET %s: %w", def.name, err)
		}
		prep.assignments = append(prep.assignments, resetAssignment{name: def.name, value: snap, v: rv.v})
		prep.retireUndo = append(prep.retireUndo, def.name)
	}
	return prep, nil
}

func (r *VarRegistry) commitReset(prep *preparedReset) error {
	for _, a := range prep.assignments {
		if err := a.v.Set(a.value); err != nil {
			return fmt.Errorf("RESET %s: %w", a.name, err)
		}
	}
	return nil
}

// variableResetValue is the RESET compare/capture string. Handlers whose SHOW
// value is not the writable setting implement resetSnapshotter.
func variableResetValue(v Variable) (string, error) {
	if s, ok := v.(resetSnapshotter); ok {
		return s.ResetSnapshot()
	}
	return v.Get()
}

func commitPersistentReset(session *Session, prep *preparedReset) error {
	if err := session.systemVariables.Registry.commitReset(prep); err != nil {
		return err
	}
	if session.txn != nil && session.txn.InTransaction() {
		for _, name := range prep.retireUndo {
			session.txn.retireLocalVarUndo(name)
		}
	}
	return nil
}
